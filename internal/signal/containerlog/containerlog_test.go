package containerlog_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zlrrr/mutil-agent-system/internal/domain"
	"github.com/zlrrr/mutil-agent-system/internal/signal"
	"github.com/zlrrr/mutil-agent-system/internal/signal/containerlog"
)

var base = time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)

// frame writes one runtime log record: an 8-byte header then the payload.
func frame(stream byte, at time.Time, message string) []byte {
	payload := at.Format(time.RFC3339Nano) + " " + message + "\n"
	var buf bytes.Buffer
	header := make([]byte, 8)
	header[0] = stream
	binary.BigEndian.PutUint32(header[4:], uint32(len(payload)))
	buf.Write(header)
	buf.WriteString(payload)
	return buf.Bytes()
}

func serve(t *testing.T, stream []byte) (*httptest.Server, string) {
	t.Helper()
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path + "?" + r.URL.RawQuery
		w.Write(stream)
	}))
	t.Cleanup(srv.Close)
	_ = gotPath
	return srv, gotPath
}

// sdd:verify TC-0102
func TestContainerLogSearch(t *testing.T) {
	var stream bytes.Buffer
	stream.Write(frame(1, base.Add(1*time.Minute), "INFO handled request id=1"))
	stream.Write(frame(2, base.Add(2*time.Minute), "ERROR connection pool exhausted"))
	stream.Write(frame(1, base.Add(3*time.Minute), "INFO handled request id=2"))
	// One record spanning several lines, whose continuation begins with bytes that look
	// exactly like a frame header — a stack trace is the everyday version of this. A
	// line-oriented parser splits it and reads the continuation as a fresh record; a
	// framed one keeps it whole. The assertion below is what distinguishes the two.
	adversarial := "ERROR pool trace\n\x01\x00\x00\x00\x00\x00\x00\x20 embedded header bytes"
	stream.Write(frame(2, base.Add(4*time.Minute), adversarial))
	stream.Write(frame(1, base.Add(90*time.Minute), "INFO long after the window"))

	srv, _ := serve(t, stream.Bytes())

	src := containerlog.New(srv.URL, signal.DefaultBounds(), containerlog.Options{
		Client: srv.Client(),
	})

	t.Run("stdout and stderr are recovered with levels and timestamps", func(t *testing.T) {
		lines, err := src.Search(context.Background(), signal.LogQuery{
			Service: "order-api",
			Window:  domain.TimeWindow{Start: base, End: base.Add(10 * time.Minute)},
		})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if len(lines) != 4 {
			t.Fatalf("got %d lines, want 4 inside the window", len(lines))
		}
		if lines[0].Level != "info" || lines[1].Level != "error" {
			t.Errorf("levels wrong: %q then %q", lines[0].Level, lines[1].Level)
		}
		if !lines[0].At.Equal(base.Add(1 * time.Minute)) {
			t.Errorf("first timestamp = %s, want %s", lines[0].At, base.Add(time.Minute))
		}
		if lines[0].Service != "order-api" {
			t.Errorf("service = %q, want order-api", lines[0].Service)
		}
	})

	// The reason the adapter reads frames rather than lines.
	t.Run("an embedded header pattern does not desynchronise the parser", func(t *testing.T) {
		lines, err := src.Search(context.Background(), signal.LogQuery{
			Service: "order-api",
			Window:  domain.TimeWindow{Start: base, End: base.Add(10 * time.Minute)},
		})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		// Both halves must live in the same record. A line parser would emit the
		// continuation separately, where it would then be dropped for having no
		// parseable timestamp — silently losing the tail of every stack trace.
		var whole bool
		for _, l := range lines {
			if strings.Contains(l.Message, "pool trace") &&
				strings.Contains(l.Message, "embedded header bytes") {
				whole = true
			}
		}
		if !whole {
			t.Error("the multi-line record was split; its continuation was parsed as a separate record")
		}
	})

	t.Run("terms and window filter", func(t *testing.T) {
		lines, err := src.Search(context.Background(), signal.LogQuery{
			Service: "order-api",
			Terms:   []string{"pool"},
			Window:  domain.TimeWindow{Start: base, End: base.Add(10 * time.Minute)},
		})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if len(lines) != 2 {
			t.Fatalf("got %d lines, want the 2 mentioning the pool", len(lines))
		}
		for _, l := range lines {
			if !strings.Contains(strings.ToLower(l.Message), "pool") {
				t.Errorf("line does not match the term: %q", l.Message)
			}
			if l.At.After(base.Add(10 * time.Minute)) {
				t.Errorf("line outside the window survived: %s", l.At)
			}
		}
	})

	t.Run("every term must match, not any", func(t *testing.T) {
		lines, err := src.Search(context.Background(), signal.LogQuery{
			Service: "order-api",
			Terms:   []string{"pool", "exhausted"},
			Window:  domain.TimeWindow{Start: base, End: base.Add(10 * time.Minute)},
		})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if len(lines) != 1 {
			t.Fatalf("got %d lines, want only the one matching both terms", len(lines))
		}
	})

	t.Run("results are bounded and the truncation is marked", func(t *testing.T) {
		bounded := containerlog.New(srv.URL, signal.Bounds{MaxRows: 2, MaxChars: 8000}, containerlog.Options{
			Client: srv.Client(),
		})
		lines, err := bounded.Search(context.Background(), signal.LogQuery{
			Service: "order-api",
			Window:  domain.TimeWindow{Start: base, End: base.Add(10 * time.Minute)},
		})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if len(lines) != 2 {
			t.Fatalf("got %d lines, want the bound of 2", len(lines))
		}
		if !strings.Contains(lines[len(lines)-1].Message, "[truncated]") {
			t.Error("truncation was silent")
		}
	})

	t.Run("a runtime failure is a degraded source", func(t *testing.T) {
		dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"message":"no such container"}`))
		}))
		defer dead.Close()

		src := containerlog.New(dead.URL, signal.DefaultBounds(), containerlog.Options{Client: dead.Client()})
		_, err := src.Search(context.Background(), signal.LogQuery{Service: "missing"})
		if err == nil {
			t.Fatal("expected an error")
		}
		if !signal.Degraded(err) {
			t.Errorf("a collector cannot tell this is a degraded source: %v", err)
		}
	})
}
