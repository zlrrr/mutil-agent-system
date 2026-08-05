package sdd

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// sdd:impl DLD-0103

// Item is one normative specification unit declared by an `sdd:item` anchor.
type Item struct {
	ID          string
	Stage       string
	Status      string
	Priority    string
	DerivesFrom []string

	Title        string
	HeadingLevel int

	// Occurrences records where the item was found, one entry per language.
	Occurrences []Occurrence
}

// Occurrence is a single language rendering of an item.
type Occurrence struct {
	Lang string
	File string
	Line int
	Body string
}

// ContentHash is the hash of the item across every language rendering. Because
// the bilingual pair is a single artifact (CON-004), a change in either language
// changes the hash and therefore cascades to descendants (CON-003).
func (it *Item) ContentHash() string {
	h := sha256.New()
	for _, occ := range it.Occurrences {
		h.Write([]byte(occ.Lang))
		h.Write([]byte{0})
		h.Write([]byte(normalize(occ.Body)))
		h.Write([]byte{0})
	}
	// Attributes are normative too: retargeting a parent is a change.
	fmt.Fprintf(h, "stage=%s;status=%s;priority=%s;from=%s",
		it.Stage, it.Status, it.Priority, strings.Join(it.DerivesFrom, ","))
	return hex.EncodeToString(h.Sum(nil))
}

// PrimaryFile returns the file of the primary-language occurrence, falling back
// to the first occurrence recorded.
func (it *Item) PrimaryFile(primary string) string {
	for _, o := range it.Occurrences {
		if o.Lang == primary {
			return o.File
		}
	}
	if len(it.Occurrences) > 0 {
		return it.Occurrences[0].File
	}
	return ""
}

// Doc is a parsed specification document.
type Doc struct {
	Path        string
	RelPath     string
	Lang        string
	Base        string // path without the language suffix
	FrontMatter map[string]string
	ItemIDs     []string
	Headings    []Heading
}

// Heading is one markdown heading, used for structural parity checks.
type Heading struct {
	Level int
	Text  string
	Line  int
}

// CodeAnchor is an `sdd:impl` or `sdd:verify` reference found in source.
type CodeAnchor struct {
	Kind    string // "impl" or "verify"
	Targets []string
	File    string
	Line    int
	IsTest  bool
}

var (
	itemAnchorRe = regexp.MustCompile(`<!--\s*sdd:item\s+([^>]*?)-->`)
	headingRe    = regexp.MustCompile(`^(#{1,6})\s+(.*?)\s*$`)
	codeAnchorRe = regexp.MustCompile(`sdd:(impl|verify)\s+([A-Za-z]+-[0-9A-Za-z_,\s-]*)`)
	langSuffixRe = regexp.MustCompile(`\.([a-z]{2}(?:-[A-Za-z]{2,4})?)\.md$`)
	fenceRe      = regexp.MustCompile("^\\s*```")
)

// ParseDoc reads and parses a single markdown specification document.
func ParseDoc(root, path string) (*Doc, []*Item, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	rel, _ := filepath.Rel(root, path)
	rel = filepath.ToSlash(rel)

	doc := &Doc{Path: path, RelPath: rel, FrontMatter: map[string]string{}}
	if m := langSuffixRe.FindStringSubmatch(filepath.Base(path)); m != nil {
		doc.Lang = m[1]
		doc.Base = strings.TrimSuffix(rel, "."+m[1]+".md")
	} else {
		doc.Base = strings.TrimSuffix(rel, ".md")
	}

	lines := splitLines(string(raw))
	idx := 0

	// Front matter: a leading `---` fenced block of `key: value` pairs.
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		idx = 1
		for idx < len(lines) && strings.TrimSpace(lines[idx]) != "---" {
			if k, v, ok := strings.Cut(lines[idx], ":"); ok {
				doc.FrontMatter[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
			}
			idx++
		}
		idx++ // consume the closing marker
	}

	var items []*Item
	inFence := false
	type pending struct {
		item      *Item
		bodyStart int
	}
	var open *pending

	closeItem := func(endLine int) {
		if open == nil {
			return
		}
		body := strings.Join(lines[open.bodyStart:endLine], "\n")
		open.item.Occurrences = []Occurrence{{
			Lang: doc.Lang, File: doc.RelPath, Line: open.bodyStart, Body: body,
		}}
		items = append(items, open.item)
		open = nil
	}

	for i := idx; i < len(lines); i++ {
		line := lines[i]
		if fenceRe.MatchString(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}

		if m := itemAnchorRe.FindStringSubmatch(line); m != nil {
			closeItem(i)
			it := parseItemAttrs(m[1])
			it.HeadingLevel = 0
			open = &pending{item: it, bodyStart: i + 1}
			doc.ItemIDs = append(doc.ItemIDs, it.ID)
			continue
		}

		if hm := headingRe.FindStringSubmatch(line); hm != nil {
			level := len(hm[1])
			text := strings.TrimSpace(hm[2])
			doc.Headings = append(doc.Headings, Heading{Level: level, Text: text, Line: i + 1})

			if open != nil {
				if open.item.HeadingLevel == 0 {
					// First heading after the anchor titles the item.
					open.item.HeadingLevel = level
					open.item.Title = stripIDPrefix(open.item.ID, text)
					continue
				}
				if level <= open.item.HeadingLevel {
					closeItem(i)
				}
			}
		}
	}
	closeItem(len(lines))

	for _, it := range items {
		for k := range it.Occurrences {
			it.Occurrences[k].File = doc.RelPath
		}
	}
	return doc, items, nil
}

