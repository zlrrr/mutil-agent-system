package orchestrator_test

import (
	"context"
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/zlrrr/mutil-agent-system/internal/arena"
	"github.com/zlrrr/mutil-agent-system/internal/catalog"
	"github.com/zlrrr/mutil-agent-system/internal/domain"
	"github.com/zlrrr/mutil-agent-system/internal/reasoner"
	"github.com/zlrrr/mutil-agent-system/internal/report"
	"github.com/zlrrr/mutil-agent-system/internal/signal/fixture"
)

// build assembles the reference scenario over fresh, isolated state.
func build(t *testing.T, opts ...func(*arena.Params)) *arena.Build {
	t.Helper()
	p := arena.Params{CaseID: "C1"}
	for _, o := range opts {
		o(&p)
	}
	b, err := arena.NewFixtureBuild(p)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return b
}

// runToHalt creates and advances a case until it terminates or awaits approval.
func runToHalt(t *testing.T, b *arena.Build, mode domain.Mode) *domain.Case {
	t.Helper()
	ctx := context.Background()
	c, err := b.Engine.Create(ctx, b.Alert(), mode)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	c, err = b.Engine.Run(ctx, c.ID)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return c
}

func approve(t *testing.T, b *arena.Build, c *domain.Case) *domain.Case {
	t.Helper()
	a, ok := c.PendingAction()
	if !ok {
		t.Fatalf("case %s has no action awaiting approval (status %s)", c.ID, c.Status)
	}
	out, err := b.Engine.Decide(context.Background(), c.ID, a.ID, domain.ApprovalDecision{
		Decision: "approved", By: "demo-operator", Comment: "restore the pool size",
	})
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	return out
}

// sdd:verify TC-0010
func TestReferenceScenarioOffline(t *testing.T) {
	b := build(t)
	c := approve(t, b, runToHalt(t, b, domain.ModeMultiWithCritic))

	if c.Status != domain.StatusClosed {
		t.Fatalf("status = %s, want closed", c.Status)
	}
	kinds := domain.DistinctKinds(c.Evidence)
	want := []domain.EvidenceKind{
		domain.KindMetric, domain.KindLog, domain.KindChange,
		domain.KindTopology, domain.KindKnowledge, domain.KindVerification,
	}
	for _, k := range want {
		var found bool
		for _, got := range kinds {
			if got == k {
				found = true
			}
		}
		if !found {
			t.Errorf("no %s evidence was collected; kinds = %v", k, kinds)
		}
	}
	lead, ok := c.Leading()
	if !ok {
		t.Fatal("no leading hypothesis")
	}
	if lead.SignatureID != b.Case.ExpectedSignature {
		t.Errorf("top hypothesis = %s, want %s", lead.SignatureID, b.Case.ExpectedSignature)
	}
	if !lead.Verdict.Permits() {
		t.Errorf("the accepted hypothesis carries verdict %q", lead.Verdict)
	}
}

// sdd:verify TC-0031
func TestCriticForcesAnotherRound(t *testing.T) {
	b := build(t)
	c := runToHalt(t, b, domain.ModeMultiWithCritic)

	if c.Round < 2 {
		t.Fatalf("the critic did not force a second round (round = %d)", c.Round)
	}
	if len(c.Demands) == 0 {
		t.Fatal("the critic demanded no evidence")
	}
	for _, d := range c.Demands {
		if !d.Satisfied() {
			t.Errorf("demand %q went unanswered", d.Descriptor)
			continue
		}
		e, ok := c.EvidenceIndex()[d.SatisfiedBy]
		if !ok {
			t.Errorf("demand %q points at evidence %q that does not exist", d.Descriptor, d.SatisfiedBy)
			continue
		}
		if e.Fact("demand") != d.Descriptor {
			t.Errorf("evidence %s does not record the demand it answers", e.ID)
		}
	}

	// The critic changed the outcome, not merely the commentary.
	var reordered bool
	var roundOneLeader string
	for _, ev := range c.Timeline {
		if ev.Type == domain.EvHypothesisProposed && roundOneLeader == "" {
			roundOneLeader = ev.Ref
		}
	}
	lead, _ := c.Leading()
	if roundOneLeader != "" && roundOneLeader != lead.ID {
		reordered = true
	}
	if !reordered {
		t.Error("the ranking after critique is the same as the first hypothesis proposed; " +
			"the adversarial round changed nothing")
	}
	if c.CriticCorrections == 0 {
		t.Error("no hypothesis was corrected by the critic")
	}
}

