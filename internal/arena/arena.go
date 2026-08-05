// Package arena wires adapters into a running engine. It is the one place that knows
// which adapter sits behind each port, so no plane below it has to.
package arena

import (
	"fmt"

	"github.com/zlrrr/mutil-agent-system/internal/agent"
	"github.com/zlrrr/mutil-agent-system/internal/catalog"
	"github.com/zlrrr/mutil-agent-system/internal/domain"
	"github.com/zlrrr/mutil-agent-system/internal/eventbus"
	"github.com/zlrrr/mutil-agent-system/internal/orchestrator"
	"github.com/zlrrr/mutil-agent-system/internal/policy"
	"github.com/zlrrr/mutil-agent-system/internal/reasoner"
	"github.com/zlrrr/mutil-agent-system/internal/signal"
	"github.com/zlrrr/mutil-agent-system/internal/signal/fixture"
	"github.com/zlrrr/mutil-agent-system/internal/store"
)

// sdd:impl DLD-1075

// Build is everything a caller needs to drive one fault case.
type Build struct {
	Engine  *orchestrator.Engine
	Case    catalog.FaultCase
	State   *fixture.State
	Signals signal.Set
	Catalog *catalog.Catalog
	Clock   domain.Clock
}

// Params configures a build.
type Params struct {
	CaseID   string
	Mode     domain.Mode
	Store    store.Store
	Bus      *eventbus.Broker
	Config   reasoner.Config
	Policy   policy.Config
	Actuator signal.Actuator // optional override, used by tests to count invocations
	Catalog  *catalog.Catalog
}

// NewFixtureBuild assembles an engine over the deterministic fixture adapters for one
// fault case. This is the default profile: no network, no credentials, no model
// provider (ADR-002).
func NewFixtureBuild(p Params) (*Build, error) {
	cat := p.Catalog
	if cat == nil {
		var err error
		cat, err = catalog.Load()
		if err != nil {
			return nil, fmt.Errorf("load catalog: %w", err)
		}
	}
	fc, ok := cat.Case(p.CaseID)
	if !ok {
		return nil, fmt.Errorf("fault case %q is not in the catalog (have %v)",
			p.CaseID, cat.CaseIDs())
	}

	cfg := p.Config
	if cfg.MaxRounds == 0 {
		cfg = reasoner.DefaultConfig()
	}
	polCfg := p.Policy
	if len(polCfg.AllowedServices) == 0 {
		polCfg = policy.DefaultConfig()
	}
	st := p.Store
	if st == nil {
		st = store.NewMemory()
	}
	bus := p.Bus
	if bus == nil {
		bus = eventbus.New()
	}

	set, state := fixture.NewSet(fc, cat, signal.DefaultBounds())
	if p.Actuator != nil {
		set.Actuator = p.Actuator
	}

	clock := orchestrator.DefaultClock(fc.Alert)
	pol := policy.New(polCfg)
	rsn := reasoner.NewRuleReasoner(cat, cfg)

	engine := orchestrator.New(orchestrator.Options{
		Agents: orchestrator.Agents{
			Collectors:   agent.Collectors(set, cfg),
			Analysis:     agent.Analysis(rsn),
			Critic:       agent.Critic(rsn),
			Remediation:  agent.Remediation(cat, cfg),
			Verification: agent.Verification(set, cfg),
			Baseline:     agent.SingleAgentBaseline(set, rsn, cfg, cat),
		},
		Policy:   pol,
		Executor: policy.NewExecutor(pol, set.Actuator, clock),
		Store:    st,
		Bus:      bus,
		Config:   cfg,
		Clock:    clock,
	})

	return &Build{
		Engine: engine, Case: fc, State: state, Signals: set,
		Catalog: cat, Clock: clock,
	}, nil
}

// Alert returns the fault case's alert.
func (b *Build) Alert() domain.Alert { return b.Case.Alert }
