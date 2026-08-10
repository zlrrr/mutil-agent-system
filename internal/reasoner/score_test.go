package reasoner

import (
	"context"
	"math"
	"math/rand"
	"testing"

	"github.com/zlrrr/mutil-agent-system/internal/catalog"
	"github.com/zlrrr/mutil-agent-system/internal/domain"
)

// sdd:verify TC-0022
func TestWeightsSumToOne(t *testing.T) {
	w := DefaultWeights()
	if diff := math.Abs(w.Sum() - 1.0); diff > 1e-9 {
		t.Errorf("weights sum to %v, want 1.0 (off by %v)", w.Sum(), diff)
	}
	if w.Metric != 0.30 || w.Log != 0.25 || w.Change != 0.20 ||
		w.Topology != 0.10 || w.Historical != 0.10 || w.Verifiable != 0.05 {
		t.Errorf("weights drifted from the values the detailed design declares: %+v", w)
	}
}

// sdd:verify TC-0022
func TestScoreBreakdown(t *testing.T) {
	in := Inputs{Metric: 1.0, Log: 1.0, Change: 1.0, Topology: 1.0, Historical: 0.4, Verifiable: 1.0}
	b := Score(in, DefaultWeights(), 0.20)

	if len(b.Terms) != 6 {
		t.Fatalf("breakdown has %d terms, want 6", len(b.Terms))
	}
	if diff := math.Abs(b.Sum() - b.Total); diff > 1e-9 {
		t.Errorf("terms minus penalty = %v but total = %v (off by %v)", b.Sum(), b.Total, diff)
	}
	for _, name := range []string{TermMetric, TermLog, TermChange, TermTopology,
		TermHistorical, TermVerifiable} {
		term, ok := b.Term(name)
		if !ok {
			t.Errorf("breakdown is missing the term %q", name)
			continue
		}
		if diff := math.Abs(term.Contribution - term.Weight*term.Value); diff > 1e-4 {
			t.Errorf("%s: contribution %v != weight %v * value %v",
				name, term.Contribution, term.Weight, term.Value)
		}
	}

	// The total is clamped to [0,1] so a report never shows an impossible confidence.
	over := Score(Inputs{Metric: 2, Log: 2, Change: 2, Topology: 2, Historical: 2, Verifiable: 2},
		DefaultWeights(), 0.20)
	if over.Total > 1.0 {
		t.Errorf("total = %v, must be clamped to 1.0", over.Total)
	}
}

// sdd:verify TC-0023
func TestCounterEvidencePenalty(t *testing.T) {
	in := Inputs{Metric: 1.0, Topology: 1.0, Verifiable: 1.0}
	w := DefaultWeights()

	clean := Score(in, w, 0.20)

	penalised := in
	penalised.Unresolved = 1
	withCounter := Score(penalised, w, 0.20)

	if diff := math.Abs((clean.Total - withCounter.Total) - 0.20); diff > 1e-9 {
		t.Errorf("one unresolved counter changed the total by %v, want exactly 0.20",
			clean.Total-withCounter.Total)
	}
	if withCounter.Penalty != 0.20 || withCounter.Unresolved != 1 {
		t.Errorf("the penalty must be visible in the breakdown: %+v", withCounter)
	}

	// Two counters cost twice as much; a resolved counter costs nothing.
	penalised.Unresolved = 2
	if got := Score(penalised, w, 0.20).Penalty; math.Abs(got-0.40) > 1e-9 {
		t.Errorf("penalty for two counters = %v, want 0.40", got)
	}
	resolved := in
	resolved.Unresolved = 0
	if Score(resolved, w, 0.20).Total != clean.Total {
		t.Error("a resolved counter must stop contributing to the penalty")
	}
}

