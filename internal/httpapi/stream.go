package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/zlrrr/mutil-agent-system/internal/eventbus"
)

// sdd:impl DLD-1072

// streamEvents serves the live event feed as server-sent events.
//
// It replays from the client's last sequence before subscribing, so a client that
// reconnects sees every event exactly once and in order — the loss the broker's
// drop-on-full policy would otherwise cause is repaired here, on the client's terms
// (ARC-012).
func (s *Server) streamEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "", "streaming is not supported by this writer")
		return
	}
	id := r.PathValue("id")

	from := intQuery(r, "from", 0)
	if last := r.Header.Get("Last-Event-ID"); last != "" {
		if n, err := strconv.Atoi(last); err == nil {
			from = n
		}
	}

	// Subscribe before replaying, so an event appended between the two is buffered
	// rather than missed. Duplicates are filtered by sequence number below.
	ch, cancel := s.engine.Bus().Subscribe(id, eventbus.DefaultBuffer)
	defer cancel()

	backlog, err := s.engine.Store().Events(r.Context(), id, from)
	if err != nil {
		s.writeStoreError(w, err)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	highest := from
	for _, e := range backlog {
		if e.Seq <= highest {
			continue
		}
		if !writeSSE(w, e.Seq, e) {
			return
		}
		highest = e.Seq
	}
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case e, open := <-ch:
			if !open {
				return
			}
			if e.Seq <= highest {
				continue // already delivered from the backlog
			}
			if !writeSSE(w, e.Seq, e) {
				return
			}
			highest = e.Seq
			flusher.Flush()
		}
	}
}

func writeSSE(w http.ResponseWriter, seq int, v any) bool {
	raw, err := json.Marshal(v)
	if err != nil {
		return false
	}
	if _, err := fmt.Fprintf(w, "id: %d\nevent: case\ndata: %s\n\n", seq, raw); err != nil {
		return false
	}
	return true
}
