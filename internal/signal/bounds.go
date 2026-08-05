package signal

import (
	"time"

	"github.com/zlrrr/mutil-agent-system/internal/domain"
)

// sdd:impl DLD-1020

// Bounds caps what any single tool result may carry. Applying the cap in the port —
// rather than trusting each caller — is what makes REQ-0016 hold everywhere.
type Bounds struct {
	MaxRows  int
	MaxChars int
	MaxSpan  time.Duration
}

// DefaultBounds are the values declared in the detailed design.
func DefaultBounds() Bounds {
	return Bounds{MaxRows: 200, MaxChars: 8000, MaxSpan: 2 * time.Hour}
}

// ApplyRowsLog truncates a log result to MaxRows, reporting whether it did.
func (b Bounds) ApplyRowsLog(in []LogLine) ([]LogLine, bool) {
	if b.MaxRows <= 0 || len(in) <= b.MaxRows {
		return in, false
	}
	return in[:b.MaxRows], true
}

// ApplyRowsPoints truncates a series to MaxRows points, keeping the earliest.
func (b Bounds) ApplyRowsPoints(in []Point) ([]Point, bool) {
	if b.MaxRows <= 0 || len(in) <= b.MaxRows {
		return in, false
	}
	return in[:b.MaxRows], true
}

// ApplyRowsChanges truncates a change result to MaxRows.
func (b Bounds) ApplyRowsChanges(in []Change) ([]Change, bool) {
	if b.MaxRows <= 0 || len(in) <= b.MaxRows {
		return in, false
	}
	return in[:b.MaxRows], true
}

// ApplyChars truncates a string to MaxChars on a rune boundary, reporting whether it
// did. An ellipsis marks the cut so a reader is never misled about completeness.
func (b Bounds) ApplyChars(s string) (string, bool) {
	if b.MaxChars <= 0 || len(s) <= b.MaxChars {
		return s, false
	}
	r := []rune(s)
	if len(r) <= b.MaxChars {
		return s, false
	}
	return string(r[:b.MaxChars]) + "…", true
}

// ApplySpan clamps a window to MaxSpan, keeping the end and moving the start forward.
func (b Bounds) ApplySpan(w domain.TimeWindow) (domain.TimeWindow, bool) {
	if b.MaxSpan <= 0 || w.Duration() <= b.MaxSpan {
		return w, false
	}
	return domain.TimeWindow{Start: w.End.Add(-b.MaxSpan), End: w.End}, true
}
