package sdd

import (
	"strings"
	"testing"
)

// sdd:verify TC-9002
func TestTraceRejectsIllegalEdges(t *testing.T) {
	doc := func(lang string) string {
		return frontMatter(lang, "1.0.0", "approved") +
			"# Fixture\n\n" +
			"<!-- sdd:item id=CON-001 stage=constitution status=approved -->\n" +
			"### CON-001 — Root\n\nRoot rule.\n\n" +
			"<!-- sdd:item id=G-001 stage=charter status=approved derives_from=CON-001 -->\n" +
			"### G-001 — Goal\n\nA goal.\n\n" +
			"<!-- sdd:item id=REQ-0001 stage=specify status=approved derives_from=G-001 -->\n" +
			"### REQ-0001 — Requirement\n\nA requirement.\n\n" +
			// Illegal: DLD may only derive from HLD, never straight from REQ.
			"<!-- sdd:item id=DLD-0001 stage=dld status=approved derives_from=REQ-0001 -->\n" +
			"### DLD-0001 — Skips the chain\n\nShortcut.\n\n" +
			// Unknown: the parent does not exist anywhere in the tree.
			"<!-- sdd:item id=ARC-001 stage=architect status=approved derives_from=REQ-4242 -->\n" +
			"### ARC-001 — Dangling parent\n\nDangling.\n"
	}
	m := loadFixture(t, map[string]string{
		"specs/f.en.md": doc("en"),
		"specs/f.zh.md": doc("zh"),
	})

	rep := m.Trace()
	illegal := requireClass(t, rep, "illegalParentStage", 1)
	if len(illegal) == 1 && illegal[0].Subject != "DLD-0001" {
		t.Errorf("illegal edge subject = %s, want DLD-0001", illegal[0].Subject)
	}
	unknown := requireClass(t, rep, "unknownParent", 1)
	if len(unknown) == 1 && !strings.Contains(unknown[0].Message, "REQ-4242") {
		t.Errorf("unknown parent message = %q", unknown[0].Message)
	}
	if !rep.HasErrors() {
		t.Error("an illegal chain must be a fatal finding")
	}
}

// sdd:verify TC-9002
func TestTraceReportsOrphanItem(t *testing.T) {
	doc := func(lang string) string {
		return frontMatter(lang, "1.0.0", "approved") +
			"# Fixture\n\n" +
			"<!-- sdd:item id=REQ-0001 stage=specify status=approved -->\n" +
			"### REQ-0001 — No parent\n\nA requirement with no goal.\n"
	}
	m := loadFixture(t, map[string]string{
		"specs/f.en.md": doc("en"),
		"specs/f.zh.md": doc("zh"),
	})
	requireClass(t, m.Trace(), "orphanItem", 1)
}

// sdd:verify TC-9003
func TestTraceDetectsCycle(t *testing.T) {
	// ARC-001 -> HLD-001 -> DLD-0001 -> ARC-001 forms a cycle. Stage legality is
	// deliberately violated too; the point is that traversal terminates.
	doc := func(lang string) string {
		return frontMatter(lang, "1.0.0", "approved") +
			"# Fixture\n\n" +
			"<!-- sdd:item id=ARC-001 stage=architect status=approved derives_from=DLD-0001 -->\n" +
			"### ARC-001 — A\n\nA.\n\n" +
			"<!-- sdd:item id=HLD-001 stage=hld status=approved derives_from=ARC-001 -->\n" +
			"### HLD-001 — B\n\nB.\n\n" +
			"<!-- sdd:item id=DLD-0001 stage=dld status=approved derives_from=HLD-001 -->\n" +
			"### DLD-0001 — C\n\nC.\n"
	}
	m := loadFixture(t, map[string]string{
		"specs/f.en.md": doc("en"),
		"specs/f.zh.md": doc("zh"),
	})

	done := make(chan *Report, 1)
	go func() { done <- m.Trace() }()
	rep := <-done // a non-terminating traversal would hang the test binary

	if !strings.Contains(joinFindings(rep), "cycle") {
		t.Errorf("expected a cycle finding, got:\n%s", joinFindings(rep))
	}
}

// sdd:verify TC-9004
func TestLintMissingCounterpart(t *testing.T) {
	m := loadFixture(t, map[string]string{
		"specs/only.en.md": frontMatter("en", "1.0.0", "approved") + "# Only English\n",
	})
	got := requireClass(t, m.Lint(), "bilingualMissingCounterpart", 1)
	if len(got) == 1 {
		if got[0].Subject != "specs/only" {
			t.Errorf("subject = %q, want specs/only", got[0].Subject)
		}
		if !strings.Contains(got[0].Message, "zh") {
			t.Errorf("message should name the missing language: %q", got[0].Message)
		}
	}
}

