package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type projectGitActionRequest struct {
	Action string `json:"action"`
	Branch string `json:"branch,omitempty"`
}

type gitBranchListPayload struct {
	Current  string   `json:"current"`
	Branches []string `json:"branches"`
}

type gitStatusPayload struct {
	Branch         string `json:"branch"`
	Head           string `json:"head"`
	Dirty          bool   `json:"dirty"`
	StagedCount    int    `json:"staged_count"`
	UnstagedCount  int    `json:"unstaged_count"`
	UntrackedCount int    `json:"untracked_count"`
	FilesChanged   int    `json:"files_changed"`
	Insertions     int    `json:"insertions"`
	Deletions      int    `json:"deletions"`
}

type projectGitActionResponse struct {
	OK               bool      `json:"ok"`
	Action           string    `json:"action"`
	ProjectID        string    `json:"project_id"`
	ExecutionWorkDir string    `json:"execution_workdir"`
	CollectStatus    string    `json:"collect_status"` // ok | missing_local_path | not_git_repo | lock_busy | timeout | validation_error | internal_error
	ErrorCode        string    `json:"error_code,omitempty"`
	ErrorMessage     string    `json:"error_message,omitempty"`
	Stdout           string    `json:"stdout,omitempty"`
	Stderr           string    `json:"stderr,omitempty"`
	ExitCode         int       `json:"exit_code"`
	ExecutedAt       time.Time `json:"executed_at"`
	Status           any       `json:"status,omitempty"`
	Branches         any       `json:"branches,omitempty"`
}

var (
	projectGitActionFeatureFlag = "PROJECT_GIT_ACTIONS_PHASE1"
	projectGitActionAdminOnly   = "PROJECT_GIT_ACTIONS_REQUIRE_ADMIN"
	branchNameRegex             = regexp.MustCompile(`^[A-Za-z0-9._/\-]{1,255}$`)
	projectGitLocks             = newProjectGitLockManager()
)

type projectGitLockManager struct {
	mu   sync.Mutex
	held map[string]bool
}

func newProjectGitLockManager() *projectGitLockManager {
	return &projectGitLockManager{held: map[string]bool{}}
}

func (m *projectGitLockManager) TryAcquire(projectID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.held[projectID] {
		return false
	}
	m.held[projectID] = true
	return true
}

func (m *projectGitLockManager) Release(projectID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.held, projectID)
}

func (h *Handler) ProjectGitAction(w http.ResponseWriter, r *http.Request) {
	if strings.ToLower(strings.TrimSpace(os.Getenv(projectGitActionFeatureFlag))) != "true" {
		writeError(w, http.StatusNotFound, "project git actions are disabled")
		return
	}
	project, ok := h.loadProjectForResource(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}

	var req projectGitActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Action = strings.TrimSpace(req.Action)
	req.Branch = strings.TrimSpace(req.Branch)
	if req.Action == "" {
		writeError(w, http.StatusBadRequest, "action is required")
		return
	}
	if !isAllowedProjectGitAction(req.Action) {
		writeError(w, http.StatusBadRequest, "unsupported action")
		return
	}
	if req.Action == "checkout_existing_branch" && req.Branch == "" {
		writeError(w, http.StatusBadRequest, "branch is required for checkout_existing_branch")
		return
	}
	if strings.ToLower(strings.TrimSpace(os.Getenv(projectGitActionAdminOnly))) == "true" {
		if !h.requireProjectLocalRepoPathWriteRole(w, r, uuidToString(project.WorkspaceID)) {
			return
		}
	}

	projectID := uuidToString(project.ID)
	startedAt := time.Now()
	if !projectGitLocks.TryAcquire(projectID) {
		writeJSON(w, http.StatusConflict, projectGitActionResponse{
			OK:            false,
			Action:        req.Action,
			ProjectID:     projectID,
			CollectStatus: "lock_busy",
			ErrorCode:     "lock_busy",
			ErrorMessage:  "another git operation is already running for this project",
			ExecutedAt:    time.Now().UTC(),
			ExitCode:      -1,
		})
		return
	}
	defer projectGitLocks.Release(projectID)

	workDir, err := h.resolveLatestProjectLocalPath(r.Context(), project.ID)
	if err != nil {
		writeJSON(w, http.StatusOK, projectGitActionResponse{
			OK:            false,
			Action:        req.Action,
			ProjectID:     projectID,
			CollectStatus: "missing_local_path",
			ErrorCode:     "missing_local_path",
			ErrorMessage:  "no local path configured for this project",
			ExecutedAt:    time.Now().UTC(),
			ExitCode:      -1,
		})
		return
	}

	resp := h.executeProjectGitAction(r.Context(), projectID, workDir, req)
	if resp.OK {
		writeJSON(w, http.StatusOK, resp)
	} else {
		writeJSON(w, http.StatusOK, resp)
	}
	slog.Info("project git action executed",
		"user_id", userID,
		"workspace_id", uuidToString(project.WorkspaceID),
		"project_id", projectID,
		"action", req.Action,
		"ok", resp.OK,
		"collect_status", resp.CollectStatus,
		"error_code", resp.ErrorCode,
		"stderr_category", categorizeGitStderr(resp.Stderr),
		"duration_ms", time.Since(startedAt).Milliseconds(),
	)
}

