package sdd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// sdd:impl DLD-0106

// Lock is the sealed baseline: the content hash of every item at the moment the
// specification tree was last accepted (CON-003).
type Lock struct {
	SDDVersion string               `json:"sddVersion"`
	SealedAt   string               `json:"sealedAt"`
	Items      map[string]LockEntry `json:"items"`
}

// LockEntry is one sealed item.
type LockEntry struct {
	Hash   string `json:"hash"`
	Stage  string `json:"stage"`
	Status string `json:"status"`
	File   string `json:"file"`
}

// DriftResult describes how the working tree differs from the sealed baseline.
type DriftResult struct {
	Sealed    bool     `json:"sealed"`
	SealedAt  string   `json:"sealedAt,omitempty"`
	Modified  []string `json:"modified"`
	Added     []string `json:"added"`
	Removed   []string `json:"removed"`
	Stale     []string `json:"stale"`
	StaleFile []string `json:"staleFiles"`
}

// Clean reports whether the tree matches its sealed baseline exactly.
func (d *DriftResult) Clean() bool {
	return len(d.Modified) == 0 && len(d.Added) == 0 &&
		len(d.Removed) == 0 && len(d.Stale) == 0
}

// LoadLock reads the sealed baseline. A missing lock file yields an unsealed lock
// rather than an error, so a fresh repository can be sealed for the first time.
func LoadLock(root, rel string) (*Lock, bool, error) {
	path := filepath.Join(root, rel)
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Lock{Items: map[string]LockEntry{}}, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var l Lock
	if err := json.Unmarshal(raw, &l); err != nil {
		return nil, false, fmt.Errorf("parse lock %s: %w", path, err)
	}
	if l.Items == nil {
		l.Items = map[string]LockEntry{}
	}
	return &l, true, nil
}

// Drift compares current item hashes against the sealed baseline and computes the
// transitive stale set — the exact blast radius of every upstream change.
func (m *Model) Drift() (*DriftResult, error) {
	lock, sealed, err := LoadLock(m.Root, m.Config.LockFile)
	if err != nil {
		return nil, err
	}
	res := &DriftResult{Sealed: sealed, SealedAt: lock.SealedAt}
	if !sealed {
		return res, nil
	}

	for _, id := range m.Order {
		entry, ok := lock.Items[id]
		if !ok {
			res.Added = append(res.Added, id)
			continue
		}
		if entry.Hash != m.Items[id].ContentHash() {
			res.Modified = append(res.Modified, id)
		}
	}
	for id := range lock.Items {
		if _, ok := m.Items[id]; !ok {
			res.Removed = append(res.Removed, id)
		}
	}
	sort.Strings(res.Removed)

	// Everything downstream of a changed or removed item must be revisited.
	roots := append(append([]string{}, res.Modified...), res.Removed...)
	changed := map[string]bool{}
	for _, id := range roots {
		changed[id] = true
	}
	for _, d := range m.DescendantsOf(roots...) {
		if strings.HasPrefix(d, "file:") {
			res.StaleFile = append(res.StaleFile, strings.TrimPrefix(d, "file:"))
			continue
		}
		if changed[d] {
			// Already reported as directly modified.
			continue
		}
		res.Stale = append(res.Stale, d)
	}
	sort.Strings(res.Stale)
	sort.Strings(res.StaleFile)
	res.StaleFile = dedupe(res.StaleFile)
	return res, nil
}

// Report converts a drift result into governance findings.
func (d *DriftResult) Report(cfg *Config) *Report {
	rep := &Report{}
	if !d.Sealed {
		rep.Add(cfg, "unsealedItem", "(tree)", cfg.LockFile, 0,
			"specification tree has never been sealed; run `sddctl seal`")
		return rep
	}
	for _, id := range d.Modified {
		rep.Add(cfg, "modifiedItem", id, "", 0,
			"content changed since the last seal; re-seal after updating descendants")
	}
	for _, id := range d.Added {
		rep.Add(cfg, "unsealedItem", id, "", 0, "new item not present in the sealed baseline")
	}
	for _, id := range d.Removed {
		rep.Add(cfg, "removedItem", id, "", 0,
			"item was sealed but no longer exists; descendants are stale")
	}
	for _, id := range d.Stale {
		rep.Add(cfg, "staleDownstream", id, "", 0,
			"an upstream item changed; this artifact must be revisited (CON-003)")
	}
	for _, f := range d.StaleFile {
		rep.Add(cfg, "staleDownstream", f, f, 0,
			"source file implements a changed design item and must be revisited")
	}
	return rep
}

// Seal records the current content hashes as the accepted baseline.
func (m *Model) Seal() (*Lock, error) {
	lock := &Lock{
		SDDVersion: m.Config.SDDVersion,
		SealedAt:   time.Now().UTC().Format(time.RFC3339),
		Items:      make(map[string]LockEntry, len(m.Items)),
	}
	for _, id := range m.Order {
		it := m.Items[id]
		lock.Items[id] = LockEntry{
			Hash:   it.ContentHash(),
			Stage:  it.Stage,
			Status: it.Status,
			File:   it.PrimaryFile(m.Config.Bilingual.Primary),
		}
	}
	path := filepath.Join(m.Root, m.Config.LockFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	raw, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		return nil, err
	}
	return lock, nil
}

// ImpactOf returns the full downstream impact set of hypothetically changing the
// given identifiers. This is what makes a goal amendment honest about its cost:
// the cascade is printed before any edit is made.
func (m *Model) ImpactOf(ids ...string) (items []string, files []string) {
	for _, d := range m.DescendantsOf(ids...) {
		if strings.HasPrefix(d, "file:") {
			files = append(files, strings.TrimPrefix(d, "file:"))
		} else {
			items = append(items, d)
		}
	}
	sort.Strings(items)
	sort.Strings(files)
	return items, dedupe(files)
}
