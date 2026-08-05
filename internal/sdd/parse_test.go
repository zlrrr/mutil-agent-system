package sdd

import (
	"path/filepath"
	"strings"
	"testing"
)

// sdd:verify TC-9001
func TestParseDocExtractsItems(t *testing.T) {
	fence := "```"
	doc := frontMatter("en", "1.0.0", "approved") +
		"# Title\n\n" +
		"## Section\n\n" +
		"<!-- sdd:item id=REQ-0001 stage=specify status=approved derives_from=G-001 priority=P0 -->\n" +
		"### REQ-0001 — First requirement\n\n" +
		"Body of the first requirement.\n\n" +
		"<!-- sdd:item id=REQ-0002 stage=specify status=draft derives_from=G-001 -->\n" +
		"### REQ-0002 — Second requirement\n\n" +
		"Body of the second requirement.\n\n" +
		"#### Sub-detail belonging to the second\n\nStill inside REQ-0002.\n\n" +
		"<!-- sdd:item id=REQ-0003 stage=specify status=approved derives_from=G-001 -->\n" +
		"### REQ-0003 — Third requirement\n\n" +
		"An example that must not be parsed as an item:\n\n" +
		fence + "markdown\n" +
		"<!-- sdd:item id=REQ-9999 stage=specify status=approved derives_from=G-001 -->\n" +
		"### REQ-9999 — Decoy inside a fenced block\n" +
		fence + "\n\n" +
		"Tail of the third requirement.\n\n" +
		"## Closing section\n\nOutside every item.\n"

	root := writeFixture(t, map[string]string{"specs/sample.en.md": doc})
	parsed, items, err := ParseDoc(root, filepath.Join(root, "specs", "sample.en.md"))
	if err != nil {
		t.Fatalf("ParseDoc: %v", err)
	}

	if len(items) != 3 {
		t.Fatalf("want 3 items, got %d", len(items))
	}
	wantIDs := []string{"REQ-0001", "REQ-0002", "REQ-0003"}
	for i, want := range wantIDs {
		if items[i].ID != want {
			t.Errorf("item %d: want id %s, got %s", i, want, items[i].ID)
		}
	}
	if got := strings.Join(parsed.ItemIDs, ","); got != "REQ-0001,REQ-0002,REQ-0003" {
		t.Errorf("doc item sequence = %q", got)
	}

	// The identifier prefix is stripped from the stored title.
	if items[0].Title != "First requirement" {
		t.Errorf("title = %q, want %q", items[0].Title, "First requirement")
	}
	if items[0].Priority != "P0" || items[1].Status != "draft" {
		t.Errorf("attributes not parsed: %+v / %+v", items[0], items[1])
	}
	if len(items[0].DerivesFrom) != 1 || items[0].DerivesFrom[0] != "G-001" {
		t.Errorf("derives_from = %v", items[0].DerivesFrom)
	}

	// A deeper heading stays inside its item; a same-level heading closes it.
	if !strings.Contains(items[1].Occurrences[0].Body, "Still inside REQ-0002") {
		t.Error("REQ-0002 body should absorb its level-4 subsection")
	}
	if strings.Contains(items[1].Occurrences[0].Body, "Third requirement") {
		t.Error("REQ-0002 body leaked into REQ-0003")
	}
	if strings.Contains(items[2].Occurrences[0].Body, "Outside every item") {
		t.Error("REQ-0003 body should end at the next level-2 heading")
	}

	// The decoy inside the fenced block must not become an item.
	for _, it := range items {
		if it.ID == "REQ-9999" {
			t.Error("anchor inside a fenced code block was parsed as an item")
		}
	}
	if parsed.FrontMatter["doc_version"] != "1.0.0" {
		t.Errorf("front matter not parsed: %v", parsed.FrontMatter)
	}
	if parsed.Lang != "en" || parsed.Base != "specs/sample" {
		t.Errorf("lang/base = %q/%q", parsed.Lang, parsed.Base)
	}
}

// sdd:verify TC-9001
func TestParseCodeAnchorsIgnoresStringLiterals(t *testing.T) {
	src := "package src\n\n" +
		"const help = \"write sdd:impl DLD-1234 above the function\"\n\n" +
		"// sdd:impl DLD-0001\n" +
		"func A() {}\n\n" +
		"// sdd:verify TC-0001, TC-0002\n" +
		"func TestB() {}\n"
	root := writeFixture(t, map[string]string{"src/a.go": src})

	anchors, err := ParseCodeAnchors(root, []string{"src"})
	if err != nil {
		t.Fatalf("ParseCodeAnchors: %v", err)
	}
	if len(anchors) != 2 {
		t.Fatalf("want 2 anchors (literal must be ignored), got %d: %+v", len(anchors), anchors)
	}
	if anchors[0].Kind != "impl" || anchors[0].Targets[0] != "DLD-0001" || anchors[0].Line != 5 {
		t.Errorf("impl anchor = %+v", anchors[0])
	}
	if anchors[1].Kind != "verify" || len(anchors[1].Targets) != 2 {
		t.Errorf("verify anchor should carry two targets: %+v", anchors[1])
	}
}

// sdd:verify TC-9001
func TestNormalizeIgnoresCosmeticEdits(t *testing.T) {
	a := "line one   \n\n\n\nline two\n"
	b := "line one\n\nline two"
	if normalize(a) != normalize(b) {
		t.Errorf("normalize should ignore trailing space and blank runs:\n%q\n%q",
			normalize(a), normalize(b))
	}
	if normalize("line one") == normalize("line ONE") {
		t.Error("normalize must remain sensitive to the words themselves")
	}
}
