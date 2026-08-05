package sdd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sdd:verify TC-9013
func TestNoExternalDependencies(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	for i, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}
		if strings.HasPrefix(trimmed, "require") {
			t.Errorf("go.mod:%d declares an external dependency (CON-008): %q", i+1, trimmed)
		}
	}
	if _, err := os.Stat(filepath.Join("..", "..", "go.sum")); err == nil {
		t.Error("go.sum exists, which means a module outside the standard library was resolved")
	}
}

// sdd:verify TC-9013
func TestVendorDirectoryAbsent(t *testing.T) {
	if _, err := os.Stat(filepath.Join("..", "..", "vendor")); err == nil {
		t.Error("a vendor directory implies third-party code (CON-008)")
	}
}
