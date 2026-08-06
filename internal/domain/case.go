package domain

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// sdd:impl DLD-1007

// Status is a state of the investigation state machine (DLD-1060).
type Status string

// The declared states.
const (
	StatusCreated          Status = "created"
	StatusTriaging         Status = "triaging"
	StatusCollecting       Status = "collecting"
	StatusHypothesising    Status = "hypothesising"
	StatusCriticising      Status = "criticising"
	StatusHumanReview      Status = "human_review"
	StatusRemediating      Status = "remediating"
	StatusAwaitingApproval Status = "awaiting_approval"
	StatusExecuting        Status = "executing"
	StatusVerifying        Status = "verifying"
	StatusReporting        Status = "reporting"
	StatusClosed           Status = "closed"
)

// Terminal reports whether the case has finished.
func (s Status) Terminal() bool { return s == StatusClosed }

// Mode names how the case was run, so that an evaluation comparison is attributable.
type Mode string

// The three comparable modes (REQ-0080).
const (
	ModeSingle          Mode = "single"
	ModeMultiNoCritic   Mode = "multi_no_critic"
	ModeMultiWithCritic Mode = "multi_with_critic"
)

// TimelineEntry is one human-readable line of the agent timeline.
type TimelineEntry struct {
	Seq      int           `json:"seq"`
	At       time.Time     `json:"at"`
	Actor    Role          `json:"actor"`
	Type     EventType     `json:"type"`
	Summary  string        `json:"summary"`
	Ref      string        `json:"ref,omitempty"`
	Duration time.Duration `json:"duration,omitempty"`
}

// Case is the projection of a case's event log. Nothing writes its fields except
// Apply, so the live view and a replayed view are the same computation (ARC-003).
type Case struct {
	ID        string    `json:"id"`
	Alert     Alert     `json:"alert"`
	Mode      Mode      `json:"mode"`
	Status    Status    `json:"status"`
	Round     int       `json:"round"`
	CreatedAt time.Time `json:"created_at"`
	ClosedAt  time.Time `json:"closed_at,omitempty"`

	Evidence      []Evidence           `json:"evidence"`
	Hypotheses    []Hypothesis         `json:"hypotheses"`
	Critiques     []Critique           `json:"critiques"`
	Demands       []EvidenceDemand     `json:"demands"`
	Actions       []Action             `json:"actions"`
	Verifications []VerificationResult `json:"verifications"`
	Timeline      []TimelineEntry      `json:"timeline"`

	RejectedHypotheses []Rejection `json:"rejected_hypotheses,omitempty"`
	RejectedContribs   []Rejection `json:"rejected_contributions,omitempty"`
	Notes              []string    `json:"notes,omitempty"`

	// CriticCorrections counts hypotheses whose verdict the critic set to something
	// other than accept. It is the evaluation's measure of adversarial value.
	CriticCorrections int  `json:"critic_corrections"`
	Truncations       int  `json:"truncations"`
	HumanReviewed     bool `json:"human_reviewed"`

	// LastSeq is the highest event sequence folded so far.
	LastSeq int `json:"last_seq"`
	// Seq holds per-(prefix,role) identifier counters, recovered during replay so
	// that a replayed case can continue issuing identifiers without collision.
	Seq map[string]int `json:"-"`

	evidenceIndex map[string]int
	hypoIndex     map[string]int
	demandIndex   map[string]int
	actionIndex   map[string]int
}

// NewCase returns an empty projection ready to fold events into.
func NewCase(id string) *Case {
	return &Case{
		ID:            id,
		Status:        StatusCreated,
		Seq:           map[string]int{},
		evidenceIndex: map[string]int{},
		hypoIndex:     map[string]int{},
		demandIndex:   map[string]int{},
		actionIndex:   map[string]int{},
	}
}

// Window returns the incident time window derived from the alert.
func (c *Case) Window() TimeWindow {
	end := c.Alert.EndsAt
	if end.IsZero() {
		end = c.Alert.StartsAt.Add(20 * time.Minute)
	}
	return TimeWindow{Start: c.Alert.StartsAt.Add(-7 * time.Minute), End: end}
}

// EvidenceIndex returns the evidence keyed by identifier.
func (c *Case) EvidenceIndex() map[string]Evidence {
	out := make(map[string]Evidence, len(c.Evidence))
	for _, e := range c.Evidence {
		out[e.ID] = e
	}
	return out
}

