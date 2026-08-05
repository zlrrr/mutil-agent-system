package httpapi

import (
	"bytes"
	"embed"
	"html/template"
	"net/http"
	"path"
	"sort"

	"github.com/zlrrr/mutil-agent-system/internal/domain"
	"github.com/zlrrr/mutil-agent-system/internal/report"
	"github.com/zlrrr/mutil-agent-system/internal/store"
)

// sdd:impl DLD-1073

//go:embed web
var assets embed.FS

var funcs = template.FuncMap{
	"inc": func(i int) int { return i + 1 },
}

var (
	indexTmpl = template.Must(template.New("index").Funcs(funcs).
			ParseFS(assets, "web/layout.html", "web/index.html"))
	caseTmpl = template.Must(template.New("case").Funcs(funcs).
			ParseFS(assets, "web/layout.html", "web/case.html"))
)

type faultCaseView struct {
	ID       string
	Title    string
	Service  string
	Expected string
}

type indexView struct {
	Title      string
	Version    string
	Cases      []store.CaseHeader
	FaultCases []faultCaseView
}

type evidenceGroup struct {
	Kind  domain.EvidenceKind
	Items []domain.Evidence
}

type caseView struct {
	Title          string
	Version        string
	Case           *domain.Case
	Leading        *domain.Hypothesis
	Timeline       []domain.TimelineEntry
	EvidenceGroups []evidenceGroup
	Report         string
}

func (s *Server) consoleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	headers, err := s.engine.Store().ListCases(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	view := indexView{Title: "Cases", Version: s.version, Cases: headers}
	for _, fc := range s.catalog.Cases {
		view.FaultCases = append(view.FaultCases, faultCaseView{
			ID: fc.ID, Title: fc.Title, Service: fc.Alert.Service,
			Expected: fc.ExpectedSignature,
		})
	}
	render(w, indexTmpl, view)
}

func (s *Server) consoleCase(w http.ResponseWriter, r *http.Request) {
	c, err := s.engine.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	view := caseView{
		Title:    c.Alert.Name,
		Version:  s.version,
		Case:     c,
		Timeline: c.Timeline,
		Report:   report.Markdown(c),
	}
	if lead, ok := c.Leading(); ok {
		view.Leading = &lead
	}

	byKind := map[domain.EvidenceKind][]domain.Evidence{}
	for _, e := range c.Evidence {
		byKind[e.Kind] = append(byKind[e.Kind], e)
	}
	for _, kind := range domain.AllEvidenceKinds {
		items := byKind[kind]
		if len(items) == 0 {
			continue
		}
		sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
		view.EvidenceGroups = append(view.EvidenceGroups, evidenceGroup{Kind: kind, Items: items})
	}
	render(w, caseTmpl, view)
}

// static serves the embedded stylesheet and script. Only the two known names are
// served, so the handler cannot be turned into a file browser.
func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	name := path.Base(r.PathValue("file"))
	var contentType string
	switch name {
	case "style.css":
		contentType = "text/css; charset=utf-8"
	case "app.js":
		contentType = "text/javascript; charset=utf-8"
	default:
		http.NotFound(w, r)
		return
	}
	raw, err := assets.ReadFile("web/" + name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(raw)
}

// render writes the template to a buffer first, so a template error produces a clean
// 500 rather than a half-written page.
func render(w http.ResponseWriter, t *template.Template, data any) {
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "layout", data); err != nil {
		http.Error(w, "render: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(buf.Bytes())
}
