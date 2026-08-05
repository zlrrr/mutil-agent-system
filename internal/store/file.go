package store

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/zlrrr/mutil-agent-system/internal/domain"
)

// sdd:impl DLD-1011

// File is the durable adapter: one JSON-lines log per case, plus a header file per
// case. The case index is rebuilt by scanning the directory, so it is derived rather
// than authoritative (ADR-004).
type File struct {
	mu   sync.RWMutex
	root string
	mem  *Memory // in-memory mirror, rebuilt on open
}

// NewFile opens (or creates) a file-backed store rooted at dir.
func NewFile(dir string) (*File, error) {
	if err := os.MkdirAll(filepath.Join(dir, "cases"), 0o755); err != nil {
		return nil, fmt.Errorf("create store root: %w", err)
	}
	f := &File{root: dir, mem: NewMemory()}
	if err := f.rebuild(); err != nil {
		return nil, err
	}
	return f, nil
}

// rebuild reconstructs the index and the event mirror by scanning the case directory.
func (f *File) rebuild() error {
	dir := filepath.Join(f.root, "cases")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("scan store: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		caseID := strings.TrimSuffix(name, ".jsonl")
		events, err := readLog(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		if len(events) == 0 {
			continue
		}
		c, err := domain.Replay(caseID, events)
		if err != nil {
			return fmt.Errorf("rebuild %s: %w", caseID, err)
		}
		h := CaseHeader{
			ID: caseID, AlertName: c.Alert.Name, Service: c.Alert.Service,
			Severity: c.Alert.Severity, Mode: c.Mode, Status: c.Status,
			CreatedAt: c.CreatedAt,
		}
		if err := f.mem.CreateCase(context.Background(), h); err != nil {
			return err
		}
		if err := f.mem.Append(context.Background(), caseID, events); err != nil {
			return err
		}
	}
	return nil
}

func readLog(path string) ([]domain.Event, error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open log %s: %w", path, err)
	}
	defer fh.Close()

	var out []domain.Event
	sc := bufio.NewScanner(fh)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	line := 0
	for sc.Scan() {
		line++
		raw := strings.TrimSpace(sc.Text())
		if raw == "" {
			continue
		}
		var e domain.Event
		if err := json.Unmarshal([]byte(raw), &e); err != nil {
			return nil, fmt.Errorf("%s:%d: malformed event: %w", path, line, err)
		}
		out = append(out, e)
	}
	return out, sc.Err()
}

func (f *File) logPath(caseID string) string {
	return filepath.Join(f.root, "cases", caseID+".jsonl")
}

// CreateCase registers a case and creates its (empty) log file.
func (f *File) CreateCase(ctx context.Context, h CaseHeader) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.mem.CreateCase(ctx, h); err != nil {
		return err
	}
	fh, err := os.OpenFile(f.logPath(h.ID), os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("create log: %w", err)
	}
	return fh.Close()
}

// ListCases returns the index.
func (f *File) ListCases(ctx context.Context) ([]CaseHeader, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.mem.ListCases(ctx)
}

// Header returns one index entry.
func (f *File) Header(ctx context.Context, caseID string) (CaseHeader, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.mem.Header(ctx, caseID)
}

// SetStatus updates the derived index entry. It is not persisted separately: a
// restart recomputes it from the log.
func (f *File) SetStatus(ctx context.Context, caseID string, status domain.Status) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.mem.SetStatus(ctx, caseID, status)
}

// Append writes the batch to disk and mirrors it in memory. The mirror is updated only
// after a successful, synced write, so a failed append leaves both consistent.
func (f *File) Append(ctx context.Context, caseID string, events []domain.Event) error {
	if len(events) == 0 {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()

	existing, err := f.mem.Events(ctx, caseID, 0)
	if err != nil {
		return err
	}
	if err := checkContiguous(existing, events); err != nil {
		return err
	}

	fh, err := os.OpenFile(f.logPath(caseID), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open log for append: %w", err)
	}
	w := bufio.NewWriter(fh)
	for _, e := range events {
		raw, err := json.Marshal(e)
		if err != nil {
			fh.Close()
			return fmt.Errorf("encode event %d: %w", e.Seq, err)
		}
		if _, err := w.Write(append(raw, '\n')); err != nil {
			fh.Close()
			return fmt.Errorf("write event %d: %w", e.Seq, err)
		}
	}
	if err := w.Flush(); err != nil {
		fh.Close()
		return fmt.Errorf("flush log: %w", err)
	}
	if err := fh.Sync(); err != nil {
		fh.Close()
		return fmt.Errorf("sync log: %w", err)
	}
	if err := fh.Close(); err != nil {
		return fmt.Errorf("close log: %w", err)
	}
	return f.mem.Append(ctx, caseID, events)
}

// Events returns the log from a sequence number onward.
func (f *File) Events(ctx context.Context, caseID string, fromSeq int) ([]domain.Event, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.mem.Events(ctx, caseID, fromSeq)
}
