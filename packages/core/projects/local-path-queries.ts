import { queryOptions, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { projectKeys } from "./queries";
import type {
  ListProjectLocalRepoPathsResponse,
  ProjectLocalRepoPath,
  UpsertProjectLocalRepoPathRequest,
} from "../types";

export const projectLocalPathKeys = {
  list: (wsId: string, projectId: string) =>
    [...projectKeys.detail(wsId, projectId), "local-repo-paths"] as const,
};

export function projectLocalPathsOptions(wsId: string, projectId: string) {
  return queryOptions({
    queryKey: projectLocalPathKeys.list(wsId, projectId),
    queryFn: () => api.listProjectLocalRepoPaths(projectId),
    select: (data) => data.items,
  });
}

export function useUpsertProjectLocalPath(wsId: string, projectId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: UpsertProjectLocalRepoPathRequest) =>
      api.upsertProjectLocalRepoPath(projectId, data),
    onSuccess: (updated) => {
      qc.setQueryData<ListProjectLocalRepoPathsResponse>(
        projectLocalPathKeys.list(wsId, projectId),
        (old) => {
          if (!old) return old;
          const idx = old.items.findIndex((x) => x.daemon_id === updated.daemon_id);
          if (idx === -1) {
            return { ...old, items: [...old.items, updated], total: old.total + 1 };
          }
          const next = [...old.items];
          next[idx] = updated;
          return { ...old, items: next };
        },
      );
    },
    onSettled: () => {
      qc.invalidateQueries({
        queryKey: projectLocalPathKeys.list(wsId, projectId),
      });
    },
  });
}

export function useDeleteProjectLocalPath(wsId: string, projectId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (daemonId: string) =>
      api.deleteProjectLocalRepoPath(projectId, daemonId),
    onMutate: async (daemonId) => {
      await qc.cancelQueries({
        queryKey: projectLocalPathKeys.list(wsId, projectId),
      });
      const prev = qc.getQueryData<ListProjectLocalRepoPathsResponse>(
        projectLocalPathKeys.list(wsId, projectId),
      );
      qc.setQueryData<ListProjectLocalRepoPathsResponse>(
        projectLocalPathKeys.list(wsId, projectId),
        (old) => {
          if (!old) return old;
          const nextItems = old.items.filter(
            (x: ProjectLocalRepoPath) => x.daemon_id !== daemonId,
          );
          return { ...old, items: nextItems, total: Math.max(0, old.total - 1) };
        },
      );
      return { prev };
    },
    onError: (_err, _id, ctx) => {
      if (ctx?.prev) {
        qc.setQueryData(projectLocalPathKeys.list(wsId, projectId), ctx.prev);
      }
    },
    onSettled: () => {
      qc.invalidateQueries({
        queryKey: projectLocalPathKeys.list(wsId, projectId),
      });
    },
  });
}

