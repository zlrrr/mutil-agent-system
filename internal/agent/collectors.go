package agent

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/zlrrr/mutil-agent-system/internal/agent/plan"
	"github.com/zlrrr/mutil-agent-system/internal/domain"
	"github.com/zlrrr/mutil-agent-system/internal/reasoner"
	"github.com/zlrrr/mutil-agent-system/internal/signal"
)

// sdd:impl DLD-1040

// Collectors returns the five evidence-gathering agents, in the fixed order their
// contributions are applied.
func Collectors(set signal.Set, cfg reasoner.Config) []Agent {
	return []Agent{
		New(domain.RoleMetrics, func(ctx context.Context, s domain.Snapshot) ([]domain.Contribution, error) {
			return collectMetrics(ctx, set, cfg, s)
		}),
		New(domain.RoleLogs, func(ctx context.Context, s domain.Snapshot) ([]domain.Contribution, error) {
			return collectLogs(ctx, set, cfg, s)
		}),
		New(domain.RoleChange, func(ctx context.Context, s domain.Snapshot) ([]domain.Contribution, error) {
			return collectChanges(ctx, set, cfg, s)
		}),
		New(domain.RoleTopology, func(ctx context.Context, s domain.Snapshot) ([]domain.Contribution, error) {
			return collectTopology(ctx, set, s)
		}),
		New(domain.RoleKnowledge, func(ctx context.Context, s domain.Snapshot) ([]domain.Contribution, error) {
			return collectKnowledge(ctx, set, s)
		}),
	}
}

// ------------------------------------------------------------------ metrics role

func collectMetrics(ctx context.Context, set signal.Set, _ reasoner.Config, s domain.Snapshot) ([]domain.Contribution, error) {
	window, spanTrunc := set.Bounds.ApplySpan(s.Window)

	// The series to query come from the recorded collection plan, not from asking the
	// source for everything (DLD-1037). A snapshot without a plan — a caller that drives
	// a collector directly — falls back to every series on offer, which is what the
	// deterministic planner would have said anyway.
	names := s.Queries.Series
	if len(names) == 0 {
		var err error
		names, err = set.Metrics.SeriesNames(ctx, s.Alert.Service)
		if err != nil {
			return nil, fmt.Errorf("list series: %w", err)
		}
	}
	names = append([]string(nil), names...)
	sort.Strings(names)

	var out []domain.Contribution
	var analyses []signal.Analysis
	idx := 0

	for _, name := range names {
		series, err := set.Metrics.Range(ctx, signal.MetricQuery{
			Service: s.Alert.Service, Series: name, Window: window,
		})
		if err != nil {
			continue // a series the environment does not expose is not an error
		}
		pts, rowTrunc := set.Bounds.ApplyRowsPoints(series.Points)
		series.Points = pts
		a := signal.Analyse(series, window)
		analyses = append(analyses, a)

		summary, charTrunc := set.Bounds.ApplyChars(a.Summary())
		out = append(out, domain.AddEvidence(domain.RoleMetrics, idx, domain.Evidence{
			Kind:       domain.KindMetric,
			Agent:      domain.RoleMetrics,
			Source:     "metrics",
			Window:     window,
			Summary:    summary,
			RawRef:     fmt.Sprintf("promql:%s{service=%q}", name, s.Alert.Service),
			Confidence: confidenceFor(a),
			Truncated:  rowTrunc || charTrunc || spanTrunc,
			Facts:      a.Facts(),
		}))
		idx++
	}

	// Co-movement is reported as timing, never as causation (REQ-0011).
	if co := coMoving(analyses); len(co) >= 2 {
		out = append(out, domain.AddEvidence(domain.RoleMetrics, idx, domain.Evidence{
			Kind:   domain.KindMetric,
			Agent:  domain.RoleMetrics,
			Source: "metrics",
			Window: window,
			Summary: fmt.Sprintf(
				"%s began within %s of each other; they moved together in time",
				strings.Join(co, ", "), signal.CoMovementWin),
			RawRef:     "analysis:co-movement",
			Confidence: 0.7,
			Facts: map[string]string{
				"co_moving": strings.Join(co, ","),
				"window":    signal.CoMovementWin.String(),
			},
		}))
		idx++
	}

	// Demand pass: answer the critic's outstanding requests for metric evidence.
	demandOut, _ := collectDemands(ctx, set, s, domain.KindMetric, domain.RoleMetrics, idx)
	return append(out, demandOut...), nil
}

