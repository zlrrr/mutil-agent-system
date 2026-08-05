package policy

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/zlrrr/mutil-agent-system/internal/domain"
	"github.com/zlrrr/mutil-agent-system/internal/signal"
)

func restoreAction() domain.Action {
	return domain.Action{
		ID: "a-remediation-001", Title: "Restore the database connection pool size",
		Tool: "set_config",
		Args: map[string]string{"service": "order-api", "key": "DB_POOL_SIZE", "value": "20"},
		Risk: domain.RiskMedium,
		Rollback: &domain.ActionCall{Tool: "set_config", Args: map[string]string{
			"service": "order-api", "key": "DB_POOL_SIZE", "value": "2"}},
		Verify: []string{"http_5xx_rate"},
	}
}

func approved(a domain.Action) domain.Action {
	fp := a.Fingerprint()
	a.Approval = &domain.ApprovalDecision{
		Decision: "approved", By: "demo-operator", At: time.Unix(1000, 0).UTC(), Fingerprint: fp,
	}
	return a
}

// countingActuator records invocations so a test can assert that nothing was touched.
type countingActuator struct{ calls []domain.ActionCall }

func (c *countingActuator) Invoke(_ context.Context, call domain.ActionCall) (signal.ActuationResult, error) {
	c.calls = append(c.calls, call)
	return signal.ActuationResult{Outcome: "applied", Detail: "recorded"}, nil
}

// sdd:verify TC-0041
func TestPolicyDenies(t *testing.T) {
	e := New(DefaultConfig())

	cases := []struct {
		name     string
		mutate   func(*domain.Action)
		wantRule string
	}{
		{"unlisted service", func(a *domain.Action) { a.Args["service"] = "billing-api" }, "service_allowlist"},
		{"no service named", func(a *domain.Action) { delete(a.Args, "service") }, "service_allowlist"},
		{"unlisted tool", func(a *domain.Action) { a.Tool = "drop_database" }, "tool_allowlist"},
		{"unlisted configuration key", func(a *domain.Action) { a.Args["key"] = "SECRET_TOKEN" }, "key_allowlist"},
		{"value below range", func(a *domain.Action) { a.Args["value"] = "0" }, "value_range"},
		{"value above range", func(a *domain.Action) { a.Args["value"] = "5000" }, "value_range"},
		{"non-numeric value", func(a *domain.Action) { a.Args["value"] = "twenty" }, "value_range"},
		{"shell tool", func(a *domain.Action) { a.Tool = "shell" }, "forbidden_category"},
		{"exec tool", func(a *domain.Action) { a.Tool = "exec" }, "forbidden_category"},
		{"sql tool", func(a *domain.Action) { a.Tool = "sql" }, "forbidden_category"},
		{"resource deletion", func(a *domain.Action) { a.Tool = "delete_resource" }, "forbidden_category"},
		{"cross-service bulk restart", func(a *domain.Action) { a.Tool = "bulk_restart" }, "forbidden_category"},
		{"shell metacharacter in an argument",
			func(a *domain.Action) { a.Args["value"] = "20; rm -rf /" }, "forbidden_category"},
		{"pipe in an argument",
			func(a *domain.Action) { a.Args["value"] = "20 | tee /etc/passwd" }, "forbidden_category"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := restoreAction()
			tc.mutate(&a)
			d := e.Evaluate(a)
			if d.Allowed {
				t.Fatalf("action was allowed: %+v", a)
			}
			if d.Rule != tc.wantRule {
				t.Errorf("rule = %q, want %q (reason: %s)", d.Rule, tc.wantRule, d.Reason)
			}
			if strings.TrimSpace(d.Reason) == "" {
				t.Error("a denial must explain itself")
			}
		})
	}
}

// sdd:verify TC-0041
func TestCategoricalDenialsIgnoreApproval(t *testing.T) {
	e := New(DefaultConfig())
	for _, tool := range []string{"shell", "exec", "sql", "delete_resource", "bulk_restart"} {
		a := approved(restoreAction())
		a.Tool = tool
		if d := e.Evaluate(a); d.Allowed {
			t.Errorf("%s was allowed despite being a forbidden category", tool)
		} else if d.Rule != "forbidden_category" {
			t.Errorf("%s denied by %q, want forbidden_category", tool, d.Rule)
		}
	}

	// A permissive configuration must not be able to enable them either.
	permissive := New(Config{
		AllowedServices: []string{"order-api"},
		AllowedTools:    []string{"shell", "sql", "delete_resource", "bulk_restart"},
		AllowedKeys:     []string{"DB_POOL_SIZE"},
	})
	a := approved(restoreAction())
	a.Tool = "shell"
	if d := permissive.Evaluate(a); d.Allowed {
		t.Error("configuration must not be able to allowlist a forbidden category")
	}
}

