package report_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/zlrrr/mutil-agent-system/internal/arena"
	"github.com/zlrrr/mutil-agent-system/internal/domain"
	"github.com/zlrrr/mutil-agent-system/internal/report"
)

// completedCase runs the reference scenario to a closed report.
func completedCase(t *testing.T) *domain.Case {
	t.Helper()
	b, err := arena.NewFixtureBuild(arena.Params{CaseID: "C1"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	c, err := b.Engine.Create(ctx, b.Alert(), domain.ModeMultiWithCritic)
	if err != nil {
		t.Fatal(err)
	}
	c, err = b.Engine.Run(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	a, ok := c.PendingAction()
	if !ok {
		t.Fatalf("the case did not reach the approval gate (status %s)", c.Status)
	}
	c, err = b.Engine.Decide(ctx, c.ID, a.ID, domain.ApprovalDecision{
		Decision: "approved", By: "demo-operator", Comment: "restore the pool size",
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// sdd:verify TC-0053
func TestReportSections(t *testing.T) {
	md := report.Markdown(completedCase(t))

	for _, section := range []string{
		"# Root cause report",
		"## Summary",
		"## Impact",
		"## Timeline",
		"## Accepted root cause",
		"### Mechanism",
		"### Score breakdown",
		"### Evidence chain",
		"## Rejected alternatives",
		"## Adversarial review",
		"### Evidence the critic demanded",
		"## Remediation",
		"## Recovery verification",
		"## Unmet evidence demands",
		"## What the multi-agent flow contributed",
	} {
		if !strings.Contains(md, section) {
			t.Errorf("the report is missing the section %q", section)
		}
	}
}

// sdd:verify TC-0053
func TestReportExplainsWhatWasRuledOut(t *testing.T) {
	c := completedCase(t)
	md := report.Markdown(c)

	if !strings.Contains(md, "sig-traffic-surge") {
		t.Error("the rejected traffic explanation is not named in the report")
	}
	// It must say why, not merely that.
	idx := strings.Index(md, "## Rejected alternatives")
	if idx < 0 {
		t.Fatal("no rejected-alternatives section")
	}
	section := md[idx:]
	if end := strings.Index(section[3:], "\n## "); end > 0 {
		section = section[:end]
	}
	if !strings.Contains(section, "Why it was not accepted") {
		t.Error("the rejected alternatives do not explain themselves")
	}
	if !strings.Contains(section, "Counter-evidence") && !strings.Contains(section, "Scored") {
		t.Errorf("no reason is given for the rejection:\n%s", section)
	}
}

// sdd:verify TC-0022
func TestReportScoreBreakdownIsArguable(t *testing.T) {
	c := completedCase(t)
	md := report.Markdown(c)

	for _, term := range []string{
		"metric_alignment", "log_alignment", "change_correlation",
		"topology_plausibility", "historical_similarity", "remediation_verifiability",
	} {
		if !strings.Contains(md, term) {
			t.Errorf("the score breakdown omits %q", term)
		}
	}
	if !strings.Contains(md, "counter-evidence penalty") {
		t.Error("the breakdown does not show the penalty line")
	}
	// Every evidence identifier in the chain must resolve to a query the reader can run.
	lead, _ := c.Leading()
	index := c.EvidenceIndex()
	for _, id := range lead.Supporting {
		e, ok := index[id]
		if !ok {
			t.Errorf("the accepted hypothesis cites %s, which is not in the case", id)
			continue
		}
		if !strings.Contains(md, e.RawRef) {
			t.Errorf("the report does not show the query behind %s (%s)", id, e.RawRef)
		}
	}
}

// sdd:verify TC-0053
func TestReportIsAPureFunction(t *testing.T) {
	c := completedCase(t)
	first := report.Markdown(c)
	for i := 0; i < 5; i++ {
		if got := report.Markdown(c); got != first {
			t.Fatalf("rendering %d differed from the first; the report is not a pure function", i)
		}
	}
}

// sdd:verify TC-0053
func TestReportRendersAbsentSectionsExplicitly(t *testing.T) {
	// An empty case still renders every section, marked as absent rather than omitted:
	// a reader must be able to tell "nothing was found" from "nobody looked".
	empty := domain.NewCase("inc-empty")
	md := report.Markdown(empty)

	for _, section := range []string{"## Impact", "## Accepted root cause", "## Remediation"} {
		if !strings.Contains(md, section) {
			t.Errorf("section %q is missing from an empty report", section)
		}
	}
	if !strings.Contains(md, "_none recorded_") {
		t.Error("absent sections must be marked explicitly")
	}
}

// sdd:verify TC-0053
func TestReportJSON(t *testing.T) {
	c := completedCase(t)
	raw, err := report.JSON(c)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("the JSON report does not parse: %v", err)
	}
	for _, key := range []string{"id", "alert", "status", "evidence", "hypotheses",
		"critiques", "demands", "actions", "verifications"} {
		if _, ok := decoded[key]; !ok {
			t.Errorf("the JSON report omits %q", key)
		}
	}
}
