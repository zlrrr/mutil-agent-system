package orchestrator_test

import (
	"context"
	"testing"

	"github.com/zlrrr/mutil-agent-system/internal/arena"
	"github.com/zlrrr/mutil-agent-system/internal/domain"
	"github.com/zlrrr/mutil-agent-system/internal/orchestrator"
	"github.com/zlrrr/mutil-agent-system/internal/reasoner"
	"github.com/zlrrr/mutil-agent-system/internal/signal/fixture"
)

// sdd:verify TC-0060
func TestTransitionTable(t *testing.T) {
	legal := [][2]domain.Status{
		{domain.StatusCreated, domain.StatusTriaging},
		{domain.StatusCriticising, domain.StatusCollecting},
		{domain.StatusCriticising, domain.StatusRemediating},
		{domain.StatusCriticising, domain.StatusHumanReview},
		{domain.StatusRemediating, domain.StatusAwaitingApproval},
		{domain.StatusAwaitingApproval, domain.StatusExecuting},
		{domain.StatusAwaitingApproval, domain.StatusReporting},
		{domain.StatusExecuting, domain.StatusVerifying},
		{domain.StatusVerifying, domain.StatusCollecting},
		{domain.StatusReporting, domain.StatusClosed},
	}
	for _, tc := range legal {
		if !orchestrator.Legal(tc[0], tc[1]) {
			t.Errorf("%s -> %s should be legal", tc[0], tc[1])
		}
	}

	illegal := [][2]domain.Status{
		{domain.StatusCreated, domain.StatusExecuting},          // no shortcut to acting
		{domain.StatusCollecting, domain.StatusRemediating},     // analysis may not be skipped
		{domain.StatusAwaitingApproval, domain.StatusVerifying}, // approval may not be skipped
		{domain.StatusClosed, domain.StatusCollecting},          // a closed case stays closed
		{domain.StatusHypothesising, domain.StatusRemediating},  // critique may not be skipped
	}
	for _, tc := range illegal {
		if orchestrator.Legal(tc[0], tc[1]) {
			t.Errorf("%s -> %s must not be legal", tc[0], tc[1])
		}
	}
	if len(orchestrator.Successors(domain.StatusClosed)) != 0 {
		t.Error("a closed case must have no successors")
	}
}

// sdd:verify TC-0060
func TestStateMachineBounded(t *testing.T) {
	// A critic that always demands more: without a budget the case would never end.
	b := build(t, func(p *arena.Params) {
		cfg := reasoner.DefaultConfig()
		cfg.MaxRounds = 3
		p.Config = cfg
	})
	ctx := context.Background()
	c, err := b.Engine.Create(ctx, b.Alert(), domain.ModeMultiWithCritic)
	if err != nil {
		t.Fatal(err)
	}

	var rounds int
	for i := 0; i < 60; i++ {
		before := c.Status
		c, err = b.Engine.Advance(ctx, c.ID)
		if err != nil {
			t.Fatalf("advance from %s: %v", before, err)
		}
		if before != domain.StatusCollecting && c.Status == domain.StatusCollecting {
			rounds++
		}
		if c.Status.Terminal() || c.Status == domain.StatusAwaitingApproval {
			break
		}
	}
	if c.Status != domain.StatusAwaitingApproval && !c.Status.Terminal() {
		t.Fatalf("the case did not terminate; last status %s", c.Status)
	}
	if c.Round > 3 {
		t.Errorf("the case ran %d rounds, exceeding the budget of 3", c.Round)
	}
	if rounds > 3 {
		t.Errorf("collection was entered %d times, exceeding the budget", rounds)
	}
}

// sdd:verify TC-0060
func TestBudgetOfOneTerminatesWithoutRemediating(t *testing.T) {
	b := build(t, func(p *arena.Params) {
		cfg := reasoner.DefaultConfig()
		cfg.MaxRounds = 1
		p.Config = cfg
	})
	c := runToHalt(t, b, domain.ModeMultiWithCritic)

	if !c.Status.Terminal() {
		t.Fatalf("status = %s; a one-round budget must still terminate", c.Status)
	}
	if c.Round != 1 {
		t.Errorf("round = %d, want 1", c.Round)
	}
	// The evidence was never completed, so nothing should have been acted upon.
	for _, a := range c.Actions {
		if a.Execution != nil && a.Execution.Outcome == "executed" {
			t.Errorf("an action ran despite the investigation being incomplete: %s", a.Title)
		}
	}
}

// sdd:verify TC-0036
func TestCloseCallEscalates(t *testing.T) {
	// C4 with a one-round budget: the top two are 0.07 apart, inside the margin, and
	// the case must escalate rather than pick the leader.
	//
	// The reference scenario used to serve here, and deliberately no longer can. Its
	// round-one leader now clears the runner-up by more than the margin, because a
	// first answer that is confidently wrong tests more than one that is a coin toss
	// (REQ-0103) — which leaves it unable to demonstrate a near-tie at all.
	b := build(t, func(p *arena.Params) {
		p.CaseID = "C4"
		cfg := reasoner.DefaultConfig()
		cfg.MaxRounds = 1
		p.Config = cfg
	})
	c := runToHalt(t, b, domain.ModeMultiWithCritic)

	if !c.HumanReviewed {
		t.Errorf("a near-tie with no budget left must reach human review; status %s, notes %v",
			c.Status, c.Notes)
	}
	lead, ok := c.Leading()
	if !ok {
		t.Fatal("no leading hypothesis")
	}
	runner, ok := c.Runner()
	if !ok {
		t.Fatal("no runner-up to compare against")
	}
	gap := lead.Breakdown.Total - runner.Breakdown.Total
	if gap >= reasoner.DefaultConfig().CloseCallMargin {
		t.Errorf("gap = %.3f; this test needs a near-tie", gap)
	}
}

