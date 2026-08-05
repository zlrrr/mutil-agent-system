package sdd

import (
	"strings"
	"testing"
)

// sdd:verify TC-9012
func TestMatrixAndGraph(t *testing.T) {
	m := loadFixture(t, fullChain("The goal body."))

	rows := m.Matrix()
	if len(rows) != 1 {
		t.Fatalf("want one requirement row, got %d", len(rows))
	}
	r := rows[0]
	if r.Requirement != "REQ-0001" || r.Title != "Requirement" {
		t.Errorf("row identity = %s / %q", r.Requirement, r.Title)
	}
	if !contains(r.Goals, "G-001") {
		t.Errorf("goals = %v", r.Goals)
	}
	for name, got := range map[string][]string{
		"architecture": r.Arch, "hld": r.HLD, "dld": r.DLD, "testCases": r.TestCases,
	} {
		if len(got) == 0 {
			t.Errorf("%s column is empty: %+v", name, r)
		}
	}
	if !contains(r.SourceFiles, "src/impl.go") {
		t.Errorf("source files = %v", r.SourceFiles)
	}
	if !contains(r.TestFiles, "src/impl_test.go") {
		t.Errorf("test files = %v", r.TestFiles)
	}
	if !r.Covered() {
		t.Error("a requirement reaching both source and tests must be marked covered")
	}

	md := m.RenderMatrix()
	if !strings.Contains(md, "REQ-0001") || !strings.Contains(md, "| yes |") {
		t.Errorf("rendered matrix missing coverage mark:\n%s", md)
	}

	graph := m.RenderGraph()
	if !strings.HasPrefix(graph, "flowchart TD") {
		t.Errorf("graph must be a mermaid flowchart:\n%s", graph)
	}
	if !strings.Contains(graph, "G_001 --> REQ_0001") {
		t.Errorf("graph missing the goal-to-requirement edge:\n%s", graph)
	}
	if !strings.Contains(graph, "HLD_001 --> DLD_0001") {
		t.Errorf("graph missing the design edge:\n%s", graph)
	}
}

// sdd:verify TC-9012
func TestMatrixMarksUncoveredRequirement(t *testing.T) {
	files := fullChain("The goal body.")
	delete(files, "src/impl.go")
	m := loadFixture(t, files)

	rows := m.Matrix()
	if len(rows) != 1 {
		t.Fatalf("want one row, got %d", len(rows))
	}
	if rows[0].Covered() {
		t.Error("a requirement with no implementing source must not be marked covered")
	}
	if !strings.Contains(m.RenderMatrix(), "| no |") {
		t.Error("rendered matrix must show the uncovered mark")
	}
}
