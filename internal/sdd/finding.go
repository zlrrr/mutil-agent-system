package sdd

import (
	"fmt"
	"sort"
	"strings"
)

// sdd:impl DLD-0102

// Severity ranks a finding.
type Severity int

// Severity levels, ordered so that higher values are more serious.
const (
	SeverityInfo Severity = iota
	SeverityWarn
	SeverityError
)

// String renders the severity as the token used in CLI output.
func (s Severity) String() string {
	switch s {
	case SeverityError:
		return "ERROR"
	case SeverityWarn:
		return "WARN"
	default:
		return "INFO"
	}
}

// Finding is a single governance violation or observation.
type Finding struct {
	Class    string   `json:"class"`
	Severity Severity `json:"-"`
	Level    string   `json:"severity"`
	Subject  string   `json:"subject"`
	File     string   `json:"file,omitempty"`
	Line     int      `json:"line,omitempty"`
	Message  string   `json:"message"`
}

// String renders a finding as one CLI line.
func (f Finding) String() string {
	loc := f.File
	if f.Line > 0 {
		loc = fmt.Sprintf("%s:%d", f.File, f.Line)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%-5s %-28s %s", f.Severity, f.Class, f.Subject)
	if f.Message != "" {
		fmt.Fprintf(&b, " — %s", f.Message)
	}
	if loc != "" {
		fmt.Fprintf(&b, " (%s)", loc)
	}
	return b.String()
}

// Report accumulates findings and answers whether a run should fail.
type Report struct {
	Findings []Finding `json:"findings"`
}

// Add appends a finding, resolving its severity through the configuration.
func (r *Report) Add(cfg *Config, class, subject, file string, line int, format string, args ...any) {
	sev := cfg.SeverityOf(class)
	r.Findings = append(r.Findings, Finding{
		Class:    class,
		Severity: sev,
		Level:    sev.String(),
		Subject:  subject,
		File:     file,
		Line:     line,
		Message:  fmt.Sprintf(format, args...),
	})
}

// Merge appends every finding of other into r.
func (r *Report) Merge(other *Report) {
	if other == nil {
		return
	}
	r.Findings = append(r.Findings, other.Findings...)
}

// Count returns the number of findings at the given severity.
func (r *Report) Count(sev Severity) int {
	n := 0
	for _, f := range r.Findings {
		if f.Severity == sev {
			n++
		}
	}
	return n
}

// HasErrors reports whether any finding is fatal.
func (r *Report) HasErrors() bool { return r.Count(SeverityError) > 0 }

// Sorted returns the findings ordered by descending severity then by subject,
// so that the most serious problems are read first.
func (r *Report) Sorted() []Finding {
	out := make([]Finding, len(r.Findings))
	copy(out, r.Findings)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Severity != out[j].Severity {
			return out[i].Severity > out[j].Severity
		}
		if out[i].Class != out[j].Class {
			return out[i].Class < out[j].Class
		}
		return out[i].Subject < out[j].Subject
	})
	return out
}

// Summary renders a one-line tally.
func (r *Report) Summary() string {
	return fmt.Sprintf("%d error(s), %d warning(s), %d info",
		r.Count(SeverityError), r.Count(SeverityWarn), r.Count(SeverityInfo))
}