func confidenceFor(a signal.Analysis) float64 {
	switch {
	case a.Saturated:
		return 0.92
	case a.Anomalous:
		return 0.86
	default:
		return 0.75
	}
}

func coMoving(as []signal.Analysis) []string {
	var anomalous []signal.Analysis
	for _, a := range as {
		if a.Anomalous {
			anomalous = append(anomalous, a)
		}
	}
	var names []string
	for i := range anomalous {
		for j := i + 1; j < len(anomalous); j++ {
			if signal.CoMoving(anomalous[i], anomalous[j]) {
				names = appendUnique(names, anomalous[i].Series)
				names = appendUnique(names, anomalous[j].Series)
			}
		}
	}
	sort.Strings(names)
	return names
}

func appendUnique(s []string, v string) []string {
	for _, e := range s {
		if e == v {
			return s
		}
	}
	return append(s, v)
}

// --------------------------------------------------------------------- logs role

func collectLogs(ctx context.Context, set signal.Set, _ reasoner.Config, s domain.Snapshot) ([]domain.Contribution, error) {
	window, spanTrunc := set.Bounds.ApplySpan(s.Window)

	terms := s.Queries.LogTerms
	if len(terms) == 0 {
		terms = plan.DefaultLogTerms
	}
	lines, err := set.Logs.Search(ctx, signal.LogQuery{
		Service: s.Alert.Service, Terms: terms, Window: window,
	})
	if err != nil {
		return nil, fmt.Errorf("search logs: %w", err)
	}
	bounded, rowTrunc := set.Bounds.ApplyRowsLog(lines)
	clusters := signal.ClusterLines(bounded)

	var out []domain.Contribution
	for i, c := range clusters {
		summary, charTrunc := set.Bounds.ApplyChars(fmt.Sprintf(
			"%d lines matched the template %q, first at %s and last at %s",
			c.Count, c.Template, c.FirstSeen.UTC().Format("15:04:05"),
			c.LastSeen.UTC().Format("15:04:05")))
		out = append(out, domain.AddEvidence(domain.RoleLogs, i, domain.Evidence{
			Kind:       domain.KindLog,
			Agent:      domain.RoleLogs,
			Source:     "logs",
			Window:     window,
			Summary:    summary,
			RawRef:     fmt.Sprintf("logs:%s?template=%q", s.Alert.Service, c.Template),
			Confidence: clusterConfidence(c, len(bounded)),
			Truncated:  rowTrunc || charTrunc || spanTrunc,
			Facts: map[string]string{
				"template":   c.Template,
				"count":      strconv.Itoa(c.Count),
				"first_seen": c.FirstSeen.UTC().Format(time.RFC3339),
				"last_seen":  c.LastSeen.UTC().Format(time.RFC3339),
				"level":      c.Level,
				"samples":    strings.Join(c.Samples, " | "),
			},
		}))
	}

	demandOut, _ := collectDemands(ctx, set, s, domain.KindLog, domain.RoleLogs, len(out))
	return append(out, demandOut...), nil
}

func clusterConfidence(c signal.Cluster, total int) float64 {
	if total == 0 {
		return 0.5
	}
	share := float64(c.Count) / float64(total)
	return 0.6 + 0.3*share
}

// ------------------------------------------------------------------- change role

