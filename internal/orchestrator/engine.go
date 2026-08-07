package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/zlrrr/mutil-agent-system/internal/agent"
	"github.com/zlrrr/mutil-agent-system/internal/domain"
	"github.com/zlrrr/mutil-agent-system/internal/eventbus"
	"github.com/zlrrr/mutil-agent-system/internal/policy"
	"github.com/zlrrr/mutil-agent-system/internal/reasoner"
	"github.com/zlrrr/mutil-agent-system/internal/store"
)

// sdd:impl DLD-1063

// Agents is the set of role agents an engine drives.
type Agents struct {
	Collectors   []agent.Agent
	Analysis     agent.Agent
	Critic       agent.Agent
	Remediation  agent.Agent
	Verification agent.Agent
	Baseline     agent.Agent
}

// Engine drives a case through the state machine. It is the only writer of case state
// and the only caller of the executor.
type Engine struct {
	agents   Agents
	policy   *policy.Engine
	executor *policy.Executor
	store    store.Store
	bus      *eventbus.Broker
	cfg      reasoner.Config
	clock    domain.Clock

	mu    sync.Mutex
	cases map[string]*domain.Case
}

// Options configures an engine.
type Options struct {
	Agents   Agents
	Policy   *policy.Engine
	Executor *policy.Executor
	Store    store.Store
	Bus      *eventbus.Broker
	Config   reasoner.Config
	Clock    domain.Clock
}

// New builds an engine.
func New(o Options) *Engine {
	if o.Bus == nil {
		o.Bus = eventbus.New()
	}
	if o.Clock == nil {
		o.Clock = domain.WallClock{}
	}
	return &Engine{
		agents: o.Agents, policy: o.Policy, executor: o.Executor,
		store: o.Store, bus: o.Bus, cfg: o.Config, clock: o.Clock,
		cases: map[string]*domain.Case{},
	}
}

// Bus exposes the broker so an HTTP handler can subscribe.
func (e *Engine) Bus() *eventbus.Broker { return e.bus }

// Store exposes the store for read paths.
func (e *Engine) Store() store.Store { return e.store }

// Config returns the active reasoning configuration.
func (e *Engine) Config() reasoner.Config { return e.cfg }

// Create opens a case from an alert and records its first event.
func (e *Engine) Create(ctx context.Context, alert domain.Alert, mode domain.Mode) (*domain.Case, error) {
	if mode == "" {
		mode = domain.ModeMultiWithCritic
	}
	id := domain.CaseID(alert)

	e.mu.Lock()
	defer e.mu.Unlock()
	if _, exists := e.cases[id]; exists {
		return nil, fmt.Errorf("case %s already exists", id)
	}

	c := domain.NewCase(id)
	if err := e.store.CreateCase(ctx, store.CaseHeader{
		ID: id, AlertName: alert.Name, Service: alert.Service,
		Severity: alert.Severity, Mode: mode, Status: domain.StatusCreated,
		CreatedAt: e.clock.Now(),
	}); err != nil {
		return nil, err
	}
	e.cases[id] = c

	ev := e.event(c, domain.RoleOrchestrator, domain.EvCaseCreated,
		fmt.Sprintf("%s raised for %s (%s)", alert.Name, alert.Service, alert.Severity),
		id, map[string]any{"alert": alert, "mode": mode})
	if err := e.commit(ctx, c, []domain.Event{ev}); err != nil {
		return nil, err
	}
	return c, nil
}

// Get returns the live projection of a case, loading it from the store when this
// process has not seen it before.
func (e *Engine) Get(ctx context.Context, caseID string) (*domain.Case, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.get(ctx, caseID)
}

func (e *Engine) get(ctx context.Context, caseID string) (*domain.Case, error) {
	if c, ok := e.cases[caseID]; ok {
		return c, nil
	}
	c, err := store.Load(ctx, e.store, caseID)
	if err != nil {
		return nil, err
	}
	e.cases[caseID] = c
	return c, nil
}

// Run advances the case until it reaches a terminal state or halts for a human
// decision. It never blocks waiting for approval: it returns, and the caller resumes
// through Decide.
func (e *Engine) Run(ctx context.Context, caseID string) (*domain.Case, error) {
	limit := e.cfg.MaxRounds * 12 // defence against a transition-table mistake
	for i := 0; i < limit; i++ {
		c, err := e.Advance(ctx, caseID)
		if err != nil {
			return nil, err
		}
		if c.Status.Terminal() || c.Status == domain.StatusAwaitingApproval {
			return c, nil
		}
	}
	return nil, fmt.Errorf("case %s did not terminate within %d steps", caseID, limit)
}

