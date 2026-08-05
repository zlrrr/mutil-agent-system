// Package report renders the document a reviewer reads, from the case projection
// alone — so a report regenerated from a replayed log is identical (ARC-003).
package report

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/zlrrr/mutil-agent-system/internal/domain"
)

// sdd:impl DLD-1070

const absent = "_none recorded_"

// Markdown renders the full root-cause report. It is a pure function of the
// projection: no clock is read and no map is iterated without sorting, so the same
// case always renders the same bytes.
func Markdown(c *domain.Case) string {
	var b strings.Builder
	index := c.EvidenceIndex()

	fmt.Fprintf(&b, "# Root cause report — %s\n\n", c.ID)
	writeSummary(&b, c)
	writeImpact(&b, c)
	writeTimeline(&b, c)
	writeRootCause(&b, c, index)
	writeRejected(&b, c, index)
	writeCritiques(&b, c)
	writeActions(&b, c)
	writeVerification(&b, c)
	writeUnmet(&b, c)
	writeValue(&b, c)
	return b.String()
}

func writeSummary(b *strings.Builder, c *domain.Case) {
	b.WriteString("## Summary\n\n")
	fmt.Fprintf(b, "| Field | Value |\n|---|---|\n")
	fmt.Fprintf(b, "| Alert | %s |\n", c.Alert.Name)
	fmt.Fprintf(b, "| Service | %s |\n", c.Alert.Service)
	fmt.Fprintf(b, "| Severity | %s |\n", c.Alert.Severity)
	fmt.Fprintf(b, "| Window | %s |\n", c.Window())
	fmt.Fprintf(b, "| Mode | %s |\n", c.Mode)
	fmt.Fprintf(b, "| Status | %s |\n", c.Status)
	fmt.Fprintf(b, "| Collection rounds | %d |\n", c.Round)
	fmt.Fprintf(b, "| Events | %d |\n\n", c.LastSeq)
	if s := c.Alert.Annotations["summary"]; s != "" {
		fmt.Fprintf(b, "%s\n\n", s)
	}
}

func writeImpact(b *strings.Builder, c *domain.Case) {
	b.WriteString("## Impact\n\n")
	var written bool
	for _, e := range c.Evidence {
		if e.Kind != domain.KindTopology {
			continue
		}
		fmt.Fprintf(b, "- Candidate origin: %s\n", dashIfEmpty(e.Fact("candidate_origin")))
		fmt.Fprintf(b, "- Also affected: %s\n", dashIfEmpty(e.Fact("affected")))
		fmt.Fprintf(b, "- Depends on: %s\n\n", dashIfEmpty(e.Fact("upstreams")))
		written = true
		break
	}
	if !written {
		b.WriteString(absent + "\n\n")
	}
}

func writeTimeline(b *strings.Builder, c *domain.Case) {
	b.WriteString("## Timeline\n\n")
	if len(c.Timeline) == 0 {
		b.WriteString(absent + "\n\n")
		return
	}
	b.WriteString("| Seq | Time | Actor | Event | Detail |\n|---|---|---|---|---|\n")
	for _, t := range c.Timeline {
		if !timelineWorthy(t.Type) {
			continue
		}
		fmt.Fprintf(b, "| %d | %s | %s | %s | %s |\n",
			t.Seq, t.At.UTC().Format("15:04:05"), t.Actor, t.Type, escape(t.Summary))
	}
	b.WriteString("\n")
}

// timelineWorthy filters the log down to the transitions a reader follows, leaving the
// full record available through the events endpoint.
func timelineWorthy(t domain.EventType) bool {
	switch t {
	case domain.EvAgentStarted, domain.EvEvidenceAdded, domain.EvHypothesisScored:
		return false
	}
	return true
}