// Leading returns the top-ranked hypothesis, if any.
func (c *Case) Leading() (Hypothesis, bool) {
	if len(c.Hypotheses) == 0 {
		return Hypothesis{}, false
	}
	// The highest-scoring explanation the critic has not rejected.
	//
	// Score measures how well the evidence fits; a verdict measures whether the
	// explanation is admissible at all. They are different questions, and an
	// explanation can fit the evidence beautifully while being impossible — one whose
	// own symptom began after the incident, for instance. Presenting such an
	// explanation as the answer, in a report that also records its rejection, would
	// make the system contradict itself.
	//
	// The ranking is left alone: it still shows the rejected explanation first, with
	// the verdict and the counter-evidence beside it. "This fit best and here is why we
	// rejected it" is worth more to a reader than quietly hiding it.
	for _, h := range c.Hypotheses {
		if h.Verdict == "" || h.Verdict.Permits() {
			return h, true
		}
	}
	return c.Hypotheses[0], true
}

// TopRanked returns the highest-scoring hypothesis regardless of verdict, which is what
// the ranking table and the score comparison report.
func (c *Case) TopRanked() (Hypothesis, bool) {
	if len(c.Hypotheses) == 0 {
		return Hypothesis{}, false
	}
	return c.Hypotheses[0], true
}

// Runner returns the second-ranked hypothesis, if any.
func (c *Case) Runner() (Hypothesis, bool) {
	if len(c.Hypotheses) < 2 {
		return Hypothesis{}, false
	}
	return c.Hypotheses[1], true
}

// UnsatisfiedDemands returns demands that no evidence has answered yet.
func (c *Case) UnsatisfiedDemands() []EvidenceDemand {
	var out []EvidenceDemand
	for _, d := range c.Demands {
		if !d.Satisfied() {
			out = append(out, d)
		}
	}
	return out
}

// PendingAction returns the first action awaiting an approval decision.
func (c *Case) PendingAction() (Action, bool) {
	for _, a := range c.Actions {
		if a.Risk.RequiresApproval() && a.Approval == nil {
			return a, true
		}
	}
	return Action{}, false
}

// ExecutedAction returns the most recent successfully executed action.
func (c *Case) ExecutedAction() (Action, bool) {
	for i := len(c.Actions) - 1; i >= 0; i-- {
		if a := c.Actions[i]; a.Execution != nil && a.Execution.Outcome == "executed" {
			return a, true
		}
	}
	return Action{}, false
}

// CritiquesFor returns the critiques raised against a hypothesis, in order.
func (c *Case) CritiquesFor(hypothesisID string) []Critique {
	var out []Critique
	for _, cr := range c.Critiques {
		if cr.HypothesisID == hypothesisID {
			out = append(out, cr)
		}
	}
	return out
}

// Recovered reports whether every recorded verification observed recovery, and whether
// any verification exists at all.
func (c *Case) Recovered() (recovered bool, any bool) {
	if len(c.Verifications) == 0 {
		return false, false
	}
	for _, v := range c.Verifications {
		if !v.Recovered {
			return false, true
		}
	}
	return true, true
}

// NextSeq returns and reserves the next identifier sequence for a prefix and role.
func (c *Case) NextSeq(prefix string, role Role) int {
	key := prefix + "-" + string(role)
	c.Seq[key]++
	return c.Seq[key]
}

// noteSeq recovers an identifier counter from an identifier seen during replay, so a
// replayed case continues numbering where the live one left off.
func (c *Case) noteSeq(id string) {
	i := strings.LastIndexByte(id, '-')
	if i < 0 {
		return
	}
	n, err := strconv.Atoi(id[i+1:])
	if err != nil {
		return
	}
	key := id[:i]
	if n > c.Seq[key] {
		c.Seq[key] = n
	}
}

