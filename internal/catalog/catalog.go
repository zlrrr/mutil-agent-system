package catalog

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/zlrrr/mutil-agent-system/internal/signal"
)

// sdd:impl DLD-1030

//go:embed data
var data embed.FS

// Catalog is the loaded signature, case and runbook set.
type Catalog struct {
	Signatures []Signature      `json:"signatures"`
	Cases      []FaultCase      `json:"cases"`
	Runbooks   []signal.Runbook `json:"runbooks"`
}

type signatureFile struct {
	Signatures []Signature `json:"signatures"`
}

type runbookFile struct {
	Runbooks []signal.Runbook `json:"runbooks"`
}

// Load parses the embedded catalog. A duplicate identifier or a dangling reference
// fails the load, and therefore fails startup: a broken catalog must not run.
func Load() (*Catalog, error) { return loadFS(data, "data") }

func loadFS(fsys fs.FS, root string) (*Catalog, error) {
	cat := &Catalog{}

	names, err := listJSON(fsys, root)
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		raw, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		base := name[strings.LastIndexByte(name, '/')+1:]
		switch {
		case strings.HasPrefix(base, "signatures"):
			var f signatureFile
			if err := json.Unmarshal(raw, &f); err != nil {
				return nil, fmt.Errorf("parse %s: %w", name, err)
			}
			cat.Signatures = append(cat.Signatures, f.Signatures...)
		case strings.HasPrefix(base, "runbooks"):
			var f runbookFile
			if err := json.Unmarshal(raw, &f); err != nil {
				return nil, fmt.Errorf("parse %s: %w", name, err)
			}
			cat.Runbooks = append(cat.Runbooks, f.Runbooks...)
		case strings.HasPrefix(base, "case-"):
			var fc FaultCase
			if err := json.Unmarshal(raw, &fc); err != nil {
				return nil, fmt.Errorf("parse %s: %w", name, err)
			}
			cat.Cases = append(cat.Cases, fc)
		}
	}

	if err := cat.validate(); err != nil {
		return nil, err
	}
	return cat, nil
}

func listJSON(fsys fs.FS, root string) ([]string, error) {
	var out []string
	err := fs.WalkDir(fsys, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".json") {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk catalog: %w", err)
	}
	sort.Strings(out) // file name order, so iteration is deterministic
	return out, nil
}

func (c *Catalog) validate() error {
	seenSig := map[string]bool{}
	for _, s := range c.Signatures {
		if s.ID == "" {
			return fmt.Errorf("catalog: signature with empty id")
		}
		if seenSig[s.ID] {
			return fmt.Errorf("catalog: duplicate signature id %q", s.ID)
		}
		seenSig[s.ID] = true
		if len(s.Requires) == 0 {
			return fmt.Errorf("catalog: signature %q requires no evidence", s.ID)
		}
	}

	seenRunbook := map[string]bool{}
	for _, r := range c.Runbooks {
		if seenRunbook[r.ID] {
			return fmt.Errorf("catalog: duplicate runbook id %q", r.ID)
		}
		seenRunbook[r.ID] = true
	}
	for _, s := range c.Signatures {
		for _, id := range s.RunbookIDs {
			if !seenRunbook[id] {
				return fmt.Errorf("catalog: signature %q references unknown runbook %q", s.ID, id)
			}
		}
	}

	seenCase := map[string]bool{}
	for _, fc := range c.Cases {
		if fc.ID == "" {
			return fmt.Errorf("catalog: fault case with empty id")
		}
		if seenCase[fc.ID] {
			return fmt.Errorf("catalog: duplicate case id %q", fc.ID)
		}
		seenCase[fc.ID] = true
		if !seenSig[fc.ExpectedSignature] {
			return fmt.Errorf("catalog: case %q expects unknown signature %q",
				fc.ID, fc.ExpectedSignature)
		}
	}
	return nil
}

// Signature returns a signature by identifier.
func (c *Catalog) Signature(id string) (Signature, bool) {
	for _, s := range c.Signatures {
		if s.ID == id {
			return s, true
		}
	}
	return Signature{}, false
}

// Case returns a fault case by identifier.
func (c *Catalog) Case(id string) (FaultCase, bool) {
	for _, fc := range c.Cases {
		if fc.ID == id {
			return fc, true
		}
	}
	return FaultCase{}, false
}

// Runbook returns a runbook by identifier.
func (c *Catalog) Runbook(id string) (signal.Runbook, bool) {
	for _, r := range c.Runbooks {
		if r.ID == id {
			return r, true
		}
	}
	return signal.Runbook{}, false
}

// CaseIDs returns every fault case identifier in declaration order.
func (c *Catalog) CaseIDs() []string {
	out := make([]string, 0, len(c.Cases))
	for _, fc := range c.Cases {
		out = append(out, fc.ID)
	}
	return out
}
