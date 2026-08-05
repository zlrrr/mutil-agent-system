package policy

import (
	"context"
	"fmt"
	"time"

	"github.com/zlrrr/mutil-agent-system/internal/domain"
	"github.com/zlrrr/mutil-agent-system/internal/signal"
)

// sdd:impl DLD-1051

// Executor is the only holder of an actuator reference in the process. Every path to a
// target system goes through Execute, and Execute always re-evaluates policy first.
type Executor struct {
	engine   *Engine
	actuator signal.Actuator
	clock    domain.Clock
}

// NewExecutor binds a policy engine to an actuator.
func NewExecutor(engine *Engine, actuator signal.Actuator, clock domain.Clock) *Executor {
	return &Executor{engine: engine, actuator: actuator, clock: clock}
}

// Execute applies an approved action, refusing rather than erroring so that every
// refusal lands in the event log and the report.
//
// The order of checks is the safety property: approval, then tamper detection, then a
// fresh policy evaluation against the action exactly as it now stands. Nothing here
// trusts a decision made earlier (REQ-0043).
func (x *Executor) Execute(ctx context.Context, a domain.Action) domain.ExecutionResult {
	now := x.clock.Now()

	// 1. An action above low risk needs a recorded approval.
	if a.Risk.RequiresApproval() {
		if a.Approval == nil {
			return refuse(a, "approval_required",
				"action requires approval and none is recorded", now)
		}
		if !a.Approval.Approved() {
			return refuse(a, "approval_rejected",
				"approval was recorded as "+a.Approval.Decision, now)
		}
		// 2. The action must be the one that was approved.
		if fp := a.Fingerprint(); a.Approval.Fingerprint != "" && fp != a.Approval.Fingerprint {
			return refuse(a, "tampered_after_approval", fmt.Sprintf(
				"action fingerprint %s does not match the approved %s",
				fp, a.Approval.Fingerprint), now)
		}
	}

	// 3. Policy is evaluated again, now, against the action as it stands.
	if d := x.engine.Evaluate(a); !d.Allowed {
		return refuse(a, d.Rule, d.Reason, now)
	}

	// 4. Only now is the target system touched.
	started := x.clock.Now()
	res, err := x.actuator.Invoke(ctx, a.Call())
	finished := x.clock.Now()
	if err != nil {
		return domain.ExecutionResult{
			ActionID: a.ID, Outcome: "failed", Detail: err.Error(),
			Duration: finished.Sub(started), At: finished,
		}
	}
	return domain.ExecutionResult{
		ActionID: a.ID, Outcome: "executed",
		Detail:   res.Outcome + ": " + res.Detail,
		Duration: finished.Sub(started), At: finished,
	}
}

func refuse(a domain.Action, rule, reason string, at time.Time) domain.ExecutionResult {
	return domain.ExecutionResult{
		ActionID: a.ID,
		Outcome:  "refused",
		Rule:     rule,
		Detail:   reason,
		At:       at,
	}
}

// Engine exposes the policy engine for gate evaluation at proposal time.
func (x *Executor) Engine() *Engine { return x.engine }
