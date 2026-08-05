package arena

import (
	"context"
	"fmt"
	"sync"

	"github.com/zlrrr/mutil-agent-system/internal/catalog"
	"github.com/zlrrr/mutil-agent-system/internal/domain"
	"github.com/zlrrr/mutil-agent-system/internal/eventbus"
	"github.com/zlrrr/mutil-agent-system/internal/orchestrator"
	"github.com/zlrrr/mutil-agent-system/internal/policy"
	"github.com/zlrrr/mutil-agent-system/internal/reasoner"
	"github.com/zlrrr/mutil-agent-system/internal/signal/profile"
	"github.com/zlrrr/mutil-agent-system/internal/store"
)

// sdd:impl DLD-1075

// Registry routes cases to the engine that owns their fixture environment, while every
// engine shares one store and one broker — so listing and streaming work across cases
// even though each case has its own simulated world.
type Registry struct {
	mu      sync.Mutex
	store   store.Store
	bus     *eventbus.Broker
	cfg     reasoner.Config
	pol     policy.Config
	cat     *catalog.Catalog
	signals profile.Config
	engines map[string]*orchestrator.Engine // keyed by case identifier
	builds  map[string]*Build
}

// NewRegistry builds a registry over a shared store and broker, serving every port from
// the fixture adapters.
func NewRegistry(st store.Store, bus *eventbus.Broker, cfg reasoner.Config, pol policy.Config, cat *catalog.Catalog) (*Registry, error) {
	return NewRegistryWithSignals(st, bus, cfg, pol, cat, profile.Config{})
}

// NewRegistryWithSignals builds a registry whose cases use a named adapter profile. The
// zero profile is the fixture profile, so this and NewRegistry agree by default.
func NewRegistryWithSignals(st store.Store, bus *eventbus.Broker, cfg reasoner.Config, pol policy.Config, cat *catalog.Catalog, signals profile.Config) (*Registry, error) {
	if cat == nil {
		var err error
		cat, err = catalog.Load()
		if err != nil {
			return nil, fmt.Errorf("load catalog: %w", err)
		}
	}
	if st == nil {
		st = store.NewMemory()
	}
	if bus == nil {
		bus = eventbus.New()
	}
	if cfg.MaxRounds == 0 {
		cfg = reasoner.DefaultConfig()
	}
	if len(pol.AllowedServices) == 0 {
		pol = policy.DefaultConfig()
	}
	return &Registry{
		store: st, bus: bus, cfg: cfg, pol: pol, cat: cat, signals: signals,
		engines: map[string]*orchestrator.Engine{},
		builds:  map[string]*Build{},
	}, nil
}

// Catalog returns the loaded catalog.
func (r *Registry) Catalog() *catalog.Catalog { return r.cat }

// Store returns the shared store.
func (r *Registry) Store() store.Store { return r.store }

// Bus returns the shared broker.
func (r *Registry) Bus() *eventbus.Broker { return r.bus }

// Create opens a case. The alert's CaseRef selects the fixture environment; without
// one, the first catalog case matching the service is used.
func (r *Registry) Create(ctx context.Context, alert domain.Alert, mode domain.Mode) (*domain.Case, error) {
	ref := alert.CaseRef
	if ref == "" {
		for _, fc := range r.cat.Cases {
			if fc.Alert.Service == alert.Service {
				ref = fc.ID
				break
			}
		}
	}
	if ref == "" {
		return nil, fmt.Errorf("no fault case matches service %q; pass case_ref explicitly (have %v)",
			alert.Service, r.cat.CaseIDs())
	}

	build, err := NewBuild(Params{
		CaseID: ref, Mode: mode, Store: r.store, Bus: r.bus,
		Config: r.cfg, Policy: r.pol, Catalog: r.cat, Signals: r.signals,
	})
	if err != nil {
		return nil, err
	}

	c, err := build.Engine.Create(ctx, alert, mode)
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	r.engines[c.ID] = build.Engine
	r.builds[c.ID] = build
	r.mu.Unlock()
	return c, nil
}

// Build returns the fixture environment behind a case, for tests and diagnostics.
func (r *Registry) Build(caseID string) (*Build, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.builds[caseID]
	return b, ok
}

func (r *Registry) engine(caseID string) (*orchestrator.Engine, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.engines[caseID]
	return e, ok
}

// Get returns a case projection, falling back to a replay from the store for a case
// this process did not open.
func (r *Registry) Get(ctx context.Context, caseID string) (*domain.Case, error) {
	if e, ok := r.engine(caseID); ok {
		return e.Get(ctx, caseID)
	}
	return store.Load(ctx, r.store, caseID)
}

// Run advances a case to its next halt.
func (r *Registry) Run(ctx context.Context, caseID string) (*domain.Case, error) {
	e, ok := r.engine(caseID)
	if !ok {
		return nil, fmt.Errorf("%w: %s has no live engine in this process", store.ErrCaseNotFound, caseID)
	}
	return e.Run(ctx, caseID)
}

// Decide records an approval decision and resumes the case.
func (r *Registry) Decide(ctx context.Context, caseID, actionID string, d domain.ApprovalDecision) (*domain.Case, error) {
	e, ok := r.engine(caseID)
	if !ok {
		return nil, fmt.Errorf("%w: %s has no live engine in this process", store.ErrCaseNotFound, caseID)
	}
	return e.Decide(ctx, caseID, actionID, d)
}
