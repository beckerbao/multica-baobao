-- =====================
-- GitLab Merge Request
-- =====================

-- name: UpsertGitLabMergeRequest :one
INSERT INTO gitlab_merge_request (
    workspace_id, project_web_url, project_path, mr_iid,
    title, description, state, html_url, source_branch,
    author_username, author_avatar_url, merged_at, closed_at,
    mr_created_at, mr_updated_at
) VALUES (
    $1, $2, $3, $4,
    $5, $6, $7, $8, sqlc.narg('source_branch'),
    sqlc.narg('author_username'), sqlc.narg('author_avatar_url'), sqlc.narg('merged_at'), sqlc.narg('closed_at'),
    $9, $10
)
ON CONFLICT (workspace_id, project_web_url, mr_iid) DO UPDATE SET
    title = EXCLUDED.title,
    description = EXCLUDED.description,
    state = EXCLUDED.state,
    html_url = EXCLUDED.html_url,
    source_branch = EXCLUDED.source_branch,
    author_username = EXCLUDED.author_username,
    author_avatar_url = EXCLUDED.author_avatar_url,
    merged_at = EXCLUDED.merged_at,
    closed_at = EXCLUDED.closed_at,
    mr_updated_at = EXCLUDED.mr_updated_at,
    updated_at = now()
RETURNING *;

-- name: GetGitLabMergeRequest :one
SELECT * FROM gitlab_merge_request
WHERE workspace_id = $1 AND project_web_url = $2 AND mr_iid = $3;

-- name: LinkIssueToGitLabMergeRequest :exec
INSERT INTO issue_gitlab_merge_request (
    issue_id, gitlab_mr_id, linked_by_type, linked_by_id
) VALUES (
    $1, $2, sqlc.narg('linked_by_type'), sqlc.narg('linked_by_id')
)
ON CONFLICT (issue_id, gitlab_mr_id) DO NOTHING;

-- name: ListIssueIDsForGitLabMergeRequest :many
SELECT issue_id FROM issue_gitlab_merge_request
WHERE gitlab_mr_id = $1;
