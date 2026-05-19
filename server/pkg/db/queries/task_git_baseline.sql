-- name: UpsertTaskGitBaseline :one
INSERT INTO task_git_baseline (
    task_id, execution_workdir, baseline_head, baseline_branch, started_at, updated_at
)
VALUES (
    $1, $2, sqlc.narg('baseline_head'), sqlc.narg('baseline_branch'),
    COALESCE(sqlc.narg('started_at'), now()),
    now()
)
ON CONFLICT (task_id) DO UPDATE
SET execution_workdir = EXCLUDED.execution_workdir,
    baseline_head = EXCLUDED.baseline_head,
    baseline_branch = EXCLUDED.baseline_branch,
    started_at = EXCLUDED.started_at,
    updated_at = now()
RETURNING *;

-- name: GetTaskGitBaseline :one
SELECT * FROM task_git_baseline
WHERE task_id = $1;