// Advance performs exactly one step of the state machine.
func (e *Engine) Advance(ctx context.Context, caseID string) (*domain.Case, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	c, err := e.get(ctx, caseID)
	if err != nil {
		return nil, err
	}

	var events []domain.Event
	var next domain.Status
	reason := ""

	switch c.Status {
	case domain.StatusCreated:
		next = domain.StatusTriaging
		reason = "alert accepted"

	case domain.StatusTriaging:
		started := e.clock.Now()
		plan := e.investigationPlan(c)
		triaged := e.event(c, domain.RoleOrchestrator, domain.EvAgentCompleted,
			fmt.Sprintf("triage: %s severity %s over %s; plan: %s",
				c.Alert.Service, c.Alert.Severity, c.Window(), plan), "", nil)
		triaged.Duration = e.clock.Now().Sub(started)
		events = append(events, triaged)
		next = domain.StatusCollecting
		reason = "investigation plan prepared"

	case domain.StatusCollecting:
		evs, err := e.runCollection(ctx, c)
		if err != nil {
			return nil, err
		}
		events = append(events, evs...)
		next = domain.StatusHypothesising
		reason = "evidence collected"

	case domain.StatusHypothesising:
		if c.Mode != domain.ModeSingle && e.agents.Analysis != nil {
			evs, err := e.runAgent(ctx, c, e.agents.Analysis)
			if err != nil {
				return nil, err
			}
			events = append(events, evs...)
		}
		next = domain.StatusCriticising
		reason = "hypotheses formed"

	case domain.StatusCriticising:
		if c.Mode == domain.ModeMultiWithCritic && e.agents.Critic != nil {
			evs, err := e.runAgent(ctx, c, e.agents.Critic)
			if err != nil {
				return nil, err
			}
			events = append(events, evs...)
		}
		// The critique may have changed the picture, so decide against a case that
		// already includes it.
		if err := e.commitAndClear(ctx, c, &events); err != nil {
			return nil, err
		}
		next, reason, events = e.afterCritique(c)

	case domain.StatusHumanReview:
		if ok, _ := e.CanRemediate(c); ok {
			next, reason = domain.StatusRemediating, "human review accepted the leading hypothesis"
		} else {
			next, reason = domain.StatusReporting, "human review found no acceptable hypothesis"
		}

	case domain.StatusRemediating:
		evs, err := e.runRemediation(ctx, c)
		if err != nil {
			return nil, err
		}
		events = append(events, evs...)
		if err := e.commitAndClear(ctx, c, &events); err != nil {
			return nil, err
		}
		switch {
		case hasPendingApproval(c):
			next, reason = domain.StatusAwaitingApproval, "a human decision is required"
		case hasExecutableAction(c):
			next, reason = domain.StatusExecuting, "low-risk action may run automatically"
		default:
			next, reason = domain.StatusReporting, "no action could be proposed"
		}

	case domain.StatusAwaitingApproval:
		a, ok := approvedAction(c)
		if !ok {
			next, reason = domain.StatusReporting, "the proposed action was not approved"
		} else {
			next, reason = domain.StatusExecuting, "approval recorded for "+a.ID
		}

	case domain.StatusExecuting:
		evs := e.runExecution(ctx, c)
		events = append(events, evs...)
		next, reason = domain.StatusVerifying, "action attempted"

	case domain.StatusVerifying:
		if e.agents.Verification != nil {
			evs, err := e.runAgent(ctx, c, e.agents.Verification)
			if err != nil {
				return nil, err
			}
			events = append(events, evs...)
		}
		if err := e.commitAndClear(ctx, c, &events); err != nil {
			return nil, err
		}
		recovered, any := c.Recovered()
		switch {
		case any && !recovered:
			// An action that did not work is evidence against the hypothesis that
			// motivated it, so the hypothesis is challenged rather than left standing
			// (REQ-0051).
			events = append(events, e.challengeActedHypothesis(c)...)
			if e.budgetRemaining(c) {
				next, reason = domain.StatusCollecting,
					"the signals did not recover; returning to investigation"
			} else {
				next, reason = domain.StatusReporting,
					"the signals did not recover and the round budget is exhausted"
			}
		default:
			next, reason = domain.StatusReporting, "recovery verified"
		}

	case domain.StatusReporting:
		events = append(events, e.event(c, domain.RoleOrchestrator, domain.EvReportGenerated,
			"root cause report generated", "", nil))
		next, reason = domain.StatusClosed, "investigation closed"

	case domain.StatusClosed:
		return c, nil
	}

	if next != "" {
		if !Legal(c.Status, next) {
			events = append(events, e.event(c, domain.RoleOrchestrator, domain.EvContributionRejected,
				fmt.Sprintf("illegal transition %s -> %s refused", c.Status, next), "",
				domain.Rejection{Reason: "transition is not declared in the state machine"}))
			if err := e.commit(ctx, c, events); err != nil {
				return nil, err
			}
			return c, fmt.Errorf("illegal transition %s -> %s", c.Status, next)
		}
		if next == domain.StatusCollecting {
			events = append(events, e.event(c, domain.RoleOrchestrator, domain.EvRoundStarted,
				fmt.Sprintf("collection round %d of %d", c.Round+1, e.cfg.MaxRounds), "",
				domain.RoundStart{Round: c.Round + 1}))
		}
		events = append(events, e.event(c, domain.RoleOrchestrator, domain.EvStateChanged,
			fmt.Sprintf("%s -> %s: %s", c.Status, next, reason), "",
			domain.StateChange{From: c.Status, To: next, Reason: reason}))
		if next == domain.StatusClosed {
			events = append(events, e.event(c, domain.RoleOrchestrator, domain.EvCaseClosed,
				"case closed", "", nil))
		}
	}

	if err := e.commit(ctx, c, events); err != nil {
		return nil, err
	}
	if err := e.store.SetStatus(ctx, c.ID, c.Status); err != nil {
		return nil, err
	}
	return c, nil
}

