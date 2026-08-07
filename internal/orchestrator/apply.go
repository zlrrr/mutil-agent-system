package orchestrator

import (
	"fmt"
	"sort"

	"github.com/zlrrr/mutil-agent-system/internal/domain"
)

// sdd:impl DLD-1061

// MaxEvidencePerCase bounds retained evidence so a case cannot grow without limit
// (ARC-011). Reaching it is recorded, never silently absorbed.
const MaxEvidencePerCase = 200

// apply validates, identifies and folds a batch of contributions into the case,
// returning the events it produced.
//
// This function is the reason agents can run concurrently: it is the single writer,
// and it sorts before it assigns, so completion order cannot reach the result.
func (e *Engine) apply(c *domain.Case, contribs []domain.Contribution) []domain.Event {
	sorted := append([]domain.Contribution(nil), contribs...)
	sort.SliceStable(sorted, func(i, j int) bool {
		oi, iok := domain.CollectorOrder(sorted[i].Role)
		oj, jok := domain.CollectorOrder(sorted[j].Role)
		switch {
		case iok && jok && oi != oj:
			return oi < oj
		case iok != jok:
			return iok
		case sorted[i].Role != sorted[j].Role:
			return sorted[i].Role < sorted[j].Role
		default:
			return sorted[i].Index < sorted[j].Index
		}
	})

	// Demand descriptors are rewritten to assigned identifiers once demands are
	// applied, honouring the contract in DLD-1034.
	demandIDByDescriptor := map[string]string{}
	for _, d := range c.Demands {
		demandIDByDescriptor[d.Descriptor] = d.ID
	}

	var events []domain.Event
	for _, contrib := range sorted {
		if !contrib.Role.MayEmit(contrib.Kind) {
			events = append(events, e.event(c, contrib.Role, domain.EvContributionRejected,
				fmt.Sprintf("%s may not emit %s", contrib.Role, contrib.Kind),
				"", domain.Rejection{
					Role: contrib.Role, Kind: contrib.Kind,
					Reason: "role is not permitted to emit this contribution kind",
				}))
			continue
		}
		if err := contrib.Validate(); err != nil {
			events = append(events, e.event(c, contrib.Role, domain.EvContributionRejected,
				fmt.Sprintf("%s emitted an invalid %s", contrib.Role, contrib.Kind),
				"", domain.Rejection{
					Role: contrib.Role, Kind: contrib.Kind, Reason: err.Error(),
				}))
			continue
		}

		switch contrib.Kind {
		case domain.ContribAddEvidence:
			events = append(events, e.applyEvidence(c, contrib, events)...)
		case domain.ContribProposeHypothesis:
			events = append(events, e.applyHypothesis(c, contrib, events)...)
		case domain.ContribDemandEvidence:
			ev := e.applyDemand(c, contrib, events, demandIDByDescriptor)
			events = append(events, ev...)
		case domain.ContribRaiseCritique:
			events = append(events, e.applyCritique(c, contrib, events, demandIDByDescriptor))
		case domain.ContribProposeAction:
			events = append(events, e.applyAction(c, contrib, events)...)
		case domain.ContribRecordVerification:
			v := *contrib.Verification
			events = append(events, e.event(c, contrib.Role, domain.EvVerificationRecorded,
				fmt.Sprintf("%s %s after the action", v.Signal, recoveredWord(v.Recovered)),
				v.ActionID, v))
		}
	}
	return events
}

func recoveredWord(b bool) string {
	if b {
		return "recovered"
	}
	return "did not recover"
}

func (e *Engine) applyEvidence(c *domain.Case, contrib domain.Contribution, pending []domain.Event) []domain.Event {
	if len(c.Evidence)+countType(pending, domain.EvEvidenceAdded) >= MaxEvidencePerCase {
		return []domain.Event{e.event(c, contrib.Role, domain.EvEvidenceTruncated,
			"evidence retention cap reached; further evidence in this round was dropped",
			"", domain.Rejection{Role: contrib.Role, Reason: "maxEvidencePerCase"})}
	}

	ev := *contrib.Evidence
	ev.ID = domain.NewID(domain.PrefixEvidence, contrib.Role, c.NextSeq(domain.PrefixEvidence, contrib.Role))
	if ev.Agent == "" {
		ev.Agent = contrib.Role
	}

	out := []domain.Event{e.event(c, contrib.Role, domain.EvEvidenceAdded, ev.Summary, ev.ID, ev)}
	if ev.Truncated {
		out = append(out, e.event(c, contrib.Role, domain.EvEvidenceTruncated,
			"tool result exceeded its bound and was truncated", ev.ID, nil))
	}

	// Evidence carrying a demand descriptor satisfies that demand.
	if desc := ev.Fact("demand"); desc != "" {
		for _, d := range c.Demands {
			if d.Descriptor == desc && !d.Satisfied() {
				out = append(out, e.event(c, contrib.Role, domain.EvDemandSatisfied,
					"the demand for "+desc+" is answered", d.ID,
					domain.DemandLink{DemandID: d.ID, EvidenceID: ev.ID}))
				break
			}
		}
	}
	return out
}

