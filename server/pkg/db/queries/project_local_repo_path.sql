-- name: ListProjectLocalRepoPathsByProject :many
SELECT * FROM project_local_repo_path
WHERE project_id = $1
ORDER BY created_at ASC;

-- name: GetProjectLocalRepoPathByProjectAndDaemon :one
SELECT * FROM project_local_repo_path
WHERE workspace_id = $1 AND project_id = $2 AND daemon_id = $3;

-- name: UpsertProjectLocalRepoPath :one
INSERT INTO project_local_repo_path (
    workspace_id, project_id, daemon_id, local_path, branch_hint
) VALUES (
    $1, $2, $3, $4, sqlc.narg('branch_hint')
)
ON CONFLICT (workspace_id, project_id, daemon_id) DO UPDATE SET
    local_path = EXCLUDED.local_path,
    branch_hint = EXCLUDED.branch_hint,
    updated_at = now()
RETURNING *;

-- name: DeleteProjectLocalRepoPath :exec
DELETE FROM project_local_repo_path
WHERE workspace_id = $1 AND project_id = $2 AND daemon_id = $3;
