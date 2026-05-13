-- Local repository mapping by daemon host:
-- binds a project to a host-local working directory for a specific daemon_id.

CREATE TABLE project_local_repo_path (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id  UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    project_id    UUID NOT NULL REFERENCES project(id) ON DELETE CASCADE,
    daemon_id     TEXT NOT NULL,
    local_path    TEXT NOT NULL,
    branch_hint   TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, project_id, daemon_id)
);

CREATE INDEX idx_project_local_repo_path_project
    ON project_local_repo_path(project_id);

CREATE INDEX idx_project_local_repo_path_daemon
    ON project_local_repo_path(workspace_id, daemon_id);
