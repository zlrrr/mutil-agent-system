// Package eval computes the multi-agent claim instead of asserting it: the same fault
// cases run in three modes over identical fixture inputs (ARC-004, REQ-0080).
package eval

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/zlrrr/mutil-agent-system/internal/arena"
	"github.com/zlrrr/mutil-agent-system/internal/catalog"
	"github.com/zlrrr/mutil-agent-system/internal/domain"
)

// sdd:impl DLD-1074

// Modes are the three comparable flows, in report order.
var Modes = []domain.Mode{
	domain.ModeSingle, domain.ModeMultiNoCritic, domain.ModeMultiWithCritic,
}

// Outcome is what one case produced in one mode.
type Outcome struct {
	CaseID            string      `json:"case_id"`
	Mode              domain.Mode `json:"mode"`
	Expected          string      `json:"expected_signature"`
	Top1              string      `json:"top1_signature"`
	Top1Correct       bool        `json:"top1_correct"`
	Top3Covered       bool        `json:"top3_covered"`
	EvidenceKinds     int         `json:"evidence_kinds"`
	Rounds            int         `json:"rounds"`
	CriticCorrections int         `json:"critic_corrections"`
	BlockedActions    int         `json:"blocked_actions"`
	Status            string      `json:"status"`
	Failure           string      `json:"failure,omitempty"`
}

// ModeSummary aggregates outcomes for one mode.
type ModeSummary struct {
	Mode              domain.Mode `json:"mode"`
	Samples           int         `json:"samples"`
	Top1Accuracy      float64     `json:"top1_accuracy"`
	Top3Coverage      float64     `json:"top3_coverage"`
	MeanEvidenceKinds float64     `json:"mean_evidence_kinds"`
	MeanRounds        float64     `json:"mean_rounds"`
	CriticCorrections int         `json:"critic_corrections"`
	BlockedActions    int         `json:"blocked_actions"`
	Failures          int         `json:"failures"`
}

// Report is the full comparison.
type Report struct {
	Outcomes  []Outcome     `json:"outcomes"`
	Summaries []ModeSummary `json:"summaries"`
}

// RunCase executes one fault case in one mode over a fresh engine and fixture set.
//
// Actions are auto-approved here so the flow completes; the approval gate itself is
// verified by its own test rather than by the evaluation, whose subject is ranking
// quality.
func RunCase(ctx context.Context, cat *catalog.Catalog, caseID string, mode domain.Mode) (Outcome, error) {
	fc, ok := cat.Case(caseID)
	if !ok {
		return Outcome{}, fmt.Errorf("unknown case %q", caseID)
	}
	out := Outcome{CaseID: caseID, Mode: mode, Expected: fc.ExpectedSignature}

	b, err := arena.NewFixtureBuild(arena.Params{CaseID: caseID, Catalog: cat})
	if err != nil {
		out.Failure = err.Error()
		return out, nil
	}
	c, err := b.Engine.Create(ctx, b.Alert(), mode)
	if err != nil {
		out.Failure = err.Error()
		return out, nil
	}
	c, err = b.Engine.Run(ctx, c.ID)
	if err != nil {
		out.Failure = err.Error()
		return out, nil
	}
	for c.Status == domain.StatusAwaitingApproval {
		a, ok := c.PendingAction()
		if !ok {
			break
		}
		c, err = b.Engine.Decide(ctx, c.ID, a.ID, domain.ApprovalDecision{
			Decision: "approved", By: "evaluation", Comment: "auto-approved for evaluation",
		})
		if err != nil {
			out.Failure = err.Error()
			return out, nil
		}
	}

	out.Status = string(c.Status)
	out.Rounds = c.Round
	out.EvidenceKinds = len(domain.DistinctKinds(c.Evidence))
	out.CriticCorrections = c.CriticCorrections
	for _, a := range c.Actions {
		if a.Risk.RequiresApproval() {
			out.BlockedActions++
		}
	}
	if len(c.Hypotheses) > 0 {
		out.Top1 = c.Hypotheses[0].SignatureID
		out.Top1Correct = out.Top1 == fc.ExpectedSignature
	}
	for i, h := range c.Hypotheses {
		if i >= 3 {
			break
		}
		if h.SignatureID == fc.ExpectedSignature {
			out.Top3Covered = true
		}
	}
	return out, nil
}

