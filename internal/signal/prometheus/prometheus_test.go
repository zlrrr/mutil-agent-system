package prometheus_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zlrrr/mutil-agent-system/internal/domain"
	"github.com/zlrrr/mutil-agent-system/internal/signal"
	"github.com/zlrrr/mutil-agent-system/internal/signal/prometheus"
)

func window() domain.TimeWindow {
	start := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	return domain.TimeWindow{Start: start, End: start.Add(10 * time.Minute)}
}

func opts(client *http.Client) prometheus.Options {
	return prometheus.Options{
		Client:     client,
		Step:       30 * time.Second,
		SeriesMap:  map[string]string{"http_error_rate": `rate(http_errors_total[1m])`},
		Units:      map[string]string{"http_error_rate": "ratio"},
		Capacities: map[string]float64{"http_error_rate": 1},
	}
}

// sdd:verify TC-0100
func TestPrometheusRange(t *testing.T) {
	var gotQuery, gotStart, gotEnd, gotStep string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("query")
		gotStart = r.URL.Query().Get("start")
		gotEnd = r.URL.Query().Get("end")
		gotStep = r.URL.Query().Get("step")
		w.Header().Set("Content-Type", "application/json")
		// Deliberately out of order, and carrying a NaN, to prove both are handled.
		fmt.Fprint(w, `{"status":"success","data":{"resultType":"matrix","result":[
			{"metric":{"service":"order-api"},"values":[
				[1772359260,"0.42"],
				[1772359200,"0.10"],
				[1772359320,"NaN"],
				[1772359380,"0.55"]
			]}]}}`)
	}))
	defer srv.Close()

	src := prometheus.New(srv.URL, signal.DefaultBounds(), opts(srv.Client()))
	series, err := src.Range(context.Background(), signal.MetricQuery{
		Service: "order-api", Series: "http_error_rate", Window: window(),
	})
	if err != nil {
		t.Fatalf("range: %v", err)
	}

	t.Run("the request carries the query and the window", func(t *testing.T) {
		if gotQuery != `rate(http_errors_total[1m])` {
			t.Errorf("query = %q, want the mapped PromQL", gotQuery)
		}
		for name, got := range map[string]string{"start": gotStart, "end": gotEnd, "step": gotStep} {
			if got == "" {
				t.Errorf("the request sent no %s", name)
			}
		}
		if gotStep != "30" {
			t.Errorf("step = %q, want 30", gotStep)
		}
	})

	t.Run("points are ordered by time with values parsed", func(t *testing.T) {
		if len(series.Points) != 3 {
			t.Fatalf("got %d points, want 3 (the NaN is not a sample)", len(series.Points))
		}
		for i := 1; i < len(series.Points); i++ {
			if !series.Points[i-1].At.Before(series.Points[i].At) {
				t.Errorf("point %d is not after point %d", i, i-1)
			}
		}
		if series.Points[0].Value != 0.10 {
			t.Errorf("first value = %v, want 0.10", series.Points[0].Value)
		}
	})

	// A zero reading and no reading support different conclusions, so this is the
	// assertion that matters most in this test.
	t.Run("a NaN sample is absent rather than zero", func(t *testing.T) {
		for _, p := range series.Points {
			if p.Value == 0 {
				t.Errorf("a NaN became a 0.0 reading at %s", p.At)
			}
		}
	})

	t.Run("unit, capacity and labels are carried", func(t *testing.T) {
		if series.Unit != "ratio" {
			t.Errorf("unit = %q, want ratio", series.Unit)
		}
		if series.Capacity != 1 {
			t.Errorf("capacity = %v, want 1", series.Capacity)
		}
		if series.Labels["service"] != "order-api" {
			t.Errorf("labels lost the series identity: %v", series.Labels)
		}
	})
}

// sdd:verify TC-0101
func TestPrometheusBoundsAndErrors(t *testing.T) {
	t.Run("an over-long series truncates and reports it", func(t *testing.T) {
		const total = 60
		var b strings.Builder
		b.WriteString(`{"status":"success","data":{"resultType":"matrix","result":[{"metric":{},"values":[`)
		for i := 0; i < total; i++ {
			if i > 0 {
				b.WriteString(",")
			}
			fmt.Fprintf(&b, `[%d,"%d"]`, 1772359200+i*30, i)
		}
		b.WriteString(`]}]}}`)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, b.String())
		}))
		defer srv.Close()

		bounds := signal.Bounds{MaxRows: 10, MaxChars: 8000, MaxSpan: 2 * time.Hour}
		src := prometheus.New(srv.URL, bounds, opts(srv.Client()))
		series, err := src.Range(context.Background(), signal.MetricQuery{
			Service: "order-api", Series: "http_error_rate", Window: window(),
		})
		if err != nil {
			t.Fatalf("range: %v", err)
		}
		if len(series.Points) != 10 {
			t.Fatalf("got %d points, want the bound of 10", len(series.Points))
		}
		if series.Labels["truncated"] != "true" {
			t.Error("truncation was silent; a reader would think the series was complete")
		}
		// The head is what onset detection reads, so it is the end that must survive.
		if series.Points[0].Value != 0 {
			t.Errorf("first retained value = %v, want the earliest sample", series.Points[0].Value)
		}
	})

	t.Run("an API error surfaces as a typed source error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"status":"error","errorType":"bad_data","error":"parse error"}`)
		}))
		defer srv.Close()

		src := prometheus.New(srv.URL, signal.DefaultBounds(), opts(srv.Client()))
		_, err := src.Range(context.Background(), signal.MetricQuery{
			Service: "order-api", Series: "http_error_rate", Window: window(),
		})
		assertDegraded(t, err, "parse error")
	})

	t.Run("an unreachable endpoint is degraded, not a panic", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		client := srv.Client()
		endpoint := srv.URL
		srv.Close() // the endpoint is now dead

		src := prometheus.New(endpoint, signal.DefaultBounds(), opts(client))
		_, err := src.Range(context.Background(), signal.MetricQuery{
			Service: "order-api", Series: "http_error_rate", Window: window(),
		})
		assertDegraded(t, err, "")
	})

	t.Run("an unmapped series is refused rather than guessed", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Error("the adapter reached the server for a series it cannot resolve")
		}))
		defer srv.Close()

		src := prometheus.New(srv.URL, signal.DefaultBounds(), opts(srv.Client()))
		_, err := src.Range(context.Background(), signal.MetricQuery{
			Service: "order-api", Series: "not_configured", Window: window(),
		})
		assertDegraded(t, err, "no configured query")
	})
}

func assertDegraded(t *testing.T, err error, contains string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error")
	}
	if !signal.Degraded(err) {
		t.Errorf("error is not a signal.SourceError, so a collector cannot tell this is a degraded source: %v", err)
	}
	var se *signal.SourceError
	if errors.As(err, &se) && se.Port != "metric" {
		t.Errorf("port = %q, want metric", se.Port)
	}
	if contains != "" && !strings.Contains(err.Error(), contains) {
		t.Errorf("error %q does not mention %q", err, contains)
	}
}
