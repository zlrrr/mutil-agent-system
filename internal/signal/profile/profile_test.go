package profile_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/zlrrr/mutil-agent-system/internal/catalog"
	"github.com/zlrrr/mutil-agent-system/internal/signal"
	"github.com/zlrrr/mutil-agent-system/internal/signal/profile"
)

func load(t *testing.T) (catalog.FaultCase, *catalog.Catalog) {
	t.Helper()
	cat, err := catalog.Load()
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	fc, ok := cat.Case("C1")
	if !ok {
		t.Fatal("the reference case is missing from the catalog")
	}
	return fc, cat
}

// adapterPackage reports the import path of the concrete type behind a port, which is
// how this test asks "which adapter is serving this?" without the ports themselves
// having to expose that.
func adapterPackage(port any) string {
	t := reflect.TypeOf(port)
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.PkgPath()
}

// sdd:verify TC-0103
func TestProfileSelection(t *testing.T) {
	fc, cat := load(t)
	bounds := signal.DefaultBounds()

	// The assertion this whole package exists for. An empty configuration is what a
	// deployment that forgot to configure anything produces, and it must not reach the
	// network.
	t.Run("an empty configuration selects fixtures", func(t *testing.T) {
		set, state, err := profile.Build(profile.Config{}, fc, cat, bounds)
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		if state == nil {
			t.Fatal("no fixture state was returned")
		}
		for name, port := range map[string]any{
			"metrics":   set.Metrics,
			"logs":      set.Logs,
			"changes":   set.Changes,
			"topology":  set.Topology,
			"knowledge": set.Knowledge,
			"actuator":  set.Actuator,
		} {
			if pkg := adapterPackage(port); !strings.HasSuffix(pkg, "/signal/fixture") {
				t.Errorf("the %s port defaulted to %s, not the fixture adapter", name, pkg)
			}
		}
	})

	t.Run("the fixture profile is the same as no configuration", func(t *testing.T) {
		set, _, err := profile.Build(profile.Config{Profile: profile.Fixture}, fc, cat, bounds)
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		if pkg := adapterPackage(set.Metrics); !strings.HasSuffix(pkg, "/signal/fixture") {
			t.Errorf("metrics served by %s, want the fixture adapter", pkg)
		}
	})

	t.Run("the live profile substitutes only the metric and log ports", func(t *testing.T) {
		set, _, err := profile.Build(profile.Config{
			Profile:       profile.Live,
			PrometheusURL: "http://prometheus:9090",
			ContainerHost: "unix:///var/run/docker.sock",
			SeriesMap:     map[string]string{"http_error_rate": "rate(errors[1m])"},
		}, fc, cat, bounds)
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		if pkg := adapterPackage(set.Metrics); !strings.HasSuffix(pkg, "/signal/prometheus") {
			t.Errorf("metrics served by %s, want the prometheus adapter", pkg)
		}
		if pkg := adapterPackage(set.Logs); !strings.HasSuffix(pkg, "/signal/containerlog") {
			t.Errorf("logs served by %s, want the containerlog adapter", pkg)
		}
		// M4 covers two ports. The others must not have quietly changed with them.
		for name, port := range map[string]any{
			"changes":   set.Changes,
			"topology":  set.Topology,
			"knowledge": set.Knowledge,
			"actuator":  set.Actuator,
		} {
			if pkg := adapterPackage(port); !strings.HasSuffix(pkg, "/signal/fixture") {
				t.Errorf("the %s port changed to %s; M4 covers only metrics and logs", name, pkg)
			}
		}
	})

	t.Run("the live profile refuses to start half-configured", func(t *testing.T) {
		for _, cfg := range []profile.Config{
			{Profile: profile.Live, ContainerHost: "unix:///x"},
			{Profile: profile.Live, PrometheusURL: "http://p:9090"},
		} {
			if _, _, err := profile.Build(cfg, fc, cat, bounds); err == nil {
				t.Errorf("a live profile missing an endpoint was accepted: %+v", cfg)
			}
		}
	})

	// Validation must be available without building, so an entry point rejects a bad
	// profile at start-up. Deferring it to Build means a server with a mistyped profile
	// starts cleanly and fails one case at a time, once someone is already relying on it.
	t.Run("a configuration can be rejected before anything is built", func(t *testing.T) {
		for name, cfg := range map[string]profile.Config{
			"unknown profile":     {Profile: "prod"},
			"live without prom":   {Profile: profile.Live, ContainerHost: "unix:///x", SeriesMap: map[string]string{"a": "b"}},
			"live without host":   {Profile: profile.Live, PrometheusURL: "http://p:9090", SeriesMap: map[string]string{"a": "b"}},
			"live without series": {Profile: profile.Live, PrometheusURL: "http://p:9090", ContainerHost: "unix:///x"},
		} {
			if err := cfg.Validate(); err == nil {
				t.Errorf("%s was accepted at validation time", name)
			}
		}
		for name, cfg := range map[string]profile.Config{
			"empty":   {},
			"fixture": {Profile: profile.Fixture},
			"live": {Profile: profile.Live, PrometheusURL: "http://p:9090",
				ContainerHost: "unix:///x", SeriesMap: map[string]string{"a": "b"}},
		} {
			if err := cfg.Validate(); err != nil {
				t.Errorf("%s was rejected: %v", name, err)
			}
		}
	})

	// A typo must not read as "give me the default". A live deployment silently served
	// simulated data is the worst outcome this package can produce.
	t.Run("an unknown profile is an error, not a fallback", func(t *testing.T) {
		_, _, err := profile.Build(profile.Config{Profile: "prod"}, fc, cat, bounds)
		if err == nil {
			t.Fatal("an unknown profile fell back silently")
		}
		if !strings.Contains(err.Error(), "prod") {
			t.Errorf("the error does not name the offending value: %v", err)
		}
		for _, legal := range profile.Names() {
			if !strings.Contains(err.Error(), legal) {
				t.Errorf("the error does not offer %q as a legal value: %v", legal, err)
			}
		}
	})
}
