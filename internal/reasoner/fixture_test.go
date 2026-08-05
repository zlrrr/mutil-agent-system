package reasoner

import (
	"testing"
	"time"

	"github.com/zlrrr/mutil-agent-system/internal/catalog"
	"github.com/zlrrr/mutil-agent-system/internal/domain"
)

var caseStart = time.Date(2026, 7, 26, 10, 7, 0, 0, time.UTC)

func at(minutes int) string {
	return caseStart.Add(time.Duration(minutes) * time.Minute).UTC().Format(time.RFC3339)
}

// evidence builders keep the reasoning tests readable and independent of the fixture
// adapters, so a rule can be triggered in isolation.

func metric(id, series string, anomalous, saturated bool, onsetMinutes int) domain.Evidence {
	facts := map[string]string{
		"series":    series,
		"anomalous": boolStr(anomalous),
		"saturated": boolStr(saturated),
	}
	if anomalous {
		facts["onset"] = at(onsetMinutes)
	}
	return domain.Evidence{
		ID: id, Kind: domain.KindMetric, Agent: domain.RoleMetrics, Source: "metrics",
		Summary: series + " observed", RawRef: "promql:" + series, Confidence: 0.86,
		Facts: facts,
	}
}

func logEv(id, template string, count int) domain.Evidence {
	return domain.Evidence{
		ID: id, Kind: domain.KindLog, Agent: domain.RoleLogs, Source: "logs",
		Summary: template, RawRef: "logs:order-api", Confidence: 0.8,
		Facts: map[string]string{"template": template, "count": itoa(count)},
	}
}

func changeEv(id, key, old, new string, minutes int) domain.Evidence {
	return domain.Evidence{
		ID: id, Kind: domain.KindChange, Agent: domain.RoleChange, Source: "change-history",
		Summary: "order-api changed " + key, RawRef: "change-history:order-api", Confidence: 0.9,
		Facts: map[string]string{
			"change_key": key, "change_old": old, "change_new": new, "changed_at": at(minutes),
		},
	}
}

func topologyEv(id, origin, affected, upstreamAnomalous string) domain.Evidence {
	facts := map[string]string{"candidate_origin": origin, "affected": affected}
	if upstreamAnomalous != "" {
		facts["upstream_anomalous"] = upstreamAnomalous
		facts["upstream_first_seen"] = at(-3)
	}
	return domain.Evidence{
		ID: id, Kind: domain.KindTopology, Agent: domain.RoleTopology, Source: "topology",
		Summary: "neighbourhood of order-api", RawRef: "topology:order-api", Confidence: 0.78,
		Facts: facts,
	}
}

func knowledgeEv(id, runbookID, score string) domain.Evidence {
	return domain.Evidence{
		ID: id, Kind: domain.KindKnowledge, Agent: domain.RoleKnowledge, Source: "runbooks",
		Summary: "runbook " + runbookID + " matched", RawRef: "runbook:" + runbookID,
		Confidence: 0.7,
		Facts:      map[string]string{"runbook_id": runbookID, "score": score},
	}
}

func counterEv(id, signatureID string) domain.Evidence {
	return domain.Evidence{
		ID: id, Kind: domain.KindMetric, Agent: domain.RoleMetrics, Source: "metrics",
		Summary: "an equal load was served without error two days earlier",
		RawRef:  "promql:http_requests_total[offset 2d]", Confidence: 0.88,
		Facts: map[string]string{
			"counters": signatureID,
			"demand":   "historical peak traffic comparison at equal load",
		},
	}
}

// snapshot assembles a snapshot from evidence, ready for the reasoner.
func snapshot(ev ...domain.Evidence) domain.Snapshot {
	index := map[string]domain.Evidence{}
	for _, e := range ev {
		index[e.ID] = e
	}
	return domain.Snapshot{
		CaseID: "inc-test",
		Alert: domain.Alert{
			Name: "OrderApiHighErrorRate", Service: "order-api",
			Severity: "P1", StartsAt: caseStart,
		},
		Mode:      domain.ModeMultiWithCritic,
		Window:    domain.TimeWindow{Start: caseStart.Add(-7 * time.Minute), End: caseStart.Add(20 * time.Minute)},
		Round:     1,
		MaxRounds: 3,
		Evidence:  ev,
		Index:     index,
	}
}

// round1Evidence is the evidence the reference scenario has after its first,
// default-query round: no pool metric and no change.
func round1Evidence() []domain.Evidence {
	return []domain.Evidence{
		metric("e-metrics-001", "http_5xx_rate", true, false, 0),
		metric("e-metrics-002", "http_request_duration_p99", true, false, 0),
		metric("e-metrics-003", "http_requests_total", true, false, -4),
		logEv("e-logs-001", "db connection timeout after #ms", 184),
		topologyEv("e-topology-001", "order-api", "checkout-web", ""),
		knowledgeEv("e-knowledge-001", "rb-db-pool", "0.4000"),
		knowledgeEv("e-knowledge-002", "rb-capacity", "0.4000"),
	}
}

// round2Evidence adds what the critic's demands produced.
func round2Evidence() []domain.Evidence {
	out := round1Evidence()
	return append(out,
		metric("e-metrics-010", "db_pool_saturation", true, true, -1),
		changeEv("e-change-002", "DB_POOL_SIZE", "20", "2", -2),
		counterEv("e-metrics-011", "sig-traffic-surge"),
	)
}

func testCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	cat, err := catalog.Load()
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	return cat
}

func newReasoner(t *testing.T) *RuleReasoner {
	t.Helper()
	return NewRuleReasoner(testCatalog(t), DefaultConfig())
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// findHypothesis returns the hypothesis for a signature, or fails the test.
func findHypothesis(t *testing.T, hs []domain.Hypothesis, signatureID string) domain.Hypothesis {
	t.Helper()
	for _, h := range hs {
		if h.SignatureID == signatureID {
			return h
		}
	}
	t.Fatalf("no hypothesis for signature %q in %v", signatureID, signatureIDs(hs))
	return domain.Hypothesis{}
}

func signatureIDs(hs []domain.Hypothesis) []string {
	out := make([]string, len(hs))
	for i, h := range hs {
		out[i] = h.SignatureID
	}
	return out
}

func hasCategory(cs []domain.Critique, category string) bool {
	for _, c := range cs {
		if c.Category == category {
			return true
		}
	}
	return false
}

func hasDescriptor(ds []domain.EvidenceDemand, descriptor string) bool {
	for _, d := range ds {
		if d.Descriptor == descriptor {
			return true
		}
	}
	return false
}