// sdd:verify TC-0024
func TestRankIsTotalOrder(t *testing.T) {
	index := map[string]domain.Evidence{
		"e1": {ID: "e1", Kind: domain.KindMetric},
		"e2": {ID: "e2", Kind: domain.KindLog},
	}
	// Equal score and equal kind count: the identifier decides, every time.
	hs := []domain.Hypothesis{
		{ID: "h-analysis-003", Supporting: []string{"e1"}, Breakdown: domain.ScoreBreakdown{Total: 0.5}},
		{ID: "h-analysis-001", Supporting: []string{"e2"}, Breakdown: domain.ScoreBreakdown{Total: 0.5}},
		{ID: "h-analysis-002", Supporting: []string{"e1"}, Breakdown: domain.ScoreBreakdown{Total: 0.5}},
	}

	want := []string{"h-analysis-001", "h-analysis-002", "h-analysis-003"}
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 100; i++ {
		shuffled := append([]domain.Hypothesis(nil), hs...)
		rng.Shuffle(len(shuffled), func(a, b int) {
			shuffled[a], shuffled[b] = shuffled[b], shuffled[a]
		})
		got := Rank(shuffled, index)
		for j, h := range got {
			if h.ID != want[j] {
				t.Fatalf("iteration %d: order = %v, want %v", i, idsOf(got), want)
			}
		}
	}

	// Score dominates, then the count of supporting evidence kinds.
	scored := []domain.Hypothesis{
		{ID: "h-a", Supporting: []string{"e1"}, Breakdown: domain.ScoreBreakdown{Total: 0.4}},
		{ID: "h-b", Supporting: []string{"e1", "e2"}, Breakdown: domain.ScoreBreakdown{Total: 0.9}},
		{ID: "h-c", Supporting: []string{"e1", "e2"}, Breakdown: domain.ScoreBreakdown{Total: 0.4}},
	}
	got := Rank(scored, index)
	if got[0].ID != "h-b" {
		t.Errorf("highest score must lead, got %v", idsOf(got))
	}
	if got[1].ID != "h-c" {
		t.Errorf("on equal scores the broader evidence base leads, got %v", idsOf(got))
	}
}

func idsOf(hs []domain.Hypothesis) []string {
	out := make([]string, len(hs))
	for i, h := range hs {
		out[i] = h.ID
	}
	return out
}

// sdd:verify TC-0020
func TestHypothesise(t *testing.T) {
	r := newReasoner(t)
	hs, err := r.Hypothesise(context.Background(), snapshot(round2Evidence()...))
	if err != nil {
		t.Fatal(err)
	}

	if len(hs) == 0 {
		t.Fatal("the reference evidence must produce hypotheses")
	}
	if len(hs) > DefaultConfig().MaxHypothesesPerRound {
		t.Errorf("returned %d hypotheses, cap is %d", len(hs), DefaultConfig().MaxHypothesesPerRound)
	}
	for _, h := range hs {
		if h.Mechanism == "" {
			t.Errorf("%s has no mechanism narrative", h.SignatureID)
		}
		if len(h.Supporting) == 0 {
			t.Errorf("%s references no evidence", h.SignatureID)
		}
		if err := h.Validate(); err != nil {
			t.Errorf("%s is invalid: %v", h.SignatureID, err)
		}
	}
	if hs[0].SignatureID != "sig-db-pool-exhaustion" {
		t.Errorf("leading hypothesis = %s, want sig-db-pool-exhaustion (order %v)",
			hs[0].SignatureID, signatureIDs(hs))
	}
	if hs[0].Breakdown.Total < DefaultConfig().AcceptThreshold {
		t.Errorf("the complete evidence set should clear the acceptance threshold, got %.2f",
			hs[0].Breakdown.Total)
	}
}

// sdd:verify TC-0020
func TestHypothesiseRanksTrafficFirstBeforeTheCriticActs(t *testing.T) {
	r := newReasoner(t)
	hs, err := r.Hypothesise(context.Background(), snapshot(round1Evidence()...))
	if err != nil {
		t.Fatal(err)
	}
	if len(hs) < 2 {
		t.Fatalf("want at least two hypotheses, got %v", signatureIDs(hs))
	}
	// This is the situation the adversarial flow exists for: the first coherent story
	// leads, and it is the wrong one.
	if hs[0].SignatureID != "sig-traffic-surge" {
		t.Errorf("round-1 leader = %s, want sig-traffic-surge (order %v)",
			hs[0].SignatureID, signatureIDs(hs))
	}
	gap := hs[0].Breakdown.Total - hs[1].Breakdown.Total
	if gap >= DefaultConfig().CloseCallMargin {
		t.Errorf("round-1 gap = %.3f, which should be inside the %.2f close-call margin",
			gap, DefaultConfig().CloseCallMargin)
	}
	if hs[0].Breakdown.Total >= DefaultConfig().AcceptThreshold {
		t.Errorf("round-1 leader scores %.2f, which should be below the %.2f threshold",
			hs[0].Breakdown.Total, DefaultConfig().AcceptThreshold)
	}
}