func writeRootCause(b *strings.Builder, c *domain.Case, index map[string]domain.Evidence) {
	b.WriteString("## Accepted root cause\n\n")
	lead, ok := c.Leading()
	if !ok {
		b.WriteString(absent + "\n\n")
		return
	}
	fmt.Fprintf(b, "**%s**\n\n", lead.Claim)
	fmt.Fprintf(b, "Confidence %.2f, verdict `%s`, signature `%s`.\n\n",
		lead.Breakdown.Total, dashIfEmpty(string(lead.Verdict)), lead.SignatureID)
	fmt.Fprintf(b, "### Mechanism\n\n%s\n\n", lead.Mechanism)

	b.WriteString("### Score breakdown\n\n")
	b.WriteString("| Term | Weight | Value | Contribution |\n|---|---|---|---|\n")
	for _, t := range lead.Breakdown.Terms {
		fmt.Fprintf(b, "| %s | %.2f | %.2f | %.4f |\n", t.Name, t.Weight, t.Value, t.Contribution)
	}
	fmt.Fprintf(b, "| _counter-evidence penalty_ | | %d unresolved | -%.4f |\n",
		lead.Breakdown.Unresolved, lead.Breakdown.Penalty)
	fmt.Fprintf(b, "| **total** | | | **%.4f** |\n\n", lead.Breakdown.Total)

	b.WriteString("### Evidence chain\n\n")
	if len(lead.Supporting) == 0 {
		b.WriteString(absent + "\n\n")
		return
	}
	b.WriteString("| Evidence | Kind | Source | Summary | Query |\n|---|---|---|---|---|\n")
	for _, id := range sortedCopy(lead.Supporting) {
		e, ok := index[id]
		if !ok {
			continue
		}
		fmt.Fprintf(b, "| `%s` | %s | %s | %s | `%s` |\n",
			e.ID, e.Kind, e.Source, escape(e.Summary), e.RawRef)
	}
	b.WriteString("\n")
}

func writeRejected(b *strings.Builder, c *domain.Case, index map[string]domain.Evidence) {
	b.WriteString("## Rejected alternatives\n\n")
	if len(c.Hypotheses) < 2 && len(c.RejectedHypotheses) == 0 {
		b.WriteString(absent + "\n\n")
		return
	}
	for i, h := range c.Hypotheses {
		if i == 0 {
			continue
		}
		fmt.Fprintf(b, "### %s — scored %.2f\n\n", h.Claim, h.Breakdown.Total)
		fmt.Fprintf(b, "Signature `%s`, verdict `%s`.\n\n", h.SignatureID, dashIfEmpty(string(h.Verdict)))
		b.WriteString("Why it was not accepted:\n\n")
		if n := h.UnresolvedCounters(); n > 0 {
			for _, id := range sortedCopy(h.Counter) {
				if e, ok := index[id]; ok {
					fmt.Fprintf(b, "- Counter-evidence `%s`: %s\n", e.ID, escape(e.Summary))
				}
			}
		}
		for _, cr := range c.CritiquesFor(h.ID) {
			if cr.Verdict == domain.VerdictAccept {
				continue
			}
			fmt.Fprintf(b, "- [%s] %s\n", cr.Category, escape(cr.Challenge))
		}
		lead, _ := c.Leading()
		fmt.Fprintf(b, "- Scored %.2f below the accepted explanation\n\n",
			lead.Breakdown.Total-h.Breakdown.Total)
	}
	for _, r := range c.RejectedHypotheses {
		fmt.Fprintf(b, "- Rejected outright: %s (%s)\n", r.Target, r.Reason)
	}
	b.WriteString("\n")
}

func writeCritiques(b *strings.Builder, c *domain.Case) {
	b.WriteString("## Adversarial review\n\n")
	if len(c.Critiques) == 0 {
		b.WriteString(absent + "\n\n")
		return
	}
	b.WriteString("| Critique | Hypothesis | Rule | Verdict | Challenge |\n|---|---|---|---|---|\n")
	for _, cr := range c.Critiques {
		fmt.Fprintf(b, "| `%s` | `%s` | %s | %s | %s |\n",
			cr.ID, cr.HypothesisID, cr.Rule, cr.Verdict, escape(cr.Challenge))
	}
	b.WriteString("\n")

	if len(c.Demands) > 0 {
		b.WriteString("### Evidence the critic demanded\n\n")
		b.WriteString("| Demand | Kind | Answered by | Descriptor |\n|---|---|---|---|\n")
		for _, d := range c.Demands {
			answered := "**unmet**"
			if d.Satisfied() {
				answered = "`" + d.SatisfiedBy + "`"
			}
			fmt.Fprintf(b, "| `%s` | %s | %s | %s |\n", d.ID, d.Kind, answered, escape(d.Descriptor))
		}
		b.WriteString("\n")
	}
}

