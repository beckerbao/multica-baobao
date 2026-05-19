CREATE TABLE IF NOT EXISTS task_git_baseline (
    task_id UUID PRIMARY KEY REFERENCES agent_task_queue(id) ON DELETE CASCADE,
    execution_workdir TEXT NOT NULL,
    baseline_head TEXT,
    baseline_branch TEXT,
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_task_git_baseline_started_at
    ON task_git_baseline (started_at DESC);
