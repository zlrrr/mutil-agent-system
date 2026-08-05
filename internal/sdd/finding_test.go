package sdd

import (
	"strings"
	"testing"
)

// sdd:verify TC-9014
func TestReportSorted(t *testing.T) {
	cfg := &Config{Severity: map[string]string{
		"noteworthy": "info",
		"suspicious": "warn",
		"broken":     "error",
	}}
	rep := &Report{}
	rep.Add(cfg, "noteworthy", "C", "c.md", 3, "info finding")
	rep.Add(cfg, "broken", "B", "b.md", 2, "error finding")
	rep.Add(cfg, "suspicious", "A", "a.md", 1, "warn finding")
	rep.Add(cfg, "broken", "A", "a.md", 4, "another error")

	got := rep.Sorted()
	wantOrder := []Severity{SeverityError, SeverityError, SeverityWarn, SeverityInfo}
	for i, want := range wantOrder {
		if got[i].Severity != want {
			t.Fatalf("position %d: severity %s, want %s (order: %v)", i, got[i].Severity, want, got)
		}
	}
	// Ties break by class then subject.
	if got[0].Subject != "A" || got[1].Subject != "B" {
		t.Errorf("equal-severity findings must order by subject: %s then %s",
			got[0].Subject, got[1].Subject)
	}

	if !rep.HasErrors() {
		t.Error("HasErrors must be true when an error finding is present")
	}
	if rep.Count(SeverityWarn) != 1 || rep.Count(SeverityError) != 2 {
		t.Errorf("counts = %s", rep.Summary())
	}
	if !strings.Contains(rep.Summary(), "2 error(s)") {
		t.Errorf("summary = %q", rep.Summary())
	}
}

// sdd:verify TC-9014
func TestUnknownFindingClassFailsClosed(t *testing.T) {
	cfg := &Config{Severity: map[string]string{}}
	if got := cfg.SeverityOf("a-class-nobody-configured"); got != SeverityError {
		t.Errorf("unknown finding class resolved to %s, want ERROR so new checks fail closed", got)
	}
}

// sdd:verify TC-9014
func TestFindingStringIncludesLocation(t *testing.T) {
	cfg := &Config{Severity: map[string]string{"broken": "error"}}
	rep := &Report{}
	rep.Add(cfg, "broken", "REQ-0001", "specs/spec.en.md", 42, "something is wrong")
	s := rep.Findings[0].String()
	for _, want := range []string{"ERROR", "broken", "REQ-0001", "specs/spec.en.md:42"} {
		if !strings.Contains(s, want) {
			t.Errorf("finding line %q missing %q", s, want)
		}
	}
}
