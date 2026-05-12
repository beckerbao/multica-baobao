# GitLab Integration Feasibility Assessment

## Goal

Target capabilities:

1. Checkout repository, switch branch, pull latest updates
2. Receive MR notifications
3. Pull code for review, post review comments back to MR
4. Create Multica issues from code-scan findings

## Feasibility Summary

Overall: **feasible**, but not a small patch.

- Estimated readiness from current backend: **~35-45%**
- Expected effort: **medium to large feature** (integration layer work, not only route additions)

## Current Strengths (already in codebase)

1. Daemon task-claim pipeline already carries repository context to runtimes.
2. Existing GitHub integration provides reusable patterns for:
   - connect/setup callback flow
   - webhook signature verification
   - event ingestion -> DB upsert -> issue link -> realtime event publish
3. Internal issue model + creation flow already exists, so "scan -> create issue" can be added without new core issue subsystem.

## Main Gaps

### 1) Resource type is GitHub-biased

- `project_resource` validation accepts only `github_repo`.
- Daemon claim path only lifts repos when `resource_type == "github_repo"`.

Impact:
- GitLab repo cannot enter runtime repo list today without backend changes.

### 2) Integration surface is GitHub-specific

- Routes and handlers are hardcoded to `/github/*` and `/api/webhooks/github`.
- Env keys, webhook headers, payload schema assume GitHub model.

Impact:
- Adding GitLab as parallel provider requires either route duplication or provider abstraction.

### 3) Persistence model for PR state is GitHub-specific

- Tables/queries are `github_installation` and `github_pull_request`.

Impact:
- MR state tracking and auto-link/auto-close logic cannot be reused directly without schema strategy.

### 4) CLI ergonomics currently defaults to GitHub

- `project resource add` shortcut path focuses on `github_repo`.
- Generic `--ref` exists, but no first-class GitLab UX yet.

## Capability-by-Capability Assessment

1. Checkout/switch branch/pull update: **High feasibility**
- Needs provider-agnostic repo resource ingestion into daemon claim response.

2. Receive MR notifications: **High feasibility**
- Add GitLab webhook endpoint + secret verification + event parser.

3. Review and comment back to MR: **Medium-High feasibility**
- Depends on runtime/tooling support for GitLab API calls with stable auth.

4. Create issue from scan findings: **High feasibility**
- Leverage existing Multica issue creation flows.

## Recommended Architecture Direction

1. Introduce provider abstraction (`github`, `gitlab`) at integration service layer:
   - connect URL generation
   - callback validation
   - webhook verification + event normalization
   - outbound comment API

2. Make repo resources provider-aware:
   - Option A: add `gitlab_repo` alongside `github_repo`
   - Option B: unify under `git_repo` + `provider` in `resource_ref`

3. Normalize PR/MR domain model:
   - either provider-neutral table, or
   - parallel GitLab tables plus unified query layer

4. Keep issue auto-link and auto-close logic provider-neutral:
   - parse identifier from title/body/branch
   - resolve issue by workspace
   - apply same transition policy

## Suggested Delivery Phases

### Phase 1 (MVP)

Scope:
- GitLab repo resources accepted by backend
- Daemon receives GitLab repos for checkout/pull workflows
- Basic MR webhook ingestion

Outcome:
- Agents can work on GitLab code in existing task lifecycle

### Phase 2

Scope:
- MR review comment posting via GitLab API
- Stable auth/token handling
- Retry/idempotency for outbound comments

Outcome:
- Round-trip review loop: pull -> review -> comment back to MR

### Phase 3

Scope:
- Auto issue creation from scan/review findings
- Optional mapping templates, dedupe keys, severity mapping

Outcome:
- Automated quality feedback into Multica issue tracking

## Risks and Mitigations

1. Hardcoded GitHub assumptions can leak into new code
- Mitigation: enforce provider interface boundaries early.

2. Event semantic mismatch (PR vs MR actions/states)
- Mitigation: normalize to internal event model before business logic.

3. Token/permission failures for outbound MR comments
- Mitigation: explicit permission checks and durable retry with audit logs.

4. Duplicate issue creation from repeated webhook events
- Mitigation: idempotency keys and conflict-safe write patterns.

## Final Recommendation

Proceed. The target is technically achievable on this codebase.

Key constraint: treat this as an integration-platform extension, not a one-off GitLab patch.
If implemented with provider abstraction from the first commit, future providers (Bitbucket, Azure DevOps) become significantly cheaper.

---

## Implementation File Map (for agents)

This section maps each capability to exact backend files and expected changes.

### A) Integration HTTP surface (connect/callback/webhook)

Current GitHub implementation:

- `server/cmd/server/router.go`
  - Public routes:
    - `POST /api/webhooks/github`
    - `GET /api/github/setup`
  - Workspace admin routes:
    - `GET /api/workspaces/{id}/github/connect`
    - `GET /api/workspaces/{id}/github/installations`
    - `DELETE /api/workspaces/{id}/github/installations/{installationId}`
- `server/internal/handler/github.go`
  - connect URL generation (`GitHubConnect`)
  - setup callback (`GitHubSetupCallback`)
  - webhook verify + dispatch (`HandleGitHubWebhook`)
  - installation event handling (`handleInstallationEvent`)
  - PR event handling (`handlePullRequestEvent`)

GitLab work:

1. Add equivalent router entries for GitLab.
2. Add a new handler file (recommended: `server/internal/handler/gitlab.go`) with:
   - connect
   - callback
   - webhook verify
   - MR event parse + normalize
3. Keep handler-specific env names separate (`GITLAB_*`) and avoid reusing GitHub env keys.

### B) Repo resource acceptance (project resources)

Current implementation:

- `server/internal/handler/project_resource.go`
  - `validateAndNormalizeResourceRef()` only accepts `github_repo`
  - `validateGithubRepoRef()`
- `server/internal/handler/project.go`
  - project create path pre-validates resource payloads using the same validator

GitLab work:

1. Extend validator to accept GitLab repo type.
   - Option 1: `gitlab_repo` (minimal invasive)
   - Option 2: `git_repo` + `provider` field (cleaner long-term)
2. Add `validateGitlabRepoRef()` (or provider-aware unified validator).
3. Ensure both endpoints are covered:
   - `POST /api/projects/{id}/resources`
   - `POST /api/projects` with embedded `resources[]`

### C) Daemon repo injection for task execution

Current implementation:

- `server/internal/handler/daemon.go`
  - two hardcoded checks: `resource_type == "github_repo"`
  - converts project resource ref `{url}` into `resp.Repos`

GitLab work:

1. Replace GitHub-only checks with provider-aware repo extraction.
2. Update both code paths:
   - issue task path
   - quick-create task path
3. Keep fallback behavior unchanged:
   - if project repos exist -> use them
   - else fallback to workspace repos

### D) PR/MR persistence and linking behavior

Current implementation:

- `server/pkg/db/queries/github.sql`
  - installation CRUD (`github_installation`)
  - PR upsert/list (`github_pull_request`)
  - issue link table usage (`issue_pull_request`)
- `server/internal/handler/github.go`
  - identifier extraction: `extractIdentifiers(...)`
  - issue lookup: `lookupIssueByIdentifier(...)`
  - issue auto-done transition: `advanceIssueToDone(...)`
- `server/pkg/protocol/events.go`
  - `EventPullRequestUpdated`, `EventPullRequestLinked`, ...

GitLab work options:

1. Minimal path:
   - Add parallel GitLab tables + queries
   - Reuse `issue_pull_request` linkage pattern
2. Preferred long-term:
   - Introduce provider-neutral PR/MR table (`provider`, `external_id`, `repo`, `state`, ...)
   - Keep unified query methods for UI/task logic

### E) CLI project resource UX

Current implementation:

- `server/cmd/multica/cmd_project.go`
  - `project resource add` shortcut defaults to `github_repo`
  - generic `--ref` supports custom payload, but no first-class GitLab shortcut

GitLab work:

1. Add built-in shortcut for GitLab repo resource.
2. Update help text examples and validation errors.
3. Keep `--ref` path as provider-agnostic escape hatch.

### F) Realtime event contracts

Current implementation:

- `server/pkg/protocol/events.go`
  - GitHub installation events and pull request events

GitLab work:

1. Either add GitLab-specific installation events, or
2. Introduce provider-neutral integration events and include provider in payload.

Recommendation:

- Keep PR/MR update events provider-neutral early to avoid frontend event explosion.

---

## Suggested Commit Sequence

Use this order to reduce merge risk and keep each PR testable.

1. **Resource layer first**
   - Extend `project_resource` validation for GitLab
   - Add tests for create/list/delete with new type
2. **Daemon repo extraction**
   - Make claim response provider-aware
   - Ensure task payload still backward-compatible
3. **GitLab webhook ingestion (read + persist)**
   - Add routes + handler + DB writes
4. **Link MR to issue + status transitions**
   - Reuse identifier logic and done-transition semantics
5. **Outbound MR comments**
   - Add API client + auth + retries
6. **CLI UX polish**
   - Add shortcuts/docs/examples

---

## Test Matrix (minimum)

### Project resource tests

- create `gitlab_repo` succeeds
- invalid URL rejected with 400
- duplicate attach returns 409
- embedded resources in `CreateProject` validate correctly

### Daemon claim tests

- project with only GitLab repo resources -> `resp.Repos` contains GitLab URLs
- project has resources -> workspace fallback not used
- no project resources -> workspace fallback preserved

### Webhook tests

- invalid signature -> 401/unauthorized
- unknown event -> accepted but ignored
- MR opened/updated/merged -> upsert state correctly
- unlink/uninstall events clean mapping correctly

### Issue linking tests

- identifier in MR title/body/branch links to correct issue in workspace
- wrong workspace prefix does not link
- terminal MR transitions issue to done only when rule conditions are satisfied

### Outbound comment tests

- comment API success path
- permission/token failure path
- retry/idempotency behavior on transient failures

---

## Acceptance Criteria (Definition of Done)

1. Agent can receive GitLab repo from task claim and run checkout/pull workflow.
2. GitLab MR webhook events are ingested, persisted, and published to realtime.
3. MR identifiers link to Multica issues using existing prefix semantics.
4. Agent/runtime can post review comments back to MR with stable auth.
5. Code-scan findings can create Multica issues without duplicate spam.
6. Existing GitHub integration behavior remains unchanged (regression-safe).

---

## Non-Goals for MVP

1. Full UI parity with GitHub integrations tab.
2. Multi-provider merge analytics dashboard.
3. Cross-instance federation (GitHub + GitLab unified search).

These can be phase-2/phase-3 follow-ups after backend contract stabilizes.
