package plan_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/zlrrr/mutil-agent-system/internal/agent"
	"github.com/zlrrr/mutil-agent-system/internal/agent/plan"
	"github.com/zlrrr/mutil-agent-system/internal/arena"
	"github.com/zlrrr/mutil-agent-system/internal/domain"
	"github.com/zlrrr/mutil-agent-system/internal/reasoner"
)

// sdd:verify TC-0115

// TestCollectionIsPlanned asserts that what the investigation looks at is a decision it
// takes and records, rather than whatever the sources happen to expose.
//
// Before this port, each collector asked its source for everything on offer. So "what did
// round one see" — which determines the first-round error, and therefore what the critic
// has to overturn (REQ-0103) — was a property of the fixture, and the most consequential
// choice in the flow was the one choice no role was accountable for.
func TestCollectionIsPlanned(t *testing.T) {
	ctx := context.Background()

	t.Run("the plans are recorded on the case", func(t *testing.T) {
		c := runReference(t, nil)

		if len(c.Plan.Roles) == 0 {
			t.Error("no investigation plan was recorded; \"why did we look there\" is " +
				"then answerable only by re-running")
		}
		if c.Plan.Window.Start.IsZero() || c.Plan.Window.End.IsZero() {
			t.Error("the recorded plan carries no window")
		}
		if c.Plan.By == "" {
			t.Error("the plan does not name the strategy that produced it")
		}
		if len(c.Queries.Series) == 0 {
			t.Error("no collection plan was recorded")
		}
		if len(c.Queries.LogTerms) == 0 {
			t.Error("the collection plan names no log terms")
		}

		// Every series the *planned* pass analysed must be one the plan asked for.
		// Demand-answered evidence is deliberately outside the plan: a demand names the
		// evidence it wants, which is the whole point of the critic having the power
		// (REQ-0031), and routing it through the plan would let a planner veto the
		// critic.
		planned := map[string]bool{}
		for _, name := range c.Queries.Series {
			planned[name] = true
		}
		for _, e := range c.Evidence {
			if e.Kind != domain.KindMetric || e.Fact("demand") != "" {
				continue
			}
			if s := e.Fact("series"); s != "" && !planned[s] {
				t.Errorf("the planned pass analysed %q, which the plan never asked for", s)
			}
		}
	})

	t.Run("a series no source offers is dropped, and the rest still runs", func(t *testing.T) {
		available := plan.Available{Series: []string{"http_5xx_rate", "db_up"}}
		fallback := plan.Queries{Series: available.Series, LogTerms: plan.DefaultLogTerms}

		got, dropped := plan.Resolve(plan.Queries{
			Series:   []string{"db_up", "imaginary_metric", "http_5xx_rate"},
			LogTerms: []string{"timeout"},
		}, available, fallback)

		if want := []string{"db_up", "http_5xx_rate"}; !reflect.DeepEqual(got.Series, want) {
			t.Errorf("kept %v, want %v", got.Series, want)
		}
		if want := []string{"imaginary_metric"}; !reflect.DeepEqual(dropped, want) {
			t.Errorf("dropped %v, want %v", dropped, want)
		}
		if !reflect.DeepEqual(got.LogTerms, []string{"timeout"}) {
			t.Errorf("the log terms were altered: %v", got.LogTerms)
		}
	})

	t.Run("a plan that would collect nothing falls back", func(t *testing.T) {
		available := plan.Available{Series: []string{"http_5xx_rate"}}
		fallback := plan.Queries{Series: available.Series, LogTerms: plan.DefaultLogTerms}

		got, dropped := plan.Resolve(plan.Queries{Series: []string{"nothing_real"}},
			available, fallback)

		if !reflect.DeepEqual(got.Series, fallback.Series) {
			t.Errorf("got %v, want the deterministic plan %v; a strategy that cannot "+
				"answer must not be able to blind the investigation", got.Series, fallback.Series)
		}
		if len(dropped) != 1 {
			t.Errorf("the dropped name was not reported: %v", dropped)
		}
	})

	t.Run("the deterministic planner asks for everything, sorted", func(t *testing.T) {
		p := plan.NewRulePlanner(30 * time.Minute)
		got, err := p.PlanCollection(ctx, domain.Snapshot{}, plan.Available{
			Series: []string{"z_series", "a_series"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"a_series", "z_series"}; !reflect.DeepEqual(got.Series, want) {
			t.Errorf("got %v, want %v: an unordered plan makes two identical "+
				"investigations produce different records", got.Series, want)
		}
	})

	// The point of the whole refactor: introducing a decision point must not change the
	// decision. The deterministic planner reproduces "ask each source for everything", so
	// the port arrives with the existing suite as its regression check.
	t.Run("the deterministic planner changes nothing about the reference case", func(t *testing.T) {
		withPort := runReference(t, nil)
		explicit := runReference(t, plan.NewRulePlanner(reasoner.DefaultConfig().ChangeLookback))

		if a, b := len(withPort.Evidence), len(explicit.Evidence); a != b {
			t.Fatalf("default planner produced %d evidence items, explicit one %d", a, b)
		}
		for i := range withPort.Evidence {
			if withPort.Evidence[i].ID != explicit.Evidence[i].ID ||
				withPort.Evidence[i].Summary != explicit.Evidence[i].Summary {
				t.Fatalf("evidence %d differs between the implicit and explicit planner", i)
			}
		}
		if a, b := withPort.Hypotheses[0].SignatureID, explicit.Hypotheses[0].SignatureID; a != b {
			t.Errorf("leading hypothesis differs: %s vs %s", a, b)
		}
	})
}

// sdd:verify TC-0116

// TestJudgementBoundary asserts where strategy substitution stops.
//
// ADR-008 deviates from the goal document, which specifies a system prompt for every
// role. A deviation that lives only in prose is one refactor away from being undone by
// someone who reads the goal document and not the ADR, so it is asserted here: the policy
// engine can only gate what it can parse (REQ-0041), and a recovery decision a strategy
// could disagree with stops being evidence.
func TestJudgementBoundary(t *testing.T) {
	t.Run("remediation and verification take no strategy", func(t *testing.T) {
		for name, fn := range map[string]any{
			"Remediation":  agent.Remediation,
			"Verification": agent.Verification,
		} {
			ft := reflect.TypeOf(fn)
			for i := 0; i < ft.NumIn(); i++ {
				in := ft.In(i)
				if in.Name() == "Reasoner" || in.Name() == "Planner" {
					t.Errorf("%s accepts a %s; REQ-0108 keeps this role deterministic",
						name, in.Name())
				}
			}
		}
	})

	t.Run("the proposed action comes from the catalog unchanged", func(t *testing.T) {
		c := runReference(t, nil)
		if len(c.Actions) == 0 {
			t.Fatal("the reference case proposed no action")
		}
		a := c.Actions[0]
		if a.Tool == "" {
			t.Error("the action names no tool")
		}
		if len(a.Args) == 0 {
			t.Error("the action carries no typed arguments; the policy engine can only " +
				"gate what it can parse")
		}
		for k, v := range a.Args {
			if k == "" || v == "" {
				t.Errorf("the action carries an empty argument %q=%q", k, v)
			}
		}
	})

	t.Run("recovery is decided by comparing values", func(t *testing.T) {
		c := runReference(t, nil)
		if len(c.Verifications) == 0 {
			t.Skip("the reference case did not reach verification in this configuration")
		}
		for _, v := range c.Verifications {
			if v.Signal == "" {
				t.Error("a verification names no signal, so it cannot be recomputed")
			}
		}
	})
}

// runReference drives C1 to completion, approving at the gate so the flow finishes.
func runReference(t *testing.T, p plan.Planner) *domain.Case {
	t.Helper()
	b, err := arena.NewFixtureBuild(arena.Params{CaseID: "C1", Planner: p})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	ctx := context.Background()
	c, err := b.Engine.Create(ctx, b.Alert(), domain.ModeMultiWithCritic)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if c, err = b.Engine.Run(ctx, c.ID); err != nil {
		t.Fatalf("run: %v", err)
	}
	for c.Status == domain.StatusAwaitingApproval {
		a, ok := c.PendingAction()
		if !ok {
			break
		}
		c, err = b.Engine.Decide(ctx, c.ID, a.ID, domain.ApprovalDecision{
			Decision: "approved", By: "test", Comment: "approved for the walkthrough",
		})
		if err != nil {
			t.Fatalf("decide: %v", err)
		}
	}
	return c
}