func (h *Handler) resolveLatestProjectLocalPath(ctx context.Context, projectID pgtype.UUID) (string, error) {
	rows, err := h.Queries.ListProjectLocalRepoPathsByProject(ctx, projectID)
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", errors.New("missing local path")
	}
	selected := rows[0]
	for _, row := range rows[1:] {
		if row.UpdatedAt.Time.After(selected.UpdatedAt.Time) {
			selected = row
		}
	}
	path := filepath.Clean(strings.TrimSpace(selected.LocalPath))
	if path == "" || path == "." {
		return "", errors.New("missing local path")
	}
	if !filepath.IsAbs(path) {
		return "", errors.New("local path must be absolute")
	}
	if st, err := os.Stat(path); err != nil || !st.IsDir() {
		return "", errors.New("local path does not exist")
	}
	return path, nil
}

func (h *Handler) executeProjectGitAction(ctx context.Context, projectID, workDir string, req projectGitActionRequest) projectGitActionResponse {
	base := projectGitActionResponse{
		OK:               false,
		Action:           req.Action,
		ProjectID:        projectID,
		ExecutionWorkDir: workDir,
		ExecutedAt:       time.Now().UTC(),
		ExitCode:         -1,
	}
	workDir = filepath.Clean(strings.TrimSpace(workDir))
	if workDir == "." || workDir == "" {
		base.CollectStatus = "missing_local_path"
		base.ErrorCode = "missing_local_path"
		base.ErrorMessage = "missing local path"
		return base
	}
	if _, stderr, code, err := runGitWithTimeout(ctx, workDir, 5*time.Second, "rev-parse", "--is-inside-work-tree"); err != nil {
		base.CollectStatus = "not_git_repo"
		base.ErrorCode = "not_git_repo"
		base.ErrorMessage = "configured path is not a git repository"
		base.Stderr = stderr
		base.ExitCode = code
		return base
	}

	switch req.Action {
	case "status":
		status, stdout, stderr, code, err := collectProjectGitStatus(ctx, workDir)
		if err != nil {
			base.CollectStatus = classifyGitActionStatus(err, stderr)
			base.ErrorCode = base.CollectStatus
			base.ErrorMessage = "failed to collect git status"
			base.Stdout, base.Stderr, base.ExitCode = stdout, stderr, code
			return base
		}
		base.OK = true
		base.CollectStatus = "ok"
		base.Status = status
		base.Stdout, base.Stderr, base.ExitCode = stdout, stderr, code
		return base
	case "branch_list":
		branches, stdout, stderr, code, err := collectProjectBranchList(ctx, workDir)
		if err != nil {
			base.CollectStatus = classifyGitActionStatus(err, stderr)
			base.ErrorCode = base.CollectStatus
			base.ErrorMessage = "failed to list branches"
			base.Stdout, base.Stderr, base.ExitCode = stdout, stderr, code
			return base
		}
		base.OK = true
		base.CollectStatus = "ok"
		base.Branches = branches
		base.Stdout, base.Stderr, base.ExitCode = stdout, stderr, code
		return base
	case "fetch":
		stdout, stderr, code, err := runGitWithTimeout(ctx, workDir, 20*time.Second, "fetch")
		if err != nil {
			base.CollectStatus = classifyGitActionStatus(err, stderr)
			base.ErrorCode = base.CollectStatus
			base.ErrorMessage = "git fetch failed"
			base.Stdout, base.Stderr, base.ExitCode = stdout, stderr, code
			return base
		}
		base.OK = true
		base.CollectStatus = "ok"
		base.Stdout, base.Stderr, base.ExitCode = stdout, stderr, code
		return base
	case "pull_ff_only":
		stdout, stderr, code, err := runGitWithTimeout(ctx, workDir, 30*time.Second, "pull", "--ff-only")
		if err != nil {
			base.CollectStatus = classifyGitActionStatus(err, stderr)
			base.ErrorCode = "ff_only_failed"
			base.ErrorMessage = "git pull --ff-only failed"
			base.Stdout, base.Stderr, base.ExitCode = stdout, stderr, code
			return base
		}
		base.OK = true
		base.CollectStatus = "ok"
		base.Stdout, base.Stderr, base.ExitCode = stdout, stderr, code
		return base
	case "checkout_existing_branch":
		if !branchNameRegex.MatchString(req.Branch) {
			base.CollectStatus = "validation_error"
			base.ErrorCode = "validation_error"
			base.ErrorMessage = "invalid branch name"
			return base
		}
		status, _, _, _, err := collectProjectGitStatus(ctx, workDir)
		if err != nil {
			base.CollectStatus = classifyGitActionStatus(err, "")
			base.ErrorCode = base.CollectStatus
			base.ErrorMessage = "failed to check working tree"
			return base
		}
		if status.Dirty {
			base.CollectStatus = "dirty_tree_overwrite_risk"
			base.ErrorCode = "dirty_tree_overwrite_risk"
			base.ErrorMessage = "working tree is dirty; commit/stash changes before checkout"
			return base
		}
		list, _, _, _, err := collectProjectBranchList(ctx, workDir)
		if err != nil {
			base.CollectStatus = classifyGitActionStatus(err, "")
			base.ErrorCode = base.CollectStatus
			base.ErrorMessage = "failed to list branches"
			return base
		}
		exists := false
		for _, b := range list.Branches {
			if b == req.Branch {
				exists = true
				break
			}
		}
		if !exists {
			base.CollectStatus = "branch_not_found"
			base.ErrorCode = "branch_not_found"
			base.ErrorMessage = "branch does not exist"
			return base
		}
		stdout, stderr, code, err := runGitWithTimeout(ctx, workDir, 20*time.Second, "checkout", req.Branch)
		if err != nil {
			base.CollectStatus = classifyGitActionStatus(err, stderr)
			base.ErrorCode = base.CollectStatus
			base.ErrorMessage = "git checkout failed"
			base.Stdout, base.Stderr, base.ExitCode = stdout, stderr, code
			return base
		}
		base.OK = true
		base.CollectStatus = "ok"
		base.Stdout, base.Stderr, base.ExitCode = stdout, stderr, code
		return base
	default:
		base.CollectStatus = "validation_error"
		base.ErrorCode = "validation_error"
		base.ErrorMessage = "unsupported action"
		return base
	}
}

