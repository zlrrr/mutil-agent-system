package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/zlrrr/mutil-agent-system/internal/catalog"
	"github.com/zlrrr/mutil-agent-system/internal/domain"
	"github.com/zlrrr/mutil-agent-system/internal/reasoner"
	"github.com/zlrrr/mutil-agent-system/internal/signal"
)

// sdd:impl DLD-1041

// Analysis returns the agent that forms hypotheses. It delegates entirely to the
// reasoner port, so swapping the reasoning strategy changes nothing here.
func Analysis(r reasoner.Reasoner) Agent {
	return New(domain.RoleAnalysis, func(ctx context.Context, s domain.Snapshot) ([]domain.Contribution, error) {
		hs, err := r.Hypothesise(ctx, s)
		if err != nil {
			return nil, fmt.Errorf("hypothesise: %w", err)
		}
		out := make([]domain.Contribution, 0, len(hs))
		for i, h := range hs {
			out = append(out, domain.ProposeHypothesis(domain.RoleAnalysis, i, h))
		}
		return out, nil
	})
}

// Critic returns the adversarial agent. It may challenge, demand and veto — and it may
// never propose a hypothesis of its own, which the role capability table enforces.
func Critic(r reasoner.Reasoner) Agent {
	return New(domain.RoleCritic, func(ctx context.Context, s domain.Snapshot) ([]domain.Contribution, error) {
		critiques, demands, err := r.Critique(ctx, s)
		if err != nil {
			return nil, fmt.Errorf("critique: %w", err)
		}
		out := make([]domain.Contribution, 0, len(critiques)+len(demands))
		i := 0
		for _, d := range demands {
			out = append(out, domain.DemandEvidence(domain.RoleCritic, i, d))
			i++
		}
		for _, c := range critiques {
			out = append(out, domain.RaiseCritique(domain.RoleCritic, i, c))
			i++
		}
		return out, nil
	})
}

// Remediation returns the agent that proposes an action for the accepted hypothesis.
// Arguments are materialised from the matched change evidence, so the action reflects
// what actually changed rather than a template default.
func Remediation(cat *catalog.Catalog, _ reasoner.Config) Agent {
	return New(domain.RoleRemediation, func(_ context.Context, s domain.Snapshot) ([]domain.Contribution, error) {
		// The accepted explanation, not the highest-scoring one. Acting on an
		// explanation the critic rejected would remediate a red herring — in C6, the
		// pool change that landed after the incident began.
		lead, ok := s.Leading()
		if !ok {
			return nil, nil
		}
		sig, ok := cat.Signature(lead.SignatureID)
		if !ok || sig.Remediation == nil {
			return nil, nil
		}
		action, err := buildAction(sig, lead, s)
		if err != nil {
			return nil, err
		}
		return []domain.Contribution{domain.ProposeAction(domain.RoleRemediation, 0, action)}, nil
	})
}

func buildAction(sig catalog.Signature, h domain.Hypothesis, s domain.Snapshot) (domain.Action, error) {
	tmpl := sig.Remediation
	args := map[string]string{}
	for k, v := range tmpl.Args {
		args[k] = v
	}
	if args["service"] == "" {
		args["service"] = s.Alert.Service
	}

	rollbackArgs := map[string]string{}
	for k, v := range tmpl.RollbackArg {
		rollbackArgs[k] = v
	}

	// Restore the value the change evidence says was replaced. This is what makes the
	// proposal specific to the incident rather than to the signature.
	if key := tmpl.RestoreFromChange; key != "" {
		for _, e := range s.EvidenceOfKind(domain.KindChange) {
			if !strings.EqualFold(e.Fact("change_key"), key) {
				continue
			}
			args["key"] = e.Fact("change_key")
			args["value"] = e.Fact("change_old")
			rollbackArgs = map[string]string{
				"service": args["service"],
				"key":     e.Fact("change_key"),
				"value":   e.Fact("change_new"),
			}
			break
		}
	}
	if args["value"] == "" {
		return domain.Action{}, fmt.Errorf(
			"remediation for %s has no value to apply: no change evidence supplied one",
			sig.ID)
	}
	if len(rollbackArgs) == 0 {
		return domain.Action{}, fmt.Errorf("remediation for %s has no rollback", sig.ID)
	}

	return domain.Action{
		Title:         tmpl.Title,
		Tool:          tmpl.Tool,
		Args:          args,
		Risk:          tmpl.Risk,
		Rationale:     tmpl.Rationale,
		Preconditions: append([]string(nil), tmpl.Precond...),
		Rollback:      &domain.ActionCall{Tool: tmpl.Tool, Args: rollbackArgs},
		Verify:        append([]string(nil), tmpl.Verify...),
		HypothesisID:  h.ID,
	}, nil
}