// sdd:verify TC-0035
func TestRemediationGuard(t *testing.T) {
	b := build(t)
	c := runToHalt(t, b, domain.ModeMultiWithCritic)

	// The reference case reaches the gate, so the guard passed on real state.
	if c.Status != domain.StatusAwaitingApproval {
		t.Fatalf("status = %s, want awaiting_approval", c.Status)
	}

	cases := []struct {
		name   string
		mutate func(*domain.Case)
		want   string
	}{
		{
			// Every candidate, not just the leader: the case picks the best-scoring
			// explanation the critic has not rejected, so leaving an admissible one
			// behind would simply promote it. The guard's verdict branch is reached
			// when nothing admissible remains, which is the state this constructs.
			name: "verdict does not permit",
			mutate: func(c *domain.Case) {
				for i := range c.Hypotheses {
					c.Hypotheses[i].Verdict = domain.VerdictRevise
				}
			},
			want: "verdict",
		},
		{
			name: "score below the acceptance threshold",
			mutate: func(c *domain.Case) {
				c.Hypotheses[0].Breakdown.Total = 0.5
			},
			want: "threshold",
		},
		{
			name: "too few evidence kinds",
			mutate: func(c *domain.Case) {
				c.Hypotheses[0].Supporting = c.Hypotheses[0].Supporting[:1]
			},
			want: "evidence kind",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clone := cloneCase(t, b, c.ID)
			tc.mutate(clone)
			ok, reason := b.Engine.CanRemediate(clone)
			if ok {
				t.Fatalf("the guard admitted a case it should refuse (%s)", tc.name)
			}
			if !containsFold(reason, tc.want) {
				t.Errorf("reason = %q, want it to mention %q", reason, tc.want)
			}
		})
	}
}

// sdd:verify TC-0021
func TestGuardRequiresChangeEvidenceWhenAChangeExists(t *testing.T) {
	b := build(t)
	c := runToHalt(t, b, domain.ModeMultiWithCritic)

	clone := cloneCase(t, b, c.ID)
	// Strip the change evidence from the leading hypothesis's support while leaving
	// the change itself in the case: the conclusion no longer accounts for it.
	index := clone.EvidenceIndex()
	var kept []string
	for _, id := range clone.Hypotheses[0].Supporting {
		if index[id].Kind != domain.KindChange {
			kept = append(kept, id)
		}
	}
	clone.Hypotheses[0].Supporting = kept

	ok, reason := b.Engine.CanRemediate(clone)
	if ok {
		t.Fatal("the guard admitted a conclusion that ignores a recorded change")
	}
	if !containsFold(reason, "change evidence") {
		t.Errorf("reason = %q, want it to mention change evidence", reason)
	}
}

// cloneCase replays a case into an independent projection so a test can mutate it
// without disturbing the engine's own state.
func cloneCase(t *testing.T, b *arena.Build, caseID string) *domain.Case {
	t.Helper()
	events, err := b.Engine.Store().Events(context.Background(), caseID, 0)
	if err != nil {
		t.Fatal(err)
	}
	c, err := domain.Replay(caseID, events)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func containsFold(haystack, needle string) bool {
	return len(haystack) >= len(needle) && indexFold(haystack, needle) >= 0
}

func indexFold(s, sub string) int {
	ls, lsub := lower(s), lower(sub)
	for i := 0; i+len(lsub) <= len(ls); i++ {
		if ls[i:i+len(lsub)] == lsub {
			return i
		}
	}
	return -1
}

func lower(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}

// sdd:verify TC-0052
func TestFailedRecoveryReturns(t *testing.T) {
	// A recording actuator with no inner target accepts the call but never flips the
	// environment to recovered, so verification observes no improvement.
	recorder := &fixture.RecordingActuator{}
	b := build(t, func(p *arena.Params) { p.Actuator = recorder })
	c := runToHalt(t, b, domain.ModeMultiWithCritic)

	a, ok := c.PendingAction()
	if !ok {
		t.Fatalf("the case did not reach the approval gate (status %s)", c.Status)
	}
	acted := a.HypothesisID

	c, err := b.Engine.Decide(context.Background(), c.ID, a.ID, domain.ApprovalDecision{
		Decision: "approved", By: "demo-operator",
	})
	if err != nil {
		t.Fatal(err)
	}
	if recorder.Count() != 1 {
		t.Fatalf("the actuator was invoked %d time(s), want 1", recorder.Count())
	}

	recovered, any := c.Recovered()
	if !any {
		t.Fatal("no verification was recorded")
	}
	if recovered {
		t.Fatal("this test needs a run in which the signals do not recover")
	}

	// The hypothesis that motivated the action is challenged, not left standing.
	var challenged bool
	for _, h := range c.Hypotheses {
		if h.ID == acted && h.Status == domain.HypothesisChallenged {
			challenged = true
		}
	}
	if !challenged {
		t.Errorf("the acted-upon hypothesis %s was not marked challenged", acted)
	}

	// With budget remaining the case returns to collection; either way it terminates.
	var returned bool
	for _, ev := range c.Timeline {
		if ev.Type == domain.EvStateChanged &&
			containsFold(ev.Summary, "verifying -> collecting") {
			returned = true
		}
	}
	if c.Round < reasoner.DefaultConfig().MaxRounds && !returned {
		t.Error("with budget remaining a failed recovery must return to investigation")
	}
	if !c.Status.Terminal() {
		t.Errorf("status = %s; the case must still terminate", c.Status)
	}
}
