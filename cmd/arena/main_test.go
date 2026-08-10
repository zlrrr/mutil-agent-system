package main

import (
	"flag"
	"os/exec"
	"strings"
	"testing"

	"github.com/zlrrr/mutil-agent-system/internal/catalog"
	"github.com/zlrrr/mutil-agent-system/internal/reasoner"
	"github.com/zlrrr/mutil-agent-system/internal/reasoner/strategy"
)

// sdd:verify TC-0112

// TestReasonerSelectableEverywhere asserts that every entry point producing a comparable
// result offers the same reasoner selection.
//
// It used to be reachable from `arena serve` alone — the one command that produces no
// comparison. So the reference scenario could not be run through the model adapter, the
// evaluation could not contrast the two, and ADR-002's claim that substituting a model
// changes nothing was unfalsifiable from outside the code (REQ-0104).
func TestReasonerSelectableEverywhere(t *testing.T) {
	const (
		kindFlag     = "reasoner"
		endpointFlag = "model-endpoint"
		nameFlag     = "model-name"
		plannerFlag  = "planner"
	)

	// Asserted by driving the real commands, not by re-registering the flags here: a
	// test that calls the shared helper three times proves the helper works and says
	// nothing about whether any command uses it.
	t.Run("arena serve rejects an unknown selection before binding a port", func(t *testing.T) {
		err := serve([]string{"--addr", "127.0.0.1:0", "--reasoner", "gpt-by-vibes"})
		requireConfigError(t, err, kindFlag)
	})

	t.Run("arena demo rejects an unknown selection", func(t *testing.T) {
		err := demo([]string{"--case", "C1", "--quiet", "--reasoner", "gpt-by-vibes"})
		requireConfigError(t, err, kindFlag)
	})

	t.Run("evalctl run rejects an unknown selection", func(t *testing.T) {
		if testing.Short() {
			t.Skip("skipping the subprocess check in short mode")
		}
		if _, err := exec.LookPath("go"); err != nil {
			t.Skip("go toolchain is not on PATH")
		}
		// evalctl exits rather than returning, so it is driven as a process.
		out, err := exec.Command("go", "run", "../evalctl", "run",
			"--cases", "C1", "--reasoner", "gpt-by-vibes").CombinedOutput()
		if err == nil {
			t.Fatalf("evalctl accepted an unknown reasoner:\n%s", out)
		}
		if !strings.Contains(string(out), kindFlag) {
			t.Errorf("evalctl's error does not name --%s:\n%s", kindFlag, out)
		}
	})

	t.Run("every command accepts the model flags", func(t *testing.T) {
		fs := flag.NewFlagSet("x", flag.ContinueOnError)
		fs.SetOutput(new(strings.Builder))
		strategy.Register(fs)
		for _, want := range []string{kindFlag, endpointFlag, nameFlag, plannerFlag} {
			if fs.Lookup(want) == nil {
				t.Errorf("the shared registration omits --%s", want)
			}
		}
	})

	t.Run("the default is the deterministic adapter", func(t *testing.T) {
		fs := flag.NewFlagSet("x", flag.ContinueOnError)
		c := strategy.Register(fs)
		if err := fs.Parse(nil); err != nil {
			t.Fatal(err)
		}
		if c.Name() != strategy.Rule {
			t.Errorf("default reasoner = %q, want %q", c.Name(), strategy.Rule)
		}
		cat, err := catalog.Load()
		if err != nil {
			t.Fatal(err)
		}
		r, err := c.Build(cat, reasoner.DefaultConfig())
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		if r != nil {
			t.Error("the rule selection built an adapter; nil means the arena's own " +
				"default, so the two cannot drift apart")
		}
	})

	t.Run("arena demo rejects an unknown planner", func(t *testing.T) {
		err := demo([]string{"--case", "C1", "--quiet", "--planner", "vibes"})
		requireConfigError(t, err, plannerFlag)
	})

	t.Run("the strategies are selectable independently", func(t *testing.T) {
		fs := flag.NewFlagSet("x", flag.ContinueOnError)
		c := strategy.Register(fs)
		if err := fs.Parse([]string{"--reasoner", "model", "--model-endpoint", "http://p",
			"--model-name", "m"}); err != nil {
			t.Fatal(err)
		}
		// Moving the reasoner must not move the planner: one switch for both would make
		// their contributions inseparable in the evaluation (REQ-0104).
		if c.Name() != strategy.Model {
			t.Errorf("reasoner = %q, want %q", c.Name(), strategy.Model)
		}
		if c.PlannerName() != strategy.Rule {
			t.Errorf("planner = %q, want %q: selecting a reasoner moved the planner too",
				c.PlannerName(), strategy.Rule)
		}
	})

	t.Run("selecting a model without an endpoint is a configuration error", func(t *testing.T) {
		cat, err := catalog.Load()
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range []strategy.Config{
			{Kind: strategy.Model, Model: "some-model"},
			{Kind: strategy.Model, Endpoint: "http://provider.invalid"},
			{Kind: "gpt-by-vibes"},
		} {
			if _, err := buildReasoner(c, cat); err == nil {
				t.Errorf("%+v built without error; a mistyped selection must fail at "+
					"start-up, not at the first case", c)
			} else if !strings.Contains(err.Error(), "-reasoner") {
				t.Errorf("error %q does not name the offending flag", err)
			}
		}
	})

	t.Run("a comma-separated selection expands to a matrix", func(t *testing.T) {
		got, err := strategy.Split(strategy.Config{
			Kind: "rule,model", Endpoint: "http://provider.invalid", Model: "m",
		})
		if err != nil {
			t.Fatalf("split: %v", err)
		}
		if len(got) != 2 || got[0].Name() != strategy.Rule || got[1].Name() != strategy.Model {
			t.Errorf("split produced %v, want rule then model in the order given", names(got))
		}
		// Duplicates would double-count a strategy in the comparison.
		again, err := strategy.Split(strategy.Config{Kind: "rule,rule"})
		if err != nil {
			t.Fatal(err)
		}
		if len(again) != 1 {
			t.Errorf("split kept %d copies of one adapter", len(again))
		}
	})
}

func names(cs []strategy.Config) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Name()
	}
	return out
}

// requireConfigError asserts a command failed for configuration reasons and named the
// flag at fault, which is what lets an operator fix it without reading the source.
func requireConfigError(t *testing.T, err error, flagName string) {
	t.Helper()
	if err == nil {
		t.Fatal("the command accepted an unknown reasoner")
	}
	if !strings.Contains(err.Error(), flagName) {
		t.Errorf("error %q does not name --%s", err, flagName)
	}
}
