// Package eventbus fans case events out to live subscribers without ever letting a
// subscriber stall an investigation (ARC-012).
package eventbus

import (
	"sort"
	"sync"

	"github.com/zlrrr/mutil-agent-system/internal/domain"
)

// sdd:impl DLD-1064

// DefaultBuffer is the per-subscriber channel depth.
const DefaultBuffer = 64

type subscription struct {
	id     uint64
	caseID string
	ch     chan domain.Event
	closed bool
}

// Broker delivers events to subscribers interested in one case.
type Broker struct {
	mu     sync.Mutex
	nextID uint64
	subs   map[uint64]*subscription
}

// New returns an empty broker.
func New() *Broker { return &Broker{subs: map[uint64]*subscription{}} }

// Subscribe registers interest in a case's events and returns the channel plus a
// cancel function. A buffer of zero uses DefaultBuffer.
func (b *Broker) Subscribe(caseID string, buffer int) (<-chan domain.Event, func()) {
	if buffer <= 0 {
		buffer = DefaultBuffer
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextID++
	s := &subscription{id: b.nextID, caseID: caseID, ch: make(chan domain.Event, buffer)}
	b.subs[s.id] = s
	id := s.id
	return s.ch, func() { b.cancel(id) }
}

func (b *Broker) cancel(id uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if s, ok := b.subs[id]; ok {
		if !s.closed {
			s.closed = true
			close(s.ch)
		}
		delete(b.subs, id)
	}
}

// Publish delivers events to matching subscribers. A subscriber whose buffer is full
// is dropped rather than waited on, so the orchestrator never blocks on a slow client.
// The dropped client reconnects and replays from its last sequence, so from its own
// point of view nothing is lost.
func (b *Broker) Publish(events ...domain.Event) {
	if len(events) == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	ids := make([]uint64, 0, len(b.subs))
	for id := range b.subs {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	for _, id := range ids {
		s := b.subs[id]
		if s.closed {
			continue
		}
		for _, e := range events {
			if s.caseID != "" && s.caseID != e.CaseID {
				continue
			}
			select {
			case s.ch <- e:
			default:
				s.closed = true
				close(s.ch)
				delete(b.subs, id)
			}
			if s.closed {
				break
			}
		}
	}
}

// Subscribers reports the number of live subscriptions, for tests and diagnostics.
func (b *Broker) Subscribers() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subs)
}
