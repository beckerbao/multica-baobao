-- GitLab merge request integration: mirrored MR state and issue links.

CREATE TABLE gitlab_merge_request (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id        UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    project_web_url     TEXT NOT NULL,
    project_path        TEXT NOT NULL,
    mr_iid              INTEGER NOT NULL,
    title               TEXT NOT NULL,
    description         TEXT NOT NULL DEFAULT '',
    state               TEXT NOT NULL
        CHECK (state IN ('open', 'closed', 'merged')),
    html_url            TEXT NOT NULL,
    source_branch       TEXT,
    author_username     TEXT,
    author_avatar_url   TEXT,
    merged_at           TIMESTAMPTZ,
    closed_at           TIMESTAMPTZ,
    mr_created_at       TIMESTAMPTZ NOT NULL,
    mr_updated_at       TIMESTAMPTZ NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, project_web_url, mr_iid)
);

CREATE INDEX idx_gitlab_merge_request_workspace ON gitlab_merge_request(workspace_id);

CREATE TABLE issue_gitlab_merge_request (
    issue_id            UUID NOT NULL REFERENCES issue(id) ON DELETE CASCADE,
    gitlab_mr_id        UUID NOT NULL REFERENCES gitlab_merge_request(id) ON DELETE CASCADE,
    linked_by_type      TEXT,
    linked_by_id        UUID,
    linked_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (issue_id, gitlab_mr_id)
);

CREATE INDEX idx_issue_gitlab_merge_request_mr ON issue_gitlab_merge_request(gitlab_mr_id);
