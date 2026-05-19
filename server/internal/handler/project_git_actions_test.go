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

func TestProjectGitAction_StatusAndBranchList(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	t.Setenv(projectGitActionFeatureFlag, "true")
	allowedRoot := t.TempDir()
	t.Setenv("LOCAL_PROJECT_PATH_ALLOWED_ROOTS", filepath.Clean(allowedRoot))
	repoDir := filepath.Join(allowedRoot, "repo-actions")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	mustGitRun(t, repoDir, "init")
	mustWriteGitFile(t, filepath.Join(repoDir, "a.txt"), "x\n")
	mustGitRun(t, repoDir, "add", ".")
	mustGitRun(t, repoDir, "-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "-m", "init")
	projectID := createProjectWithLocalPathForGitActions(t, repoDir, "daemon-action-1")

	for _, action := range []string{"status", "branch_list"} {
		w := httptest.NewRecorder()
		req := newRequest("POST", "/api/projects/"+projectID+"/git-actions", map[string]any{"action": action})
		req = withURLParam(req, "id", projectID)
		testHandler.ProjectGitAction(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: expected 200, got %d: %s", action, w.Code, w.Body.String())
		}
		var resp projectGitActionResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("%s decode: %v", action, err)
		}
		if !resp.OK || resp.CollectStatus != "ok" {
			t.Fatalf("%s resp=%+v", action, resp)
		}
	}
}

func TestProjectGitAction_CheckoutValidationAndDirtyTreeBlock(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	t.Setenv(projectGitActionFeatureFlag, "true")
	allowedRoot := t.TempDir()
	t.Setenv("LOCAL_PROJECT_PATH_ALLOWED_ROOTS", filepath.Clean(allowedRoot))
	repoDir := filepath.Join(allowedRoot, "repo-checkout")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	mustGitRun(t, repoDir, "init")
	mustWriteGitFile(t, filepath.Join(repoDir, "a.txt"), "x\n")
	mustGitRun(t, repoDir, "add", ".")
	mustGitRun(t, repoDir, "-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "-m", "init")
	mustGitRun(t, repoDir, "checkout", "-b", "feature/test")
	mustGitRun(t, repoDir, "checkout", "master")
	mustWriteGitFile(t, filepath.Join(repoDir, "a.txt"), "dirty\n")
	projectID := createProjectWithLocalPathForGitActions(t, repoDir, "daemon-action-2")

	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/projects/"+projectID+"/git-actions", map[string]any{
		"action": "checkout_existing_branch",
		"branch": "feature/test",
	})
	req = withURLParam(req, "id", projectID)
	testHandler.ProjectGitAction(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("checkout: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp projectGitActionResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.CollectStatus != "dirty_tree_overwrite_risk" {
		t.Fatalf("collect_status=%q, want dirty_tree_overwrite_risk", resp.CollectStatus)
	}
}

