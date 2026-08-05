package agent

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/zlrrr/mutil-agent-system/internal/catalog"
	"github.com/zlrrr/mutil-agent-system/internal/domain"
	"github.com/zlrrr/mutil-agent-system/internal/reasoner"
	"github.com/zlrrr/mutil-agent-system/internal/signal"
	"github.com/zlrrr/mutil-agent-system/internal/signal/fixture"
)

func referenceCase(t *testing.T) (catalog.FaultCase, *catalog.Catalog, signal.Set, *fixture.State) {
	t.Helper()
	cat, err := catalog.Load()
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	fc, ok := cat.Case("C1")
	if !ok {
		t.Fatal("the reference case C1 is missing from the catalog")
	}
	set, state := fixture.NewSet(fc, cat, signal.DefaultBounds())
	return fc, cat, set, state
}

func baseSnapshot(fc catalog.FaultCase) domain.Snapshot {
	return domain.Snapshot{
		CaseID: "inc-test", Alert: fc.Alert, Mode: domain.ModeMultiWithCritic,
		Window: fc.Window(), Round: 1, MaxRounds: 3, Status: domain.StatusCollecting,
		Index: map[string]domain.Evidence{},
	}
}

// runCollector executes one collector and returns its evidence with provisional ids.
func runCollector(t *testing.T, set signal.Set, role domain.Role, s domain.Snapshot) []domain.Evidence {
	t.Helper()
	var target Agent
	for _, a := range Collectors(set, reasoner.DefaultConfig()) {
		if a.Role() == role {
			target = a
		}
	}
	if target == nil {
		t.Fatalf("no collector for role %s", role)
	}
	contribs, err := target.Run(context.Background(), s)
	if err != nil {
		t.Fatalf("%s collector: %v", role, err)
	}
	var out []domain.Evidence
	for i, c := range contribs {
		if c.Kind != domain.ContribAddEvidence {
			t.Errorf("%s emitted %s; collectors may only add evidence", role, c.Kind)
			continue
		}
		e := *c.Evidence
		e.ID = domain.NewID(domain.PrefixEvidence, role, i+1)
		if err := e.Validate(); err != nil {
			t.Errorf("%s produced invalid evidence: %v", role, err)
		}
		out = append(out, e)
	}
	return out
}

// sdd:verify TC-0001
func TestAgentsArePure(t *testing.T) {
	fc, cat, set, _ := referenceCase(t)
	cfg := reasoner.DefaultConfig()
	rsn := reasoner.NewRuleReasoner(cat, cfg)

	agents := append(Collectors(set, cfg),
		Analysis(rsn), Critic(rsn), Remediation(cat, cfg), Verification(set, cfg),
		SingleAgentBaseline(set, rsn, cfg, cat))

	for _, a := range agents {
		t.Run(string(a.Role()), func(t *testing.T) {
			s := baseSnapshot(fc)
			s.Evidence = []domain.Evidence{{
				ID: "e-metrics-001", Kind: domain.KindMetric, Agent: domain.RoleMetrics,
				Source: "metrics", Summary: "seed", RawRef: "promql:seed", Confidence: 0.5,
				Facts: map[string]string{"series": "seed"},
			}}
			s.Index = map[string]domain.Evidence{s.Evidence[0].ID: s.Evidence[0]}
			before := deepCopySnapshot(s)

			contribs, err := a.Run(context.Background(), s)
			if err != nil {
				t.Fatalf("%s: %v", a.Role(), err)
			}

			if !reflect.DeepEqual(before, deepCopySnapshot(s)) {
				t.Errorf("%s mutated the snapshot it was given", a.Role())
			}
			for _, c := range contribs {
				if !a.Role().MayEmit(c.Kind) {
					t.Errorf("%s emitted %s, which its role does not permit", a.Role(), c.Kind)
				}
			}
		})
	}
}

func deepCopySnapshot(s domain.Snapshot) domain.Snapshot {
	out := s
	out.Evidence = append([]domain.Evidence(nil), s.Evidence...)
	out.Hypotheses = append([]domain.Hypothesis(nil), s.Hypotheses...)
	out.Critiques = append([]domain.Critique(nil), s.Critiques...)
	out.Demands = append([]domain.EvidenceDemand(nil), s.Demands...)
	out.Actions = append([]domain.Action(nil), s.Actions...)
	out.Index = nil
	return out
}

