// Package planmodel is the model-backed adapter for the planner port.
//
// It decides one thing: which of the series a source offers are worth querying this
// round. That is where judgement pays — a narrower first round is a different
// investigation, and whether it is a better one is the question the evaluation exists to
// answer (ADR-008).
//
// It decides nothing about the window or the role set. Those are bounds, and a bound a
// strategy can move is not a bound: a planner able to widen the window could make every
// case unbounded, and one able to drop a role could blind a whole evidence kind without
// saying so.
package planmodel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/zlrrr/mutil-agent-system/internal/agent/plan"
	"github.com/zlrrr/mutil-agent-system/internal/domain"
)

// sdd:impl DLD-1038

// ProviderError reports a planner that could not reach or parse its provider. It exists
// so a failure never arrives looking like a plan: an empty plan means "nothing worth
// querying", which is a decision, and a transport error must not be able to impersonate
// one.
type ProviderError struct {
	Endpoint string
	Status   int
	Err      error
}

func (e *ProviderError) Error() string {
	if e.Status != 0 {
		return fmt.Sprintf("planner provider %s: status %d: %v", e.Endpoint, e.Status, e.Err)
	}
	return fmt.Sprintf("planner provider %s: %v", e.Endpoint, e.Err)
}

func (e *ProviderError) Unwrap() error { return e.Err }

// Options configures the adapter.
type Options struct {
	Timeout time.Duration
	Client  *http.Client
	// APIKey is sent as a bearer token when set. It is read from the environment by the
	// caller, never from a command line.
	APIKey string
}

// Planner is the model-backed adapter.
type Planner struct {
	endpoint string
	model    string
	client   *http.Client
	opts     Options
	fallback *plan.RulePlanner
}

// New builds the adapter. The lookback is used by the deterministic triage it delegates.
func New(endpoint, model string, lookback time.Duration, opts Options) *Planner {
	if opts.Timeout <= 0 {
		opts.Timeout = 30 * time.Second
	}
	client := opts.Client
	if client == nil {
		client = &http.Client{Timeout: opts.Timeout}
	}
	return &Planner{
		endpoint: endpoint, model: model, client: client, opts: opts,
		fallback: plan.NewRulePlanner(lookback),
	}
}

// Name identifies this strategy in the recorded plan.
func (p *Planner) Name() string { return "model" }

// Triage returns the deterministic plan. See the package comment: the window and the role
// set are bounds rather than judgements.
func (p *Planner) Triage(ctx context.Context, s domain.Snapshot) (plan.Plan, error) {
	return p.fallback.Triage(ctx, s)
}

// PlanCollection asks the provider which of the available series to query.
func (p *Planner) PlanCollection(ctx context.Context, s domain.Snapshot, a plan.Available) (plan.Queries, error) {
	if len(a.Series) == 0 {
		return plan.Queries{}, nil
	}
	available := append([]string(nil), a.Series...)
	sort.Strings(available)

	var body struct {
		Series []string `json:"series"`
		Reason string   `json:"reason"`
	}
	if err := p.ask(ctx, collectionPrompt(s, available), &body); err != nil {
		return plan.Queries{}, err
	}

	// Untrusted: unknown names are dropped by plan.Resolve, but a response naming
	// nothing usable is treated as a failure rather than as the decision "query
	// nothing", because the caller must be able to tell those apart.
	if len(body.Series) == 0 {
		return plan.Queries{}, &ProviderError{
			Endpoint: p.endpoint,
			Err:      fmt.Errorf("the response named no series out of %d offered", len(available)),
		}
	}

	reason := strings.TrimSpace(body.Reason)
	if reason == "" {
		reason = "selected by the model planner"
	}
	return plan.Queries{
		Series:   body.Series,
		LogTerms: append([]string(nil), plan.DefaultLogTerms...),
		Reason:   reason,
	}, nil
}

// collectionPrompt describes the choice without describing the answer. It offers the
// series by name only: a prompt that summarised what each series had already done would
// be asking the model to confirm an analysis the collectors have not run yet.
func collectionPrompt(s domain.Snapshot, available []string) string {
	var b strings.Builder
	b.WriteString("You are planning one round of evidence collection for an incident.\n\n")
	fmt.Fprintf(&b, "Alert: %s on service %s, severity %s.\n",
		s.Alert.Name, s.Alert.Service, s.Alert.Severity)
	if sum := s.Alert.Annotations["summary"]; sum != "" {
		fmt.Fprintf(&b, "Summary: %s\n", sum)
	}
	fmt.Fprintf(&b, "Collection round: %d of %d.\n\n", max(s.Round, 1), s.MaxRounds)
	b.WriteString("Metric series available for this service:\n")
	for _, name := range available {
		fmt.Fprintf(&b, "- %s\n", name)
	}
	b.WriteString("\nChoose the series worth querying this round. Reply with JSON only:\n")
	b.WriteString(`{"series":["<name>",...],"reason":"<one sentence>"}` + "\n")
	b.WriteString("Use only names from the list above. Choose at least one.\n")
	return b.String()
}

// ask performs one chat-completion and decodes the assistant's content as JSON.
func (p *Planner) ask(ctx context.Context, prompt string, into any) error {
	payload, err := json.Marshal(map[string]any{
		"model":       p.model,
		"temperature": 0,
		"messages": []map[string]string{
			{"role": "system", "content": "You reply with JSON and nothing else."},
			{"role": "user", "content": prompt},
		},
	})
	if err != nil {
		return &ProviderError{Endpoint: p.endpoint, Err: err}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(payload))
	if err != nil {
		return &ProviderError{Endpoint: p.endpoint, Err: err}
	}
	req.Header.Set("Content-Type", "application/json")
	if p.opts.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.opts.APIKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return &ProviderError{Endpoint: p.endpoint, Err: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &ProviderError{Endpoint: p.endpoint, Status: resp.StatusCode,
			Err: fmt.Errorf("unexpected status")}
	}

	var envelope struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return &ProviderError{Endpoint: p.endpoint, Status: resp.StatusCode, Err: err}
	}
	if len(envelope.Choices) == 0 {
		return &ProviderError{Endpoint: p.endpoint, Status: resp.StatusCode,
			Err: fmt.Errorf("the response carried no choices")}
	}
	content := strings.TrimSpace(envelope.Choices[0].Message.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	if err := json.Unmarshal([]byte(strings.TrimSpace(content)), into); err != nil {
		return &ProviderError{Endpoint: p.endpoint, Status: resp.StatusCode, Err: err}
	}
	return nil
}
