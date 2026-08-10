// Package plan holds the strategy port for the two judgements the investigation makes
// before it has any evidence: what to investigate, and what to look at.
//
// Those were not decisions before this package existed. Each collector asked its source
// for everything on offer and analysed all of it, so "what did round one see" — which in
// this system determines the first-round error, and therefore what the critic has to
// overturn (REQ-0103) — was a property of the fixture rather than of any agent. The most
// consequential choice in the flow was the one choice no role was accountable for
// (ADR-008, REQ-0107).
package plan

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/zlrrr/mutil-agent-system/internal/domain"
)

// sdd:impl DLD-1036

// Plan and Queries live in domain, not here: they are case state, recorded on the event
// log and read back by every agent, and a type the domain cannot name is a type the case
// cannot carry.
type (
	Plan    = domain.InvestigationPlan
	Queries = domain.QueryPlan
)

// Available is what the sources actually offer, so a strategy chooses from a real menu
// rather than inventing series names.
type Available struct {
	Series []string
}

// Planner decides what to investigate and what to look at. Its default adapter is
// deterministic; a model-backed adapter implements the same port (ARC-019).
type Planner interface {
	Name() string
	Triage(ctx context.Context, s domain.Snapshot) (Plan, error)
	PlanCollection(ctx context.Context, s domain.Snapshot, a Available) (Queries, error)
}

// DefaultLogTerms are the error terms a collection plan searches for when nothing narrows
// them.
var DefaultLogTerms = []string{
	"error", "timeout", "refused", "exception", "panic", "slow query", "exhausted",
}

// RulePlanner is the deterministic adapter. It reproduces exactly what the collectors did
// before this port existed, which is what lets the port be introduced as a refactor with
// the existing suite as its regression check.
type RulePlanner struct {
	lookback time.Duration
}

// NewRulePlanner builds the deterministic planner. The lookback extends the window before
// the incident so a change that preceded the symptom can be found.
func NewRulePlanner(lookback time.Duration) *RulePlanner {
	return &RulePlanner{lookback: lookback}
}

// Name identifies this strategy in the recorded plan.
func (p *RulePlanner) Name() string { return "rule" }

// Triage returns the alert window extended by the lookback, and every collector role.
func (p *RulePlanner) Triage(_ context.Context, s domain.Snapshot) (Plan, error) {
	w := s.Window
	if w.Start.IsZero() && !s.Alert.StartsAt.IsZero() {
		w = domain.TimeWindow{Start: s.Alert.StartsAt, End: s.Alert.EndsAt}
	}
	roles := append([]domain.Role(nil), domain.CollectorRoles...)
	return Plan{
		Window: w,
		Roles:  roles,
		Reason: fmt.Sprintf("alert window extended by %s so a change preceding the symptom is in range", p.lookback),
	}, nil
}

// PlanCollection asks for everything the sources offer.
//
// That is a poor strategy and a correct baseline. It is the plan a narrower choice has to
// justify itself against, and it is what the collectors did before the decision existed —
// so adopting the port changes no observable output.
func (p *RulePlanner) PlanCollection(_ context.Context, _ domain.Snapshot, a Available) (Queries, error) {
	series := append([]string(nil), a.Series...)
	// Sorted because the plan is recorded on the case, and an unordered plan would make
	// two identical investigations produce different records (REQ-0090).
	sort.Strings(series)
	return Queries{
		Series:   series,
		LogTerms: append([]string(nil), DefaultLogTerms...),
		Reason:   "every series the source offers, with the default error terms",
	}, nil
}

// Resolve applies a planned collection to what is actually available, dropping names no
// source offers and reporting them so the omission is visible rather than silent.
//
// A plan that survives this with nothing left is replaced by the deterministic plan: a
// strategy that cannot answer must not be able to blind the investigation (REQ-0107).
func Resolve(q Queries, a Available, fallback Queries) (Queries, []string) {
	offered := map[string]bool{}
	for _, name := range a.Series {
		offered[name] = true
	}
	var kept, dropped []string
	for _, name := range q.Series {
		if offered[name] {
			kept = append(kept, name)
			continue
		}
		dropped = append(dropped, name)
	}
	sort.Strings(kept)
	sort.Strings(dropped)

	out := q
	out.Series = kept
	if out.Empty() {
		return fallback, dropped
	}
	return out, dropped
}
