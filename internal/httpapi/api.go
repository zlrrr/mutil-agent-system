// Package httpapi exposes the case lifecycle over HTTP and serves the console from the
// same binary (ARC-014).
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/zlrrr/mutil-agent-system/internal/catalog"
	"github.com/zlrrr/mutil-agent-system/internal/domain"
	"github.com/zlrrr/mutil-agent-system/internal/eventbus"
	"github.com/zlrrr/mutil-agent-system/internal/report"
	"github.com/zlrrr/mutil-agent-system/internal/store"
)

// sdd:impl DLD-1071

// Runtime is what the HTTP surface needs from the orchestration layer. Both a single
// engine and the multi-case registry satisfy it, so the API does not care whether it is
// serving one fixture environment or several.
type Runtime interface {
	Create(ctx context.Context, alert domain.Alert, mode domain.Mode) (*domain.Case, error)
	Get(ctx context.Context, caseID string) (*domain.Case, error)
	Run(ctx context.Context, caseID string) (*domain.Case, error)
	Decide(ctx context.Context, caseID, actionID string, d domain.ApprovalDecision) (*domain.Case, error)
	Store() store.Store
	Bus() *eventbus.Broker
}

// Server binds a runtime to HTTP handlers.
type Server struct {
	engine  Runtime
	catalog *catalog.Catalog
	version string
}

// NewServer builds the HTTP surface over a runtime.
func NewServer(engine Runtime, cat *catalog.Catalog, version string) *Server {
	return &Server{engine: engine, catalog: cat, version: version}
}

// Handler returns the routed handler for the whole surface.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /api/version", s.versionInfo)
	mux.HandleFunc("GET /api/cases", s.listCases)
	mux.HandleFunc("POST /api/cases", s.createCase)
	mux.HandleFunc("GET /api/cases/{id}", s.getCase)
	mux.HandleFunc("GET /api/cases/{id}/events", s.getEvents)
	mux.HandleFunc("GET /api/cases/{id}/stream", s.streamEvents)
	mux.HandleFunc("POST /api/cases/{id}/actions/{actionID}/decision", s.decide)
	mux.HandleFunc("GET /api/cases/{id}/report.md", s.reportMarkdown)
	mux.HandleFunc("GET /api/cases/{id}/report.json", s.reportJSON)
	mux.HandleFunc("GET /api/catalog/cases", s.listFaultCases)

	mux.HandleFunc("GET /", s.consoleIndex)
	mux.HandleFunc("GET /cases/{id}", s.consoleCase)
	mux.HandleFunc("GET /static/{file}", s.static)

	return mux
}

// ---------------------------------------------------------------------- payloads

