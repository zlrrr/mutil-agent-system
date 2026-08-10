package reasoner_test

import (
	"context"
	"encoding/json"
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

// sdd:verify TC-0113

// TestReasonerContract holds every adapter to one behavioural contract.
//
// ADR-002 listed this suite as a consequence of shipping two adapters, and it was never
// written — so for five milestones the two were held only to a shared *shape*. A Go
// interface constrains signatures; what the orchestrator actually depends on is that a
// hypothesis cites evidence that exists, that a critique names a hypothesis that was
// examined, and that a failure to reach a backing service cannot arrive looking like the
// conclusion "nothing matched".
//
// Both adapters run against the same snapshot, so a divergence is a divergence in
// behaviour rather than in the inputs each was given.
func TestReasonerContract(t *testing.T) {
	s, cat := referenceSnapshot(t)

	adapters := []struct {
		name string
		make func(*testing.T) reasoner.Reasoner
	}{
		{"rule", func(*testing.T) reasoner.Reasoner {
			return reasoner.NewRuleReasoner(cat, reasoner.DefaultConfig())
		}},
		{"model", func(t *testing.T) reasoner.Reasoner {
			return model.New(stubProvider(t, s), "contract-model", cat,
				reasoner.DefaultConfig(), model.Options{})
		}},
	}

	// Hypotheses from the deterministic adapter, kept for the failure case below: a
	// critique of nothing is correctly a no-op, so asserting that an unreachable provider
	// errors requires giving it something to criticise.
	ruleHyps, err := reasoner.NewRuleReasoner(cat, reasoner.DefaultConfig()).
		Hypothesise(context.Background(), s)
	if err != nil {
		t.Fatalf("rule hypothesise: %v", err)
	}

	for _, a := range adapters {
		t.Run(a.name, func(t *testing.T) {
			r := a.make(t)

			t.Run("it states which strategy it is", func(t *testing.T) {
				if r.Name() != a.name {
					t.Errorf("Name() = %q, want %q; attribution that travels with the "+
						"result cannot be wrong about itself", r.Name(), a.name)
				}
			})

			hs, err := r.Hypothesise(context.Background(), s)
			if err != nil {
				t.Fatalf("hypothesise: %v", err)
			}
			t.Run("every hypothesis is bound to the case and the catalog", func(t *testing.T) {
				if len(hs) == 0 {
					t.Fatal("no hypothesis formed from the reference case's evidence")
				}
				for _, h := range hs {
					if _, ok := cat.Signature(h.SignatureID); !ok {
						t.Errorf("hypothesis names signature %q, absent from the catalog",
							h.SignatureID)
					}
					if strings.TrimSpace(h.Claim) == "" || strings.TrimSpace(h.Mechanism) == "" {
						t.Errorf("%s carries no claim or no mechanism", h.SignatureID)
					}
					if len(h.Supporting) == 0 {
						t.Errorf("%s cites no evidence", h.SignatureID)
					}
					for _, id := range h.Supporting {
						if _, ok := s.Index[id]; !ok {
							t.Errorf("%s cites evidence %q that is not in the snapshot",
								h.SignatureID, id)
						}
					}
				}
			})

			// The critic examines what analysis proposed, so it is given exactly that.
			cs, ds, err := r.Critique(context.Background(), withHypotheses(s, hs))
			if err != nil {
				t.Fatalf("critique: %v", err)
			}
			t.Run("every critique names an examined hypothesis and a closed-set verdict", func(t *testing.T) {
				proposed := map[string]bool{}
				for _, h := range hs {
					proposed[h.ID] = true
				}
				if len(cs) == 0 {
					t.Fatal("no critique produced for the proposed hypotheses")
				}
				for _, c := range cs {
					if !proposed[c.HypothesisID] {
						t.Errorf("critique targets %q, which was never proposed", c.HypothesisID)
					}
					if !c.Verdict.Valid() {
						t.Errorf("critique on %s carries verdict %q, outside the closed set",
							c.HypothesisID, c.Verdict)
					}
					if strings.TrimSpace(c.Challenge) == "" {
						t.Errorf("critique on %s states no challenge", c.HypothesisID)
					}
				}
			})

			t.Run("every demand is answerable as written", func(t *testing.T) {
				for _, d := range ds {
					if strings.TrimSpace(d.Descriptor) == "" {
						t.Error("a demand carries no descriptor, so nothing could answer it")
					}
					if !d.Kind.Valid() {
						t.Errorf("demand %q carries kind %q, outside the closed set",
							d.Descriptor, d.Kind)
					}
				}
			})
		})
	}

	// Only an adapter that reaches outward can fail this way, but the property belongs to
	// the contract rather than to one adapter: whatever the strategy, "nothing matched"
	// is a conclusion, and a transport failure must never be able to impersonate one.
	t.Run("a failure to reach a backing service is an error, not an empty result", func(t *testing.T) {
		dead := httptest.NewServer(http.NotFoundHandler())
		dead.Close() // closed before use: the address refuses connections
		r := model.New(dead.URL, "contract-model", cat, reasoner.DefaultConfig(), model.Options{})

		hs, err := r.Hypothesise(context.Background(), s)
		if err == nil {
			t.Errorf("hypothesise returned %d hypotheses and no error from an unreachable "+
				"provider; silence would be read as 'nothing matched'", len(hs))
		}
		if _, _, err := r.Critique(context.Background(), withHypotheses(s, ruleHyps)); err == nil {
			t.Error("critique returned no error from an unreachable provider")
		}
	})
}

// referenceSnapshot drives C1 to completion and returns its snapshot, so both adapters
// are exercised against identifiers the rest of the system produced rather than ones the
// test invented.
func referenceSnapshot(t *testing.T) (domain.Snapshot, *catalog.Catalog) {
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
	s := c.Snapshot(reasoner.DefaultConfig().MaxRounds)
	if len(s.Evidence) == 0 {
		t.Fatal("the reference case produced no evidence")
	}
	return s, b.Catalog
}

func withHypotheses(s domain.Snapshot, hs []domain.Hypothesis) domain.Snapshot {
	out := s
	out.Hypotheses = hs
	return out
}

// stubProvider answers both prompts with a well-formed selection drawn from the snapshot,
// so the model adapter is measured on its handling of a valid response rather than on a
// provider's mood.
func stubProvider(t *testing.T, s domain.Snapshot) string {
	t.Helper()

	var cited []string
	for _, e := range s.Evidence {
		if e.Kind == domain.KindMetric || e.Kind == domain.KindLog {
			cited = append(cited, e.ID)
		}
		if len(cited) == 2 {
			break
		}
	}
	if len(cited) < 2 {
		t.Fatal("the reference case lacks the evidence this stub needs to cite")
	}
	ids, _ := json.Marshal(cited)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct{ Content string } `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		var prompt string
		if len(req.Messages) > 0 {
			prompt = req.Messages[len(req.Messages)-1].Content
		}

		content := fmt.Sprintf(
			`{"selections":[{"signature_id":"sig-db-pool-exhaustion","evidence_ids":%s}]}`, ids)
		if strings.Contains(prompt, "critique") || strings.Contains(prompt, "Critique") {
			content = `{"critiques":[]}`
			if id := firstHypothesisID(prompt); id != "" {
				content = fmt.Sprintf(
					`{"critiques":[{"hypothesis_id":%q,"category":"coverage_gap",`+
						`"challenge":"the saturation metric is missing","verdict":"revise"}]}`, id)
			}
		}
		body, _ := json.Marshal(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": content}}},
		})
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// firstHypothesisID recovers an identifier from the critique prompt, so the stub
// criticises a hypothesis that was actually proposed rather than one it made up.
func firstHypothesisID(prompt string) string {
	i := strings.Index(prompt, "h-analysis-")
	if i < 0 {
		return ""
	}
	rest := prompt[i:]
	end := strings.IndexFunc(rest, func(r rune) bool {
		return !(r == '-' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z')
	})
	if end < 0 {
		return rest
	}
	return rest[:end]
}
