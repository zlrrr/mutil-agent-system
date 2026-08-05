// Package orchestrator decides what happens next and is the single writer of case
// state (ARC-004, ARC-007, ARC-011).
package orchestrator

import (
	"github.com/zlrrr/mutil-agent-system/internal/domain"
)

// sdd:impl DLD-1060

// transitions is the declared state graph. An edge that is not here does not exist:
// Legal consults this table and nothing else, so the control flow is readable in one
// place rather than distributed across handlers.
var transitions = map[domain.Status][]domain.Status{
	domain.StatusCreated:       {domain.StatusTriaging},
	domain.StatusTriaging:      {domain.StatusCollecting},
	domain.StatusCollecting:    {domain.StatusHypothesising},
	domain.StatusHypothesising: {domain.StatusCriticising},
	domain.StatusCriticising: {
		domain.StatusCollecting,  // unsatisfied demands and round budget remains
		domain.StatusHumanReview, // close call with the budget exhausted
		domain.StatusRemediating, // acceptance condition met
		domain.StatusReporting,   // no acceptable hypothesis and budget exhausted
	},
	domain.StatusHumanReview: {
		domain.StatusCollecting, domain.StatusRemediating, domain.StatusReporting,
	},
	domain.StatusRemediating: {
		domain.StatusAwaitingApproval, domain.StatusExecuting, domain.StatusReporting,
	},
	domain.StatusAwaitingApproval: {domain.StatusExecuting, domain.StatusReporting},
	domain.StatusExecuting:        {domain.StatusVerifying, domain.StatusReporting},
	domain.StatusVerifying:        {domain.StatusCollecting, domain.StatusReporting},
	domain.StatusReporting:        {domain.StatusClosed},
	domain.StatusClosed:           nil,
}

// Legal reports whether a transition is declared.
func Legal(from, to domain.Status) bool {
	for _, s := range transitions[from] {
		if s == to {
			return true
		}
	}
	return false
}

// Successors returns the states reachable from a state, for the console and for tests.
func Successors(from domain.Status) []domain.Status {
	out := make([]domain.Status, len(transitions[from]))
	copy(out, transitions[from])
	return out
}