// challengeActedHypothesis marks the hypothesis behind a failed remediation as
// challenged. Acting on an explanation and observing no recovery is a result, and it
// belongs on the hypothesis rather than only in the verification record.
func (e *Engine) challengeActedHypothesis(c *domain.Case) []domain.Event {
	action, ok := c.ExecutedAction()
	if !ok {
		return nil
	}
	for _, h := range c.Hypotheses {
		if h.ID != action.HypothesisID || h.Status == domain.HypothesisChallenged {
			continue
		}
		updated := h
		updated.Status = domain.HypothesisChallenged
		return []domain.Event{e.event(c, domain.RoleVerification, domain.EvHypothesisScored,
			fmt.Sprintf("%q is challenged: acting on it did not restore the signals", h.Claim),
			h.ID, updated)}
	}
	return nil
}

// investigationPlan names the roles triage will fan out to, so the plan is visible in
// the timeline rather than implied by what happens next.
func (e *Engine) investigationPlan(c *domain.Case) string {
	if c.Mode == domain.ModeSingle {
		return string(domain.RoleBaseline)
	}
	names := make([]string, 0, len(e.agents.Collectors))
	for _, a := range e.agents.Collectors {
		names = append(names, string(a.Role()))
	}
	return strings.Join(names, ", ")
}

// afterCritique decides where a criticised case goes next. It is the junction where
// the critic's power becomes control flow rather than commentary.
func (e *Engine) afterCritique(c *domain.Case) (domain.Status, string, []domain.Event) {
	var events []domain.Event
	open := c.OpenDemands()

	if len(open) > 0 && e.budgetRemaining(c) {
		return domain.StatusCollecting,
			fmt.Sprintf("the critic requires %d further piece(s) of evidence", len(open)),
			events
	}
	// Everything still unanswered is reported, whether the budget ran out or the
	// collection round simply found nothing. Both are honest outcomes; silently
	// dropping either is not (REQ-0031).
	if unsatisfied := c.UnsatisfiedDemands(); len(unsatisfied) > 0 {
		for _, d := range unsatisfied {
			reason := "round budget exhausted"
			if d.Round < c.Round {
				reason = "no source in this case could answer it"
			}
			events = append(events, e.event(c, domain.RoleOrchestrator, domain.EvDemandUnmet,
				"unmet demand: "+d.Descriptor, d.ID,
				domain.Rejection{Target: d.Descriptor, Reason: reason}))
		}
		events = append(events, e.event(c, domain.RoleOrchestrator, domain.EvBudgetExhausted,
			fmt.Sprintf("%d evidence demand(s) remain unmet after %d rounds",
				len(unsatisfied), c.Round), "",
			domain.Rejection{Reason: fmt.Sprintf("%d unmet demand(s)", len(unsatisfied))}))
	}

	if ok, _ := e.CanRemediate(c); ok {
		return domain.StatusRemediating, "the leading hypothesis meets the acceptance condition", events
	}
	// A near-tie with the budget spent is escalated rather than guessed.
	if lead, ok := c.Leading(); ok && !c.HumanReviewed {
		if runner, ok2 := c.Runner(); ok2 {
			if lead.Breakdown.Total-runner.Breakdown.Total < e.cfg.CloseCallMargin {
				return domain.StatusHumanReview,
					"the top two hypotheses are too close to separate automatically", events
			}
		}
	}
	_, why := e.CanRemediate(c)
	return domain.StatusReporting, why, events
}

