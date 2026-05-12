package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVerifyGitLabWebhookToken(t *testing.T) {
	secret := "gitlab-secret"
	if !verifyGitLabWebhookToken(secret, secret) {
		t.Fatal("expected valid token to verify")
	}
	if verifyGitLabWebhookToken(secret, "wrong") {
		t.Fatal("expected invalid token to fail")
	}
	if verifyGitLabWebhookToken("", secret) {
		t.Fatal("expected empty secret to fail")
	}
	if verifyGitLabWebhookToken(secret, "") {
		t.Fatal("expected empty token to fail")
	}
}

func TestDeriveGitLabMRState(t *testing.T) {
	cases := []struct {
		action string
		state  string
		want   string
		ok     bool
	}{
		{action: "open", state: "opened", want: "open", ok: true},
		{action: "update", state: "opened", want: "open", ok: true},
		{action: "update", state: "closed", want: "closed", ok: true},
		{action: "merge", state: "merged", want: "merged", ok: true},
		{action: "note", state: "opened", want: "", ok: false},
	}
	for _, tc := range cases {
		got, ok := deriveGitLabMRState(tc.action, tc.state)
		if ok != tc.ok || got != tc.want {
			t.Fatalf("deriveGitLabMRState(%q, %q) = (%q, %v), want (%q, %v)",
				tc.action, tc.state, got, ok, tc.want, tc.ok)
		}
	}
}

func TestHandleGitLabWebhook_AcceptsMergeRequestEvent(t *testing.T) {
	if testHandler == nil {
		t.Skip("handler test fixture not initialized")
	}
	t.Setenv("GITLAB_WEBHOOK_SECRET", "gl-secret")

	body := map[string]any{
		"object_kind": "merge_request",
		"user": map[string]any{
			"username":   "alice",
			"avatar_url": "",
		},
		"project": map[string]any{
			"web_url":             "https://gitlab.com/example/repo",
			"path_with_namespace": "example/repo",
		},
		"object_attributes": map[string]any{
			"iid":           101,
			"title":         "MR title",
			"state":         "opened",
			"action":        "update",
			"source_branch": "feature/test",
			"url":           "https://gitlab.com/example/repo/-/merge_requests/101",
			"created_at":    "2026-05-01T00:00:00Z",
			"updated_at":    "2026-05-02T00:00:00Z",
		},
	}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/webhooks/gitlab", bytes.NewReader(raw))
	req.Header.Set("X-Gitlab-Event", "Merge Request Hook")
	req.Header.Set("X-Gitlab-Token", "gl-secret")

	w := httptest.NewRecorder()
	testHandler.HandleGitLabWebhook(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d (%s)", w.Code, w.Body.String())
	}
}

func TestHandleGitLabWebhook_PersistsAndLinksIssue(t *testing.T) {
	if testHandler == nil {
		t.Skip("handler test fixture not initialized")
	}
	ctx := context.Background()
	t.Setenv("GITLAB_WEBHOOK_SECRET", "gl-secret")
	var regclass *string
	if err := testPool.QueryRow(ctx, `SELECT to_regclass('public.gitlab_merge_request')`).Scan(&regclass); err != nil {
		t.Fatalf("check gitlab schema: %v", err)
	}
	if regclass == nil || *regclass == "" {
		t.Skip("gitlab integration tables are not migrated in this test database")
	}

	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/issues?workspace_id="+testWorkspaceID, map[string]any{
		"title": "GitLab webhook link test",
	})
	testHandler.CreateIssue(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("CreateIssue: %d %s", w.Code, w.Body.String())
	}
	var created IssueResponse
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatalf("decode issue: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM issue_gitlab_merge_request WHERE issue_id = $1`, created.ID)
		testPool.Exec(ctx, `DELETE FROM gitlab_merge_request WHERE workspace_id = $1`, testWorkspaceID)
		testPool.Exec(ctx, `DELETE FROM issue WHERE id = $1`, created.ID)
	})

	var projectID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO project (workspace_id, title) VALUES ($1, $2) RETURNING id
	`, testWorkspaceID, "gitlab webhook project").Scan(&projectID); err != nil {
		t.Fatalf("create project: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM project WHERE id = $1`, projectID)
	})

	if _, err := testPool.Exec(ctx, `
		INSERT INTO project_resource (
			project_id, workspace_id, resource_type, resource_ref, position
		)
		VALUES ($1, $2, 'gitlab_repo', $3::jsonb, 0)
	`, projectID, testWorkspaceID, `{"url":"https://gitlab.com/example/repo"}`); err != nil {
		t.Fatalf("seed gitlab_repo resource: %v", err)
	}

	body := map[string]any{
		"object_kind": "merge_request",
		"user": map[string]any{
			"username": "alice",
		},
		"project": map[string]any{
			"web_url":             "https://gitlab.com/example/repo",
			"path_with_namespace": "example/repo",
		},
		"object_attributes": map[string]any{
			"iid":           333,
			"title":         "Fix " + created.Identifier,
			"description":   "",
			"state":         "merged",
			"action":        "merge",
			"source_branch": "feature/test",
			"url":           "https://gitlab.com/example/repo/-/merge_requests/333",
			"created_at":    "2026-05-01T00:00:00Z",
			"updated_at":    "2026-05-02T00:00:00Z",
			"merged_at":     "2026-05-02T00:00:00Z",
		},
	}
	raw, _ := json.Marshal(body)
	hookReq := httptest.NewRequest("POST", "/api/webhooks/gitlab", bytes.NewReader(raw))
	hookReq.Header.Set("X-Gitlab-Event", "Merge Request Hook")
	hookReq.Header.Set("X-Gitlab-Token", "gl-secret")
	rec := httptest.NewRecorder()
	testHandler.HandleGitLabWebhook(rec, hookReq)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("webhook: expected 202, got %d (%s)", rec.Code, rec.Body.String())
	}

	var mrID string
	if err := testPool.QueryRow(ctx, `
		SELECT id
		FROM gitlab_merge_request
		WHERE workspace_id = $1 AND project_web_url = $2 AND mr_iid = $3
	`, testWorkspaceID, "https://gitlab.com/example/repo", 333).Scan(&mrID); err != nil {
		t.Fatalf("query gitlab_merge_request: %v", err)
	}

	var linkedIssueID string
	if err := testPool.QueryRow(ctx, `
		SELECT issue_id
		FROM issue_gitlab_merge_request
		WHERE gitlab_mr_id = $1
	`, mrID).Scan(&linkedIssueID); err != nil {
		t.Fatalf("query issue_gitlab_merge_request: %v", err)
	}
	if linkedIssueID != created.ID {
		t.Fatalf("linked issue_id = %s, want %s", linkedIssueID, created.ID)
	}
}
