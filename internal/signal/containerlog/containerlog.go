// Package containerlog is the live adapter for the log port, reading container logs from
// a container runtime's HTTP API. Like the metric adapter it is an alternative to the
// fixture adapter, never a replacement (REQ-0098).
//
// The runtime is reached over its Unix socket with a custom transport, which is why no
// client library is needed and the module stays dependency-free (CON-008).
package containerlog

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/zlrrr/mutil-agent-system/internal/signal"
)

// sdd:impl DLD-1026

const portName = "log"

// Options configures the adapter.
type Options struct {
	// Timeout bounds a single request. Defaults to 10s.
	Timeout time.Duration
	// Client overrides the HTTP client, which is how tests point the adapter at a
	// local server instead of a runtime socket.
	Client *http.Client
	// Container maps a service name to a container name or id. Defaults to identity,
	// which is what compose's service naming already gives.
	Container func(service string) string
}

// Source implements signal.LogSource against a container runtime.
type Source struct {
	host   string
	base   string
	client *http.Client
	opts   Options
	bounds signal.Bounds
}

// New builds a log source for a runtime host. A "unix://" prefix dials the socket at the
// given path; anything else is used as an HTTP base URL.
func New(host string, bounds signal.Bounds, opts Options) *Source {
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Second
	}
	if opts.Container == nil {
		opts.Container = func(s string) string { return s }
	}

	base := host
	client := opts.Client
	if client == nil {
		if path, ok := strings.CutPrefix(host, "unix://"); ok {
			// The URL host is a placeholder: the transport ignores the address and
			// dials the socket, but net/http still requires a well-formed URL.
			base = "http://runtime"
			client = &http.Client{
				Timeout: opts.Timeout,
				Transport: &http.Transport{
					DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
						var d net.Dialer
						return d.DialContext(ctx, "unix", path)
					},
				},
			}
		} else {
			client = &http.Client{Timeout: opts.Timeout}
		}
	}
	return &Source{host: host, base: base, client: client, opts: opts, bounds: bounds}
}

// Search retrieves log lines matching the query's terms within its window.
func (s *Source) Search(ctx context.Context, q signal.LogQuery) ([]signal.LogLine, error) {
	container := s.opts.Container(q.Service)

	params := url.Values{}
	params.Set("stdout", "1")
	params.Set("stderr", "1")
	params.Set("timestamps", "1")
	if !q.Window.Start.IsZero() {
		params.Set("since", fmt.Sprint(q.Window.Start.UTC().Unix()))
	}
	if !q.Window.End.IsZero() {
		params.Set("until", fmt.Sprint(q.Window.End.UTC().Unix()))
	}

	target := fmt.Sprintf("%s/containers/%s/logs?%s", s.base, url.PathEscape(container), params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, s.fail(container, err)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, s.fail(container, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, s.fail(container, fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body))))
	}

	records, err := readFrames(resp.Body)
	if err != nil {
		return nil, s.fail(container, err)
	}

	var out []signal.LogLine
	for _, rec := range records {
		line, ok := parseRecord(rec, q.Service)
		if !ok {
			continue
		}
		if !q.Window.Start.IsZero() && !q.Window.Contains(line.At) {
			continue
		}
		if !matchesAll(line.Message, q.Terms) {
			continue
		}
		line.Message, _ = s.bounds.ApplyChars(line.Message)
		out = append(out, line)
	}

	out, truncated := s.bounds.ApplyRowsLog(out)
	if truncated && len(out) > 0 {
		// Marked in-band: the port contract says truncation is never silent, and a
		// log result has no envelope of its own to carry the flag.
		out[len(out)-1].Message += " [truncated]"
	}
	return out, nil
}

// record is one demultiplexed runtime log record.
type record struct {
	stream  byte
	payload string
}

// readFrames consumes the runtime's stream framing: an 8-byte header per record whose
// first byte is the stream type and whose last four are the payload length, big-endian.
//
// Reading this as lines would be wrong, not merely fragile: a payload whose own bytes
// contain the header pattern would desynchronise a line-oriented parser, and log content
// is exactly the place an attacker controls.
func readFrames(r io.Reader) ([]record, error) {
	br := bufio.NewReader(r)
	var out []record
	var header [8]byte

	for {
		if _, err := io.ReadFull(br, header[:]); err != nil {
			if err == io.EOF {
				return out, nil
			}
			if err == io.ErrUnexpectedEOF {
				// A partial header means a stream cut mid-record. What was read
				// already is still usable, so it is returned rather than discarded.
				return out, nil
			}
			return out, err
		}

		stream := header[0]
		if stream > 2 {
			return out, fmt.Errorf("unknown stream type %d; the frame header is not where it was expected", stream)
		}
		size := binary.BigEndian.Uint32(header[4:8])
		if size == 0 {
			continue
		}

		payload := make([]byte, size)
		if _, err := io.ReadFull(br, payload); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return out, nil
			}
			return out, err
		}
		out = append(out, record{stream: stream, payload: string(payload)})
	}
}

// parseRecord splits the leading RFC3339Nano timestamp from the message and assigns a
// level. A record without a parseable timestamp is dropped: a log line whose time is
// unknown cannot be placed in a window, and placing it wrongly is worse than losing it.
func parseRecord(rec record, service string) (signal.LogLine, bool) {
	text := strings.TrimRight(rec.payload, "\r\n")
	stamp, message, found := strings.Cut(text, " ")
	if !found {
		return signal.LogLine{}, false
	}
	at, err := time.Parse(time.RFC3339Nano, stamp)
	if err != nil {
		return signal.LogLine{}, false
	}

	level := "info"
	if rec.stream == 2 {
		level = "error"
	}
	if l, ok := levelFromMessage(message); ok {
		level = l
	}

	return signal.LogLine{
		At:      at.UTC(),
		Service: service,
		Level:   level,
		Message: strings.TrimSpace(message),
	}, true
}

// levelFromMessage recognises a level the message states about itself, which is more
// accurate than the stream it arrived on — plenty of runtimes send warnings to stderr.
func levelFromMessage(msg string) (string, bool) {
	upper := strings.ToUpper(msg)
	for _, level := range []string{"FATAL", "ERROR", "WARN", "INFO", "DEBUG"} {
		if strings.Contains(upper, level) {
			if level == "WARN" {
				return "warn", true
			}
			return strings.ToLower(level), true
		}
	}
	return "", false
}

func matchesAll(message string, terms []string) bool {
	lower := strings.ToLower(message)
	for _, term := range terms {
		if term == "" {
			continue
		}
		if !strings.Contains(lower, strings.ToLower(term)) {
			return false
		}
	}
	return true
}

func (s *Source) fail(container string, err error) error {
	return &signal.SourceError{Port: portName, Endpoint: s.host + " [" + container + "]", Err: err}
}
