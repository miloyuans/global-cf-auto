package domain

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSourcesParsesMetadata(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "domains.txt")
	data := "gamestores.us.com|yuang6496|2026-01-03\nexample.com\n"
	if err := os.WriteFile(filePath, []byte(data), 0o644); err != nil {
		t.Fatalf("failed to write temp domain file: %v", err)
	}

	repo := NewFileRepository([]string{filePath}, "", "", "")
	sources, err := repo.LoadSources()
	if err != nil {
		t.Fatalf("LoadSources returned error: %v", err)
	}

	if len(sources) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(sources))
	}

	first := sources[0]
	if first.Domain != "gamestores.us.com" {
		t.Errorf("unexpected domain: %s", first.Domain)
	}
	if first.Source != "yuang6496" {
		t.Errorf("expected source to use second column, got %s", first.Source)
	}
	if first.Expiry != "2026-01-03" {
		t.Errorf("unexpected expiry: %s", first.Expiry)
	}

	second := sources[1]
	if second.Domain != "example.com" {
		t.Errorf("unexpected domain: %s", second.Domain)
	}
	if second.Source != filePath {
		t.Errorf("expected source to fall back to path, got %s", second.Source)
	}
	if second.Expiry != "" {
		t.Errorf("expected empty expiry, got %s", second.Expiry)
	}
}
