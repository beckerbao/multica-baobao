package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestListProjectTaskChanges_ReturnsTasksWithChangeMetadata(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()

	// Create project.
	req := newRequest("POST", "/api/projects?workspace_id="+testWorkspaceID, map[string]any{
		"title": "task-changes-proj",
	})
	w := httptest.NewRecorder()
	testHandler.CreateProject(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("CreateProject: %d %s", w.Code, w.Body.String())
	}
	var project ProjectResponse
	if err := json.NewDecoder(w.Body).Decode(&project); err != nil {
		t.Fatalf("decode project: %v", err)
	}
	t.Cleanup(func() {
		w := httptest.NewRecorder()
		r := newRequest("DELETE", "/api/projects/"+project.ID, nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", project.ID)
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
		testHandler.DeleteProject(w, r)
	})

	var agentID, runtimeID string
	if err := testPool.QueryRow(ctx, `SELECT id, runtime_id FROM agent WHERE workspace_id = $1 LIMIT 1`, testWorkspaceID).Scan(&agentID, &runtimeID); err != nil {
		t.Fatalf("get agent: %v", err)
	}

	// Create issue under project.
	var issueID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO issue (workspace_id, project_id, title, status, priority, creator_id, creator_type, number, position)
		VALUES ($1, $2, 'task-change-issue', 'in_progress', 'none', $3, 'member', 90001, 0)
		RETURNING id
	`, testWorkspaceID, project.ID, testUserID).Scan(&issueID); err != nil {
		t.Fatalf("create issue: %v", err)
	}

	// Insert completed task with change metadata.
	resultJSON := `{"execution_workdir":"/tmp/repo","change_summary":{"collect_status":"ok","changed_files":[{"path":"a.txt","status":"M"}],"diff_stat":{"files_changed":1,"insertions":1,"deletions":0}}}`
	if _, err := testPool.Exec(ctx, `
		INSERT INTO agent_task_queue (agent_id, runtime_id, issue_id, status, priority, result, started_at, completed_at)
		VALUES ($1, $2, $3, 'completed', 0, $4::jsonb, now(), now())
	`, agentID, runtimeID, issueID, resultJSON); err != nil {
		t.Fatalf("create task: %v", err)
	}

	// Query project task changes endpoint.
	w = httptest.NewRecorder()
	req = newRequest("GET", "/api/projects/"+project.ID+"/task-changes?limit=20", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", project.ID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	testHandler.ListProjectTaskChanges(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("ListProjectTaskChanges: %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		Items []AgentTaskResponse `json:"items"`
		Total int                 `json:"total"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Total == 0 || len(resp.Items) == 0 {
		t.Fatalf("expected non-empty changes list, got total=%d len=%d", resp.Total, len(resp.Items))
	}
}
