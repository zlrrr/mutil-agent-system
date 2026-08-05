package eval_test

import (
	"context"
	"strings"
	"testing"

	"github.com/zlrrr/mutil-agent-system/internal/catalog"
	"github.com/zlrrr/mutil-agent-system/internal/domain"
	"github.com/zlrrr/mutil-agent-system/internal/eval"
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
		o, err := eval.RunCase(ctx, cat, "C1", mode)
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
		o, err := eval.RunCase(ctx, cat, id, domain.ModeMultiWithCritic)
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
