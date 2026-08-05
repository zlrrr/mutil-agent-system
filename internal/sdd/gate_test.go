package sdd

import (
	"strings"
	"testing"
)

// sdd:verify TC-9011
func TestGateImplement(t *testing.T) {
	// A design item with no implementing source file must block the gate.
	files := fullChain("The goal body.")
	delete(files, "src/impl.go")
	m := loadFixture(t, files)
	if _, err := m.Seal(); err != nil {
		t.Fatalf("seal: %v", err)
	}
	m, err := Load(m.Root)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}

	res, err := m.Gate("implement")
	if err != nil {
		t.Fatalf("gate: %v", err)
	}
	if res.Passed {
		t.Fatal("implement gate must fail when a design item has no implementation")
	}
	var found bool
	for _, c := range res.Checks {
		if c.Name == "no-unimplemented-dld" {
			if c.Passed {
				t.Error("no-unimplemented-dld should have failed")
			}
			for _, b := range c.Blocked {
				if b == "DLD-0001" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Errorf("gate must name the blocking design item: %+v", res.Checks)
	}
}

// sdd:verify TC-9011
func TestGatePassesOnCompleteSealedTree(t *testing.T) {
	m := loadFixture(t, fullChain("The goal body."))
	if _, err := m.Seal(); err != nil {
		t.Fatalf("seal: %v", err)
	}
	m, _ = Load(m.Root)

	res, err := m.Gate("implement")
	if err != nil {
		t.Fatalf("gate: %v", err)
	}
	if !res.Passed {
		for _, c := range res.Checks {
			t.Logf("  %-24s passed=%v %s %v", c.Name, c.Passed, c.Detail, c.Blocked)
		}
		t.Fatal("a complete, sealed, fully anchored tree must pass the implement gate")
	}
}

// sdd:verify TC-9011
func TestGateFailsOnUnknownRequirement(t *testing.T) {
	m := loadFixture(t, fullChain("The goal body."))
	res, err := m.Gate("bogus")
	if err != nil {
		t.Fatalf("gate: %v", err)
	}
	if res.Passed {
		t.Fatal("an unrecognised precondition must fail the gate, not be ignored")
	}
	if len(res.Unknowns) != 1 || res.Unknowns[0] != "this-requirement-does-not-exist" {
		t.Errorf("unknowns = %v", res.Unknowns)
	}
}

// sdd:verify TC-9011
func TestGateUnknownStageIsAnError(t *testing.T) {
	m := loadFixture(t, fullChain("The goal body."))
	if _, err := m.Gate("no-such-stage"); err == nil {
		t.Fatal("gate for an undeclared stage must return an error")
	} else if !strings.Contains(err.Error(), "no-such-stage") {
		t.Errorf("error should name the stage: %v", err)
	}
}

// sdd:verify TC-9011
func TestGateDetectsDrift(t *testing.T) {
	m := loadFixture(t, fullChain("The goal body."))
	// Never sealed: the no-drift precondition must fail rather than pass silently.
	res, err := m.Gate("implement")
	if err != nil {
		t.Fatalf("gate: %v", err)
	}
	for _, c := range res.Checks {
		if c.Name == "no-drift" && c.Passed {
			t.Error("no-drift must fail on a tree that was never sealed")
		}
	}
}