func (e *Engine) applyHypothesis(c *domain.Case, contrib domain.Contribution, pending []domain.Event) []domain.Event {
	h := *contrib.Hypothesis

	// Evidence-before-conclusion, enforced rather than scored (REQ-0021).
	known := c.EvidenceIndex()
	for _, id := range pendingEvidenceIDs(pending) {
		known[id] = domain.Evidence{ID: id}
	}
	var missing []string
	for _, id := range h.Supporting {
		if _, ok := known[id]; !ok {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		return []domain.Event{e.event(c, contrib.Role, domain.EvHypothesisRejected,
			"hypothesis references evidence that does not exist", h.SignatureID,
			domain.Rejection{
				Role: contrib.Role, Target: h.SignatureID,
				Reason: fmt.Sprintf("unknown supporting evidence: %v", missing),
			})}
	}

	if h.ID == "" {
		h.ID = domain.NewID(domain.PrefixHypothesis, contrib.Role, c.NextSeq(domain.PrefixHypothesis, contrib.Role))
	}
	typ := domain.EvHypothesisProposed
	for _, existing := range c.Hypotheses {
		if existing.ID == h.ID {
			typ = domain.EvHypothesisScored
			break
		}
	}
	return []domain.Event{e.event(c, contrib.Role, typ,
		fmt.Sprintf("%s (score %.2f)", h.Claim, h.Breakdown.Total), h.ID, h)}
}

func (e *Engine) applyDemand(c *domain.Case, contrib domain.Contribution, pending []domain.Event, byDesc map[string]string) []domain.Event {
	d := *contrib.Demand
	d.Round = c.Round
	if id, ok := byDesc[d.Descriptor]; ok {
		d.ID = id // already demanded in an earlier round; keep its identity
		// ...and the round it was first raised in, so re-raising cannot reset the
		// record of when it was attempted.
		if i, ok := c.DemandByID(id); ok && i.Round > 0 {
			d.Round = i.Round
		}
	} else {
		d.ID = domain.NewID(domain.PrefixDemand, contrib.Role, c.NextSeq(domain.PrefixDemand, contrib.Role))
		byDesc[d.Descriptor] = d.ID
	}
	return []domain.Event{e.event(c, contrib.Role, domain.EvEvidenceDemanded,
		"the critic requires "+d.Descriptor, d.ID, d)}
}

func (e *Engine) applyCritique(c *domain.Case, contrib domain.Contribution, pending []domain.Event, byDesc map[string]string) domain.Event {
	cr := *contrib.Critique
	cr.ID = domain.NewID(domain.PrefixCritique, contrib.Role, c.NextSeq(domain.PrefixCritique, contrib.Role))
	// Rewrite descriptor references to the identifiers just assigned (DLD-1034).
	for i, ref := range cr.DemandIDs {
		if id, ok := byDesc[ref]; ok {
			cr.DemandIDs[i] = id
		}
	}
	return e.event(c, contrib.Role, domain.EvCritiqueRaised,
		fmt.Sprintf("[%s] %s", cr.Verdict, cr.Challenge), cr.HypothesisID, cr)
}

func (e *Engine) applyAction(c *domain.Case, contrib domain.Contribution, pending []domain.Event) []domain.Event {
	a := *contrib.Action
	a.ID = domain.NewID(domain.PrefixAction, contrib.Role, c.NextSeq(domain.PrefixAction, contrib.Role))

	// Policy classifies at proposal time; it is evaluated again before execution.
	d := e.policy.Evaluate(a)
	if !d.Allowed {
		return []domain.Event{
			e.event(c, contrib.Role, domain.EvActionProposed, a.Title, a.ID, a),
			e.event(c, domain.RoleExecutor, domain.EvActionRefused,
				"policy refused the proposed action: "+d.Reason, a.ID,
				domain.ExecutionResult{
					ActionID: a.ID, Outcome: "refused", Rule: d.Rule, Detail: d.Reason,
					At: e.clock.Now(),
				}),
		}
	}
	out := []domain.Event{e.event(c, contrib.Role, domain.EvActionProposed,
		fmt.Sprintf("%s (%s risk)", a.Title, a.Risk), a.ID, a)}
	if d.RequiresApproval {
		out = append(out, e.event(c, domain.RoleExecutor, domain.EvApprovalRequested,
			"a human decision is required before this action can run", a.ID, nil))
	}
	return out
}

func pendingEvidenceIDs(events []domain.Event) []string {
	var out []string
	for _, ev := range events {
		if ev.Type == domain.EvEvidenceAdded {
			out = append(out, ev.Ref)
		}
	}
	return out
}

func countType(events []domain.Event, t domain.EventType) int {
	n := 0
	for _, e := range events {
		if e.Type == t {
			n++
		}
	}
	return n
}
