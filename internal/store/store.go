// Package store persists cases and their event logs behind a port small enough that
// a database adapter remains a contained addition (ADR-004).
package store

import (
	"context"
	"errors"
	"time"

	"github.com/zlrrr/mutil-agent-system/internal/domain"
)

// sdd:impl DLD-1010

// CaseHeader is the index entry for a case: enough to list cases without folding
// their logs.
type CaseHeader struct {
	ID        string        `json:"id"`
	AlertName string        `json:"alert_name"`
	Service   string        `json:"service"`
	Severity  string        `json:"severity"`
	Mode      domain.Mode   `json:"mode"`
	Status    domain.Status `json:"status"`
	CreatedAt time.Time     `json:"created_at"`
}

// Store is the persistence port. It exposes appends and ordered range reads — the only
// access pattern the design has — and deliberately nothing a relational engine would be
// needed for.
type Store interface {
	CreateCase(ctx context.Context, h CaseHeader) error
	ListCases(ctx context.Context) ([]CaseHeader, error)
	Header(ctx context.Context, caseID string) (CaseHeader, error)
	SetStatus(ctx context.Context, caseID string, status domain.Status) error
	Append(ctx context.Context, caseID string, events []domain.Event) error
	Events(ctx context.Context, caseID string, fromSeq int) ([]domain.Event, error)
}

// Store errors, distinguishable by the caller.
var (
	ErrCaseNotFound = errors.New("case not found")
	ErrCaseExists   = errors.New("case already exists")
	ErrSequenceGap  = errors.New("event sequence gap")
)

// Load folds a case's whole log into a projection.
func Load(ctx context.Context, s Store, caseID string) (*domain.Case, error) {
	events, err := s.Events(ctx, caseID, 0)
	if err != nil {
		return nil, err
	}
	return domain.Replay(caseID, events)
}
