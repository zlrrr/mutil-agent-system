package reasoner

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/zlrrr/mutil-agent-system/internal/catalog"
	"github.com/zlrrr/mutil-agent-system/internal/domain"
)

// sdd:impl DLD-1034

// RuleContext is everything a critique rule may look at. Rules are pure functions of
// it, which is what lets each be triggered and asserted on its own (ARC-009).
type RuleContext struct {
	Snapshot domain.Snapshot
	Ranked   []domain.Hypothesis
	Matches  map[string]catalog.MatchResult
	Config   Config
	Catalog  *catalog.Catalog
}

// CritiqueRule is one independent power of the critic.
//
// Contract on identifiers: a rule leaves Critique.ID and EvidenceDemand.ID empty and
// references demands by *descriptor* in Critique.DemandIDs. The orchestrator assigns
// identifiers and rewrites those references, because only the orchestrator may mint an
// identifier (ARC-004).
type CritiqueRule interface {
	Name() string
	Apply(rc RuleContext, h domain.Hypothesis, rank int) ([]domain.Critique, []domain.EvidenceDemand)
}

// DefaultRules returns the ensemble in the order the detailed design declares. Order
// matters only for which demands survive the per-round cap.
func DefaultRules(cat *catalog.Catalog, cfg Config) []CritiqueRule {
	return []CritiqueRule{
		coverageGapRule{},
		alternativeExplanationRule{},
		temporalOrderRule{},
		sourceVsVictimRule{},
		unverifiableRemediationRule{},
		closeCallRule{},
	}
}

// Critique runs the ensemble over the ranked hypotheses.
func (r *RuleReasoner) Critique(_ context.Context, s domain.Snapshot) ([]domain.Critique, []domain.EvidenceDemand, error) {
	matches := map[string]catalog.MatchResult{}
	for _, m := range r.Matches(s) {
		matches[m.Signature.ID] = m
	}
	rc := RuleContext{
		Snapshot: s, Ranked: s.Hypotheses, Matches: matches,
		Config: r.cfg, Catalog: r.cat,
	}

	var critiques []domain.Critique
	var demands []domain.EvidenceDemand
	critiqued := map[string]bool{}

	for _, rule := range r.rules {
		for rank, h := range s.Hypotheses {
			cs, ds := rule.Apply(rc, h, rank)
			for _, c := range cs {
				c.Rule = rule.Name()
				critiques = append(critiques, c)
				critiqued[c.HypothesisID] = true
			}
			for _, d := range ds {
				d.Rule = rule.Name()
				demands = append(demands, d)
			}
		}
	}

	// A hypothesis nobody challenged receives an explicit accept, so that every
	// examined hypothesis carries a verdict (REQ-0030) rather than an absence.
	for _, h := range s.Hypotheses {
		if critiqued[h.ID] {
			continue
		}
		critiques = append(critiques, domain.Critique{
			HypothesisID: h.ID,
			Rule:         "no_objection",
			Category:     "no_objection",
			Challenge: "no rule found an unsupported step, an unexplained alternative " +
				"or a timing conflict in this explanation",
			Verdict: domain.VerdictAccept,
		})
	}

	demands = dedupeDemands(demands, r.cfg.MaxDemandsPerRound)
	return critiques, demands, nil
}

// dedupeDemands removes repeated descriptors, keeping the first occurrence, and applies
// the per-round cap. Order is (rule order, hypothesis rank) by construction.
func dedupeDemands(in []domain.EvidenceDemand, max int) []domain.EvidenceDemand {
	seen := map[string]bool{}
	out := make([]domain.EvidenceDemand, 0, len(in))
	for _, d := range in {
		if seen[d.Descriptor] {
			continue
		}
		seen[d.Descriptor] = true
		out = append(out, d)
		if max > 0 && len(out) == max {
			break
		}
	}
	return out
}

// ------------------------------------------------------------- rule: coverage gap

type coverageGapRule struct{}

