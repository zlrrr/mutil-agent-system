package signal

import (
	"strings"
	"testing"
	"time"

	"github.com/zlrrr/mutil-agent-system/internal/domain"
)

var base = time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)

// step builds a one-minute series that holds at from and switches to to at minute at.
func step(name, unit string, from, to float64, at int, capacity float64) Series {
	s := Series{Name: name, Unit: unit, Capacity: capacity}
	for i := -5; i < 25; i++ {
		v := from
		if i >= at {
			v = to
		}
		s.Points = append(s.Points, Point{At: base.Add(time.Duration(i) * time.Minute), Value: v})
	}
	return s
}

func window() domain.TimeWindow {
	return domain.TimeWindow{Start: base, End: base.Add(20 * time.Minute)}
}

// sdd:verify TC-0011
func TestAnomalyOnset(t *testing.T) {
	// The error rate steps from 0.2% to 18% seven minutes into the window.
	s := step("http_5xx_rate", "ratio", 0.002, 0.18, 7, 0)
	a := Analyse(s, window())

	if !a.Anomalous {
		t.Fatal("a 90x step must be detected as anomalous")
	}
	want := base.Add(7 * time.Minute)
	if diff := a.Onset.Sub(want); diff < -time.Minute || diff > time.Minute {
		t.Errorf("onset = %s, want within one sample of %s", a.Onset, want)
	}
	if a.Baseline != 0.002 {
		t.Errorf("baseline = %v, want 0.002 (the median of the pre-window samples)", a.Baseline)
	}
	if a.Peak != 0.18 {
		t.Errorf("peak = %v, want 0.18", a.Peak)
	}
	if a.Facts()["anomalous"] != "true" {
		t.Errorf("facts must report the anomaly: %v", a.Facts())
	}

	// A series that never moves must not be reported as anomalous.
	flat := step("db_up", "count", 1, 1, 0, 0)
	if Analyse(flat, window()).Anomalous {
		t.Error("a flat series must not be anomalous")
	}
}

// sdd:verify TC-0011
func TestAnomalyReportsSaturationAndNoCausalClaim(t *testing.T) {
	s := step("db_pool_saturation", "ratio", 0.20, 1.0, 6, 1.0)
	a := Analyse(s, window())

	if !a.Saturated {
		t.Error("a series reaching its declared capacity must be reported saturated")
	}
	if a.Facts()["saturated"] != "true" {
		t.Errorf("facts must carry saturation: %v", a.Facts())
	}

	// The metrics role reports what moved, never why (REQ-0011).
	causal := []string{"because", "caused", "due to", "led to", "results in", "root cause"}
	summary := strings.ToLower(a.Summary())
	for _, word := range causal {
		if strings.Contains(summary, word) {
			t.Errorf("metric summary contains the causal phrase %q: %s", word, summary)
		}
	}
}

// sdd:verify TC-0011
func TestCoMovementIsTimingNotCausation(t *testing.T) {
	w := window()
	a := Analyse(step("http_5xx_rate", "ratio", 0.002, 0.18, 7, 0), w)
	b := Analyse(step("http_request_duration_p99", "ms", 180, 2200, 7, 0), w)
	c := Analyse(step("http_requests_total", "rps", 120, 400, 3, 0), w)

	if !CoMoving(a, b) {
		t.Error("two series stepping in the same minute must be reported as co-moving")
	}
	if CoMoving(a, c) {
		t.Error("series four minutes apart must not be reported as co-moving")
	}
	flat := Analyse(step("db_up", "count", 1, 1, 0, 0), w)
	if CoMoving(a, flat) {
		t.Error("a non-anomalous series cannot co-move")
	}
}

// sdd:verify TC-0012
func TestLogClustering(t *testing.T) {
	var lines []LogLine
	for i := 0; i < 184; i++ {
		lines = append(lines, LogLine{
			At: base.Add(time.Duration(i) * time.Second), Service: "order-api", Level: "error",
			Message: "db connection timeout after 3000ms (attempt " + itoa(1000+i) + ")",
		})
	}
	for i := 0; i < 3; i++ {
		lines = append(lines, LogLine{
			At: base.Add(time.Duration(400+i) * time.Second), Service: "order-api", Level: "warn",
			Message: "pool exhausted, waiters=17",
		})
	}

	clusters := ClusterLines(lines)
	if len(clusters) != 2 {
		t.Fatalf("want 2 clusters, got %d: %+v", len(clusters), clusters)
	}
	if clusters[0].Count != 184 || clusters[1].Count != 3 {
		t.Errorf("counts = %d, %d; want 184 then 3", clusters[0].Count, clusters[1].Count)
	}
	if len(clusters[0].Samples) > LogSamplesPerCluster {
		t.Errorf("cluster kept %d samples, cap is %d", len(clusters[0].Samples), LogSamplesPerCluster)
	}
	if !clusters[0].FirstSeen.Equal(base) {
		t.Errorf("first seen = %s, want %s", clusters[0].FirstSeen, base)
	}
	if !strings.Contains(clusters[0].Template, "#") {
		t.Errorf("the template must normalise digits: %q", clusters[0].Template)
	}

	// The number of retained clusters is capped.
	var many []LogLine
	for i := 0; i < MaxClusters+4; i++ {
		many = append(many, LogLine{At: base, Service: "order-api",
			Message: "distinct template " + string(rune('a'+i))})
	}
	if got := len(ClusterLines(many)); got != MaxClusters {
		t.Errorf("retained %d clusters, cap is %d", got, MaxClusters)
	}
}