func runGitWithTimeout(ctx context.Context, workDir string, timeout time.Duration, args ...string) (stdout string, stderr string, code int, err error) {
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, "git", append([]string{"-C", workDir}, args...)...)
	out, runErr := cmd.CombinedOutput()
	text := string(out)
	exitCode := 0
	if runErr != nil {
		if ee, ok := runErr.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			exitCode = -1
		}
	}
	if cmdCtx.Err() == context.DeadlineExceeded {
		return "", text, exitCode, context.DeadlineExceeded
	}
	return text, text, exitCode, runErr
}

func collectProjectBranchList(ctx context.Context, workDir string) (gitBranchListPayload, string, string, int, error) {
	currentOut, currentErrOut, currentCode, err := runGitWithTimeout(ctx, workDir, 5*time.Second, "branch", "--show-current")
	if err != nil {
		return gitBranchListPayload{}, currentOut, currentErrOut, currentCode, err
	}
	listOut, listErrOut, listCode, err := runGitWithTimeout(ctx, workDir, 5*time.Second, "branch", "--list", "--format=%(refname:short)")
	if err != nil {
		return gitBranchListPayload{}, listOut, listErrOut, listCode, err
	}
	lines := strings.Split(listOut, "\n")
	branches := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			branches = append(branches, line)
		}
	}
	return gitBranchListPayload{
		Current:  strings.TrimSpace(currentOut),
		Branches: branches,
	}, currentOut + "\n" + listOut, "", 0, nil
}

