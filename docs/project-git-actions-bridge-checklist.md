# Project Git Actions - Bridge Checklist (Detailed)

## 0. Objective
- [x] Deliver Phase 1 Git actions safely on top of existing local-path + live-status foundation.
- [x] Keep execution authoritative at backend; UI only triggers typed actions.
- [x] Preserve current behavior for task snapshots and live status.

## 1. Current Baseline (Already Implemented)
- [x] Project local path mapping exists per daemon (`project_local_repo_path`).
- [x] Task snapshot changes exist (`/api/projects/{id}/task-changes`).
- [x] Live repo status endpoint exists (`/api/projects/{id}/live-git-status`).
- [x] Project detail UI shows:
  - [x] Task snapshot section.
  - [x] Live repo status section.

## 2. Phase 1 Target Scope (This rollout)
- [x] Read actions:
  - [x] `status` (branch, dirty/clean, staged/unstaged/untracked counts, diff stat).
  - [x] `branch_list` + current branch.
- [x] Low-risk actions:
  - [x] `fetch`.
  - [x] `pull_ff_only`.
  - [x] `checkout_existing_branch`.
- [x] Out of scope (Phase 2+):
  - [x] `add`, `commit`, `restore --staged`, `checkout -b`, `stash`, `merge/rebase`.

## 3. Backend Architecture

### 3.1 Git Action Service
- [x] Add centralized service (single entry) for Git operations.
- [x] Define typed action enum:
  - [x] `status`
  - [x] `branch_list`
  - [x] `fetch`
  - [x] `pull_ff_only`
  - [x] `checkout_existing_branch`
- [x] Map action -> fixed argv (no shell).
- [x] Return structured result:
  - [x] `ok` boolean
  - [x] `action`
  - [x] `execution_workdir`
  - [x] `stdout`
  - [x] `stderr`
  - [x] `exit_code`
  - [x] optional parsed payload (status/branches)

### 3.2 Path Resolution
- [x] Resolve project -> local path from `project_local_repo_path`.
- [x] Selection rule: latest `updated_at` mapping.
- [x] Validate absolute normalized path and existence.
- [x] Validate git repo (`rev-parse --is-inside-work-tree`) for all actions.

### 3.3 Input Validation
- [x] Branch input validation for checkout:
  - [x] allowed charset and length.
  - [x] reject traversal/special cases.
- [x] Ensure checkout only accepts existing branches from `branch --list` output.

### 3.4 Timeout + Cancellation
- [x] Add per-action timeout (configurable, e.g. 5s read, 20s network actions).
- [x] Wire request context cancellation down to command exec.

### 3.5 Allowlist + Guardrails
- [x] Hard allowlist at backend action dispatcher.
- [x] No generic command endpoint.
- [x] Reject unknown actions with 400.

### 3.6 Locking / Concurrency
- [x] Add per-project operation lock (in-memory mutex map for MVP).
- [x] Reject/queue concurrent operations on same project.
- [x] Return clear response when lock is held.

### 3.7 Audit & Logs
- [x] Structured logs for each action:
  - [x] user_id, workspace_id, project_id, action, duration, result.
- [x] Include summarized stderr category.

## 4. Backend API Surface
- [x] Add endpoint: `POST /api/projects/{id}/git-actions`.
- [x] Request schema:
  - [x] `action` (enum)
  - [x] `branch` (required for checkout only)
- [x] Response schema:
  - [x] common execution envelope + parsed payload per action.
- [x] Keep `GET /api/projects/{id}/live-git-status` for read-only dashboard refresh.

## 5. Error Model (User-Friendly)
- [x] Normalize error categories:
  - [x] `not_git_repo`
  - [x] `missing_local_path`
  - [x] `lock_busy`
  - [x] `dirty_tree_overwrite_risk`
  - [x] `ff_only_failed`
  - [x] `remote_auth_failed`
  - [x] `branch_not_found`
  - [x] `timeout`
  - [x] `internal_error`
- [x] Preserve raw stderr in details field for debugging.

## 6. Frontend UX (Phase 1)

### 6.1 Git Actions Panel
- [x] Add “Git Actions” block in project detail (near Live Repo Status).
- [x] Render:
  - [x] current branch
  - [x] clean/dirty indicator
  - [x] counts summary (staged/unstaged/untracked)
  - [x] last refreshed timestamp

### 6.2 Action Controls
- [x] Buttons:
  - [x] Refresh status
  - [x] Fetch
  - [x] Pull (ff-only)
- [x] Branch checkout control:
  - [x] branch dropdown from `branch_list`
  - [x] checkout button
- [x] Loading/disabled state while operation running.
- [x] Confirm modal for `pull` and `checkout`.

### 6.3 Result Feedback
- [x] Success toast per action.
- [x] Error toast mapped from backend error category.
- [x] Expandable “details” section for raw stderr/stdout.

## 7. Security & Policy
- [x] Ensure only workspace members with project access can run actions.
- [x] Optional stricter role gate (owner/admin) behind config flag.
- [x] Never expose arbitrary filesystem paths outside selected project mapping.

## 8. Testing Plan

### 8.1 Unit Tests (Backend)
- [x] Action dispatcher allowlist mapping.
- [x] Input validation (branch).
- [x] Error categorization mapper.
- [x] Per-project lock behavior.

### 8.2 Integration Tests (Backend Handler)
- [x] Setup temp git repo + mapping.
- [x] `status` returns expected parsed fields.
- [x] `branch_list` returns current + branches.
- [x] `fetch` success path.
- [x] `pull_ff_only` success/failure path.
- [x] `checkout_existing_branch` success + branch_not_found failure.
- [x] non-git path -> `not_git_repo`.

### 8.3 Frontend Tests
- [x] Panel renders status summary.
- [x] Buttons disabled during running action.
- [x] Error/success feedback rendering.

## 9. Rollout
- [x] Feature flag: `PROJECT_GIT_ACTIONS_PHASE1`.
- [x] Enable for dev workspace first.
- [x] Observe logs/error categories for 1-2 days. (runbook + log filters documented in `docs/project-git-actions-rollout-runbook.md`)
- [x] Expand to wider cohort. (gating criteria + rollback documented in `docs/project-git-actions-rollout-runbook.md`)

## 10. Acceptance Criteria (Phase 1)
- [x] All Git actions execute only in resolved project local path.
- [x] Unknown/non-allowlisted commands cannot run.
- [x] User can complete flow:
  - [x] refresh status
  - [x] fetch
  - [x] checkout existing branch
  - [x] pull ff-only
- [x] Clear, actionable errors for common failures.
- [x] Tests + typechecks pass.
