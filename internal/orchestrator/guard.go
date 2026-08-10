package orchestrator

import (
	"fmt"

	"github.com/zlrrr/mutil-agent-system/internal/domain"
)

// sdd:impl DLD-1062

// CanRemediate decides whether a case may leave analysis and act.
//
// The guard is evaluated by the orchestrator rather than by the agent proposing the
// hypothesis: the party that wants to advance is not the party that decides whether it
// may (ARC-007). The returned reason names the first unmet condition and is recorded on
// the transition, so a refusal is always explicable.
func (e *Engine) CanRemediate(c *domain.Case) (bool, string) {
	lead, ok := c.Leading()
	if !ok {
		return false, "no hypothesis has been proposed"
	}

	// 1. The critic must have accepted it.
	if lead.Verdict != "" && !lead.Verdict.Permits() {
		return false, fmt.Sprintf("the leading hypothesis carries verdict %q", lead.Verdict)
	}

	// 2. The evidence must fit what the explanation claimed.
	//
	// Fit rather than Total, because Total is a ranking quantity: it charges a signature
	// for evidence kinds it never claimed, so an explanation requiring one kind is capped
	// near 0.55 and can never be acted on however completely its own requirements are
	// met (D13). "Did it claim enough" is a separate question, asked separately by the
	// evidence-kind and change-evidence conditions below.
	if lead.Breakdown.Fit < e.cfg.AcceptThreshold {
		return false, fmt.Sprintf(
			"the leading hypothesis fits its own requirements at %.2f, below the "+
				"acceptance threshold of %.2f",
			lead.Breakdown.Fit, e.cfg.AcceptThreshold)
	}

	// 3. Evidence must span enough kinds.
	index := c.EvidenceIndex()
	kinds := lead.SupportingKinds(index)
	if len(kinds) < e.cfg.MinEvidenceKinds {
		return false, fmt.Sprintf(
			"the leading hypothesis is supported by %d evidence kind(s), fewer than the required %d",
			len(kinds), e.cfg.MinEvidenceKinds)
	}

	// 4. If anything changed in the window, the conclusion must account for it.
	if hasChangeEvidence(c) && !lead.HasKind(index, domain.KindChange) {
		return false, "a change was recorded in the window but the leading hypothesis " +
			"is not supported by change evidence"
	}

	// 5. A near-tie is not a conclusion. It may proceed only after the budget is spent
	//    and a human has looked at it.
	if runner, ok := c.Runner(); ok {
		gap := lead.Breakdown.Total - runner.Breakdown.Total
		if gap < e.cfg.CloseCallMargin && !c.HumanReviewed {
			return false, fmt.Sprintf(
				"the top two hypotheses differ by %.2f, inside the %.2f margin",
				gap, e.cfg.CloseCallMargin)
		}
	}
	return true, ""
}

// hasChangeEvidence reports whether any change was actually recorded — a "no change
// found" record does not count as a change.
func hasChangeEvidence(c *domain.Case) bool {
	for _, e := range c.Evidence {
		if e.Kind != domain.KindChange {
			continue
		}
		if e.Fact("changes_found") == "0" {
			continue
		}
		if e.Fact("change_key") != "" {
			return true
		}
	}
	return false
}

// budgetRemaining reports whether another collection round is permitted.
func (e *Engine) budgetRemaining(c *domain.Case) bool { return c.Round < e.cfg.MaxRounds }
