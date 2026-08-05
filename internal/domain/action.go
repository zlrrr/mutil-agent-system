package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"time"
)

// sdd:impl DLD-1005

// Risk classifies how much damage an action could do if it is wrong.
type Risk string

// The three risk classes. Only low-risk actions may execute without a human decision.
const (
	RiskLow    Risk = "low"
	RiskMedium Risk = "medium"
	RiskHigh   Risk = "high"
)

// Valid reports whether r is a declared risk class.
func (r Risk) Valid() bool {
	return r == RiskLow || r == RiskMedium || r == RiskHigh
}

// RequiresApproval reports whether the risk class halts the state machine (REQ-0042).
func (r Risk) RequiresApproval() bool { return r != RiskLow }

// ActionCall is a typed invocation of an actuator tool. The executor accepts only this
// shape — never natural language (CON-012).
type ActionCall struct {
	Tool string            `json:"tool"`
	Args map[string]string `json:"args"`
}

// ApprovalDecision records a human's judgement on a proposed action.
type ApprovalDecision struct {
	Decision    string    `json:"decision"` // approved | rejected
	By          string    `json:"by"`
	Comment     string    `json:"comment,omitempty"`
	At          time.Time `json:"at"`
	Fingerprint string    `json:"fingerprint"`
}

// Approved reports whether the decision permits execution.
func (d ApprovalDecision) Approved() bool { return d.Decision == "approved" }

// ExecutionResult records what happened when an action ran, or why it did not.
type ExecutionResult struct {
	ActionID string        `json:"action_id"`
	Outcome  string        `json:"outcome"` // executed | refused | failed
	Rule     string        `json:"rule,omitempty"`
	Detail   string        `json:"detail"`
	Duration time.Duration `json:"duration"`
	At       time.Time     `json:"at"`
}

// Action is a complete remediation proposal: what it would do, what it costs if wrong,
// how to undo it and how to tell whether it worked (REQ-0040).
type Action struct {
	ID            string            `json:"id"`
	Title         string            `json:"title"`
	Tool          string            `json:"tool"`
	Args          map[string]string `json:"args"`
	Risk          Risk              `json:"risk"`
	Rationale     string            `json:"rationale"`
	Preconditions []string          `json:"preconditions,omitempty"`
	Rollback      *ActionCall       `json:"rollback,omitempty"`
	Verify        []string          `json:"verify,omitempty"`
	HypothesisID  string            `json:"hypothesis_id"`
	Approval      *ApprovalDecision `json:"approval,omitempty"`
	Execution     *ExecutionResult  `json:"execution,omitempty"`
}

// Validation errors for actions.
var (
	ErrUnknownRisk     = errors.New("unknown risk class")
	ErrMissingTool     = errors.New("action must name a tool")
	ErrMissingRollback = errors.New("action above low risk must declare a rollback")
	ErrMissingVerify   = errors.New("action above low risk must declare a verification signal")
	ErrMissingTitle    = errors.New("action must carry a title")
)

// Validate refuses an incomplete proposal. An action that cannot be undone or checked
// is not proposable above the low risk class.
func (a Action) Validate() error {
	if !a.Risk.Valid() {
		return ErrUnknownRisk
	}
	if strings.TrimSpace(a.Tool) == "" {
		return ErrMissingTool
	}
	if strings.TrimSpace(a.Title) == "" {
		return ErrMissingTitle
	}
	if a.Risk != RiskLow {
		if a.Rollback == nil || strings.TrimSpace(a.Rollback.Tool) == "" {
			return ErrMissingRollback
		}
		if len(a.Verify) == 0 {
			return ErrMissingVerify
		}
	}
	return nil
}

// Fingerprint hashes the tool and its sorted arguments. Comparing the fingerprint taken
// at approval with the one taken at execution is how post-approval tampering is caught
// (REQ-0043).
func (a Action) Fingerprint() string {
	keys := make([]string, 0, len(a.Args))
	for k := range a.Args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	h.Write([]byte(a.Tool))
	for _, k := range keys {
		h.Write([]byte{0})
		h.Write([]byte(k))
		h.Write([]byte{'='})
		h.Write([]byte(a.Args[k]))
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// Call returns the action as a typed actuator invocation.
func (a Action) Call() ActionCall {
	args := make(map[string]string, len(a.Args))
	for k, v := range a.Args {
		args[k] = v
	}
	return ActionCall{Tool: a.Tool, Args: args}
}

// VerificationResult records whether a signal recovered after an action ran.
type VerificationResult struct {
	ActionID  string `json:"action_id"`
	Signal    string `json:"signal"`
	Before    string `json:"before"`
	After     string `json:"after"`
	Recovered bool   `json:"recovered"`
	Detail    string `json:"detail,omitempty"`
}

// Validate enforces that a verification names both a signal and an action.
func (v VerificationResult) Validate() error {
	if strings.TrimSpace(v.Signal) == "" {
		return errors.New("verification must name a signal")
	}
	if strings.TrimSpace(v.ActionID) == "" {
		return errors.New("verification must reference an action")
	}
	return nil
}
