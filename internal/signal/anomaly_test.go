package signal

import (
	"strings"
	"testing"
	"time"

	"github.com/zlrrr/mutil-agent-system/internal/domain"
)

// sdd:verify TC-0109
func TestCollapseIsAnomalous(t *testing.T) {
	start := time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC)
	at := func(i int) time.Time { return start.Add(time.Duration(i) * time.Minute) }

	// The baseline is taken from samples preceding the window, so the series must carry
	// a pre-window run: `pre` values sit before the incident, `values` inside it.
	build := func(name string, pre []float64, values ...float64) Series {
		pts := make([]Point, 0, len(pre)+len(values))
		for i, v := range pre {
			pts = append(pts, Point{At: at(i - len(pre)), Value: v})
		}
		for i, v := range values {
			pts = append(pts, Point{At: at(i), Value: v})
		}
		return Series{Name: name, Unit: "count", Points: pts}
	}
	steady := []float64{1, 1, 1, 1, 1}
	window := domain.TimeWindow{Start: start, End: at(11)}

	t.Run("an availability gauge falling to zero is anomalous", func(t *testing.T) {
		a := Analyse(build("db_up", steady, 1, 1, 1, 1, 1, 1, 0, 0, 0, 0, 0, 0), window)

		if !a.Anomalous {
			t.Fatal("a gauge that fell to zero and stayed there was not reported as anomalous")
		}
		if !a.Collapsed {
			t.Error("the fall was not recorded as a collapse")
		}
		if !a.Onset.Equal(at(6)) {
			t.Errorf("onset = %s, want %s", a.Onset.UTC(), at(6).UTC())
		}
		if a.Facts()["collapsed"] != "true" {
			t.Errorf("facts do not carry the collapse: %v", a.Facts())
		}
		// The summary is the text a reviewer reads. Describing a collapse as a rise
		// would be a false report of what the data shows.
		if strings.Contains(a.Summary(), "rose") {
			t.Errorf("summary describes a collapse as a rise: %q", a.Summary())
		}
		if !strings.Contains(a.Summary(), "fell") {
			t.Errorf("summary does not say the series fell: %q", a.Summary())
		}
	})

	t.Run("a throughput series falling far below baseline is anomalous", func(t *testing.T) {
		a := Analyse(build("http_requests_total", []float64{400, 405, 398, 402, 400},
			400, 410, 395, 405, 400, 12, 8, 10, 9, 11, 10, 9), window)

		if !a.Collapsed {
			t.Error("a series that fell to 2% of its baseline was not reported as collapsed")
		}
		if !a.Onset.Equal(at(5)) {
			t.Errorf("onset = %s, want %s", a.Onset.UTC(), at(5).UTC())
		}
	})

	// Otherwise every noisy sample becomes a finding.
	t.Run("a single dipping sample is not a collapse", func(t *testing.T) {
		a := Analyse(build("db_up", steady, 1, 1, 1, 1, 1, 0, 1, 1, 1, 1, 1, 1), window)
		if a.Collapsed {
			t.Error("a one-sample dip was reported as a collapse")
		}
	})

	t.Run("a series already at zero has not fallen", func(t *testing.T) {
		a := Analyse(build("db_up", []float64{0, 0, 0, 0, 0},
			0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0), window)
		if a.Collapsed {
			t.Error("a series that was always zero was reported as having collapsed")
		}
	})
}
