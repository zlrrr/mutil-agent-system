package reasoner

import (
	"context"
	"sort"
	"strings"

	"github.com/zlrrr/mutil-agent-system/internal/catalog"
	"github.com/zlrrr/mutil-agent-system/internal/domain"
)

// sdd:impl DLD-1033

// RuleReasoner is the deterministic default adapter: a rule engine over the fault
// signature catalog. It performs no I/O and holds no clock, so the same evidence always
// yields the same hypotheses (ADR-002).
type RuleReasoner struct {
	cat   *catalog.Catalog
	cfg   Config
	rules []CritiqueRule
}

// NewRuleReasoner builds the deterministic reasoner over a catalog.
func NewRuleReasoner(cat *catalog.Catalog, cfg Config) *RuleReasoner {
	r := &RuleReasoner{cat: cat, cfg: cfg}
	r.rules = DefaultRules(cat, cfg)
	return r
}

// Rules exposes the critique ensemble, so each rule can be tested individually.
func (r *RuleReasoner) Rules() []CritiqueRule { return r.rules }

// Matches evaluates every signature against a snapshot and returns those with at least
// one matched pattern, in catalog order.
func (r *RuleReasoner) Matches(s domain.Snapshot) []catalog.MatchResult {
	var out []catalog.MatchResult
	for _, sig := range r.cat.Signatures {
		m := catalog.Match(sig, s.Evidence)
		if !m.Any() {
			continue
		}
		out = append(out, m)
	}
	return out
}

// Hypothesise forms and ranks candidate root causes.
//
// A signature that matches no pattern at all is discarded rather than scored: an
// explanation with nothing behind it is not a weak hypothesis, it is not a hypothesis.
func (r *RuleReasoner) Hypothesise(_ context.Context, s domain.Snapshot) ([]domain.Hypothesis, error) {
	existing := map[string]domain.Hypothesis{}
	for _, h := range s.Hypotheses {
		existing[h.SignatureID] = h
	}

	var out []domain.Hypothesis
	for _, m := range r.Matches(s) {
		match := m
		in := termInputs(match, s, &match)

		h := domain.Hypothesis{
			SignatureID: match.Signature.ID,
			Claim:       match.Signature.Claim,
			Mechanism:   match.Signature.Mechanism,
			Supporting:  match.Supporting,
			Status:      domain.HypothesisProposed,
		}
		// Reuse the identifier a previous round assigned, so scores update across
		// rounds instead of the same explanation appearing twice.
		//
		// The verdict is deliberately *not* carried forward: the critic re-examines
		// every hypothesis against the round's evidence, so a challenge answered by
		// new evidence must be able to clear.
		if prev, ok := existing[match.Signature.ID]; ok {
			h.ID = prev.ID
			// A hypothesis that was acted upon without restoring the signals stays
			// challenged: that outcome does not stop being true in a later round,
			// unlike a critique the round's new evidence may have answered.
			if prev.Status == domain.HypothesisChallenged {
				h.Status = domain.HypothesisChallenged
			}
			if prev.Resolved != nil {
				h.Resolved = map[string]bool{}
				for k, v := range prev.Resolved {
					h.Resolved[k] = v
				}
			}
		}

		// Counter-evidence: any evidence naming this signature in its counters fact.
		for _, e := range s.Evidence {
			for _, sigID := range splitList(e.Fact("counters")) {
				if sigID == match.Signature.ID {
					h.AddCounter(e.ID)
				}
			}
		}

		in.Unresolved = h.UnresolvedCounters()
		h.Breakdown = Score(in, r.cfg.Weights, r.cfg.CounterPenalty)
		out = append(out, h)
	}

	out = Rank(out, s.Index)
	if len(out) > r.cfg.MaxHypothesesPerRound {
		out = out[:r.cfg.MaxHypothesesPerRound]
	}
	return out, nil
}

func splitList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}
