import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";

const { apiMock } = vi.hoisted(() => ({
  apiMock: {
    listCommandTemplates: vi.fn(),
    listRuntimes: vi.fn(),
    listCommandRuns: vi.fn(),
    listPreviewRegistry: vi.fn(),
    syncSelfHostedPreviewRegistry: vi.fn(),
    runCommand: vi.fn(),
  },
}));

vi.mock("@multica/core/api", () => ({
  api: apiMock,
}));

vi.mock("@multica/core", () => ({
  useWorkspaceId: () => "workspace-1",
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
    apiMock.listCommandTemplates.mockResolvedValue({ templates: [] });
    apiMock.listRuntimes.mockResolvedValue([]);
    apiMock.listCommandRuns.mockResolvedValue({ command_runs: [], total: 0 });
    apiMock.listPreviewRegistry.mockResolvedValue({
      previews: [],
      last_checked_at: "2026-05-29T00:00:00Z",
    });
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
    expect(screen.getByText("Unlinked (self-hosted preview)")).toBeInTheDocument();
    expect(screen.getByText("Last Successful Check")).toBeInTheDocument();
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
});