// Apply folds one event into the projection. It is the only writer of case state.
func (c *Case) Apply(e Event) error {
	if e.Seq != c.LastSeq+1 {
		return fmt.Errorf("event sequence gap: got %d, want %d", e.Seq, c.LastSeq+1)
	}
	c.LastSeq = e.Seq

	switch e.Type {
	case EvCaseCreated:
		var p struct {
			Alert Alert `json:"alert"`
			Mode  Mode  `json:"mode"`
		}
		if err := decode(e.Payload, &p); err != nil {
			return err
		}
		c.Alert = p.Alert
		c.Mode = p.Mode
		c.CreatedAt = e.At
		c.Status = StatusCreated

	case EvStateChanged:
		var p StateChange
		if err := decode(e.Payload, &p); err != nil {
			return err
		}
		c.Status = p.To
		if p.To == StatusHumanReview {
			c.HumanReviewed = true
		}
		if p.To == StatusClosed {
			c.ClosedAt = e.At
		}

	case EvRoundStarted:
		var p RoundStart
		if err := decode(e.Payload, &p); err != nil {
			return err
		}
		c.Round = p.Round

	case EvEvidenceAdded:
		var ev Evidence
		if err := decode(e.Payload, &ev); err != nil {
			return err
		}
		c.noteSeq(ev.ID)
		if i, ok := c.evidenceIndex[ev.ID]; ok {
			c.Evidence[i] = ev
		} else {
			c.evidenceIndex[ev.ID] = len(c.Evidence)
			c.Evidence = append(c.Evidence, ev)
		}

	case EvEvidenceTruncated:
		c.Truncations++

	case EvHypothesisProposed, EvHypothesisScored:
		var h Hypothesis
		if err := decode(e.Payload, &h); err != nil {
			return err
		}
		c.noteSeq(h.ID)
		if i, ok := c.hypoIndex[h.ID]; ok {
			c.Hypotheses[i] = h
		} else {
			c.hypoIndex[h.ID] = len(c.Hypotheses)
			c.Hypotheses = append(c.Hypotheses, h)
		}
		c.reindexHypotheses()

	case EvHypothesisRejected:
		var p Rejection
		if err := decode(e.Payload, &p); err != nil {
			return err
		}
		c.RejectedHypotheses = append(c.RejectedHypotheses, p)

	case EvCritiqueRaised:
		var cr Critique
		if err := decode(e.Payload, &cr); err != nil {
			return err
		}
		c.noteSeq(cr.ID)
		c.Critiques = append(c.Critiques, cr)
		if i, ok := c.hypoIndex[cr.HypothesisID]; ok {
			h := &c.Hypotheses[i]
			h.Verdict = CombineVerdicts(h.Verdict, cr.Verdict)
			if !cr.Verdict.Permits() {
				h.Status = HypothesisChallenged
			}
			for _, cid := range cr.CounterIDs {
				h.AddCounter(cid)
			}
			if cr.Verdict != VerdictAccept {
				c.CriticCorrections++
			}
		}

	case EvEvidenceDemanded:
		var d EvidenceDemand
		if err := decode(e.Payload, &d); err != nil {
			return err
		}
		c.noteSeq(d.ID)
		if i, ok := c.demandIndex[d.ID]; ok {
			c.Demands[i] = d
		} else {
			c.demandIndex[d.ID] = len(c.Demands)
			c.Demands = append(c.Demands, d)
		}

	case EvDemandSatisfied:
		var p DemandLink
		if err := decode(e.Payload, &p); err != nil {
			return err
		}
		if i, ok := c.demandIndex[p.DemandID]; ok {
			c.Demands[i].SatisfiedBy = p.EvidenceID
		}

	case EvDemandUnmet:
		var p Rejection
		if err := decode(e.Payload, &p); err != nil {
			return err
		}
		c.Notes = append(c.Notes, "unmet evidence demand: "+p.Target+" ("+p.Reason+")")

	case EvContributionRejected:
		var p Rejection
		if err := decode(e.Payload, &p); err != nil {
			return err
		}
		c.RejectedContribs = append(c.RejectedContribs, p)

	case EvActionProposed:
		var a Action
		if err := decode(e.Payload, &a); err != nil {
			return err
		}
		c.noteSeq(a.ID)
		if i, ok := c.actionIndex[a.ID]; ok {
			c.Actions[i] = a
		} else {
			c.actionIndex[a.ID] = len(c.Actions)
			c.Actions = append(c.Actions, a)
		}

	case EvApprovalRecorded:
		var p ApprovalRecord
		if err := decode(e.Payload, &p); err != nil {
			return err
		}
		if i, ok := c.actionIndex[p.ActionID]; ok {
			d := p.Decision
			c.Actions[i].Approval = &d
		}

	case EvActionExecuted, EvActionRefused:
		var r ExecutionResult
		if err := decode(e.Payload, &r); err != nil {
			return err
		}
		if i, ok := c.actionIndex[r.ActionID]; ok {
			rr := r
			c.Actions[i].Execution = &rr
		}

	case EvVerificationRecorded:
		var v VerificationResult
		if err := decode(e.Payload, &v); err != nil {
			return err
		}
		c.Verifications = append(c.Verifications, v)

	case EvBudgetExhausted:
		var p Rejection
		if err := decode(e.Payload, &p); err != nil {
			return err
		}
		c.Notes = append(c.Notes, "budget exhausted: "+p.Reason)
	}

	c.Timeline = append(c.Timeline, TimelineEntry{
		Seq: e.Seq, At: e.At, Actor: e.Actor, Type: e.Type,
		Summary: e.Summary, Ref: e.Ref, Duration: e.Duration,
	})
	return nil
}