// Verification returns the agent that re-queries the triggering signals after an
// action ran. It compares the incident window against a post-action window and reports
// per signal whether the value recovered.
func Verification(set signal.Set, _ reasoner.Config) Agent {
	return New(domain.RoleVerification, func(ctx context.Context, s domain.Snapshot) ([]domain.Contribution, error) {
		action, ok := executedAction(s)
		if !ok {
			return nil, nil
		}
		post := domain.TimeWindow{Start: s.Window.End, End: s.Window.End.Add(10 * time.Minute)}

		var out []domain.Contribution
		i := 0
		for _, name := range action.Verify {
			before, errB := meanOver(ctx, set, s.Alert.Service, name, s.Window)
			after, errA := meanOver(ctx, set, s.Alert.Service, name, post)
			if errB != nil || errA != nil {
				continue
			}
			recovered := after <= before*0.5
			detail := fmt.Sprintf("%s moved from a mean of %.4f during the incident to %.4f after the action",
				name, before, after)

			out = append(out, domain.AddEvidence(domain.RoleVerification, i, domain.Evidence{
				Kind:       domain.KindVerification,
				Agent:      domain.RoleVerification,
				Source:     "metrics",
				Window:     post,
				Summary:    detail,
				RawRef:     fmt.Sprintf("promql:%s{service=%q}@post-action", name, s.Alert.Service),
				Confidence: 0.9,
				Facts: map[string]string{
					"signal":    name,
					"before":    fmt.Sprintf("%.4f", before),
					"after":     fmt.Sprintf("%.4f", after),
					"recovered": boolText(recovered),
					"action_id": action.ID,
				},
			}))
			i++
			out = append(out, domain.RecordVerification(domain.RoleVerification, i, domain.VerificationResult{
				ActionID:  action.ID,
				Signal:    name,
				Before:    fmt.Sprintf("%.4f", before),
				After:     fmt.Sprintf("%.4f", after),
				Recovered: recovered,
				Detail:    detail,
			}))
			i++
		}
		return out, nil
	})
}

func executedAction(s domain.Snapshot) (domain.Action, bool) {
	for i := len(s.Actions) - 1; i >= 0; i-- {
		a := s.Actions[i]
		if a.Execution != nil && a.Execution.Outcome == "executed" {
			return a, true
		}
	}
	return domain.Action{}, false
}

func meanOver(ctx context.Context, set signal.Set, service, name string, w domain.TimeWindow) (float64, error) {
	series, err := set.Metrics.Range(ctx, signal.MetricQuery{Service: service, Series: name, Window: w})
	if err != nil {
		return 0, err
	}
	var sum float64
	var n int
	for _, p := range series.Points {
		if w.Contains(p.At) {
			sum += p.Value
			n++
		}
	}
	if n == 0 {
		return 0, fmt.Errorf("series %s has no samples in %s", name, w)
	}
	return sum / float64(n), nil
}

func boolText(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// sdd:impl DLD-1042

// SingleAgentBaseline returns the comparison mode: one agent with every port, running
// each collector's default pass once, then hypothesising, then proposing an action.
//
// It is the honest model of "one context window, one pass". It is given the same tools
// and the same default queries as the multi-agent flow and differs only in the absence
// of the adversarial round — so a difference in outcome is attributable to the flow.
func SingleAgentBaseline(set signal.Set, r reasoner.Reasoner, cfg reasoner.Config, cat *catalog.Catalog) Agent {
	return New(domain.RoleBaseline, func(ctx context.Context, s domain.Snapshot) ([]domain.Contribution, error) {
		var out []domain.Contribution
		index := 0

		for _, c := range Collectors(set, cfg) {
			cs, err := c.Run(ctx, s)
			if err != nil {
				continue
			}
			for _, contrib := range cs {
				if contrib.Kind != domain.ContribAddEvidence {
					continue
				}
				contrib.Role = domain.RoleBaseline
				contrib.Index = index
				contrib.Evidence.Agent = domain.RoleBaseline
				out = append(out, contrib)
				index++
			}
		}

		// Hypothesise over the evidence this single pass gathered, which the caller
		// has not yet applied — so build the snapshot the reasoner needs.
		local := s
		local.Evidence = append(append([]domain.Evidence(nil), s.Evidence...), evidenceOf(out)...)
		local.Index = indexOf(local.Evidence)

		hs, err := r.Hypothesise(ctx, local)
		if err != nil {
			return nil, err
		}
		for i, h := range hs {
			out = append(out, domain.ProposeHypothesis(domain.RoleBaseline, index+i, h))
		}
		if len(hs) == 0 {
			return out, nil
		}

		if sig, ok := cat.Signature(hs[0].SignatureID); ok && sig.Remediation != nil {
			local.Hypotheses = hs
			if action, err := buildAction(sig, hs[0], local); err == nil {
				out = append(out, domain.ProposeAction(domain.RoleBaseline, index+len(hs), action))
			}
		}
		return out, nil
	})
}

func evidenceOf(cs []domain.Contribution) []domain.Evidence {
	var out []domain.Evidence
	for i, c := range cs {
		if c.Kind != domain.ContribAddEvidence || c.Evidence == nil {
			continue
		}
		e := *c.Evidence
		if e.ID == "" {
			// A provisional identifier so the reasoner can reference it; the
			// orchestrator assigns the durable one when it applies the contribution.
			e.ID = fmt.Sprintf("e-%s-%03d", domain.RoleBaseline, i+1)
		}
		out = append(out, e)
	}
	return out
}

func indexOf(ev []domain.Evidence) map[string]domain.Evidence {
	out := make(map[string]domain.Evidence, len(ev))
	for _, e := range ev {
		out[e.ID] = e
	}
	return out
}

// SortRoles returns roles in their canonical order, for deterministic rendering.
func SortRoles(roles []domain.Role) []domain.Role {
	out := append([]domain.Role(nil), roles...)
	sort.Slice(out, func(i, j int) bool {
		oi, iok := domain.CollectorOrder(out[i])
		oj, jok := domain.CollectorOrder(out[j])
		if iok && jok {
			return oi < oj
		}
		if iok != jok {
			return iok
		}
		return out[i] < out[j]
	})
	return out
}
