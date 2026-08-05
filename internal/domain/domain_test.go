package domain

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func sampleEvidence() Evidence {
	return Evidence{
		ID: "e-metrics-001", Kind: KindMetric, Agent: RoleMetrics, Source: "metrics",
		Window:  TimeWindow{Start: time.Unix(0, 0).UTC(), End: time.Unix(600, 0).UTC()},
		Summary: "http_5xx_rate rose from 0.20% to 18.00%",
		RawRef:  "promql:http_5xx_rate", Confidence: 0.86,
		Facts: map[string]string{"series": "http_5xx_rate", "saturated": "false"},
	}
}

// sdd:verify TC-0002
func TestEvidenceValidation(t *testing.T) {
	base := sampleEvidence()

	cases := []struct {
		name string
		mut  func(*Evidence)
		want error
	}{
		{"missing raw reference", func(e *Evidence) { e.RawRef = "" }, ErrMissingRawRef},
		{"unknown kind", func(e *Evidence) { e.Kind = "hunch" }, ErrUnknownEvidenceKind},
		{"confidence above one", func(e *Evidence) { e.Confidence = 1.5 }, ErrConfidenceRange},
		{"confidence below zero", func(e *Evidence) { e.Confidence = -0.1 }, ErrConfidenceRange},
		{"missing source", func(e *Evidence) { e.Source = "" }, ErrMissingSource},
		{"missing summary", func(e *Evidence) { e.Summary = "  " }, ErrMissingSummary},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := base
			tc.mut(&e)
			if err := e.Validate(); !errors.Is(err, tc.want) {
				t.Errorf("Validate() = %v, want %v", err, tc.want)
			}
		})
	}
	if err := base.Validate(); err != nil {
		t.Errorf("valid evidence rejected: %v", err)
	}
}

// sdd:verify TC-0002
func TestEvidenceIsImmutableThroughTheProjection(t *testing.T) {
	c := NewCase("inc-test")
	ev := sampleEvidence()
	events := []Event{
		{Seq: 1, CaseID: "inc-test", Type: EvCaseCreated, At: time.Unix(0, 0).UTC(),
			Payload: MustPayload(map[string]any{"alert": Alert{Name: "A", Service: "s"}})},
		{Seq: 2, CaseID: "inc-test", Type: EvEvidenceAdded, At: time.Unix(1, 0).UTC(),
			Payload: MustPayload(ev)},
	}
	for _, e := range events {
		if err := c.Apply(e); err != nil {
			t.Fatalf("apply: %v", err)
		}
	}
	// Mutating the caller's copy must not reach stored state.
	ev.Summary = "tampered"
	ev.Facts["series"] = "tampered"

	got := c.Evidence[0]
	if got.Summary == "tampered" || got.Facts["series"] == "tampered" {
		t.Error("stored evidence was mutated through the caller's copy")
	}
	if got.RawRef != "promql:http_5xx_rate" {
		t.Errorf("raw reference = %q", got.RawRef)
	}
}

// sdd:verify TC-0004
func TestReplayRejectsSequenceGap(t *testing.T) {
	events := []Event{
		{Seq: 1, Type: EvCaseCreated, Payload: MustPayload(map[string]any{"alert": Alert{}})},
		{Seq: 3, Type: EvEvidenceAdded, Payload: MustPayload(sampleEvidence())},
	}
	if _, err := Replay("inc-test", events); err == nil {
		t.Fatal("replay must reject a sequence gap")
	} else if got := err.Error(); got == "" {
		t.Error("the error must name the gap")
	}
}

// sdd:verify TC-0050
func TestReplayEquality(t *testing.T) {
	live := NewCase("inc-test")
	events := buildLog(t)
	for _, e := range events {
		if err := live.Apply(e); err != nil {
			t.Fatalf("live apply seq %d: %v", e.Seq, err)
		}
	}

	replayed, err := Replay("inc-test", events)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}

	if !reflect.DeepEqual(live.Evidence, replayed.Evidence) {
		t.Error("replayed evidence differs from the live projection")
	}
	if !reflect.DeepEqual(live.Hypotheses, replayed.Hypotheses) {
		t.Errorf("replayed hypotheses differ:\n live=%+v\n replayed=%+v",
			live.Hypotheses, replayed.Hypotheses)
	}
	if !reflect.DeepEqual(live.Critiques, replayed.Critiques) {
		t.Error("replayed critiques differ from the live projection")
	}
	if live.Status != replayed.Status || live.Round != replayed.Round {
		t.Errorf("status/round differ: %s/%d vs %s/%d",
			live.Status, live.Round, replayed.Status, replayed.Round)
	}
	if live.LastSeq != replayed.LastSeq {
		t.Errorf("last sequence differs: %d vs %d", live.LastSeq, replayed.LastSeq)
	}
}