type createCaseRequest struct {
	AlertName   string            `json:"alert_name"`
	Service     string            `json:"service"`
	Severity    string            `json:"severity"`
	StartsAt    time.Time         `json:"starts_at"`
	EndsAt      time.Time         `json:"ends_at"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	CaseRef     string            `json:"case_ref"`
	Mode        domain.Mode       `json:"mode"`
}

type decisionRequest struct {
	Decision string `json:"decision"`
	By       string `json:"by"`
	Comment  string `json:"comment"`
}

type apiError struct {
	Error string `json:"error"`
	Field string `json:"field,omitempty"`
}

// ---------------------------------------------------------------------- handlers

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": s.version})
}

func (s *Server) versionInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"version": s.version,
		"cases":   s.catalog.CaseIDs(),
		"modes":   []domain.Mode{domain.ModeSingle, domain.ModeMultiNoCritic, domain.ModeMultiWithCritic},
	})
}

func (s *Server) listCases(w http.ResponseWriter, r *http.Request) {
	headers, err := s.engine.Store().ListCases(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "", err.Error())
		return
	}
	if headers == nil {
		headers = []store.CaseHeader{}
	}
	writeJSON(w, http.StatusOK, headers)
}

func (s *Server) listFaultCases(w http.ResponseWriter, _ *http.Request) {
	type entry struct {
		ID       string `json:"id"`
		Title    string `json:"title"`
		Service  string `json:"service"`
		Expected string `json:"expected_signature"`
	}
	out := make([]entry, 0, len(s.catalog.Cases))
	for _, fc := range s.catalog.Cases {
		out = append(out, entry{
			ID: fc.ID, Title: fc.Title, Service: fc.Alert.Service,
			Expected: fc.ExpectedSignature,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createCase(w http.ResponseWriter, r *http.Request) {
	var req createCaseRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "", "malformed request body: "+err.Error())
		return
	}
	// A case_ref alone is enough: the catalog already declares that scenario's alert,
	// which is what makes the console's one-click demo possible without duplicating it.
	if req.CaseRef != "" && req.AlertName == "" {
		fc, ok := s.catalog.Case(req.CaseRef)
		if !ok {
			writeError(w, http.StatusBadRequest, "case_ref",
				fmt.Sprintf("unknown fault case %q (have %v)", req.CaseRef, s.catalog.CaseIDs()))
			return
		}
		req.AlertName = fc.Alert.Name
		req.Service = fc.Alert.Service
		req.Severity = fc.Alert.Severity
		req.StartsAt = fc.Alert.StartsAt
		req.EndsAt = fc.Alert.EndsAt
		if req.Labels == nil {
			req.Labels = fc.Alert.Labels
		}
		if req.Annotations == nil {
			req.Annotations = fc.Alert.Annotations
		}
	}

	switch {
	case req.AlertName == "":
		writeError(w, http.StatusBadRequest, "alert_name", "alert_name is required")
		return
	case req.Service == "":
		writeError(w, http.StatusBadRequest, "service", "service is required")
		return
	case req.StartsAt.IsZero():
		writeError(w, http.StatusBadRequest, "starts_at", "starts_at is required")
		return
	}
	if req.Severity == "" {
		req.Severity = "P2"
	}

	alert := domain.Alert{
		Name: req.AlertName, Service: req.Service, Severity: req.Severity,
		StartsAt: req.StartsAt.UTC(), EndsAt: req.EndsAt.UTC(),
		Labels: req.Labels, Annotations: req.Annotations, CaseRef: req.CaseRef,
	}
	c, err := s.engine.Create(r.Context(), alert, req.Mode)
	if err != nil {
		writeError(w, http.StatusConflict, "", err.Error())
		return
	}
	c, err = s.engine.Run(r.Context(), c.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) getCase(w http.ResponseWriter, r *http.Request) {
	c, ok := s.lookup(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) getEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	from := intQuery(r, "from", 0)
	events, err := s.engine.Store().Events(r.Context(), id, from)
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	if events == nil {
		events = []domain.Event{}
	}
	writeJSON(w, http.StatusOK, events)
}

func (s *Server) decide(w http.ResponseWriter, r *http.Request) {
	id, actionID := r.PathValue("id"), r.PathValue("actionID")

	var req decisionRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "", "malformed request body: "+err.Error())
		return
	}
	if req.Decision != "approved" && req.Decision != "rejected" {
		writeError(w, http.StatusBadRequest, "decision",
			`decision must be "approved" or "rejected"`)
		return
	}
	if req.By == "" {
		writeError(w, http.StatusBadRequest, "by", "by is required: an approval must name its approver")
		return
	}

	c, err := s.engine.Decide(r.Context(), id, actionID, domain.ApprovalDecision{
		Decision: req.Decision, By: req.By, Comment: req.Comment,
	})
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) reportMarkdown(w http.ResponseWriter, r *http.Request) {
	c, ok := s.lookup(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, report.Markdown(c))
}

func (s *Server) reportJSON(w http.ResponseWriter, r *http.Request) {
	c, ok := s.lookup(w, r)
	if !ok {
		return
	}
	raw, err := report.JSON(c)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(raw)
}

// ----------------------------------------------------------------------- helpers

func (s *Server) lookup(w http.ResponseWriter, r *http.Request) (*domain.Case, bool) {
	c, err := s.engine.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeStoreError(w, err)
		return nil, false
	}
	return c, true
}

func (s *Server) writeStoreError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrCaseNotFound) {
		writeError(w, http.StatusNotFound, "id", err.Error())
		return
	}
	writeError(w, http.StatusInternalServerError, "", err.Error())
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func writeError(w http.ResponseWriter, status int, field, msg string) {
	writeJSON(w, status, apiError{Error: msg, Field: field})
}

func intQuery(r *http.Request, key string, def int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
		return def
	}
	return n
}
