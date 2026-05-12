package handler

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

func gitlabWebhookSecret() string {
	return strings.TrimSpace(os.Getenv("GITLAB_WEBHOOK_SECRET"))
}

func verifyGitLabWebhookToken(secret, token string) bool {
	if secret == "" || token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(secret), []byte(token)) == 1
}

type gitlabMergeRequestPayload struct {
	ObjectKind string `json:"object_kind"`
	User       struct {
		Username string `json:"username"`
		Avatar   string `json:"avatar_url"`
	} `json:"user"`
	Project struct {
		WebURL            string `json:"web_url"`
		PathWithNamespace string `json:"path_with_namespace"`
	} `json:"project"`
	ObjectAttributes struct {
		IID          int32  `json:"iid"`
		Title        string `json:"title"`
		Description  string `json:"description"`
		State        string `json:"state"`
		Action       string `json:"action"`
		SourceBranch string `json:"source_branch"`
		URL          string `json:"url"`
		CreatedAt    string `json:"created_at"`
		UpdatedAt    string `json:"updated_at"`
		MergedAt     string `json:"merged_at"`
		ClosedAt     string `json:"closed_at"`
	} `json:"object_attributes"`
}

func deriveGitLabMRState(action, state string) (string, bool) {
	a := strings.ToLower(strings.TrimSpace(action))
	s := strings.ToLower(strings.TrimSpace(state))
	switch a {
	case "open", "reopen", "update":
		if s == "merged" {
			return "merged", true
		}
		if s == "closed" {
			return "closed", true
		}
		return "open", true
	case "merge":
		return "merged", true
	default:
		return "", false
	}
}

func normalizeRepoURL(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimSuffix(raw, "/")
	return raw
}

func (h *Handler) resolveWorkspaceByGitLabRepoURL(ctx context.Context, repoURL string) (pgtype.UUID, string, bool) {
	if h.DB == nil {
		return pgtype.UUID{}, "", false
	}
	repoURL = normalizeRepoURL(repoURL)
	if repoURL == "" {
		return pgtype.UUID{}, "", false
	}

	rows, err := h.DB.Query(ctx, `
		SELECT workspace_id
		FROM project_resource
		WHERE resource_type = 'gitlab_repo'
			AND (resource_ref->>'url' = $1 OR resource_ref->>'url' = $2)
		LIMIT 2
	`, repoURL, repoURL+"/")
	if err != nil {
		slog.Warn("gitlab: failed to resolve workspace by repo url", "repo_url", repoURL, "err", err)
		return pgtype.UUID{}, "", false
	}
	defer rows.Close()

	count := 0
	var workspaceID pgtype.UUID
	for rows.Next() {
		count++
		if err := rows.Scan(&workspaceID); err != nil {
			return pgtype.UUID{}, "", false
		}
	}
	if count != 1 {
		return pgtype.UUID{}, "", false
	}
	return workspaceID, uuidToString(workspaceID), true
}

func (h *Handler) upsertGitLabMergeRequest(ctx context.Context, workspaceID pgtype.UUID, p gitlabMergeRequestPayload, state string) (pgtype.UUID, error) {
	mr, err := h.Queries.UpsertGitLabMergeRequest(ctx, db.UpsertGitLabMergeRequestParams{
		WorkspaceID:     workspaceID,
		ProjectWebUrl:   normalizeRepoURL(p.Project.WebURL),
		ProjectPath:     strings.TrimSpace(p.Project.PathWithNamespace),
		MrIid:           p.ObjectAttributes.IID,
		Title:           p.ObjectAttributes.Title,
		Description:     p.ObjectAttributes.Description,
		State:           state,
		HtmlUrl:         p.ObjectAttributes.URL,
		MrCreatedAt:     parseGHTimeRequired(p.ObjectAttributes.CreatedAt),
		MrUpdatedAt:     parseGHTimeRequired(p.ObjectAttributes.UpdatedAt),
		SourceBranch:    strToText(strings.TrimSpace(p.ObjectAttributes.SourceBranch)),
		AuthorUsername:  strToText(strings.TrimSpace(p.User.Username)),
		AuthorAvatarUrl: strToText(strings.TrimSpace(p.User.Avatar)),
		MergedAt:        parseGHTime(p.ObjectAttributes.MergedAt),
		ClosedAt:        parseGHTime(p.ObjectAttributes.ClosedAt),
	})
	if err != nil {
		return pgtype.UUID{}, err
	}
	return mr.ID, nil
}

