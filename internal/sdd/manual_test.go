package sdd

import (
	"path/filepath"
	"strings"
	"testing"
)

// repoRoot is the repository root relative to this package.
const repoRoot = "../.."

// sdd:verify TC-0091
func TestManualBilingualParity(t *testing.T) {
	m, err := Load(repoRoot)
	if err != nil {
		t.Fatalf("load the specification tree: %v", err)
	}

	rep := m.Lint()
	if rep.HasErrors() {
		t.Errorf("the documentation tree has bilingual parity errors:\n%s", joinReport(rep))
	}

	// The manual specifically must exist as a pair and be in parity.
	var found bool
	for _, d := range m.Docs {
		if !strings.Contains(d.RelPath, "manual/user-manual") {
			continue
		}
		found = true
		if d.FrontMatter["doc_version"] == "" {
			t.Errorf("%s declares no doc_version", d.RelPath)
		}
	}
	if !found {
		t.Errorf("no user manual was found under %s",
			filepath.Join(repoRoot, "docs", "manual"))
	}

	// Every language a document exists in must exist for both.
	byBase := map[string]map[string]bool{}
	for _, d := range m.Docs {
		if d.Lang == "" {
			continue
		}
		if byBase[d.Base] == nil {
			byBase[d.Base] = map[string]bool{}
		}
		byBase[d.Base][d.Lang] = true
	}
	for base, langs := range byBase {
		for _, want := range m.Config.Bilingual.Languages {
			if !langs[want] {
				t.Errorf("%s has no %s rendering", base, want)
			}
		}
	}
}

// sdd:verify TC-0091
func TestSpecificationTreeIsClean(t *testing.T) {
	m, err := Load(repoRoot)
	if err != nil {
		t.Fatalf("load the specification tree: %v", err)
	}

	trace := m.Trace()
	if trace.HasErrors() {
		t.Errorf("the specification tree has traceability errors:\n%s", joinReport(trace))
	}

	// Every P0 requirement must reach a test that actually exists.
	covered := map[string]bool{}
	for _, tc := range m.ItemsByPrefix("TC") {
		if len(m.AnchorsFor("verify", tc.ID)) == 0 {
			continue
		}
		for _, parent := range tc.DerivesFrom {
			covered[parent] = true
		}
	}
	for _, req := range m.ItemsByPrefix("REQ") {
		if req.Priority != "P0" {
			continue
		}
		if !covered[req.ID] {
			t.Errorf("P0 requirement %s (%s) has no executed test", req.ID, req.Title)
		}
	}
}

func joinReport(r *Report) string {
	var b strings.Builder
	for _, f := range r.Sorted() {
		b.WriteString("  ")
		b.WriteString(f.String())
		b.WriteByte('\n')
	}
	return b.String()
}
