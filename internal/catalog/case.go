package catalog

import (
	"time"

	"github.com/zlrrr/mutil-agent-system/internal/domain"
	"github.com/zlrrr/mutil-agent-system/internal/signal"
)

// sdd:impl DLD-1030

// FaultCase declares a reproducible scenario as data: the alert, the fixture signals,
// the expected root cause and the expected remediation. Adding one requires no code
// change (REQ-0082).
type FaultCase struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`

	Alert domain.Alert `json:"alert"`

	// DefaultSeries are the series a collector queries without being asked. Series
	// outside this list exist but are only reached when a demand names them — which
	// is what makes the first round incomplete and the critic's demand consequential.
	DefaultSeries []string        `json:"default_series"`
	Series        []signal.Series `json:"series"`
	PostRecovery  []signal.Series `json:"post_recovery_series,omitempty"`

	Logs     []signal.LogLine `json:"logs"`
	Changes  []signal.Change  `json:"changes"`
	Topology signal.Topology  `json:"topology"`

	// DemandResponses answer specific critic demands in a later round.
	DemandResponses []DemandResponse `json:"demand_responses,omitempty"`

	// RecoveryTrigger names the actuator call after which the post-recovery series
	// become visible to the metric source.
	RecoveryTrigger *RecoveryTrigger `json:"recovery_trigger,omitempty"`

	ExpectedSignature   string `json:"expected_signature"`
	ExpectedRemediation string `json:"expected_remediation"`

	// InitialConfig seeds the simulated actuator's configuration state.
	InitialConfig map[string]string `json:"initial_config,omitempty"`
}

// DemandResponse is evidence prepared for a specific demand descriptor.
type DemandResponse struct {
	Descriptor string              `json:"descriptor"`
	Kind       domain.EvidenceKind `json:"kind"`
	Source     string              `json:"source"`
	Summary    string              `json:"summary"`
	RawRef     string              `json:"raw_ref"`
	Confidence float64             `json:"confidence"`
	Facts      map[string]string   `json:"facts,omitempty"`
	// Series, when named, makes this response also expose a metric series that the
	// default pass did not reach.
	Series string `json:"series,omitempty"`
	// Changes, when true, makes this response expose the change history over the
	// extended lookback window.
	Changes bool `json:"changes,omitempty"`
}

// RecoveryTrigger names the actuator call that flips the environment to recovered.
type RecoveryTrigger struct {
	Tool string            `json:"tool"`
	Args map[string]string `json:"args"`
}

// Matches reports whether a call satisfies the recovery trigger.
func (r *RecoveryTrigger) Matches(call domain.ActionCall) bool {
	if r == nil || r.Tool != call.Tool {
		return false
	}
	for k, v := range r.Args {
		if call.Args[k] != v {
			return false
		}
	}
	return true
}

// Window returns the incident window implied by the case's alert.
func (fc FaultCase) Window() domain.TimeWindow {
	end := fc.Alert.EndsAt
	if end.IsZero() {
		end = fc.Alert.StartsAt.Add(20 * time.Minute)
	}
	return domain.TimeWindow{Start: fc.Alert.StartsAt.Add(-7 * time.Minute), End: end}
}

// SeriesByName returns a declared series.
func (fc FaultCase) SeriesByName(name string) (signal.Series, bool) {
	for _, s := range fc.Series {
		if s.Name == name {
			return s, true
		}
	}
	return signal.Series{}, false
}

// PostRecoveryByName returns a declared post-recovery series.
func (fc FaultCase) PostRecoveryByName(name string) (signal.Series, bool) {
	for _, s := range fc.PostRecovery {
		if s.Name == name {
			return s, true
		}
	}
	return signal.Series{}, false
}

// ResponseFor returns the prepared answer to a demand descriptor.
func (fc FaultCase) ResponseFor(descriptor string) (DemandResponse, bool) {
	for _, r := range fc.DemandResponses {
		if r.Descriptor == descriptor {
			return r, true
		}
	}
	return DemandResponse{}, false
}