// sdd:verify TC-0031
func TestUnmetDemandsAreReportedNotDropped(t *testing.T) {
	// C1 with a one-round budget: the demands cannot be answered before the budget
	// runs out, and the case must say so rather than proceed silently.
	b := build(t, func(p *arena.Params) {
		cfg := reasoner.DefaultConfig()
		cfg.MaxRounds = 1
		p.Config = cfg
	})
	c := runToHalt(t, b, domain.ModeMultiWithCritic)

	if c.Round != 1 {
		t.Errorf("round = %d, want exactly the one-round budget", c.Round)
	}
	unmet := c.UnsatisfiedDemands()
	if len(unmet) == 0 {
		t.Fatal("with a one-round budget the demands cannot have been answered")
	}
	md := report.Markdown(c)
	if !strings.Contains(md, "Unmet evidence demands") {
		t.Error("the report has no unmet-demand section")
	}
	for _, d := range unmet {
		if !strings.Contains(md, d.Descriptor) {
			t.Errorf("the report does not mention the unmet demand %q", d.Descriptor)
		}
	}
	var noted bool
	for _, n := range c.Notes {
		if strings.Contains(n, "unmet evidence demand") {
			noted = true
		}
	}
	if !noted {
		t.Errorf("the case does not record its unmet demands: %v", c.Notes)
	}
}

// sdd:verify TC-0042
func TestApprovalGateHalts(t *testing.T) {
	recorder := &fixture.RecordingActuator{}
	b := build(t, func(p *arena.Params) { p.Actuator = recorder })
	c := runToHalt(t, b, domain.ModeMultiWithCritic)

	if c.Status != domain.StatusAwaitingApproval {
		t.Fatalf("status = %s, want awaiting_approval", c.Status)
	}
	if recorder.Count() != 0 {
		t.Errorf("the actuator was invoked %d time(s) before approval: %+v",
			recorder.Count(), recorder.Calls())
	}
	a, ok := c.PendingAction()
	if !ok {
		t.Fatal("no action is pending approval")
	}
	if a.Risk == domain.RiskLow {
		t.Error("a low-risk action should not have halted the machine")
	}
	var requested bool
	for _, e := range c.Timeline {
		if e.Type == domain.EvApprovalRequested && e.Ref == a.ID {
			requested = true
		}
	}
	if !requested {
		t.Error("no approval_requested event was recorded")
	}
}

// sdd:verify TC-0046
func TestRejectedApprovalReports(t *testing.T) {
	recorder := &fixture.RecordingActuator{}
	b := build(t, func(p *arena.Params) { p.Actuator = recorder })
	c := runToHalt(t, b, domain.ModeMultiWithCritic)

	a, ok := c.PendingAction()
	if !ok {
		t.Fatal("no action is pending approval")
	}
	c, err := b.Engine.Decide(context.Background(), c.ID, a.ID, domain.ApprovalDecision{
		Decision: "rejected", By: "demo-operator", Comment: "we will schedule a change window",
	})
	if err != nil {
		t.Fatalf("decide: %v", err)
	}

	if c.Status != domain.StatusClosed {
		t.Errorf("status = %s, want closed: a rejection still produces a report", c.Status)
	}
	if recorder.Count() != 0 {
		t.Errorf("the actuator ran %d time(s) after a rejection", recorder.Count())
	}
	md := report.Markdown(c)
	if !strings.Contains(md, "rejected") {
		t.Error("the report does not record the rejection")
	}
	if !strings.Contains(md, a.Title) {
		t.Error("the report does not record the un-executed action")
	}
}

