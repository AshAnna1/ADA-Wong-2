package main

import (
	"os"
	"path/filepath"
	"testing"
)

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