// reindexHypotheses restores the rank order and the identifier index after an update.
// Ranking is a total order: score descending, supporting-kind count descending,
// identifier ascending (REQ-0024).
func (c *Case) reindexHypotheses() {
	index := c.EvidenceIndex()
	sort.SliceStable(c.Hypotheses, func(i, j int) bool {
		a, b := c.Hypotheses[i], c.Hypotheses[j]
		if a.Breakdown.Total != b.Breakdown.Total {
			return a.Breakdown.Total > b.Breakdown.Total
		}
		ka, kb := len(a.SupportingKinds(index)), len(b.SupportingKinds(index))
		if ka != kb {
			return ka > kb
		}
		return a.ID < b.ID
	})
	c.hypoIndex = make(map[string]int, len(c.Hypotheses))
	for i, h := range c.Hypotheses {
		c.hypoIndex[h.ID] = i
	}
}

// Replay folds a whole log into a fresh projection. Equality with the live projection
// is the property REQ-0003 asserts.
func Replay(caseID string, events []Event) (*Case, error) {
	c := NewCase(caseID)
	for _, e := range events {
		if err := c.Apply(e); err != nil {
			return nil, fmt.Errorf("replay %s seq %d: %w", caseID, e.Seq, err)
		}
	}
	return c, nil
}

// Snapshot is the read-only view an agent receives. Agents never hold a *Case, so they
// cannot mutate one (ARC-004).
type Snapshot struct {
	CaseID     string
	Alert      Alert
	Mode       Mode
	Window     TimeWindow
	Round      int
	MaxRounds  int
	Status     Status
	Evidence   []Evidence
	Index      map[string]Evidence
	Hypotheses []Hypothesis
	Critiques  []Critique
	Demands    []EvidenceDemand
	Actions    []Action
}

// Snapshot builds the read-only view of the current projection.
func (c *Case) Snapshot(maxRounds int) Snapshot {
	cp := func(s []Evidence) []Evidence { out := make([]Evidence, len(s)); copy(out, s); return out }
	hs := make([]Hypothesis, len(c.Hypotheses))
	copy(hs, c.Hypotheses)
	cs := make([]Critique, len(c.Critiques))
	copy(cs, c.Critiques)
	ds := make([]EvidenceDemand, len(c.Demands))
	copy(ds, c.Demands)
	as := make([]Action, len(c.Actions))
	copy(as, c.Actions)
	return Snapshot{
		CaseID: c.ID, Alert: c.Alert, Mode: c.Mode, Window: c.Window(),
		Round: c.Round, MaxRounds: maxRounds, Status: c.Status,
		Evidence: cp(c.Evidence), Index: c.EvidenceIndex(),
		Hypotheses: hs, Critiques: cs, Demands: ds, Actions: as,
	}
}

// UnsatisfiedDemands returns the demands still open in the snapshot.
func (s Snapshot) UnsatisfiedDemands() []EvidenceDemand {
	var out []EvidenceDemand
	for _, d := range s.Demands {
		if !d.Satisfied() {
			out = append(out, d)
		}
	}
	return out
}

// EvidenceOfKind returns the snapshot's evidence of one kind, in identifier order.
// Leading returns the highest-scoring hypothesis the critic has not rejected, matching
// Case.Leading so an agent and the control plane agree on which explanation is the
// case's answer.
func (s Snapshot) Leading() (Hypothesis, bool) {
	if len(s.Hypotheses) == 0 {
		return Hypothesis{}, false
	}
	for _, h := range s.Hypotheses {
		if h.Verdict == "" || h.Verdict.Permits() {
			return h, true
		}
	}
	return s.Hypotheses[0], true
}

func (s Snapshot) EvidenceOfKind(k EvidenceKind) []Evidence {
	var out []Evidence
	for _, e := range s.Evidence {
		if e.Kind == k {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
