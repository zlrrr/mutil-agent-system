// Package reasoner decides how an agent reaches a conclusion, behind a port whose
// default adapter is deterministic and offline (ARC-005, ADR-002).
package reasoner

import (
	"context"
	"time"

	"github.com/zlrrr/mutil-agent-system/internal/domain"
)

// sdd:impl DLD-1033

// Config carries the tunable constants declared in the detailed design. Keeping them
// in one struct means changing a threshold is a configuration edit that `sddctl drift`
// can cascade from.
type Config struct {
	MaxRounds             int
	MaxHypothesesPerRound int
	MaxDemandsPerRound    int
	AcceptThreshold       float64
	CloseCallMargin       float64
	MinEvidenceKinds      int
	ChangeLookback        time.Duration
	Weights               Weights
	CounterPenalty        float64
}

// DefaultConfig returns the values declared in DLD section 2.
func DefaultConfig() Config {
	return Config{
		MaxRounds:             3,
		MaxHypothesesPerRound: 3,
		MaxDemandsPerRound:    4,
		AcceptThreshold:       0.75,
		CloseCallMargin:       0.15,
		MinEvidenceKinds:      2,
		ChangeLookback:        30 * time.Minute,
		Weights:               DefaultWeights(),
		CounterPenalty:        0.20,
	}
}

// Reasoner forms hypotheses and critiques them. Swapping the adapter changes no
// contract, no state transition and no stored schema.
type Reasoner interface {
	Hypothesise(ctx context.Context, s domain.Snapshot) ([]domain.Hypothesis, error)
	Critique(ctx context.Context, s domain.Snapshot) ([]domain.Critique, []domain.EvidenceDemand, error)
}