func (coverageGapRule) Name() string { return "coverage_gap" }

// Apply challenges a hypothesis whose signature requires evidence that nothing has
// supplied, and demands exactly that evidence.
func (coverageGapRule) Apply(rc RuleContext, h domain.Hypothesis, _ int) ([]domain.Critique, []domain.EvidenceDemand) {
	m, ok := rc.Matches[h.SignatureID]
	if !ok {
		return nil, nil
	}
	var cs []domain.Critique
	var ds []domain.EvidenceDemand
	for _, p := range m.UnmatchedDemands() {
		if demandAlreadyAnswered(rc.Snapshot, p.Demand) {
			continue
		}
		ds = append(ds, domain.EvidenceDemand{
			Descriptor: p.Demand,
			Kind:       p.Kind,
			Reason: fmt.Sprintf("%q requires that %s, and no evidence shows it",
				h.Claim, p.Label),
		})
		cs = append(cs, domain.Critique{
			HypothesisID: h.ID,
			Category:     "coverage_gap",
			Challenge: fmt.Sprintf(
				"this explanation depends on %s, but no %s evidence establishes it; "+
					"until it does, the claim rests on the remaining evidence alone",
				p.Label, p.Kind),
			Verdict:   domain.VerdictRevise,
			DemandIDs: []string{p.Demand},
		})
	}
	return cs, ds
}

// -------------------------------------------------- rule: alternative explanation

type alternativeExplanationRule struct{}

func (alternativeExplanationRule) Name() string { return "alternative_explanation" }

// Apply challenges the leading hypothesis whenever a rival explanation is consistent
// with the same evidence and nothing has yet ruled it out. This is the stronger
// trigger noted in DLD-1034: a rival nobody has excluded always earns a challenge.
func (alternativeExplanationRule) Apply(rc RuleContext, h domain.Hypothesis, rank int) ([]domain.Critique, []domain.EvidenceDemand) {
	if rank != 0 {
		return nil, nil
	}
	var cs []domain.Critique
	var ds []domain.EvidenceDemand

	ids := make([]string, 0, len(rc.Matches))
	for id := range rc.Matches {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		if id == h.SignatureID {
			continue
		}
		rival := rc.Matches[id]
		for _, disc := range rival.Signature.Discriminators {
			if demandAlreadyAnswered(rc.Snapshot, disc.Descriptor) {
				continue
			}
			ds = append(ds, domain.EvidenceDemand{
				Descriptor: disc.Descriptor,
				Kind:       disc.Kind,
				Reason:     disc.Reason,
			})
			cs = append(cs, domain.Critique{
				HypothesisID: h.ID,
				Category:     "alternative_explanation",
				Challenge: fmt.Sprintf(
					"%q is also consistent with the evidence collected so far, and "+
						"nothing yet separates it from this explanation; %s",
					rival.Signature.Claim, disc.Reason),
				Verdict:   domain.VerdictRevise,
				DemandIDs: []string{disc.Descriptor},
			})
		}
	}
	return cs, ds
}

// ----------------------------------------------------------- rule: temporal order

type temporalOrderRule struct{}

func (temporalOrderRule) Name() string { return "temporal_order" }

// Apply refuses a change-attributing explanation whose change did not precede the
// anomaly. A cause that arrives after its effect is not a cause.
func (temporalOrderRule) Apply(rc RuleContext, h domain.Hypothesis, _ int) ([]domain.Critique, []domain.EvidenceDemand) {
	m, ok := rc.Matches[h.SignatureID]
	if !ok || m.Signature.AttributesTo != "change" || !m.HasChangeAt {
		return nil, nil
	}
	onset, ok := HypothesisOnset(m, rc.Snapshot)
	if !ok || !m.ChangeAt.After(onset) {
		return nil, nil
	}

	var counters []string
	for _, e := range rc.Snapshot.EvidenceOfKind(domain.KindChange) {
		if e.Fact("changed_at") == m.ChangeAt.UTC().Format(time.RFC3339) {
			counters = append(counters, e.ID)
		}
	}
	return []domain.Critique{{
		HypothesisID: h.ID,
		Category:     "temporal_order",
		Challenge: fmt.Sprintf(
			"the change this explanation blames is recorded at %s, after the anomaly "+
				"began at %s; a change cannot cause a symptom that preceded it",
			m.ChangeAt.UTC().Format("15:04:05"), onset.UTC().Format("15:04:05")),
		Verdict:    domain.VerdictReject,
		CounterIDs: counters,
	}}, nil
}

