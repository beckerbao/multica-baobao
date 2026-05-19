package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGetProjectLiveGitStatus_OK_WithChangedAndUntracked(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	allowedRoot := t.TempDir()
	t.Setenv("LOCAL_PROJECT_PATH_ALLOWED_ROOTS", filepath.Clean(allowedRoot))
	repoDir := filepath.Join(allowedRoot, "live-git-repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	mustGitCmd(t, repoDir, "init")
	mustWriteFile(t, filepath.Join(repoDir, "a.txt"), "before\n")
	mustGitCmd(t, repoDir, "add", ".")
	mustGitCmd(t, repoDir, "-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "-m", "init")
	mustWriteFile(t, filepath.Join(repoDir, "a.txt"), "after\n")
	mustWriteFile(t, filepath.Join(repoDir, "b.txt"), "new\n")

	projectID := createProjectWithLocalPath(t, repoDir, "daemon-live-1")

	w := httptest.NewRecorder()
	req := newRequest("GET", "/api/projects/"+projectID+"/live-git-status", nil)
	req = withURLParam(req, "id", projectID)
	testHandler.GetProjectLiveGitStatus(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GetProjectLiveGitStatus: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp ProjectLiveGitStatusResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.CollectStatus != "ok" {
		t.Fatalf("collect_status=%q, want ok", resp.CollectStatus)
	}
	if resp.ExecutionWorkDir != repoDir {
		t.Fatalf("execution_workdir=%q, want %q", resp.ExecutionWorkDir, repoDir)
	}
	if len(resp.ChangedFiles) == 0 {
		t.Fatal("expected changed_files > 0")
	}
}

func TestGetProjectLiveGitStatus_GitUnavailable(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	allowedRoot := t.TempDir()
	t.Setenv("LOCAL_PROJECT_PATH_ALLOWED_ROOTS", filepath.Clean(allowedRoot))
	nonGitDir := filepath.Join(allowedRoot, "non-git")
	if err := os.MkdirAll(nonGitDir, 0o755); err != nil {
		t.Fatalf("mkdir non-git: %v", err)
	}
	projectID := createProjectWithLocalPath(t, nonGitDir, "daemon-live-2")

	w := httptest.NewRecorder()
	req := newRequest("GET", "/api/projects/"+projectID+"/live-git-status", nil)
	req = withURLParam(req, "id", projectID)
	testHandler.GetProjectLiveGitStatus(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GetProjectLiveGitStatus: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp ProjectLiveGitStatusResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.CollectStatus != "git_unavailable" {
		t.Fatalf("collect_status=%q, want git_unavailable", resp.CollectStatus)
	}
}

func createProjectWithLocalPath(t *testing.T, localPath, daemonID string) string {
	t.Helper()
	ctx := context.Background()
	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/projects?workspace_id="+testWorkspaceID, map[string]any{
		"title": "Project live git status fixture",
	})
	testHandler.CreateProject(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("CreateProject: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var project ProjectResponse
	if err := json.NewDecoder(w.Body).Decode(&project); err != nil {
		t.Fatalf("decode project: %v", err)
	}
	t.Cleanup(func() {
		r := newRequest("DELETE", "/api/projects/"+project.ID, nil)
		r = withURLParam(r, "id", project.ID)
		testHandler.DeleteProject(httptest.NewRecorder(), r)
	})

	if _, err := testPool.Exec(ctx, `
		INSERT INTO project_local_repo_path (workspace_id, project_id, daemon_id, local_path)
		VALUES ($1, $2, $3, $4)
	`, testWorkspaceID, project.ID, daemonID, localPath); err != nil {
		t.Fatalf("seed local path: %v", err)
	}
	return project.ID
}

func mustGitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v (%s)", args, err, string(out))
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

