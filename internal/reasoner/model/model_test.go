package model_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zlrrr/mutil-agent-system/internal/arena"
	"github.com/zlrrr/mutil-agent-system/internal/catalog"
	"github.com/zlrrr/mutil-agent-system/internal/domain"
	"github.com/zlrrr/mutil-agent-system/internal/reasoner"
	"github.com/zlrrr/mutil-agent-system/internal/reasoner/model"
)

// snapshot drives the reference case far enough to have real evidence and hypotheses,
// so the adapter is exercised against identifiers the rest of the system produced rather
// than ones the test invented.
func snapshot(t *testing.T) (domain.Snapshot, *catalog.Catalog) {
	t.Helper()
	b, err := arena.NewFixtureBuild(arena.Params{CaseID: "C1"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	ctx := context.Background()
	c, err := b.Engine.Create(ctx, b.Alert(), domain.ModeMultiWithCritic)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if c, err = b.Engine.Run(ctx, c.ID); err != nil {
		t.Fatalf("run: %v", err)
	}
	s := c.Snapshot(3)
	if len(s.Evidence) == 0 {
		t.Fatal("the reference case produced no evidence")
	}
	return s, b.Catalog
}

// serve returns a provider that replies with the given assistant content.
func serve(t *testing.T, content string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		body, _ := json.Marshal(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": content}}},
		})
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newReasoner(t *testing.T, srv *httptest.Server, cat *catalog.Catalog) *model.Reasoner {
	t.Helper()
	return model.New(srv.URL, "test-model", cat, reasoner.DefaultConfig(),
		model.Options{Client: srv.Client()})
}

// sdd:verify TC-0105
func TestModelHypothesise(t *testing.T) {
	s, cat := snapshot(t)

	var metricID, logID string
	for _, e := range s.Evidence {
		if metricID == "" && e.Kind == domain.KindMetric {
			metricID = e.ID
		}
		if logID == "" && e.Kind == domain.KindLog {
			logID = e.ID
		}
	}
	if metricID == "" || logID == "" {
		t.Fatal("the snapshot lacks the evidence kinds this test needs")
	}

	// The provider also offers a score and a claim of its own. Both must be ignored.
	content := fmt.Sprintf(`{"selections":[
		{"signature_id":"sig-db-pool-exhaustion","evidence_ids":["%s","%s"],
		 "reasoning":"pool saturation with connection timeouts",
		 "score":0.99,"claim":"the moon was in the wrong phase"}]}`, metricID, logID)

	r := newReasoner(t, serve(t, content), cat)
	hs, err := r.Hypothesise(context.Background(), s)
	if err != nil {
		t.Fatalf("hypothesise: %v", err)
	}
	if len(hs) != 1 {
		t.Fatalf("got %d hypotheses, want 1", len(hs))
	}
	h := hs[0]

	t.Run("claim and mechanism come from the catalog", func(t *testing.T) {
		sig, ok := cat.Signature("sig-db-pool-exhaustion")
		if !ok {
			t.Fatal("the catalog lost its signature")
		}
		if h.Claim != sig.Claim {
			t.Errorf("claim = %q, want the catalog's; provider prose must not become a causal claim", h.Claim)
		}
		if h.Mechanism != sig.Mechanism {
			t.Errorf("mechanism = %q, want the catalog's", h.Mechanism)
		}
		if strings.Contains(h.Claim, "moon") {
			t.Error("the provider's invented claim reached the hypothesis")
		}
	})

	t.Run("supporting evidence is exactly what was cited", func(t *testing.T) {
		for _, want := range []string{metricID, logID} {
			var found bool
			for _, got := range h.Supporting {
				if got == want {
					found = true
				}
			}
			if !found {
				t.Errorf("cited evidence %s is not in the supporting set %v", want, h.Supporting)
			}
		}
	})

	// The assertion ADR-007 exists for: a number a model asserts does not decompose,
	// and REQ-0022 requires that it does.
	t.Run("the score is computed locally and decomposes", func(t *testing.T) {
		if h.Breakdown.Total == 0.99 {
			t.Error("the provider's score was used verbatim")
		}
		if len(h.Breakdown.Terms) == 0 {
			t.Fatal("the score carries no breakdown")
		}
		if diff := h.Breakdown.Sum() - h.Breakdown.Total; diff > 1e-9 || diff < -1e-9 {
			t.Errorf("breakdown sums to %v but total is %v", h.Breakdown.Sum(), h.Breakdown.Total)
		}
		var weight float64
		for _, term := range h.Breakdown.Terms {
			weight += term.Weight
		}
		if diff := weight - 1.0; diff > 1e-9 || diff < -1e-9 {
			t.Errorf("term weights sum to %v, want 1.0", weight)
		}
	})
}

// sdd:verify TC-0106
func TestModelOutputIsUntrusted(t *testing.T) {
	s, cat := snapshot(t)

	var realID string
	for _, e := range s.Evidence {
		if e.Kind == domain.KindMetric {
			realID = e.ID
			break
		}
	}

	t.Run("invalid selections are dropped and valid ones survive", func(t *testing.T) {
		content := fmt.Sprintf(`{"selections":[
			{"signature_id":"sig-invented-by-the-model","evidence_ids":["%s"]},
			{"signature_id":"sig-db-pool-exhaustion","evidence_ids":["e-does-not-exist"]},
			{"signature_id":"sig-traffic-surge","evidence_ids":["e-nope","%s"]}]}`, realID, realID)

		r := newReasoner(t, serve(t, content), cat)
		hs, err := r.Hypothesise(context.Background(), s)
		if err != nil {
			t.Fatalf("hypothesise: %v", err)
		}
		if len(hs) != 1 {
			t.Fatalf("got %d hypotheses, want only the one with a real signature and real evidence", len(hs))
		}
		if hs[0].SignatureID != "sig-traffic-surge" {
			t.Errorf("survivor is %s, want sig-traffic-surge", hs[0].SignatureID)
		}
		for _, id := range hs[0].Supporting {
			if strings.HasPrefix(id, "e-nope") || strings.HasPrefix(id, "e-does-not-exist") {
				t.Errorf("a hallucinated evidence identifier %s reached the hypothesis", id)
			}
		}
	})

	t.Run("an invalid verdict is dropped", func(t *testing.T) {
		if len(s.Hypotheses) == 0 {
			t.Skip("no hypotheses to critique")
		}
		hID := s.Hypotheses[0].ID
		content := fmt.Sprintf(`{"critiques":[
			{"hypothesis_id":"%s","category":"x","challenge":"weak","verdict":"looks_fine_to_me"},
			{"hypothesis_id":"h-not-real","category":"x","challenge":"weak","verdict":"revise"},
			{"hypothesis_id":"%s","category":"coverage","challenge":"no saturation metric","verdict":"revise",
			 "demands":["database connection pool saturation metric"]}]}`, hID, hID)

		r := newReasoner(t, serve(t, content), cat)
		cs, ds, err := r.Critique(context.Background(), s)
		if err != nil {
			t.Fatalf("critique: %v", err)
		}
		if len(cs) != 1 {
			t.Fatalf("got %d critiques, want only the valid one", len(cs))
		}
		if cs[0].Verdict != domain.VerdictRevise {
			t.Errorf("verdict = %q, want revise", cs[0].Verdict)
		}
		if len(ds) != 1 {
			t.Errorf("got %d demands, want 1", len(ds))
		}
	})

	// A failing provider must not be able to say "nothing matched". The two outcomes
	// lead to different decisions, so they must stay distinguishable.
	t.Run("failure is a typed error, never an empty result", func(t *testing.T) {
		bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer bad.Close()

		r := model.New(bad.URL, "m", cat, reasoner.DefaultConfig(), model.Options{Client: bad.Client()})
		hs, err := r.Hypothesise(context.Background(), s)
		if err == nil {
			t.Fatal("a 500 produced no error")
		}
		if hs != nil {
			t.Error("a failure also returned hypotheses")
		}
		var pe *model.ProviderError
		if !errors.As(err, &pe) {
			t.Errorf("error is not a ProviderError: %v", err)
		} else if pe.Status != http.StatusInternalServerError {
			t.Errorf("status = %d, want 500", pe.Status)
		}

		garbled := serve(t, "I'm afraid I can't do that")
		r2 := newReasoner(t, garbled, cat)
		if _, err := r2.Hypothesise(context.Background(), s); err == nil {
			t.Error("unparseable content produced no error")
		}
	})

	t.Run("a well-formed empty response is not an error", func(t *testing.T) {
		r := newReasoner(t, serve(t, `{"selections":[]}`), cat)
		hs, err := r.Hypothesise(context.Background(), s)
		if err != nil {
			t.Errorf("an empty selection list must mean 'nothing matched', not a failure: %v", err)
		}
		if len(hs) != 0 {
			t.Errorf("got %d hypotheses from an empty response", len(hs))
		}
	})

	t.Run("a fenced response is still parsed", func(t *testing.T) {
		r := newReasoner(t, serve(t, "```json\n{\"selections\":[]}\n```"), cat)
		if _, err := r.Hypothesise(context.Background(), s); err != nil {
			t.Errorf("a code-fenced reply was rejected: %v", err)
		}
	})
}
