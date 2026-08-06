package reasoner

import (
	"math"
	"sort"
	"strconv"
	"time"

	"github.com/zlrrr/mutil-agent-system/internal/catalog"
	"github.com/zlrrr/mutil-agent-system/internal/domain"
)

// sdd:impl DLD-1032

// Term names, used in the stored breakdown and rendered in the console and report.
const (
	TermMetric     = "metric_alignment"
	TermLog        = "log_alignment"
	TermChange     = "change_correlation"
	TermTopology   = "topology_plausibility"
	TermHistorical = "historical_similarity"
	TermVerifiable = "remediation_verifiability"
)

// Weights are the declared term weights. They sum to 1.0, asserted by TC-0022.
type Weights struct {
	Metric     float64
	Log        float64
	Change     float64
	Topology   float64
	Historical float64
	Verifiable float64
}

// DefaultWeights returns the weights declared in DLD section 2.
func DefaultWeights() Weights {
	return Weights{
		Metric:     0.30,
		Log:        0.25,
		Change:     0.20,
		Topology:   0.10,
		Historical: 0.10,
		Verifiable: 0.05,
	}
}

// Sum returns the total of the declared weights.
func (w Weights) Sum() float64 {
	return w.Metric + w.Log + w.Change + w.Topology + w.Historical + w.Verifiable
}

// Inputs are the per-signature term values, computed once and scored purely.
type Inputs struct {
	Metric     float64
	Log        float64
	Change     float64
	Topology   float64
	Historical float64
	Verifiable float64
	Unresolved int
}

// Score computes the breakdown from term values. It is a pure function: given the same
// inputs it always produces the same number, and the number always decomposes.
func Score(in Inputs, w Weights, penalty float64) domain.ScoreBreakdown {
	terms := []domain.ScoreTerm{
		{Name: TermMetric, Weight: w.Metric, Value: in.Metric},
		{Name: TermLog, Weight: w.Log, Value: in.Log},
		{Name: TermChange, Weight: w.Change, Value: in.Change},
		{Name: TermTopology, Weight: w.Topology, Value: in.Topology},
		{Name: TermHistorical, Weight: w.Historical, Value: in.Historical},
		{Name: TermVerifiable, Weight: w.Verifiable, Value: in.Verifiable},
	}
	var total float64
	for i := range terms {
		terms[i].Contribution = round4(terms[i].Weight * terms[i].Value)
		total += terms[i].Contribution
	}
	p := round4(penalty * float64(in.Unresolved))
	b := domain.ScoreBreakdown{
		Terms:      terms,
		Penalty:    p,
		Unresolved: in.Unresolved,
		Total:      round4(clamp(total - p)),
	}
	return b
}

func clamp(v float64) float64 { return math.Min(1, math.Max(0, v)) }

// round4 keeps stored values free of floating-point noise so that a breakdown printed
// in a report sums exactly to the total a reader can check by hand.
func round4(v float64) float64 { return math.Round(v*10000) / 10000 }

// Rank orders hypotheses by score descending, then by supporting-kind count
// descending, then by identifier ascending — a total order, so equal inputs rank
// equally in every run and every process (REQ-0024).
func Rank(hs []domain.Hypothesis, index map[string]domain.Evidence) []domain.Hypothesis {
	out := append([]domain.Hypothesis(nil), hs...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Breakdown.Total != b.Breakdown.Total {
			return a.Breakdown.Total > b.Breakdown.Total
		}
		ka, kb := len(a.SupportingKinds(index)), len(b.SupportingKinds(index))
		if ka != kb {
			return ka > kb
		}
		return a.ID < b.ID
	})
	return out
}

