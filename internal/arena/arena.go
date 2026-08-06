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
	"github.com/zlrrr/mutil-agent-system/internal/signal/profile"
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
	// Signals selects which adapter serves each port. The zero value is the fixture
	// profile, so a caller that says nothing gets the offline path (REQ-0098).
	Signals profile.Config
	// Reasoner overrides how explanations are selected. Nil means the deterministic
	// rule engine, which is the default by design rather than by omission (ADR-002).
	Reasoner reasoner.Reasoner
}

// NewFixtureBuild assembles an engine over the deterministic fixture adapters for one
// fault case. This is the default profile: no network, no credentials, no model
// provider (ADR-002).
//
// It forces the fixture profile regardless of p.Signals, so a caller that names this
// function gets what the name says. Use NewBuild to honour a configured profile.
func NewFixtureBuild(p Params) (*Build, error) {
	p.Signals = profile.Config{Profile: profile.Fixture}
	return NewBuild(p)
}

// NewBuild assembles an engine using the adapter profile in p.Signals.
func NewBuild(p Params) (*Build, error) {
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

	set, state, err := profile.Build(p.Signals, fc, cat, signal.DefaultBounds())
	if err != nil {
		return nil, fmt.Errorf("signal profile: %w", err)
	}
	if p.Actuator != nil {
		set.Actuator = p.Actuator
	}

	clock := orchestrator.DefaultClock(fc.Alert)
	pol := policy.New(polCfg)
	var rsn reasoner.Reasoner = reasoner.NewRuleReasoner(cat, cfg)
	if p.Reasoner != nil {
		rsn = p.Reasoner
	}

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