// sdd:verify TC-0011
func TestMetricsCollectorReportsOnsetWithoutCausation(t *testing.T) {
	fc, _, set, _ := referenceCase(t)
	ev := runCollector(t, set, domain.RoleMetrics, baseSnapshot(fc))

	if len(ev) == 0 {
		t.Fatal("the metrics collector produced no evidence")
	}
	var found bool
	for _, e := range ev {
		if e.Fact("series") != "http_5xx_rate" {
			continue
		}
		found = true
		if e.Fact("anomalous") != "true" {
			t.Errorf("the error rate should be anomalous: %v", e.Facts)
		}
		onset, err := time.Parse(time.RFC3339, e.Fact("onset"))
		if err != nil {
			t.Fatalf("onset is not a timestamp: %q", e.Fact("onset"))
		}
		if diff := onset.Sub(fc.Alert.StartsAt); diff < -time.Minute || diff > time.Minute {
			t.Errorf("onset %s is not within a sample of the alert start %s", onset, fc.Alert.StartsAt)
		}
		if !strings.HasPrefix(e.RawRef, "promql:") {
			t.Errorf("evidence must name the query that produced it: %q", e.RawRef)
		}
	}
	if !found {
		t.Error("the default pass did not query the error rate")
	}

	// The default pass must not reach the pool metric: that is the gap the critic
	// exists to notice.
	for _, e := range ev {
		if e.Fact("series") == "db_pool_saturation" {
			t.Error("the default pass reached the pool metric; round one should be incomplete")
		}
	}
}

// sdd:verify TC-0012
func TestLogCollectorClustersAndBounds(t *testing.T) {
	fc, _, set, _ := referenceCase(t)
	ev := runCollector(t, set, domain.RoleLogs, baseSnapshot(fc))

	if len(ev) == 0 {
		t.Fatal("the log collector produced no evidence")
	}
	first := ev[0]
	if !strings.Contains(first.Fact("template"), "connection timeout") {
		t.Errorf("the largest cluster should be the timeout template, got %q", first.Fact("template"))
	}
	if first.Fact("count") == "" {
		t.Error("a cluster must report how many lines it represents")
	}
	if len(strings.Split(first.Fact("samples"), " | ")) > signal.LogSamplesPerCluster {
		t.Errorf("a cluster kept more than %d samples", signal.LogSamplesPerCluster)
	}
	if len(ev) > signal.MaxClusters {
		t.Errorf("the collector returned %d clusters, cap is %d", len(ev), signal.MaxClusters)
	}
}

// sdd:verify TC-0013
func TestChangeCollector(t *testing.T) {
	fc, _, set, state := referenceCase(t)

	// The default pass looks only from the alert onward, so it finds nothing.
	ev := runCollector(t, set, domain.RoleChange, baseSnapshot(fc))
	for _, e := range ev {
		if e.Fact("change_key") == "DB_POOL_SIZE" {
			t.Error("the default pass found the change; the critic's demand would be pointless")
		}
	}

	// With the demand outstanding, the extended lookback finds it.
	s := baseSnapshot(fc)
	s.Demands = []domain.EvidenceDemand{{
		ID: "d-critic-001", Kind: domain.KindChange,
		Descriptor: "configuration changes for the service in the 30 minutes before onset",
		Reason:     "the pool size may have changed",
	}}
	ev = runCollector(t, set, domain.RoleChange, s)

	var found bool
	for _, e := range ev {
		if e.Fact("change_key") != "DB_POOL_SIZE" {
			continue
		}
		found = true
		if e.Fact("change_old") != "20" || e.Fact("change_new") != "2" {
			t.Errorf("change evidence must carry both values, got %v", e.Facts)
		}
		if e.Fact("changed_at") == "" {
			t.Error("change evidence must carry its timestamp")
		}
		if e.Fact("demand") == "" {
			t.Error("demand-driven evidence must record which demand it answers")
		}
	}
	if !found {
		t.Errorf("the demanded change was not found; evidence: %+v", ev)
	}
	if n := len(state.Calls()); n != 0 {
		t.Errorf("the change collector invoked the actuator %d time(s); it must never act", n)
	}
}

// sdd:verify TC-0014
func TestTopologyCollector(t *testing.T) {
	fc, _, set, _ := referenceCase(t)
	ev := runCollector(t, set, domain.RoleTopology, baseSnapshot(fc))

	if len(ev) != 1 {
		t.Fatalf("want one topology record, got %d", len(ev))
	}
	e := ev[0]
	if !strings.Contains(e.Fact("candidate_origin"), "order-api") {
		t.Errorf("order-api should be the candidate origin, got %q", e.Fact("candidate_origin"))
	}
	if !strings.Contains(e.Fact("affected"), "checkout-web") {
		t.Errorf("checkout-web should be reported as affected, got %q", e.Fact("affected"))
	}
	if strings.Contains(e.Fact("candidate_origin"), "checkout-web") {
		t.Error("a service that became anomalous later must not be a candidate origin")
	}
	if e.Fact("upstream_anomalous") != "" {
		t.Errorf("no upstream is anomalous in this case, got %q", e.Fact("upstream_anomalous"))
	}
}