func writeActions(b *strings.Builder, c *domain.Case) {
	b.WriteString("## Remediation\n\n")
	if len(c.Actions) == 0 {
		b.WriteString(absent + "\n\n")
		return
	}
	for _, a := range c.Actions {
		fmt.Fprintf(b, "### %s\n\n", a.Title)
		fmt.Fprintf(b, "- Risk: **%s**\n", a.Risk)
		fmt.Fprintf(b, "- Call: `%s %s`\n", a.Tool, renderArgs(a.Args))
		if a.Rollback != nil {
			fmt.Fprintf(b, "- Rollback: `%s %s`\n", a.Rollback.Tool, renderArgs(a.Rollback.Args))
		}
		if len(a.Verify) > 0 {
			fmt.Fprintf(b, "- Verified by: %s\n", strings.Join(a.Verify, ", "))
		}
		for _, p := range a.Preconditions {
			fmt.Fprintf(b, "- Precondition: %s\n", p)
		}
		if a.Rationale != "" {
			fmt.Fprintf(b, "- Rationale: %s\n", a.Rationale)
		}
		switch {
		case a.Approval == nil:
			b.WriteString("- Approval: **not yet decided**\n")
		default:
			fmt.Fprintf(b, "- Approval: **%s** by %s at %s%s\n",
				a.Approval.Decision, a.Approval.By,
				a.Approval.At.UTC().Format(time.RFC3339), commentSuffix(a.Approval.Comment))
		}
		switch {
		case a.Execution == nil:
			b.WriteString("- Execution: not attempted\n\n")
		case a.Execution.Outcome == "executed":
			fmt.Fprintf(b, "- Execution: **executed** in %s — %s\n\n",
				a.Execution.Duration, a.Execution.Detail)
		default:
			fmt.Fprintf(b, "- Execution: **%s** (%s) — %s\n\n",
				a.Execution.Outcome, a.Execution.Rule, a.Execution.Detail)
		}
	}
}

func commentSuffix(s string) string {
	if s == "" {
		return ""
	}
	return " — " + s
}

func writeVerification(b *strings.Builder, c *domain.Case) {
	b.WriteString("## Recovery verification\n\n")
	if len(c.Verifications) == 0 {
		b.WriteString(absent + "\n\n")
		return
	}
	b.WriteString("| Signal | Before | After | Recovered |\n|---|---|---|---|\n")
	for _, v := range c.Verifications {
		fmt.Fprintf(b, "| %s | %s | %s | %s |\n", v.Signal, v.Before, v.After, yesNo(v.Recovered))
	}
	b.WriteString("\n")
}

func writeUnmet(b *strings.Builder, c *domain.Case) {
	b.WriteString("## Unmet evidence demands\n\n")
	unmet := c.UnsatisfiedDemands()
	if len(unmet) == 0 {
		b.WriteString("_all evidence the critic demanded was collected_\n\n")
		return
	}
	for _, d := range unmet {
		fmt.Fprintf(b, "- `%s` %s — %s\n", d.ID, escape(d.Descriptor), escape(d.Reason))
	}
	b.WriteString("\n")
}

func writeValue(b *strings.Builder, c *domain.Case) {
	b.WriteString("## What the multi-agent flow contributed\n\n")
	kinds := domain.DistinctKinds(c.Evidence)
	names := make([]string, 0, len(kinds))
	for _, k := range kinds {
		names = append(names, string(k))
	}
	fmt.Fprintf(b, "- Collection rounds: **%d**\n", c.Round)
	fmt.Fprintf(b, "- Evidence kinds gathered: **%d** (%s)\n", len(kinds), strings.Join(names, ", "))
	fmt.Fprintf(b, "- Evidence items: **%d**\n", len(c.Evidence))
	fmt.Fprintf(b, "- Critiques raised: **%d**, of which **%d** were not a plain accept\n",
		len(c.Critiques), c.CriticCorrections)
	fmt.Fprintf(b, "- Evidence demanded by the critic: **%d**, answered: **%d**\n",
		len(c.Demands), len(c.Demands)-len(c.UnsatisfiedDemands()))
	blocked := 0
	for _, a := range c.Actions {
		if a.Risk.RequiresApproval() {
			blocked++
		}
	}
	fmt.Fprintf(b, "- Actions held at the approval gate: **%d**\n", blocked)
	if c.HumanReviewed {
		b.WriteString("- The case was escalated to human review\n")
	}
	b.WriteString("\n")
}

// JSON renders the projection as a machine-readable report.
func JSON(c *domain.Case) ([]byte, error) { return json.MarshalIndent(c, "", "  ") }

func renderArgs(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, m[k]))
	}
	return strings.Join(parts, " ")
}

func sortedCopy(v []string) []string {
	out := append([]string(nil), v...)
	sort.Strings(out)
	return out
}

func escape(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	return strings.ReplaceAll(s, "\n", " ")
}

func dashIfEmpty(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
