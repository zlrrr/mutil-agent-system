package store

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/zlrrr/mutil-agent-system/internal/domain"
)

func header(id string) CaseHeader {
	return CaseHeader{
		ID: id, AlertName: "OrderApiHighErrorRate", Service: "order-api",
		Severity: "P1", Mode: domain.ModeMultiWithCritic, Status: domain.StatusCreated,
		CreatedAt: time.Unix(1000, 0).UTC(),
	}
}

func log(caseID string, n int) []domain.Event {
	at := time.Unix(1000, 0).UTC()
	out := make([]domain.Event, 0, n)
	for i := 1; i <= n; i++ {
		e := domain.Event{
			Seq: i, CaseID: caseID, Actor: domain.RoleOrchestrator,
			Type: domain.EvStateChanged, Summary: "step", At: at.Add(time.Duration(i) * time.Second),
			Payload: domain.MustPayload(domain.StateChange{
				From: domain.StatusCreated, To: domain.StatusTriaging}),
		}
		if i == 1 {
			e.Type = domain.EvCaseCreated
			e.Payload = domain.MustPayload(map[string]any{
				"alert": domain.Alert{Name: "OrderApiHighErrorRate", Service: "order-api"},
				"mode":  domain.ModeMultiWithCritic,
			})
		}
		out = append(out, e)
	}
	return out
}

// sdd:verify TC-0004
func TestSequenceGapRejected(t *testing.T) {
	for _, tc := range []struct {
		name  string
		build func(t *testing.T) Store
	}{
		{"memory", func(t *testing.T) Store { return NewMemory() }},
		{"file", func(t *testing.T) Store {
			s, err := NewFile(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			return s
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			s := tc.build(t)
			if err := s.CreateCase(ctx, header("inc-1")); err != nil {
				t.Fatal(err)
			}

			gapped := log("inc-1", 2)
			gapped[0].Seq = 2 // the log must start at 1
			if err := s.Append(ctx, "inc-1", gapped); !errors.Is(err, ErrSequenceGap) {
				t.Fatalf("Append with a gap = %v, want ErrSequenceGap", err)
			}
			if got, _ := s.Events(ctx, "inc-1", 0); len(got) != 0 {
				t.Errorf("a refused batch must not be partially written, got %d event(s)", len(got))
			}

			if err := s.Append(ctx, "inc-1", log("inc-1", 3)); err != nil {
				t.Fatalf("Append: %v", err)
			}
			next := log("inc-1", 5)[3:] // sequences 4 and 5 continue the log
			if err := s.Append(ctx, "inc-1", next); err != nil {
				t.Fatalf("continuing Append: %v", err)
			}
			got, err := s.Events(ctx, "inc-1", 0)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 5 {
				t.Fatalf("want 5 events, got %d", len(got))
			}
			for i, e := range got {
				if e.Seq != i+1 {
					t.Errorf("event %d has sequence %d", i, e.Seq)
				}
			}
		})
	}
}

// sdd:verify TC-0004
func TestUnknownCaseIsReported(t *testing.T) {
	ctx := context.Background()
	s := NewMemory()
	if _, err := s.Events(ctx, "inc-missing", 0); !errors.Is(err, ErrCaseNotFound) {
		t.Errorf("Events on an unknown case = %v, want ErrCaseNotFound", err)
	}
	if err := s.CreateCase(ctx, header("inc-1")); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateCase(ctx, header("inc-1")); !errors.Is(err, ErrCaseExists) {
		t.Errorf("duplicate CreateCase = %v, want ErrCaseExists", err)
	}
}

// sdd:verify TC-0005
func TestFileStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	first, err := NewFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.CreateCase(ctx, header("inc-round-trip")); err != nil {
		t.Fatal(err)
	}
	events := log("inc-round-trip", 4)
	if err := first.Append(ctx, "inc-round-trip", events); err != nil {
		t.Fatal(err)
	}
	before, err := Load(ctx, first, "inc-round-trip")
	if err != nil {
		t.Fatal(err)
	}

	// Discard the instance entirely: a new store must rebuild from the directory.
	second, err := NewFile(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	headers, err := second.ListCases(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(headers) != 1 || headers[0].ID != "inc-round-trip" {
		t.Fatalf("index was not rebuilt: %+v", headers)
	}
	if headers[0].AlertName != "OrderApiHighErrorRate" {
		t.Errorf("header was not recovered from the log: %+v", headers[0])
	}

	after, err := Load(ctx, second, "inc-round-trip")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before.Timeline, after.Timeline) {
		t.Error("the replayed projection differs after a restart")
	}
	if after.LastSeq != 4 {
		t.Errorf("last sequence after restart = %d, want 4", after.LastSeq)
	}

	// The log is inspectable on disk, which is the point of the format (ADR-004).
	if _, err := filepath.Glob(filepath.Join(dir, "cases", "*.jsonl")); err != nil {
		t.Fatal(err)
	}
}

// sdd:verify TC-0005
func TestListCasesIsOrdered(t *testing.T) {
	ctx := context.Background()
	s := NewMemory()
	for i, id := range []string{"inc-b", "inc-a", "inc-c"} {
		h := header(id)
		h.CreatedAt = time.Unix(int64(1000+i), 0).UTC()
		if err := s.CreateCase(ctx, h); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.ListCases(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Newest first; the order must be total, so repeated calls agree.
	want := []string{"inc-c", "inc-a", "inc-b"}
	for i, h := range got {
		if h.ID != want[i] {
			t.Fatalf("order = %v, want %v", ids(got), want)
		}
	}
	again, _ := s.ListCases(ctx)
	if !reflect.DeepEqual(ids(got), ids(again)) {
		t.Error("ListCases order is not stable across calls")
	}
}

func ids(hs []CaseHeader) []string {
	out := make([]string, len(hs))
	for i, h := range hs {
		out[i] = h.ID
	}
	return out
}
