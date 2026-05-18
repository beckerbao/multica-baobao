"use client";

import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { AlertCircle, ChevronRight, HardDrive, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";
import {
  projectLocalPathsOptions,
  useDeleteProjectLocalPath,
  useUpsertProjectLocalPath,
} from "@multica/core/projects";
import { runtimeListOptions } from "@multica/core/runtimes/queries";
import { useWorkspaceId } from "@multica/core/hooks";
import type { ProjectLocalRepoPath } from "@multica/core/types";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@multica/ui/components/ui/select";
import { useT } from "../../i18n";

export function ProjectLocalPathSection({ projectId }: { projectId: string }) {
  const { t } = useT("projects");
  const wsId = useWorkspaceId();
  const [open, setOpen] = useState(true);
  const [daemonId, setDaemonId] = useState("");
  const [localPath, setLocalPath] = useState("");
  const [branchHint, setBranchHint] = useState("");

  const { data: mappings = [] } = useQuery(projectLocalPathsOptions(wsId, projectId));
  const { data: runtimes = [] } = useQuery(runtimeListOptions(wsId));
  const upsert = useUpsertProjectLocalPath(wsId, projectId);
  const remove = useDeleteProjectLocalPath(wsId, projectId);

  const daemonChoices = useMemo(
    () =>
      runtimes
        .filter((rt) => rt.daemon_id)
        .map((rt) => ({
          runtimeId: rt.id,
          daemonId: rt.daemon_id as string,
          label: `${rt.name} (${rt.daemon_id})`,
        })),
    [runtimes],
  );

  const onSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const d = daemonId.trim();
    const p = localPath.trim();
    if (!d || !p) return;
    try {
      await upsert.mutateAsync({
        daemon_id: d,
        local_path: p,
        branch_hint: branchHint.trim() || undefined,
      });
      toast.success(t(($) => $.local_paths.toast_saved));
      setLocalPath("");
      setBranchHint("");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t(($) => $.local_paths.toast_save_failed));
    }
  };

  const onRemove = async (d: string) => {
    try {
      await remove.mutateAsync(d);
      toast.success(t(($) => $.local_paths.toast_removed));
    } catch {
      toast.error(t(($) => $.local_paths.toast_remove_failed));
    }
  };

  return (
    <div>
      <button
        className={`flex w-full items-center gap-1 rounded-md px-2 py-1 text-xs font-medium transition-colors mb-2 hover:bg-accent/70 ${open ? "" : "text-muted-foreground hover:text-foreground"}`}
        onClick={() => setOpen(!open)}
      >
        {t(($) => $.local_paths.section_header)}
        <ChevronRight
          className={`!size-3 shrink-0 stroke-[2.5] text-muted-foreground transition-transform ${open ? "rotate-90" : ""}`}
        />
      </button>

      {open && (
        <div className="pl-2 space-y-2">
          {mappings.length === 0 && (
            <p className="text-xs text-muted-foreground">{t(($) => $.local_paths.empty)}</p>
          )}

          {mappings.map((item) => (
            <LocalPathRow key={item.id} item={item} onRemove={onRemove} />
          ))}

          <form onSubmit={onSubmit} className="space-y-2 rounded-md border p-2">
            <div className="text-xs font-medium text-muted-foreground flex items-center gap-1">
              <Plus className="size-3.5" />
              {t(($) => $.local_paths.add_title)}
            </div>
            <Select
              value={daemonId}
              onValueChange={(value) => setDaemonId(value ?? "")}
            >
              <SelectTrigger size="sm">
                <SelectValue>
                  {daemonId || t(($) => $.local_paths.daemon_placeholder)}
                </SelectValue>
              </SelectTrigger>
              <SelectContent>
                {daemonChoices.map((d) => (
                  <SelectItem key={`${d.runtimeId}:${d.daemonId}`} value={d.daemonId}>
                    {d.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {daemonChoices.length === 0 && (
              <div className="text-[11px] text-amber-700 flex items-center gap-1">
                <AlertCircle className="size-3.5" />
                {t(($) => $.local_paths.no_daemon_hint)}
              </div>
            )}
            <Input
              value={localPath}
              onChange={(e) => setLocalPath(e.target.value)}
              placeholder={t(($) => $.local_paths.path_placeholder)}
              className="h-8 text-xs"
            />
            <Input
              value={branchHint}
              onChange={(e) => setBranchHint(e.target.value)}
              placeholder={t(($) => $.local_paths.branch_placeholder)}
              className="h-8 text-xs"
            />
            <Button
              type="submit"
              size="sm"
              className="h-7 text-xs"
              disabled={!daemonId.trim() || !localPath.trim() || upsert.isPending}
            >
              {t(($) => $.local_paths.save_button)}
            </Button>
          </form>
        </div>
      )}
    </div>
  );
}

function LocalPathRow({
  item,
  onRemove,
}: {
  item: ProjectLocalRepoPath;
  onRemove: (daemonId: string) => void;
}) {
  const { t } = useT("projects");
  return (
    <div className="group rounded-md border px-2 py-1.5 text-xs">
      <div className="flex items-center gap-2">
        <HardDrive className="size-3.5 text-muted-foreground shrink-0" />
        <span className="font-medium truncate">{item.daemon_id}</span>
        <button
          type="button"
          onClick={() => onRemove(item.daemon_id)}
          className="ml-auto opacity-0 group-hover:opacity-100 transition-opacity rounded-sm p-0.5 hover:bg-accent"
          title={t(($) => $.local_paths.remove_tooltip)}
        >
          <Trash2 className="size-3 text-muted-foreground" />
        </button>
      </div>
      <div className="mt-1 text-muted-foreground break-all">{item.local_path}</div>
      {item.branch_hint && (
        <div className="mt-1 text-[11px] text-muted-foreground">
          {t(($) => $.local_paths.branch_label)}: {item.branch_hint}
        </div>
      )}
    </div>
  );
}
