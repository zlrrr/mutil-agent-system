// Package signal defines the six boundaries to the outside world and the analysis
// primitives applied to what comes back. It imports only the domain (ARC-001).
package signal

import (
	"context"
	"time"

	"github.com/zlrrr/mutil-agent-system/internal/domain"
)

// sdd:impl DLD-1020

// Point is one sample of a metric series.
type Point struct {
	At    time.Time `json:"at"`
	Value float64   `json:"value"`
}

// Series is a named metric series, optionally with a declared capacity so that
// saturation is expressible rather than inferred.
type Series struct {
	Name     string            `json:"name"`
	Unit     string            `json:"unit,omitempty"`
	Capacity float64           `json:"capacity,omitempty"`
	Labels   map[string]string `json:"labels,omitempty"`
	Points   []Point           `json:"points"`
}

// MetricQuery asks a metric source for one series over a window.
type MetricQuery struct {
	Service string
	Series  string
	Window  domain.TimeWindow
}

// LogLine is one retrieved log record.
type LogLine struct {
	At      time.Time `json:"at"`
	Service string    `json:"service"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
}

// LogQuery asks a log source for lines matching terms within a window.
type LogQuery struct {
	Service string
	Terms   []string
	Window  domain.TimeWindow
}

// Change is one configuration or deployment change.
type Change struct {
	At       time.Time `json:"at"`
	Service  string    `json:"service"`
	Type     string    `json:"type"` // config | deploy | infra
	Key      string    `json:"key"`
	Old      string    `json:"old"`
	New      string    `json:"new"`
	Author   string    `json:"author,omitempty"`
	Ref      string    `json:"ref,omitempty"`
	Revision string    `json:"revision,omitempty"`
}

// ServiceEdge is one dependency relation in the topology.
type ServiceEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// ServiceNode carries what the topology knows about one service's behaviour during the
// incident.
type ServiceNode struct {
	Name      string    `json:"name"`
	Anomalous bool      `json:"anomalous"`
	FirstSeen time.Time `json:"first_seen,omitempty"`
	Role      string    `json:"role,omitempty"`
}

// Topology is the dependency neighbourhood of the affected service.
type Topology struct {
	Service string        `json:"service"`
	Nodes   []ServiceNode `json:"nodes"`
	Edges   []ServiceEdge `json:"edges"`
}

// Upstreams returns the services the given service depends on.
func (t Topology) Upstreams(service string) []string {
	var out []string
	for _, e := range t.Edges {
		if e.From == service {
			out = append(out, e.To)
		}
	}
	return out
}

// Downstreams returns the services that depend on the given service.
func (t Topology) Downstreams(service string) []string {
	var out []string
	for _, e := range t.Edges {
		if e.To == service {
			out = append(out, e.From)
		}
	}
	return out
}

// Node returns a node by name.
func (t Topology) Node(name string) (ServiceNode, bool) {
	for _, n := range t.Nodes {
		if n.Name == name {
			return n, true
		}
	}
	return ServiceNode{}, false
}

// Runbook is one knowledge-base entry.
type Runbook struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Symptoms []string `json:"symptoms"`
	Body     string   `json:"body"`
	Score    float64  `json:"-"`
	Matched  []string `json:"-"`
}

// KnowledgeQuery asks the knowledge source for entries matching a service and symptoms.
type KnowledgeQuery struct {
	Service  string
	Symptoms []string
}

// ActuationResult is what an actuator reports after a typed invocation.
type ActuationResult struct {
	Outcome string `json:"outcome"`
	Detail  string `json:"detail"`
}

// The six ports. Each has a fixture adapter so that the offline path is the primary
// path (ARC-006).
type (
	// MetricSource retrieves a metric series over a window.
	MetricSource interface {
		Range(ctx context.Context, q MetricQuery) (Series, error)
		SeriesNames(ctx context.Context, service string) ([]string, error)
	}
	// LogSource retrieves log lines matching terms within a window.
	LogSource interface {
		Search(ctx context.Context, q LogQuery) ([]LogLine, error)
	}
	// ChangeSource retrieves configuration and deployment changes within a window.
	ChangeSource interface {
		Changes(ctx context.Context, service string, w domain.TimeWindow) ([]Change, error)
	}
	// TopologySource retrieves the dependency neighbourhood of a service.
	TopologySource interface {
		Neighbourhood(ctx context.Context, service string) (Topology, error)
	}
	// KnowledgeSource retrieves runbook entries matching a query.
	KnowledgeSource interface {
		Search(ctx context.Context, q KnowledgeQuery) ([]Runbook, error)
	}
	// Actuator applies a typed action to a target system. Only the executor holds one.
	Actuator interface {
		Invoke(ctx context.Context, call domain.ActionCall) (ActuationResult, error)
	}
)

// DemandResponder serves evidence registered against a critic's demand descriptor.
// Round two is targeted rather than a blind repeat because collectors ask this
// (DLD-1040).
type DemandResponder interface {
	Respond(ctx context.Context, descriptor string, kind domain.EvidenceKind) (DemandEvidence, bool)
}

// DemandEvidence is a prepared answer to a specific evidence demand.
type DemandEvidence struct {
	Descriptor string
	Kind       domain.EvidenceKind
	Source     string
	Summary    string
	RawRef     string
	Confidence float64
	Facts      map[string]string
}

// Set bundles the six ports plus the result bounds, so an agent receives one value.
type Set struct {
	Metrics   MetricSource
	Logs      LogSource
	Changes   ChangeSource
	Topology  TopologySource
	Knowledge KnowledgeSource
	Actuator  Actuator
	Demands   DemandResponder
	Bounds    Bounds
}
