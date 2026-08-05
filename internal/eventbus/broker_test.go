package eventbus

import (
	"sync"
	"testing"
	"time"

	"github.com/zlrrr/mutil-agent-system/internal/domain"
)

func event(seq int, caseID string) domain.Event {
	return domain.Event{Seq: seq, CaseID: caseID, Type: domain.EvStateChanged, Summary: "step"}
}

// sdd:verify TC-0064
func TestBrokerDeliversInOrder(t *testing.T) {
	b := New()
	ch, cancel := b.Subscribe("inc-1", 8)
	defer cancel()

	for i := 1; i <= 5; i++ {
		b.Publish(event(i, "inc-1"))
	}
	for i := 1; i <= 5; i++ {
		select {
		case e := <-ch:
			if e.Seq != i {
				t.Fatalf("received sequence %d, want %d", e.Seq, i)
			}
		case <-time.After(time.Second):
			t.Fatalf("event %d was never delivered", i)
		}
	}
}

// sdd:verify TC-0064
func TestBrokerFiltersByCase(t *testing.T) {
	b := New()
	ch, cancel := b.Subscribe("inc-1", 4)
	defer cancel()

	b.Publish(event(1, "inc-2"), event(1, "inc-1"))
	select {
	case e := <-ch:
		if e.CaseID != "inc-1" {
			t.Fatalf("received an event for %s", e.CaseID)
		}
	case <-time.After(time.Second):
		t.Fatal("the matching event was not delivered")
	}
}

// sdd:verify TC-0064
func TestBrokerNeverBlocksOnAFullSubscriber(t *testing.T) {
	b := New()
	_, cancel := b.Subscribe("inc-1", 1) // deliberately never drained
	defer cancel()

	done := make(chan struct{})
	go func() {
		for i := 1; i <= 500; i++ {
			b.Publish(event(i, "inc-1"))
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Publish blocked on a subscriber that stopped reading")
	}
	if got := b.Subscribers(); got != 0 {
		t.Errorf("the overflowing subscriber should have been dropped, %d remain", got)
	}
}

// sdd:verify TC-0064
func TestBrokerCancelIsIdempotent(t *testing.T) {
	b := New()
	ch, cancel := b.Subscribe("inc-1", 2)
	cancel()
	cancel() // a second cancel must not panic on an already-closed channel

	if _, open := <-ch; open {
		t.Error("the channel should be closed after cancel")
	}
	b.Publish(event(1, "inc-1")) // must not panic
	if got := b.Subscribers(); got != 0 {
		t.Errorf("subscribers = %d after cancel, want 0", got)
	}
}

// sdd:verify TC-0064
func TestBrokerIsConcurrencySafe(t *testing.T) {
	b := New()
	stop := make(chan struct{})
	var subs, pubs sync.WaitGroup

	for i := 0; i < 8; i++ {
		ch, cancel := b.Subscribe("inc-1", 16)
		subs.Add(1)
		go func(ch <-chan domain.Event, cancel func()) {
			defer subs.Done()
			defer cancel()
			for {
				select {
				case _, open := <-ch:
					if !open {
						return
					}
				case <-stop:
					return
				}
			}
		}(ch, cancel)
	}

	for i := 0; i < 8; i++ {
		pubs.Add(1)
		go func(base int) {
			defer pubs.Done()
			for j := 1; j <= 50; j++ {
				b.Publish(event(base*50+j, "inc-1"))
			}
		}(i)
	}

	published := make(chan struct{})
	go func() { pubs.Wait(); close(published) }()
	select {
	case <-published:
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent publishers did not finish; Publish is blocking")
	}

	close(stop)
	subs.Wait()
	if got := b.Subscribers(); got != 0 {
		t.Errorf("subscribers = %d after all cancels, want 0", got)
	}
}
