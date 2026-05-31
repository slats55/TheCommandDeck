"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import { useWorkspaceId } from "@multica/core";
import { useWSEvent } from "@multica/core/realtime";
import type {
  CommandTemplate,
  CommandRun,
  CommandRunUpdatedPayload,
  CommandRunListResponse,
  PreviewRegistryEntry,
  PreviewLifecycleStatus,
} from "@multica/core/types";

function isCommandRun(value: unknown): value is CommandRun {
  if (!value || typeof value !== "object") return false;
  const run = value as Record<string, unknown>;
  const hasString = (key: string) => typeof run[key] === "string";
  const hasOptionalString = (key: string) => run[key] === undefined || typeof run[key] === "string";
  const hasOptionalNumber = (key: string) => run[key] === undefined || typeof run[key] === "number";
  return (
    hasString("id") &&
    hasString("status") &&
    hasString("command") &&
    hasString("working_directory") &&
    hasString("created_at") &&
    typeof run.stdout_truncated === "boolean" &&
    typeof run.stderr_truncated === "boolean" &&
    hasOptionalNumber("exit_code") &&
    hasOptionalString("stdout") &&
    hasOptionalString("stderr") &&
    hasOptionalNumber("duration_ms") &&
    hasOptionalString("started_at") &&
    hasOptionalString("finished_at") &&
    hasOptionalString("cancellation_requested_at") &&
    hasOptionalString("cancellation_requested_by_type") &&
    hasOptionalString("cancellation_requested_by_id")
  );
}

function isCommandRunUpdatedPayload(value: unknown): value is CommandRunUpdatedPayload {
  if (!value || typeof value !== "object") return false;
  const payload = value as Record<string, unknown>;
  return isCommandRun(payload.run);
}

