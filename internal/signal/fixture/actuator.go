package fixture

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/zlrrr/mutil-agent-system/internal/catalog"
	"github.com/zlrrr/mutil-agent-system/internal/domain"
	"github.com/zlrrr/mutil-agent-system/internal/signal"
)

// sdd:impl DLD-1024

// Actuator is the simulated target system. It accepts only typed calls, records every
// invocation, and flips the environment to its recovered state when the fault case's
// recovery trigger matches — which is how verification can observe recovery without
// any real infrastructure.
type Actuator struct {
	fc catalog.FaultCase
	st *State
}

// Invoke applies a typed action call to the simulated environment.
func (a *Actuator) Invoke(_ context.Context, call domain.ActionCall) (signal.ActuationResult, error) {
	a.st.mu.Lock()
	a.st.calls = append(a.st.calls, call)
	a.st.mu.Unlock()

	switch call.Tool {
	case "set_config":
		service, key, value := call.Args["service"], call.Args["key"], call.Args["value"]
		if service == "" || key == "" {
			return signal.ActuationResult{}, fmt.Errorf("set_config requires service and key")
		}
		a.st.mu.Lock()
		prev := a.st.config[service+"/"+key]
		a.st.config[service+"/"+key] = value
		a.st.mu.Unlock()
		a.maybeRecover(call)
		return signal.ActuationResult{
			Outcome: "applied",
			Detail:  fmt.Sprintf("%s %s: %q -> %q", service, key, prev, value),
		}, nil

	case "restart_service":
		a.maybeRecover(call)
		return signal.ActuationResult{
			Outcome: "applied",
			Detail:  fmt.Sprintf("%s rolling restart completed", call.Args["service"]),
		}, nil

	case "scale_service":
		a.maybeRecover(call)
		return signal.ActuationResult{
			Outcome: "applied",
			Detail: fmt.Sprintf("%s scaled to %s replicas",
				call.Args["service"], call.Args["replicas"]),
		}, nil

	case "create_ticket":
		return signal.ActuationResult{
			Outcome: "created",
			Detail:  "ticket recorded: " + call.Args["title"],
		}, nil
	}
	return signal.ActuationResult{}, fmt.Errorf("actuator does not implement tool %q", call.Tool)
}

func (a *Actuator) maybeRecover(call domain.ActionCall) {
	if !a.fc.RecoveryTrigger.Matches(call) {
		return
	}
	a.st.mu.Lock()
	a.st.recovered = true
	a.st.mu.Unlock()
}

// RecordingActuator wraps an actuator and counts invocations, so a test can assert
// that the approval gate really did prevent execution (TC-0042).
type RecordingActuator struct {
	Inner signal.Actuator
	calls []domain.ActionCall
}

// Invoke records the call and delegates when an inner actuator is present.
func (r *RecordingActuator) Invoke(ctx context.Context, call domain.ActionCall) (signal.ActuationResult, error) {
	r.calls = append(r.calls, call)
	if r.Inner == nil {
		return signal.ActuationResult{Outcome: "applied", Detail: "recorded"}, nil
	}
	return r.Inner.Invoke(ctx, call)
}

// Calls returns the recorded invocations.
func (r *RecordingActuator) Calls() []domain.ActionCall { return r.calls }

// Count returns the number of recorded invocations.
func (r *RecordingActuator) Count() int { return len(r.calls) }

// Describe renders the simulated configuration for the console and for diagnostics.
func (s *State) Describe() string {
	cfg := s.Config()
	keys := make([]string, 0, len(cfg))
	for k := range cfg {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s=%s", k, cfg[k])
	}
	return b.String()
}
