package sdd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureConfig is the governance configuration used by the unit tests. It mirrors
// the real chain in .sdd/config.json but points at throw-away roots.
const fixtureConfig = `{
  "project": "fixture",
  "sddVersion": "test",
  "docRoots": ["specs"],
  "codeRoots": ["src"],
  "lockFile": ".sdd/sdd.lock.json",
  "bilingual": {
    "enabled": true,
    "languages": ["en", "zh"],
    "primary": "en",
    "pattern": "<base>.<lang>.md",
    "exempt": ["README.md"]
  },
  "stages": [
    {"name": "constitution", "prefix": "CON", "title": "Constitution", "parents": []},
    {"name": "charter", "prefix": "G", "title": "Goals", "parents": ["CON"]},
    {"name": "specify", "prefix": "REQ", "title": "Requirements", "parents": ["G"]},
    {"name": "architect", "prefix": "ARC", "title": "Architecture", "parents": ["REQ"]},
    {"name": "hld", "prefix": "HLD", "title": "High Level Design", "parents": ["ARC"]},
    {"name": "dld", "prefix": "DLD", "title": "Detailed Design", "parents": ["HLD"]},
    {"name": "verify", "prefix": "TC", "title": "Test Cases", "parents": ["REQ", "DLD"]}
  ],
  "codeAnchors": {
    "impl":   {"keyword": "sdd:impl",   "targetPrefixes": ["DLD"]},
    "verify": {"keyword": "sdd:verify", "targetPrefixes": ["TC"]}
  },
  "gates": [
    {"stage": "implement", "requires": ["dld-approved", "no-drift", "no-unimplemented-dld", "no-orphan-code"]},
    {"stage": "bogus", "requires": ["this-requirement-does-not-exist"]}
  ],
  "severity": {
    "orphanItem": "error",
    "unknownParent": "error",
    "illegalParentStage": "error",
    "duplicateId": "error",
    "bilingualMissingCounterpart": "error",
    "bilingualStructureMismatch": "error",
    "bilingualVersionMismatch": "error",
    "orphanCodeAnchor": "error",
    "nonBilingualDoc": "error",
    "unimplementedDld": "warn",
    "unverifiedTestCase": "warn",
    "untestedRequirement": "warn",
    "modifiedItem": "warn",
    "unsealedItem": "warn",
    "removedItem": "warn",
    "staleDownstream": "error"
  }
}`

// writeFixture materialises a temporary repository from a path->content map and
// returns its root. The governance configuration is written automatically.
func writeFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	all := map[string]string{".sdd/config.json": fixtureConfig}
	for k, v := range files {
		all[k] = v
	}
	for rel, content := range all {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	return root
}

// loadFixture builds a model from a fixture repository.
func loadFixture(t *testing.T, files map[string]string) *Model {
	t.Helper()
	root := writeFixture(t, files)
	m, err := Load(root)
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	return m
}

// frontMatter renders the document header shared by every fixture document.
func frontMatter(lang, version, status string) string {
	return "---\nlang: " + lang + "\ndoc_version: " + version + "\nstatus: " + status + "\n---\n\n"
}

// chainDoc builds a complete, legal one-of-each-stage document in one language.
func chainDoc(lang, version, goalBody string) string {
	return frontMatter(lang, version, "approved") +
		"# Fixture\n\n" +
		"<!-- sdd:item id=G-001 stage=charter status=approved derives_from=CON-001 priority=P0 -->\n" +
		"### G-001 — Goal\n\n" + goalBody + "\n\n" +
		"<!-- sdd:item id=REQ-0001 stage=specify status=approved derives_from=G-001 priority=P0 -->\n" +
		"### REQ-0001 — Requirement\n\nThe system must do the thing.\n\n" +
		"<!-- sdd:item id=ARC-001 stage=architect status=approved derives_from=REQ-0001 -->\n" +
		"### ARC-001 — Architecture\n\nA boundary exists.\n\n" +
		"<!-- sdd:item id=HLD-001 stage=hld status=approved derives_from=ARC-001 -->\n" +
		"### HLD-001 — Module\n\nThe module is responsible for the thing.\n\n" +
		"<!-- sdd:item id=DLD-0001 stage=dld status=approved derives_from=HLD-001 -->\n" +
		"### DLD-0001 — Unit\n\nThe function returns the thing.\n\n" +
		"<!-- sdd:item id=TC-0001 stage=verify status=approved derives_from=REQ-0001 -->\n" +
		"### TC-0001 — Case\n\nAsserts the thing.\n"
}

// constitutionDoc provides the root item every goal derives from.
func constitutionDoc(lang string) string {
	return frontMatter(lang, "1.0.0", "approved") +
		"# Constitution\n\n" +
		"<!-- sdd:item id=CON-001 stage=constitution status=approved -->\n" +
		"### CON-001 — Root\n\nSpecification precedes implementation.\n"
}

// fullChain is a fixture repository whose requirement reaches source and tests.
func fullChain(goalBody string) map[string]string {
	return map[string]string{
		"specs/con.en.md":   constitutionDoc("en"),
		"specs/con.zh.md":   constitutionDoc("zh"),
		"specs/chain.en.md": chainDoc("en", "1.0.0", goalBody),
		"specs/chain.zh.md": chainDoc("zh", "1.0.0", goalBody),
		"src/impl.go":       "package src\n\n// sdd:impl DLD-0001\nfunc Thing() int { return 1 }\n",
		"src/impl_test.go":  "package src\n\n// sdd:verify TC-0001\nfunc TestThing(t *testing.T) {}\n",
	}
}

// findingsOfClass returns every finding of a class, for precise assertions.
func findingsOfClass(rep *Report, class string) []Finding {
	var out []Finding
	for _, f := range rep.Findings {
		if f.Class == class {
			out = append(out, f)
		}
	}
	return out
}

// requireClass asserts that exactly n findings of a class are present.
func requireClass(t *testing.T, rep *Report, class string, n int) []Finding {
	t.Helper()
	got := findingsOfClass(rep, class)
	if len(got) != n {
		t.Errorf("want %d %s finding(s), got %d", n, class, len(got))
		for _, f := range rep.Sorted() {
			t.Logf("  %s", f)
		}
	}
	return got
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}

func joinFindings(rep *Report) string {
	var b strings.Builder
	for _, f := range rep.Sorted() {
		b.WriteString(f.String())
		b.WriteByte('\n')
	}
	return b.String()
}
