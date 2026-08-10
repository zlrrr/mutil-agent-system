// Package strategy chooses which adapter answers the reasoner port, and registers the
// flags that select it.
//
// It exists for the same reason `signal/profile` does: the choice of adapter is a
// deployment decision, and a deployment decision belongs in one place that every entry
// point reads. Before this package the selection lived in `arena serve` alone — so the
// one command that could substitute a model was the one command that produces no
// comparison, and ADR-002's central claim could not be checked from anywhere (REQ-0104).
package strategy

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/zlrrr/mutil-agent-system/internal/catalog"
	"github.com/zlrrr/mutil-agent-system/internal/reasoner"
	"github.com/zlrrr/mutil-agent-system/internal/reasoner/model"
)

// sdd:impl DLD-1075

// Kinds are the adapters a deployment may select, in report order.
const (
	Rule  = "rule"
	Model = "model"
)

// Names returns the selectable adapters, sorted, for error messages and usage text.
func Names() []string {
	out := []string{Rule, Model}
	sort.Strings(out)
	return out
}

// Config is the resolved selection. The zero value selects the deterministic rule
// adapter, which is the default by design rather than by omission (ADR-002).
type Config struct {
	// Kind may name one adapter or a comma-separated list. A list is meaningful only to
	// callers that run a matrix — the evaluation — and Split reports the difference.
	Kind     string
	Endpoint string
	Model    string
	// APIKey is read from the environment only. A credential passed on a command line
	// lands in the shell history and in the output of `ps`.
	APIKey string
}

// Register binds the shared flags onto a flag set and returns the config they fill.
// Environment variables supply the defaults, so a container needs no argv.
func Register(fs *flag.FlagSet) *Config {
	c := &Config{}
	fs.StringVar(&c.Kind, "reasoner", env("ARENA_REASONER", Rule),
		"reasoner adapter: "+strings.Join(Names(), "|"))
	fs.StringVar(&c.Endpoint, "model-endpoint", env("ARENA_MODEL_ENDPOINT", ""),
		"chat-completions URL, required by the model reasoner")
	fs.StringVar(&c.Model, "model-name", env("ARENA_MODEL_NAME", ""),
		"model identifier, required by the model reasoner")
	c.APIKey = os.Getenv("ARENA_MODEL_API_KEY")
	return c
}

// Split expands a comma-separated selection into one config per adapter, preserving the
// order given so a report reads in the order the operator asked for.
func Split(c Config) ([]Config, error) {
	var out []Config
	seen := map[string]bool{}
	for _, k := range strings.Split(c.Kind, ",") {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		if seen[k] {
			continue
		}
		seen[k] = true
		one := c
		one.Kind = k
		if err := one.Validate(); err != nil {
			return nil, err
		}
		out = append(out, one)
	}
	if len(out) == 0 {
		out = append(out, Config{Kind: Rule})
	}
	return out, nil
}

// Validate rejects an unusable selection without building anything, so an entry point can
// fail at start-up rather than at the first case.
func (c Config) Validate() error {
	switch c.Kind {
	case Rule, "":
		return nil
	case Model:
		// No provider has been chosen (charter Q1), so there is no default endpoint and
		// guessing one would be a decision this code cannot make.
		if c.Endpoint == "" {
			return fmt.Errorf("adapter %q requires a model endpoint", Model)
		}
		if c.Model == "" {
			return fmt.Errorf("adapter %q requires a model name", Model)
		}
		return nil
	default:
		return fmt.Errorf("unknown reasoner %q; expected one of %s", c.Kind, strings.Join(Names(), ", "))
	}
}

// Name returns the adapter this configuration selects.
func (c Config) Name() string {
	if c.Kind == "" {
		return Rule
	}
	return c.Kind
}

// Build returns the adapter, or nil for the rule adapter — which the arena treats as the
// default, so "rule" and an unset selection agree rather than diverging.
func (c Config) Build(cat *catalog.Catalog, cfg reasoner.Config) (reasoner.Reasoner, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	switch c.Kind {
	case Rule, "":
		return nil, nil
	case Model:
		return model.New(c.Endpoint, c.Model, cat, cfg, model.Options{APIKey: c.APIKey}), nil
	}
	return nil, fmt.Errorf("unknown reasoner %q", c.Kind)
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
