package plan_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/zlrrr/mutil-agent-system/internal/agent/plan"
	"github.com/zlrrr/mutil-agent-system/internal/agent/plan/planmodel"
	"github.com/zlrrr/mutil-agent-system/internal/domain"
)

// sdd:verify TC-0117

// TestPlannerContract holds every planner adapter to one behavioural contract.
//
// ADR-008 named this as the price of adding ports: "each needs the contract treatment
// REQ-0105 gave the reasoner, or it repeats D35". D35 was a contract test recorded as a
// consequence and never written, which left two adapters agreeing only on a Go interface
// for five milestones. This one is written with the port rather than after it.
func TestPlannerContract(t *testing.T) {
	const lookback = 30 * time.Minute
	snap := plannerSnapshot()
	available := plan.Available{Series: []string{"db_up", "http_5xx_rate", "http_requests_total"}}

	adapters := []struct {
		name string
		make func(*testing.T) plan.Planner
	}{
		{"rule", func(*testing.T) plan.Planner { return plan.NewRulePlanner(lookback) }},
		{"model", func(t *testing.T) plan.Planner {
			return planmodel.New(stubPlanner(t, []string{"db_up", "http_5xx_rate"}),
				"contract-model", lookback, planmodel.Options{})
		}},
	}

	for _, a := range adapters {
		t.Run(a.name, func(t *testing.T) {
			p := a.make(t)

			t.Run("it states which strategy it is", func(t *testing.T) {
				if p.Name() != a.name {
					t.Errorf("Name() = %q, want %q", p.Name(), a.name)
				}
			})

			t.Run("triage returns the alert's window and the collector roles", func(t *testing.T) {
				got, err := p.Triage(context.Background(), snap)
				if err != nil {
					t.Fatalf("triage: %v", err)
				}
				if !got.Window.Start.Equal(snap.Window.Start) || !got.Window.End.Equal(snap.Window.End) {
					t.Errorf("window = %v, want the alert's %v; a strategy that can widen "+
						"the window can make every case unbounded", got.Window, snap.Window)
				}
				if !reflect.DeepEqual(got.Roles, domain.CollectorRoles) {
					t.Errorf("roles = %v, want %v; a strategy that can drop a role can "+
						"blind an evidence kind without saying so", got.Roles, domain.CollectorRoles)
				}
			})

			t.Run("every planned series survives resolution against the sources", func(t *testing.T) {
				q, err := p.PlanCollection(context.Background(), snap, available)
				if err != nil {
					t.Fatalf("plan collection: %v", err)
				}
				if q.Empty() {
					t.Fatal("the plan would collect nothing, which is not a decision this " +
						"port may return: it is indistinguishable from a failure")
				}
				fallback, _ := plan.NewRulePlanner(lookback).PlanCollection(
					context.Background(), snap, available)
				_, dropped := plan.Resolve(q, available, fallback)
				if len(dropped) != 0 {
					t.Errorf("the plan named %v, which no source offers", dropped)
				}
				if len(q.LogTerms) == 0 {
					t.Error("the plan names no log terms, so the log collector has nothing to search")
				}
			})
		})
	}

	// Only an adapter that reaches outward can fail this way, but the property belongs to
	// the port: an empty plan is a decision, and a failure must not be able to look like
	// one. The caller can only fall back if it is told.
	t.Run("a failure to reach a provider is an error, not an empty plan", func(t *testing.T) {
		dead := httptest.NewServer(http.NotFoundHandler())
		dead.Close()
		p := planmodel.New(dead.URL, "contract-model", lookback, planmodel.Options{})

		q, err := p.PlanCollection(context.Background(), snap, available)
		if err == nil {
			t.Fatalf("an unreachable provider produced %v and no error", q.Series)
		}
		if !q.Empty() {
			t.Error("a failed plan carried series; the caller would execute a plan nobody made")
		}
	})

	t.Run("a provider that names nothing usable is a failure, not a decision", func(t *testing.T) {
		p := planmodel.New(stubPlanner(t, nil), "contract-model", lookback, planmodel.Options{})
		if _, err := p.PlanCollection(context.Background(), snap, available); err == nil {
			t.Error("an empty selection was accepted as 'query nothing'; the caller cannot " +
				"then tell a considered narrowing from a broken response")
		}
	})

	t.Run("names the sources do not offer are dropped, not queried", func(t *testing.T) {
		p := planmodel.New(stubPlanner(t, []string{"db_up", "invented_metric"}),
			"contract-model", lookback, planmodel.Options{})
		q, err := p.PlanCollection(context.Background(), snap, available)
		if err != nil {
			t.Fatalf("plan collection: %v", err)
		}
		fallback, _ := plan.NewRulePlanner(lookback).PlanCollection(
			context.Background(), snap, available)
		resolved, dropped := plan.Resolve(q, available, fallback)
		if want := []string{"invented_metric"}; !reflect.DeepEqual(dropped, want) {
			t.Errorf("dropped %v, want %v", dropped, want)
		}
		if want := []string{"db_up"}; !reflect.DeepEqual(resolved.Series, want) {
			t.Errorf("kept %v, want %v", resolved.Series, want)
		}
	})
}

func plannerSnapshot() domain.Snapshot {
	start := time.Date(2026, 7, 26, 10, 7, 0, 0, time.UTC)
	return domain.Snapshot{
		Alert: domain.Alert{
			Name: "OrderApiHighErrorRate", Service: "order-api", Severity: "P1",
			StartsAt: start, EndsAt: start.Add(20 * time.Minute),
			Annotations: map[string]string{"summary": "5xx rate above 10% for 5 minutes"},
		},
		Window:    domain.TimeWindow{Start: start.Add(-7 * time.Minute), End: start.Add(20 * time.Minute)},
		Round:     1,
		MaxRounds: 3,
	}
}

// stubPlanner answers with a fixed selection, so the adapter is measured on how it handles
// a response rather than on a provider's mood.
func stubPlanner(t *testing.T, series []string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		inner, _ := json.Marshal(map[string]any{
			"series": series, "reason": "the signals the alert names",
		})
		body, _ := json.Marshal(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": string(inner)}}},
		})
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}