// sdd:verify TC-0072
func TestHypothesisIdentityStable(t *testing.T) {
	r := newReasoner(t)
	ctx := context.Background()

	first, err := r.Hypothesise(ctx, snapshot(round1Evidence()...))
	if err != nil {
		t.Fatal(err)
	}
	// Simulate the orchestrator having assigned identifiers, then a second round.
	for i := range first {
		first[i].ID = "h-analysis-00" + itoa(i+1)
	}
	s2 := snapshot(round2Evidence()...)
	s2.Hypotheses = first
	s2.Round = 2

	second, err := r.Hypothesise(ctx, s2)
	if err != nil {
		t.Fatal(err)
	}

	idBySignature := map[string]string{}
	for _, h := range first {
		idBySignature[h.SignatureID] = h.ID
	}
	for _, h := range second {
		if want, ok := idBySignature[h.SignatureID]; ok && h.ID != want {
			t.Errorf("%s changed identity across rounds: %s -> %s", h.SignatureID, want, h.ID)
		}
	}
	// The score must update rather than duplicate.
	before := findHypothesis(t, first, "sig-db-pool-exhaustion")
	after := findHypothesis(t, second, "sig-db-pool-exhaustion")
	if after.Breakdown.Total <= before.Breakdown.Total {
		t.Errorf("the new evidence should raise the score: %.2f -> %.2f",
			before.Breakdown.Total, after.Breakdown.Total)
	}
	// And the traffic explanation must fall, because counter-evidence arrived.
	if tr := findHypothesis(t, second, "sig-traffic-surge"); tr.UnresolvedCounters() == 0 {
		t.Error("the traffic explanation should carry counter-evidence after round 2")
	}
}

// sdd:verify TC-0118
func TestScoreDoesNotChargeUnclaimedTerms(t *testing.T) {
	cat, err := catalog.Load()
	if err != nil {
		t.Fatal(err)
	}
	w := DefaultWeights()

	// sig-traffic-surge declares one evidence kind; sig-db-pool-exhaustion declares three.
	// Both are scored with every term they claim fully satisfied, so the only thing that
	// separates them is how much they claimed.
	narrow, ok := cat.Signature("sig-traffic-surge")
	if !ok {
		t.Fatal("the catalog lost sig-traffic-surge")
	}
	broad, ok := cat.Signature("sig-db-pool-exhaustion")
	if !ok {
		t.Fatal("the catalog lost sig-db-pool-exhaustion")
	}

	full := func(sig catalog.Signature) domain.ScoreBreakdown {
		in := Inputs{
			Topology: 1, Historical: 1, Verifiable: 1,
			Applicable: declaredKinds(sig),
		}
		if in.Applicable.Metric {
			in.Metric = 1
		}
		if in.Applicable.Log {
			in.Log = 1
		}
		if in.Applicable.Change {
			in.Change = 1
		}
		return Score(in, w, DefaultConfig().CounterPenalty)
	}

	n, b := full(narrow), full(broad)

	t.Run("the applicable share excludes kinds the signature never declared", func(t *testing.T) {
		// One metric requirement, no log or change: those two weights are not at stake.
		want := w.Metric + w.Topology + w.Historical + w.Verifiable
		if n.Applicable != round4(want) {
			t.Errorf("applicable = %.2f, want %.2f", n.Applicable, want)
		}
		if b.Applicable != 1.0 {
			t.Errorf("a signature declaring all three kinds has applicable %.2f, want 1.00",
				b.Applicable)
		}
	})

	t.Run("a fully satisfied signature fits completely, whatever it claimed", func(t *testing.T) {
		if n.Fit != 1.0 {
			t.Errorf("the one-requirement signature fits %.4f with every declared term "+
				"satisfied; it is being charged for evidence it never claimed", n.Fit)
		}
		if b.Fit != 1.0 {
			t.Errorf("the three-requirement signature fits %.4f", b.Fit)
		}
	})

	t.Run("the total still records how much was claimed", func(t *testing.T) {
		if n.Total >= b.Total {
			t.Errorf("the one-requirement signature totals %.2f against the "+
				"three-requirement signature's %.2f; ranking must still prefer the "+
				"explanation that committed more and proved it", n.Total, b.Total)
		}
		if n.Total > 0.6 {
			t.Errorf("total = %.2f; it is meant to stay near the applicable share", n.Total)
		}
	})

	t.Run("fit is never below total", func(t *testing.T) {
		for _, got := range []domain.ScoreBreakdown{n, b} {
			if got.Fit < got.Total {
				t.Errorf("fit %.4f is below total %.4f", got.Fit, got.Total)
			}
		}
		if b.Fit != b.Total {
			t.Errorf("a signature claiming every kind has fit %.4f and total %.4f; "+
				"with nothing excluded the two must agree", b.Fit, b.Total)
		}
	})

	t.Run("applicable is read from the declaration, not from the match", func(t *testing.T) {
		// A signature whose requirements all failed still claimed them. Reading
		// applicability from the match would make it *more* applicable the more of its
		// own claims it failed, which is the arithmetic rewarding failure.
		none := Score(Inputs{Applicable: declaredKinds(broad)}, w, DefaultConfig().CounterPenalty)
		if none.Applicable != b.Applicable {
			t.Errorf("a signature that matched nothing has applicable %.2f, but declared "+
				"the same requirements as one with %.2f", none.Applicable, b.Applicable)
		}
		if none.Fit != 0 {
			t.Errorf("a signature matching nothing fits %.4f, want 0", none.Fit)
		}
	})
}
