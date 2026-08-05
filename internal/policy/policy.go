// Package policy is the only path to an actuator and the last check before one runs
// (ARC-010).
package policy

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/zlrrr/mutil-agent-system/internal/domain"
)

// sdd:impl DLD-1050

// Decision is the outcome of evaluating an action against policy.
type Decision struct {
	Allowed          bool   `json:"allowed"`
	RequiresApproval bool   `json:"requires_approval"`
	Rule             string `json:"rule"`
	Reason           string `json:"reason"`
}

// Range bounds a numeric configuration value.
type Range struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

// Config declares what is permitted. Everything not listed is refused.
type Config struct {
	AllowedServices []string         `json:"allowed_services"`
	AllowedTools    []string         `json:"allowed_tools"`
	AllowedKeys     []string         `json:"allowed_config_keys"`
	ValueRanges     map[string]Range `json:"value_ranges"`
}

// DefaultConfig is the demo allowlist.
func DefaultConfig() Config {
	return Config{
		AllowedServices: []string{"order-api", "payment-api", "inventory-api", "checkout-web"},
		AllowedTools:    []string{"set_config", "restart_service", "scale_service", "create_ticket"},
		AllowedKeys:     []string{"DB_POOL_SIZE", "FEATURE_FLAG_SAFE_MODE", "RATE_LIMIT_QPS", "PAYMENT_URL"},
		ValueRanges: map[string]Range{
			"DB_POOL_SIZE":   {Min: 1, Max: 200},
			"RATE_LIMIT_QPS": {Min: 1, Max: 10000},
		},
	}
}

// forbiddenTools are refused as a category, before any allowlist is consulted, so no
// configuration can enable them (CON-009).
var forbiddenTools = map[string]string{
	"shell":           "arbitrary shell execution",
	"exec":            "arbitrary process execution",
	"bash":            "arbitrary shell execution",
	"sh":              "arbitrary shell execution",
	"sql":             "free-form database statements",
	"psql":            "free-form database statements",
	"delete_resource": "resource deletion",
	"bulk_restart":    "cross-service bulk operations",
	"drop_table":      "destructive schema change",
}

// shellMetacharacters mark an argument that is trying to become a command.
const shellMetacharacters = "|&;<>$`\n"

// Engine evaluates actions against the configured policy.
type Engine struct{ cfg Config }

// New returns a policy engine over a configuration.
func New(cfg Config) *Engine { return &Engine{cfg: cfg} }

// Config returns a copy of the active policy, for the console and for tests asserting
// that retrieved content did not alter it (TC-0044).
func (e *Engine) Config() Config {
	out := Config{
		AllowedServices: append([]string(nil), e.cfg.AllowedServices...),
		AllowedTools:    append([]string(nil), e.cfg.AllowedTools...),
		AllowedKeys:     append([]string(nil), e.cfg.AllowedKeys...),
		ValueRanges:     map[string]Range{},
	}
	for k, v := range e.cfg.ValueRanges {
		out.ValueRanges[k] = v
	}
	return out
}

// Evaluate decides whether an action may run, and whether a human must decide first.
//
// The order matters and is part of the contract: categorical denials are checked
// before any allowlist, so a permissive configuration can never enable them.
func (e *Engine) Evaluate(a domain.Action) Decision {
	// 1. Categorical denial by tool.
	if what, forbidden := forbiddenTools[strings.ToLower(a.Tool)]; forbidden {
		return Decision{
			Rule:   "forbidden_category",
			Reason: fmt.Sprintf("%s is refused unconditionally", what),
		}
	}
	// 1b. Categorical denial by argument shape.
	for _, k := range sortedKeys(a.Args) {
		if strings.ContainsAny(a.Args[k], shellMetacharacters) {
			return Decision{
				Rule: "forbidden_category",
				Reason: fmt.Sprintf(
					"argument %q contains a shell metacharacter, which is refused unconditionally", k),
			}
		}
	}

	// 2. Service allowlist.
	service := a.Args["service"]
	if service == "" {
		return Decision{Rule: "service_allowlist", Reason: "action names no target service"}
	}
	if !contains(e.cfg.AllowedServices, service) {
		return Decision{
			Rule:   "service_allowlist",
			Reason: fmt.Sprintf("service %q is not on the allowlist", service),
		}
	}

	// 3. Tool allowlist.
	if !contains(e.cfg.AllowedTools, a.Tool) {
		return Decision{
			Rule:   "tool_allowlist",
			Reason: fmt.Sprintf("tool %q is not on the allowlist", a.Tool),
		}
	}

	// 4. Configuration key allowlist.
	if a.Tool == "set_config" {
		key := a.Args["key"]
		if !contains(e.cfg.AllowedKeys, key) {
			return Decision{
				Rule:   "key_allowlist",
				Reason: fmt.Sprintf("configuration key %q is not on the allowlist", key),
			}
		}
		// 5. Value range.
		if r, ok := e.cfg.ValueRanges[key]; ok {
			v, err := strconv.ParseFloat(a.Args["value"], 64)
			if err != nil {
				return Decision{
					Rule:   "value_range",
					Reason: fmt.Sprintf("%s expects a numeric value, got %q", key, a.Args["value"]),
				}
			}
			if v < r.Min || v > r.Max {
				return Decision{
					Rule: "value_range",
					Reason: fmt.Sprintf("%s value %g is outside the permitted range [%g, %g]",
						key, v, r.Min, r.Max),
				}
			}
		}
	}

	// 6. Risk gate.
	return Decision{
		Allowed:          true,
		RequiresApproval: a.Risk.RequiresApproval(),
		Rule:             "risk_gate",
		Reason: fmt.Sprintf("%s risk action against %s",
			a.Risk, service),
	}
}

func contains(list []string, v string) bool {
	for _, e := range list {
		if e == v {
			return true
		}
	}
	return false
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
