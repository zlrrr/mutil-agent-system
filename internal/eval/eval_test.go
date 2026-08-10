package eval_test

import (
	"context"
	"strings"
	"testing"

	"time"

	"github.com/zlrrr/mutil-agent-system/internal/agent/plan"
	"github.com/zlrrr/mutil-agent-system/internal/catalog"
	"github.com/zlrrr/mutil-agent-system/internal/domain"
	"github.com/zlrrr/mutil-agent-system/internal/eval"
	"github.com/zlrrr/mutil-agent-system/internal/reasoner"
)

func loadCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	cat, err := catalog.Load()
	if err != nil {
		t.Fatal(err)
	}
	return cat
}

// sdd:verify TC-0080
func TestThreeModesOverIdenticalInputs(t *testing.T) {
	cat := loadCatalog(t)
	ctx := context.Background()

	results := map[domain.Mode]eval.Outcome{}
	for _, mode := range eval.Modes {
		o, err := eval.RunCase(ctx, cat, "C1", mode, eval.Options{})
		if err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
		if o.Failure != "" {
			t.Fatalf("%s failed: %s", mode, o.Failure)
		}
		if o.Mode != mode {
			t.Errorf("outcome records mode %s, want %s", o.Mode, mode)
		}
		if o.Expected != "sig-db-pool-exhaustion" {
			t.Errorf("expected signature = %q", o.Expected)
		}
		results[mode] = o
	}

	if results[domain.ModeSingle].Top1Correct {
		t.Error("the single-agent mode reached the correct root cause; " +
			"the comparison would demonstrate nothing")
	}
	if results[domain.ModeMultiNoCritic].Top1Correct {
		t.Error("the no-critic mode reached the correct root cause")
	}
	if !results[domain.ModeMultiWithCritic].Top1Correct {
		t.Errorf("the full flow did not reach the correct root cause, got %q",
			results[domain.ModeMultiWithCritic].Top1)
	}

	// The difference must be attributable to the flow, not to the evidence available:
	// all three modes ran against the same fixture.
	full := results[domain.ModeMultiWithCritic]
	single := results[domain.ModeSingle]
	if full.Rounds <= single.Rounds {
		t.Errorf("the full flow ran %d rounds and the single agent %d; the adversarial "+
			"round is what differs", full.Rounds, single.Rounds)
	}
	if full.CriticCorrections == 0 {
		t.Error("the full flow recorded no critic corrections")
	}
	if single.CriticCorrections != 0 {
		t.Error("the single-agent mode recorded critic corrections; it has no critic")
	}
}

// sdd:verify TC-0081
func TestEvaluationSummary(t *testing.T) {
	cat := loadCatalog(t)
	rep, err := eval.RunAll(context.Background(), cat, nil)
	if err != nil {
		t.Fatal(err)
	}

	if len(rep.Outcomes) != len(eval.Modes)*len(cat.Cases) {
		t.Fatalf("ran %d outcomes, want %d modes x %d cases",
			len(rep.Outcomes), len(eval.Modes), len(cat.Cases))
	}
	if len(rep.Summaries) != len(eval.Modes) {
		t.Fatalf("want a summary per mode, got %d", len(rep.Summaries))
	}

	byMode := map[domain.Mode]eval.ModeSummary{}
	for _, s := range rep.Summaries {
		byMode[s.Mode] = s
		if s.Samples != len(cat.Cases) {
			t.Errorf("%s reports %d samples, want %d", s.Mode, s.Samples, len(cat.Cases))
		}
		if s.Failures != 0 {
			t.Errorf("%s recorded %d failed run(s)", s.Mode, s.Failures)
		}
		if s.MeanEvidenceKinds <= 0 {
			t.Errorf("%s gathered no evidence", s.Mode)
		}
		if s.Top1Accuracy < 0 || s.Top1Accuracy > 1 {
			t.Errorf("%s top-1 accuracy = %v, must be a rate", s.Mode, s.Top1Accuracy)
		}
	}

	full := byMode[domain.ModeMultiWithCritic]
	single := byMode[domain.ModeSingle]
	if full.Top1Accuracy <= single.Top1Accuracy {
		t.Errorf("the full flow's top-1 accuracy (%.2f) does not exceed the single agent's (%.2f)",
			full.Top1Accuracy, single.Top1Accuracy)
	}
	if full.MeanEvidenceKinds <= single.MeanEvidenceKinds {
		t.Errorf("the full flow gathered %.1f evidence kinds and the single agent %.1f",
			full.MeanEvidenceKinds, single.MeanEvidenceKinds)
	}
	if full.BlockedActions == 0 {
		t.Error("no action was held at the approval gate across the whole library")
	}

	md := rep.Markdown()
	if !strings.Contains(md, "Samples") {
		t.Error("the report does not show the sample size beside its rates")
	}
	for _, mode := range eval.Modes {
		if !strings.Contains(md, string(mode)) {
			t.Errorf("the report omits the %s row", mode)
		}
	}
	if !strings.Contains(md, "Per case") {
		t.Error("the report does not break results down per case")
	}
	// The comparison must state what the baseline is, so a reader can judge it.
	if !strings.Contains(md, "same tools") {
		t.Error("the report does not state that the baseline has the same tools")
	}
}