// collectChanges queries the incident window only. The lookback extension is applied
// exclusively when a demand asks for it — which is what makes the first round
// incomplete and the critic's demand consequential (DLD-1040).
func collectChanges(ctx context.Context, set signal.Set, _ reasoner.Config, s domain.Snapshot) ([]domain.Contribution, error) {
	// The default pass asks "what changed since the alert fired" — the question an
	// operator asks first. Reaching further back is exactly what the critic must
	// demand, and the demand pass below is where that happens.
	window, spanTrunc := set.Bounds.ApplySpan(domain.TimeWindow{
		Start: s.Alert.StartsAt, End: s.Window.End,
	})

	changes, err := set.Changes.Changes(ctx, s.Alert.Service, window)
	if err != nil {
		return nil, fmt.Errorf("read changes: %w", err)
	}
	bounded, rowTrunc := set.Bounds.ApplyRowsChanges(changes)

	var out []domain.Contribution
	for i, ch := range bounded {
		summary, charTrunc := set.Bounds.ApplyChars(fmt.Sprintf(
			"%s changed %s from %q to %q at %s (%s)",
			ch.Service, ch.Key, ch.Old, ch.New, ch.At.UTC().Format("15:04:05"), ch.Ref))
		out = append(out, domain.AddEvidence(domain.RoleChange, i, domain.Evidence{
			Kind:       domain.KindChange,
			Agent:      domain.RoleChange,
			Source:     "change-history",
			Window:     window,
			Summary:    summary,
			RawRef:     fmt.Sprintf("change-history:%s?ref=%s", ch.Service, ch.Ref),
			Confidence: 0.9,
			Truncated:  rowTrunc || charTrunc || spanTrunc,
			Facts: map[string]string{
				"change_key":  ch.Key,
				"change_old":  ch.Old,
				"change_new":  ch.New,
				"changed_at":  ch.At.UTC().Format(time.RFC3339),
				"change_type": ch.Type,
				"revision":    ch.Revision,
			},
		}))
	}
	if len(out) == 0 {
		out = append(out, domain.AddEvidence(domain.RoleChange, 0, domain.Evidence{
			Kind:   domain.KindChange,
			Agent:  domain.RoleChange,
			Source: "change-history",
			Window: window,
			Summary: fmt.Sprintf(
				"no change is recorded for %s inside the incident window", s.Alert.Service),
			RawRef:     fmt.Sprintf("change-history:%s?window=%s", s.Alert.Service, window),
			Confidence: 0.6,
			Facts:      map[string]string{"changes_found": "0"},
		}))
	}

	demandOut, _ := collectDemands(ctx, set, s, domain.KindChange, domain.RoleChange, len(out))
	return append(out, demandOut...), nil
}

// ----------------------------------------------------------------- topology role

func collectTopology(ctx context.Context, set signal.Set, s domain.Snapshot) ([]domain.Contribution, error) {
	topo, err := set.Topology.Neighbourhood(ctx, s.Alert.Service)
	if err != nil {
		return nil, fmt.Errorf("read topology: %w", err)
	}

	var anomalous []signal.ServiceNode
	for _, n := range topo.Nodes {
		if n.Anomalous {
			anomalous = append(anomalous, n)
		}
	}
	sort.Slice(anomalous, func(i, j int) bool {
		if !anomalous[i].FirstSeen.Equal(anomalous[j].FirstSeen) {
			return anomalous[i].FirstSeen.Before(anomalous[j].FirstSeen)
		}
		return anomalous[i].Name < anomalous[j].Name
	})

	var origins, affected []string
	for i, n := range anomalous {
		if i == 0 || n.FirstSeen.Equal(anomalous[0].FirstSeen) {
			origins = append(origins, n.Name)
		} else {
			affected = append(affected, n.Name)
		}
	}

	// A service that depends on an earlier-anomalous service may be a victim.
	var upstreamAnomalous string
	var upstreamAt time.Time
	self, hasSelf := topo.Node(s.Alert.Service)
	for _, up := range topo.Upstreams(s.Alert.Service) {
		n, ok := topo.Node(up)
		if !ok || !n.Anomalous {
			continue
		}
		if hasSelf && !n.FirstSeen.Before(self.FirstSeen) {
			continue
		}
		if upstreamAnomalous == "" || n.FirstSeen.Before(upstreamAt) {
			upstreamAnomalous, upstreamAt = n.Name, n.FirstSeen
		}
	}

	facts := map[string]string{
		"candidate_origin": strings.Join(origins, ","),
		"affected":         strings.Join(affected, ","),
		"upstreams":        strings.Join(topo.Upstreams(s.Alert.Service), ","),
		"downstreams":      strings.Join(topo.Downstreams(s.Alert.Service), ","),
	}
	summary := fmt.Sprintf(
		"%s is the earliest anomalous service in its neighbourhood; %s became anomalous later",
		strings.Join(origins, ", "), joinOr(affected, "no other service"))
	if upstreamAnomalous != "" {
		facts["upstream_anomalous"] = upstreamAnomalous
		facts["upstream_first_seen"] = upstreamAt.UTC().Format(time.RFC3339)
		summary = fmt.Sprintf(
			"%s depends on %s, which became anomalous earlier at %s; %s may be downstream of the origin",
			s.Alert.Service, upstreamAnomalous, upstreamAt.UTC().Format("15:04:05"),
			s.Alert.Service)
	}

	return []domain.Contribution{
		domain.AddEvidence(domain.RoleTopology, 0, domain.Evidence{
			Kind:       domain.KindTopology,
			Agent:      domain.RoleTopology,
			Source:     "topology",
			Window:     s.Window,
			Summary:    summary,
			RawRef:     fmt.Sprintf("topology:%s", s.Alert.Service),
			Confidence: 0.78,
			Facts:      facts,
		}),
	}, nil
}

