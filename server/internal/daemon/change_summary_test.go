package daemon

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestParseNameStatus(t *testing.T) {
	raw := "M\tserver/main.go\nA\tREADME.md\nD\told.txt\n"
	got := parseNameStatus(raw)
	if len(got) != 3 {
		t.Fatalf("len=%d, want 3", len(got))
	}
	if got[0].Status != "M" || got[0].Path != "server/main.go" {
		t.Fatalf("first=%+v", got[0])
	}
}

func TestParseShortStat(t *testing.T) {
	got := parseShortStat(" 3 files changed, 25 insertions(+), 7 deletions(-)")
	if got.FilesChanged != 3 || got.Insertions != 25 || got.Deletions != 7 {
		t.Fatalf("got=%+v", got)
	}
}

func TestParsePorcelainStatus(t *testing.T) {
	raw := " M a.txt\nA  b.txt\n?? c.txt\n"
	got := parsePorcelainStatus(raw)
	if len(got) != 3 {
		t.Fatalf("len=%d, want 3", len(got))
	}
	if got[0].Status != "M" || got[0].Path != "a.txt" {
		t.Fatalf("first=%+v", got[0])
	}
	if got[2].Status != "??" || got[2].Path != "c.txt" {
		t.Fatalf("third=%+v", got[2])
	}
}

func TestMergeChangeFiles(t *testing.T) {
	primary := []TaskChangeFile{{Path: "a.txt", Status: "M"}}
	secondary := []TaskChangeFile{{Path: "a.txt", Status: "M"}, {Path: "b.txt", Status: "??"}}
	got := mergeChangeFiles(primary, secondary)
	if len(got) != 2 {
		t.Fatalf("len=%d, want 2", len(got))
	}
	if got[1].Path != "b.txt" {
		t.Fatalf("second=%+v", got[1])
	}
}

func TestCollectTaskChangeSummary_GitUnavailable(t *testing.T) {
	dir := t.TempDir()
	got := collectTaskChangeSummary(dir)
	if got == nil || got.CollectStatus != "git_unavailable" {
		t.Fatalf("collect_status=%v, want git_unavailable", got)
	}
}

func TestCollectTaskChangeSummary_MissingExecutionWorkdir(t *testing.T) {
	got := collectTaskChangeSummary("")
	if got == nil || got.CollectStatus != "missing_execution_workdir" {
		t.Fatalf("collect_status=%v, want missing_execution_workdir", got)
	}
}

func TestCollectTaskChangeSummary_OK(t *testing.T) {
	dir := t.TempDir()
	mustGit(t, dir, "init")
	mustWrite(t, filepath.Join(dir, "a.txt"), "before\n")
	mustGit(t, dir, "add", ".")
	mustGit(t, dir, "-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "-m", "init")
	mustWrite(t, filepath.Join(dir, "a.txt"), "after\n")
	mustWrite(t, filepath.Join(dir, "b.txt"), "new\n")

	got := collectTaskChangeSummary(dir)
	if got == nil {
		t.Fatal("nil summary")
	}
	if got.CollectStatus != "ok" {
		t.Fatalf("collect_status=%q, want ok", got.CollectStatus)
	}
	if got.DiffStat.FilesChanged == 0 {
		t.Fatalf("diff_stat=%+v", got.DiffStat)
	}
	if len(got.ChangedFiles) == 0 {
		t.Fatal("expected changed files")
	}
}

func TestCollectTaskChangeSummary_CleanRepo(t *testing.T) {
	dir := t.TempDir()
	mustGit(t, dir, "init")
	mustWrite(t, filepath.Join(dir, "a.txt"), "before\n")
	mustGit(t, dir, "add", ".")
	mustGit(t, dir, "-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "-m", "init")

	got := collectTaskChangeSummary(dir)
	if got == nil {
		t.Fatal("nil summary")
	}
	if got.CollectStatus != "ok" {
		t.Fatalf("collect_status=%q, want ok", got.CollectStatus)
	}
	if len(got.ChangedFiles) != 0 {
		t.Fatalf("changed_files=%d, want 0", len(got.ChangedFiles))
	}
	if got.DiffStat.FilesChanged != 0 || got.DiffStat.Insertions != 0 || got.DiffStat.Deletions != 0 {
		t.Fatalf("diff_stat=%+v, want zeros", got.DiffStat)
	}
}

func TestCollectTaskChangeSummary_Truncated(t *testing.T) {
	dir := t.TempDir()
	mustGit(t, dir, "init")
	mustWrite(t, filepath.Join(dir, "base.txt"), "base\n")
	mustGit(t, dir, "add", ".")
	mustGit(t, dir, "-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "-m", "init")
	for i := 0; i < maxChangedFiles+50; i++ {
		mustWrite(t, filepath.Join(dir, "f_"+strings.Repeat("x", 10)+"_"+strconv.Itoa(i)+".txt"), "x\n")
	}
	got := collectTaskChangeSummary(dir)
	if got == nil {
		t.Fatal("nil summary")
	}
	if got.CollectStatus != "truncated" {
		t.Fatalf("collect_status=%q, want truncated", got.CollectStatus)
	}
	if len(got.ChangedFiles) > maxChangedFiles {
		t.Fatalf("changed_files=%d, want <= %d", len(got.ChangedFiles), maxChangedFiles)
	}
}

func TestCollectTaskChangeSummaryWithBaseline_UsesBaselineDelta(t *testing.T) {
	dir := t.TempDir()
	mustGit(t, dir, "init")
	mustWrite(t, filepath.Join(dir, "a.txt"), "v1\n")
	mustGit(t, dir, "add", ".")
	mustGit(t, dir, "-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "-m", "init")
	baseline := strings.TrimSpace(gitOutput(t, dir, "rev-parse", "HEAD"))

	mustWrite(t, filepath.Join(dir, "a.txt"), "v2\n")
	mustWrite(t, filepath.Join(dir, "b.txt"), "new\n")
	mustGit(t, dir, "add", ".")
	mustGit(t, dir, "-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "-m", "second")

	// Untracked files should still appear via porcelain merge.
	mustWrite(t, filepath.Join(dir, "c.txt"), "untracked\n")

	got := collectTaskChangeSummaryWithBaseline(dir, baseline)
	if got == nil {
		t.Fatal("nil summary")
	}
	if got.CollectStatus != "ok" {
		t.Fatalf("collect_status=%q, want ok", got.CollectStatus)
	}
	paths := map[string]bool{}
	for _, f := range got.ChangedFiles {
		paths[f.Path] = true
	}
	if !paths["a.txt"] || !paths["b.txt"] || !paths["c.txt"] {
		t.Fatalf("changed_files missing expected paths: %+v", got.ChangedFiles)
	}
}

func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v (%s)", args, err, string(out))
	}
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v (%s)", args, err, string(out))
	}
	return string(out)
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