func TestProjectGitAction_NotGitRepo(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	t.Setenv(projectGitActionFeatureFlag, "true")
	allowedRoot := t.TempDir()
	t.Setenv("LOCAL_PROJECT_PATH_ALLOWED_ROOTS", filepath.Clean(allowedRoot))
	dir := filepath.Join(allowedRoot, "plain-dir")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	projectID := createProjectWithLocalPathForGitActions(t, dir, "daemon-action-3")

	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/projects/"+projectID+"/git-actions", map[string]any{"action": "status"})
	req = withURLParam(req, "id", projectID)
	testHandler.ProjectGitAction(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp projectGitActionResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.CollectStatus != "not_git_repo" {
		t.Fatalf("collect_status=%q, want not_git_repo", resp.CollectStatus)
	}
}

func TestProjectGitAction_UnknownActionReturns400(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	t.Setenv(projectGitActionFeatureFlag, "true")
	projectID := createProjectWithLocalPathForGitActions(t, t.TempDir(), "daemon-action-4")
	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/projects/"+projectID+"/git-actions", map[string]any{"action": "reset_hard"})
	req = withURLParam(req, "id", projectID)
	testHandler.ProjectGitAction(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestProjectGitAction_FetchAndPullFFOnly(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	t.Setenv(projectGitActionFeatureFlag, "true")
	root := t.TempDir()
	remoteBare := filepath.Join(root, "remote.git")
	if err := os.MkdirAll(remoteBare, 0o755); err != nil {
		t.Fatalf("mkdir remote: %v", err)
	}
	mustGitRun(t, root, "init", "--bare", remoteBare)

	seed := filepath.Join(root, "seed")
	if err := os.MkdirAll(seed, 0o755); err != nil {
		t.Fatalf("mkdir seed: %v", err)
	}
	mustGitRun(t, seed, "init")
	mustWriteGitFile(t, filepath.Join(seed, "a.txt"), "init\n")
	mustGitRun(t, seed, "add", ".")
	mustGitRun(t, seed, "-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "-m", "init")
	mustGitRun(t, seed, "remote", "add", "origin", remoteBare)
	mustGitRun(t, seed, "branch", "-M", "main")
	mustGitRun(t, seed, "push", "-u", "origin", "main")

	local := filepath.Join(root, "local")
	mustGitRun(t, root, "clone", remoteBare, local)
	mustGitRun(t, local, "checkout", "-b", "main", "origin/main")

	projectID := createProjectWithLocalPathForGitActions(t, local, "daemon-action-5")

	// fetch success
	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/projects/"+projectID+"/git-actions", map[string]any{"action": "fetch"})
	req = withURLParam(req, "id", projectID)
	testHandler.ProjectGitAction(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("fetch: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var fetchResp projectGitActionResponse
	if err := json.NewDecoder(w.Body).Decode(&fetchResp); err != nil {
		t.Fatalf("decode fetch: %v", err)
	}
	if !fetchResp.OK {
		t.Fatalf("fetch failed: %+v", fetchResp)
	}

	// update remote through seed, then local pull --ff-only should succeed
	mustWriteGitFile(t, filepath.Join(seed, "b.txt"), "from remote\n")
	mustGitRun(t, seed, "add", ".")
	mustGitRun(t, seed, "-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "-m", "remote update")
	mustGitRun(t, seed, "push", "origin", "main")

	w = httptest.NewRecorder()
	req = newRequest("POST", "/api/projects/"+projectID+"/git-actions", map[string]any{"action": "pull_ff_only"})
	req = withURLParam(req, "id", projectID)
	testHandler.ProjectGitAction(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("pull: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var pullResp projectGitActionResponse
	if err := json.NewDecoder(w.Body).Decode(&pullResp); err != nil {
		t.Fatalf("decode pull: %v", err)
	}
	if !pullResp.OK {
		t.Fatalf("pull failed: %+v", pullResp)
	}

	// create diverged local commit then remote commit to force ff-only failure
	mustWriteGitFile(t, filepath.Join(local, "local-only.txt"), "local\n")
	mustGitRun(t, local, "add", ".")
	mustGitRun(t, local, "-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "-m", "local diverge")
	mustWriteGitFile(t, filepath.Join(seed, "remote-2.txt"), "remote2\n")
	mustGitRun(t, seed, "add", ".")
	mustGitRun(t, seed, "-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "-m", "remote diverge")
	mustGitRun(t, seed, "push", "origin", "main")

	w = httptest.NewRecorder()
	req = newRequest("POST", "/api/projects/"+projectID+"/git-actions", map[string]any{"action": "pull_ff_only"})
	req = withURLParam(req, "id", projectID)
	testHandler.ProjectGitAction(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("pull fail: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var pullFailResp projectGitActionResponse
	if err := json.NewDecoder(w.Body).Decode(&pullFailResp); err != nil {
		t.Fatalf("decode pull fail: %v", err)
	}
	if pullFailResp.OK || pullFailResp.ErrorCode != "ff_only_failed" {
		t.Fatalf("expected ff_only_failed, got %+v", pullFailResp)
	}
}

func TestProjectGitLockManager_Behavior(t *testing.T) {
	m := newProjectGitLockManager()
	if !m.TryAcquire("p1") {
		t.Fatal("first acquire should succeed")
	}
	if m.TryAcquire("p1") {
		t.Fatal("second acquire should fail while held")
	}
	m.Release("p1")
	if !m.TryAcquire("p1") {
		t.Fatal("acquire after release should succeed")
	}
}

func createProjectWithLocalPathForGitActions(t *testing.T, localPath, daemonID string) string {
	t.Helper()
	ctx := context.Background()
	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/projects?workspace_id="+testWorkspaceID, map[string]any{
		"title": "Project git action fixture",
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

func mustGitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v (%s)", args, err, string(out))
	}
}

func mustWriteGitFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}