func joinOr(v []string, empty string) string {
	if len(v) == 0 {
		return empty
	}
	return strings.Join(v, ", ")
}

// ---------------------------------------------------------------- knowledge role

// collectKnowledge searches the runbook corpus using a symptom set built from the alert
// and the log templates already collected — both stable across rounds, so a runbook
// score does not drift as evidence accumulates.
func collectKnowledge(ctx context.Context, set signal.Set, s domain.Snapshot) ([]domain.Contribution, error) {
	symptoms := []string{s.Alert.Name, s.Alert.Service}
	for _, v := range sortedValues(s.Alert.Annotations) {
		symptoms = append(symptoms, v)
	}
	for _, e := range s.EvidenceOfKind(domain.KindLog) {
		if t := e.Fact("template"); t != "" {
			symptoms = append(symptoms, t)
		}
	}

	books, err := set.Knowledge.Search(ctx, signal.KnowledgeQuery{
		Service: s.Alert.Service, Symptoms: symptoms,
	})
	if err != nil {
		return nil, fmt.Errorf("search knowledge: %w", err)
	}

	var out []domain.Contribution
	for i, rb := range books {
		summary, _ := set.Bounds.ApplyChars(fmt.Sprintf(
			"runbook %q matches %d of %d declared symptoms (%s)",
			rb.Title, len(rb.Matched), len(rb.Symptoms), strings.Join(rb.Matched, ", ")))
		out = append(out, domain.AddEvidence(domain.RoleKnowledge, i, domain.Evidence{
			Kind:       domain.KindKnowledge,
			Agent:      domain.RoleKnowledge,
			Source:     "runbooks",
			Window:     s.Window,
			Summary:    summary,
			RawRef:     "runbook:" + rb.ID,
			Confidence: 0.5 + 0.4*rb.Score,
			Facts: map[string]string{
				"runbook_id": rb.ID,
				"score":      strconv.FormatFloat(rb.Score, 'f', 4, 64),
				"matched":    strings.Join(rb.Matched, ","),
				"title":      rb.Title,
			},
		}))
	}
	return out, nil
}

func sortedValues(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, m[k])
	}
	return out
}

// ------------------------------------------------------------------ demand pass

// collectDemands answers the critic's outstanding requests for evidence of one kind.
// Evidence produced here carries Facts["demand"], which is how the orchestrator marks
// the demand satisfied.
func collectDemands(ctx context.Context, set signal.Set, s domain.Snapshot, kind domain.EvidenceKind, role domain.Role, startIndex int) ([]domain.Contribution, error) {
	if set.Demands == nil {
		return nil, nil
	}
	var out []domain.Contribution
	i := startIndex
	for _, d := range s.UnsatisfiedDemands() {
		if d.Kind != kind {
			continue
		}
		resp, ok := set.Demands.Respond(ctx, d.Descriptor, kind)
		if !ok {
			continue
		}
		facts := map[string]string{"demand": d.Descriptor}
		for k, v := range resp.Facts {
			facts[k] = v
		}
		summary, charTrunc := set.Bounds.ApplyChars(resp.Summary)
		out = append(out, domain.AddEvidence(role, i, domain.Evidence{
			Kind:       kind,
			Agent:      role,
			Source:     resp.Source,
			Window:     s.Window,
			Summary:    summary,
			RawRef:     resp.RawRef,
			Confidence: resp.Confidence,
			Truncated:  charTrunc,
			Facts:      facts,
		}))
		i++
	}
	return out, nil
}
