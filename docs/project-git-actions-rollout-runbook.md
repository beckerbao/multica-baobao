# Project Git Actions Phase 1 - Rollout Runbook

## Purpose

Operational runbook to safely roll out `PROJECT_GIT_ACTIONS_PHASE1` and evaluate stability before wider enablement.

## Prerequisites

- Backend deployed with support for:
  - `POST /api/projects/{id}/git-actions`
  - `GET /api/projects/{id}/live-git-status`
- Feature flags available:
  - `PROJECT_GIT_ACTIONS_PHASE1`
  - `PROJECT_GIT_ACTIONS_REQUIRE_ADMIN` (optional stricter gate)
- Local path mappings configured for pilot projects.

## Step 0 - Enable in dev workspace only

Set env:

```bash
PROJECT_GIT_ACTIONS_PHASE1=true
PROJECT_GIT_ACTIONS_REQUIRE_ADMIN=false
```

Restart backend after env change.

## Step 1 - Pilot smoke flow (same day)

For 1-2 pilot projects, verify:

1. `status` works and shows clean/dirty counts.
2. `branch_list` returns expected branches.
3. `fetch` succeeds on a valid remote.
4. `checkout_existing_branch` succeeds for an existing branch.
5. `pull_ff_only`:
   - succeeds in fast-forward case
   - returns `ff_only_failed` for non-FF case
6. non-git folder returns `not_git_repo`.

## Step 2 - Observe logs/error categories for 1-2 days

### What to watch

- `collect_status` distribution:
  - expected majority: `ok`
  - watch spikes: `remote_auth_failed`, `internal_error`, `timeout`
- `stderr_category` distribution:
  - expected low conflict/auth rates
- latency:
  - `duration_ms` p50/p95 for each action

### Suggested log filters

```bash
# Example plain grep from backend logs
grep "project git action executed" backend.log

# Quick category counts
grep "project git action executed" backend.log | grep -o 'collect_status=[^ ]*' | sort | uniq -c
grep "project git action executed" backend.log | grep -o 'stderr_category=[^ ]*' | sort | uniq -c
```

## Step 3 - Expand cohort

Expand when all are true:

- No sustained `internal_error` spike.
- `remote_auth_failed` is explainable by credentials/user setup.
- p95 latency is acceptable for team workflow.
- Pilot users report successful status/fetch/checkout/pull flows.

Then enable flag for wider workspace cohort.

## Rollback

If instability is detected:

```bash
PROJECT_GIT_ACTIONS_PHASE1=false
```

Restart backend and retain logs for incident review.
