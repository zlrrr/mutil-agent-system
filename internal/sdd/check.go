package sdd

import (
	"fmt"
	"sort"
	"strings"
)

// sdd:impl DLD-0105

// Lint enforces the bilingual document rules of CON-004.
func (m *Model) Lint() *Report {
	rep := &Report{}
	cfg := m.Config
	if !cfg.Bilingual.Enabled {
		return rep
	}

	exempt := map[string]bool{}
	for _, e := range cfg.Bilingual.Exempt {
		exempt[e] = true
	}

	byBase := map[string]map[string]*Doc{}
	for _, d := range m.Docs {
		if d.Lang == "" {
			base := d.RelPath[strings.LastIndex(d.RelPath, "/")+1:]
			if !exempt[base] && !exempt[d.RelPath] {
				rep.Add(cfg, "nonBilingualDoc", d.RelPath, d.RelPath, 0,
					"document has no language suffix; expected <base>.%s.md",
					strings.Join(cfg.Bilingual.Languages, "|"))
			}
			continue
		}
		if byBase[d.Base] == nil {
			byBase[d.Base] = map[string]*Doc{}
		}
		byBase[d.Base][d.Lang] = d
	}

	bases := make([]string, 0, len(byBase))
	for b := range byBase {
		bases = append(bases, b)
	}
	sort.Strings(bases)

	for _, base := range bases {
		group := byBase[base]
		for _, lang := range cfg.Bilingual.Languages {
			if _, ok := group[lang]; !ok {
				rep.Add(cfg, "bilingualMissingCounterpart", base, base+".*.md", 0,
					"missing the %s rendering of this document", lang)
			}
		}
		primary := group[cfg.Bilingual.Primary]
		if primary == nil {
			continue
		}
		for _, lang := range cfg.Bilingual.Languages {
			if lang == cfg.Bilingual.Primary {
				continue
			}
			other := group[lang]
			if other == nil {
				continue
			}
			m.compareDocs(rep, primary, other)
		}
	}
	return rep
}

func (m *Model) compareDocs(rep *Report, a, b *Doc) {
	cfg := m.Config

	av, bv := a.FrontMatter["doc_version"], b.FrontMatter["doc_version"]
	if av != bv {
		rep.Add(cfg, "bilingualVersionMismatch", a.Base, b.RelPath, 0,
			"doc_version %q (%s) != %q (%s); both languages must be updated together",
			av, a.Lang, bv, b.Lang)
	}
	if as, bs := a.FrontMatter["status"], b.FrontMatter["status"]; as != bs {
		rep.Add(cfg, "bilingualVersionMismatch", a.Base, b.RelPath, 0,
			"status %q (%s) != %q (%s)", as, a.Lang, bs, b.Lang)
	}

	if strings.Join(a.ItemIDs, ",") != strings.Join(b.ItemIDs, ",") {
		rep.Add(cfg, "bilingualStructureMismatch", a.Base, b.RelPath, 0,
			"item identifier sequence differs: %s has [%s], %s has [%s]",
			a.Lang, strings.Join(a.ItemIDs, " "), b.Lang, strings.Join(b.ItemIDs, " "))
	}

	al, bl := headingLevels(a.Headings), headingLevels(b.Headings)
	if al != bl {
		rep.Add(cfg, "bilingualStructureMismatch", a.Base, b.RelPath, 0,
			"heading skeleton differs: %s has %d headings [%s], %s has %d headings [%s]",
			a.Lang, len(a.Headings), al, b.Lang, len(b.Headings), bl)
	}
}

func headingLevels(hs []Heading) string {
	parts := make([]string, len(hs))
	for i, h := range hs {
		parts[i] = fmt.Sprint(h.Level)
	}
	return strings.Join(parts, ".")
}