// sdd:verify TC-0045
func TestAuditTrail(t *testing.T) {
	b := build(t)
	c := approve(t, b, runToHalt(t, b, domain.ModeMultiWithCritic))

	var approvalSeen, executionSeen bool
	for _, a := range c.Actions {
		if a.Approval != nil {
			approvalSeen = true
			if a.Approval.By != "demo-operator" || a.Approval.Decision != "approved" {
				t.Errorf("approval record = %+v", a.Approval)
			}
			if a.Approval.At.IsZero() {
				t.Error("the approval carries no timestamp")
			}
			if a.Approval.Fingerprint == "" {
				t.Error("the approval is not bound to the action it approved")
			}
		}
		if a.Execution != nil && a.Execution.Outcome == "executed" {
			executionSeen = true
			if a.Execution.Detail == "" {
				t.Error("the execution record carries no detail")
			}
		}
	}
	if !approvalSeen || !executionSeen {
		t.Fatalf("audit incomplete: approval=%v execution=%v", approvalSeen, executionSeen)
	}

	md := report.Markdown(c)
	for _, want := range []string{"demo-operator", "approved", "executed"} {
		if !strings.Contains(md, want) {
			t.Errorf("the report omits %q from the audit trail", want)
		}
	}
}

// sdd:verify TC-0051
func TestVerificationAfterRemediation(t *testing.T) {
	b := build(t)
	c := runToHalt(t, b, domain.ModeMultiWithCritic)

	if len(c.Verifications) != 0 {
		t.Fatal("verification ran before the action was approved")
	}
	if b.State.Recovered() {
		t.Fatal("the environment reports recovery before anything was done to it")
	}

	c = approve(t, b, c)

	if len(c.Verifications) == 0 {
		t.Fatal("no verification was recorded after the action ran")
	}
	recovered, any := c.Recovered()
	if !any || !recovered {
		t.Errorf("verification did not observe recovery: %+v", c.Verifications)
	}
	for _, v := range c.Verifications {
		if v.Before == "" || v.After == "" {
			t.Errorf("verification for %s lacks a before or after value: %+v", v.Signal, v)
		}
		if v.ActionID == "" {
			t.Errorf("verification for %s is not bound to an action", v.Signal)
		}
	}
	var kindSeen bool
	for _, e := range c.Evidence {
		if e.Kind == domain.KindVerification {
			kindSeen = true
		}
	}
	if !kindSeen {
		t.Error("verification results were not stored as evidence")
	}
}

