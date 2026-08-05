// Package fixture provides the deterministic, in-process adapters for every signal
// port. It is the primary path, not a degraded one: the whole flow runs on it offline
// and reproducibly (ARC-006, ADR-002).
package fixture

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/zlrrr/mutil-agent-system/internal/catalog"
	"github.com/zlrrr/mutil-agent-system/internal/domain"
	"github.com/zlrrr/mutil-agent-system/internal/signal"
)

// sdd:impl DLD-1023

// State is the mutable part of a fixture environment: the simulated configuration and
// whether the recovery trigger has fired. The metric source and the actuator share it,
// which is how verification observes recovery only after the action ran.
type State struct {
	mu        sync.RWMutex
	config    map[string]string
	recovered bool
	calls     []domain.ActionCall
}

// NewState seeds a fixture environment from a fault case.
func NewState(fc catalog.FaultCase) *State {
	cfg := make(map[string]string, len(fc.InitialConfig))
	for k, v := range fc.InitialConfig {
		cfg[k] = v
	}
	return &State{config: cfg}
}

// Recovered reports whether the recovery trigger has fired.
func (s *State) Recovered() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.recovered
}

// Calls returns the actuator invocations recorded so far.
func (s *State) Calls() []domain.ActionCall {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.ActionCall, len(s.calls))
	copy(out, s.calls)
	return out
}

// Config returns a copy of the simulated configuration.
func (s *State) Config() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]string, len(s.config))
	for k, v := range s.config {
		out[k] = v
	}
	return out
}

// NewSet builds every port from one fault case definition, plus the shared state so a
// caller (a test, or the executor) can inspect what was actuated.
func NewSet(fc catalog.FaultCase, cat *catalog.Catalog, bounds signal.Bounds) (signal.Set, *State) {
	st := NewState(fc)
	return signal.Set{
		Metrics:   &metricSource{fc: fc, st: st},
		Logs:      &logSource{fc: fc},
		Changes:   &changeSource{fc: fc},
		Topology:  &topologySource{fc: fc},
		Knowledge: &knowledgeSource{cat: cat},
		Actuator:  &Actuator{fc: fc, st: st},
		Demands:   &demandResponder{fc: fc, st: st},
		Bounds:    bounds,
	}, st
}

// ---------------------------------------------------------------- metric source

type metricSource struct {
	fc catalog.FaultCase
	st *State
}

func (m *metricSource) Range(_ context.Context, q signal.MetricQuery) (signal.Series, error) {
	s, ok := m.resolve(q.Series)
	if !ok {
		return signal.Series{}, fmt.Errorf("series %q is not declared for case %s", q.Series, m.fc.ID)
	}
	return s, nil
}

// resolve returns the post-recovery variant of a series once the recovery trigger has
// fired, and the incident variant otherwise.
func (m *metricSource) resolve(name string) (signal.Series, bool) {
	if m.st.Recovered() {
		if s, ok := m.fc.PostRecoveryByName(name); ok {
			return s, true
		}
	}
	return m.fc.SeriesByName(name)
}

// SeriesNames returns only the series a collector reaches without being asked. Series
// outside this list exist but require a demand — which is what makes the first round
// incomplete (DLD-1040).
func (m *metricSource) SeriesNames(_ context.Context, _ string) ([]string, error) {
	out := append([]string(nil), m.fc.DefaultSeries...)
	return out, nil
}

// ------------------------------------------------------------------- log source

type logSource struct{ fc catalog.FaultCase }