// sdd:verify TC-0012
func TestNormaliseCollapsesVariableParts(t *testing.T) {
	cases := [][2]string{
		{"timeout after 3000ms", "timeout after #ms"},
		{"timeout after 250ms", "timeout after #ms"},
		{`user "alice" not found`, `user "…" not found`},
		{`user "bob" not found`, `user "…" not found`},
	}
	for _, tc := range cases {
		if got := Normalise(tc[0]); got != tc[1] {
			t.Errorf("Normalise(%q) = %q, want %q", tc[0], got, tc[1])
		}
	}
	if Normalise("connection refused") == Normalise("connection timeout") {
		t.Error("normalisation must keep genuinely different messages apart")
	}
}

// sdd:verify TC-0016
func TestBounds(t *testing.T) {
	b := Bounds{MaxRows: 3, MaxChars: 10, MaxSpan: time.Hour}

	rows := make([]LogLine, 10)
	got, truncated := b.ApplyRowsLog(rows)
	if len(got) != 3 || !truncated {
		t.Errorf("ApplyRowsLog = %d rows, truncated=%v; want 3, true", len(got), truncated)
	}
	if _, truncated := b.ApplyRowsLog(rows[:2]); truncated {
		t.Error("a result within the bound must not be marked truncated")
	}

	s, truncated := b.ApplyChars("0123456789abcdef")
	if !truncated {
		t.Error("an over-long string must be marked truncated")
	}
	if len([]rune(s)) != 11 { // 10 runes plus the ellipsis
		t.Errorf("truncated string has %d runes: %q", len([]rune(s)), s)
	}
	if !strings.HasSuffix(s, "…") {
		t.Errorf("truncation must be visible in the value: %q", s)
	}

	wide := domain.TimeWindow{Start: base, End: base.Add(5 * time.Hour)}
	clamped, truncated := b.ApplySpan(wide)
	if !truncated || clamped.Duration() != time.Hour {
		t.Errorf("ApplySpan = %s (truncated=%v), want a one-hour window", clamped.Duration(), truncated)
	}
	if !clamped.End.Equal(wide.End) {
		t.Error("clamping must keep the window's end, which is the interesting edge")
	}

	points := make([]Point, 10)
	if got, truncated := b.ApplyRowsPoints(points); len(got) != 3 || !truncated {
		t.Errorf("ApplyRowsPoints = %d points, truncated=%v", len(got), truncated)
	}
	changes := make([]Change, 10)
	if got, truncated := b.ApplyRowsChanges(changes); len(got) != 3 || !truncated {
		t.Errorf("ApplyRowsChanges = %d changes, truncated=%v", len(got), truncated)
	}
}

// sdd:verify TC-0014
func TestTopologyNavigation(t *testing.T) {
	topo := Topology{
		Service: "order-api",
		Nodes: []ServiceNode{
			{Name: "order-api", Anomalous: true, FirstSeen: base},
			{Name: "checkout-web", Anomalous: true, FirstSeen: base.Add(2 * time.Minute)},
			{Name: "postgres", Anomalous: false},
		},
		Edges: []ServiceEdge{
			{From: "checkout-web", To: "order-api"},
			{From: "order-api", To: "postgres"},
		},
	}
	if got := topo.Upstreams("order-api"); len(got) != 1 || got[0] != "postgres" {
		t.Errorf("upstreams = %v, want [postgres]", got)
	}
	if got := topo.Downstreams("order-api"); len(got) != 1 || got[0] != "checkout-web" {
		t.Errorf("downstreams = %v, want [checkout-web]", got)
	}
	if n, ok := topo.Node("checkout-web"); !ok || !n.Anomalous {
		t.Error("checkout-web should be a known anomalous node")
	}
	if _, ok := topo.Node("absent"); ok {
		t.Error("an unknown node must not resolve")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
