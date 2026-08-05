package signal

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/zlrrr/mutil-agent-system/internal/domain"
)

// sdd:impl DLD-1021

// Anomaly detection constants, declared in the detailed design (DLD section 2).
const (
	AnomalyFactor  = 3.0
	AnomalySustain = 2
	CoMovementWin  = 60 * time.Second
	anomalyEpsilon = 1e-9
	baselineFloor  = 1e-6
)

// Analysis is what the metrics role learns about one series. It reports facts and an
// onset, never a causal relationship between series (REQ-0011).
type Analysis struct {
	Series    string
	Baseline  float64
	Peak      float64
	Onset     time.Time
	Anomalous bool
	Saturated bool
	Capacity  float64
	Ratio     float64
	HasRatio  bool
	Unit      string
}

// Analyse determines whether a series is anomalous within a window and, if so, when it
// started.
//
//	baseline  = median of the samples strictly before the window
//	threshold = max(baseline * AnomalyFactor, baseline + epsilon)
//	onset     = first sample from which AnomalySustain consecutive samples exceed it
func Analyse(s Series, w domain.TimeWindow) Analysis {
	a := Analysis{Series: s.Name, Capacity: s.Capacity, Unit: s.Unit}

	var pre []float64
	var in []Point
	for _, p := range s.Points {
		switch {
		case p.At.Before(w.Start):
			pre = append(pre, p.Value)
		case w.Contains(p.At):
			in = append(in, p)
		}
	}
	a.Baseline = median(pre)
	if len(in) == 0 {
		return a
	}

	a.Peak = in[0].Value
	for _, p := range in {
		if p.Value > a.Peak {
			a.Peak = p.Value
		}
	}
	if s.Capacity > 0 && a.Peak >= s.Capacity-anomalyEpsilon {
		a.Saturated = true
	}
	if a.Baseline > baselineFloor {
		a.Ratio = a.Peak / a.Baseline
		a.HasRatio = true
	}

	threshold := math.Max(a.Baseline*AnomalyFactor, a.Baseline+anomalyEpsilon)
	if a.Baseline <= baselineFloor {
		// With no meaningful baseline, any non-zero sustained value is the onset.
		threshold = anomalyEpsilon
	}
	for i := range in {
		if !exceedsFrom(in, i, threshold, AnomalySustain) {
			continue
		}
		a.Onset = in[i].At
		a.Anomalous = true
		break
	}
	// A saturated series is anomalous even when its baseline already sat at capacity.
	if a.Saturated && !a.Anomalous {
		a.Onset = in[0].At
		a.Anomalous = true
	}
	return a
}

func exceedsFrom(pts []Point, i int, threshold float64, sustain int) bool {
	if i+sustain > len(pts) {
		// Near the end of the series, accept a shorter run so a step in the final
		// samples is still detected.
		sustain = len(pts) - i
	}
	if sustain <= 0 {
		return false
	}
	for k := 0; k < sustain; k++ {
		if pts[i+k].Value <= threshold {
			return false
		}
	}
	return true
}

func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	mid := len(s) / 2
	if len(s)%2 == 1 {
		return s[mid]
	}
	return (s[mid-1] + s[mid]) / 2
}

// CoMoving reports whether two anomalous series began within CoMovementWin of each
// other. It is a statement about timing, not about causation.
func CoMoving(a, b Analysis) bool {
	if !a.Anomalous || !b.Anomalous {
		return false
	}
	d := a.Onset.Sub(b.Onset)
	if d < 0 {
		d = -d
	}
	return d <= CoMovementWin
}

// Facts renders the analysis as evidence facts, in a form the signature matcher and
// the console both read.
func (a Analysis) Facts() map[string]string {
	f := map[string]string{
		"series":    a.Series,
		"baseline":  formatValue(a.Baseline, a.Unit),
		"peak":      formatValue(a.Peak, a.Unit),
		"anomalous": boolString(a.Anomalous),
		"saturated": boolString(a.Saturated),
	}
	if a.Anomalous {
		f["onset"] = a.Onset.UTC().Format(time.RFC3339)
	}
	if a.HasRatio {
		f["ratio"] = fmt.Sprintf("%.1fx", a.Ratio)
	} else {
		f["ratio"] = "n/a"
	}
	if a.Capacity > 0 {
		f["capacity"] = formatValue(a.Capacity, a.Unit)
	}
	return f
}

// Summary renders a one-line description of the series behaviour. It deliberately
// contains no causal verb: the metrics role reports what moved, not why.
func (a Analysis) Summary() string {
	if !a.Anomalous {
		return fmt.Sprintf("%s stayed near its baseline of %s during the window",
			a.Series, formatValue(a.Baseline, a.Unit))
	}
	s := fmt.Sprintf("%s rose from a baseline of %s to a peak of %s starting at %s",
		a.Series, formatValue(a.Baseline, a.Unit), formatValue(a.Peak, a.Unit),
		a.Onset.UTC().Format("15:04:05"))
	if a.Saturated {
		s += fmt.Sprintf("; it reached its declared capacity of %s",
			formatValue(a.Capacity, a.Unit))
	}
	return s
}

func formatValue(v float64, unit string) string {
	switch unit {
	case "ratio":
		return fmt.Sprintf("%.2f%%", v*100)
	case "ms":
		return fmt.Sprintf("%.0fms", v)
	case "rps":
		return fmt.Sprintf("%.0f rps", v)
	case "count", "":
		if v == math.Trunc(v) {
			return fmt.Sprintf("%.0f", v)
		}
		return fmt.Sprintf("%.2f", v)
	default:
		return fmt.Sprintf("%.2f %s", v, unit)
	}
}

func boolString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
