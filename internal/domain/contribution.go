package domain

import (
	"errors"
	"fmt"
)

// sdd:impl DLD-1006

// ContributionKind names the one payload a contribution carries. The set is closed:
// adding a kind is an architectural amendment, not a feature (ARC-002).
type ContributionKind string

// The contribution kinds an agent may return.
const (
	ContribAddEvidence        ContributionKind = "add_evidence"
	ContribProposeHypothesis  ContributionKind = "propose_hypothesis"
	ContribRaiseCritique      ContributionKind = "raise_critique"
	ContribDemandEvidence     ContributionKind = "demand_evidence"
	ContribProposeAction      ContributionKind = "propose_action"
	ContribRecordVerification ContributionKind = "record_verification"
	ContribRecordExecution    ContributionKind = "record_execution"
)

// Contribution is the only thing an agent may return, and the only channel through
// which anything an agent concludes can reach case state.
type Contribution struct {
	Kind         ContributionKind    `json:"kind"`
	Role         Role                `json:"role"`
	Index        int                 `json:"index"`
	Evidence     *Evidence           `json:"evidence,omitempty"`
	Hypothesis   *Hypothesis         `json:"hypothesis,omitempty"`
	Critique     *Critique           `json:"critique,omitempty"`
	Demand       *EvidenceDemand     `json:"demand,omitempty"`
	Action       *Action             `json:"action,omitempty"`
	Verification *VerificationResult `json:"verification,omitempty"`
	Execution    *ExecutionResult    `json:"execution,omitempty"`
}

// Contribution validation errors.
var (
	ErrUnknownContribution = errors.New("unknown contribution kind")
	ErrPayloadMismatch     = errors.New("contribution payload does not match its kind")
	ErrRoleNotPermitted    = errors.New("role may not emit this contribution kind")
)

// roleCapabilities declares what each role is allowed to say. Enforcing this in one
// table — rather than trusting each agent — is what makes role boundaries real
// (REQ-0005).
var roleCapabilities = map[Role][]ContributionKind{
	RoleMetrics:      {ContribAddEvidence},
	RoleLogs:         {ContribAddEvidence},
	RoleChange:       {ContribAddEvidence},
	RoleTopology:     {ContribAddEvidence},
	RoleKnowledge:    {ContribAddEvidence},
	RoleAnalysis:     {ContribProposeHypothesis},
	RoleCritic:       {ContribRaiseCritique, ContribDemandEvidence},
	RoleRemediation:  {ContribProposeAction},
	RoleExecutor:     {ContribRecordExecution},
	RoleVerification: {ContribAddEvidence, ContribRecordVerification},
	RoleBaseline:     {ContribAddEvidence, ContribProposeHypothesis, ContribProposeAction},
}

// MayEmit reports whether the role is permitted to emit a contribution kind. An
// unrecognised role may emit nothing.
func (r Role) MayEmit(k ContributionKind) bool {
	for _, allowed := range roleCapabilities[r] {
		if allowed == k {
			return true
		}
	}
	return false
}

// Capabilities returns the contribution kinds a role may emit, for documentation and
// for the console.
func (r Role) Capabilities() []ContributionKind {
	out := make([]ContributionKind, len(roleCapabilities[r]))
	copy(out, roleCapabilities[r])
	return out
}

// Validate checks that exactly the payload matching Kind is present, that no other
// payload is set, and that the payload itself is valid.
func (c Contribution) Validate() error {
	set := 0
	if c.Evidence != nil {
		set++
	}
	if c.Hypothesis != nil {
		set++
	}
	if c.Critique != nil {
		set++
	}
	if c.Demand != nil {
		set++
	}
	if c.Action != nil {
		set++
	}
	if c.Verification != nil {
		set++
	}
	if c.Execution != nil {
		set++
	}
	if set != 1 {
		return fmt.Errorf("%w: %d payloads set", ErrPayloadMismatch, set)
	}

	switch c.Kind {
	case ContribAddEvidence:
		if c.Evidence == nil {
			return ErrPayloadMismatch
		}
		return c.Evidence.Validate()
	case ContribProposeHypothesis:
		if c.Hypothesis == nil {
			return ErrPayloadMismatch
		}
		return c.Hypothesis.Validate()
	case ContribRaiseCritique:
		if c.Critique == nil {
			return ErrPayloadMismatch
		}
		return c.Critique.Validate()
	case ContribDemandEvidence:
		if c.Demand == nil {
			return ErrPayloadMismatch
		}
		return c.Demand.Validate()
	case ContribProposeAction:
		if c.Action == nil {
			return ErrPayloadMismatch
		}
		return c.Action.Validate()
	case ContribRecordVerification:
		if c.Verification == nil {
			return ErrPayloadMismatch
		}
		return c.Verification.Validate()
	case ContribRecordExecution:
		if c.Execution == nil {
			return ErrPayloadMismatch
		}
		return nil
	}
	return ErrUnknownContribution
}

// Helper constructors keep agent code declarative and make the emitting role explicit.

// AddEvidence builds an evidence contribution.
func AddEvidence(role Role, index int, e Evidence) Contribution {
	return Contribution{Kind: ContribAddEvidence, Role: role, Index: index, Evidence: &e}
}

// ProposeHypothesis builds a hypothesis contribution.
func ProposeHypothesis(role Role, index int, h Hypothesis) Contribution {
	return Contribution{Kind: ContribProposeHypothesis, Role: role, Index: index, Hypothesis: &h}
}

// RaiseCritique builds a critique contribution.
func RaiseCritique(role Role, index int, c Critique) Contribution {
	return Contribution{Kind: ContribRaiseCritique, Role: role, Index: index, Critique: &c}
}

// DemandEvidence builds an evidence-demand contribution.
func DemandEvidence(role Role, index int, d EvidenceDemand) Contribution {
	return Contribution{Kind: ContribDemandEvidence, Role: role, Index: index, Demand: &d}
}

// ProposeAction builds an action contribution.
func ProposeAction(role Role, index int, a Action) Contribution {
	return Contribution{Kind: ContribProposeAction, Role: role, Index: index, Action: &a}
}

// RecordVerification builds a verification contribution.
func RecordVerification(role Role, index int, v VerificationResult) Contribution {
	return Contribution{Kind: ContribRecordVerification, Role: role, Index: index, Verification: &v}
}