// runCollection fans the collectors out concurrently and gathers their contributions
// into a fixed order, so completion order cannot reach the result (REQ-0061).
func (e *Engine) runCollection(ctx context.Context, c *domain.Case) ([]domain.Event, error) {
	agents := e.agents.Collectors
	if c.Mode == domain.ModeSingle {
		if e.agents.Baseline == nil {
			return nil, fmt.Errorf("single mode requires a baseline agent")
		}
		agents = []agent.Agent{e.agents.Baseline}
	}

	snap := c.Snapshot(e.cfg.MaxRounds)
	results := make([][]domain.Contribution, len(agents))
	errs := make([]error, len(agents))

	var wg sync.WaitGroup
	for i, a := range agents {
		wg.Add(1)
		go func(i int, a agent.Agent) {
			defer wg.Done()
			results[i], errs[i] = a.Run(ctx, snap)
		}(i, a)
	}
	wg.Wait()

	var events []domain.Event
	var contribs []domain.Contribution
	for i, a := range agents {
		started := e.clock.Now()
		if errs[i] != nil {
			// One collector failing does not abort the case; the round continues.
			events = append(events, e.event(c, a.Role(), domain.EvAgentFailed,
				fmt.Sprintf("%s failed: %v", a.Role(), errs[i]), "", nil))
			continue
		}
		events = append(events, e.event(c, a.Role(), domain.EvAgentStarted,
			fmt.Sprintf("%s began collecting", a.Role()), "", nil))
		ev := e.event(c, a.Role(), domain.EvAgentCompleted,
			fmt.Sprintf("%s returned %d contribution(s)", a.Role(), len(results[i])), "", nil)
		ev.Duration = e.clock.Now().Sub(started)
		events = append(events, ev)
		contribs = append(contribs, results[i]...)
	}
	return append(events, e.applyAfter(c, events, contribs)...), nil
}

// applyAfter folds contributions after the agent-lifecycle events have been built, so
// identifiers are assigned in the deterministic contribution order.
func (e *Engine) applyAfter(c *domain.Case, _ []domain.Event, contribs []domain.Contribution) []domain.Event {
	return e.apply(c, contribs)
}

func (e *Engine) runAgent(ctx context.Context, c *domain.Case, a agent.Agent) ([]domain.Event, error) {
	started := e.clock.Now()
	events := []domain.Event{e.event(c, a.Role(), domain.EvAgentStarted,
		fmt.Sprintf("%s started", a.Role()), "", nil)}

	contribs, err := a.Run(ctx, c.Snapshot(e.cfg.MaxRounds))
	if err != nil {
		events = append(events, e.event(c, a.Role(), domain.EvAgentFailed,
			fmt.Sprintf("%s failed: %v", a.Role(), err), "", nil))
		return events, nil
	}
	done := e.event(c, a.Role(), domain.EvAgentCompleted,
		fmt.Sprintf("%s returned %d contribution(s)", a.Role(), len(contribs)), "", nil)
	done.Duration = e.clock.Now().Sub(started)
	events = append(events, done)
	return append(events, e.apply(c, contribs)...), nil
}

func (e *Engine) runRemediation(ctx context.Context, c *domain.Case) ([]domain.Event, error) {
	if e.agents.Remediation == nil {
		return nil, nil
	}
	// A single-agent run has already proposed its action; do not propose a second.
	if lead, ok := c.Leading(); ok {
		for _, a := range c.Actions {
			if a.HypothesisID == lead.ID {
				return nil, nil
			}
		}
	}
	return e.runAgent(ctx, c, e.agents.Remediation)
}

