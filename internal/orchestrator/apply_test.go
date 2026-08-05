package orchestrator_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zlrrr/mutil-agent-system/internal/agent"
	"github.com/zlrrr/mutil-agent-system/internal/domain"
	"github.com/zlrrr/mutil-agent-system/internal/eventbus"
	"github.com/zlrrr/mutil-agent-system/internal/orchestrator"
	"github.com/zlrrr/mutil-agent-system/internal/policy"
	"github.com/zlrrr/mutil-agent-system/internal/reasoner"
	"github.com/zlrrr/mutil-agent-system/internal/signal"
	"github.com/zlrrr/mutil-agent-system/internal/store"
)

// scriptedAgent returns fixed contributions, so the orchestrator's handling of them can
// be asserted without going through a reasoner.
type scriptedAgent struct {
	role     domain.Role
	contribs []domain.Contribution
	delay    time.Duration
	err      error
}

func (s scriptedAgent) Role() domain.Role { return s.role }

func (s scriptedAgent) Run(context.Context, domain.Snapshot) ([]domain.Contribution, error) {
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
	if s.err != nil {
		return nil, s.err
	}
	return s.contribs, nil
}

func evidenceFor(role domain.Role, index int, summary string) domain.Contribution {
	return domain.AddEvidence(role, index, domain.Evidence{
		Kind: domain.KindMetric, Agent: role, Source: "test", Summary: summary,
		RawRef: "test:" + summary, Confidence: 0.5,
		Facts: map[string]string{"series": summary},
	})
}

// scriptedEngine builds an engine whose collectors are fully scripted.
func scriptedEngine(t *testing.T, collectors []agent.Agent) (*orchestrator.Engine, string) {
	t.Helper()
	alert := domain.Alert{
		Name: "ScriptedAlert", Service: "order-api", Severity: "P1",
		StartsAt: time.Date(2026, 7, 26, 10, 7, 0, 0, time.UTC),
	}
	pol := policy.New(policy.DefaultConfig())
	clock := domain.NewLogicalClock(alert.StartsAt, time.Second)
	e := orchestrator.New(orchestrator.Options{
		Agents:   orchestrator.Agents{Collectors: collectors},
		Policy:   pol,
		Executor: policy.NewExecutor(pol, noopActuator{}, clock),
		Store:    store.NewMemory(),
		Bus:      eventbus.New(),
		Config:   reasoner.DefaultConfig(),
		Clock:    clock,
	})
	c, err := e.Create(context.Background(), alert, domain.ModeMultiWithCritic)
	if err != nil {
		t.Fatal(err)
	}
	return e, c.ID
}

type noopActuator struct{}

func (noopActuator) Invoke(context.Context, domain.ActionCall) (signal.ActuationResult, error) {
	return signal.ActuationResult{Outcome: "applied"}, nil
}

// advanceTo drives the case until it reaches a status or runs out of steps.
func advanceTo(t *testing.T, e *orchestrator.Engine, caseID string, target domain.Status) *domain.Case {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < 30; i++ {
		c, err := e.Advance(ctx, caseID)
		if err != nil {
			t.Fatalf("advance: %v", err)
		}
		if c.Status == target || c.Status.Terminal() {
			return c
		}
	}
	t.Fatalf("never reached %s", target)
	return nil
}