// --------------------------------------------------------- rule: source vs victim

type sourceVsVictimRule struct{}

func (sourceVsVictimRule) Name() string { return "source_vs_victim" }

// Apply challenges any explanation that localises the cause inside a service which
// itself depends on a service that became anomalous earlier — and demands the evidence
// that would let an explanation form upstream instead.
//
// Both halves matter. Without the skip, the rule also challenges the explanation that
// already blames the upstream, using the very evidence that supports it. Without the
// demands, the rule says "you may be looking at the wrong service" and asks for nothing,
// which ends the investigation rather than redirecting it: a challenge no evidence can
// answer is a veto, not a critique (REQ-0101).
func (sourceVsVictimRule) Apply(rc RuleContext, h domain.Hypothesis, _ int) ([]domain.Critique, []domain.EvidenceDemand) {
	for _, e := range rc.Snapshot.EvidenceOfKind(domain.KindTopology) {
		upstream := e.Fact("upstream_anomalous")
		if upstream == "" {
			continue
		}
		if restsOnUpstreamEvidence(rc, h, upstream) {
			// This explanation already rests on evidence from the upstream service.
			// Objecting to it would be objecting to the answer the rule exists to
			// reach, using the very evidence that supports it.
			return nil, nil
		}

		ds := upstreamDemands(rc, upstream)
		demandIDs := make([]string, 0, len(ds))
		for _, d := range ds {
			demandIDs = append(demandIDs, d.Descriptor)
		}

		return []domain.Critique{{
			HypothesisID: h.ID,
			Category:     "source_vs_victim",
			Challenge: fmt.Sprintf(
				"%s depends on %s, which became anomalous at %s — earlier than %s; "+
					"this explanation may be describing a victim rather than the origin",
				rc.Snapshot.Alert.Service, upstream, e.Fact("upstream_first_seen"),
				rc.Snapshot.Alert.Service),
			Verdict:    domain.VerdictRevise,
			CounterIDs: []string{e.ID},
			DemandIDs:  demandIDs,
		}}, ds
	}
	return nil, nil
}

// restsOnUpstreamEvidence reports whether any evidence supporting this hypothesis
// concerns the named service.
//
// The question is asked of the evidence rather than of the signature, because a
// signature's remediation names a service by catalog convention, not by inference —
// every signature in this catalog happens to remediate `order-api`, so reading intent
// from that field would make every explanation look upstream-aware.
func restsOnUpstreamEvidence(rc RuleContext, h domain.Hypothesis, service string) bool {
	for _, id := range h.Supporting {
		e, ok := rc.Snapshot.Index[id]
		if !ok {
			continue
		}
		if e.Fact("subject") == service {
			return true
		}
	}
	return false
}

// upstreamDemands returns the evidence that would let an explanation form in the
// upstream service: the requirement descriptors of every signature whose remediation
// acts there, minus anything already answered.
func upstreamDemands(rc RuleContext, upstream string) []domain.EvidenceDemand {
	var out []domain.EvidenceDemand
	seen := map[string]bool{}

	ids := make([]string, 0, len(rc.Catalog.Signatures))
	for _, sig := range rc.Catalog.Signatures {
		ids = append(ids, sig.ID)
	}
	sort.Strings(ids)

	for _, id := range ids {
		sig, ok := rc.Catalog.Signature(id)
		if !ok {
			continue
		}
		for _, p := range sig.Requires {
			if p.Demand == "" || seen[p.Demand] {
				continue
			}
			if demandAlreadyAnswered(rc.Snapshot, p.Demand) {
				continue
			}
			seen[p.Demand] = true
			out = append(out, domain.EvidenceDemand{
				Descriptor: p.Demand,
				Kind:       p.Kind,
				Reason: fmt.Sprintf(
					"an explanation localised in %s would need this, and nothing has looked there yet",
					upstream),
			})
		}
	}
	return out
}