// runExecution is the only place an actuator is reached, and it goes through the
// executor, which re-evaluates policy first (ARC-010).
func (e *Engine) runExecution(ctx context.Context, c *domain.Case) []domain.Event {
	a, ok := approvedAction(c)
	if !ok {
		a, ok = lowRiskAction(c)
	}
	if !ok {
		return []domain.Event{e.event(c, domain.RoleExecutor, domain.EvActionRefused,
			"no action is eligible for execution", "",
			domain.ExecutionResult{Outcome: "refused", Detail: "no eligible action", At: e.clock.Now()})}
	}

	res := e.executor.Execute(ctx, a)
	typ := domain.EvActionExecuted
	summary := fmt.Sprintf("%s: %s", a.Title, res.Detail)
	if res.Outcome != "executed" {
		typ = domain.EvActionRefused
		summary = fmt.Sprintf("%s refused (%s): %s", a.Title, res.Rule, res.Detail)
	}
	return []domain.Event{e.event(c, domain.RoleExecutor, typ, summary, a.ID, res)}
}

// Decide records an approval decision and resumes the case.
func (e *Engine) Decide(ctx context.Context, caseID, actionID string, d domain.ApprovalDecision) (*domain.Case, error) {
	e.mu.Lock()
	c, err := e.get(ctx, caseID)
	if err != nil {
		e.mu.Unlock()
		return nil, err
	}

	var target *domain.Action
	for i := range c.Actions {
		if c.Actions[i].ID == actionID {
			target = &c.Actions[i]
			break
		}
	}
	if target == nil {
		e.mu.Unlock()
		return nil, fmt.Errorf("action %s not found in case %s", actionID, caseID)
	}
	if d.At.IsZero() {
		d.At = e.clock.Now()
	}
	// Bind the decision to the exact action that was shown to the approver.
	d.Fingerprint = target.Fingerprint()

	ev := e.event(c, domain.RoleOperator, domain.EvApprovalRecorded,
		fmt.Sprintf("%s %s the action: %s", d.By, d.Decision, d.Comment), actionID,
		domain.ApprovalRecord{ActionID: actionID, Decision: d})
	if err := e.commit(ctx, c, []domain.Event{ev}); err != nil {
		e.mu.Unlock()
		return nil, err
	}
	e.mu.Unlock()

	return e.Run(ctx, caseID)
}

// ---------------------------------------------------------------------- helpers

func hasPendingApproval(c *domain.Case) bool {
	_, ok := c.PendingAction()
	return ok
}

func hasExecutableAction(c *domain.Case) bool {
	_, ok := lowRiskAction(c)
	return ok
}

func lowRiskAction(c *domain.Case) (domain.Action, bool) {
	for _, a := range c.Actions {
		if a.Risk == domain.RiskLow && a.Execution == nil {
			return a, true
		}
	}
	return domain.Action{}, false
}

func approvedAction(c *domain.Case) (domain.Action, bool) {
	for _, a := range c.Actions {
		if a.Approval != nil && a.Approval.Approved() && a.Execution == nil {
			return a, true
		}
	}
	return domain.Action{}, false
}

// event builds an unsequenced event; commit assigns the sequence number.
func (e *Engine) event(c *domain.Case, actor domain.Role, t domain.EventType, summary, ref string, payload any) domain.Event {
	return domain.Event{
		CaseID: c.ID, Actor: actor, Type: t, Summary: summary, Ref: ref,
		At: e.clock.Now(), Payload: domain.MustPayload(payload),
	}
}

// commit numbers the batch, folds it into the projection, persists it and publishes it.
// Numbering here — rather than at construction — keeps sequence assignment in one place.
func (e *Engine) commit(ctx context.Context, c *domain.Case, events []domain.Event) error {
	if len(events) == 0 {
		return nil
	}
	for i := range events {
		events[i].Seq = c.LastSeq + 1 + i
	}
	for _, ev := range events {
		if err := c.Apply(ev); err != nil {
			return fmt.Errorf("apply event %d to %s: %w", ev.Seq, c.ID, err)
		}
	}
	if err := e.store.Append(ctx, c.ID, events); err != nil {
		return err
	}
	e.bus.Publish(events...)
	return nil
}

// commitAndClear flushes the events built so far, so a subsequent decision sees a case
// that already includes them.
func (e *Engine) commitAndClear(ctx context.Context, c *domain.Case, events *[]domain.Event) error {
	if err := e.commit(ctx, c, *events); err != nil {
		return err
	}
	*events = nil
	return nil
}

// DefaultClock returns the deterministic clock used by the fixture profile: it starts
// at the alert time and advances one second per event, so every timestamp in a run is a
// pure function of the run's inputs (REQ-0090).
func DefaultClock(alert domain.Alert) domain.Clock {
	return domain.NewLogicalClock(alert.StartsAt.UTC(), time.Second)
}
