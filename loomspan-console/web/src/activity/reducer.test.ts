import { describe, expect, it } from "vitest";
import {
  activityReducer,
  initialActivityState,
  type ActivityAction,
  type ActivityState,
} from "./reducer";
import { BrowserAPIError } from "../api/client";
import type { Activity } from "../api/contracts";

function makeActivity(cursor: string, kind: Activity["kind"] = "STEP_COMPLETED"): Activity {
  return {
    instanceId: "11111111-1111-4111-8111-111111111111",
    cursor,
    sessionId: "session-1",
    traceId: "trace-1",
    canonicalSequence: parseInt(cursor, 10),
    timestamp: "2026-07-25T12:00:00Z",
    kind,
    executionStatus: "COMPLETED",
    summary: "test",
    details: {},
  };
}

describe("activityReducer", () => {
  it("returns initial state on reset", () => {
    const state: ActivityState = {
      ...initialActivityState,
      activities: [makeActivity("1")],
      connected: true,
      error: null,
      loading: true,
    };
    expect(activityReducer(state, { type: "reset" })).toEqual(initialActivityState);
  });

  it("sets loading on recent-loading", () => {
    const state = activityReducer(initialActivityState, { type: "recent-loading" });
    expect(state.loading).toBe(true);
    expect(state.error).toBeNull();
  });

  it("stores items on recent-success without append", () => {
    const items = [makeActivity("1"), makeActivity("2")];
    const state = activityReducer(initialActivityState, {
      type: "recent-success",
      items,
      hasMore: false,
      nextCursor: "",
      append: false,
      coverage: {},
    });
    expect(state.activities).toEqual(items);
    expect(state.loading).toBe(false);
  });

  it("appends items on recent-success with append", () => {
    const existing = [makeActivity("1")];
    const state: ActivityState = {
      ...initialActivityState,
      activities: existing,
    };
    const newItems = [makeActivity("2"), makeActivity("3")];
    const result = activityReducer(state, {
      type: "recent-success",
      items: newItems,
      hasMore: false,
      nextCursor: "",
      append: true,
      coverage: {},
    });
    expect(result.activities).toEqual([...existing, ...newItems]);
  });

  it("keeps terminal activities in activities and routes to recentCompletions on recent-success", () => {
    const items = [makeActivity("1"), makeActivity("2", "TRACE_COMPLETED")];
    const state = activityReducer(initialActivityState, {
      type: "recent-success",
      items,
      hasMore: false,
      nextCursor: "",
      append: false,
      coverage: {},
    });
    expect(state.activities).toEqual(items);
    expect(state.recentCompletions).toEqual([makeActivity("2", "TRACE_COMPLETED")]);
  });

  it("sets error on recent-error", () => {
    const error = new BrowserAPIError("CONSOLE_ERROR", "fail", 500);
    const state = activityReducer(initialActivityState, { type: "recent-error", error });
    expect(state.loading).toBe(false);
    expect(state.error).toBe(error);
  });

  it("sets connected on stream-connected", () => {
    const state = activityReducer(initialActivityState, { type: "stream-connected" });
    expect(state.connected).toBe(true);
    expect(state.reconnectAttempt).toBe(0);
  });

  it("updates connection fact on stream-connection", () => {
    const fact = { connected: false, reason: "relay_frame_limit" };
    const state = activityReducer(initialActivityState, { type: "stream-connection", fact });
    expect(state.connected).toBe(false);
    expect(state.connectionFact).toEqual(fact);
  });

  it("appends activity on stream-activity", () => {
    const activity = makeActivity("7");
    const state = activityReducer(initialActivityState, {
      type: "stream-activity",
      activity,
    });
    expect(state.activities).toEqual([activity]);
    expect(state.lastCursor).toBe("7");
  });

  it("deduplicates streamed replay by interval cursor", () => {
    const first = activityReducer(initialActivityState, {
      type: "stream-activity",
      activity: makeActivity("1"),
    });
    const replayed = activityReducer(first, {
      type: "stream-activity",
      activity: makeActivity("1"),
    });
    expect(replayed.activities).toHaveLength(1);
  });

  it("clears the old interval before accepting reset replay", () => {
    const first = activityReducer(initialActivityState, {
      type: "stream-activity",
      activity: makeActivity("1"),
    });
    const reset = activityReducer(first, {
      type: "stream-continuity",
      continuity: {
        intervalId: "interval-2",
        targetScopeId: "scope-2",
        instanceId: "22222222-2222-4222-8222-222222222222",
        reset: { cause: "instance_changed", timestamp: "2026-07-25T12:01:00Z" },
      },
    });
    expect(reset.activities).toEqual([]);
    expect(reset.continuity?.instanceId).toBe("22222222-2222-4222-8222-222222222222");
  });

  it("routes terminal activities to recentCompletions", () => {
    const activity = makeActivity("7", "TRACE_COMPLETED");
    const state = activityReducer(initialActivityState, {
      type: "stream-activity",
      activity,
    });
    expect(state.activities).toEqual([activity]);
    expect(state.recentCompletions).toEqual([activity]);
  });

  it("keeps one recent completion per execution and replaces it with observation-ended", () => {
    const completed = makeActivity("7", "TRACE_COMPLETED");
    const observationEnded = {
      ...makeActivity("8", "EXECUTION_OBSERVATION_ENDED"),
      details: { applicationTraceAvailability: "CORE_FINALIZATION_FAILED" },
    };
    const first = activityReducer(initialActivityState, {
      type: "stream-activity",
      activity: completed,
    });
    const second = activityReducer(first, {
      type: "stream-activity",
      activity: observationEnded,
    });
    expect(second.recentCompletions).toEqual([observationEnded]);
  });

  it("evicts oldest when exceeding maxActivities", () => {
    let state = initialActivityState;
    for (let i = 0; i < 257; i++) {
      state = activityReducer(state, {
        type: "stream-activity",
        activity: makeActivity(String(i)),
      });
    }
    expect(state.activities).toHaveLength(256);
    expect(state.activities[0].cursor).toBe("1");
    expect(state.activities[255].cursor).toBe("256");
  });

  it("sets disconnected and increments reconnectAttempt on stream-closed", () => {
    const state: ActivityState = {
      ...initialActivityState,
      connected: true,
    };
    const result = activityReducer(state, { type: "stream-closed" });
    expect(result.connected).toBe(false);
    expect(result.connectionFact).toEqual({ connected: false, reason: "stream_closed" });
    expect(result.reconnectAttempt).toBe(1);
  });

  it("sets error, disconnected, and increments reconnectAttempt on stream-error", () => {
    const error = new BrowserAPIError("CONSOLE_ERROR", "stream failed", 0);
    const state: ActivityState = {
      ...initialActivityState,
      connected: true,
    };
    const result = activityReducer(state, { type: "stream-error", error });
    expect(result.connected).toBe(false);
    expect(result.error).toBe(error);
    expect(result.reconnectAttempt).toBe(1);
  });

  it("stores continuity on stream-continuity", () => {
    const continuity = {
      intervalId: "interval-1",
      targetScopeId: "scope-1",
      instanceId: "11111111-1111-4111-8111-111111111111",
      firstCursor: "1",
      lastCursor: "10",
      observedAt: "2026-07-25T12:00:00Z",
    };
    const state = activityReducer(initialActivityState, {
      type: "stream-continuity",
      continuity,
    });
    expect(state.continuity).toEqual(continuity);
  });

  it("does not clear activity when a reconnect repeats the current interval reset fact", () => {
    const continuity = {
      intervalId: "interval-2",
      targetScopeId: "scope-1",
      instanceId: "11111111-1111-4111-8111-111111111111",
      reset: { cause: "upstream_stale_cursor" as const, timestamp: "2026-07-25T12:01:00Z" },
    };
    const withContinuity = activityReducer(initialActivityState, {
      type: "stream-continuity",
      continuity,
    });
    const withActivity = activityReducer(withContinuity, {
      type: "stream-activity",
      activity: makeActivity("7"),
    });
    const reconnected = activityReducer(withActivity, {
      type: "stream-continuity",
      continuity,
    });
    expect(reconnected.activities).toEqual([makeActivity("7")]);
    expect(reconnected.lastCursor).toBe("7");
  });

  it("sets disconnected on stream-replay-gap", () => {
    const state: ActivityState = {
      ...initialActivityState,
      connected: true,
    };
    const result = activityReducer(state, {
      type: "stream-replay-gap",
      reason: "relay_frame_limit",
    });
    expect(result.connected).toBe(false);
    expect(result.connectionFact).toEqual({
      connected: false,
      reason: "relay_frame_limit",
    });
  });

  it("resets reconnectAttempt to 0 on stream-connected after error", () => {
    const state: ActivityState = {
      ...initialActivityState,
      connected: false,
      reconnectAttempt: 3,
    };
    const result = activityReducer(state, { type: "stream-connected" });
    expect(result.connected).toBe(true);
    expect(result.reconnectAttempt).toBe(0);
  });

  it("updates baselineObservedAt on baseline-refreshed", () => {
    const state = activityReducer(initialActivityState, {
      type: "baseline-refreshed",
      observedAt: "2026-07-25T12:01:00Z",
    });
    expect(state.baselineObservedAt).toBe("2026-07-25T12:01:00Z");
  });

  it("deduplicates by cursor on recent-success with append", () => {
    const existing = [makeActivity("1"), makeActivity("2")];
    const state: ActivityState = {
      ...initialActivityState,
      activities: existing,
    };
    const result = activityReducer(state, {
      type: "recent-success",
      items: [makeActivity("2"), makeActivity("3")],
      hasMore: false,
      nextCursor: "",
      append: true,
      coverage: {},
    });
    expect(result.activities).toEqual([makeActivity("1"), makeActivity("2"), makeActivity("3")]);
  });

  it("finite resync clears activity and completions from a previous continuity interval", () => {
    const terminal = {
      ...makeActivity("2"),
      kind: "TRACE_COMPLETED" as const,
      executionStatus: "COMPLETED",
    };
    const state: ActivityState = {
      ...initialActivityState,
      activities: [makeActivity("1"), terminal],
      recentCompletions: [terminal],
      continuity: {
        intervalId: "interval-old",
        targetScopeId: "scope-1",
        instanceId: "11111111-1111-4111-8111-111111111111",
      },
      lastCursor: "2",
    };
    const replacement = { ...makeActivity("1"), sessionId: "session-new" };
    const result = activityReducer(state, {
      type: "recent-success",
      items: [replacement],
      hasMore: false,
      nextCursor: "",
      append: false,
      continuity: {
        intervalId: "interval-new",
        targetScopeId: "scope-2",
        instanceId: "22222222-2222-4222-8222-222222222222",
      },
      coverage: { globalEvictedThroughCursor: "8" },
    });
    expect(result.activities).toEqual([replacement]);
    expect(result.recentCompletions).toEqual([]);
    expect(result.lastCursor).toBe("1");
  });
});