// RunAll executes every case in every mode.
func RunAll(ctx context.Context, cat *catalog.Catalog, caseIDs []string) (Report, error) {
	if len(caseIDs) == 0 {
		caseIDs = cat.CaseIDs()
	}
	sort.Strings(caseIDs)

	var rep Report
	for _, mode := range Modes {
		for _, id := range caseIDs {
			o, err := RunCase(ctx, cat, id, mode)
			if err != nil {
				return rep, err
			}
			rep.Outcomes = append(rep.Outcomes, o)
		}
	}
	rep.Summaries = Summarise(rep.Outcomes)
	return rep, nil
}

// Summarise aggregates outcomes per mode. Every rate is reported with the sample size
// it was computed from, because a rate without a denominator is not a measurement.
func Summarise(outcomes []Outcome) []ModeSummary {
	byMode := map[domain.Mode][]Outcome{}
	for _, o := range outcomes {
		byMode[o.Mode] = append(byMode[o.Mode], o)
	}
	var out []ModeSummary
	for _, mode := range Modes {
		os := byMode[mode]
		if len(os) == 0 {
			continue
		}
		s := ModeSummary{Mode: mode, Samples: len(os)}
		var top1, top3, kinds, rounds int
		for _, o := range os {
			if o.Failure != "" {
				s.Failures++
			}
			if o.Top1Correct {
				top1++
			}
			if o.Top3Covered {
				top3++
			}
			kinds += o.EvidenceKinds
			rounds += o.Rounds
			s.CriticCorrections += o.CriticCorrections
			s.BlockedActions += o.BlockedActions
		}
		n := float64(len(os))
		s.Top1Accuracy = float64(top1) / n
		s.Top3Coverage = float64(top3) / n
		s.MeanEvidenceKinds = float64(kinds) / n
		s.MeanRounds = float64(rounds) / n
		out = append(out, s)
	}
	return out
}

// Markdown renders the comparison for the README and the manual.
func (r Report) Markdown() string {
	var b strings.Builder
	b.WriteString("## Single agent versus multi agent\n\n")
	b.WriteString("All three modes run over identical fixture inputs, so a difference " +
		"is attributable to the flow rather than to the data.\n\n")
	b.WriteString("| Mode | Samples | Top-1 accuracy | Top-3 coverage | Mean evidence kinds | Mean rounds | Critic corrections | Actions held for approval |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|\n")
	for _, s := range r.Summaries {
		fmt.Fprintf(&b, "| %s | %d | %.0f%% | %.0f%% | %.1f | %.1f | %d | %d |\n",
			s.Mode, s.Samples, s.Top1Accuracy*100, s.Top3Coverage*100,
			s.MeanEvidenceKinds, s.MeanRounds, s.CriticCorrections, s.BlockedActions)
	}
	b.WriteString("\n### Per case\n\n")
	b.WriteString("| Case | Mode | Expected | Top-1 | Correct | Rounds | Kinds | Status |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|\n")
	for _, o := range r.Outcomes {
		fmt.Fprintf(&b, "| %s | %s | `%s` | `%s` | %s | %d | %d | %s |\n",
			o.CaseID, o.Mode, o.Expected, orDash(o.Top1), tick(o.Top1Correct),
			o.Rounds, o.EvidenceKinds, o.Status)
	}
	b.WriteString("\nThe single-agent mode is given the same tools and the same default " +
		"queries as the multi-agent flow; it differs only in performing one pass with no " +
		"adversarial round. It is a model of \"one context window, one look\", not of a " +
		"weaker toolset.\n")
	return b.String()
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func tick(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
