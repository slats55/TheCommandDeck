import { describe, expect, it } from "vitest";
import {
  aggregateAgentTokens,
  aggregateDailyCost,
  computeDailyTotals,
  formatDuration,
} from "./utils";

describe("aggregateDailyCost", () => {
  it("collapses multiple rows per day into one stack and sorts by date asc", () => {
    const result = aggregateDailyCost([
      {
        date: "2026-05-10",
        model: "claude-sonnet-4-6",
        input_tokens: 1_000_000,
        output_tokens: 500_000,
        cache_read_tokens: 0,
        cache_write_tokens: 0,
        task_count: 3,
      },
      {
        date: "2026-05-09",
        model: "claude-sonnet-4-6",
        input_tokens: 1_000_000,
        output_tokens: 0,
        cache_read_tokens: 0,
        cache_write_tokens: 0,
        task_count: 1,
      },
    ]);

    // Sort: oldest day first.
    expect(result.map((r) => r.date)).toEqual(["2026-05-09", "2026-05-10"]);
    // claude-sonnet-4-6: input $3/M, output $15/M.
    // 2026-05-09 → 1M input × $3 = $3 input, $0 output, $0 cache.
    expect(result[0]).toMatchObject({ input: 3, output: 0, cacheWrite: 0, total: 3 });
    // 2026-05-10 → $3 input + (0.5M × $15) = $7.5 output. Total $10.5.
    expect(result[1]).toMatchObject({ input: 3, output: 7.5, cacheWrite: 0, total: 10.5 });
  });

  it("treats unmapped models as zero-cost", () => {
    const result = aggregateDailyCost([
      {
        date: "2026-05-10",
        model: "made-up-model",
        input_tokens: 999_999_999,
        output_tokens: 0,
        cache_read_tokens: 0,
        cache_write_tokens: 0,
        task_count: 0,
      },
    ]);
    expect(result[0]?.total).toBe(0);
  });
});

describe("aggregateAgentTokens", () => {
  it("folds per-(agent, model) rows into per-agent totals and sorts by cost desc", () => {
    const rows = aggregateAgentTokens([
      {
        agent_id: "small-spender",
        model: "claude-sonnet-4-6",
        input_tokens: 100_000,
        output_tokens: 0,
        cache_read_tokens: 0,
        cache_write_tokens: 0,
        task_count: 1,
      },
      {
        agent_id: "big-spender",
        model: "claude-sonnet-4-6",
        input_tokens: 5_000_000,
        output_tokens: 0,
        cache_read_tokens: 0,
        cache_write_tokens: 0,
        task_count: 3,
      },
      {
        agent_id: "big-spender",
        model: "claude-haiku-4-5",
        input_tokens: 1_000_000,
        output_tokens: 0,
        cache_read_tokens: 0,
        cache_write_tokens: 0,
        task_count: 2,
      },
    ]);

    expect(rows.map((r) => r.agentId)).toEqual(["big-spender", "small-spender"]);
    expect(rows[0]?.taskCount).toBe(5);
    // big-spender across two models — verify cost > small-spender's.
    expect(rows[0]!.cost).toBeGreaterThan(rows[1]!.cost);
  });
});

describe("computeDailyTotals", () => {
  it("sums tokens across rows and adds estimated cost", () => {
    const totals = computeDailyTotals([
      {
        date: "2026-05-10",
        model: "claude-sonnet-4-6",
        input_tokens: 1_000_000,
        output_tokens: 0,
        cache_read_tokens: 0,
        cache_write_tokens: 0,
        task_count: 2,
      },
      {
        date: "2026-05-09",
        model: "claude-sonnet-4-6",
        input_tokens: 2_000_000,
        output_tokens: 0,
        cache_read_tokens: 0,
        cache_write_tokens: 0,
        task_count: 3,
      },
    ]);
    expect(totals.input).toBe(3_000_000);
    expect(totals.cost).toBe(9); // 3M × $3/M
    expect(totals.taskCount).toBe(5);
  });
});

describe("formatDuration", () => {
  it("formats seconds-only durations", () => {
    expect(formatDuration(45, "<1m")).toBe("45s");
  });
  it("formats minutes and seconds when under one hour", () => {
    expect(formatDuration(150, "<1m")).toBe("2m 30s");
    expect(formatDuration(60, "<1m")).toBe("1m");
  });
  it("formats hours and minutes when under one day", () => {
    expect(formatDuration(3 * 3600 + 17 * 60, "<1m")).toBe("3h 17m");
    expect(formatDuration(3600, "<1m")).toBe("1h");
  });
  it("formats days and hours when more than 24 hours", () => {
    expect(formatDuration(2 * 86400 + 5 * 3600, "<1m")).toBe("2d 5h");
  });
  it("falls back to the supplied label for sub-second durations", () => {
    expect(formatDuration(0, "<1m")).toBe("<1m");
    expect(formatDuration(0.4, "<1m")).toBe("<1m");
  });
});