// Trace validates the derivation graph: stage legality, orphans, cycles and code
// anchor integrity (CON-001, CON-002, CON-006).
func (m *Model) Trace() *Report {
	rep := &Report{}
	rep.Merge(m.parseRep)
	cfg := m.Config

	for _, id := range m.Order {
		it := m.Items[id]
		file := it.PrimaryFile(cfg.Bilingual.Primary)

		stageCfg, ok := cfg.StageByPrefix(PrefixOf(id))
		if !ok {
			rep.Add(cfg, "unknownParent", id, file, 0,
				"identifier prefix %q is not a configured stage", PrefixOf(id))
			continue
		}
		if it.Stage != "" && it.Stage != stageCfg.Name {
			rep.Add(cfg, "illegalParentStage", id, file, 0,
				"anchor declares stage %q but prefix %q belongs to stage %q",
				it.Stage, stageCfg.Prefix, stageCfg.Name)
		}

		if len(stageCfg.Parents) > 0 && len(it.DerivesFrom) == 0 {
			rep.Add(cfg, "orphanItem", id, file, 0,
				"stage %q requires derives_from one of [%s]",
				stageCfg.Name, strings.Join(stageCfg.Parents, ", "))
		}

		for _, p := range it.DerivesFrom {
			if _, exists := m.Items[p]; !exists {
				rep.Add(cfg, "unknownParent", id, file, 0,
					"derives_from %q which does not exist", p)
				continue
			}
			if !cfg.IsLegalParent(id, p) {
				rep.Add(cfg, "illegalParentStage", id, file, 0,
					"derives_from %q (stage %q) is not a legal parent of stage %q; "+
						"the chain may not be skipped",
					p, PrefixOf(p), stageCfg.Name)
			}
		}
	}

	if cycle := m.findCycle(); cycle != nil {
		rep.Add(cfg, "illegalParentStage", cycle[0], "", 0,
			"derivation cycle: %s", strings.Join(cycle, " -> "))
	}

	// Code anchors must target existing items of a permitted prefix.
	for _, a := range m.Anchors {
		spec, ok := cfg.CodeAnchors[a.Kind]
		if !ok {
			continue
		}
		for _, t := range a.Targets {
			if _, exists := m.Items[t]; !exists {
				rep.Add(cfg, "orphanCodeAnchor", t, a.File, a.Line,
					"sdd:%s anchor targets an item that does not exist", a.Kind)
				continue
			}
			allowed := false
			for _, p := range spec.TargetPrefixes {
				if PrefixOf(t) == p {
					allowed = true
					break
				}
			}
			if !allowed {
				rep.Add(cfg, "orphanCodeAnchor", t, a.File, a.Line,
					"sdd:%s anchor may only target [%s]",
					a.Kind, strings.Join(spec.TargetPrefixes, ", "))
			}
		}
	}

	// Every detailed-design item should be realised in code.
	for _, it := range m.ItemsByPrefix("DLD") {
		if it.Status == "deferred" || it.Status == "superseded" {
			continue
		}
		if len(m.AnchorsFor("impl", it.ID)) == 0 {
			rep.Add(cfg, "unimplementedDld", it.ID, it.PrimaryFile(cfg.Bilingual.Primary), 0,
				"no source file carries `// sdd:impl %s`", it.ID)
		}
	}

	// Every test case should be realised by a test function.
	for _, it := range m.ItemsByPrefix("TC") {
		if it.Status == "deferred" || it.Status == "superseded" {
			continue
		}
		if len(m.AnchorsFor("verify", it.ID)) == 0 {
			rep.Add(cfg, "unverifiedTestCase", it.ID, it.PrimaryFile(cfg.Bilingual.Primary), 0,
				"no test function carries `// sdd:verify %s`", it.ID)
		}
	}

	// Every requirement must be reachable from a test case.
	tcByReq := map[string][]string{}
	for _, tc := range m.ItemsByPrefix("TC") {
		for _, p := range tc.DerivesFrom {
			if PrefixOf(p) == "REQ" {
				tcByReq[p] = append(tcByReq[p], tc.ID)
			}
		}
	}
	for _, req := range m.ItemsByPrefix("REQ") {
		if req.Status == "superseded" {
			continue
		}
		if len(tcByReq[req.ID]) == 0 {
			rep.Add(cfg, "untestedRequirement", req.ID, req.PrimaryFile(cfg.Bilingual.Primary), 0,
				"no test case derives from this requirement (priority %s)", req.Priority)
		}
	}

	return rep
}

func (m *Model) findCycle() []string {
	const (
		white = 0
		grey  = 1
		black = 2
	)
	color := map[string]int{}
	var stack []string
	var cycle []string

	var visit func(string) bool
	visit = func(id string) bool {
		color[id] = grey
		stack = append(stack, id)
		for _, p := range m.Items[id].DerivesFrom {
			if _, ok := m.Items[p]; !ok {
				continue
			}
			switch color[p] {
			case grey:
				for i, s := range stack {
					if s == p {
						cycle = append(append([]string{}, stack[i:]...), p)
						return true
					}
				}
				cycle = []string{id, p}
				return true
			case white:
				if visit(p) {
					return true
				}
			}
		}
		stack = stack[:len(stack)-1]
		color[id] = black
		return false
	}

	for _, id := range m.Order {
		if color[id] == white {
			if visit(id) {
				return cycle
			}
		}
	}
	return nil
}

// Validate runs every check and returns the combined report.
func (m *Model) Validate() *Report {
	rep := &Report{}
	rep.Merge(m.Lint())
	rep.Merge(m.Trace())
	drift, err := m.Drift()
	if err == nil {
		rep.Merge(drift.Report(m.Config))
	}
	return rep
}
