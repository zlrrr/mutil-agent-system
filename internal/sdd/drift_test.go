package sdd

import (
	"os"
	"path/filepath"
	"testing"
)

// sdd:verify TC-9008
func TestSealThenDriftIsClean(t *testing.T) {
	root := writeFixture(t, fullChain("The goal body."))
	m, err := Load(root)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	before, err := m.Drift()
	if err != nil {
		t.Fatalf("drift: %v", err)
	}
	if before.Sealed {
		t.Error("a fresh fixture must report as unsealed")
	}

	if _, err := m.Seal(); err != nil {
		t.Fatalf("seal: %v", err)
	}
	reloaded, err := Load(root)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	after, err := reloaded.Drift()
	if err != nil {
		t.Fatalf("drift after seal: %v", err)
	}
	if !after.Sealed || !after.Clean() {
		t.Errorf("sealed tree must be clean: %+v", after)
	}
	if rep := after.Report(reloaded.Config); rep.HasErrors() {
		t.Errorf("clean drift must produce no errors:\n%s", joinFindings(rep))
	}
}

// sdd:verify TC-9009
func TestDriftCascadesToCode(t *testing.T) {
	files := fullChain("The original goal body.")
	root := writeFixture(t, files)

	m, err := Load(root)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, err := m.Seal(); err != nil {
		t.Fatalf("seal: %v", err)
	}

	// Amend the goal in both languages, as CON-004 requires.
	edited := chainDoc("en", "1.1.0", "The goal body was amended, changing what we intend.")
	if err := os.WriteFile(filepath.Join(root, "specs", "chain.en.md"), []byte(edited), 0o644); err != nil {
		t.Fatalf("edit en: %v", err)
	}
	editedZH := chainDoc("zh", "1.1.0", "The goal body was amended, changing what we intend.")
	if err := os.WriteFile(filepath.Join(root, "specs", "chain.zh.md"), []byte(editedZH), 0o644); err != nil {
		t.Fatalf("edit zh: %v", err)
	}

	m2, err := Load(root)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	d, err := m2.Drift()
	if err != nil {
		t.Fatalf("drift: %v", err)
	}

	if d.Clean() {
		t.Fatal("editing a sealed goal must produce drift")
	}
	if !contains(d.Modified, "G-001") {
		t.Errorf("G-001 must be reported modified, got %v", d.Modified)
	}
	for _, want := range []string{"REQ-0001", "ARC-001", "HLD-001", "DLD-0001", "TC-0001"} {
		if !contains(d.Stale, want) {
			t.Errorf("%s must be stale after its ancestor changed; stale = %v", want, d.Stale)
		}
	}
	if contains(d.Stale, "G-001") {
		t.Error("a directly modified item must not be repeated in the stale set")
	}
	if !contains(d.StaleFile, "src/impl.go") {
		t.Errorf("the implementing source file must be stale; staleFiles = %v", d.StaleFile)
	}
	if !contains(d.StaleFile, "src/impl_test.go") {
		t.Errorf("the verifying test file must be stale; staleFiles = %v", d.StaleFile)
	}

	rep := d.Report(m2.Config)
	if !rep.HasErrors() {
		t.Error("a stale tree must fail validation (CON-003)")
	}

	// Re-sealing is the only way to clear staleness.
	if _, err := m2.Seal(); err != nil {
		t.Fatalf("re-seal: %v", err)
	}
	m3, _ := Load(root)
	if d3, _ := m3.Drift(); !d3.Clean() {
		t.Errorf("re-sealing must clear staleness: %+v", d3)
	}
}

// sdd:verify TC-9009
func TestDriftDetectsAddedAndRemoved(t *testing.T) {
	files := fullChain("The goal body.")
	root := writeFixture(t, files)
	m, _ := Load(root)
	if _, err := m.Seal(); err != nil {
		t.Fatalf("seal: %v", err)
	}

	// Remove the whole chain document pair: every sealed item in it disappears.
	for _, f := range []string{"specs/chain.en.md", "specs/chain.zh.md"} {
		if err := os.Remove(filepath.Join(root, filepath.FromSlash(f))); err != nil {
			t.Fatalf("remove %s: %v", f, err)
		}
	}
	m2, _ := Load(root)
	d, _ := m2.Drift()
	if !contains(d.Removed, "G-001") || !contains(d.Removed, "DLD-0001") {
		t.Errorf("removed items must be reported: %v", d.Removed)
	}

	// Add a brand-new constitution article: it is unsealed, not stale.
	extra := constitutionDoc("en") +
		"\n<!-- sdd:item id=CON-002 stage=constitution status=approved -->\n### CON-002 — New\n\nNew rule.\n"
	extraZH := constitutionDoc("zh") +
		"\n<!-- sdd:item id=CON-002 stage=constitution status=approved -->\n### CON-002 — 新\n\n新规则。\n"
	os.WriteFile(filepath.Join(root, "specs", "con.en.md"), []byte(extra), 0o644)
	os.WriteFile(filepath.Join(root, "specs", "con.zh.md"), []byte(extraZH), 0o644)

	m3, _ := Load(root)
	d3, _ := m3.Drift()
	if !contains(d3.Added, "CON-002") {
		t.Errorf("new items must be reported as added: %v", d3.Added)
	}
}

// sdd:verify TC-9009
func TestContentHashCoversBothLanguagesAndAttributes(t *testing.T) {
	base := fullChain("The goal body.")

	root1 := writeFixture(t, base)
	m1, _ := Load(root1)
	h1 := m1.Items["G-001"].ContentHash()

	// A change confined to the Chinese rendering still changes the item hash,
	// because the pair is a single artifact (CON-004).
	zhOnly := map[string]string{}
	for k, v := range base {
		zhOnly[k] = v
	}
	zhOnly["specs/chain.zh.md"] = chainDoc("zh", "1.0.0", "目标正文已被修改。")
	m2, _ := Load(writeFixture(t, zhOnly))
	if m2.Items["G-001"].ContentHash() == h1 {
		t.Error("a Chinese-only edit must change the item hash")
	}

	// Cosmetic whitespace does not.
	cosmetic := map[string]string{}
	for k, v := range base {
		cosmetic[k] = v
	}
	cosmetic["specs/chain.en.md"] = chainDoc("en", "1.0.0", "The goal body.   \n\n\n")
	m3, _ := Load(writeFixture(t, cosmetic))
	if m3.Items["G-001"].ContentHash() != h1 {
		t.Error("trailing whitespace and blank runs must not change the item hash")
	}
}

// sdd:verify TC-9010
func TestImpactOf(t *testing.T) {
	m := loadFixture(t, fullChain("The goal body."))

	items, files := m.ImpactOf("G-001")
	for _, want := range []string{"REQ-0001", "ARC-001", "HLD-001", "DLD-0001", "TC-0001"} {
		if !contains(items, want) {
			t.Errorf("impact of G-001 must include %s; got %v", want, items)
		}
	}
	if !contains(files, "src/impl.go") || !contains(files, "src/impl_test.go") {
		t.Errorf("impact must reach source files; got %v", files)
	}
	if contains(items, "G-001") {
		t.Error("impact must not include the queried item itself")
	}

	// A leaf has no impact, and the query does not require any edit to be made.
	leafItems, leafFiles := m.ImpactOf("TC-0001")
	if len(leafItems) != 0 || len(leafFiles) != 1 {
		t.Errorf("TC-0001 should reach only its verifying test file; got %v / %v",
			leafItems, leafFiles)
	}
	if d, _ := m.Drift(); d.Sealed {
		t.Error("querying impact must not seal or modify the tree")
	}
}
