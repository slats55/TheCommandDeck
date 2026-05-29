"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import { useWorkspaceId } from "@multica/core";
import type { CommandTemplate, CommandRun, PreviewRegistryEntry } from "@multica/core/types";

export default function CommandDeckPage() {
  const wsId = useWorkspaceId();
  const queryClient = useQueryClient();
  const [selectedTemplateId, setSelectedTemplateId] = useState<string>("");
  const [selectedRuntimeId, setSelectedRuntimeId] = useState<string>("");
  const [statusMessage, setStatusMessage] = useState<string>("");

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

  const templates: CommandTemplate[] = templatesData?.templates ?? [];
  const runs: CommandRun[] = runsData?.command_runs ?? [];
  const runtimes = runtimesData ?? [];
  const previews: PreviewRegistryEntry[] = previewsData?.previews ?? [];

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
      default: return status;
    }
  };

  const statusColor = (status: string): string => {
    switch (status) {
      case "completed": return "text-green-600";
      case "failed":
      case "timeout": return "text-red-600";
      case "running":
      case "pending": return "text-amber-600";
      default: return "text-gray-500";
    }
  };

  const previewHealthLabel = (status: PreviewRegistryEntry["health_status"]): string => {
    switch (status) {
      case "healthy": return "Healthy";
      case "unhealthy": return "Unhealthy";
      default: return "Unknown";
    }
  };

  const previewHealthColor = (status: PreviewRegistryEntry["health_status"]): string => {
    switch (status) {
      case "healthy": return "text-green-600";
      case "unhealthy": return "text-red-600";
      default: return "text-amber-600";
    }
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
          {previewsData?.last_checked_at && (
            <span className="text-xs text-muted-foreground">
              Checked {new Date(previewsData.last_checked_at).toLocaleTimeString()}
            </span>
          )}
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
                    <dt className="text-xs uppercase text-muted-foreground">Runtime</dt>
                    <dd>
                      {preview.runtime_name ?? preview.runtime_id ?? "No runtime registered"}
                      {preview.runtime_status && (
                        <span className="ml-2 text-muted-foreground">
                          ({preview.runtime_status})
                        </span>
                      )}
                    </dd>
                  </div>
                  <div>
                    <dt className="text-xs uppercase text-muted-foreground">Machine</dt>
                    <dd className="break-all">{preview.machine_identity ?? "-"}</dd>
                  </div>
                  <div>
                    <dt className="text-xs uppercase text-muted-foreground">Command</dt>
                    <dd>{preview.command ?? "Not command-started"}</dd>
                  </div>
                </dl>

                {preview.health_error && (
                  <p className="mt-3 break-all text-xs text-red-600">
                    {preview.health_error}
                  </p>
                )}
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
              No online runtimes available. Connect a daemon to execute commands.
            </p>
          ) : (
            <select
              className="mt-1 block w-full rounded-md border px-3 py-2 text-sm"
              value={selectedRuntimeId}
              onChange={(e) => setSelectedRuntimeId(e.target.value)}
            >
              <option value="">-- Select a runtime --</option>
              {runtimes
                .filter((rt) => rt.status === "online")
                .map((rt) => (
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