func parseItemAttrs(s string) *Item {
	it := &Item{}
	for _, field := range strings.Fields(s) {
		k, v, ok := strings.Cut(field, "=")
		if !ok {
			continue
		}
		v = strings.Trim(v, `"'`)
		switch k {
		case "id":
			it.ID = v
		case "stage":
			it.Stage = v
		case "status":
			it.Status = v
		case "priority":
			it.Priority = strings.ToUpper(v)
		case "derives_from":
			for _, p := range strings.Split(v, ",") {
				if p = strings.TrimSpace(p); p != "" {
					it.DerivesFrom = append(it.DerivesFrom, p)
				}
			}
		}
	}
	return it
}

// stripIDPrefix removes a leading "ID — " or "ID -" from a heading title so that
// the stored title is the human-readable part only.
func stripIDPrefix(id, text string) string {
	t := strings.TrimSpace(strings.TrimPrefix(text, id))
	t = strings.TrimLeft(t, " —-–:")
	if t == "" {
		return text
	}
	return t
}

// ParseCodeAnchors scans a Go source tree for implementation and verification
// anchors (CON-001, CON-006).
func ParseCodeAnchors(root string, codeRoots []string) ([]CodeAnchor, error) {
	var anchors []CodeAnchor
	for _, cr := range codeRoots {
		dir := filepath.Join(root, cr)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer f.Close()
			rel, _ := filepath.Rel(root, path)
			rel = filepath.ToSlash(rel)
			isTest := strings.HasSuffix(path, "_test.go")

			sc := bufio.NewScanner(f)
			sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
			ln := 0
			for sc.Scan() {
				ln++
				line := sc.Text()
				if !strings.Contains(line, "sdd:") {
					continue
				}
				// Only comment lines declare anchors; this keeps string literals
				// that merely mention the keyword from being treated as anchors.
				trimmed := strings.TrimSpace(line)
				if !strings.HasPrefix(trimmed, "//") && !strings.HasPrefix(trimmed, "*") {
					continue
				}
				for _, m := range codeAnchorRe.FindAllStringSubmatch(line, -1) {
					var targets []string
					for _, t := range strings.FieldsFunc(m[2], func(r rune) bool {
						return r == ',' || r == ' ' || r == '\t'
					}) {
						if PrefixOf(t) != "" {
							targets = append(targets, t)
						}
					}
					if len(targets) == 0 {
						continue
					}
					anchors = append(anchors, CodeAnchor{
						Kind: m[1], Targets: targets, File: rel, Line: ln, IsTest: isTest,
					})
				}
			}
			return sc.Err()
		})
		if err != nil {
			return nil, err
		}
	}
	return anchors, nil
}

// CollectDocs walks the configured document roots and returns every markdown file.
func CollectDocs(root string, docRoots []string) ([]string, error) {
	var out []string
	for _, dr := range docRoots {
		dir := filepath.Join(root, dr)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".md") {
				return nil
			}
			out = append(out, path)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.Split(s, "\n")
}

// normalize makes hashing insensitive to cosmetic edits: trailing whitespace and
// runs of blank lines. Any change to the words themselves still cascades.
func normalize(s string) string {
	lines := splitLines(s)
	out := make([]string, 0, len(lines))
	blank := false
	for _, l := range lines {
		l = strings.TrimRight(l, " \t")
		if l == "" {
			if blank {
				continue
			}
			blank = true
		} else {
			blank = false
		}
		out = append(out, l)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}