// sdd:verify TC-0062
func TestEventCompleteness(t *testing.T) {
	b := build(t)
	c := approve(t, b, runToHalt(t, b, domain.ModeMultiWithCritic))

	events, err := b.Engine.Store().Events(context.Background(), c.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 {
		t.Fatal("no events were recorded")
	}
	for i, e := range events {
		if e.Seq != i+1 {
			t.Fatalf("event %d has sequence %d; the log must be contiguous from 1", i, e.Seq)
		}
		if e.CaseID != c.ID {
			t.Errorf("event %d belongs to case %s", e.Seq, e.CaseID)
		}
		if e.At.IsZero() {
			t.Errorf("event %d has no timestamp", e.Seq)
		}
		if e.Actor == "" {
			t.Errorf("event %d has no actor", e.Seq)
		}
	}

	seen := map[domain.EventType]int{}
	for _, e := range events {
		seen[e.Type]++
	}
	for _, want := range []domain.EventType{
		domain.EvCaseCreated, domain.EvStateChanged, domain.EvRoundStarted,
		domain.EvAgentStarted, domain.EvAgentCompleted, domain.EvEvidenceAdded,
		domain.EvHypothesisProposed, domain.EvCritiqueRaised, domain.EvEvidenceDemanded,
		domain.EvDemandSatisfied, domain.EvActionProposed, domain.EvApprovalRequested,
		domain.EvApprovalRecorded, domain.EvActionExecuted, domain.EvVerificationRecorded,
		domain.EvReportGenerated, domain.EvCaseClosed,
	} {
		if seen[want] == 0 {
			t.Errorf("no %s event was recorded", want)
		}
	}

	// Every state the case entered is represented by a transition event.
	var transitions int
	for _, e := range events {
		if e.Type == domain.EvStateChanged {
			transitions++
		}
	}
	if transitions < 8 {
		t.Errorf("only %d state transitions were recorded for a full run", transitions)
	}

	// Work events carry a duration.
	for _, e := range events {
		if e.Type == domain.EvAgentCompleted && e.Duration == 0 {
			t.Errorf("agent_completed at sequence %d carries no duration", e.Seq)
		}
	}
}

// sdd:verify TC-0050
func TestReplayReproducesTheCaseAndReport(t *testing.T) {
	b := build(t)
	live := approve(t, b, runToHalt(t, b, domain.ModeMultiWithCritic))

	events, err := b.Engine.Store().Events(context.Background(), live.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := domain.Replay(live.ID, events)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}

	if !reflect.DeepEqual(live.Evidence, replayed.Evidence) {
		t.Error("replayed evidence differs from the live projection")
	}
	if !reflect.DeepEqual(live.Hypotheses, replayed.Hypotheses) {
		t.Error("replayed hypotheses differ from the live projection")
	}
	if !reflect.DeepEqual(live.Actions, replayed.Actions) {
		t.Error("replayed actions differ from the live projection")
	}
	if !reflect.DeepEqual(live.Verifications, replayed.Verifications) {
		t.Error("replayed verifications differ from the live projection")
	}
	if live.Status != replayed.Status {
		t.Errorf("status: live %s, replayed %s", live.Status, replayed.Status)
	}

	if a, bb := report.Markdown(live), report.Markdown(replayed); a != bb {
		t.Errorf("the regenerated report differs from the live report (%d vs %d bytes)",
			len(a), len(bb))
	}
}

// sdd:verify TC-0070
func TestDeterministicRun(t *testing.T) {
	first := build(t)
	second := build(t)
	a := approve(t, first, runToHalt(t, first, domain.ModeMultiWithCritic))
	bb := approve(t, second, runToHalt(t, second, domain.ModeMultiWithCritic))

	if a.ID != bb.ID {
		t.Fatalf("case identifiers differ: %s vs %s", a.ID, bb.ID)
	}
	if len(a.Evidence) != len(bb.Evidence) {
		t.Fatalf("evidence counts differ: %d vs %d", len(a.Evidence), len(bb.Evidence))
	}
	for i := range a.Evidence {
		if a.Evidence[i].ID != bb.Evidence[i].ID {
			t.Fatalf("evidence %d: %s vs %s", i, a.Evidence[i].ID, bb.Evidence[i].ID)
		}
	}
	for i := range a.Hypotheses {
		if a.Hypotheses[i].ID != bb.Hypotheses[i].ID ||
			a.Hypotheses[i].Breakdown.Total != bb.Hypotheses[i].Breakdown.Total {
			t.Fatalf("hypothesis %d differs: %+v vs %+v", i, a.Hypotheses[i], bb.Hypotheses[i])
		}
	}
	if report.Markdown(a) != report.Markdown(bb) {
		t.Error("two runs of the same case produced different reports")
	}
}

// sdd:verify TC-0070
func TestDeterministicAcrossProcesses(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping the subprocess comparison in short mode")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain is not on PATH")
	}
	run := func() string {
		cmd := exec.Command("go", "run", "../../cmd/arena", "demo", "--case", "C1", "--quiet")
		cmd.Env = append(os.Environ(), "GOFLAGS=")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("arena demo failed: %v\n%s", err, out)
		}
		return string(out)
	}
	if a, b := run(), run(); a != b {
		t.Errorf("two separate processes produced different reports (%d vs %d bytes)",
			len(a), len(b))
	}
}

// sdd:verify TC-0080
func TestThreeModes(t *testing.T) {
	results := map[domain.Mode]*domain.Case{}
	for _, mode := range []domain.Mode{
		domain.ModeSingle, domain.ModeMultiNoCritic, domain.ModeMultiWithCritic,
	} {
		b := build(t)
		c := runToHalt(t, b, mode)
		if c.Status == domain.StatusAwaitingApproval {
			c = approve(t, b, c)
		}
		if c.Mode != mode {
			t.Errorf("case records mode %s, want %s", c.Mode, mode)
		}
		results[mode] = c
	}

	expected := "sig-db-pool-exhaustion"
	for _, mode := range []domain.Mode{domain.ModeSingle, domain.ModeMultiNoCritic} {
		c := results[mode]
		if len(c.Hypotheses) == 0 {
			t.Fatalf("%s produced no hypothesis", mode)
		}
		if c.Hypotheses[0].SignatureID == expected {
			t.Errorf("%s reached the correct root cause; the comparison would be vacuous", mode)
		}
		if c.Round != 1 {
			t.Errorf("%s ran %d rounds; a single pass is one round", mode, c.Round)
		}
	}
	full := results[domain.ModeMultiWithCritic]
	if full.Hypotheses[0].SignatureID != expected {
		t.Errorf("the full flow reached %s, want %s", full.Hypotheses[0].SignatureID, expected)
	}
	if full.Round <= results[domain.ModeSingle].Round {
		t.Error("the full flow should have taken more rounds than a single pass")
	}
}