export default function CommandDeckPage() {
  const wsId = useWorkspaceId();
  const queryClient = useQueryClient();
  const [selectedTemplateId, setSelectedTemplateId] = useState<string>("");
  const [selectedRuntimeId, setSelectedRuntimeId] = useState<string>("");
  const [statusMessage, setStatusMessage] = useState<string>("");
  const [retiringPreviewId, setRetiringPreviewId] = useState<string | null>(null);

  // Fetch command templates
  const {
    data: templatesData,
    isLoading: templatesLoading,
    error: templatesError,
  } = useQuery({
    queryKey: ["commanddeck", "templates", wsId],
    queryFn: () => api.listCommandTemplates(wsId),
  });

  // Fetch runtimes (for selecting where to run)
  const {
    data: runtimesData,
    isLoading: runtimesLoading,
  } = useQuery({
    queryKey: ["runtimes", wsId],
    queryFn: () => api.listRuntimes({ workspace_id: wsId }),
  });

  // Fetch command runs
  const {
    data: runsData,
    isLoading: runsLoading,
  } = useQuery({
    queryKey: ["commanddeck", "runs", wsId],
    queryFn: () => api.listCommandRuns(),
    refetchInterval: 5000, // Poll every 5 seconds for pending runs
  });

  const {
    data: previewsData,
    isLoading: previewsLoading,
  } = useQuery({
    queryKey: ["commanddeck", "previews", wsId],
    queryFn: () => api.listPreviewRegistry(),
    refetchInterval: 15000,
  });

  const previewSyncMutation = useMutation({
    mutationFn: () => api.syncSelfHostedPreviewRegistry(),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["commanddeck", "previews", wsId] });
    },
  });
  const previewRetireMutation = useMutation({
    mutationFn: (previewId: string) => api.retirePreviewRegistryEntry(previewId),
    onMutate: (previewId) => {
      setRetiringPreviewId(previewId);
    },
    onSuccess: () => {
      setStatusMessage("Preview retired from active registry.");
      queryClient.invalidateQueries({ queryKey: ["commanddeck", "previews", wsId] });
    },
    onError: (err: Error) => {
      setStatusMessage(`Failed to retire preview: ${err.message}`);
    },
    onSettled: () => {
      setRetiringPreviewId(null);
    },
  });

  // Execute command mutation
  const runMutation = useMutation({
    mutationFn: () =>
      api.runCommand({
        runtime_id: selectedRuntimeId,
        template_id: selectedTemplateId || undefined,
      }),
    onSuccess: () => {
      setStatusMessage("Command dispatched successfully.");
      queryClient.invalidateQueries({ queryKey: ["commanddeck", "runs", wsId] });
    },
    onError: (err: Error) => {
      setStatusMessage(`Failed to dispatch command: ${err.message}`);
    },
  });
  const cancelMutation = useMutation({
    mutationFn: (runId: string) => api.cancelCommandRun(runId),
    onSuccess: () => {
      setStatusMessage("Cancellation requested.");
      queryClient.invalidateQueries({ queryKey: ["commanddeck", "runs", wsId] });
    },
    onError: (err: Error) => {
      setStatusMessage(`Failed to cancel command: ${err.message}`);
    },
  });

  const templates: CommandTemplate[] = templatesData?.templates ?? [];
  const runs: CommandRun[] = runsData?.command_runs ?? [];
  const runtimes = runtimesData ?? [];
  const onlineRuntimes = runtimes.filter((rt) => rt.status === "online");
  const previews: PreviewRegistryEntry[] = previewsData?.previews ?? [];
  const runtimesById = new Map(runtimes.map((runtime) => [runtime.id, runtime]));

  useWSEvent("command_run:updated", (payload) => {
    if (!isCommandRunUpdatedPayload(payload)) {
      return;
    }
    const incoming = payload.run;
    queryClient.setQueryData<CommandRunListResponse>(
      ["commanddeck", "runs", wsId],
      (current) => {
        if (!current) {
          return { command_runs: [incoming], total: 1 };
        }
        const withoutIncoming = current.command_runs.filter((run) => run.id !== incoming.id);
        const nextRuns = [incoming, ...withoutIncoming];
        return {
          command_runs: nextRuns,
          total: nextRuns.length,
        };
      },
    );
  });

  const handleRun = () => {
    if (!selectedRuntimeId) {
      setStatusMessage("Please select a runtime first.");
      return;
    }
    setStatusMessage("Dispatching...");
    runMutation.mutate();
  };

  const statusLabel = (status: string): string => {
    switch (status) {
      case "pending": return "Pending";
      case "running": return "Running";
      case "completed": return "Completed";
      case "failed": return "Failed";
      case "timeout": return "Timed out";
      case "cancelled": return "Cancelled";
      default: return status;
    }
  };

  const statusColor = (status: string): string => {
    switch (status) {
      case "completed": return "text-green-600";
      case "failed":
      case "timeout": return "text-red-600";
      case "cancelled": return "text-orange-600";
      case "running":
      case "pending": return "text-amber-600";
      default: return "text-gray-500";
    }
  };

  const previewHealthLabel = (status: PreviewRegistryEntry["health_status"]): string => {
    switch (status) {
      case "healthy": return "Healthy";
      case "unhealthy": return "Unhealthy";
      case "unavailable": return "Unavailable";
      default: return "Unknown";
    }
  };

  const isPreviewStale = (preview: PreviewRegistryEntry): boolean => {
    if (!preview.last_checked_at) return true;
    const checked = new Date(preview.last_checked_at).getTime();
    if (Number.isNaN(checked)) return true;
    return Date.now() - checked > 5 * 60 * 1000;
  };

  const deriveLegacyLifecycleStatus = (preview: PreviewRegistryEntry): PreviewLifecycleStatus => {
    if (isPreviewStale(preview)) return "stale";
    if (preview.health_status === "healthy") return "healthy";
    if (preview.last_success_at || preview.health_status === "unavailable" || preview.health_status === "unhealthy") {
      return "offline";
    }
    return "registered";
  };

  const previewLifecycleStatus = (preview: PreviewRegistryEntry): PreviewLifecycleStatus => {
    return preview.lifecycle_status ?? deriveLegacyLifecycleStatus(preview);
  };

  const previewLifecycleLabel = (status: PreviewLifecycleStatus): string => {
    switch (status) {
      case "healthy": return "Healthy";
      case "stale": return "Stale";
      case "offline": return "Offline";
      case "runtime_disconnected": return "Runtime disconnected";
      case "retired": return "Retired";
      default: return "Registered";
    }
  };

  const previewLifecycleColor = (status: PreviewLifecycleStatus): string => {
    switch (status) {
      case "healthy": return "text-green-600";
      case "stale":
      case "runtime_disconnected": return "text-amber-600";
      case "offline":
      case "retired": return "text-red-600";
      default: return "text-muted-foreground";
    }
  };

  const canRetirePreview = (preview: PreviewRegistryEntry): boolean => {
    const status = previewLifecycleStatus(preview);
    return status === "stale" || status === "offline" || status === "runtime_disconnected";
  };

  const previewHealthColor = (status: PreviewRegistryEntry["health_status"]): string => {
    switch (status) {
      case "healthy": return "text-green-600";
      case "unhealthy":
      case "unavailable": return "text-red-600";
      default: return "text-amber-600";
    }
  };

  const runtimeHealthLabel = (status?: string): string => {
    switch (status) {
      case "online": return "Online";
      case "stale": return "Stale";
      case "offline": return "Offline";
      default: return "Unknown";
    }
  };

  const runtimeHealthColor = (status?: string): string => {
    switch (status) {
      case "online": return "text-green-600";
      case "stale": return "text-amber-600";
      case "offline": return "text-red-600";
      default: return "text-muted-foreground";
    }
  };

  const previewRuntimeProvenanceLabel = (preview: PreviewRegistryEntry): string => {
    if (!preview.runtime_id) {
      return "Runtime provenance not yet established";
    }
    const runtime = runtimesById.get(preview.runtime_id);
    const healthStatus = runtime?.health_status;
    if (healthStatus === "offline") {
      return "Reported by verified runtime (runtime offline)";
    }
    if (healthStatus === "stale") {
      return "Reported by verified runtime (runtime stale)";
    }
    if (healthStatus === "online") {
      return "Reported by verified runtime";
    }
    return "Reported by verified runtime (runtime status unknown)";
  };

  return (
    <div className="p-6 max-w-4xl mx-auto space-y-6">
      <div>
        <h1 className="text-2xl font-semibold">Command Deck</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Only allowlisted, non-destructive commands can run from CommandDeck.
        </p>
      </div>

      {/* Security Notice */}
      <div className="rounded-lg border border-amber-200 bg-amber-50 p-4 text-sm text-amber-800">
        <strong>Security:</strong> Only approved commands are available
        (git status, git branch, git rev-parse, git diff).
        Raw command input is not supported. All commands execute within
        workspace boundaries with runtime identity preserved.
      </div>

      {/* Preview Registry */}
      <div className="rounded-lg border bg-card p-6 space-y-4">
        <div className="flex items-center justify-between gap-3">
          <h2 className="text-lg font-medium">Preview Registry</h2>
          <div className="flex items-center gap-3">
            {previewsData?.last_checked_at && (
              <span className="text-xs text-muted-foreground">
                Checked {new Date(previewsData.last_checked_at).toLocaleTimeString()}
              </span>
            )}
            <button
              className="rounded-md border px-3 py-1 text-xs font-medium hover:bg-muted disabled:opacity-50"
              disabled={previewSyncMutation.isPending}
              onClick={() => previewSyncMutation.mutate()}
            >
              {previewSyncMutation.isPending ? "Refreshing..." : "Register/Refresh Preview"}
            </button>
          </div>
        </div>

        {previewsLoading ? (
          <p className="text-sm text-muted-foreground">Checking previews...</p>
        ) : previews.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No previews are registered for this workspace.
          </p>
        ) : (
          <div className="space-y-3">
            {previews.map((preview) => (
              <div key={preview.id} className="rounded-md border p-4">
                <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
                  <div className="min-w-0">
                    <a
                      href={preview.preview_url}
                      target="_blank"
                      rel="noreferrer"
                      className="break-all font-medium text-primary hover:underline"
                    >
                      {preview.preview_url}
                    </a>
                    <p className="mt-1 text-xs text-muted-foreground">
                      Workspace: {preview.workspace_name}
                    </p>
                  </div>
                  <div className="shrink-0 text-sm">
                    <span className={previewHealthColor(preview.health_status)}>
                      {previewHealthLabel(preview.health_status)}
                    </span>
                    {preview.health_status_code != null && (
                      <span className="ml-2 text-muted-foreground">
                        HTTP {preview.health_status_code}
                      </span>
                    )}
                  </div>
                </div>

                <dl className="mt-4 grid gap-3 text-sm sm:grid-cols-2">
                  <div>
                    <dt className="text-xs uppercase text-muted-foreground">Port</dt>
                    <dd className="font-mono">{preview.port || "-"}</dd>
                  </div>
                  <div>
                    <dt className="text-xs uppercase text-muted-foreground">Registered</dt>
                    <dd>
                      {preview.registered_at
                        ? new Date(preview.registered_at).toLocaleString()
                        : "-"}
                    </dd>
                  </div>
                  <div>
                    <dt className="text-xs uppercase text-muted-foreground">Runtime</dt>
                    <dd>
                      <div>{preview.runtime_name ?? preview.runtime_id ?? "-"}</div>
                      <div className="text-xs text-muted-foreground">{previewRuntimeProvenanceLabel(preview)}</div>
                    </dd>
                  </div>
                  <div>
                    <dt className="text-xs uppercase text-muted-foreground">Machine</dt>
                    <dd className="break-all">{preview.machine_identity ?? "-"}</dd>
                  </div>
                  <div>
                    <dt className="text-xs uppercase text-muted-foreground">Last Checked</dt>
                    <dd>
                      {preview.last_checked_at
                        ? new Date(preview.last_checked_at).toLocaleString()
                        : "-"}
                    </dd>
                  </div>
                  <div>
                    <dt className="text-xs uppercase text-muted-foreground">Last Successful Check</dt>
                    <dd>
                      {preview.last_success_at
                        ? new Date(preview.last_success_at).toLocaleString()
                        : "-"}
                    </dd>
                  </div>
                  <div>
                    <dt className="text-xs uppercase text-muted-foreground">Lifecycle</dt>
                    <dd className={previewLifecycleColor(previewLifecycleStatus(preview))}>
                      {previewLifecycleLabel(previewLifecycleStatus(preview))}
                    </dd>
                  </div>
                  <div>
                    <dt className="text-xs uppercase text-muted-foreground">Command</dt>
                    <dd>{preview.command ?? "Not command-started"}</dd>
                  </div>
                </dl>

                {canRetirePreview(preview) && (
                  <div className="mt-3 flex justify-end">
                    <button
                      className="rounded-md border px-2 py-1 text-xs font-medium hover:bg-muted disabled:opacity-50"
                      disabled={previewRetireMutation.isPending}
                      onClick={() => previewRetireMutation.mutate(preview.id)}
                    >
                      {previewRetireMutation.isPending && retiringPreviewId === preview.id ? "Retiring..." : "Retire Preview"}
                    </button>
                  </div>
                )}

                {preview.health_message && (
                  <p className="mt-3 break-all text-xs text-red-600">
                    {preview.health_message}
                  </p>
                )}
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Command Runner Panel */}
      <div className="rounded-lg border bg-card p-6 space-y-4">
        <h2 className="text-lg font-medium">Runtime Health</h2>
        {runtimesLoading ? (
          <p className="text-sm text-muted-foreground">Loading runtimes...</p>
        ) : runtimes.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No runtimes are registered for this workspace.
          </p>
        ) : (
          <div className="space-y-2">
            {runtimes.map((runtime) => (
              <div
                key={runtime.id}
                className="flex flex-col gap-1 rounded-md border p-3 text-sm sm:flex-row sm:items-center sm:justify-between"
              >
                <div className="min-w-0">
                  <p className="truncate font-medium">{runtime.name || runtime.id}</p>
                  <p className="text-xs text-muted-foreground">
                    {runtime.provider} · {runtime.runtime_mode}
                  </p>
                </div>
                <div className="text-right">
                  <p className={runtimeHealthColor(runtime.health_status)}>
                    {runtimeHealthLabel(runtime.health_status)}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    Last seen{" "}
                    {runtime.last_seen_at
                      ? new Date(runtime.last_seen_at).toLocaleTimeString()
                      : "unknown"}
                  </p>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Command Runner Panel */}
      <div className="rounded-lg border bg-card p-6 space-y-4">
        <h2 className="text-lg font-medium">Run a Command</h2>

        {/* Template selection */}
        <div>
          <label className="text-sm font-medium">
            Command Template
          </label>
          {templatesLoading ? (
            <p className="text-sm text-muted-foreground">Loading templates...</p>
          ) : templatesError ? (
            <p className="text-sm text-red-500">
              Failed to load templates. Backend may not have CommandDeck enabled.
            </p>
          ) : templates.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No templates available. Run database migrations to seed built-in command templates.
            </p>
          ) : (
            <select
              className="mt-1 block w-full rounded-md border px-3 py-2 text-sm"
              value={selectedTemplateId}
              onChange={(e) => setSelectedTemplateId(e.target.value)}
            >
              <option value="">-- Select a command --</option>
              {templates.map((t) => (
                <option key={t.id} value={t.id}>
                  {t.name} — {t.command}
                </option>
              ))}
            </select>
          )}
        </div>

        {/* Runtime selection */}
        <div>
          <label className="text-sm font-medium">
            Target Runtime
          </label>
          {runtimesLoading ? (
            <p className="text-sm text-muted-foreground">Loading runtimes...</p>
          ) : runtimes.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No runtimes are registered for this workspace.
            </p>
          ) : onlineRuntimes.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No online runtimes available. Connect a daemon to execute commands.
            </p>
          ) : (
            <select
              className="mt-1 block w-full rounded-md border px-3 py-2 text-sm"
              value={selectedRuntimeId}
              onChange={(e) => setSelectedRuntimeId(e.target.value)}
            >
              <option value="">-- Select a runtime --</option>
              {onlineRuntimes.map((rt) => (
                <option key={rt.id} value={rt.id}>
                  {rt.name ?? rt.id} ({rt.status})
                </option>
              ))}
            </select>
          )}
        </div>

        {/* Run button */}
        <button
          className="rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
          disabled={
            runMutation.isPending ||
            !selectedRuntimeId ||
            !selectedTemplateId
          }
          onClick={handleRun}
        >
          {runMutation.isPending ? "Dispatching..." : "Run Command"}
        </button>

        {/* Status message */}
        {statusMessage && (
          <p className="text-sm text-muted-foreground">{statusMessage}</p>
        )}
      </div>

      {/* Command Run History */}
      <div className="rounded-lg border bg-card p-6 space-y-4">
        <h2 className="text-lg font-medium">Run History</h2>

        {runsLoading ? (
          <p className="text-sm text-muted-foreground">Loading runs...</p>
        ) : runs.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No commands have been run yet. Select a template and runtime above to execute your first command.
          </p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b text-left">
                  <th className="py-2 pr-4 font-medium">Command</th>
                  <th className="py-2 pr-4 font-medium">Status</th>
                  <th className="py-2 pr-4 font-medium">Exit Code</th>
                  <th className="py-2 pr-4 font-medium">Duration</th>
                  <th className="py-2 pr-4 font-medium">Created</th>
                  <th className="py-2 font-medium">Output</th>
                  <th className="py-2 font-medium">Actions</th>
                </tr>
              </thead>
              <tbody>
                {runs.map((run) => (
                  <tr key={run.id} className="border-b last:border-0">
                    <td className="py-2 pr-4 font-mono">
                      {run.command}
                    </td>
                    <td className={`py-2 pr-4 ${statusColor(run.status)}`}>
                      {statusLabel(run.status)}
                    </td>
                    <td className="py-2 pr-4 font-mono">
                      {run.exit_code != null ? run.exit_code : "—"}
                    </td>
                    <td className="py-2 pr-4">
                      {run.duration_ms != null ? `${run.duration_ms}ms` : "—"}
                    </td>
                    <td className="py-2 pr-4 text-muted-foreground">
                      {run.created_at
                        ? new Date(run.created_at).toLocaleTimeString()
                        : "—"}
                    </td>
                    <td className="py-2 max-w-xs truncate font-mono text-xs">
                      {run.stdout
                        ? run.stdout.slice(0, 80)
                        : run.stderr
                          ? `ERR: ${run.stderr.slice(0, 80)}`
                          : "—"}
                      {(run.stdout_truncated || run.stderr_truncated) && (
                        <span className="ml-2 text-amber-600">truncated</span>
                      )}
                      {run.cancellation_requested_at && (
                        <span className="ml-2 text-muted-foreground">
                          cancel requested {new Date(run.cancellation_requested_at).toLocaleTimeString()}
                        </span>
                      )}
                    </td>
                    <td className="py-2">
                      {(run.status === "pending" || run.status === "running") ? (
                        <button
                          className="rounded-md border px-2 py-1 text-xs font-medium hover:bg-muted disabled:opacity-50"
                          disabled={cancelMutation.isPending}
                          onClick={() => cancelMutation.mutate(run.id)}
                        >
                          {cancelMutation.isPending ? "Cancelling..." : "Cancel run"}
                        </button>
                      ) : (
                        <span className="text-xs text-muted-foreground">—</span>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}
