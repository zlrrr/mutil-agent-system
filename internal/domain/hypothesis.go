package domain

import (
	"errors"
	"sort"
	"strings"
)

// sdd:impl DLD-1003

// HypothesisStatus tracks a candidate root cause through the investigation.
type HypothesisStatus string

// Hypothesis lifecycle states.
const (
	HypothesisProposed   HypothesisStatus = "proposed"
	HypothesisChallenged HypothesisStatus = "challenged"
	HypothesisAccepted   HypothesisStatus = "accepted"
	HypothesisRejected   HypothesisStatus = "rejected"
)

// ScoreTerm is one named, weighted component of a hypothesis score. Storing the term
// rather than only the total is what makes a ranking arguable (ARC-008).
type ScoreTerm struct {
	Name         string  `json:"name"`
	Weight       float64 `json:"weight"`
	Value        float64 `json:"value"`
	Contribution float64 `json:"contribution"`
}

// ScoreBreakdown is the full derivation of a hypothesis score.
type ScoreBreakdown struct {
	Terms      []ScoreTerm `json:"terms"`
	Penalty    float64     `json:"penalty"`
	Unresolved int         `json:"unresolved_counters"`
	Total      float64     `json:"total"`
}

// Sum returns the arithmetic the breakdown claims: contributions less penalty. Callers
// assert this against Total (TC-0022).
func (b ScoreBreakdown) Sum() float64 {
	var t float64
	for _, term := range b.Terms {
		t += term.Contribution
	}
	return t - b.Penalty
}

// Term returns a named term and whether it was present.
func (b ScoreBreakdown) Term(name string) (ScoreTerm, bool) {
	for _, t := range b.Terms {
		if t.Name == name {
			return t, true
		}
	}
	return ScoreTerm{}, false
}

// Hypothesis is a candidate root cause bound to the evidence that supports it.
type Hypothesis struct {
	ID          string           `json:"id"`
	SignatureID string           `json:"signature_id"`
	Claim       string           `json:"claim"`
	Mechanism   string           `json:"mechanism"`
	Supporting  []string         `json:"supporting_evidence_ids"`
	Counter     []string         `json:"counter_evidence_ids,omitempty"`
	Resolved    map[string]bool  `json:"resolved_counters,omitempty"`
	Breakdown   ScoreBreakdown   `json:"breakdown"`
	Status      HypothesisStatus `json:"status"`
	Verdict     Verdict          `json:"verdict,omitempty"`
}

// Validation errors for hypotheses.
var (
	ErrNoSupportingEvidence = errors.New("hypothesis must reference at least one evidence identifier")
	ErrMissingClaim         = errors.New("hypothesis must carry a claim")
	ErrMissingMechanism     = errors.New("hypothesis must explain a mechanism")
)

// Validate enforces the evidence-before-conclusion rule at the type level (REQ-0021).
func (h Hypothesis) Validate() error {
	if len(h.Supporting) == 0 {
		return ErrNoSupportingEvidence
	}
	if strings.TrimSpace(h.Claim) == "" {
		return ErrMissingClaim
	}
	if strings.TrimSpace(h.Mechanism) == "" {
		return ErrMissingMechanism
	}
	return nil
}

// SupportingKinds returns the sorted distinct evidence kinds backing the hypothesis.
func (h Hypothesis) SupportingKinds(index map[string]Evidence) []EvidenceKind {
	seen := map[EvidenceKind]bool{}
	for _, id := range h.Supporting {
		if e, ok := index[id]; ok {
			seen[e.Kind] = true
		}
	}
	var out []EvidenceKind
	for _, k := range AllEvidenceKinds {
		if seen[k] {
			out = append(out, k)
		}
	}
	return out
}

// HasKind reports whether the hypothesis is supported by evidence of a given kind.
func (h Hypothesis) HasKind(index map[string]Evidence, k EvidenceKind) bool {
	for _, id := range h.Supporting {
		if e, ok := index[id]; ok && e.Kind == k {
			return true
		}
	}
	return false
}

// UnresolvedCounters counts counter-evidence that has not been explained away.
func (h Hypothesis) UnresolvedCounters() int {
	n := 0
	for _, id := range h.Counter {
		if !h.Resolved[id] {
			n++
		}
	}
	return n
}

// AddCounter records counter-evidence, keeping the list sorted and duplicate-free.
func (h *Hypothesis) AddCounter(id string) {
	for _, existing := range h.Counter {
		if existing == id {
			return
		}
	}
	h.Counter = append(h.Counter, id)
	sort.Strings(h.Counter)
}
