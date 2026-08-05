package store

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/zlrrr/mutil-agent-system/internal/domain"
)

// sdd:impl DLD-1010

// Memory is the in-process adapter used by tests and by the default demo profile.
type Memory struct {
	mu      sync.RWMutex
	headers map[string]CaseHeader
	order   []string
	events  map[string][]domain.Event
}

// NewMemory returns an empty in-memory store.
func NewMemory() *Memory {
	return &Memory{headers: map[string]CaseHeader{}, events: map[string][]domain.Event{}}
}

// CreateCase registers a new case, refusing a duplicate identifier.
func (m *Memory) CreateCase(_ context.Context, h CaseHeader) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.headers[h.ID]; ok {
		return fmt.Errorf("%w: %s", ErrCaseExists, h.ID)
	}
	m.headers[h.ID] = h
	m.order = append(m.order, h.ID)
	return nil
}

// ListCases returns headers newest first, breaking ties by identifier so the order is
// total and stable.
func (m *Memory) ListCases(_ context.Context) ([]CaseHeader, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]CaseHeader, 0, len(m.order))
	for _, id := range m.order {
		out = append(out, m.headers[id])
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// Header returns one case's index entry.
func (m *Memory) Header(_ context.Context, caseID string) (CaseHeader, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	h, ok := m.headers[caseID]
	if !ok {
		return CaseHeader{}, fmt.Errorf("%w: %s", ErrCaseNotFound, caseID)
	}
	return h, nil
}

// SetStatus updates the denormalised status on the index entry.
func (m *Memory) SetStatus(_ context.Context, caseID string, status domain.Status) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	h, ok := m.headers[caseID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrCaseNotFound, caseID)
	}
	h.Status = status
	m.headers[caseID] = h
	return nil
}

// Append writes a batch of events. Sequence numbers arrive already assigned by the
// orchestrator; a batch that does not continue the log is refused whole (REQ-0003).
func (m *Memory) Append(_ context.Context, caseID string, events []domain.Event) error {
	if len(events) == 0 {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.headers[caseID]; !ok {
		return fmt.Errorf("%w: %s", ErrCaseNotFound, caseID)
	}
	existing := m.events[caseID]
	if err := checkContiguous(existing, events); err != nil {
		return err
	}
	m.events[caseID] = append(existing, events...)
	return nil
}

// Events returns a copy of the log from a sequence number onward.
func (m *Memory) Events(_ context.Context, caseID string, fromSeq int) ([]domain.Event, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.headers[caseID]; !ok {
		return nil, fmt.Errorf("%w: %s", ErrCaseNotFound, caseID)
	}
	all := m.events[caseID]
	out := make([]domain.Event, 0, len(all))
	for _, e := range all {
		if e.Seq > fromSeq {
			out = append(out, e)
		}
	}
	return out, nil
}

// checkContiguous refuses a batch whose numbering does not continue the log exactly.
func checkContiguous(existing, batch []domain.Event) error {
	want := len(existing) + 1
	for _, e := range batch {
		if e.Seq != want {
			return fmt.Errorf("%w: got seq %d, want %d", ErrSequenceGap, e.Seq, want)
		}
		want++
	}
	return nil
}