func collectProjectGitStatus(ctx context.Context, workDir string) (gitStatusPayload, string, string, int, error) {
	var status gitStatusPayload
	branchOut, branchErrOut, branchCode, err := runGitWithTimeout(ctx, workDir, 5*time.Second, "branch", "--show-current")
	if err != nil {
		return status, branchOut, branchErrOut, branchCode, err
	}
	headOut, headErrOut, headCode, err := runGitWithTimeout(ctx, workDir, 5*time.Second, "rev-parse", "HEAD")
	if err != nil {
		return status, headOut, headErrOut, headCode, err
	}
	porcelainOut, porcelainErrOut, porcelainCode, err := runGitWithTimeout(ctx, workDir, 5*time.Second, "status", "--porcelain")
	if err != nil {
		return status, porcelainOut, porcelainErrOut, porcelainCode, err
	}
	shortOut, shortErrOut, shortCode, err := runGitWithTimeout(ctx, workDir, 5*time.Second, "diff", "--shortstat")
	if err != nil {
		return status, shortOut, shortErrOut, shortCode, err
	}
	status.Branch = strings.TrimSpace(branchOut)
	status.Head = strings.TrimSpace(headOut)
	lines := strings.Split(porcelainOut, "\n")
	for _, line := range lines {
		if len(line) < 3 {
			continue
		}
		x := line[0]
		y := line[1]
		if x == '?' && y == '?' {
			status.UntrackedCount++
			continue
		}
		if x != ' ' {
			status.StagedCount++
		}
		if y != ' ' {
			status.UnstagedCount++
		}
	}
	status.Dirty = status.StagedCount > 0 || status.UnstagedCount > 0 || status.UntrackedCount > 0
	stat := parseLiveShortStat(shortOut)
	status.FilesChanged = stat.FilesChanged
	status.Insertions = stat.Insertions
	status.Deletions = stat.Deletions
	return status, branchOut + "\n" + headOut + "\n" + porcelainOut + "\n" + shortOut, "", 0, nil
}

func classifyGitActionStatus(err error, stderr string) string {
	if err == nil {
		return "ok"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	lower := strings.ToLower(stderr)
	switch {
	case strings.Contains(lower, "not a git repository"):
		return "not_git_repo"
	case strings.Contains(lower, "authentication failed"),
		strings.Contains(lower, "could not read username"),
		strings.Contains(lower, "permission denied"),
		strings.Contains(lower, "repository not found"):
		return "remote_auth_failed"
	}
	return "internal_error"
}

func isAllowedProjectGitAction(action string) bool {
	switch action {
	case "status", "branch_list", "fetch", "pull_ff_only", "checkout_existing_branch":
		return true
	default:
		return false
	}
}

func categorizeGitStderr(stderr string) string {
	lower := strings.ToLower(stderr)
	switch {
	case strings.Contains(lower, "authentication failed"), strings.Contains(lower, "permission denied"):
		return "auth"
	case strings.Contains(lower, "not a git repository"):
		return "not_git_repo"
	case strings.Contains(lower, "conflict"), strings.Contains(lower, "would be overwritten"):
		return "conflict"
	default:
		return "generic"
	}
}