// sdd:verify TC-0104
func TestMisleadingLogsDoNotWin(t *testing.T) {
	withC4 := func(p *arena.Params) { p.CaseID = "C4" }

	b := build(t, withC4)
	c := runToHalt(t, b, domain.ModeMultiWithCritic)
	if c.Status == domain.StatusAwaitingApproval {
		c = approve(t, b, c)
	}
	if len(c.Hypotheses) == 0 {
		t.Fatal("C4 produced no hypothesis")
	}

	const expected = "sig-traffic-surge"

	// The point of this case. C1 is won by demoting sig-traffic-surge from first
	// place, so a system that had learned "the leading explanation is wrong", or
	// simply "traffic is never the cause", would still score perfectly on C1..C3 and
	// fail here. Only a case where the critic must decline to object distinguishes a
	// discriminator from a bias.
	t.Run("the explanation C1 demotes is the one that wins here", func(t *testing.T) {
		if got := c.Hypotheses[0].SignatureID; got != expected {
			t.Errorf("accepted %s, want %s — the outcome looks like a fixed bias rather than evidence", got, expected)
		}
	})

	t.Run("the loudest evidence does not decide the outcome", func(t *testing.T) {
		// The database errors are the highest-volume log signal in the whole catalog.
		var logLines int
		for _, e := range c.Evidence {
			if e.Kind == domain.KindLog {
				if n, err := strconv.Atoi(e.Fact("count")); err == nil {
					logLines += n
				}
			}
		}
		if logLines == 0 {
			t.Error("no log volume was recorded; this case cannot demonstrate anything about loud evidence")
		}
		for i, h := range c.Hypotheses {
			if i == 0 {
				continue
			}
			if h.Breakdown.Total >= c.Hypotheses[0].Breakdown.Total {
				t.Errorf("%s scores %.2f, not below the leader's %.2f",
					h.SignatureID, h.Breakdown.Total, c.Hypotheses[0].Breakdown.Total)
			}
		}
	})

	// Each database explanation must be refuted by evidence, not merely out-scored.
	// Being beaten on points would leave open the possibility that the ranking is
	// arbitrary; carrying counter-evidence is a positive finding against the claim.
	t.Run("each database explanation is refuted by discriminating evidence", func(t *testing.T) {
		for _, want := range []string{"sig-db-pool-exhaustion", "sig-db-outage"} {
			var found *domain.Hypothesis
			for i := range c.Hypotheses {
				if c.Hypotheses[i].SignatureID == want {
					found = &c.Hypotheses[i]
				}
			}
			if found == nil {
				continue // never proposed at all, which is a stronger form of the same thing
			}
			if len(found.Counter) == 0 {
				t.Errorf("%s was out-scored but never contradicted; the ranking rests on arithmetic alone", want)
			}
		}
	})
}