// sdd:verify TC-0042
func TestRiskGate(t *testing.T) {
	e := New(DefaultConfig())

	medium := e.Evaluate(restoreAction())
	if !medium.Allowed {
		t.Fatalf("the reference action should be permitted: %+v", medium)
	}
	if !medium.RequiresApproval {
		t.Error("a medium-risk action must require approval")
	}

	low := restoreAction()
	low.Risk = domain.RiskLow
	if d := e.Evaluate(low); !d.Allowed || d.RequiresApproval {
		t.Errorf("a low-risk permitted action should not require approval: %+v", d)
	}

	high := restoreAction()
	high.Risk = domain.RiskHigh
	if d := e.Evaluate(high); !d.RequiresApproval {
		t.Error("a high-risk action must require approval")
	}
}

// sdd:verify TC-0043
func TestExecutorRechecksPolicy(t *testing.T) {
	clock := domain.NewLogicalClock(time.Unix(1000, 0).UTC(), time.Second)

	t.Run("refuses without an approval", func(t *testing.T) {
		act := &countingActuator{}
		x := NewExecutor(New(DefaultConfig()), act, clock)
		res := x.Execute(context.Background(), restoreAction())
		if res.Outcome != "refused" || res.Rule != "approval_required" {
			t.Errorf("result = %+v, want a refusal for a missing approval", res)
		}
		if len(act.calls) != 0 {
			t.Errorf("the actuator was invoked %d time(s) without approval", len(act.calls))
		}
	})

	t.Run("refuses a rejected approval", func(t *testing.T) {
		act := &countingActuator{}
		x := NewExecutor(New(DefaultConfig()), act, clock)
		a := restoreAction()
		a.Approval = &domain.ApprovalDecision{Decision: "rejected", By: "operator"}
		res := x.Execute(context.Background(), a)
		if res.Outcome != "refused" || res.Rule != "approval_rejected" {
			t.Errorf("result = %+v, want a refusal for a rejected approval", res)
		}
		if len(act.calls) != 0 {
			t.Error("a rejected action reached the actuator")
		}
	})

	t.Run("refuses an action altered after approval", func(t *testing.T) {
		act := &countingActuator{}
		x := NewExecutor(New(DefaultConfig()), act, clock)
		a := approved(restoreAction())
		a.Args["value"] = "200" // tampered after the operator saw it
		res := x.Execute(context.Background(), a)
		if res.Outcome != "refused" || res.Rule != "tampered_after_approval" {
			t.Errorf("result = %+v, want tampered_after_approval", res)
		}
		if len(act.calls) != 0 {
			t.Error("a tampered action reached the actuator")
		}
	})

	t.Run("refuses when the policy tightened after approval", func(t *testing.T) {
		act := &countingActuator{}
		tightened := New(Config{
			AllowedServices: []string{"payment-api"}, // order-api is no longer permitted
			AllowedTools:    []string{"set_config"},
			AllowedKeys:     []string{"DB_POOL_SIZE"},
		})
		x := NewExecutor(tightened, act, clock)
		res := x.Execute(context.Background(), approved(restoreAction()))
		if res.Outcome != "refused" || res.Rule != "service_allowlist" {
			t.Errorf("result = %+v, want a refusal from the re-evaluation", res)
		}
		if len(act.calls) != 0 {
			t.Error("the action ran despite the policy having tightened")
		}
	})

	t.Run("executes an approved, permitted action", func(t *testing.T) {
		act := &countingActuator{}
		x := NewExecutor(New(DefaultConfig()), act, clock)
		res := x.Execute(context.Background(), approved(restoreAction()))
		if res.Outcome != "executed" {
			t.Fatalf("result = %+v, want executed", res)
		}
		if len(act.calls) != 1 {
			t.Fatalf("the actuator was invoked %d time(s), want 1", len(act.calls))
		}
		call := act.calls[0]
		if call.Tool != "set_config" || call.Args["value"] != "20" {
			t.Errorf("the actuator received %+v", call)
		}
	})

	t.Run("a low-risk action needs no approval", func(t *testing.T) {
		act := &countingActuator{}
		x := NewExecutor(New(DefaultConfig()), act, clock)
		a := restoreAction()
		a.Risk = domain.RiskLow
		if res := x.Execute(context.Background(), a); res.Outcome != "executed" {
			t.Errorf("result = %+v, want executed", res)
		}
	})
}