func buildLog(t *testing.T) []Event {
	t.Helper()
	at := time.Unix(0, 0).UTC()
	next := func() time.Time { at = at.Add(time.Second); return at }

	h := Hypothesis{
		ID: "h-analysis-001", SignatureID: "sig-x", Claim: "a claim",
		Mechanism: "a mechanism", Supporting: []string{"e-metrics-001"},
		Breakdown: ScoreBreakdown{Total: 0.8}, Status: HypothesisProposed,
	}
	return []Event{
		{Seq: 1, CaseID: "inc-test", Type: EvCaseCreated, At: next(),
			Payload: MustPayload(map[string]any{
				"alert": Alert{Name: "A", Service: "svc", StartsAt: at}, "mode": ModeMultiWithCritic})},
		{Seq: 2, CaseID: "inc-test", Type: EvStateChanged, At: next(),
			Payload: MustPayload(StateChange{From: StatusCreated, To: StatusCollecting})},
		{Seq: 3, CaseID: "inc-test", Type: EvRoundStarted, At: next(),
			Payload: MustPayload(RoundStart{Round: 1})},
		{Seq: 4, CaseID: "inc-test", Type: EvEvidenceAdded, At: next(),
			Payload: MustPayload(sampleEvidence())},
		{Seq: 5, CaseID: "inc-test", Type: EvHypothesisProposed, At: next(),
			Payload: MustPayload(h)},
		{Seq: 6, CaseID: "inc-test", Type: EvCritiqueRaised, At: next(),
			Payload: MustPayload(Critique{
				ID: "c-critic-001", HypothesisID: "h-analysis-001", Rule: "coverage_gap",
				Category: "coverage_gap", Challenge: "needs change evidence",
				Verdict: VerdictRevise})},
		{Seq: 7, CaseID: "inc-test", Type: EvStateChanged, At: next(),
			Payload: MustPayload(StateChange{From: StatusCollecting, To: StatusReporting})},
	}
}

// sdd:verify TC-0003
func TestRoleCapabilities(t *testing.T) {
	cases := []struct {
		role Role
		kind ContributionKind
		want bool
	}{
		{RoleMetrics, ContribAddEvidence, true},
		{RoleMetrics, ContribProposeHypothesis, false},
		{RoleLogs, ContribRaiseCritique, false},
		{RoleAnalysis, ContribProposeHypothesis, true},
		{RoleAnalysis, ContribRaiseCritique, false},
		{RoleCritic, ContribRaiseCritique, true},
		{RoleCritic, ContribDemandEvidence, true},
		{RoleCritic, ContribProposeHypothesis, false},
		{RoleCritic, ContribProposeAction, false},
		{RoleRemediation, ContribProposeAction, true},
		{RoleVerification, ContribRecordVerification, true},
		{Role("impostor"), ContribAddEvidence, false},
	}
	for _, tc := range cases {
		if got := tc.role.MayEmit(tc.kind); got != tc.want {
			t.Errorf("%s.MayEmit(%s) = %v, want %v", tc.role, tc.kind, got, tc.want)
		}
	}
}

// sdd:verify TC-0003
func TestContributionValidation(t *testing.T) {
	ev := sampleEvidence()
	h := Hypothesis{ID: "h1", Claim: "c", Mechanism: "m", Supporting: []string{"e1"}}

	t.Run("payload must match kind", func(t *testing.T) {
		c := Contribution{Kind: ContribAddEvidence, Hypothesis: &h}
		if err := c.Validate(); !errors.Is(err, ErrPayloadMismatch) {
			t.Errorf("Validate() = %v, want ErrPayloadMismatch", err)
		}
	})
	t.Run("exactly one payload", func(t *testing.T) {
		c := Contribution{Kind: ContribAddEvidence, Evidence: &ev, Hypothesis: &h}
		if err := c.Validate(); !errors.Is(err, ErrPayloadMismatch) {
			t.Errorf("Validate() = %v, want ErrPayloadMismatch", err)
		}
	})
	t.Run("delegates to the payload", func(t *testing.T) {
		bad := Hypothesis{ID: "h1", Claim: "c", Mechanism: "m"} // no supporting evidence
		c := Contribution{Kind: ContribProposeHypothesis, Hypothesis: &bad}
		if err := c.Validate(); !errors.Is(err, ErrNoSupportingEvidence) {
			t.Errorf("Validate() = %v, want ErrNoSupportingEvidence", err)
		}
	})
	t.Run("accepts a well-formed contribution", func(t *testing.T) {
		if err := AddEvidence(RoleMetrics, 0, ev).Validate(); err != nil {
			t.Errorf("valid contribution rejected: %v", err)
		}
	})
}

