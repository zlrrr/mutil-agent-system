// Package catalog holds the embedded fault signatures and fault cases. Both are data,
// so extending coverage is a reviewable diff rather than a code change (ADR-006).
package catalog

import (
	"sort"
	"strings"
	"time"

	"github.com/zlrrr/mutil-agent-system/internal/domain"
)

// sdd:impl DLD-1031

// Pattern is one evidence requirement of a signature.
type Pattern struct {
	Kind      domain.EvidenceKind `json:"kind"`
	Match     []string            `json:"match"`
	Saturated bool                `json:"saturated,omitempty"`
	Label     string              `json:"label"`
	// Demand is the descriptor a critic issues when this pattern goes unmatched.
	Demand string `json:"demand,omitempty"`
}

// Discriminator names evidence that would tell this signature apart from a rival.
type Discriminator struct {
	Descriptor string              `json:"descriptor"`
	Kind       domain.EvidenceKind `json:"kind"`
	Reason     string              `json:"reason"`
}

// RemediationTemplate is the action a signature implies, before its arguments are
// materialised from the matched change evidence.
type RemediationTemplate struct {
	Title       string            `json:"title"`
	Tool        string            `json:"tool"`
	Args        map[string]string `json:"args"`
	Risk        domain.Risk       `json:"risk"`
	Rationale   string            `json:"rationale"`
	Precond     []string          `json:"preconditions,omitempty"`
	RollbackArg map[string]string `json:"rollback_args,omitempty"`
	Verify      []string          `json:"verify"`
	// RestoreFromChange, when set, names the change key whose previous value should
	// be written back, so the action reflects what actually changed.
	RestoreFromChange string `json:"restore_from_change,omitempty"`
}

// Signature is one recognisable fault mechanism.
type Signature struct {
	ID             string               `json:"id"`
	Claim          string               `json:"claim"`
	Mechanism      string               `json:"mechanism"`
	Requires       []Pattern            `json:"requires"`
	Discriminators []Discriminator      `json:"discriminators,omitempty"`
	Remediation    *RemediationTemplate `json:"remediation,omitempty"`
	RunbookIDs     []string             `json:"runbook_ids,omitempty"`
	AttributesTo   string               `json:"attributes_to,omitempty"` // "change" when it blames a change
}

// PatternMatch records that one pattern was satisfied by one piece of evidence.
type PatternMatch struct {
	Pattern    Pattern
	EvidenceID string
	Kind       domain.EvidenceKind
}

// MatchResult is the outcome of matching a signature against an evidence set.
type MatchResult struct {
	Signature  Signature
	Matched    []PatternMatch
	Unmatched  []Pattern
	Supporting []string
	// FractionByKind is the share of that kind's required patterns which matched.
	FractionByKind map[domain.EvidenceKind]float64
	// ChangeAt is the timestamp of the matched change, when one matched.
	ChangeAt    time.Time
	HasChangeAt bool
}

// Any reports whether at least one required pattern matched.
func (m MatchResult) Any() bool { return len(m.Matched) > 0 }

// Fraction returns the matched share for a kind, or zero when the signature declares
// no pattern of that kind — a signature with no log requirement is genuinely
// unsupported by logs, and its score should say so.
func (m MatchResult) Fraction(k domain.EvidenceKind) float64 {
	return m.FractionByKind[k]
}

// UnmatchedDemands returns the demand descriptors for patterns that found no evidence.
func (m MatchResult) UnmatchedDemands() []Pattern {
	out := make([]Pattern, 0, len(m.Unmatched))
	for _, p := range m.Unmatched {
		if p.Demand != "" {
			out = append(out, p)
		}
	}
	return out
}

// matchPattern reports whether one evidence item satisfies a pattern. Matching is
// case-insensitive over the summary and every fact value, and order-independent.
func matchPattern(p Pattern, e domain.Evidence) bool {
	if e.Kind != p.Kind {
		return false
	}
	if p.Saturated && e.Fact("saturated") != "true" {
		return false
	}
	hay := e.SearchText()
	for _, needle := range p.Match {
		if !strings.Contains(hay, strings.ToLower(needle)) {
			return false
		}
	}
	return true
}

// Match evaluates a signature against an evidence set. Evidence is considered in
// identifier order, so the same set always produces the same matches.
func Match(sig Signature, evidence []domain.Evidence) MatchResult {
	ordered := append([]domain.Evidence(nil), evidence...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })

	res := MatchResult{Signature: sig, FractionByKind: map[domain.EvidenceKind]float64{}}
	required := map[domain.EvidenceKind]int{}
	matchedCount := map[domain.EvidenceKind]int{}
	support := map[string]bool{}

	for _, p := range sig.Requires {
		required[p.Kind]++
		var hit *domain.Evidence
		for i := range ordered {
			if matchPattern(p, ordered[i]) {
				hit = &ordered[i]
				break
			}
		}
		if hit == nil {
			res.Unmatched = append(res.Unmatched, p)
			continue
		}
		matchedCount[p.Kind]++
		res.Matched = append(res.Matched, PatternMatch{Pattern: p, EvidenceID: hit.ID, Kind: p.Kind})
		support[hit.ID] = true
		if p.Kind == domain.KindChange && !res.HasChangeAt {
			if ts := hit.Fact("changed_at"); ts != "" {
				if t, err := time.Parse(time.RFC3339, ts); err == nil {
					res.ChangeAt, res.HasChangeAt = t, true
				}
			}
		}
	}

	for kind, n := range required {
		if n > 0 {
			res.FractionByKind[kind] = float64(matchedCount[kind]) / float64(n)
		}
	}

	res.Supporting = make([]string, 0, len(support))
	for id := range support {
		res.Supporting = append(res.Supporting, id)
	}
	sort.Strings(res.Supporting)
	return res
}

// AddSupport records an evidence identifier as supporting, keeping the list sorted and
// duplicate-free. Used for evidence that contributes a term without matching a required
// pattern — topology and runbook evidence, for example.
func (m *MatchResult) AddSupport(id string) {
	for _, existing := range m.Supporting {
		if existing == id {
			return
		}
	}
	m.Supporting = append(m.Supporting, id)
	sort.Strings(m.Supporting)
}