// sdd:verify TC-0015
func TestKnowledgeCollector(t *testing.T) {
	fc, _, set, _ := referenceCase(t)

	s := baseSnapshot(fc)
	// The knowledge role reads the log templates already collected.
	logs := runCollector(t, set, domain.RoleLogs, s)
	s.Evidence = logs
	s.Index = map[string]domain.Evidence{}
	for _, e := range logs {
		s.Index[e.ID] = e
	}

	ev := runCollector(t, set, domain.RoleKnowledge, s)
	if len(ev) == 0 {
		t.Fatal("no runbook matched the incident symptoms")
	}
	var found bool
	for _, e := range ev {
		if e.Fact("runbook_id") == "rb-db-pool" {
			found = true
			if e.Fact("score") == "" || e.Fact("matched") == "" {
				t.Errorf("a runbook match must record its score and matched terms: %v", e.Facts)
			}
		}
	}
	if !found {
		t.Error("the connection-pool runbook should be among the matches")
	}
}

// sdd:verify TC-0040
func TestRemediationProposal(t *testing.T) {
	fc, cat, set, _ := referenceCase(t)
	cfg := reasoner.DefaultConfig()
	rsn := reasoner.NewRuleReasoner(cat, cfg)

	// Build the round-2 evidence the same way the flow does.
	s := baseSnapshot(fc)
	s.Demands = []domain.EvidenceDemand{
		{ID: "d1", Kind: domain.KindChange,
			Descriptor: "configuration changes for the service in the 30 minutes before onset",
			Reason:     "r"},
		{ID: "d2", Kind: domain.KindMetric,
			Descriptor: "database connection pool saturation metric", Reason: "r"},
	}
	var all []domain.Evidence
	for _, role := range domain.CollectorRoles {
		s.Evidence = all
		s.Index = indexOf(all)
		all = append(all, runCollector(t, set, role, s)...)
	}
	s.Evidence = all
	s.Index = indexOf(all)

	hs, err := rsn.Hypothesise(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	for i := range hs {
		hs[i].ID = domain.NewID(domain.PrefixHypothesis, domain.RoleAnalysis, i+1)
	}
	s.Hypotheses = hs
	if hs[0].SignatureID != "sig-db-pool-exhaustion" {
		t.Fatalf("this test needs the pool explanation to lead, got %s", hs[0].SignatureID)
	}

	contribs, err := Remediation(cat, cfg).Run(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	if len(contribs) != 1 {
		t.Fatalf("want one proposed action, got %d", len(contribs))
	}
	a := *contribs[0].Action

	if a.Tool != "set_config" || a.Args["service"] != "order-api" ||
		a.Args["key"] != "DB_POOL_SIZE" || a.Args["value"] != "20" {
		t.Errorf("action = %s %v, want set_config order-api DB_POOL_SIZE 20", a.Tool, a.Args)
	}
	if a.Risk != domain.RiskMedium {
		t.Errorf("risk = %s, want medium", a.Risk)
	}
	if a.Rollback == nil || a.Rollback.Args["value"] != "2" {
		t.Errorf("rollback must restore the value the change replaced, got %+v", a.Rollback)
	}
	if len(a.Verify) == 0 || a.Verify[0] != "http_5xx_rate" {
		t.Errorf("verification signals = %v, want http_5xx_rate first", a.Verify)
	}
	if err := a.Validate(); err != nil {
		t.Errorf("the proposed action is incomplete: %v", err)
	}
}

// sdd:verify TC-0080
func TestBaselineSeesTheSameToolsAndOnlyOnePass(t *testing.T) {
	fc, cat, set, _ := referenceCase(t)
	cfg := reasoner.DefaultConfig()
	rsn := reasoner.NewRuleReasoner(cat, cfg)

	contribs, err := SingleAgentBaseline(set, rsn, cfg, cat).Run(context.Background(), baseSnapshot(fc))
	if err != nil {
		t.Fatal(err)
	}

	var evidence, hypotheses, actions int
	kinds := map[domain.EvidenceKind]bool{}
	for _, c := range contribs {
		switch c.Kind {
		case domain.ContribAddEvidence:
			evidence++
			kinds[c.Evidence.Kind] = true
		case domain.ContribProposeHypothesis:
			hypotheses++
		case domain.ContribProposeAction:
			actions++
		}
	}
	// The same tools: it reaches every collector kind the multi-agent flow does.
	for _, k := range []domain.EvidenceKind{
		domain.KindMetric, domain.KindLog, domain.KindChange,
		domain.KindTopology, domain.KindKnowledge,
	} {
		if !kinds[k] {
			t.Errorf("the baseline did not gather %s evidence; it must have the same tools", k)
		}
	}
	if evidence == 0 || hypotheses == 0 {
		t.Fatalf("the baseline produced %d evidence and %d hypotheses", evidence, hypotheses)
	}
	// One pass: it never demands evidence and never critiques.
	for _, c := range contribs {
		if c.Kind == domain.ContribDemandEvidence || c.Kind == domain.ContribRaiseCritique {
			t.Errorf("the baseline emitted %s; it models a single pass with no adversarial round", c.Kind)
		}
	}
}
