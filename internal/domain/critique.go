package domain

import (
	"errors"
	"strings"
)

// sdd:impl DLD-1004

// Verdict is the critic's judgement on a hypothesis.
type Verdict string

// The four verdicts a critique may carry.
const (
	VerdictAccept         Verdict = "accept"
	VerdictAcceptWithRisk Verdict = "accept_with_risk"
	VerdictRevise         Verdict = "revise"
	VerdictReject         Verdict = "reject"
)

// Valid reports whether v is one of the declared verdicts.
func (v Verdict) Valid() bool {
	switch v {
	case VerdictAccept, VerdictAcceptWithRisk, VerdictRevise, VerdictReject:
		return true
	}
	return false
}

// Severity ranks verdicts so that combining several yields the most serious.
func (v Verdict) Severity() int {
	switch v {
	case VerdictAccept:
		return 0
	case VerdictAcceptWithRisk:
		return 1
	case VerdictRevise:
		return 2
	case VerdictReject:
		return 3
	}
	return 3
}

// Permits reports whether a verdict allows the case to proceed to remediation.
func (v Verdict) Permits() bool {
	return v == VerdictAccept || v == VerdictAcceptWithRisk
}

// CombineVerdicts returns the most severe of the given verdicts; an empty input is
// an explicit accept, because a hypothesis nobody challenged is not thereby suspect.
//
// Unset values are skipped rather than treated as maximally severe: an absent verdict
// means "not yet judged", and letting it outrank a real judgement would make every
// combination unsatisfiable.
func CombineVerdicts(vs ...Verdict) Verdict {
	out := VerdictAccept
	for _, v := range vs {
		if !v.Valid() {
			continue
		}
		if v.Severity() > out.Severity() {
			out = v
		}
	}
	return out
}

// Critique is a structured objection to a hypothesis (REQ-0030).
type Critique struct {
	ID           string   `json:"id"`
	HypothesisID string   `json:"hypothesis_id"`
	Rule         string   `json:"rule"`
	Category     string   `json:"category"`
	Challenge    string   `json:"challenge"`
	Verdict      Verdict  `json:"verdict"`
	DemandIDs    []string `json:"demand_ids,omitempty"`
	CounterIDs   []string `json:"counter_evidence_ids,omitempty"`
}

// Validation errors for critiques and demands.
var (
	ErrUnknownVerdict     = errors.New("unknown verdict")
	ErrMissingChallenge   = errors.New("critique must carry a challenge")
	ErrMissingHypothesis  = errors.New("critique must reference a hypothesis")
	ErrMissingDescriptor  = errors.New("demand must carry a descriptor")
	ErrUnknownDemandKind  = errors.New("demand must name a known evidence kind")
	ErrMissingDemandCause = errors.New("demand must carry a reason")
)

// Validate enforces the critique invariants.
func (c Critique) Validate() error {
	if !c.Verdict.Valid() {
		return ErrUnknownVerdict
	}
	if strings.TrimSpace(c.HypothesisID) == "" {
		return ErrMissingHypothesis
	}
	if strings.TrimSpace(c.Challenge) == "" {
		return ErrMissingChallenge
	}
	return nil
}

// EvidenceDemand is the critic's request for specific additional evidence. It is what
// gives the critic power over control flow rather than over prose (ARC-009).
type EvidenceDemand struct {
	ID          string       `json:"id"`
	Descriptor  string       `json:"descriptor"`
	Kind        EvidenceKind `json:"kind"`
	Reason      string       `json:"reason"`
	Rule        string       `json:"rule"`
	SatisfiedBy string       `json:"satisfied_by,omitempty"`
	// Round is the collection round in which the demand was first raised. It is what
	// separates a demand nobody has tried to answer yet from one a collection round
	// attempted and no source could supply — two states that must not drive the same
	// control flow.
	Round int `json:"round,omitempty"`
}

// Satisfied reports whether evidence answering this demand has been collected.
func (d EvidenceDemand) Satisfied() bool { return d.SatisfiedBy != "" }

// Validate enforces the demand invariants.
func (d EvidenceDemand) Validate() error {
	if strings.TrimSpace(d.Descriptor) == "" {
		return ErrMissingDescriptor
	}
	if !d.Kind.Valid() {
		return ErrUnknownDemandKind
	}
	if strings.TrimSpace(d.Reason) == "" {
		return ErrMissingDemandCause
	}
	return nil
}
