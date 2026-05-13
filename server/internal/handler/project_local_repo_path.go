package handler

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

type ProjectLocalRepoPathResponse struct {
	ID          string  `json:"id"`
	WorkspaceID string  `json:"workspace_id"`
	ProjectID   string  `json:"project_id"`
	DaemonID    string  `json:"daemon_id"`
	LocalPath   string  `json:"local_path"`
	BranchHint  *string `json:"branch_hint"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

type UpsertProjectLocalRepoPathRequest struct {
	DaemonID   string  `json:"daemon_id"`
	LocalPath  string  `json:"local_path"`
	BranchHint *string `json:"branch_hint"`
}

func projectLocalRepoPathToResponse(row db.ProjectLocalRepoPath) ProjectLocalRepoPathResponse {
	return ProjectLocalRepoPathResponse{
		ID:          uuidToString(row.ID),
		WorkspaceID: uuidToString(row.WorkspaceID),
		ProjectID:   uuidToString(row.ProjectID),
		DaemonID:    row.DaemonID,
		LocalPath:   row.LocalPath,
		BranchHint:  textToPtr(row.BranchHint),
		CreatedAt:   timestampToString(row.CreatedAt),
		UpdatedAt:   timestampToString(row.UpdatedAt),
	}
}

func validateLocalRepoPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errBadRequest("local_path is required")
	}
	if !filepath.IsAbs(path) {
		return "", errBadRequest("local_path must be an absolute path")
	}
	cleaned := filepath.Clean(path)
	if cleaned == "." || cleaned == string(filepath.Separator) {
		return "", errBadRequest("local_path is invalid")
	}
	if !isPathAllowedByEnv(cleaned) {
		return "", errBadRequest("local_path is outside allowed roots")
	}
	return cleaned, nil
}

func isPathAllowedByEnv(path string) bool {
	raw := strings.TrimSpace(os.Getenv("LOCAL_PROJECT_PATH_ALLOWED_ROOTS"))
	// Unset policy means no root restriction in this environment.
	if raw == "" {
		return true
	}

	cleanPath := filepath.Clean(path)
	for _, part := range strings.Split(raw, ",") {
		root := strings.TrimSpace(part)
		if root == "" {
			continue
		}
		root = filepath.Clean(root)
		if cleanPath == root || strings.HasPrefix(cleanPath, root+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func errBadRequest(msg string) error { return badRequestError(msg) }

type badRequestError string

func (e badRequestError) Error() string { return string(e) }

func (h *Handler) requireProjectLocalRepoPathWriteRole(w http.ResponseWriter, r *http.Request, workspaceID string) bool {
	member, ok := h.requireWorkspaceMember(w, r, workspaceID, "workspace not found")
	if !ok {
		return false
	}
	if member.Role != "owner" && member.Role != "admin" {
		writeError(w, http.StatusForbidden, "forbidden")
		return false
	}
	return true
}

func (h *Handler) ListProjectLocalRepoPaths(w http.ResponseWriter, r *http.Request) {
	project, ok := h.loadProjectForResource(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	rows, err := h.Queries.ListProjectLocalRepoPathsByProject(r.Context(), project.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list project local repo paths")
		return
	}
	resp := make([]ProjectLocalRepoPathResponse, len(rows))
	for i, row := range rows {
		resp[i] = projectLocalRepoPathToResponse(row)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": resp, "total": len(resp)})
}

func (h *Handler) UpsertProjectLocalRepoPath(w http.ResponseWriter, r *http.Request) {
	project, ok := h.loadProjectForResource(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	if !h.requireProjectLocalRepoPathWriteRole(w, r, uuidToString(project.WorkspaceID)) {
		return
	}

	var req UpsertProjectLocalRepoPathRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.DaemonID = strings.TrimSpace(req.DaemonID)
	if req.DaemonID == "" {
		writeError(w, http.StatusBadRequest, "daemon_id is required")
		return
	}
	localPath, err := validateLocalRepoPath(req.LocalPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	branchHint := pgtype.Text{}
	if req.BranchHint != nil && strings.TrimSpace(*req.BranchHint) != "" {
		branchHint = strToText(strings.TrimSpace(*req.BranchHint))
	}

	row, err := h.Queries.UpsertProjectLocalRepoPath(r.Context(), db.UpsertProjectLocalRepoPathParams{
		WorkspaceID: project.WorkspaceID,
		ProjectID:   project.ID,
		DaemonID:    req.DaemonID,
		LocalPath:   localPath,
		BranchHint:  branchHint,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to upsert project local repo path")
		return
	}
	writeJSON(w, http.StatusOK, projectLocalRepoPathToResponse(row))
}

func (h *Handler) DeleteProjectLocalRepoPath(w http.ResponseWriter, r *http.Request) {
	project, ok := h.loadProjectForResource(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	if !h.requireProjectLocalRepoPathWriteRole(w, r, uuidToString(project.WorkspaceID)) {
		return
	}
	daemonID := strings.TrimSpace(chi.URLParam(r, "daemonId"))
	if daemonID == "" {
		writeError(w, http.StatusBadRequest, "daemon id is required")
		return
	}

	_, err := h.Queries.GetProjectLocalRepoPathByProjectAndDaemon(r.Context(), db.GetProjectLocalRepoPathByProjectAndDaemonParams{
		WorkspaceID: project.WorkspaceID,
		ProjectID:   project.ID,
		DaemonID:    daemonID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "project local repo path not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load project local repo path")
		return
	}

	if err := h.Queries.DeleteProjectLocalRepoPath(r.Context(), db.DeleteProjectLocalRepoPathParams{
		WorkspaceID: project.WorkspaceID,
		ProjectID:   project.ID,
		DaemonID:    daemonID,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete project local repo path")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
