import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import type { ReactNode } from "react";
import { I18nProvider } from "@multica/core/i18n/react";
import enCommon from "../../locales/en/common.json";
import enProjects from "../../locales/en/projects.json";
import { ProjectDetail } from "./project-detail";

const resources = { en: { common: enCommon, projects: enProjects } };

const runProjectGitActionMock = vi.fn();
const toastMock = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
}));

let mutationPending = false;

vi.mock("sonner", () => ({ toast: toastMock }));
vi.mock("@multica/core/api", () => ({
  api: {
    runProjectGitAction: (...args: unknown[]) => runProjectGitActionMock(...args),
  },
}));

vi.mock("@tanstack/react-query", async () => {
  const actual = await vi.importActual<typeof import("@tanstack/react-query")>("@tanstack/react-query");
  return {
    ...actual,
    useQuery: vi.fn((opts: { queryKey?: unknown[] }) => {
      const key = opts?.queryKey ?? [];
      const keyText = JSON.stringify(key);
      if (keyText.includes("\"detail\"")) {
        return { data: { id: "p1", title: "Project A", status: "planned", priority: "none", issue_count: 0, done_count: 0 }, isLoading: false };
      }
      if (keyText.includes("\"task-changes\"")) {
        return { data: [], isLoading: false };
      }
      if (keyText.includes("\"live-git-status\"")) {
        return {
          data: {
            collect_status: "ok",
            execution_workdir: "/tmp/repo",
            git_branch: "main",
            changed_files: [],
            diff_stat: { files_changed: 0, insertions: 0, deletions: 0 },
          },
          isLoading: false,
        };
      }
      return { data: [], isLoading: false };
    }),
    useQueryClient: vi.fn(() => ({ invalidateQueries: vi.fn().mockResolvedValue(undefined) })),
    useMutation: vi.fn((config: { onSuccess?: (resp: any) => void; onError?: (err: unknown) => void; mutationFn: (payload: any) => Promise<any> }) => ({
      isPending: mutationPending,
      mutate: async (payload: any) => {
        try {
          const resp = await config.mutationFn(payload);
          await config.onSuccess?.(resp);
        } catch (err) {
          config.onError?.(err);
        }
      },
    })),
  };
});

vi.mock("@multica/core/auth", () => ({
  useAuthStore: () => "u1",
}));
vi.mock("@multica/core/hooks", () => ({
  useWorkspaceId: () => "ws1",
}));
vi.mock("@multica/core/paths", () => ({
  useWorkspacePaths: () => ({ projects: () => "/projects" }),
  useCurrentWorkspace: () => ({ name: "WS" }),
}));
vi.mock("@multica/core/workspace/hooks", () => ({
  useActorName: () => ({ getActorName: () => "User" }),
}));
vi.mock("@multica/core/projects/mutations", () => ({
  useUpdateProject: () => ({ mutate: vi.fn() }),
  useDeleteProject: () => ({ mutate: vi.fn() }),
}));
vi.mock("@multica/core/pins", () => ({
  pinListOptions: () => ({}),
  useCreatePin: () => ({ mutate: vi.fn() }),
  useDeletePin: () => ({ mutate: vi.fn() }),
}));
vi.mock("@multica/core/issues/mutations", () => ({
  useUpdateIssue: () => ({ mutate: vi.fn() }),
}));
vi.mock("@multica/core/workspace/queries", () => ({
  memberListOptions: () => ({}),
  agentListOptions: () => ({}),
}));
vi.mock("@multica/core/issues/queries", () => ({
  myIssueListOptions: () => ({}),
  childIssueProgressOptions: () => ({}),
}));
vi.mock("@multica/core/modals", () => ({
  useModalStore: { getState: () => ({ open: vi.fn() }) },
}));
vi.mock("@multica/core/issues/stores/view-store", () => ({
  createIssueViewStore: () => ({}),
}));
vi.mock("@multica/core/issues/stores/view-store-context", () => ({
  ViewStoreProvider: ({ children }: { children: ReactNode }) => <>{children}</>,
  useViewStore: (selector: (state: any) => unknown) => selector({
    viewMode: "list",
    statusFilters: [],
    priorityFilters: [],
    assigneeFilters: [],
    includeNoAssignee: true,
    creatorFilters: [],
    labelFilters: [],
  }),
}));

vi.mock("../../navigation", () => ({
  AppLink: ({ children }: { children: ReactNode }) => <a>{children}</a>,
  useNavigation: () => ({ push: vi.fn() }),
}));
vi.mock("../../common/actor-avatar", () => ({ ActorAvatar: () => <span /> }));
vi.mock("../../editor", () => ({
  TitleEditor: () => <div />,
  ContentEditor: () => <div />,
}));
vi.mock("../../issues/components/priority-icon", () => ({ PriorityIcon: () => <span /> }));
vi.mock("./project-resources-section", () => ({ ProjectResourcesSection: () => <div /> }));
vi.mock("./project-local-path-section", () => ({ ProjectLocalPathSection: () => <div /> }));
vi.mock("../../issues/components/issues-header", () => ({ IssuesHeader: () => <div /> }));
vi.mock("../../issues/components/board-view", () => ({ BoardView: () => <div /> }));
vi.mock("../../issues/components/list-view", () => ({ ListView: () => <div /> }));
vi.mock("../../issues/components/batch-action-toolbar", () => ({ BatchActionToolbar: () => <div /> }));
vi.mock("./project-issue-metrics", () => ({
  getProjectIssueMetrics: () => ({ total: 0, done: 0, progress: 0 }),
}));

function renderDetail() {
  return render(
    <I18nProvider locale="en" resources={resources}>
      <ProjectDetail projectId="p1" />
    </I18nProvider>,
  );
}

describe("ProjectDetail Git Actions panel", () => {
  beforeEach(() => {
    mutationPending = false;
    runProjectGitActionMock.mockReset();
    toastMock.success.mockReset();
    toastMock.error.mockReset();
    vi.stubGlobal("confirm", vi.fn(() => true));
  });

  it("renders git action controls and disables them while pending", () => {
    mutationPending = true;
    renderDetail();
    expect(screen.getByText("Git Actions")).toBeTruthy();
    expect(screen.getByText("Refresh Status")).toBeDisabled();
    expect(screen.getByText("Fetch")).toBeDisabled();
    expect(screen.getByText("Pull (ff-only)")).toBeDisabled();
  });

  it("shows error feedback and stderr details when action fails", async () => {
    runProjectGitActionMock.mockResolvedValueOnce({
      ok: false,
      action: "fetch",
      error_code: "remote_auth_failed",
      error_message: "auth failed",
      stderr: "fatal: Authentication failed",
      stdout: "",
    });
    renderDetail();
    fireEvent.click(screen.getByText("Fetch"));
    expect(await screen.findByText("fatal: Authentication failed")).toBeTruthy();
    expect(toastMock.error).toHaveBeenCalled();
  });
});
