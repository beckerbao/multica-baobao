package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestProjectLocalRepoPathLifecycle(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	allowedRoot := t.TempDir()
	t.Setenv("LOCAL_PROJECT_PATH_ALLOWED_ROOTS", filepath.Clean(allowedRoot))
	allowedPath := filepath.Join(allowedRoot, "ms-activity")

	// Create project fixture.
	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/projects?workspace_id="+testWorkspaceID, map[string]any{
		"title": "Project local repo path lifecycle",
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

	// Upsert mapping.
	w = httptest.NewRecorder()
	req = newRequest("POST", "/api/projects/"+project.ID+"/local-repo-paths", map[string]any{
		"daemon_id":   "daemon-debug-1",
		"local_path":  allowedPath,
		"branch_hint": "main",
	})
	req = withURLParam(req, "id", project.ID)
	testHandler.UpsertProjectLocalRepoPath(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("UpsertProjectLocalRepoPath: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var upsertResp ProjectLocalRepoPathResponse
	if err := json.NewDecoder(w.Body).Decode(&upsertResp); err != nil {
		t.Fatalf("decode upsert: %v", err)
	}
	if upsertResp.DaemonID != "daemon-debug-1" || upsertResp.LocalPath != allowedPath {
		t.Fatalf("unexpected upsert response: %+v", upsertResp)
	}

	// List mapping.
	w = httptest.NewRecorder()
	req = newRequest("GET", "/api/projects/"+project.ID+"/local-repo-paths", nil)
	req = withURLParam(req, "id", project.ID)
	testHandler.ListProjectLocalRepoPaths(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("ListProjectLocalRepoPaths: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var listResp struct {
		Items []ProjectLocalRepoPathResponse `json:"items"`
		Total int                            `json:"total"`
	}
	if err := json.NewDecoder(w.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if listResp.Total != 1 || len(listResp.Items) != 1 {
		t.Fatalf("expected 1 local repo mapping, got total=%d items=%d", listResp.Total, len(listResp.Items))
	}

	// Invalid path: non-absolute.
	w = httptest.NewRecorder()
	req = newRequest("POST", "/api/projects/"+project.ID+"/local-repo-paths", map[string]any{
		"daemon_id":  "daemon-debug-1",
		"local_path": "relative/path",
	})
	req = withURLParam(req, "id", project.ID)
	testHandler.UpsertProjectLocalRepoPath(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for relative local_path, got %d: %s", w.Code, w.Body.String())
	}

	// Delete mapping.
	w = httptest.NewRecorder()
	req = newRequest("DELETE", "/api/projects/"+project.ID+"/local-repo-paths/daemon-debug-1", nil)
	req = withURLParams(req, "id", project.ID, "daemonId", "daemon-debug-1")
	testHandler.DeleteProjectLocalRepoPath(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("DeleteProjectLocalRepoPath: expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestProjectLocalRepoPathWriteRequiresAdminOrOwner(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	allowedRoot := t.TempDir()
	t.Setenv("LOCAL_PROJECT_PATH_ALLOWED_ROOTS", filepath.Clean(allowedRoot))
	allowedPath := filepath.Join(allowedRoot, "repo")
	ctx := context.Background()

	// Create project fixture as owner.
	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/projects?workspace_id="+testWorkspaceID, map[string]any{
		"title": "Project local repo path permission",
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

	// Seed a non-admin workspace member.
	var memberUserID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO "user" (name, email)
		VALUES ('local-path-member', 'local-path-member@multica.test')
		RETURNING id
	`).Scan(&memberUserID); err != nil {
		t.Fatalf("create member user: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM "user" WHERE id = $1`, memberUserID)
	})
	if _, err := testPool.Exec(ctx, `
		INSERT INTO member (workspace_id, user_id, role) VALUES ($1, $2, 'member')
	`, testWorkspaceID, memberUserID); err != nil {
		t.Fatalf("add member: %v", err)
	}

	// Member cannot upsert mapping.
	w = httptest.NewRecorder()
	req = newRequestAs(memberUserID, "POST", "/api/projects/"+project.ID+"/local-repo-paths", map[string]any{
		"daemon_id":  "daemon-debug-2",
		"local_path": allowedPath,
	})
	req = withURLParam(req, "id", project.ID)
	testHandler.UpsertProjectLocalRepoPath(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin member, got %d: %s", w.Code, w.Body.String())
	}
}