// sdd:verify TC-9004
func TestLintRejectsDocumentWithoutLanguageSuffix(t *testing.T) {
	m := loadFixture(t, map[string]string{
		"specs/notes.md":  "# Notes without a language\n",
		"specs/README.md": "# Exempt by configuration\n",
	})
	got := requireClass(t, m.Lint(), "nonBilingualDoc", 1)
	if len(got) == 1 && got[0].Subject != "specs/notes.md" {
		t.Errorf("subject = %q; README.md is exempt and must not be reported", got[0].Subject)
	}
}

// sdd:verify TC-9005
func TestLintVersionMismatch(t *testing.T) {
	m := loadFixture(t, map[string]string{
		"specs/f.en.md": frontMatter("en", "1.0.0", "approved") + "# Doc\n",
		"specs/f.zh.md": frontMatter("zh", "1.1.0", "approved") + "# 文档\n",
	})
	got := requireClass(t, m.Lint(), "bilingualVersionMismatch", 1)
	if len(got) == 1 && !strings.Contains(got[0].Message, "1.1.0") {
		t.Errorf("message should report both versions: %q", got[0].Message)
	}
}

// sdd:verify TC-9005
func TestLintStatusMismatch(t *testing.T) {
	m := loadFixture(t, map[string]string{
		"specs/f.en.md": frontMatter("en", "1.0.0", "approved") + "# Doc\n",
		"specs/f.zh.md": frontMatter("zh", "1.0.0", "draft") + "# 文档\n",
	})
	requireClass(t, m.Lint(), "bilingualVersionMismatch", 1)
}

// sdd:verify TC-9006
func TestLintStructureMismatch(t *testing.T) {
	en := frontMatter("en", "1.0.0", "approved") + "# Doc\n\n" +
		"<!-- sdd:item id=CON-001 stage=constitution status=approved -->\n" +
		"### CON-001 — One\n\nBody.\n\n" +
		"<!-- sdd:item id=CON-002 stage=constitution status=approved -->\n" +
		"### CON-002 — Two\n\nBody.\n"
	// The Chinese rendering drops CON-002 and adds an unrelated heading.
	zh := frontMatter("zh", "1.0.0", "approved") + "# 文档\n\n" +
		"<!-- sdd:item id=CON-001 stage=constitution status=approved -->\n" +
		"### CON-001 — 一\n\n正文。\n\n" +
		"## 多出来的一节\n\n正文。\n"

	m := loadFixture(t, map[string]string{"specs/f.en.md": en, "specs/f.zh.md": zh})
	got := requireClass(t, m.Lint(), "bilingualStructureMismatch", 2)
	joined := ""
	for _, f := range got {
		joined += f.Message
	}
	if !strings.Contains(joined, "CON-002") {
		t.Errorf("mismatch should name the missing item: %q", joined)
	}
	if !strings.Contains(joined, "heading skeleton") {
		t.Errorf("mismatch should report the heading skeleton difference: %q", joined)
	}
}

// sdd:verify TC-9007
func TestTraceOrphanCodeAnchor(t *testing.T) {
	files := fullChain("The goal body.")
	files["src/bad.go"] = "package src\n\n" +
		"const doc = \"sdd:impl DLD-7777 in a literal is not an anchor\"\n\n" +
		"// sdd:impl DLD-9999\n" +
		"func Bad() {}\n"
	m := loadFixture(t, files)

	got := requireClass(t, m.Trace(), "orphanCodeAnchor", 1)
	if len(got) == 1 {
		if got[0].Subject != "DLD-9999" {
			t.Errorf("subject = %q, want DLD-9999", got[0].Subject)
		}
		if got[0].Line != 5 {
			t.Errorf("line = %d, want 5", got[0].Line)
		}
	}
}

// sdd:verify TC-9007
func TestTraceRejectsAnchorOfWrongKind(t *testing.T) {
	files := fullChain("The goal body.")
	// `verify` anchors may only target TC items.
	files["src/wrong.go"] = "package src\n\n// sdd:verify DLD-0001\nfunc W() {}\n"
	m := loadFixture(t, files)
	requireClass(t, m.Trace(), "orphanCodeAnchor", 1)
}

// sdd:verify TC-9002
func TestTraceCleanOnWellFormedTree(t *testing.T) {
	m := loadFixture(t, fullChain("The goal body."))
	rep := m.Trace()
	if rep.HasErrors() {
		t.Errorf("a well-formed tree must produce no errors:\n%s", joinFindings(rep))
	}
}
