import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent, act, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";

const { apiMock } = vi.hoisted(() => ({
  apiMock: {
    listCommandTemplates: vi.fn(),
    listRuntimes: vi.fn(),
    listCommandRuns: vi.fn(),
    listPreviewRegistry: vi.fn(),
    syncSelfHostedPreviewRegistry: vi.fn(),
    retirePreviewRegistryEntry: vi.fn(),
    listCommandWorkflowExecutions: vi.fn(),
    createCommandWorkflowExecution: vi.fn(),
    updateCommandWorkflowExecutionStatus: vi.fn(),
    runCommand: vi.fn(),
    cancelCommandRun: vi.fn(),
  },
}));

const { wsHandlers } = vi.hoisted(() => ({
  wsHandlers: new Map<string, ((payload: unknown, actorId?: string) => void)[]>(),
}));

vi.mock("@multica/core/api", () => ({
  api: apiMock,
}));

vi.mock("@multica/core", () => ({
  useWorkspaceId: () => "workspace-1",
}));

vi.mock("@multica/core/realtime", () => ({
  useWSEvent: (event: string, handler: (payload: unknown, actorId?: string) => void) => {
    const handlers = wsHandlers.get(event) ?? [];
    handlers.push(handler);
    wsHandlers.set(event, handlers);
  },
}));

import CommandDeckPage from "./page";

function createWrapper() {
  const qc = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
}

