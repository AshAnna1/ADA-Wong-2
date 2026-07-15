package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestToolVersion(t *testing.T) {
	if toolVersion != "v1.3" {
		t.Fatalf("expected tool version v1.3, got %s", toolVersion)
	}
}

func TestDocsContent(t *testing.T) {
	content, err := loadDocsContent("docs/ttse-guide.md")
	if err != nil {
		t.Fatalf("loadDocsContent returned error: %v", err)
	}
	if content == "" {
		t.Fatal("expected documentation content to be non-empty")
	}
	if !contains(content, "ttse start") || !contains(content, "ttse docs") {
		t.Fatalf("expected documentation content to include usage examples, got %q", content)
	}
}

func TestHistoryLog(t *testing.T) {
	sandboxRoot := filepath.Join(t.TempDir(), "sandbox")
	if err := appendHistory(sandboxRoot, "started", 1); err != nil {
		t.Fatalf("appendHistory returned error: %v", err)
	}

	entries, err := readHistory(sandboxRoot)
	if err != nil {
		t.Fatalf("readHistory returned error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(entries))
	}
	if entries[0].Action != "started" || entries[0].Step != 1 {
		t.Fatalf("unexpected history entry: %+v", entries[0])
	}
}

func TestPrepareAndCleanupSandbox(t *testing.T) {
	sandboxRoot := filepath.Join(t.TempDir(), "sandbox")

	step, err := prepareSandbox(sandboxRoot)
	if err != nil {
		t.Fatalf("prepareSandbox returned error: %v", err)
	}
	if step != 1 {
		t.Fatalf("expected first step to be 1, got %d", step)
	}

	for _, name := range []string{"upper", "work", "merged", "checkpoints", "step"} {
		if _, err := os.Stat(filepath.Join(sandboxRoot, name)); err != nil {
			t.Fatalf("expected %s to exist: %v", name, err)
		}
	}

	if err := cleanupSandbox(sandboxRoot); err != nil {
		t.Fatalf("cleanupSandbox returned error: %v", err)
	}

	for _, name := range []string{"upper", "work", "merged", "checkpoints", "step"} {
		if _, err := os.Stat(filepath.Join(sandboxRoot, name)); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be removed, got err=%v", name, err)
		}
	}
}
