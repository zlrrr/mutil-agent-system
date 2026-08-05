// Package domain holds the entities every plane exchanges: evidence, hypotheses,
// critiques, actions, the contribution algebra that carries them, and the event log
// the case projection is folded from.
//
// It imports nothing from internal/ (ARC-001) and uses the standard library only.
package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// sdd:impl DLD-1001

// Role names an agent role. Roles are language-neutral identifiers that appear in
// identifiers, events and the console.
type Role string

// The roles that participate in an investigation.
const (
	RoleMetrics      Role = "metrics"
	RoleLogs         Role = "logs"
	RoleChange       Role = "change"
	RoleTopology     Role = "topology"
	RoleKnowledge    Role = "knowledge"
	RoleAnalysis     Role = "analysis"
	RoleCritic       Role = "critic"
	RoleRemediation  Role = "remediation"
	RoleExecutor     Role = "executor"
	RoleVerification Role = "verification"
	RoleBaseline     Role = "baseline"
	RoleOrchestrator Role = "orchestrator"
	RoleOperator     Role = "operator"
)

// CollectorRoles is the fixed order in which collector contributions are applied.
// Ordering here — rather than at completion time — is what makes concurrent
// collection deterministic (ARC-004).
var CollectorRoles = []Role{RoleMetrics, RoleLogs, RoleChange, RoleTopology, RoleKnowledge}

// CollectorOrder returns the application rank of a collector role, and false when the
// role is not a collector.
func CollectorOrder(r Role) (int, bool) {
	for i, c := range CollectorRoles {
		if c == r {
			return i, true
		}
	}
	return 0, false
}

// Identifier prefixes. The prefix makes an identifier self-describing in a report.
const (
	PrefixEvidence   = "e"
	PrefixHypothesis = "h"
	PrefixCritique   = "c"
	PrefixDemand     = "d"
	PrefixAction     = "a"
)

// Alert is the external trigger that opens a case.
type Alert struct {
	Name        string            `json:"alert_name"`
	Service     string            `json:"service"`
	Severity    string            `json:"severity"`
	StartsAt    time.Time         `json:"starts_at"`
	EndsAt      time.Time         `json:"ends_at,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	CaseRef     string            `json:"case_ref,omitempty"` // fault case identifier for the fixture profile
}

// CaseID derives a stable case identifier from the alert itself. No clock and no
// random source participate, so the same alert always opens the same case (REQ-0004).
func CaseID(a Alert) string {
	h := sha256.Sum256([]byte(a.Name + "|" + a.Service + "|" + a.StartsAt.UTC().Format(time.RFC3339)))
	return "inc-" + hex.EncodeToString(h[:])[:10]
}

// NewID builds a case-scoped entity identifier from a prefix, the producing role and a
// per-role sequence number.
func NewID(prefix string, role Role, seq int) string {
	return fmt.Sprintf("%s-%s-%03d", prefix, role, seq)
}

// TimeWindow is a closed interval.
type TimeWindow struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// Extend returns a window whose start is moved back by d.
func (w TimeWindow) Extend(d time.Duration) TimeWindow {
	return TimeWindow{Start: w.Start.Add(-d), End: w.End}
}

// Contains reports whether t lies within the window, inclusive of both ends.
func (w TimeWindow) Contains(t time.Time) bool {
	return !t.Before(w.Start) && !t.After(w.End)
}

// Duration is the length of the window.
func (w TimeWindow) Duration() time.Duration { return w.End.Sub(w.Start) }

// String renders the window in the form stored in evidence records.
func (w TimeWindow) String() string {
	return w.Start.UTC().Format(time.RFC3339) + "/" + w.End.UTC().Format(time.RFC3339)
}

// Clock supplies timestamps. The fixture profile uses a logical clock so that a run is
// byte-reproducible (REQ-0090); the live profile uses wall time.
type Clock interface {
	Now() time.Time
}

// LogicalClock advances by a fixed step on every read, making every timestamp in a run
// a pure function of the run's inputs.
type LogicalClock struct {
	current time.Time
	step    time.Duration
}

// NewLogicalClock starts a logical clock at start, advancing by step per read.
func NewLogicalClock(start time.Time, step time.Duration) *LogicalClock {
	return &LogicalClock{current: start, step: step}
}

// Now returns the current logical time and advances the clock.
func (c *LogicalClock) Now() time.Time {
	t := c.current
	c.current = c.current.Add(c.step)
	return t
}

// WallClock reports real time.
type WallClock struct{}

// Now returns the current wall-clock time in UTC.
func (WallClock) Now() time.Time { return time.Now().UTC() }