// sdd:verify TC-0107
func TestVictimAlertReachesUpstreamCause(t *testing.T) {
	b := build(t, func(p *arena.Params) { p.CaseID = "C5" })
	c := runToHalt(t, b, domain.ModeMultiWithCritic)
	if c.Status == domain.StatusAwaitingApproval {
		c = approve(t, b, c)
	}
	if len(c.Hypotheses) == 0 {
		t.Fatal("C5 produced no hypothesis")
	}

	const upstream = "order-api"

	t.Run("the accepted cause is the upstream one", func(t *testing.T) {
		if got := c.Hypotheses[0].SignatureID; got != "sig-db-pool-exhaustion" {
			t.Errorf("accepted %s; the fault is upstream, in %s", got, upstream)
		}
		if c.Alert.Service == upstream {
			t.Fatal("this case is pointless unless the alert names the victim")
		}
	})

	// The system alerted on one service and acted on another. That is the whole point:
	// the page names the symptom, not the subject.
	t.Run("the action targets the upstream, not the service that alerted", func(t *testing.T) {
		if len(c.Actions) == 0 {
			t.Fatal("no action was proposed, so the case never reached the upstream")
		}
		a := c.Actions[0]
		if got := a.Args["service"]; got != upstream {
			t.Errorf("action targets %q, want %q — it is remediating the victim", got, upstream)
		}
		if a.Args["service"] == c.Alert.Service {
			t.Error("the action targets the alerting service, which is the victim")
		}
	})

	// The rule existed and was unit-tested from the start, but no fault case had ever
	// made it fire end to end. A rule that only runs in its own unit test is a rule
	// nobody has watched work.
	t.Run("the source_vs_victim rule fires", func(t *testing.T) {
		var fired bool
		for _, cr := range c.Critiques {
			if cr.Rule == "source_vs_victim" {
				fired = true
			}
		}
		if !fired {
			t.Error("source_vs_victim never fired, so this case does not exercise it")
		}
	})

	// Without this, the rule objects to the answer it exists to reach, using the very
	// evidence that supports it.
	t.Run("it does not challenge the explanation that already looks upstream", func(t *testing.T) {
		accepted := c.Hypotheses[0].ID
		for _, cr := range c.Critiques {
			if cr.Rule == "source_vs_victim" && cr.HypothesisID == accepted {
				t.Errorf("source_vs_victim challenged %s, the upstream explanation itself", accepted)
			}
		}
	})

	// A challenge that demands nothing ends the investigation instead of redirecting it.
	t.Run("the challenge redirects rather than vetoes", func(t *testing.T) {
		if c.Round < 2 {
			t.Errorf("the case closed after %d round(s); the victim challenge asked for nothing", c.Round)
		}
		var upstreamEvidence int
		for _, e := range c.Evidence {
			if e.Fact("subject") == upstream {
				upstreamEvidence++
			}
		}
		if upstreamEvidence == 0 {
			t.Error("no evidence about the upstream service was ever collected")
		}
	})
}

// sdd:verify TC-0108
func TestPostOnsetChangeIsRejected(t *testing.T) {
	b := build(t, func(p *arena.Params) { p.CaseID = "C6" })
	c := runToHalt(t, b, domain.ModeMultiWithCritic)
	if c.Status == domain.StatusAwaitingApproval {
		c = approve(t, b, c)
	}
	if len(c.Hypotheses) < 2 {
		t.Fatal("C6 needs at least the tempting explanation and the real one")
	}

	const tempting = "sig-db-pool-exhaustion"
	const actual = "sig-misconfigured-dependency"

	var temptingH *domain.Hypothesis
	for i := range c.Hypotheses {
		if c.Hypotheses[i].SignatureID == tempting {
			temptingH = &c.Hypotheses[i]
		}
	}
	if temptingH == nil {
		t.Fatalf("%s was never proposed, so nothing here is being refuted", tempting)
	}

	// The whole point: the pool explanation fits. It is refuted by the clock alone.
	t.Run("the tempting explanation fits the evidence", func(t *testing.T) {
		if temptingH.Breakdown.Total < 0.75 {
			t.Errorf("%s scores %.2f; this case only tests timing if the fit is otherwise strong",
				tempting, temptingH.Breakdown.Total)
		}
	})

	t.Run("it is rejected on timing", func(t *testing.T) {
		if temptingH.Verdict != domain.VerdictReject {
			t.Errorf("verdict = %q, want reject", temptingH.Verdict)
		}
		var fired bool
		for _, cr := range c.Critiques {
			if cr.Rule == "temporal_order" && cr.HypothesisID == temptingH.ID {
				fired = true
			}
		}
		if !fired {
			t.Error("temporal_order did not challenge the explanation whose change postdates onset")
		}
	})

	// A rejected explanation may still rank first — it fits best. What it must not do is
	// be reported as the answer while the same report records its rejection.
	t.Run("the accepted cause is the admissible one", func(t *testing.T) {
		lead, ok := c.Leading()
		if !ok {
			t.Fatal("no leading hypothesis")
		}
		if lead.SignatureID != actual {
			t.Errorf("accepted %s, want %s", lead.SignatureID, actual)
		}
		if !lead.Verdict.Permits() {
			t.Errorf("the accepted explanation carries verdict %q", lead.Verdict)
		}
		md := report.Markdown(c)
		if !strings.Contains(md, actual) {
			t.Error("the report does not name the accepted cause")
		}
	})

	t.Run("the remediation acts on the real cause", func(t *testing.T) {
		if len(c.Actions) == 0 {
			t.Fatal("no action was proposed")
		}
		if got := c.Actions[0].Args["key"]; got != "PAYMENT_URL" {
			t.Errorf("action changes %q; the pool change was the red herring", got)
		}
	})
}

