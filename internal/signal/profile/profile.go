// Package profile chooses which adapter serves each signal port.
//
// The whole point of this package is REQ-0098: the fixture profile is the default, and
// selecting a live backend is something a deployment must say explicitly. A system whose
// adapters could drift to "live" by accident would make the determinism the rest of the
// design rests on (REQ-0090) a property of the environment rather than of the code.
package profile

import (
	"fmt"
	"sort"
	"strings"

	"github.com/zlrrr/mutil-agent-system/internal/catalog"
	"github.com/zlrrr/mutil-agent-system/internal/signal"
	"github.com/zlrrr/mutil-agent-system/internal/signal/containerlog"
	"github.com/zlrrr/mutil-agent-system/internal/signal/fixture"
	"github.com/zlrrr/mutil-agent-system/internal/signal/prometheus"
)

// sdd:impl DLD-1027

// Profile names an adapter selection.
type Profile string

const (
	// Fixture serves every port from the embedded fault case. It is the default.
	Fixture Profile = "fixture"
	// Live serves the metric and log ports from real backends and leaves the rest on
	// fixtures, which is the scope of milestone M4.
	Live Profile = "live"
)

// Config is the deployment's adapter configuration.
type Config struct {
	Profile       Profile
	PrometheusURL string
	ContainerHost string
	// SeriesMap maps a catalog series name to the PromQL that produces it.
	SeriesMap map[string]string
	// Units and Capacities carry the metadata the fixture data declares inline.
	Units      map[string]string
	Capacities map[string]float64
}

// Validate checks a configuration without building anything, so an entry point can
// reject a bad profile at start-up rather than at the first case.
//
// Deferring this to Build would mean a server with a mistyped profile starts cleanly and
// then fails one case at a time — the failure arrives when someone is already relying on
// it, which is the worst possible moment to learn about a typo.
func (c Config) Validate() error {
	switch c.Profile {
	case Fixture, "":
		return nil
	case Live:
		if c.PrometheusURL == "" {
			return fmt.Errorf("profile %q requires a Prometheus URL", Live)
		}
		if c.ContainerHost == "" {
			return fmt.Errorf("profile %q requires a container host", Live)
		}
		if len(c.SeriesMap) == 0 {
			return fmt.Errorf("profile %q requires a series map: without it no series can be resolved", Live)
		}
		return nil
	default:
		return fmt.Errorf("unknown profile %q; expected one of %s", c.Profile, strings.Join(Names(), ", "))
	}
}

// Build returns the signal set for a configuration, plus the fixture state so a caller
// can inspect what was actuated. The state is meaningful for every profile because the
// actuator stays simulated until a milestone gives it a live target (charter N1).
func Build(cfg Config, fc catalog.FaultCase, cat *catalog.Catalog, bounds signal.Bounds) (signal.Set, *fixture.State, error) {
	set, state := fixture.NewSet(fc, cat, bounds)

	switch cfg.Profile {
	case Fixture, "":
		// The zero value is the safe value. A missing configuration selects fixtures
		// rather than failing or guessing.
		return set, state, nil

	case Live:
		if cfg.PrometheusURL == "" {
			return signal.Set{}, nil, fmt.Errorf("profile %q requires a Prometheus URL", Live)
		}
		if cfg.ContainerHost == "" {
			return signal.Set{}, nil, fmt.Errorf("profile %q requires a container host", Live)
		}
		set.Metrics = prometheus.New(cfg.PrometheusURL, bounds, prometheus.Options{
			SeriesMap:  cfg.SeriesMap,
			Units:      cfg.Units,
			Capacities: cfg.Capacities,
		})
		set.Logs = containerlog.New(cfg.ContainerHost, bounds, containerlog.Options{})
		return set, state, nil

	default:
		// Never treated as a request for the default. A typo that silently fell back
		// to fixtures would let a live deployment look healthy while reading
		// simulated data — the most expensive failure this package can have.
		return signal.Set{}, nil, fmt.Errorf("unknown profile %q; expected one of %s",
			cfg.Profile, strings.Join(Names(), ", "))
	}
}

// Names returns the legal profile names, sorted, for error messages and help text.
func Names() []string {
	names := []string{string(Fixture), string(Live)}
	sort.Strings(names)
	return names
}
