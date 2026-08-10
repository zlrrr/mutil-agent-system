package domain

import (
	"encoding/json"
	"time"
)

// sdd:impl DLD-1007

// EventType names what happened. Every state change, agent invocation, tool call and
// policy decision has one, because the report must be reconstructable from the log
// alone (CON-011).
type EventType string

// The event vocabulary.
const (
	EvCaseCreated          EventType = "case_created"
	EvStateChanged         EventType = "state_changed"
	EvPlanRecorded         EventType = "plan_recorded"
	EvRoundStarted         EventType = "round_started"
	EvAgentStarted         EventType = "agent_started"
	EvAgentCompleted       EventType = "agent_completed"
	EvAgentFailed          EventType = "agent_failed"
	EvToolCalled           EventType = "tool_called"
	EvEvidenceAdded        EventType = "evidence_added"
	EvEvidenceTruncated    EventType = "evidence_truncated"
	EvHypothesisProposed   EventType = "hypothesis_proposed"
	EvHypothesisRejected   EventType = "hypothesis_rejected"
	EvHypothesisScored     EventType = "hypothesis_scored"
	EvCritiqueRaised       EventType = "critique_raised"
	EvEvidenceDemanded     EventType = "evidence_demanded"
	EvDemandSatisfied      EventType = "demand_satisfied"
	EvDemandUnmet          EventType = "demand_unmet"
	EvContributionRejected EventType = "contribution_rejected"
	EvActionProposed       EventType = "action_proposed"
	EvApprovalRequested    EventType = "approval_requested"
	EvApprovalRecorded     EventType = "approval_recorded"
	EvActionExecuted       EventType = "action_executed"
	EvActionRefused        EventType = "action_refused"
	EvVerificationRecorded EventType = "verification_recorded"
	EvBudgetExhausted      EventType = "budget_exhausted"
	EvReportGenerated      EventType = "report_generated"
	EvCaseClosed           EventType = "case_closed"
)

// Event is one immutable entry in a case's log.
type Event struct {
	Seq      int             `json:"seq"`
	CaseID   string          `json:"case_id"`
	Actor    Role            `json:"actor"`
	Type     EventType       `json:"type"`
	Summary  string          `json:"summary"`
	Ref      string          `json:"ref,omitempty"`
	Duration time.Duration   `json:"duration,omitempty"`
	At       time.Time       `json:"at"`
	Payload  json.RawMessage `json:"payload,omitempty"`
}

// Payloads carried by events that do not embed a whole entity.

// StateChange is the payload of a state_changed event.
type StateChange struct {
	From   Status `json:"from"`
	To     Status `json:"to"`
	Reason string `json:"reason,omitempty"`
}

// RoundStart is the payload of a round_started event.
type RoundStart struct {
	Round int `json:"round"`
}

// Rejection is the payload of a contribution_rejected or hypothesis_rejected event.
type Rejection struct {
	Role   Role             `json:"role,omitempty"`
	Kind   ContributionKind `json:"kind,omitempty"`
	Target string           `json:"target,omitempty"`
	Reason string           `json:"reason"`
}

// DemandLink is the payload of demand_satisfied.
type DemandLink struct {
	DemandID   string `json:"demand_id"`
	EvidenceID string `json:"evidence_id"`
}

// ApprovalRecord is the payload of approval_recorded.
type ApprovalRecord struct {
	ActionID string           `json:"action_id"`
	Decision ApprovalDecision `json:"decision"`
}

// ToolCall is the payload of tool_called.
type ToolCall struct {
	Port  string `json:"port"`
	Query string `json:"query"`
	Rows  int    `json:"rows"`
}

// MustPayload marshals a payload, panicking only on a programming error — the payload
// types in this package are all encodable.
func MustPayload(v any) json.RawMessage {
	if v == nil {
		return nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		panic("domain: unencodable event payload: " + err.Error())
	}
	return raw
}

// decode unmarshals an event payload into v.
func decode(raw json.RawMessage, v any) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, v)
}