// HandleGitLabWebhook (POST /api/webhooks/gitlab) ingests merge_request
// webhook events. For MVP we normalize open/update/merge to pull_request
// updates and publish them only when a single workspace can be inferred from
// a matching gitlab_repo project_resource URL.
func (h *Handler) HandleGitLabWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "read body failed")
		return
	}

	secret := gitlabWebhookSecret()
	if secret == "" {
		writeError(w, http.StatusServiceUnavailable, "gitlab webhooks not configured")
		return
	}
	if !verifyGitLabWebhookToken(secret, r.Header.Get("X-Gitlab-Token")) {
		writeError(w, http.StatusUnauthorized, "invalid token")
		return
	}

	if strings.ToLower(strings.TrimSpace(r.Header.Get("X-Gitlab-Event"))) != "merge request hook" {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	var p gitlabMergeRequestPayload
	if err := json.Unmarshal(body, &p); err != nil {
		slog.Warn("gitlab: bad merge request payload", "err", err)
		w.WriteHeader(http.StatusAccepted)
		return
	}
	state, ok := deriveGitLabMRState(p.ObjectAttributes.Action, p.ObjectAttributes.State)
	if !ok {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	workspaceUUID, workspaceID, resolved := h.resolveWorkspaceByGitLabRepoURL(r.Context(), p.Project.WebURL)
	if !resolved {
		// Acknowledge to avoid webhook retries; missing mapping is expected
		// before a workspace has attached a gitlab_repo resource.
		w.WriteHeader(http.StatusAccepted)
		return
	}
	mrID, err := h.upsertGitLabMergeRequest(r.Context(), workspaceUUID, p, state)
	if err != nil {
		slog.Warn("gitlab: upsert merge request failed", "err", err)
		w.WriteHeader(http.StatusAccepted)
		return
	}

	idents := extractIdentifiers(p.ObjectAttributes.Title, p.ObjectAttributes.Description, p.ObjectAttributes.SourceBranch)
	prefix := h.getIssuePrefix(r.Context(), workspaceUUID)
	linkedIssueIDs := make([]string, 0, len(idents))
	for _, id := range idents {
		issue, ok := h.lookupIssueByIdentifier(r.Context(), workspaceUUID, prefix, id)
		if !ok {
			continue
		}
		if err := h.Queries.LinkIssueToGitLabMergeRequest(r.Context(), db.LinkIssueToGitLabMergeRequestParams{
			IssueID:      issue.ID,
			GitlabMrID:   mrID,
			LinkedByType: strToText("system"),
		}); err != nil {
			slog.Warn("gitlab: link issue failed", "err", err)
			continue
		}
		linkedIssueIDs = append(linkedIssueIDs, uuidToString(issue.ID))
		if state == "merged" && issue.Status != "done" && issue.Status != "cancelled" {
			h.advanceIssueToDone(r.Context(), issue, workspaceID)
		}
	}

	h.publish(protocol.EventPullRequestUpdated, workspaceID, "system", "", map[string]any{
		"provider": "gitlab",
		"pull_request": map[string]any{
			"number":            p.ObjectAttributes.IID,
			"title":             p.ObjectAttributes.Title,
			"state":             state,
			"html_url":          p.ObjectAttributes.URL,
			"branch":            p.ObjectAttributes.SourceBranch,
			"author_login":      p.User.Username,
			"author_avatar_url": p.User.Avatar,
			"repo_path":         p.Project.PathWithNamespace,
			"repo_web_url":      p.Project.WebURL,
			"pr_created_at":     p.ObjectAttributes.CreatedAt,
			"pr_updated_at":     p.ObjectAttributes.UpdatedAt,
			"merged_at":         p.ObjectAttributes.MergedAt,
			"closed_at":         p.ObjectAttributes.ClosedAt,
		},
		"linked_issue_ids": linkedIssueIDs,
	})
	w.WriteHeader(http.StatusAccepted)
}