// sdd:verify TC-0021
func TestHypothesisRequiresEvidence(t *testing.T) {
	h := Hypothesis{ID: "h1", Claim: "claim", Mechanism: "mechanism"}
	if err := h.Validate(); !errors.Is(err, ErrNoSupportingEvidence) {
		t.Errorf("a hypothesis with no evidence must be invalid, got %v", err)
	}
	h.Supporting = []string{"e-metrics-001"}
	if err := h.Validate(); err != nil {
		t.Errorf("supported hypothesis rejected: %v", err)
	}
	h.Mechanism = ""
	if err := h.Validate(); !errors.Is(err, ErrMissingMechanism) {
		t.Errorf("a hypothesis without a mechanism must be invalid, got %v", err)
	}
}

// sdd:verify TC-0030
func TestVerdictCombination(t *testing.T) {
	cases := []struct {
		in   []Verdict
		want Verdict
	}{
		{nil, VerdictAccept},
		{[]Verdict{VerdictAccept}, VerdictAccept},
		{[]Verdict{VerdictAccept, VerdictRevise}, VerdictRevise},
		{[]Verdict{VerdictRevise, VerdictReject}, VerdictReject},
		{[]Verdict{VerdictAcceptWithRisk, VerdictAccept}, VerdictAcceptWithRisk},
		// An unset verdict means "not yet judged" and must not outrank a real one.
		{[]Verdict{"", VerdictRevise}, VerdictRevise},
		{[]Verdict{""}, VerdictAccept},
	}
	for _, tc := range cases {
		if got := CombineVerdicts(tc.in...); got != tc.want {
			t.Errorf("CombineVerdicts(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
	if VerdictReject.Permits() || VerdictRevise.Permits() {
		t.Error("revise and reject must not permit progression")
	}
	if !VerdictAccept.Permits() || !VerdictAcceptWithRisk.Permits() {
		t.Error("accept and accept_with_risk must permit progression")
	}
}

// sdd:verify TC-0040
func TestActionValidationAndFingerprint(t *testing.T) {
	a := Action{
		ID: "a1", Title: "Restore pool size", Tool: "set_config",
		Args: map[string]string{"service": "order-api", "key": "DB_POOL_SIZE", "value": "20"},
		Risk: RiskMedium,
	}
	if err := a.Validate(); !errors.Is(err, ErrMissingRollback) {
		t.Errorf("a medium-risk action without a rollback must be invalid, got %v", err)
	}
	a.Rollback = &ActionCall{Tool: "set_config", Args: map[string]string{"value": "2"}}
	if err := a.Validate(); !errors.Is(err, ErrMissingVerify) {
		t.Errorf("a medium-risk action without a verification must be invalid, got %v", err)
	}
	a.Verify = []string{"http_5xx_rate"}
	if err := a.Validate(); err != nil {
		t.Errorf("a complete action was rejected: %v", err)
	}

	before := a.Fingerprint()
	if before != a.Fingerprint() {
		t.Error("fingerprint must be stable for identical arguments")
	}
	a.Args["value"] = "200"
	if a.Fingerprint() == before {
		t.Error("fingerprint must change when an argument changes")
	}
}

// sdd:verify TC-0070
func TestDeterministicIdentifiers(t *testing.T) {
	alert := Alert{
		Name: "OrderApiHighErrorRate", Service: "order-api",
		StartsAt: time.Date(2026, 7, 26, 10, 7, 0, 0, time.UTC),
	}
	first := CaseID(alert)
	if first != CaseID(alert) {
		t.Error("case identifier must be stable for the same alert")
	}
	other := alert
	other.StartsAt = other.StartsAt.Add(time.Minute)
	if CaseID(other) == first {
		t.Error("a different alert must yield a different case identifier")
	}
	if got := NewID(PrefixEvidence, RoleMetrics, 7); got != "e-metrics-007" {
		t.Errorf("NewID = %q, want e-metrics-007", got)
	}
}

// sdd:verify TC-0074
func TestTimeWindow(t *testing.T) {
	start := time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)
	w := TimeWindow{Start: start, End: start.Add(20 * time.Minute)}

	if !w.Contains(start) || !w.Contains(w.End) {
		t.Error("the window must include both endpoints")
	}
	if w.Contains(start.Add(-time.Second)) {
		t.Error("the window must exclude times before its start")
	}
	ext := w.Extend(30 * time.Minute)
	if !ext.Start.Equal(start.Add(-30*time.Minute)) || !ext.End.Equal(w.End) {
		t.Errorf("Extend moved the wrong endpoint: %s", ext)
	}
}

// sdd:verify TC-0070
func TestLogicalClockIsDeterministic(t *testing.T) {
	start := time.Date(2026, 7, 26, 10, 7, 0, 0, time.UTC)
	a := NewLogicalClock(start, time.Second)
	b := NewLogicalClock(start, time.Second)
	for i := 0; i < 10; i++ {
		if !a.Now().Equal(b.Now()) {
			t.Fatalf("two logical clocks diverged at read %d", i)
		}
	}
	if got := NewLogicalClock(start, time.Second).Now(); !got.Equal(start) {
		t.Errorf("first read = %s, want the start time %s", got, start)
	}
}