func (l *logSource) Search(_ context.Context, q signal.LogQuery) ([]signal.LogLine, error) {
	var out []signal.LogLine
	for _, line := range l.fc.Logs {
		if q.Service != "" && line.Service != q.Service {
			continue
		}
		if !q.Window.Contains(line.At) {
			continue
		}
		if len(q.Terms) > 0 && !matchesAny(line.Message, q.Terms) {
			continue
		}
		out = append(out, line)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out, nil
}

func matchesAny(s string, terms []string) bool {
	low := strings.ToLower(s)
	for _, t := range terms {
		if strings.Contains(low, strings.ToLower(t)) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------- change source

type changeSource struct{ fc catalog.FaultCase }

func (c *changeSource) Changes(_ context.Context, service string, w domain.TimeWindow) ([]signal.Change, error) {
	var out []signal.Change
	for _, ch := range c.fc.Changes {
		if service != "" && ch.Service != service {
			continue
		}
		if !w.Contains(ch.At) {
			continue
		}
		out = append(out, ch)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out, nil
}

// -------------------------------------------------------------- topology source

type topologySource struct{ fc catalog.FaultCase }

func (t *topologySource) Neighbourhood(_ context.Context, _ string) (signal.Topology, error) {
	return t.fc.Topology, nil
}

// ------------------------------------------------------------- knowledge source

type knowledgeSource struct{ cat *catalog.Catalog }

// Search scores each runbook by the share of its declared symptoms that appear in the
// incident's symptom corpus. Retrieved text is data and never instruction (CON-012).
func (k *knowledgeSource) Search(_ context.Context, q signal.KnowledgeQuery) ([]signal.Runbook, error) {
	corpus := strings.ToLower(strings.Join(q.Symptoms, " | "))
	var out []signal.Runbook
	for _, rb := range k.cat.Runbooks {
		var matched []string
		for _, sym := range rb.Symptoms {
			if strings.Contains(corpus, strings.ToLower(sym)) {
				matched = append(matched, sym)
			}
		}
		if len(matched) == 0 || len(rb.Symptoms) == 0 {
			continue
		}
		hit := rb
		hit.Matched = matched
		hit.Score = float64(len(matched)) / float64(len(rb.Symptoms))
		out = append(out, hit)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// ------------------------------------------------------------ demand responder

type demandResponder struct {
	fc catalog.FaultCase
	st *State
}

// Respond serves the evidence a fault case has registered against a demand descriptor.
// A response naming a series is analysed here so collectors stay simple; a response
// naming changes reaches back over the extended lookback window.
func (d *demandResponder) Respond(_ context.Context, descriptor string, kind domain.EvidenceKind) (signal.DemandEvidence, bool) {
	r, ok := d.fc.ResponseFor(descriptor)
	if !ok || r.Kind != kind {
		return signal.DemandEvidence{}, false
	}

	out := signal.DemandEvidence{
		Descriptor: descriptor,
		Kind:       r.Kind,
		Source:     r.Source,
		Summary:    r.Summary,
		RawRef:     r.RawRef,
		Confidence: r.Confidence,
		Facts:      map[string]string{},
	}
	for k, v := range r.Facts {
		out.Facts[k] = v
	}

	switch {
	case r.Series != "":
		s, ok := d.fc.SeriesByName(r.Series)
		if !ok {
			return signal.DemandEvidence{}, false
		}
		a := signal.Analyse(s, d.fc.Window())
		if out.Summary == "" {
			out.Summary = a.Summary()
		}
		for k, v := range a.Facts() {
			if _, exists := out.Facts[k]; !exists {
				out.Facts[k] = v
			}
		}
	case r.Changes:
		w := d.fc.Window().Extend(30 * time.Minute)
		var found []signal.Change
		for _, ch := range d.fc.Changes {
			if ch.Service == d.fc.Alert.Service && w.Contains(ch.At) {
				found = append(found, ch)
			}
		}
		sort.SliceStable(found, func(i, j int) bool { return found[i].At.Before(found[j].At) })
		if len(found) == 0 {
			out.Summary = "no configuration or deployment change is recorded for " +
				d.fc.Alert.Service + " in the 30 minutes before onset"
			out.Facts["changes_found"] = "0"
			return out, true
		}
		ch := found[len(found)-1]
		out.Summary = fmt.Sprintf("%s changed %s from %q to %q at %s (%s)",
			ch.Service, ch.Key, ch.Old, ch.New, ch.At.UTC().Format("15:04:05"), ch.Ref)
		out.Facts["change_key"] = ch.Key
		out.Facts["change_old"] = ch.Old
		out.Facts["change_new"] = ch.New
		out.Facts["changed_at"] = ch.At.UTC().Format(time.RFC3339)
		out.Facts["change_type"] = ch.Type
		out.Facts["changes_found"] = fmt.Sprint(len(found))
	}

	if out.Confidence == 0 {
		out.Confidence = 0.8
	}
	return out, true
}
