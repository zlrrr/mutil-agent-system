package domain

import (
	"errors"
	"sort"
	"strings"
)

// sdd:impl DLD-1002

// EvidenceKind classifies an observation by the source that produced it. The set is
// closed: progression guards are expressed over it (ARC-007).
type EvidenceKind string

// The evidence kinds a case can collect.
const (
	KindMetric       EvidenceKind = "metric"
	KindLog          EvidenceKind = "log"
	KindChange       EvidenceKind = "change"
	KindTopology     EvidenceKind = "topology"
	KindKnowledge    EvidenceKind = "knowledge"
	KindVerification EvidenceKind = "verification"
)

// AllEvidenceKinds lists every kind in a stable order.
var AllEvidenceKinds = []EvidenceKind{
	KindMetric, KindLog, KindChange, KindTopology, KindKnowledge, KindVerification,
}

// Valid reports whether k is a known evidence kind.
func (k EvidenceKind) Valid() bool {
	for _, v := range AllEvidenceKinds {
		if v == k {
			return true
		}
	}
	return false
}

// Evidence is an immutable observation from one source, bounded to a time window and
// traceable to the query that produced it (REQ-0002).
type Evidence struct {
	ID         string            `json:"id"`
	Kind       EvidenceKind      `json:"kind"`
	Agent      Role              `json:"agent"`
	Source     string            `json:"source"`
	Window     TimeWindow        `json:"window"`
	Summary    string            `json:"summary"`
	RawRef     string            `json:"raw_ref"`
	Confidence float64           `json:"confidence"`
	Truncated  bool              `json:"truncated,omitempty"`
	Facts      map[string]string `json:"facts,omitempty"`
}

// Validation errors for evidence, distinguishable by the caller.
var (
	ErrUnknownEvidenceKind = errors.New("unknown evidence kind")
	ErrConfidenceRange     = errors.New("confidence must be within [0,1]")
	ErrMissingRawRef       = errors.New("evidence must carry a raw query reference")
	ErrMissingSource       = errors.New("evidence must name its source")
	ErrMissingSummary      = errors.New("evidence must carry a summary")
)

// Validate enforces the invariants an evidence record must satisfy before the
// orchestrator will store it.
func (e Evidence) Validate() error {
	if !e.Kind.Valid() {
		return ErrUnknownEvidenceKind
	}
	if e.Confidence < 0 || e.Confidence > 1 {
		return ErrConfidenceRange
	}
	if strings.TrimSpace(e.RawRef) == "" {
		return ErrMissingRawRef
	}
	if strings.TrimSpace(e.Source) == "" {
		return ErrMissingSource
	}
	if strings.TrimSpace(e.Summary) == "" {
		return ErrMissingSummary
	}
	return nil
}

// Fact returns a fact value, or the empty string when absent.
func (e Evidence) Fact(key string) string { return e.Facts[key] }

// FactsSorted returns the facts as key-ordered pairs so rendering and matching are
// deterministic regardless of map iteration order.
func (e Evidence) FactsSorted() [][2]string {
	keys := make([]string, 0, len(e.Facts))
	for k := range e.Facts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([][2]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, [2]string{k, e.Facts[k]})
	}
	return out
}

// SearchText is the lower-cased concatenation of the summary and every fact value,
// used by signature pattern matching (DLD-1031).
func (e Evidence) SearchText() string {
	var b strings.Builder
	b.WriteString(strings.ToLower(e.Summary))
	for _, kv := range e.FactsSorted() {
		b.WriteByte(' ')
		b.WriteString(strings.ToLower(kv[0]))
		b.WriteByte('=')
		b.WriteString(strings.ToLower(kv[1]))
	}
	return b.String()
}

// DistinctKinds returns the sorted distinct kinds present in a set of evidence.
func DistinctKinds(ev []Evidence) []EvidenceKind {
	seen := map[EvidenceKind]bool{}
	for _, e := range ev {
		seen[e.Kind] = true
	}
	var out []EvidenceKind
	for _, k := range AllEvidenceKinds {
		if seen[k] {
			out = append(out, k)
		}
	}
	return out
}
