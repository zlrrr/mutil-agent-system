package sdd

import (
	"fmt"
	"sort"
	"strings"
)

// sdd:impl DLD-0108

// MatrixRow is one requirement's end-to-end traceability record.
type MatrixRow struct {
	Requirement string   `json:"requirement"`
	Title       string   `json:"title"`
	Priority    string   `json:"priority"`
	Status      string   `json:"status"`
	Goals       []string `json:"goals"`
	Arch        []string `json:"architecture"`
	HLD         []string `json:"hld"`
	DLD         []string `json:"dld"`
	Tasks       []string `json:"tasks"`
	TestCases   []string `json:"testCases"`
	SourceFiles []string `json:"sourceFiles"`
	TestFiles   []string `json:"testFiles"`
}

// Covered reports whether the requirement reaches both code and an executed test.
func (r MatrixRow) Covered() bool {
	return len(r.SourceFiles) > 0 && len(r.TestFiles) > 0
}

// Matrix builds the full requirement-to-artifact traceability matrix.
func (m *Model) Matrix() []MatrixRow {
	var rows []MatrixRow
	for _, req := range m.ItemsByPrefix("REQ") {
		row := MatrixRow{
			Requirement: req.ID,
			Title:       req.Title,
			Priority:    req.Priority,
			Status:      req.Status,
			Goals:       filterPrefix(req.DerivesFrom, "G"),
		}

		desc := m.DescendantsOf(req.ID)
		for _, d := range desc {
			switch {
			case strings.HasPrefix(d, "file:"):
				continue
			case PrefixOf(d) == "ARC":
				row.Arch = append(row.Arch, d)
			case PrefixOf(d) == "HLD":
				row.HLD = append(row.HLD, d)
			case PrefixOf(d) == "DLD":
				row.DLD = append(row.DLD, d)
			case PrefixOf(d) == "T":
				row.Tasks = append(row.Tasks, d)
			case PrefixOf(d) == "TC":
				row.TestCases = append(row.TestCases, d)
			}
		}

		files := map[string]bool{}
		testFiles := map[string]bool{}
		for _, dld := range row.DLD {
			for _, a := range m.AnchorsFor("impl", dld) {
				if a.IsTest {
					testFiles[a.File] = true
				} else {
					files[a.File] = true
				}
			}
		}
		for _, tc := range row.TestCases {
			for _, a := range m.AnchorsFor("verify", tc) {
				testFiles[a.File] = true
			}
		}
		row.SourceFiles = keys(files)
		row.TestFiles = keys(testFiles)
		rows = append(rows, row)
	}
	return rows
}

// RenderMatrix formats the traceability matrix as a markdown table.
func (m *Model) RenderMatrix() string {
	var b strings.Builder
	b.WriteString("| Requirement | P | Goals | ARC | HLD | DLD | Tasks | Tests | Source | Covered |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|---|---|\n")
	for _, r := range m.Matrix() {
		mark := "no"
		if r.Covered() {
			mark = "yes"
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s | %s | %s | %s |\n",
			r.Requirement, dash(r.Priority), join(r.Goals), join(r.Arch), join(r.HLD),
			join(r.DLD), join(r.Tasks), join(r.TestCases),
			join(shorten(r.SourceFiles)), mark)
	}
	return b.String()
}

// RenderGraph emits the artifact derivation graph as a Mermaid flowchart.
func (m *Model) RenderGraph() string {
	var b strings.Builder
	b.WriteString("flowchart TD\n")
	for _, s := range m.Config.Stages {
		items := m.ItemsByPrefix(s.Prefix)
		if len(items) == 0 {
			continue
		}
		fmt.Fprintf(&b, "  subgraph %s[\"%s\"]\n", s.Prefix, s.Title)
		for _, it := range items {
			fmt.Fprintf(&b, "    %s[\"%s\"]\n", nodeID(it.ID), it.ID)
		}
		b.WriteString("  end\n")
	}
	for _, id := range m.Order {
		for _, p := range m.Items[id].DerivesFrom {
			if _, ok := m.Items[p]; ok {
				fmt.Fprintf(&b, "  %s --> %s\n", nodeID(p), nodeID(id))
			}
		}
	}
	return b.String()
}

func nodeID(id string) string { return strings.ReplaceAll(id, "-", "_") }

func filterPrefix(ids []string, prefix string) []string {
	var out []string
	for _, id := range ids {
		if PrefixOf(id) == prefix {
			out = append(out, id)
		}
	}
	return out
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func join(v []string) string {
	if len(v) == 0 {
		return "—"
	}
	return strings.Join(v, "<br>")
}

func dash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// shorten trims the repository-relative prefix that every source path shares, so
// the matrix stays readable when rendered in a document.
func shorten(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = strings.TrimPrefix(p, "internal/")
	}
	return out
}
