// Package sdd implements the specification-driven-development governance engine:
// artifact parsing, traceability graph construction, bilingual parity linting,
// content sealing and cascading drift detection.
//
// The package has no third-party dependencies (CON-008).
package sdd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// sdd:impl DLD-0101

// Config is the machine-readable governance configuration loaded from
// .sdd/config.json. It defines the artifact chain, the legal parent stages and
// the severity of every finding class.
type Config struct {
	Project     string          `json:"project"`
	SDDVersion  string          `json:"sddVersion"`
	DocRoots    []string        `json:"docRoots"`
	CodeRoots   []string        `json:"codeRoots"`
	LockFile    string          `json:"lockFile"`
	Bilingual   BilingualConfig `json:"bilingual"`
	Stages      []StageConfig   `json:"stages"`
	CodeAnchors map[string]struct {
		Keyword        string   `json:"keyword"`
		TargetPrefixes []string `json:"targetPrefixes"`
		Description    string   `json:"description"`
	} `json:"codeAnchors"`
	Gates    []GateConfig      `json:"gates"`
	Severity map[string]string `json:"severity"`
}

// BilingualConfig declares the document pairing rules enforced by CON-004.
type BilingualConfig struct {
	Enabled   bool     `json:"enabled"`
	Languages []string `json:"languages"`
	Primary   string   `json:"primary"`
	Pattern   string   `json:"pattern"`
	Exempt    []string `json:"exempt"`
}

// StageConfig binds an identifier prefix to a stage and to its legal parents.
type StageConfig struct {
	Name    string   `json:"name"`
	Prefix  string   `json:"prefix"`
	Title   string   `json:"title"`
	Parents []string `json:"parents"`
}

// GateConfig declares the preconditions for entering a development stage.
type GateConfig struct {
	Stage       string   `json:"stage"`
	Requires    []string `json:"requires"`
	Description string   `json:"description"`
}

// DefaultConfigPath is the configuration location relative to the repository root.
const DefaultConfigPath = ".sdd/config.json"

// LoadConfig reads and validates the governance configuration at root.
func LoadConfig(root string) (*Config, error) {
	path := filepath.Join(root, DefaultConfigPath)
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read sdd config %s: %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse sdd config %s: %w", path, err)
	}
	if len(cfg.Stages) == 0 {
		return nil, fmt.Errorf("sdd config %s declares no stages", path)
	}
	seen := map[string]bool{}
	for _, s := range cfg.Stages {
		if s.Prefix == "" {
			return nil, fmt.Errorf("stage %q has an empty prefix", s.Name)
		}
		if seen[s.Prefix] {
			return nil, fmt.Errorf("duplicate stage prefix %q", s.Prefix)
		}
		seen[s.Prefix] = true
	}
	for _, s := range cfg.Stages {
		for _, p := range s.Parents {
			if !seen[p] {
				return nil, fmt.Errorf("stage %q declares unknown parent prefix %q", s.Name, p)
			}
		}
	}
	if cfg.LockFile == "" {
		cfg.LockFile = ".sdd/sdd.lock.json"
	}
	return &cfg, nil
}

// StageByPrefix returns the stage configuration owning an identifier prefix.
func (c *Config) StageByPrefix(prefix string) (StageConfig, bool) {
	for _, s := range c.Stages {
		if s.Prefix == prefix {
			return s, true
		}
	}
	return StageConfig{}, false
}

// StageByName returns the stage configuration with the given name.
func (c *Config) StageByName(name string) (StageConfig, bool) {
	for _, s := range c.Stages {
		if s.Name == name {
			return s, true
		}
	}
	return StageConfig{}, false
}

// PrefixOf extracts the identifier prefix of an item ID such as "REQ-0012".
// It returns an empty string when the identifier is not well formed.
func PrefixOf(id string) string {
	i := strings.IndexByte(id, '-')
	if i <= 0 || i == len(id)-1 {
		return ""
	}
	return id[:i]
}

// IsLegalParent reports whether a parent identifier may be referenced by a child
// identifier under the configured chain (CON-002).
func (c *Config) IsLegalParent(childID, parentID string) bool {
	childStage, ok := c.StageByPrefix(PrefixOf(childID))
	if !ok {
		return false
	}
	pp := PrefixOf(parentID)
	for _, allowed := range childStage.Parents {
		if allowed == pp {
			return true
		}
	}
	return false
}

// SeverityOf resolves a finding class to its configured severity, defaulting to
// "error" for unknown classes so that new checks fail closed.
func (c *Config) SeverityOf(class string) Severity {
	if v, ok := c.Severity[class]; ok {
		switch v {
		case "error":
			return SeverityError
		case "warn":
			return SeverityWarn
		case "info":
			return SeverityInfo
		}
	}
	return SeverityError
}

// GateByStage returns the gate declared for a development stage.
func (c *Config) GateByStage(stage string) (GateConfig, bool) {
	for _, g := range c.Gates {
		if g.Stage == stage {
			return g, true
		}
	}
	return GateConfig{}, false
}