// termInputs derives the six term values for one signature match against a snapshot.
func termInputs(m catalog.MatchResult, s domain.Snapshot, sup *catalog.MatchResult) Inputs {
	in := Inputs{
		Metric: m.Fraction(domain.KindMetric),
		Log:    m.Fraction(domain.KindLog),
	}

	// change_correlation: 1.0 when a matched change precedes the onset of the symptom
	// this explanation accounts for, 0.5 when a change matched but no onset is known,
	// 0.0 otherwise.
	//
	// The comparison is against this hypothesis's own supporting metrics, not the
	// earliest anomaly anywhere in the case: a change that preceded the symptom it is
	// blamed for is corroborated even when some unrelated series moved earlier.
	if m.Fraction(domain.KindChange) > 0 {
		onset, ok := HypothesisOnset(m, s)
		switch {
		case !ok || !m.HasChangeAt:
			in.Change = 0.5
		case m.ChangeAt.Before(onset) || m.ChangeAt.Equal(onset):
			in.Change = 1.0
		default:
			in.Change = 0.0
		}
	}

	// topology_plausibility: candidate origin 1.0, affected only 0.4, contradicted 0.0.
	in.Topology, _ = topologyValue(s, sup)

	// historical_similarity: the best matching runbook this signature declares.
	in.Historical = runbookValue(s, m.Signature, sup)

	// remediation_verifiability: a remediation that declares how it would be checked.
	if r := m.Signature.Remediation; r != nil && len(r.Verify) > 0 {
		in.Verifiable = 1.0
	}
	return in
}

// HypothesisOnset returns the earliest onset among the metric evidence this signature
// actually matched, falling back to the alert's start time when the signature requires
// no metric of its own.
//
// Using the hypothesis's own metrics — rather than the earliest anomaly in the case —
// is what lets a genuine cause be corroborated in the presence of an unrelated series
// that moved first, which is precisely the situation the reference scenario contains.
func HypothesisOnset(m catalog.MatchResult, s domain.Snapshot) (time.Time, bool) {
	matched := map[string]bool{}
	for _, pm := range m.Matched {
		if pm.Kind == domain.KindMetric {
			matched[pm.EvidenceID] = true
		}
	}

	var best time.Time
	found := false
	for _, e := range s.EvidenceOfKind(domain.KindMetric) {
		if len(matched) > 0 && !matched[e.ID] {
			continue
		}
		ts := e.Fact("onset")
		if ts == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			continue
		}
		if !found || t.Before(best) {
			best, found = t, true
		}
	}
	if !found && !s.Alert.StartsAt.IsZero() {
		return s.Alert.StartsAt, true
	}
	return best, found
}

// topologyValue reads the topology evidence's verdict about the affected service.
func topologyValue(s domain.Snapshot, sup *catalog.MatchResult) (float64, bool) {
	for _, e := range s.EvidenceOfKind(domain.KindTopology) {
		switch {
		case listContains(e.Fact("candidate_origin"), s.Alert.Service):
			if sup != nil {
				sup.AddSupport(e.ID)
			}
			return 1.0, true
		case e.Fact("affected") != "" && e.Fact("candidate_origin") != "":
			if sup != nil {
				sup.AddSupport(e.ID)
			}
			return 0.4, true
		}
	}
	return 0, false
}

// runbookValue returns the best score among the runbooks this signature declares,
// read from the knowledge evidence facts.
func runbookValue(s domain.Snapshot, sig catalog.Signature, sup *catalog.MatchResult) float64 {
	if len(sig.RunbookIDs) == 0 {
		return 0
	}
	want := map[string]bool{}
	for _, id := range sig.RunbookIDs {
		want[id] = true
	}
	best := 0.0
	for _, e := range s.EvidenceOfKind(domain.KindKnowledge) {
		id := e.Fact("runbook_id")
		if !want[id] {
			continue
		}
		v, err := strconv.ParseFloat(e.Fact("score"), 64)
		if err != nil {
			continue
		}
		if v > best {
			best = v
			if sup != nil {
				sup.AddSupport(e.ID)
			}
		}
	}
	return best
}

// ScoreSignature scores a signature against a chosen subset of the case's evidence,
// using the same term derivation and weights the rule adapter uses.
//
// It exists so an alternative reasoner — the model adapter (ADR-007) — can select which
// signature the evidence supports without also inheriting the job of scoring it. The
// selection is the part where judgement helps; the arithmetic is the part that has to
// stay decomposable and identical whichever adapter produced the selection.
func ScoreSignature(
	sig catalog.Signature,
	cited []domain.Evidence,
	s domain.Snapshot,
	w Weights,
	penalty float64,
	unresolved int,
) (domain.ScoreBreakdown, []string) {
	m := catalog.Match(sig, cited)
	in := termInputs(m, s, &m)
	in.Unresolved = unresolved
	return Score(in, w, penalty), m.Supporting
}
