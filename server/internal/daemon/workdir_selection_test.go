package daemon

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSelectExecutionWorkDir_UsesPreferredWhenValidGitRepo(t *testing.T) {
	defaultDir := t.TempDir()
	preferred := t.TempDir()
	if out, err := exec.Command("git", "-C", preferred, "init").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v (%s)", err, string(out))
	}

	gotDir, gotSource, gotReason := selectExecutionWorkDir(defaultDir, preferred)
	if gotDir != filepath.Clean(preferred) {
		t.Fatalf("dir=%q, want=%q", gotDir, filepath.Clean(preferred))
	}
	if gotSource != "preferred_workdir" {
		t.Fatalf("source=%q, want preferred_workdir", gotSource)
	}
	if gotReason != "preferred_workdir_valid" {
		t.Fatalf("reason=%q, want preferred_workdir_valid", gotReason)
	}
}

func TestSelectExecutionWorkDir_FallbackWhenPreferredInvalid(t *testing.T) {
	defaultDir := t.TempDir()
	invalidPreferred := filepath.Join(t.TempDir(), "missing")

	gotDir, gotSource, gotReason := selectExecutionWorkDir(defaultDir, invalidPreferred)
	if gotDir != defaultDir {
		t.Fatalf("dir=%q, want default=%q", gotDir, defaultDir)
	}
	if gotSource != "task_workdir_fallback" {
		t.Fatalf("source=%q, want task_workdir_fallback", gotSource)
	}
	if gotReason != "path_not_found" {
		t.Fatalf("reason=%q, want path_not_found", gotReason)
	}
}