// ------------------------------------------------- rule: unverifiable remediation

type unverifiableRemediationRule struct{}

func (unverifiableRemediationRule) Name() string { return "unverifiable_remediation" }

// Apply flags an explanation that implies no action whose effect could be checked.
// Such a conclusion may still be right, but it cannot be confirmed by acting on it.
func (unverifiableRemediationRule) Apply(rc RuleContext, h domain.Hypothesis, _ int) ([]domain.Critique, []domain.EvidenceDemand) {
	m, ok := rc.Matches[h.SignatureID]
	if !ok {
		return nil, nil
	}
	if r := m.Signature.Remediation; r != nil && len(r.Verify) > 0 {
		return nil, nil
	}
	return []domain.Critique{{
		HypothesisID: h.ID,
		Category:     "unverifiable_remediation",
		Challenge: "this explanation implies no action whose effect could be measured, " +
			"so accepting it cannot be confirmed by acting on it",
		Verdict: domain.VerdictAcceptWithRisk,
	}}, nil
}

// ---------------------------------------------------------------- rule: close call

type closeCallRule struct{}

func (closeCallRule) Name() string { return "close_call" }

// Apply refuses to let a near-tie resolve itself by rounding. When the top two are
// within the margin, the leader is challenged and the evidence that would separate
// them is demanded.
func (closeCallRule) Apply(rc RuleContext, h domain.Hypothesis, rank int) ([]domain.Critique, []domain.EvidenceDemand) {
	if rank != 0 || len(rc.Ranked) < 2 {
		return nil, nil
	}
	runner := rc.Ranked[1]
	gap := h.Breakdown.Total - runner.Breakdown.Total
	if gap >= rc.Config.CloseCallMargin {
		return nil, nil
	}

	var ds []domain.EvidenceDemand
	var refs []string
	if m, ok := rc.Matches[h.SignatureID]; ok {
		for _, disc := range m.Signature.Discriminators {
			if demandAlreadyAnswered(rc.Snapshot, disc.Descriptor) {
				continue
			}
			ds = append(ds, domain.EvidenceDemand{
				Descriptor: disc.Descriptor,
				Kind:       disc.Kind,
				Reason:     disc.Reason,
			})
			refs = append(refs, disc.Descriptor)
		}
	}
	return []domain.Critique{{
		HypothesisID: h.ID,
		Category:     "close_call",
		Challenge: fmt.Sprintf(
			"the leading explanation is only %.2f ahead of %q, which is inside the "+
				"%.2f margin; the ranking is not yet meaningful",
			gap, runner.Claim, rc.Config.CloseCallMargin),
		Verdict:   domain.VerdictRevise,
		DemandIDs: refs,
	}}, ds
}

// ------------------------------------------------------------------------ helpers

// demandAlreadyAnswered reports whether evidence answering a descriptor has already
// been collected, so the critic does not demand what it already has.
func demandAlreadyAnswered(s domain.Snapshot, descriptor string) bool {
	if descriptor == "" {
		return true
	}
	for _, e := range s.Evidence {
		if e.Fact("demand") == descriptor {
			return true
		}
	}
	for _, d := range s.Demands {
		if d.Descriptor == descriptor && d.Satisfied() {
			return true
		}
	}
	return false
}

// listContains reports whether a comma-separated fact value contains an entry.
func listContains(list, want string) bool {
	for _, p := range strings.Split(list, ",") {
		if strings.TrimSpace(p) == want {
			return true
		}
	}
	return false
}
