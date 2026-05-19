# Per-Project Git Actions: Implementation Checklist

## Goal

Provide Git action buttons per project/repository (for example: pull, checkout branch, add, commit) with safe guardrails and predictable UX.

## Scope

- [ ] Each project card/view can run Git actions in that project's repository root.
- [ ] Actions are executed by backend service, not directly from frontend.
- [ ] Initial rollout includes only a safe subset of commands.

## Phase Plan

## Phase 1 (Read + Low Risk Actions)

- [ ] `git status` summary (branch, dirty state, staged/unstaged counts).
- [ ] `git branch --list` and current branch.
- [ ] `git fetch`.
- [ ] `git pull --ff-only`.
- [ ] `git checkout <existing-branch>`.

## Phase 2 (Controlled Write Actions)

- [ ] `git add <selected-files>` (no wildcard expansion by shell).
- [ ] `git commit -m "<message>"`.
- [ ] Optional `git restore --staged <file>` for unstage.

## Phase 3 (Advanced Actions - Optional Later)

- [ ] `git checkout -b <new-branch>`.
- [ ] `git stash` / `git stash pop`.
- [ ] `git merge` / `git rebase` (only after conflict UX is ready).

## Backend Command Execution (authoritative)

- [ ] Build centralized Git execution service (single entry point).
- [ ] Map project ID -> validated local repo path.
- [ ] Verify target path is a Git repo before running any action.
- [ ] Run commands with explicit argv (no shell interpolation).
- [ ] Enforce command allowlist by action key.
- [ ] Validate/sanitize all dynamic inputs:
  - [ ] Branch names.
  - [ ] File paths for add/unstage.
  - [ ] Commit message length/content policy.
- [ ] Add per-command timeout and cancellation support.
- [ ] Capture stdout/stderr and structured exit codes.
- [ ] Add audit logs (user, project, action, timestamp, result).

## Safety Guardrails

- [ ] Block dangerous commands in UI and backend:
  - [ ] `git reset --hard`
  - [ ] `git clean -fd`
  - [ ] force push and other destructive ops in initial versions
- [ ] Require confirmation dialog for write actions (`pull`, `checkout`, `add`, `commit`).
- [ ] Detect dirty working tree and warn/block where action can cause loss.
- [ ] For checkout, warn when uncommitted changes would be overwritten.
- [ ] For pull, prefer `--ff-only` to avoid silent merge commits.

## UI/UX

- [ ] Add Git status panel per project:
  - [ ] Current branch.
  - [ ] Dirty/clean indicator.
  - [ ] Ahead/behind indicator if available.
- [ ] Add action buttons with loading/disabled states.
- [ ] Add branch picker for checkout (existing branches first).
- [ ] Add staged files selector for `git add`.
- [ ] Add commit modal with:
  - [ ] Required commit message.
  - [ ] Preview of staged files.
  - [ ] Clear success/error feedback.
- [ ] Preserve last refresh timestamp and "refresh status" action.

## Concurrency and Locking

- [ ] Prevent concurrent conflicting Git operations per project.
- [ ] Add per-project operation lock queue.
- [ ] Show "operation in progress" state in UI.

## Error Handling

- [ ] Translate common Git errors into user-readable messages:
  - [ ] Merge conflict required.
  - [ ] Local changes would be overwritten.
  - [ ] Not a git repository.
  - [ ] Remote/auth failures.
- [ ] Keep raw stderr available in expandable details section.

## Testing

- [ ] Unit tests:
  - [ ] Action-to-command mapping.
  - [ ] Input validation and allowlist enforcement.
  - [ ] Locking behavior.
- [ ] Integration tests with temp repos:
  - [ ] Pull fast-forward success/failure.
  - [ ] Checkout branch with/without dirty tree.
  - [ ] Add/commit happy path and validation failures.
- [ ] E2E tests:
  - [ ] User can inspect status and switch branch.
  - [ ] Commit flow from select files -> message -> success.
  - [ ] Error surfaces correctly for conflict/dirty scenarios.

## Observability

- [ ] Add metrics:
  - [ ] Action success/failure rate by command.
  - [ ] Median execution time.
  - [ ] Most frequent error categories.
- [ ] Add dashboard and alert for abnormal failure spikes.

## Rollout

- [ ] Feature flag by workspace/user cohort.
- [ ] Start with Phase 1 only.
- [ ] Gradually enable Phase 2 after stability metrics.
- [ ] Publish usage docs and known limitations.

## Acceptance Criteria

- [ ] Git actions execute only inside the selected project's repository.
- [ ] Only allowlisted commands can run.
- [ ] Destructive commands are blocked.
- [ ] Users can complete core flow: status -> checkout -> add -> commit -> pull.
- [ ] Test suite covers happy path plus conflict/validation edge cases.
