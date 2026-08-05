package reasoner

import (
	"context"
	"strings"
	"testing"

	"github.com/zlrrr/mutil-agent-system/internal/catalog"
	"github.com/zlrrr/mutil-agent-system/internal/domain"
)

// critiqued runs the ensemble over a snapshot whose hypotheses have been formed and
// identified, mirroring what the orchestrator hands the critic.
func critiqued(t *testing.T, ev []domain.Evidence) (domain.Snapshot, []domain.Critique, []domain.EvidenceDemand) {
	t.Helper()
	r := newReasoner(t)
	ctx := context.Background()

	s := snapshot(ev...)
	hs, err := r.Hypothesise(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	for i := range hs {
		hs[i].ID = "h-analysis-00" + itoa(i+1)
	}
	s.Hypotheses = hs

	cs, ds, err := r.Critique(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	return s, cs, ds
}

// sdd:verify TC-0030
func TestCritiqueProducesVerdicts(t *testing.T) {
	s, cs, _ := critiqued(t, round1Evidence())

	if len(cs) == 0 {
		t.Fatal("the critic produced no critique for the round-1 hypothesis set")
	}
	byHypothesis := map[string]bool{}
	for _, c := range cs {
		if !c.Verdict.Valid() {
			t.Errorf("critique %q carries the invalid verdict %q", c.Category, c.Verdict)
		}
		if strings.TrimSpace(c.Challenge) == "" {
			t.Errorf("critique %q carries no challenge text", c.Category)
		}
		if c.Rule == "" {
			t.Errorf("critique %q is not attributed to a rule", c.Category)
		}
		byHypothesis[c.HypothesisID] = true
	}
	for _, h := range s.Hypotheses {
		if !byHypothesis[h.ID] {
			t.Errorf("%s (%s) received no verdict", h.ID, h.SignatureID)
		}
	}
}

// sdd:verify TC-0030
func TestCriticNeverProposesAHypothesis(t *testing.T) {
	// The capability table is the enforcement; this asserts the role's declaration
	// matches the architecture's claim that the critic may only challenge.
	for _, kind := range []domain.ContributionKind{
		domain.ContribProposeHypothesis, domain.ContribProposeAction,
		domain.ContribAddEvidence, domain.ContribRecordExecution,
	} {
		if domain.RoleCritic.MayEmit(kind) {
			t.Errorf("the critic must not be able to emit %s", kind)
		}
	}
	if !domain.RoleCritic.MayEmit(domain.ContribRaiseCritique) ||
		!domain.RoleCritic.MayEmit(domain.ContribDemandEvidence) {
		t.Error("the critic must be able to challenge and to demand evidence")
	}
}

// sdd:verify TC-0032
func TestAlternativeExplanationRule(t *testing.T) {
	s, cs, ds := critiqued(t, round1Evidence())

	lead := s.Hypotheses[0]
	if lead.SignatureID != "sig-traffic-surge" {
		t.Fatalf("this test assumes the traffic explanation leads round 1, got %s", lead.SignatureID)
	}

	var found bool
	for _, c := range cs {
		if c.Category == "alternative_explanation" && c.HypothesisID == lead.ID {
			found = true
			if strings.TrimSpace(c.Challenge) == "" {
				t.Error("the alternative must be named in the challenge")
			}
		}
	}
	if !found {
		t.Errorf("no alternative explanation was raised against the leader; categories seen: %v",
			categories(cs))
	}
	if !hasDescriptor(ds, "database connection pool saturation metric") {
		t.Errorf("the discriminating evidence was not demanded; demands: %v", descriptors(ds))
	}
}

// sdd:verify TC-0032
func TestAlternativeIsNotRaisedOnceItIsRuledOut(t *testing.T) {
	// Round 2 has answered every discriminator, so the rule must fall silent rather
	// than repeat itself.
	_, cs, _ := critiqued(t, round2Evidence())
	for _, c := range cs {
		if c.Category == "alternative_explanation" &&
			strings.Contains(c.Challenge, "connection pool") {
			t.Error("an alternative that the evidence already excludes was raised again")
		}
	}
}

// sdd:verify TC-0033
func TestTemporalOrderRule(t *testing.T) {
	// The change is recorded four minutes *after* the pool saturated.
	ev := append(round1Evidence(),
		metric("e-metrics-010", "db_pool_saturation", true, true, -1),
		changeEv("e-change-002", "DB_POOL_SIZE", "20", "2", 3),
	)
	_, cs, _ := critiqued(t, ev)

	var found bool
	for _, c := range cs {
		if c.Category != "temporal_order" {
			continue
		}
		found = true
		if c.Verdict != domain.VerdictReject {
			t.Errorf("a change after the symptom must be rejected, got %q", c.Verdict)
		}
		if len(c.CounterIDs) == 0 {
			t.Error("the temporal conflict must attach counter-evidence")
		}
		if !strings.Contains(c.Challenge, "after") {
			t.Errorf("the challenge should state the ordering: %q", c.Challenge)
		}
	}
	if !found {
		t.Errorf("no temporal-order critique was raised; categories: %v", categories(cs))
	}

	// The same evidence with the change *before* the symptom must not trigger it.
	ok := append(round1Evidence(),
		metric("e-metrics-010", "db_pool_saturation", true, true, -1),
		changeEv("e-change-002", "DB_POOL_SIZE", "20", "2", -2),
	)
	_, cs2, _ := critiqued(t, ok)
	if hasCategory(cs2, "temporal_order") {
		t.Error("a change preceding the symptom must not raise a temporal conflict")
	}
}

// sdd:verify TC-0034
func TestSourceVsVictimRule(t *testing.T) {
	ev := round1Evidence()
	for i := range ev {
		if ev[i].Kind == domain.KindTopology {
			ev[i] = topologyEv("e-topology-001", "postgres", "order-api", "postgres")
		}
	}
	_, cs, _ := critiqued(t, ev)

	var found bool
	for _, c := range cs {
		if c.Category == "source_vs_victim" {
			found = true
			if !strings.Contains(c.Challenge, "postgres") {
				t.Errorf("the challenge must name the upstream candidate: %q", c.Challenge)
			}
			if c.Verdict.Permits() {
				t.Errorf("a source-versus-victim challenge must not permit progression, got %q", c.Verdict)
			}
		}
	}
	if !found {
		t.Errorf("no source-versus-victim critique was raised; categories: %v", categories(cs))
	}

	// Without an earlier-anomalous upstream the rule stays silent.
	_, clean, _ := critiqued(t, round1Evidence())
	if hasCategory(clean, "source_vs_victim") {
		t.Error("the rule fired without an earlier-anomalous upstream")
	}
}

// sdd:verify TC-0030
func TestCoverageGapRuleDemandsExactlyWhatIsMissing(t *testing.T) {
	s, cs, ds := critiqued(t, round1Evidence())

	pool := findHypothesis(t, s.Hypotheses, "sig-db-pool-exhaustion")
	var found bool
	for _, c := range cs {
		if c.Category == "coverage_gap" && c.HypothesisID == pool.ID {
			found = true
			if c.Verdict != domain.VerdictRevise {
				t.Errorf("a coverage gap should ask for a revision, got %q", c.Verdict)
			}
			if len(c.DemandIDs) == 0 {
				t.Error("a coverage gap must reference the demand it raised")
			}
		}
	}
	if !found {
		t.Errorf("no coverage gap was reported for the pool explanation; categories: %v",
			categories(cs))
	}
	for _, want := range []string{
		"database connection pool saturation metric",
		"configuration changes for the service in the 30 minutes before onset",
	} {
		if !hasDescriptor(ds, want) {
			t.Errorf("the critic did not demand %q; demands: %v", want, descriptors(ds))
		}
	}
}

// sdd:verify TC-0036
func TestCloseCallRule(t *testing.T) {
	_, cs, _ := critiqued(t, round1Evidence())
	if !hasCategory(cs, "close_call") {
		t.Errorf("a 0.0x gap between the top two must raise a close call; categories: %v",
			categories(cs))
	}
	for _, c := range cs {
		if c.Category == "close_call" && c.Verdict.Permits() {
			t.Errorf("a close call must not permit progression, got %q", c.Verdict)
		}
	}

	// Round 2 separates them decisively, so the rule must fall silent.
	_, cs2, _ := critiqued(t, round2Evidence())
	if hasCategory(cs2, "close_call") {
		t.Error("a decisive gap must not raise a close call")
	}
}

// sdd:verify TC-0030
func TestUnverifiableRemediationRule(t *testing.T) {
	_, cs, _ := critiqued(t, round1Evidence())
	// sig-db-outage declares no remediation, so it can be believed but not confirmed.
	var found bool
	for _, c := range cs {
		if c.Category == "unverifiable_remediation" {
			found = true
			if c.Verdict != domain.VerdictAcceptWithRisk {
				t.Errorf("an unverifiable explanation should be accepted with risk, got %q", c.Verdict)
			}
		}
	}
	if !found {
		t.Errorf("the unverifiable-remediation rule never fired; categories: %v", categories(cs))
	}
}

// sdd:verify TC-0031
func TestDemandsAreCappedAndDeduplicated(t *testing.T) {
	cfg := DefaultConfig()
	_, _, ds := critiqued(t, round1Evidence())

	if len(ds) > cfg.MaxDemandsPerRound {
		t.Errorf("%d demands were issued, cap is %d", len(ds), cfg.MaxDemandsPerRound)
	}
	seen := map[string]bool{}
	for _, d := range ds {
		if seen[d.Descriptor] {
			t.Errorf("descriptor %q was demanded twice", d.Descriptor)
		}
		seen[d.Descriptor] = true
		if err := d.Validate(); err != nil {
			t.Errorf("demand %q is invalid: %v", d.Descriptor, err)
		}
		if d.Rule == "" {
			t.Errorf("demand %q is not attributed to a rule", d.Descriptor)
		}
	}
}

// sdd:verify TC-0030
func TestUnchallengedHypothesisGetsAnExplicitAccept(t *testing.T) {
	_, cs, _ := critiqued(t, round2Evidence())
	var accepted bool
	for _, c := range cs {
		if c.Rule == "no_objection" && c.Verdict == domain.VerdictAccept {
			accepted = true
		}
	}
	if !accepted {
		t.Errorf("an unchallenged hypothesis must receive an explicit accept; categories: %v",
			categories(cs))
	}
}

// sdd:verify TC-0020
func TestSignatureMatchingIsOrderIndependent(t *testing.T) {
	cat := testCatalog(t)
	sig, ok := cat.Signature("sig-db-pool-exhaustion")
	if !ok {
		t.Fatal("the reference signature is missing from the catalog")
	}

	ev := round2Evidence()
	forward := catalog.Match(sig, ev)

	reversed := make([]domain.Evidence, len(ev))
	for i := range ev {
		reversed[i] = ev[len(ev)-1-i]
	}
	backward := catalog.Match(sig, reversed)

	if len(forward.Matched) != len(backward.Matched) {
		t.Errorf("match count depends on input order: %d vs %d",
			len(forward.Matched), len(backward.Matched))
	}
	if strings.Join(forward.Supporting, ",") != strings.Join(backward.Supporting, ",") {
		t.Errorf("supporting set depends on input order:\n %v\n %v",
			forward.Supporting, backward.Supporting)
	}
	if !forward.HasChangeAt {
		t.Error("the matched change timestamp was not captured")
	}
}

func categories(cs []domain.Critique) []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range cs {
		if !seen[c.Category] {
			seen[c.Category] = true
			out = append(out, c.Category)
		}
	}
	return out
}

func descriptors(ds []domain.EvidenceDemand) []string {
	out := make([]string, len(ds))
	for i, d := range ds {
		out[i] = d.Descriptor
	}
	return out
}
