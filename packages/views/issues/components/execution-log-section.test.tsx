import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { I18nProvider } from "@multica/core/i18n/react";
import type { AgentTask } from "@multica/core/types/agent";
import enCommon from "../../locales/en/common.json";
import enIssues from "../../locales/en/issues.json";
import { ExecutionLogSection } from "./execution-log-section";

const TEST_RESOURCES = { en: { common: enCommon, issues: enIssues } };

const mockApi = vi.hoisted(() => ({
  listTasksByIssue: vi.fn(),
  rerunIssue: vi.fn(),
  cancelTask: vi.fn(),
}));

vi.mock("@multica/core/api", () => ({ api: mockApi }));
vi.mock("../../common/actor-avatar", () => ({
  ActorAvatar: () => <span data-testid="actor-avatar">avatar</span>,
}));
vi.mock("../../common/task-transcript", () => ({
  TranscriptButton: () => <button>transcript</button>,
}));
vi.mock("sonner", () => ({
  toast: { error: vi.fn(), success: vi.fn() },
}));

function makeTask(id: string, overrides: Partial<AgentTask> = {}): AgentTask {
  return {
    id,
    agent_id: "agent-1",
    runtime_id: "rt-1",
    issue_id: "issue-1",
    status: "completed",
    priority: 0,
    dispatched_at: "2026-01-01T00:00:00Z",
    started_at: "2026-01-01T00:00:00Z",
    completed_at: "2026-01-01T00:01:00Z",
    result: null,
    error: null,
    created_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

function renderSection() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <I18nProvider locale="en" resources={TEST_RESOURCES}>
        <ExecutionLogSection issueId="issue-1" />
      </I18nProvider>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  mockApi.listTasksByIssue.mockReset();
  mockApi.rerunIssue.mockReset();
  mockApi.cancelTask.mockReset();
});

describe("ExecutionLogSection code changes", () => {
  it("renders execution folder, branch/head, diff stat and changed files", async () => {
    mockApi.listTasksByIssue.mockResolvedValueOnce([
      makeTask("task-1", {
        result: {
          execution_workdir: "/tmp/repo",
          change_summary: {
            collect_status: "ok",
            git_branch: "main",
            head_after: "1234567890abcdef",
            diff_stat: { files_changed: 2, insertions: 10, deletions: 4 },
            changed_files: [
              { status: "M", path: "a.txt" },
              { status: "??", path: "b.txt" },
            ],
          },
        },
      }),
    ]);

    renderSection();
    await waitFor(() => {
      expect(screen.getByText("Execution log")).toBeTruthy();
    });
    fireEvent.click(screen.getByText("Show past runs (1)"));

    expect(screen.getByText("Code Changes")).toBeTruthy();
    expect(screen.getByText("Execution folder:")).toBeTruthy();
    expect(screen.getByText("/tmp/repo")).toBeTruthy();
    expect(
      screen.getAllByText((_, el) => (el?.textContent ?? "").includes("Branch:")).length,
    ).toBeGreaterThan(0);
    expect(screen.getByText("main")).toBeTruthy();
    expect(screen.getByText("Diff stat: 2 files, +10/-4")).toBeTruthy();
    expect(screen.getByText("a.txt")).toBeTruthy();
    expect(screen.getByText("b.txt")).toBeTruthy();
  });

  it("renders git_unavailable message", async () => {
    mockApi.listTasksByIssue.mockResolvedValueOnce([
      makeTask("task-2", {
        result: {
          execution_workdir: "/tmp/non-git",
          change_summary: { collect_status: "git_unavailable", changed_files: [] },
        },
      }),
    ]);
    renderSection();
    await waitFor(() => {
      expect(screen.getByText("Show past runs (1)")).toBeTruthy();
    });
    fireEvent.click(screen.getByText("Show past runs (1)"));
    expect(screen.getByText("Git metadata unavailable for this task.")).toBeTruthy();
  });

  it("renders truncated and no changes states", async () => {
    mockApi.listTasksByIssue.mockResolvedValueOnce([
      makeTask("task-3", {
        result: {
          execution_workdir: "/tmp/repo",
          change_summary: { collect_status: "truncated", changed_files: [] },
        },
      }),
      makeTask("task-4", {
        result: {
          execution_workdir: "/tmp/repo",
          change_summary: { collect_status: "ok", changed_files: [] },
        },
      }),
    ]);
    renderSection();
    await waitFor(() => {
      expect(screen.getByText("Show past runs (2)")).toBeTruthy();
    });
    fireEvent.click(screen.getByText("Show past runs (2)"));
    expect(screen.getByText("Change list truncated due to size limits.")).toBeTruthy();
    expect(screen.getByText("No file changes detected.")).toBeTruthy();
  });

  it("renders collect error state", async () => {
    mockApi.listTasksByIssue.mockResolvedValueOnce([
      makeTask("task-5", {
        result: {
          execution_workdir: "/tmp/repo",
          change_summary: { collect_status: "error", changed_files: [] },
        },
      }),
    ]);
    renderSection();
    await waitFor(() => {
      expect(screen.getByText("Show past runs (1)")).toBeTruthy();
    });
    fireEvent.click(screen.getByText("Show past runs (1)"));
    expect(screen.getByText("Failed to collect git metadata.")).toBeTruthy();
  });
});
