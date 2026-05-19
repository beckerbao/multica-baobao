import { queryOptions } from "@tanstack/react-query";
import { api } from "../api";

export const projectKeys = {
  all: (wsId: string) => ["projects", wsId] as const,
  list: (wsId: string) => [...projectKeys.all(wsId), "list"] as const,
  detail: (wsId: string, id: string) =>
    [...projectKeys.all(wsId), "detail", id] as const,
  taskChanges: (wsId: string, id: string, limit: number) =>
    [...projectKeys.all(wsId), "task-changes", id, limit] as const,
  liveGitStatus: (wsId: string, id: string) =>
    [...projectKeys.all(wsId), "live-git-status", id] as const,
};

export function projectListOptions(wsId: string) {
  return queryOptions({
    queryKey: projectKeys.list(wsId),
    queryFn: () => api.listProjects(),
    select: (data) => data.projects,
  });
}

export function projectDetailOptions(wsId: string, id: string) {
  return queryOptions({
    queryKey: projectKeys.detail(wsId, id),
    queryFn: () => api.getProject(id),
  });
}

export function projectTaskChangesOptions(wsId: string, id: string, limit = 50) {
  return queryOptions({
    queryKey: projectKeys.taskChanges(wsId, id, limit),
    queryFn: () => api.listProjectTaskChanges(id, { limit }),
    select: (data) => data.items,
  });
}

export function projectLiveGitStatusOptions(wsId: string, id: string) {
  return queryOptions({
    queryKey: projectKeys.liveGitStatus(wsId, id),
    queryFn: () => api.getProjectLiveGitStatus(id),
  });
}
