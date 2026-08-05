package sdd

import (
	"fmt"
	"sort"
	"strings"
)

// sdd:impl DLD-0107

// GateResult is the outcome of a stage gate evaluation.
type GateResult struct {
	Stage    string      `json:"stage"`
	Passed   bool        `json:"passed"`
	Checks   []GateCheck `json:"checks"`
	Unknowns []string    `json:"unknownRequirements,omitempty"`
}

// GateCheck is one precondition evaluated by a gate.
type GateCheck struct {
	Name    string   `json:"name"`
	Passed  bool     `json:"passed"`
	Detail  string   `json:"detail,omitempty"`
	Blocked []string `json:"blocking,omitempty"`
}

// Gate evaluates the preconditions for entering a development stage (CON-002).
func (m *Model) Gate(stage string) (*GateResult, error) {
	gc, ok := m.Config.GateByStage(stage)
	if !ok {
		return nil, fmt.Errorf("no gate declared for stage %q", stage)
	}
	res := &GateResult{Stage: stage, Passed: true}
	for _, req := range gc.Requires {
		chk, known := m.evaluateRequirement(req)
		if !known {
			res.Unknowns = append(res.Unknowns, req)
			res.Passed = false
			continue
		}
		res.Checks = append(res.Checks, chk)
		if !chk.Passed {
			res.Passed = false
		}
	}
	return res, nil
}

func (m *Model) evaluateRequirement(req string) (GateCheck, bool) {
	chk := GateCheck{Name: req, Passed: true}

	if strings.HasSuffix(req, "-approved") {
		stageName := strings.TrimSuffix(req, "-approved")
		sc, ok := m.Config.StageByName(stageName)
		if !ok {
			return chk, false
		}
		var pending []string
		for _, it := range m.ItemsByPrefix(sc.Prefix) {
			if it.Status != "approved" && it.Status != "superseded" && it.Status != "deferred" {
				pending = append(pending, fmt.Sprintf("%s(%s)", it.ID, statusOr(it.Status)))
			}
		}
		if len(pending) > 0 {
			chk.Passed = false
			chk.Blocked = pending
			chk.Detail = fmt.Sprintf("%d %s item(s) are not approved", len(pending), sc.Prefix)
		} else {
			chk.Detail = fmt.Sprintf("all %s items approved", sc.Prefix)
		}
		return chk, true
	}

	switch req {
	case "no-drift":
		d, err := m.Drift()
		if err != nil {
			chk.Passed, chk.Detail = false, err.Error()
			return chk, true
		}
		if !d.Sealed {
			chk.Passed, chk.Detail = false, "specification tree has never been sealed"
			return chk, true
		}
		if !d.Clean() {
			chk.Passed = false
			chk.Blocked = append(append(append([]string{}, d.Modified...), d.Stale...), d.Removed...)
			chk.Detail = fmt.Sprintf("%d modified, %d stale, %d added, %d removed",
				len(d.Modified), len(d.Stale), len(d.Added), len(d.Removed))
		} else {
			chk.Detail = "tree matches sealed baseline"
		}
		return chk, true

	case "no-orphan-goals":
		return m.checkCoverage(chk, "G", "REQ"), true
	case "no-orphan-requirements":
		return m.checkCoverage(chk, "REQ", "ARC"), true

	case "no-unimplemented-dld":
		var missing []string
		for _, it := range m.ItemsByPrefix("DLD") {
			if it.Status == "deferred" || it.Status == "superseded" {
				continue
			}
			if len(m.AnchorsFor("impl", it.ID)) == 0 {
				missing = append(missing, it.ID)
			}
		}
		if len(missing) > 0 {
			chk.Passed, chk.Blocked = false, missing
			chk.Detail = fmt.Sprintf("%d design item(s) have no implementation anchor", len(missing))
		} else {
			chk.Detail = "every design item is anchored in source"
		}
		return chk, true

	case "no-orphan-code":
		var bad []string
		for _, a := range m.Anchors {
			for _, t := range a.Targets {
				if _, ok := m.Items[t]; !ok {
					bad = append(bad, fmt.Sprintf("%s:%d->%s", a.File, a.Line, t))
				}
			}
		}
		if len(bad) > 0 {
			chk.Passed, chk.Blocked = false, bad
			chk.Detail = fmt.Sprintf("%d anchor(s) target missing items", len(bad))
		} else {
			chk.Detail = "every code anchor resolves"
		}
		return chk, true

	case "no-untested-requirements":
		covered := map[string]bool{}
		for _, tc := range m.ItemsByPrefix("TC") {
			verified := len(m.AnchorsFor("verify", tc.ID)) > 0
			for _, p := range tc.DerivesFrom {
				if PrefixOf(p) == "REQ" && verified {
					covered[p] = true
				}
			}
		}
		var missing []string
		for _, req := range m.ItemsByPrefix("REQ") {
			if req.Status == "superseded" || req.Status == "deferred" {
				continue
			}
			if !covered[req.ID] {
				missing = append(missing, req.ID)
			}
		}
		if len(missing) > 0 {
			chk.Passed, chk.Blocked = false, missing
			chk.Detail = fmt.Sprintf("%d requirement(s) have no executed test", len(missing))
		} else {
			chk.Detail = "every requirement is covered by an executed test case"
		}
		return chk, true

	case "bilingual-parity":
		rep := m.Lint()
		if rep.HasErrors() {
			chk.Passed = false
			for _, f := range rep.Sorted() {
				if f.Severity == SeverityError {
					chk.Blocked = append(chk.Blocked, f.Subject)
				}
			}
			chk.Detail = fmt.Sprintf("%d bilingual parity error(s)", rep.Count(SeverityError))
		} else {
			chk.Detail = "all document pairs are structurally identical"
		}
		return chk, true
	}

	return chk, false
}

func (m *Model) checkCoverage(chk GateCheck, parentPrefix, childPrefix string) GateCheck {
	covered := map[string]bool{}
	for _, it := range m.ItemsByPrefix(childPrefix) {
		for _, p := range it.DerivesFrom {
			covered[p] = true
		}
	}
	var missing []string
	for _, it := range m.ItemsByPrefix(parentPrefix) {
		if it.Status == "superseded" || it.Status == "deferred" {
			continue
		}
		if !covered[it.ID] {
			missing = append(missing, it.ID)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		chk.Passed, chk.Blocked = false, missing
		chk.Detail = fmt.Sprintf("%d %s item(s) have no %s descendant",
			len(missing), parentPrefix, childPrefix)
	} else {
		chk.Detail = fmt.Sprintf("every %s item is refined by a %s item", parentPrefix, childPrefix)
	}
	return chk
}

func statusOr(s string) string {
	if s == "" {
		return "unset"
	}
	return s
}