// sdd:verify TC-0003
func TestRoleCapabilityEnforced(t *testing.T) {
	// A collector that oversteps: it proposes a hypothesis and raises a critique.
	overreaching := scriptedAgent{role: domain.RoleMetrics, contribs: []domain.Contribution{
		evidenceFor(domain.RoleMetrics, 0, "legitimate_series"),
		{Kind: domain.ContribProposeHypothesis, Role: domain.RoleMetrics, Index: 1,
			Hypothesis: &domain.Hypothesis{
				ID: "h-x", Claim: "I know the answer", Mechanism: "because I say so",
				Supporting: []string{"e-metrics-001"}}},
		{Kind: domain.ContribRaiseCritique, Role: domain.RoleMetrics, Index: 2,
			Critique: &domain.Critique{HypothesisID: "h-x", Challenge: "not my job",
				Verdict: domain.VerdictReject}},
	}}

	e, caseID := scriptedEngine(t, []agent.Agent{overreaching})
	c := advanceTo(t, e, caseID, domain.StatusHypothesising)

	if len(c.Hypotheses) != 0 {
		t.Errorf("a collector's hypothesis was accepted: %+v", c.Hypotheses)
	}
	if len(c.Critiques) != 0 {
		t.Errorf("a collector's critique was accepted: %+v", c.Critiques)
	}
	if len(c.Evidence) != 1 {
		t.Errorf("the legitimate evidence was not applied: %d item(s)", len(c.Evidence))
	}
	if len(c.RejectedContribs) != 2 {
		t.Fatalf("want 2 recorded rejections, got %d: %+v", len(c.RejectedContribs), c.RejectedContribs)
	}
	for _, r := range c.RejectedContribs {
		if r.Role != domain.RoleMetrics {
			t.Errorf("rejection attributed to %s", r.Role)
		}
		if !strings.Contains(r.Reason, "not permitted") {
			t.Errorf("rejection reason = %q", r.Reason)
		}
	}
}

// sdd:verify TC-0021
func TestUnsupportedHypothesisRejected(t *testing.T) {
	// The analysis role is permitted to propose, but not to propose nothing.
	analysis := scriptedAgent{role: domain.RoleAnalysis, contribs: []domain.Contribution{
		{Kind: domain.ContribProposeHypothesis, Role: domain.RoleAnalysis, Index: 0,
			Hypothesis: &domain.Hypothesis{
				Claim: "unsupported", Mechanism: "none", Supporting: []string{"e-does-not-exist"}}},
	}}
	collector := scriptedAgent{role: domain.RoleMetrics, contribs: []domain.Contribution{
		evidenceFor(domain.RoleMetrics, 0, "series_a"),
	}}

	alert := domain.Alert{
		Name: "ScriptedAlert", Service: "order-api", Severity: "P1",
		StartsAt: time.Date(2026, 7, 26, 10, 7, 0, 0, time.UTC),
	}
	pol := policy.New(policy.DefaultConfig())
	clock := domain.NewLogicalClock(alert.StartsAt, time.Second)
	e := orchestrator.New(orchestrator.Options{
		Agents: orchestrator.Agents{
			Collectors: []agent.Agent{collector},
			Analysis:   analysis,
		},
		Policy:   pol,
		Executor: policy.NewExecutor(pol, noopActuator{}, clock),
		Store:    store.NewMemory(),
		Bus:      eventbus.New(),
		Config:   reasoner.DefaultConfig(),
		Clock:    clock,
	})
	created, err := e.Create(context.Background(), alert, domain.ModeMultiWithCritic)
	if err != nil {
		t.Fatal(err)
	}
	c := advanceTo(t, e, created.ID, domain.StatusCriticising)

	if len(c.Hypotheses) != 0 {
		t.Errorf("a hypothesis referencing missing evidence was accepted: %+v", c.Hypotheses)
	}
	if len(c.RejectedHypotheses) == 0 {
		t.Fatal("the rejection was not recorded")
	}
	if !strings.Contains(c.RejectedHypotheses[0].Reason, "e-does-not-exist") {
		t.Errorf("rejection reason = %q, want it to name the missing evidence",
			c.RejectedHypotheses[0].Reason)
	}
}

// sdd:verify TC-0061
func TestCollectionDeterminism(t *testing.T) {
	// Collectors complete in a different order every run; the result must not.
	makeAgents := func(delays []time.Duration) []agent.Agent {
		roles := domain.CollectorRoles
		out := make([]agent.Agent, 0, len(roles))
		for i, role := range roles {
			out = append(out, scriptedAgent{
				role:  role,
				delay: delays[i%len(delays)],
				contribs: []domain.Contribution{
					evidenceFor(role, 0, string(role)+"_first"),
					evidenceFor(role, 1, string(role)+"_second"),
				},
			})
		}
		return out
	}

	var reference []string
	for run := 0; run < 20; run++ {
		delays := []time.Duration{
			time.Duration(run%5) * time.Millisecond,
			time.Duration((run+3)%5) * time.Millisecond,
			time.Duration((run+1)%5) * time.Millisecond,
		}
		e, caseID := scriptedEngine(t, makeAgents(delays))
		c := advanceTo(t, e, caseID, domain.StatusHypothesising)

		got := make([]string, len(c.Evidence))
		for i, ev := range c.Evidence {
			got[i] = ev.ID + "=" + ev.Summary
		}
		if reference == nil {
			reference = got
			continue
		}
		if len(got) != len(reference) {
			t.Fatalf("run %d produced %d evidence items, first run produced %d",
				run, len(got), len(reference))
		}
		for i := range got {
			if got[i] != reference[i] {
				t.Fatalf("run %d differs at position %d: %s vs %s", run, i, got[i], reference[i])
			}
		}
	}
	if len(reference) != len(domain.CollectorRoles)*2 {
		t.Errorf("expected two items per collector, got %d", len(reference))
	}
	// The order follows the declared collector order, not completion order.
	if !strings.HasPrefix(reference[0], "e-metrics-001") {
		t.Errorf("first applied evidence = %s, want the metrics collector first", reference[0])
	}
}

