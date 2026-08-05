// Package agent binds a role, its ports and its reasoner into a pure function that
// returns proposed changes and nothing else (ARC-004).
package agent

import (
	"context"

	"github.com/zlrrr/mutil-agent-system/internal/domain"
)

// sdd:impl DLD-1040

// Agent is a role-bounded function of a case snapshot.
//
// An agent never mutates state, never writes an event and never touches an actuator.
// Its only effect is the contributions it returns, which the orchestrator validates,
// identifies and applies.
type Agent interface {
	Role() domain.Role
	Run(ctx context.Context, s domain.Snapshot) ([]domain.Contribution, error)
}

// funcAgent adapts a function to the Agent interface, so each role is written as a
// plain function rather than a type with one method.
type funcAgent struct {
	role domain.Role
	run  func(ctx context.Context, s domain.Snapshot) ([]domain.Contribution, error)
}

func (a funcAgent) Role() domain.Role { return a.role }

func (a funcAgent) Run(ctx context.Context, s domain.Snapshot) ([]domain.Contribution, error) {
	return a.run(ctx, s)
}

// New wraps a function as an agent of a role.
func New(role domain.Role, run func(ctx context.Context, s domain.Snapshot) ([]domain.Contribution, error)) Agent {
	return funcAgent{role: role, run: run}
}