// sdd:verify TC-0110
func TestRoundOneErrorIsFullySupported(t *testing.T) {
	// What makes C1 worth running is *why* its first round is wrong.
	//
	// It used to be wrong the cheap way: the leader was an explanation nothing
	// contradicted, winning because the evidence that would have beaten it had not
	// been collected yet. A critic that corrects that is only correcting an omission.
	//
	// C1 now contains a real two-minute database outage, coincident with the incident
	// and far too short to explain it. Round one therefore reaches an explanation with
	// every requirement it declares satisfied by evidence actually in hand — the answer
	// a careful reasoner would give on that evidence — and the second round has to take
	// it apart rather than merely fill a gap.
	cat, err := catalog.Load()
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}

	t.Run("round one leads with a wrong explanation nothing has left unsupported", func(t *testing.T) {
		// A one-round budget stops the case exactly where round one ended.
		b := build(t, func(p *arena.Params) {
			cfg := reasoner.DefaultConfig()
			cfg.MaxRounds = 1
			p.Config = cfg
		})
		c := runToHalt(t, b, domain.ModeMultiWithCritic)
		if len(c.Hypotheses) < 2 {
			t.Fatalf("round one proposed %d hypotheses; there is no ranking to examine",
				len(c.Hypotheses))
		}

		const wrong = "sig-db-outage"
		lead := c.Hypotheses[0]
		if lead.SignatureID != wrong {
			t.Fatalf("round one leads with %s, want %s", lead.SignatureID, wrong)
		}

		sig, ok := cat.Signature(wrong)
		if !ok {
			t.Fatalf("signature %s is not in the catalog", wrong)
		}
		m := catalog.Match(sig, c.Evidence)
		if len(m.Unmatched) != 0 {
			var missing []string
			for _, p := range m.Unmatched {
				missing = append(missing, p.Label)
			}
			t.Errorf("the round-one leader still wants %v; it is winning on an "+
				"unexamined gap rather than on support", missing)
		}

		// And it is not a coin toss the critic could break by rounding.
		if gap := lead.Breakdown.Total - c.Hypotheses[1].Breakdown.Total; gap < reasoner.DefaultConfig().CloseCallMargin {
			t.Errorf("the round-one leader is only %.2f ahead; a near-tie is a weaker "+
				"error than a confident one", gap)
		}
	})

	t.Run("the second round refutes it rather than out-scoring it", func(t *testing.T) {
		b := build(t)
		c := runToHalt(t, b, domain.ModeMultiWithCritic)
		if c.Status == domain.StatusAwaitingApproval {
			c = approve(t, b, c)
		}
		lead, ok := c.Leading()
		if !ok {
			t.Fatal("the case accepted no explanation")
		}
		if lead.SignatureID != "sig-db-pool-exhaustion" {
			t.Errorf("accepted %s, want sig-db-pool-exhaustion", lead.SignatureID)
		}

		var outage *domain.Hypothesis
		for i := range c.Hypotheses {
			if c.Hypotheses[i].SignatureID == "sig-db-outage" {
				outage = &c.Hypotheses[i]
			}
		}
		if outage == nil {
			t.Fatal("the round-one leader disappeared from the ranking instead of being answered")
		}
		if len(outage.Counter) == 0 {
			t.Error("the outage explanation was out-scored but never contradicted; " +
				"a fully supported wrong answer has to be argued with, not outvoted")
		}
		// The counter-evidence must be the duration mismatch, not something incidental.
		for _, id := range outage.Counter {
			e, ok := c.EvidenceIndex()[id]
			if !ok {
				t.Errorf("counter-evidence %s is not in the evidence set", id)
				continue
			}
			if e.Fact("errors_after_recovery") == "" {
				t.Errorf("counter-evidence %s does not record how long the errors "+
					"outlasted the database", id)
			}
		}
	})
}

