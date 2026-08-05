package sdd

import (
	"fmt"
	"sort"
	"strings"
)

// sdd:impl DLD-0104

// Model is the fully resolved specification tree: documents, merged items, the
// derivation graph and every code anchor pointing into it.
type Model struct {
	Root    string
	Config  *Config
	Docs    []*Doc
	Items   map[string]*Item
	Order   []string // item IDs in stage order, then lexical
	Anchors []CodeAnchor

	children map[string][]string
	parseRep *Report
}

// Load parses every document and source file under root and builds the model.
func Load(root string) (*Model, error) {
	cfg, err := LoadConfig(root)
	if err != nil {
		return nil, err
	}
	return LoadWithConfig(root, cfg)
}

// LoadWithConfig builds the model using an already-loaded configuration.
func LoadWithConfig(root string, cfg *Config) (*Model, error) {
	m := &Model{
		Root:     root,
		Config:   cfg,
		Items:    map[string]*Item{},
		children: map[string][]string{},
		parseRep: &Report{},
	}

	paths, err := CollectDocs(root, cfg.DocRoots)
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)

	for _, p := range paths {
		doc, items, err := ParseDoc(root, p)
		if err != nil {
			return nil, err
		}
		m.Docs = append(m.Docs, doc)

		seenInDoc := map[string]bool{}
		for _, it := range items {
			if it.ID == "" {
				m.parseRep.Add(cfg, "duplicateId", "(anonymous)", doc.RelPath, it.Occurrences[0].Line,
					"sdd:item anchor without an id attribute")
				continue
			}
			if seenInDoc[it.ID] {
				m.parseRep.Add(cfg, "duplicateId", it.ID, doc.RelPath, it.Occurrences[0].Line,
					"identifier declared twice in the same document")
				continue
			}
			seenInDoc[it.ID] = true
			m.mergeItem(it, doc)
		}
	}

	anchors, err := ParseCodeAnchors(root, cfg.CodeRoots)
	if err != nil {
		return nil, err
	}
	m.Anchors = anchors

	m.buildOrder()
	m.buildChildren()
	return m, nil
}

func (m *Model) mergeItem(it *Item, doc *Doc) {
	existing, ok := m.Items[it.ID]
	if !ok {
		m.Items[it.ID] = it
		return
	}
	// A second language rendering of the same item: attributes must agree,
	// because the pair is one artifact (CON-004).
	if existing.Stage != it.Stage || existing.Status != it.Status ||
		existing.Priority != it.Priority ||
		strings.Join(existing.DerivesFrom, ",") != strings.Join(it.DerivesFrom, ",") {
		m.parseRep.Add(m.Config, "bilingualStructureMismatch", it.ID, doc.RelPath,
			it.Occurrences[0].Line,
			"anchor attributes differ from the counterpart document (%s)",
			existing.Occurrences[0].File)
	}
	existing.Occurrences = append(existing.Occurrences, it.Occurrences...)
	sort.SliceStable(existing.Occurrences, func(i, j int) bool {
		return existing.Occurrences[i].Lang < existing.Occurrences[j].Lang
	})
	if existing.Title == "" {
		existing.Title = it.Title
	}
}

func (m *Model) buildOrder() {
	stageRank := map[string]int{}
	for i, s := range m.Config.Stages {
		stageRank[s.Prefix] = i
	}
	m.Order = make([]string, 0, len(m.Items))
	for id := range m.Items {
		m.Order = append(m.Order, id)
	}
	sort.Slice(m.Order, func(i, j int) bool {
		a, b := m.Order[i], m.Order[j]
		ra, oka := stageRank[PrefixOf(a)]
		rb, okb := stageRank[PrefixOf(b)]
		if !oka {
			ra = len(stageRank) + 1
		}
		if !okb {
			rb = len(stageRank) + 1
		}
		if ra != rb {
			return ra < rb
		}
		return a < b
	})
}

func (m *Model) buildChildren() {
	m.children = map[string][]string{}
	for _, id := range m.Order {
		for _, p := range m.Items[id].DerivesFrom {
			m.children[p] = append(m.children[p], id)
		}
	}
	// Code anchors are leaves of the graph: they descend from their target item.
	for _, a := range m.Anchors {
		for _, t := range a.Targets {
			key := anchorKey(a)
			m.children[t] = append(m.children[t], key)
		}
	}
	for k := range m.children {
		sort.Strings(m.children[k])
		m.children[k] = dedupe(m.children[k])
	}
}

func anchorKey(a CodeAnchor) string {
	return "file:" + a.File
}

func dedupe(in []string) []string {
	out := in[:0]
	var last string
	for i, v := range in {
		if i == 0 || v != last {
			out = append(out, v)
		}
		last = v
	}
	return out
}

// ChildrenOf returns the direct descendants of an identifier, which may include
// synthetic `file:` nodes for source files carrying anchors.
func (m *Model) ChildrenOf(id string) []string { return m.children[id] }

// DescendantsOf returns every transitive descendant of an identifier, including
// source-file nodes. Used to compute the blast radius of a change (CON-003).
func (m *Model) DescendantsOf(ids ...string) []string {
	seen := map[string]bool{}
	var walk func(string)
	walk = func(id string) {
		for _, c := range m.children[id] {
			if seen[c] {
				continue
			}
			seen[c] = true
			walk(c)
		}
	}
	for _, id := range ids {
		walk(id)
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ItemsByPrefix returns every item whose identifier carries the given prefix,
// in model order.
func (m *Model) ItemsByPrefix(prefix string) []*Item {
	var out []*Item
	for _, id := range m.Order {
		if PrefixOf(id) == prefix {
			out = append(out, m.Items[id])
		}
	}
	return out
}

// AnchorsFor returns all code anchors of a kind targeting an identifier.
func (m *Model) AnchorsFor(kind, id string) []CodeAnchor {
	var out []CodeAnchor
	for _, a := range m.Anchors {
		if a.Kind != kind {
			continue
		}
		for _, t := range a.Targets {
			if t == id {
				out = append(out, a)
				break
			}
		}
	}
	return out
}

// Stats summarises the tree for CLI output.
func (m *Model) Stats() string {
	var parts []string
	for _, s := range m.Config.Stages {
		if n := len(m.ItemsByPrefix(s.Prefix)); n > 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", s.Prefix, n))
		}
	}
	return fmt.Sprintf("%d docs, %d items (%s), %d code anchors",
		len(m.Docs), len(m.Items), strings.Join(parts, " "), len(m.Anchors))
}