// sdd:verify TC-0073
func TestAgentFailureIsolated(t *testing.T) {
	failing := scriptedAgent{role: domain.RoleLogs, err: context.DeadlineExceeded}
	working := scriptedAgent{role: domain.RoleMetrics, contribs: []domain.Contribution{
		evidenceFor(domain.RoleMetrics, 0, "series_a"),
	}}

	e, caseID := scriptedEngine(t, []agent.Agent{working, failing})
	c := advanceTo(t, e, caseID, domain.StatusHypothesising)

	if len(c.Evidence) != 1 {
		t.Errorf("the surviving collector's evidence was lost: %d item(s)", len(c.Evidence))
	}
	var failed bool
	for _, ev := range c.Timeline {
		if ev.Type == domain.EvAgentFailed && ev.Actor == domain.RoleLogs {
			failed = true
			if !strings.Contains(ev.Summary, "failed") {
				t.Errorf("failure event summary = %q", ev.Summary)
			}
		}
	}
	if !failed {
		t.Error("the failing collector produced no agent_failed event")
	}
	if c.Status == domain.StatusClosed {
		t.Error("one collector failing must not abort the case")
	}
}

// sdd:verify TC-0071
func TestCaseBounds(t *testing.T) {
	// Far more evidence than the retention cap allows.
	var contribs []domain.Contribution
	for i := 0; i < orchestrator.MaxEvidencePerCase+50; i++ {
		contribs = append(contribs, evidenceFor(domain.RoleMetrics, i, "series_"+itoa(i)))
	}
	flood := scriptedAgent{role: domain.RoleMetrics, contribs: contribs}

	e, caseID := scriptedEngine(t, []agent.Agent{flood})
	c := advanceTo(t, e, caseID, domain.StatusHypothesising)

	if len(c.Evidence) > orchestrator.MaxEvidencePerCase {
		t.Errorf("retained %d evidence items, cap is %d",
			len(c.Evidence), orchestrator.MaxEvidencePerCase)
	}
	if c.Truncations == 0 {
		t.Error("reaching the retention cap must be recorded, not silently absorbed")
	}
	var truncated bool
	for _, ev := range c.Timeline {
		if ev.Type == domain.EvEvidenceTruncated {
			truncated = true
		}
	}
	if !truncated {
		t.Error("no evidence_truncated event was recorded")
	}
}

// sdd:verify TC-0061
func TestConcurrentCollectionIsRaceFree(t *testing.T) {
	agents := make([]agent.Agent, 0, len(domain.CollectorRoles))
	for i, role := range domain.CollectorRoles {
		agents = append(agents, scriptedAgent{
			role:     role,
			delay:    time.Duration(i) * time.Millisecond,
			contribs: []domain.Contribution{evidenceFor(role, 0, string(role))},
		})
	}
	e, caseID := scriptedEngine(t, agents)

	// Concurrent readers while the engine advances: the mutex must hold.
	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_, _ = e.Get(context.Background(), caseID)
				}
			}
		}()
	}
	c := advanceTo(t, e, caseID, domain.StatusHypothesising)
	close(stop)
	wg.Wait()

	if len(c.Evidence) != len(domain.CollectorRoles) {
		t.Errorf("evidence count = %d, want %d", len(c.Evidence), len(domain.CollectorRoles))
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