// sdd:verify TC-0081
func TestEvaluationIsReproducible(t *testing.T) {
	cat := loadCatalog(t)
	ctx := context.Background()

	first, err := eval.RunAll(ctx, cat, []string{"C1"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := eval.RunAll(ctx, cat, []string{"C1"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Markdown() != second.Markdown() {
		t.Error("two evaluation runs produced different reports")
	}
}

// sdd:verify TC-0082
func TestEveryCatalogCaseIsSolvedByTheFullFlow(t *testing.T) {
	cat := loadCatalog(t)
	ctx := context.Background()
	for _, id := range cat.CaseIDs() {
		o, err := eval.RunCase(ctx, cat, id, domain.ModeMultiWithCritic, eval.Options{})
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if o.Failure != "" {
			t.Errorf("%s failed: %s", id, o.Failure)
			continue
		}
		if !o.Top1Correct {
			t.Errorf("%s: top-1 = %q, want %q", id, o.Top1, o.Expected)
		}
		if o.EvidenceKinds < 4 {
			t.Errorf("%s gathered only %d evidence kinds", id, o.EvidenceKinds)
		}
	}
}

// sdd:verify TC-0114
func TestEvaluationAttributesReasoner(t *testing.T) {
	cat := loadCatalog(t)
	ctx := context.Background()

	// Two strategies in one invocation. Both are deterministic here — the second is the
	// same rule engine under a different name — because what is under test is the
	// attribution, not the strategies. A comparison that loses track of which adapter
	// produced which row is worthless whatever the adapters were.
	rep, err := eval.RunAllWith(ctx, cat, []string{"C1"}, []eval.Options{
		{},
		{
			Reasoner: "second",
			NewReasoner: func(c *catalog.Catalog) reasoner.Reasoner {
				return reasoner.NewRuleReasoner(c, reasoner.DefaultConfig())
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("every outcome names its reasoner", func(t *testing.T) {
		if len(rep.Outcomes) != 2*len(eval.Modes) {
			t.Fatalf("ran %d outcomes, want %d adapters x %d modes",
				len(rep.Outcomes), 2, len(eval.Modes))
		}
		for _, o := range rep.Outcomes {
			if o.Reasoner == "" {
				t.Errorf("%s/%s records no reasoner", o.CaseID, o.Mode)
			}
		}
	})

	t.Run("summaries are grouped by mode and reasoner, not by mode alone", func(t *testing.T) {
		if len(rep.Summaries) != 2*len(eval.Modes) {
			t.Fatalf("got %d summary rows, want %d: grouping by mode alone would average "+
				"two strategies into a number describing neither",
				len(rep.Summaries), 2*len(eval.Modes))
		}
		seen := map[string]bool{}
		for _, s := range rep.Summaries {
			key := string(s.Mode) + "/" + s.Reasoner
			if seen[key] {
				t.Errorf("duplicate summary for %s", key)
			}
			seen[key] = true
			if s.Reasoner == "" {
				t.Errorf("summary for %s records no reasoner", s.Mode)
			}
			if s.Samples != 1 {
				t.Errorf("%s reports %d samples over one case", key, s.Samples)
			}
		}
	})

	t.Run("a strategy varied while the other is fixed gets its own row", func(t *testing.T) {
		// Two planners over one reasoner: grouping that ignored the planner would
		// average two different investigations into one number (REQ-0106).
		varied, err := eval.RunAllWith(ctx, cat, []string{"C1"}, []eval.Options{
			{Planner: "rule"},
			{
				Planner:    "second-planner",
				NewPlanner: func() plan.Planner { return plan.NewRulePlanner(30 * time.Minute) },
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(varied.Summaries) != 2*len(eval.Modes) {
			t.Fatalf("got %d summary rows for two planners over %d modes, want %d",
				len(varied.Summaries), len(eval.Modes), 2*len(eval.Modes))
		}
		for _, s := range varied.Summaries {
			if s.Planner == "" {
				t.Errorf("summary for %s records no planner", s.Mode)
			}
		}
	})

	t.Run("the rendered report names both", func(t *testing.T) {
		md := rep.Markdown()
		if !strings.Contains(md, "Reasoner") {
			t.Error("the report has no reasoner column")
		}
		if !strings.Contains(md, "Planner") {
			t.Error("the report has no planner column")
		}
		for _, want := range []string{"`rule`", "`second`"} {
			if !strings.Contains(md, want) {
				t.Errorf("the report never names %s", want)
			}
		}
	})

	t.Run("an unnamed selection reports the adapter's own name", func(t *testing.T) {
		for _, o := range rep.Outcomes {
			if o.Reasoner == "rule" {
				return
			}
		}
		t.Error("the default selection did not report itself as the rule adapter")
	})
}
