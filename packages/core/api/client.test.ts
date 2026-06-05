import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiClient, ApiError } from "./client";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("ApiClient", () => {
  it("preserves HTTP status on failed requests", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ error: "workspace slug already exists" }), {
          status: 409,
          statusText: "Conflict",
          headers: { "Content-Type": "application/json" },
        }),
      ),
    );

    const client = new ApiClient("https://api.example.test");

    try {
      await client.createWorkspace({ name: "Test", slug: "test" });
      throw new Error("expected createWorkspace to fail");
    } catch (error) {
      expect(error).toBeInstanceOf(ApiError);
      expect(error).toMatchObject({
        message: "workspace slug already exists",
        status: 409,
        statusText: "Conflict",
      });
    }
  });

  it("logs an expected 401 at warn, not error, and still throws ApiError", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ error: "missing authorization" }), {
          status: 401,
          statusText: "Unauthorized",
          headers: { "Content-Type": "application/json" },
        }),
      ),
    );

    const logger = {
      debug: vi.fn(),
      info: vi.fn(),
      warn: vi.fn(),
      error: vi.fn(),
    };
    const client = new ApiClient("https://api.example.test", { logger });

    // The pre-login session probe hits these unauthenticated on every cold
    // load; an expected 401 must not be logged as a red error.
    await expect(client.getMe()).rejects.toMatchObject({ status: 401 });

    expect(logger.error).not.toHaveBeenCalled();
    expect(logger.warn).toHaveBeenCalledTimes(1);
    expect(logger.warn.mock.calls[0]![0]).toContain("401");
  });

  it("logs a 500 at error level", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ error: "boom" }), {
          status: 500,
          statusText: "Internal Server Error",
          headers: { "Content-Type": "application/json" },
        }),
      ),
    );

    const logger = {
      debug: vi.fn(),
      info: vi.fn(),
      warn: vi.fn(),
      error: vi.fn(),
    };
    const client = new ApiClient("https://api.example.test", { logger });

    await expect(client.getMe()).rejects.toMatchObject({ status: 500 });

    expect(logger.error).toHaveBeenCalledTimes(1);
    expect(logger.error.mock.calls[0]![0]).toContain("500");
  });

  it("uses the expected HTTP contract for autopilot endpoints", async () => {
    const fetchMock = vi.fn().mockImplementation(() => Promise.resolve(
      new Response(JSON.stringify({ autopilots: [], runs: [], total: 0 }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    ));
    vi.stubGlobal("fetch", fetchMock);

    const client = new ApiClient("https://api.example.test");

    await client.listAutopilots({ status: "active" });
    await client.getAutopilot("ap-1");
    await client.createAutopilot({
      title: "Daily triage",
      assignee_id: "agent-1",
      execution_mode: "create_issue",
    });
    await client.updateAutopilot("ap-1", { status: "paused" });
    await client.deleteAutopilot("ap-1");
    await client.triggerAutopilot("ap-1");
    await client.listAutopilotRuns("ap-1", { limit: 10, offset: 20 });
    await client.createAutopilotTrigger("ap-1", {
      kind: "schedule",
      cron_expression: "0 9 * * *",
      timezone: "UTC",
    });
    await client.updateAutopilotTrigger("ap-1", "tr-1", { enabled: false });
    await client.deleteAutopilotTrigger("ap-1", "tr-1");

    const calls = fetchMock.mock.calls.map(([url, init]) => ({
      url,
      method: init?.method ?? "GET",
      body: init?.body,
    }));

    expect(calls).toMatchObject([
      { url: "https://api.example.test/api/autopilots?status=active", method: "GET" },
      { url: "https://api.example.test/api/autopilots/ap-1", method: "GET" },
      {
        url: "https://api.example.test/api/autopilots",
        method: "POST",
        body: JSON.stringify({
          title: "Daily triage",
          assignee_id: "agent-1",
          execution_mode: "create_issue",
        }),
      },
      {
        url: "https://api.example.test/api/autopilots/ap-1",
        method: "PATCH",
        body: JSON.stringify({ status: "paused" }),
      },
      { url: "https://api.example.test/api/autopilots/ap-1", method: "DELETE" },
      { url: "https://api.example.test/api/autopilots/ap-1/trigger", method: "POST" },
      { url: "https://api.example.test/api/autopilots/ap-1/runs?limit=10&offset=20", method: "GET" },
      {
        url: "https://api.example.test/api/autopilots/ap-1/triggers",
        method: "POST",
        body: JSON.stringify({
          kind: "schedule",
          cron_expression: "0 9 * * *",
          timezone: "UTC",
        }),
      },
      {
        url: "https://api.example.test/api/autopilots/ap-1/triggers/tr-1",
        method: "PATCH",
        body: JSON.stringify({ enabled: false }),
      },
      { url: "https://api.example.test/api/autopilots/ap-1/triggers/tr-1", method: "DELETE" },
    ]);
  });

  it("emits X-Client-* headers when identity is configured", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify([]), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const client = new ApiClient("https://api.example.test", {
      identity: { platform: "desktop", version: "1.2.3", os: "macos" },
    });
    await client.listWorkspaces();

    const headers = fetchMock.mock.calls[0]![1]!.headers as Record<string, string>;
    expect(headers["X-Client-Platform"]).toBe("desktop");
    expect(headers["X-Client-Version"]).toBe("1.2.3");
    expect(headers["X-Client-OS"]).toBe("macos");
  });

  it("omits X-Client-* headers when identity is not configured", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify([]), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const client = new ApiClient("https://api.example.test");
    await client.listWorkspaces();

    const headers = fetchMock.mock.calls[0]![1]!.headers as Record<string, string>;
    expect(headers["X-Client-Platform"]).toBeUndefined();
    expect(headers["X-Client-Version"]).toBeUndefined();
    expect(headers["X-Client-OS"]).toBeUndefined();
  });

  it("uses a fixed server-derived endpoint for preview registry", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ previews: [], last_checked_at: "2026-05-29T00:00:00Z" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const client = new ApiClient("https://api.example.test");
    await client.listPreviewRegistry();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0]!;
    expect(url).toBe("https://api.example.test/api/commandrunner/previews");
    expect(init?.method ?? "GET").toBe("GET");
    expect(init?.body).toBeUndefined();
  });

  it("uses a fixed trusted sync endpoint for preview registry refresh", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ previews: [], last_checked_at: "2026-05-29T00:00:00Z" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const client = new ApiClient("https://api.example.test");
    await client.syncSelfHostedPreviewRegistry();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0]!;
    expect(url).toBe("https://api.example.test/api/commandrunner/previews/self-hosted/sync");
    expect(init?.method).toBe("POST");
    expect(init?.body).toBeUndefined();
  });

  it("falls back to an empty preview registry response for malformed data", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ previews: [{ id: "bad" }] }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      ),
    );

    const client = new ApiClient("https://api.example.test");
    const result = await client.listPreviewRegistry();

    expect(result).toEqual({ previews: [], last_checked_at: "" });
  });

  it("posts run cancellation using the fixed commandrunner endpoint", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ status: "cancellation_requested", id: "run-1" }), {
        status: 202,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const client = new ApiClient("https://api.example.test");
    const result = await client.cancelCommandRun("run-1");

    expect(result).toEqual({ status: "cancellation_requested", id: "run-1" });
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0]!;
    expect(url).toBe("https://api.example.test/api/commandrunner/run/run-1/cancel");
    expect(init?.method).toBe("POST");
  });

  it("uses workspace-scoped command workflow execution endpoints", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ workflow_executions: [], total: 0 }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({
          id: "wf-1",
          workspace_id: "ws-1",
          title: "Preview lifecycle",
          objective: "Track rollout",
          status: "planned",
          created_by_type: "member",
          created_by_id: "u-1",
          created_at: "2026-05-30T00:00:00Z",
          updated_at: "2026-05-30T00:00:00Z",
        }), {
          status: 201,
          headers: { "Content-Type": "application/json" },
        }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({
          id: "wf-1",
          workspace_id: "ws-1",
          title: "Preview lifecycle",
          objective: "Track rollout",
          status: "running",
          created_by_type: "member",
          created_by_id: "u-1",
          created_at: "2026-05-30T00:00:00Z",
          updated_at: "2026-05-30T00:01:00Z",
        }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      );
    vi.stubGlobal("fetch", fetchMock);

    const client = new ApiClient("https://api.example.test");
    await client.listCommandWorkflowExecutions();
    await client.createCommandWorkflowExecution({
      title: "Preview lifecycle",
      objective: "Track rollout",
    });
    await client.updateCommandWorkflowExecutionStatus("wf-1", "running");

    const calls = fetchMock.mock.calls.map(([url, init]) => ({
      url,
      method: init?.method ?? "GET",
      body: init?.body,
    }));
    expect(calls).toMatchObject([
      {
        url: "https://api.example.test/api/commandrunner/workflows",
        method: "GET",
      },
      {
        url: "https://api.example.test/api/commandrunner/workflows",
        method: "POST",
        body: JSON.stringify({
          title: "Preview lifecycle",
          objective: "Track rollout",
        }),
      },
      {
        url: "https://api.example.test/api/commandrunner/workflows/wf-1/status",
        method: "PATCH",
        body: JSON.stringify({ status: "running" }),
      },
    ]);
  });

  it("falls back to empty workflow execution list for malformed data", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ workflow_executions: [{ id: "bad" }] }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      ),
    );

    const client = new ApiClient("https://api.example.test");
    const result = await client.listCommandWorkflowExecutions();

    expect(result).toEqual({ workflow_executions: [], total: 0 });
  });

  describe("getAttachment", () => {
    it("returns the parsed attachment for a well-formed response", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue(
          new Response(
            JSON.stringify({
              id: "att-1",
              workspace_id: "ws-1",
              issue_id: null,
              comment_id: null,
              uploader_type: "member",
              uploader_id: "u-1",
              filename: "report.md",
              url: "https://static.example.test/ws/att-1.md",
              download_url:
                "https://static.example.test/ws/att-1.md?Policy=p&Signature=s&Key-Pair-Id=k",
              content_type: "text/markdown",
              size_bytes: 123,
              created_at: "2026-05-11T00:00:00Z",
            }),
            { status: 200, headers: { "Content-Type": "application/json" } },
          ),
        ),
      );

      const client = new ApiClient("https://api.example.test");
      const att = await client.getAttachment("att-1");

      expect(att.id).toBe("att-1");
      expect(att.download_url).toContain("Policy=");
    });

    it("falls back to an empty attachment when the response is missing download_url", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue(
          new Response(JSON.stringify({ id: "att-1" }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        ),
      );

      const client = new ApiClient("https://api.example.test");
      const att = await client.getAttachment("att-1");

      // parseWithFallback returns the EMPTY_ATTACHMENT record so callers can
      // safely read `download_url` without crashing — they'll see "" and
      // surface a user-facing error instead of opening `undefined`.
      expect(att.id).toBe("");
      expect(att.download_url).toBe("");
    });
  });

  describe("chat attachment wiring", () => {
    it("uploadFile includes chat_session_id in the FormData body", async () => {
      const fetchMock = vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ id: "att-1", url: "https://cdn/x" }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      );
      vi.stubGlobal("fetch", fetchMock);

      const client = new ApiClient("https://api.example.test");
      const file = new File(["hi"], "hi.png", { type: "image/png" });
      await client.uploadFile(file, { chatSessionId: "session-123" });

      expect(fetchMock).toHaveBeenCalledTimes(1);
      const [url, init] = fetchMock.mock.calls[0]!;
      expect(url).toBe("https://api.example.test/api/upload-file");
      expect(init?.method).toBe("POST");
      const body = init?.body as FormData;
      expect(body).toBeInstanceOf(FormData);
      expect(body.get("chat_session_id")).toBe("session-123");
      expect(body.get("issue_id")).toBeNull();
      expect(body.get("comment_id")).toBeNull();
    });

    it("sendChatMessage serialises attachment_ids onto the JSON body when present", async () => {
      const fetchMock = vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ message_id: "m1", task_id: "t1", created_at: "" }), {
          status: 201,
          headers: { "Content-Type": "application/json" },
        }),
      );
      vi.stubGlobal("fetch", fetchMock);

      const client = new ApiClient("https://api.example.test");
      await client.sendChatMessage("session-1", "hello", ["att-1", "att-2"]);

      const [, init] = fetchMock.mock.calls[0]!;
      expect(JSON.parse(init?.body as string)).toEqual({
        content: "hello",
        attachment_ids: ["att-1", "att-2"],
      });
    });

    it("sendChatMessage omits attachment_ids when the list is empty or undefined", async () => {
      const fetchMock = vi.fn().mockImplementation(() =>
        Promise.resolve(
          new Response(JSON.stringify({ message_id: "m1", task_id: "t1", created_at: "" }), {
            status: 201,
            headers: { "Content-Type": "application/json" },
          }),
        ),
      );
      vi.stubGlobal("fetch", fetchMock);

      const client = new ApiClient("https://api.example.test");
      await client.sendChatMessage("session-1", "hello");
      await client.sendChatMessage("session-1", "again", []);

      expect(JSON.parse(fetchMock.mock.calls[0]![1]?.body as string)).toEqual({ content: "hello" });
      expect(JSON.parse(fetchMock.mock.calls[1]![1]?.body as string)).toEqual({ content: "again" });
    });
  });
});
