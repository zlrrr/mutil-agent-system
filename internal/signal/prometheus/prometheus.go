// Package prometheus is the live adapter for the metric port, querying the Prometheus
// HTTP API. It is an alternative to the fixture adapter, never a replacement: the
// offline path stays the default (REQ-0098).
//
// Only the standard library is used. The Prometheus API speaks JSON over HTTP, so a
// client library would buy nothing that encoding/json does not already provide, and
// would cost the dependency-free property the whole module rests on (CON-008).
package prometheus

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"

	"github.com/zlrrr/mutil-agent-system/internal/signal"
)

// sdd:impl DLD-1025

const portName = "metric"

// Options configures the adapter. The zero value is usable except for SeriesMap, without
// which the adapter can resolve no series and says so rather than guessing a query.
type Options struct {
	// Step is the resolution of a range query. Defaults to 15s.
	Step time.Duration
	// Timeout bounds a single request. Defaults to 10s.
	Timeout time.Duration
	// Client overrides the HTTP client, which is how tests point the adapter at a
	// local server without touching the network.
	Client *http.Client
	// SeriesMap maps a series name used by the catalog to the PromQL that produces it.
	// Adding a series is therefore configuration, not code.
	SeriesMap map[string]string
	// Units maps a series name to its unit, for the series the catalog labels.
	Units map[string]string
	// Capacities maps a series name to a declared capacity, so saturation stays
	// expressible rather than inferred.
	Capacities map[string]float64
}

// Source implements signal.MetricSource against a Prometheus server.
type Source struct {
	endpoint string
	client   *http.Client
	opts     Options
	bounds   signal.Bounds
}

// New builds a metric source for a Prometheus base URL, e.g. "http://prometheus:9090".
func New(endpoint string, bounds signal.Bounds, opts Options) *Source {
	if opts.Step <= 0 {
		opts.Step = 15 * time.Second
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Second
	}
	client := opts.Client
	if client == nil {
		client = &http.Client{Timeout: opts.Timeout}
	}
	return &Source{endpoint: endpoint, client: client, opts: opts, bounds: bounds}
}

// rangeResponse is the shape of /api/v1/query_range.
type rangeResponse struct {
	Status    string `json:"status"`
	ErrorType string `json:"errorType"`
	Error     string `json:"error"`
	Data      struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Values [][2]json.RawMessage
		} `json:"result"`
	} `json:"data"`
}

// labelResponse is the shape of /api/v1/label/__name__/values.
type labelResponse struct {
	Status    string   `json:"status"`
	ErrorType string   `json:"errorType"`
	Error     string   `json:"error"`
	Data      []string `json:"data"`
}

// Range retrieves one series over a window.
func (s *Source) Range(ctx context.Context, q signal.MetricQuery) (signal.Series, error) {
	promQL, ok := s.opts.SeriesMap[q.Series]
	if !ok {
		return signal.Series{}, s.fail(q.Series, fmt.Errorf("series %q has no configured query", q.Series))
	}

	params := url.Values{}
	params.Set("query", promQL)
	params.Set("start", formatTime(q.Window.Start))
	params.Set("end", formatTime(q.Window.End))
	params.Set("step", strconv.FormatFloat(s.opts.Step.Seconds(), 'f', -1, 64))

	var body rangeResponse
	if err := s.get(ctx, "/api/v1/query_range", params, &body); err != nil {
		return signal.Series{}, err
	}
	if body.Status != "success" {
		return signal.Series{}, s.fail(q.Series, fmt.Errorf("%s: %s", body.ErrorType, body.Error))
	}
	if len(body.Data.Result) == 0 {
		// No result is absence of evidence, not a zero reading. An empty series is
		// returned so the caller can record that it looked and found nothing.
		return signal.Series{Name: q.Series, Unit: s.opts.Units[q.Series]}, nil
	}

	first := body.Data.Result[0]
	points := make([]signal.Point, 0, len(first.Values))
	for _, v := range first.Values {
		p, ok := decodePoint(v)
		if !ok {
			// NaN, a stale marker or an unparseable value: the sample is absent. A
			// zero reading and no reading support different conclusions, so this
			// must not become 0.0.
			continue
		}
		points = append(points, p)
	}
	sort.Slice(points, func(i, j int) bool { return points[i].At.Before(points[j].At) })

	// Keep the earliest points. Onset detection and change correlation both read the
	// start of the window, so trimming the head would move a hypothesis's apparent
	// onset (DLD-1025).
	points, truncated := s.bounds.ApplyRowsPoints(points)

	labels := map[string]string{}
	for k, v := range first.Metric {
		labels[k] = v
	}
	if truncated {
		labels["truncated"] = "true"
	}

	return signal.Series{
		Name:     q.Series,
		Unit:     s.opts.Units[q.Series],
		Capacity: s.opts.Capacities[q.Series],
		Labels:   labels,
		Points:   points,
	}, nil
}

// SeriesNames returns the metric names the server exposes, narrowed to those the adapter
// can actually resolve — it never advertises a series it would then fail to serve.
func (s *Source) SeriesNames(ctx context.Context, _ string) ([]string, error) {
	var body labelResponse
	if err := s.get(ctx, "/api/v1/label/__name__/values", url.Values{}, &body); err != nil {
		return nil, err
	}
	if body.Status != "success" {
		return nil, s.fail("__name__", fmt.Errorf("%s: %s", body.ErrorType, body.Error))
	}

	available := make(map[string]bool, len(body.Data))
	for _, n := range body.Data {
		available[n] = true
	}
	var out []string
	for name, promQL := range s.opts.SeriesMap {
		// A mapped name is resolvable when the server knows the metric it selects.
		// The mapping may be an expression, so a direct hit is checked first and the
		// raw name second.
		if available[name] || available[promQL] {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out, nil
}

func (s *Source) get(ctx context.Context, path string, params url.Values, into any) error {
	target := s.endpoint + path
	if len(params) > 0 {
		target += "?" + params.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return s.fail(path, err)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return s.fail(path, err)
	}
	defer resp.Body.Close()

	// Prometheus reports query errors as 4xx with a JSON body, so the body is decoded
	// even on an error status and only an unreadable one is fatal here.
	if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
		return s.fail(path, fmt.Errorf("status %d: %w", resp.StatusCode, err))
	}
	return nil
}

func (s *Source) fail(what string, err error) error {
	return &signal.SourceError{Port: portName, Endpoint: s.endpoint + " [" + what + "]", Err: err}
}

// decodePoint reads one [timestamp, "value"] pair, reporting whether it is a real
// sample. Prometheus encodes the timestamp as a number and the value as a string.
func decodePoint(v [2]json.RawMessage) (signal.Point, bool) {
	var ts float64
	if err := json.Unmarshal(v[0], &ts); err != nil {
		return signal.Point{}, false
	}
	var raw string
	if err := json.Unmarshal(v[1], &raw); err != nil {
		return signal.Point{}, false
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return signal.Point{}, false
	}
	sec, frac := math.Modf(ts)
	return signal.Point{
		At:    time.Unix(int64(sec), int64(frac*1e9)).UTC(),
		Value: f,
	}, true
}

func formatTime(t time.Time) string {
	return strconv.FormatFloat(float64(t.UTC().UnixNano())/1e9, 'f', 3, 64)
}