// sdd:verify TC-0111
func TestUnanswerableDemandDoesNotVeto(t *testing.T) {
	// C4 demands the duration comparison that settles C1, and C4 has no source that
	// can answer it: its database never went down, so there is no recovery time to
	// compare against. That makes it the natural fixture for the failure mode.
	//
	// Before the round accounting distinguished "not yet attempted" from "attempted
	// and unanswerable", this demand was re-issued every round, the rule that raised
	// it held the leader in `revise`, and C4 accepted nothing at all: budget gone,
	// escalated to human review, wrong cause reported. An objection nothing can answer
	// is a veto however slowly it arrives.
	const descriptor = "error rate duration compared with the database recovery time"

	b := build(t, func(p *arena.Params) { p.CaseID = "C4" })
	c := runToHalt(t, b, domain.ModeMultiWithCritic)
	if c.Status == domain.StatusAwaitingApproval {
		c = approve(t, b, c)
	}

	var demand *domain.EvidenceDemand
	for i := range c.Demands {
		if c.Demands[i].Descriptor == descriptor {
			demand = &c.Demands[i]
		}
	}
	if demand == nil {
		t.Fatalf("C4 never demanded %q; this case can no longer demonstrate anything "+
			"about unanswerable demands", descriptor)
	}
	if demand.Satisfied() {
		t.Fatalf("%q was answered; the fixture no longer exercises the failure", descriptor)
	}

	// Asked across both cases that carry unanswerable demands, because the rules that
	// raise them differ: C4's comes from the alternative-explanation rule, C5's four
	// from source-versus-victim, and each rule decides for itself whether to re-ask.
	for _, id := range []string{"C4", "C5"} {
		t.Run(id+": every demand is raised once, not once per round", func(t *testing.T) {
			b := build(t, func(p *arena.Params) { p.CaseID = id })
			run := runToHalt(t, b, domain.ModeMultiWithCritic)
			if run.Status == domain.StatusAwaitingApproval {
				run = approve(t, b, run)
			}
			raised := map[string]int{}
			for _, ev := range run.Timeline {
				if ev.Type != domain.EvEvidenceDemanded {
					continue
				}
				raised[ev.Ref]++
			}
			if len(raised) == 0 {
				t.Fatalf("%s demanded nothing", id)
			}
			for _, d := range run.Demands {
				if n := raised[d.ID]; n != 1 {
					t.Errorf("%s: %q was demanded %d times; a question a collection "+
						"round already failed to answer is not worth asking again",
						id, d.Descriptor, n)
				}
			}
		})
	}

	t.Run("it does not consume the round budget", func(t *testing.T) {
		if c.Round >= reasoner.DefaultConfig().MaxRounds {
			t.Errorf("the case ran %d of %d rounds; one unanswerable demand should not "+
				"cost the case its remaining budget", c.Round, reasoner.DefaultConfig().MaxRounds)
		}
	})

	t.Run("it does not block acceptance", func(t *testing.T) {
		if c.Status != domain.StatusClosed {
			t.Errorf("case ended in %s, want closed", c.Status)
		}
		lead, ok := c.Leading()
		if !ok || !lead.Verdict.Permits() {
			t.Fatalf("no explanation was accepted (leading ok=%v)", ok)
		}
	})

	t.Run("it is still reported as unmet, with a reason", func(t *testing.T) {
		md := report.Markdown(c)
		if !strings.Contains(md, descriptor) {
			t.Error("the report does not mention the demand nobody could answer")
		}
		var noted bool
		for _, ev := range c.Timeline {
			if ev.Type == domain.EvDemandUnmet && strings.Contains(ev.Summary, descriptor) {
				noted = true
			}
		}
		if !noted {
			t.Error("no demand_unmet event records it; not answering is a finding, " +
				"and dropping it silently hides one")
		}
	})
}
