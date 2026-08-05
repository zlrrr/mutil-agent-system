package signal

import (
	"sort"
	"strings"
	"time"
	"unicode"
)

// sdd:impl DLD-1022

// Log clustering constants, declared in the detailed design (DLD section 2).
const (
	LogSamplesPerCluster = 3
	MaxClusters          = 5
)

// Cluster is a group of log lines sharing a normalised message template.
type Cluster struct {
	Template  string
	Count     int
	FirstSeen time.Time
	LastSeen  time.Time
	Samples   []string
	Level     string
}

// Normalise reduces a log message to its template by replacing the parts that vary
// between occurrences: digit runs, quoted segments and long hexadecimal identifiers.
func Normalise(msg string) string {
	var b strings.Builder
	runes := []rune(msg)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case r == '"' || r == '\'':
			quote := r
			b.WriteString(`"…"`)
			i++
			for i < len(runes) && runes[i] != quote {
				i++
			}
		case unicode.IsDigit(r):
			// A long hexadecimal-looking run is an identifier, not a quantity.
			j := i
			for j < len(runes) && isHexRune(runes[j]) {
				j++
			}
			if j-i >= 8 {
				b.WriteString("<id>")
				i = j - 1
				continue
			}
			for i < len(runes) && unicode.IsDigit(runes[i]) {
				i++
			}
			i--
			b.WriteByte('#')
		case isHexRune(r):
			j := i
			for j < len(runes) && isHexRune(runes[j]) {
				j++
			}
			if j-i >= 8 && containsDigit(runes[i:j]) {
				b.WriteString("<id>")
				i = j - 1
				continue
			}
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

func isHexRune(r rune) bool {
	return unicode.IsDigit(r) ||
		(r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}

func containsDigit(rs []rune) bool {
	for _, r := range rs {
		if unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

// ClusterLines groups lines by normalised template and returns at most MaxClusters,
// ordered by count descending, then first-seen ascending, then template ascending — a
// total order, so the result is identical on every run (REQ-0012).
func ClusterLines(lines []LogLine) []Cluster {
	byTemplate := map[string]*Cluster{}
	for _, l := range lines {
		key := Normalise(l.Message)
		c, ok := byTemplate[key]
		if !ok {
			c = &Cluster{Template: key, FirstSeen: l.At, LastSeen: l.At, Level: l.Level}
			byTemplate[key] = c
		}
		c.Count++
		if l.At.Before(c.FirstSeen) {
			c.FirstSeen = l.At
		}
		if l.At.After(c.LastSeen) {
			c.LastSeen = l.At
		}
		if len(c.Samples) < LogSamplesPerCluster {
			c.Samples = append(c.Samples, l.Message)
		}
	}

	out := make([]Cluster, 0, len(byTemplate))
	for _, c := range byTemplate {
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		if !out[i].FirstSeen.Equal(out[j].FirstSeen) {
			return out[i].FirstSeen.Before(out[j].FirstSeen)
		}
		return out[i].Template < out[j].Template
	})
	if len(out) > MaxClusters {
		out = out[:MaxClusters]
	}
	return out
}
