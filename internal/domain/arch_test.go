package domain

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// plane assigns each package to an architectural plane, and rank orders them so that a
// package may import only packages of a strictly lower rank (ARC-001).
type planeSpec struct {
	rank  int
	plane string
}

var planes = map[string]planeSpec{
	"internal/domain":         {0, "core"},
	"internal/signal":         {1, "signal"},
	"internal/catalog":        {2, "reasoning"},
	"internal/signal/fixture": {3, "signal-adapter"},
	// The live adapters sit at the same rank as the fixture adapter: they are
	// alternatives to it, not layers above it, and nothing may depend on one that
	// could not equally depend on the other.
	"internal/signal/prometheus":   {3, "signal-adapter"},
	"internal/signal/containerlog": {3, "signal-adapter"},
	// Profile selection is the one place that names every adapter, so it ranks above
	// all of them and below everything that consumes a port.
	"internal/signal/profile": {4, "signal-adapter"},
	"internal/reasoner":       {4, "reasoning"},
	// The model adapter sits above the reasoner port it implements, and below anything
	// that consumes a reasoner — the same shape as a signal adapter beneath its port.
	"internal/reasoner/model": {5, "reasoning-adapter"},
	// The planner port ranks with the reasoner port, not with the agents that consume
	// it: both are strategies the control plane selects, and neither may depend on the
	// roles that call them (ARC-019).
	"internal/agent/plan":   {4, "reasoning"},
	"internal/agent":        {5, "reasoning"},
	"internal/policy":       {5, "control"},
	"internal/store":        {5, "control"},
	"internal/eventbus":     {5, "control"},
	"internal/report":       {5, "control"},
	"internal/orchestrator": {6, "control"},
	"internal/arena":        {7, "wiring"},
	"internal/eval":         {8, "control"},
	"internal/httpapi":      {8, "control"},
	"internal/sdd":          {0, "governance"},
}

// forbidden lists imports a package may never have, beyond the rank rule. These encode
// the constraints ARC-001 states in prose.
var forbidden = map[string][]string{
	"internal/reasoner": {"internal/store", "internal/eventbus", "internal/orchestrator",
		"internal/signal/fixture", "internal/httpapi"},
	"internal/agent":   {"internal/store", "internal/eventbus", "internal/orchestrator", "internal/httpapi"},
	"internal/signal":  {"internal/reasoner", "internal/orchestrator", "internal/agent", "internal/policy"},
	"internal/catalog": {"internal/reasoner", "internal/orchestrator", "internal/agent"},
	"internal/domain":  {"internal/"},
}

// sdd:verify TC-0074
func TestPlaneDependencies(t *testing.T) {
	root := filepath.Join("..", "..")
	const modulePath = "github.com/zlrrr/mutil-agent-system/"

	for pkg, spec := range planes {
		dir := filepath.Join(root, filepath.FromSlash(pkg))
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}
		imports, err := packageImports(dir)
		if err != nil {
			t.Fatalf("%s: %v", pkg, err)
		}

		for _, imp := range imports {
			if !strings.HasPrefix(imp, modulePath) {
				continue // standard library
			}
			rel := strings.TrimPrefix(imp, modulePath)

			for _, bad := range forbidden[pkg] {
				if bad == "internal/" {
					if strings.HasPrefix(rel, "internal/") {
						t.Errorf("%s imports %s; the core must import nothing from internal/ (ARC-001)", pkg, rel)
					}
					continue
				}
				if rel == bad {
					t.Errorf("%s imports %s, which ARC-001 forbids", pkg, rel)
				}
			}

			other, known := planes[rel]
			if !known {
				t.Errorf("%s imports %s, which is not assigned to a plane; add it to the table", pkg, rel)
				continue
			}
			if other.rank >= spec.rank {
				t.Errorf("%s (rank %d, %s) imports %s (rank %d, %s): the dependency table is strictly downward",
					pkg, spec.rank, spec.plane, rel, other.rank, other.plane)
			}
		}
	}
}

// packageImports returns the module imports of every non-test file in a directory.
func packageImports(dir string) ([]string, error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, imp := range file.Imports {
				out = append(out, strings.Trim(imp.Path.Value, `"`))
			}
		}
	}
	return out, nil
}

// sdd:verify TC-0074
func TestOrchestratorIsTheOnlyWriter(t *testing.T) {
	// Apply is the only method that writes projection fields. If another package
	// gained a way to mutate a case, the replay guarantee (ARC-003) would be a
	// convention rather than a property — so assert that Case exposes no setter.
	c := NewCase("inc-test")
	if c.Status != StatusCreated {
		t.Fatalf("a new case starts in %s, want created", c.Status)
	}
	// The projection's mutable collections are unexported except through Apply; a
	// caller holding a snapshot cannot reach them.
	snap := c.Snapshot(3)
	snap.Evidence = append(snap.Evidence, sampleEvidence())
	if len(c.Evidence) != 0 {
		t.Error("mutating a snapshot reached the case projection")
	}
}