describe("CommandDeckPage Preview Registry", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    wsHandlers.clear();
    apiMock.listCommandTemplates.mockResolvedValue({ templates: [] });
    apiMock.listRuntimes.mockResolvedValue([]);
    apiMock.listCommandRuns.mockResolvedValue({ command_runs: [], total: 0 });
    apiMock.listPreviewRegistry.mockResolvedValue({
      previews: [],
      last_checked_at: "2026-05-29T00:00:00Z",
    });
    apiMock.retirePreviewRegistryEntry.mockResolvedValue({
      previews: [],
      last_checked_at: "2026-05-29T00:00:00Z",
    });
    apiMock.listCommandWorkflowExecutions.mockResolvedValue({
      workflow_executions: [],
      total: 0,
    });
    apiMock.createCommandWorkflowExecution.mockResolvedValue({
      id: "wf-1",
      workspace_id: "workspace-1",
      title: "Workflow",
      objective: "Objective",
      status: "planned",
      created_by_type: "member",
      created_by_id: "user-1",
      created_at: "2026-05-29T00:00:00Z",
      updated_at: "2026-05-29T00:00:00Z",
    });
    apiMock.updateCommandWorkflowExecutionStatus.mockResolvedValue({
      id: "wf-1",
      workspace_id: "workspace-1",
      title: "Workflow",
      objective: "Objective",
      status: "running",
      created_by_type: "member",
      created_by_id: "user-1",
      created_at: "2026-05-29T00:00:00Z",
      updated_at: "2026-05-29T00:01:00Z",
    });
  });

  const emitCommandRunUpdated = (payload: unknown) => {
    const handlers = wsHandlers.get("command_run:updated") ?? [];
    for (const handler of handlers) {
      handler(payload);
    }
  };

  it("shows cancel control only for active runs and sends run-scoped cancellation", async () => {
    apiMock.listCommandRuns.mockResolvedValueOnce({
      command_runs: [
        {
          id: "run-1",
          status: "running",
          command: "git status",
          working_directory: "/tmp/ws",
          stdout_truncated: false,
          stderr_truncated: false,
          created_at: "2026-05-29T00:00:00Z",
        },
        {
          id: "run-2",
          status: "completed",
          command: "git diff --stat",
          working_directory: "/tmp/ws",
          stdout_truncated: false,
          stderr_truncated: false,
          created_at: "2026-05-29T00:00:00Z",
        },
      ],
      total: 2,
    });
    apiMock.cancelCommandRun.mockResolvedValue({ status: "cancellation_requested", id: "run-1" });

    render(<CommandDeckPage />, { wrapper: createWrapper() });

    const cancel = await screen.findByRole("button", { name: "Cancel run" });
    fireEvent.click(cancel);

    await waitFor(() => {
      expect(apiMock.cancelCommandRun).toHaveBeenCalledWith("run-1");
    });
    expect(screen.getByText("Cancellation requested.")).toBeInTheDocument();
  });

  it("renders runtime heartbeat health states from server evidence", async () => {
    apiMock.listRuntimes.mockResolvedValue([
      {
        id: "rt-online",
        name: "Online Runtime",
        provider: "codex",
        runtime_mode: "local",
        status: "online",
        health_status: "online",
        last_seen_at: "2026-05-29T00:00:00Z",
      },
      {
        id: "rt-stale",
        name: "Stale Runtime",
        provider: "claude",
        runtime_mode: "local",
        status: "online",
        health_status: "stale",
        last_seen_at: "2026-05-29T00:00:00Z",
      },
      {
        id: "rt-offline",
        name: "Offline Runtime",
        provider: "copilot",
        runtime_mode: "local",
        status: "offline",
        health_status: "offline",
        last_seen_at: "2026-05-29T00:00:00Z",
      },
      {
        id: "rt-unknown",
        name: "Unknown Runtime",
        provider: "gemini",
        runtime_mode: "local",
        status: "offline",
        health_status: "unknown",
        last_seen_at: null,
      },
    ]);

    render(<CommandDeckPage />, { wrapper: createWrapper() });

    expect(await screen.findByText("Runtime Health")).toBeInTheDocument();
    expect(await screen.findByText("Online Runtime")).toBeInTheDocument();
    expect(screen.getByText("Stale Runtime")).toBeInTheDocument();
    expect(screen.getByText("Offline Runtime")).toBeInTheDocument();
    expect(screen.getByText("Unknown Runtime")).toBeInTheDocument();
    expect(screen.getByText("Online")).toBeInTheDocument();
    expect(screen.getByText("Stale")).toBeInTheDocument();
    expect(screen.getByText("Offline")).toBeInTheDocument();
    expect(screen.getByText("Unknown")).toBeInTheDocument();
    expect(screen.getByText("Last seen unknown")).toBeInTheDocument();
  });

  it("keeps execution disabled when no runtime is online", async () => {
    apiMock.listRuntimes.mockResolvedValue([
      {
        id: "rt-offline-only",
        name: "Offline Runtime",
        provider: "codex",
        runtime_mode: "local",
        status: "offline",
        health_status: "offline",
        last_seen_at: "2026-05-29T00:00:00Z",
      },
    ]);

    render(<CommandDeckPage />, { wrapper: createWrapper() });

    expect(await screen.findByText("No online runtimes available. Connect a daemon to execute commands.")).toBeInTheDocument();
  });

  it("applies live command_run:updated events to run history without polling", async () => {
    apiMock.listCommandRuns.mockResolvedValueOnce({
      command_runs: [
        {
          id: "run-live-1",
          status: "pending",
          command: "git status",
          working_directory: "/tmp/ws",
          stdout_truncated: false,
          stderr_truncated: false,
          created_at: "2026-05-29T00:00:00Z",
        },
      ],
      total: 1,
    });

    render(<CommandDeckPage />, { wrapper: createWrapper() });
    expect(await screen.findByText("Pending")).toBeInTheDocument();

    act(() => {
      emitCommandRunUpdated({
        run: {
          id: "run-live-1",
          status: "running",
          command: "git status",
          working_directory: "/tmp/ws",
          stdout_truncated: false,
          stderr_truncated: false,
          created_at: "2026-05-29T00:00:00Z",
        },
      });
    });

    expect(await screen.findByText("Running")).toBeInTheDocument();
  });

  it("ignores malformed command_run:updated payloads", async () => {
    apiMock.listCommandRuns.mockResolvedValueOnce({
      command_runs: [
        {
          id: "run-live-2",
          status: "pending",
          command: "git status",
          working_directory: "/tmp/ws",
          stdout_truncated: false,
          stderr_truncated: false,
          created_at: "2026-05-29T00:00:00Z",
        },
      ],
      total: 1,
    });

    render(<CommandDeckPage />, { wrapper: createWrapper() });
    expect(await screen.findByText("Pending")).toBeInTheDocument();

    act(() => {
      emitCommandRunUpdated({ run: { id: "run-live-2", status: 42 } });
    });

    const runHistory = screen.getByRole("heading", { name: "Run History" }).closest("div");
    if (!runHistory) {
      throw new Error("Run History section not found");
    }
    expect(within(runHistory).queryByText("Running")).not.toBeInTheDocument();
    expect(within(runHistory).getByText("Pending")).toBeInTheDocument();
  });

  it("shows real preview URL and healthy state from the API response", async () => {
    apiMock.listPreviewRegistry.mockResolvedValueOnce({
      previews: [
        {
          id: "self-hosted-web",
          workspace_id: "workspace-1",
          workspace_name: "Acme",
          workspace_slug: "acme",
          runtime_id: null,
          runtime_name: null,
          runtime_status: null,
          machine_identity: null,
          preview_url: "http://localhost:3000",
          port: 3000,
          health_status: "healthy",
          health_status_code: 200,
          last_checked_at: "2026-05-29T00:00:00Z",
          last_success_at: "2026-05-29T00:00:00Z",
          registered_at: "2026-05-28T00:00:00Z",
          updated_at: "2026-05-29T00:00:00Z",
          source: "self_hosted_stack",
        },
      ],
      last_checked_at: "2026-05-29T00:00:00Z",
    });

    render(<CommandDeckPage />, { wrapper: createWrapper() });

    expect(await screen.findByText("Preview Registry")).toBeInTheDocument();
    expect(await screen.findByText("http://localhost:3000")).toBeInTheDocument();
    expect(screen.getByText("Healthy")).toBeInTheDocument();
    expect(screen.getByText("HTTP 200")).toBeInTheDocument();
    expect(screen.getByText("Runtime provenance not yet established")).toBeInTheDocument();
    expect(screen.getByText("Last Successful Check")).toBeInTheDocument();
  });

  it("renders verified runtime provenance and offline runtime truth", async () => {
    apiMock.listRuntimes.mockResolvedValueOnce([
      {
        id: "rt-offline-preview",
        name: "Preview Runtime",
        provider: "codex",
        runtime_mode: "local",
        status: "offline",
        health_status: "offline",
        last_seen_at: "2026-05-29T00:00:00Z",
      },
    ]);
    apiMock.listPreviewRegistry.mockResolvedValueOnce({
      previews: [
        {
          id: "preview-verified-runtime",
          workspace_id: "workspace-1",
          workspace_name: "Acme",
          workspace_slug: "acme",
          runtime_id: "rt-offline-preview",
          runtime_name: "Preview Runtime",
          runtime_status: "offline",
          machine_identity: "daemon-1",
          preview_url: "http://localhost:3000",
          port: 3000,
          health_status: "healthy",
          health_status_code: 200,
          last_checked_at: "2026-05-29T00:00:00Z",
          last_success_at: "2026-05-29T00:00:00Z",
          registered_at: "2026-05-28T00:00:00Z",
          updated_at: "2026-05-29T00:00:00Z",
          source: "self_hosted_stack",
        },
      ],
      last_checked_at: "2026-05-29T00:00:00Z",
    });

    render(<CommandDeckPage />, { wrapper: createWrapper() });

    expect(await screen.findByText("Reported by verified runtime (runtime offline)")).toBeInTheDocument();
  });

  it("renders structured truncation and cancellation evidence in run history", async () => {
    apiMock.listCommandRuns.mockResolvedValueOnce({
      command_runs: [
        {
          id: "run-3",
          status: "cancelled",
          command: "git status",
          working_directory: "/tmp/ws",
          stdout: "line 1",
          stdout_truncated: true,
          stderr_truncated: false,
          cancellation_requested_at: "2026-05-29T00:00:00Z",
          created_at: "2026-05-29T00:00:00Z",
        },
      ],
      total: 1,
    });
    render(<CommandDeckPage />, { wrapper: createWrapper() });
    expect(await screen.findByText("truncated")).toBeInTheDocument();
    expect(screen.getByText(/cancel requested/i)).toBeInTheDocument();
  });

  it("shows a truthful loading state", () => {
    apiMock.listPreviewRegistry.mockReturnValueOnce(new Promise(() => {}));

    render(<CommandDeckPage />, { wrapper: createWrapper() });

    expect(screen.getByText("Checking previews...")).toBeInTheDocument();
  });

  it("shows a truthful empty state", async () => {
    render(<CommandDeckPage />, { wrapper: createWrapper() });

    expect(
      await screen.findByText("No previews are registered for this workspace."),
    ).toBeInTheDocument();
  });

  it("renders safe unavailable messages without raw internal error details", async () => {
    apiMock.listPreviewRegistry.mockResolvedValueOnce({
      previews: [
        {
          id: "self-hosted-web",
          workspace_id: "workspace-1",
          workspace_name: "Acme",
          workspace_slug: "acme",
          preview_url: "http://localhost:3000",
          port: 3000,
          health_status: "unavailable",
          health_message: "Preview is currently unavailable.",
          health_error: "dial tcp 10.0.0.5:3000: connectex: No connection could be made",
          last_checked_at: "2026-05-29T00:00:00Z",
          registered_at: "2026-05-28T00:00:00Z",
          updated_at: "2026-05-29T00:00:00Z",
          source: "self_hosted_stack",
        },
      ],
      last_checked_at: "2026-05-29T00:00:00Z",
    });

    render(<CommandDeckPage />, { wrapper: createWrapper() });

    expect(await screen.findByText("Unavailable")).toBeInTheDocument();
    expect(screen.getByText("Preview is currently unavailable.")).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.queryByText(/10\.0\.0\.5/)).not.toBeInTheDocument();
      expect(screen.queryByText(/dial tcp/)).not.toBeInTheDocument();
    });
  });

  it("refresh action calls only the trusted sync endpoint", async () => {
    apiMock.syncSelfHostedPreviewRegistry.mockResolvedValue({
      previews: [],
      last_checked_at: "2026-05-29T00:00:00Z",
    });

    render(<CommandDeckPage />, { wrapper: createWrapper() });
    const button = await screen.findByRole("button", { name: "Register/Refresh Preview" });
    fireEvent.click(button);

    await waitFor(() => {
      expect(apiMock.syncSelfHostedPreviewRegistry).toHaveBeenCalledTimes(1);
    });
  });

  it("renders stale lifecycle when last check is too old", async () => {
    apiMock.listPreviewRegistry.mockResolvedValueOnce({
      previews: [
        {
          id: "self-hosted-web",
          workspace_id: "workspace-1",
          workspace_name: "Acme",
          workspace_slug: "acme",
          preview_url: "http://localhost:3000",
          port: 3000,
          health_status: "healthy",
          health_status_code: 200,
          last_checked_at: "2026-05-29T00:00:00Z",
          last_success_at: "2026-05-29T00:00:00Z",
          registered_at: "2026-05-28T00:00:00Z",
          updated_at: "2026-05-29T00:00:00Z",
          source: "self_hosted_stack",
        },
      ],
      last_checked_at: "2026-05-29T00:00:00Z",
    });

    render(<CommandDeckPage />, { wrapper: createWrapper() });
    expect(await screen.findByText("Lifecycle")).toBeInTheDocument();
    expect(await screen.findByText("Stale")).toBeInTheDocument();
  });

  it("retires stale preview records through the trusted lifecycle endpoint", async () => {
    apiMock.listPreviewRegistry.mockResolvedValueOnce({
      previews: [
        {
          id: "preview-stale-1",
          workspace_id: "workspace-1",
          workspace_name: "Acme",
          workspace_slug: "acme",
          preview_url: "http://localhost:3000",
          port: 3000,
          health_status: "healthy",
          lifecycle_status: "stale",
          health_status_code: 200,
          last_checked_at: "2026-05-29T00:00:00Z",
          last_success_at: "2026-05-29T00:00:00Z",
          registered_at: "2026-05-28T00:00:00Z",
          updated_at: "2026-05-29T00:00:00Z",
          source: "self_hosted_stack",
        },
      ],
      last_checked_at: "2026-05-29T00:00:00Z",
    });

    render(<CommandDeckPage />, { wrapper: createWrapper() });

    const retireButton = await screen.findByRole("button", { name: "Retire Preview" });
    fireEvent.click(retireButton);

    await waitFor(() => {
      expect(apiMock.retirePreviewRegistryEntry).toHaveBeenCalledWith("preview-stale-1");
    });
  });

  it("shows workflow empty state without fake records", async () => {
    render(<CommandDeckPage />, { wrapper: createWrapper() });

    expect(await screen.findByText("Workflow Execution Records")).toBeInTheDocument();
    expect(await screen.findByText("No workflow execution records exist for this workspace.")).toBeInTheDocument();
  });

  it("creates a workflow record with optional command run evidence", async () => {
    apiMock.listCommandRuns.mockResolvedValueOnce({
      command_runs: [
        {
          id: "run-link-1",
          status: "completed",
          command: "git status",
          working_directory: "/tmp/ws",
          stdout_truncated: false,
          stderr_truncated: false,
          created_at: "2026-05-29T00:00:00Z",
        },
      ],
      total: 1,
    });

    render(<CommandDeckPage />, { wrapper: createWrapper() });

    fireEvent.change(await screen.findByLabelText("Workflow Title"), {
      target: { value: "Preview rollout" },
    });
    fireEvent.change(screen.getByLabelText("Objective"), {
      target: { value: "Track launch and verification steps" },
    });
    fireEvent.change(screen.getByLabelText("Command Run Evidence (optional)"), {
      target: { value: "run-link-1" },
    });

    fireEvent.click(screen.getByRole("button", { name: "Create Workflow Record" }));

    await waitFor(() => {
      expect(apiMock.createCommandWorkflowExecution).toHaveBeenCalledWith({
        title: "Preview rollout",
        objective: "Track launch and verification steps",
        status: "planned",
        command_run_id: "run-link-1",
      });
    });
  });

  it("updates workflow lifecycle status through the trusted status endpoint", async () => {
    apiMock.listCommandWorkflowExecutions.mockResolvedValueOnce({
      workflow_executions: [
        {
          id: "wf-progress-1",
          workspace_id: "workspace-1",
          title: "Preview lifecycle control",
          objective: "Move from planned to running",
          status: "planned",
          created_by_type: "member",
          created_by_id: "user-1",
          created_at: "2026-05-29T00:00:00Z",
          updated_at: "2026-05-29T00:00:00Z",
        },
      ],
      total: 1,
    });

    render(<CommandDeckPage />, { wrapper: createWrapper() });
    const transition = await screen.findByRole("button", { name: "Move to Running" });
    fireEvent.click(transition);

    await waitFor(() => {
      expect(apiMock.updateCommandWorkflowExecutionStatus).toHaveBeenCalledWith("wf-progress-1", "running");
    });
  });
});
