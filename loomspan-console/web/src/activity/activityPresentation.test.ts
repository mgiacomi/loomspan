import { describe, expect, it } from "vitest";
import {
  presentActivity,
  formatDateTime,
  formatTimestamp,
  formatElapsed,
  formatDelta,
} from "./activityPresentation";
import type { Activity, ActivityKind } from "../api/contracts";
import { ACTIVITY_KIND_LABELS } from "../api/contracts";

function makeActivity(
  kind: ActivityKind,
  details?: Record<string, unknown>,
  frame?: { route?: string; frameId?: string },
): Activity {
  return {
    instanceId: "11111111-1111-4111-8111-111111111111",
    cursor: "1",
    sessionId: "session-1",
    traceId: "trace-1",
    canonicalSequence: 1,
    timestamp: "2026-07-25T12:00:00Z",
    kind,
    executionStatus: "RUNNING",
    summary: "test",
    details: details ?? {},
    ...frame,
  };
}

function factValue(activity: Activity, label: string): string | undefined {
  return presentActivity(activity).facts.find((fact) => fact.label === label)?.value;
}

const ALL_KINDS: ActivityKind[] = [
  "TRACE_STARTED",
  "FRAME_OPENED",
  "FRAME_CLOSED",
  "MODEL_REQUEST_SENT",
  "MODEL_RESPONSE_RECEIVED",
  "MODEL_ATTEMPT_FAILED",
  "PLAN_CREATED",
  "PLAN_UPDATED",
  "PLAN_VALIDATION_FAILED",
  "PLAN_RETRY_REQUESTED",
  "TOOL_CALL_STARTED",
  "TOOL_CALL_COMPLETED",
  "TOOL_CALL_FAILED",
  "STEP_STARTED",
  "STEP_ACTION_REJECTED",
  "STEP_COMPLETED",
  "ERROR_RECORDED",
  "TRACE_COMPLETED",
  "EXECUTION_OBSERVATION_ENDED",
];

describe("presentActivity", () => {
  it("each activity kind has a distinct concise label", () => {
    const labels = new Set<string>();
    for (const kind of ALL_KINDS) {
      const p = presentActivity(makeActivity(kind));
      expect(p.label).toBe(ACTIVITY_KIND_LABELS[kind]);
      labels.add(p.label);
    }
    expect(labels.size).toBe(ALL_KINDS.length);
  });

  it("classifies terminal kinds", () => {
    expect(presentActivity(makeActivity("TRACE_COMPLETED")).isTerminal).toBe(true);
    expect(presentActivity(makeActivity("EXECUTION_OBSERVATION_ENDED")).isTerminal).toBe(true);
    expect(presentActivity(makeActivity("STEP_COMPLETED")).isTerminal).toBe(false);
  });

  it("classifies error kinds", () => {
    expect(presentActivity(makeActivity("ERROR_RECORDED")).isError).toBe(true);
    expect(presentActivity(makeActivity("STEP_ACTION_REJECTED")).isError).toBe(true);
    expect(presentActivity(makeActivity("PLAN_VALIDATION_FAILED")).isError).toBe(true);
    expect(presentActivity(makeActivity("TOOL_CALL_FAILED")).isError).toBe(true);
    expect(presentActivity(makeActivity("MODEL_ATTEMPT_FAILED")).isError).toBe(true);
    expect(presentActivity(makeActivity("STEP_COMPLETED")).isError).toBe(false);
  });

  it("classifies frame boundary kinds", () => {
    expect(presentActivity(makeActivity("FRAME_OPENED")).isFrameBoundary).toBe(true);
    expect(presentActivity(makeActivity("FRAME_CLOSED")).isFrameBoundary).toBe(true);
    expect(presentActivity(makeActivity("STEP_STARTED")).isFrameBoundary).toBe(false);
  });

  it("extracts outcome from TRACE_COMPLETED details", () => {
    const p = presentActivity(makeActivity("TRACE_COMPLETED", { outcome: "succeeded" }));
    expect(p.outcome).toBe("succeeded");
  });

  it("defaults outcome to completed when not specified", () => {
    const p = presentActivity(makeActivity("TRACE_COMPLETED"));
    expect(p.outcome).toBe("completed");
  });

  it("reports artifact availability for TRACE_COMPLETED", () => {
    const p = presentActivity(makeActivity("TRACE_COMPLETED", { applicationTraceAvailability: "AVAILABLE" }));
    expect(p.artifactAvailable).toBe(true);
  });

  it("does not report artifact availability for non-terminal kinds", () => {
    const p = presentActivity(makeActivity("STEP_COMPLETED", { applicationTraceAvailability: "AVAILABLE" }));
    expect(p.artifactAvailable).toBe(false);
  });

  it("EXECUTION_OBSERVATION_ENDED has no invented outcome", () => {
    const p = presentActivity(makeActivity("EXECUTION_OBSERVATION_ENDED"));
    expect(p.outcome).toBeNull();
  });

  it("states no headline or facts when details carry nothing for the kind", () => {
    const p = presentActivity(makeActivity("PLAN_CREATED", { planId: "plan-uuid" }));
    expect(p.headline).toBeNull();
    expect(p.facts).toEqual([]);
  });

  it("prefers the frame route as scope and keeps the frame identity available", () => {
    const p = presentActivity(
      makeActivity("STEP_STARTED", { skillName: "support.handle_billing" }, {
        route: "support.handle_billing#step-3",
        frameId: "frame-uuid",
      }),
    );
    expect(p.scope).toBe("support.handle_billing#step-3");
    expect(p.scopeTitle).toBe("frame-uuid");
  });

  it("falls back to the recorded skill name when no route is present", () => {
    const p = presentActivity(makeActivity("MODEL_REQUEST_SENT", { skillName: "support.handle_billing" }));
    expect(p.scope).toBe("support.handle_billing");
    expect(p.scopeTitle).toBeNull();
  });

  it("describes a model request by its segment and attempt without exposing the attempt id inline", () => {
    const activity = makeActivity("MODEL_REQUEST_SENT", {
      segment: "planning",
      attemptNumber: 2,
      attemptId: "attempt-uuid",
      attemptReason: "RETRY",
      providerAttemptNumber: 3,
    });
    const p = presentActivity(activity);
    expect(p.headline).toBe("planning");
    expect(p.facts).toEqual([
      { label: "attempt", value: "2", title: "attempt-uuid" },
      { label: "reason", value: "RETRY" },
      { label: "provider attempt", value: "3" },
    ]);
  });

  it("omits an initial attempt reason and a provider attempt that matches the attempt", () => {
    const p = presentActivity(
      makeActivity("MODEL_RESPONSE_RECEIVED", {
        attemptNumber: 1,
        attemptReason: "INITIAL",
        providerAttemptNumber: 1,
      }),
    );
    expect(p.facts).toEqual([{ label: "attempt", value: "1" }]);
  });

  it("states no attempt fact when only an opaque attempt id survived detail truncation", () => {
    const p = presentActivity(makeActivity("MODEL_REQUEST_SENT", { attemptId: "attempt-uuid" }));
    expect(p.facts).toEqual([]);
  });

  it("describes a failed provider attempt with its category and retry decision", () => {
    const activity = makeActivity("MODEL_ATTEMPT_FAILED", {
      failureCategory: "RATE_LIMITED",
      failureClassification: "TRANSIENT",
      retryDecision: "RETRY",
      retryDelayMillis: 1500,
      retryDelaySource: "RETRY_AFTER",
    });
    expect(presentActivity(activity).headline).toBe("RATE_LIMITED");
    expect(factValue(activity, "decision")).toBe("RETRY");
    expect(factValue(activity, "delay")).toBe("1.5s (RETRY_AFTER)");
  });

  it("falls back to the failure classification when no category was recorded", () => {
    const p = presentActivity(makeActivity("MODEL_ATTEMPT_FAILED", { failureClassification: "TRANSIENT" }));
    expect(p.headline).toBe("TRANSIENT");
  });

  it("names the invoked capability on tool calls", () => {
    const activity = makeActivity("TOOL_CALL_COMPLETED", {
      capabilityName: "crm.lookupAccount",
      linkedTaskId: "task-2",
      unplanned: true,
    });
    expect(presentActivity(activity).headline).toBe("crm.lookupAccount");
    expect(factValue(activity, "task")).toBe("task-2");
    expect(factValue(activity, "planning")).toBe("unplanned");
  });

  it("numbers steps and states the chosen action", () => {
    const activity = makeActivity("STEP_COMPLETED", { stepNumber: 3, stepAction: "CALL_TOOL" });
    expect(presentActivity(activity).headline).toBe("Step 3");
    expect(factValue(activity, "action")).toBe("CALL_TOOL");
  });

  it("leads an error with its message and shows the simple exception name with the qualified name retained", () => {
    const activity = makeActivity("ERROR_RECORDED", {
      message: "Tool execution failed",
      classification: "PERMANENT",
      exceptionType: "com.lokiscale.loomspan.LoomspanToolException",
    });
    const p = presentActivity(activity);
    expect(p.headline).toBe("Tool execution failed");
    expect(p.facts).toContainEqual({
      label: "exception",
      value: "LoomspanToolException",
      title: "com.lokiscale.loomspan.LoomspanToolException",
    });
  });

  it("ignores non-scalar and empty detail values rather than rendering them", () => {
    const p = presentActivity(
      makeActivity("TOOL_CALL_STARTED", { capabilityName: "", linkedTaskId: { nested: true } }),
    );
    expect(p.headline).toBeNull();
    expect(p.facts).toEqual([]);
  });
});

describe("formatTimestamp", () => {
  it("formats a valid ISO timestamp", () => {
    const result = formatTimestamp("2026-07-25T12:00:00Z");
    expect(result).toMatch(/\d{2}:\d{2}:\d{2}/);
  });

  it("returns input on invalid timestamp", () => {
    expect(formatTimestamp("not-a-date")).toBe("not-a-date");
  });
});

describe("formatDateTime", () => {
  it("formats a local calendar date and time with zero padding", () => {
    const local = new Date(2026, 7, 8, 5, 43, 25);
    expect(formatDateTime(local.toISOString())).toBe("08/08/2026 05:43:25");
  });

  it("pads a single-digit month, day, and time", () => {
    const local = new Date(2026, 0, 2, 3, 4, 5);
    expect(formatDateTime(local.toISOString())).toBe("01/02/2026 03:04:05");
  });

  it("returns input on invalid timestamp", () => {
    expect(formatDateTime("not-a-date")).toBe("not-a-date");
  });
});

describe("formatElapsed", () => {
  it("formats sub-second elapsed time", () => {
    expect(formatElapsed("2026-07-25T12:00:00.000Z", "2026-07-25T12:00:00.500Z")).toBe("500ms");
  });

  it("formats sub-minute elapsed time", () => {
    expect(formatElapsed("2026-07-25T12:00:00Z", "2026-07-25T12:00:05Z")).toBe("5.0s");
  });

  it("formats multi-minute elapsed time", () => {
    expect(formatElapsed("2026-07-25T12:00:00Z", "2026-07-25T12:02:30Z")).toBe("2m 30s");
  });

  it("returns dash on invalid input", () => {
    expect(formatElapsed("invalid", "also-invalid")).toBe("—");
  });
});

describe("formatDelta", () => {
  it("states the elapsed time since the preceding event", () => {
    expect(formatDelta("2026-07-25T12:00:00Z", "2026-07-25T12:00:02Z")).toBe("+2.0s");
  });

  it("states a zero delta for events sharing a timestamp", () => {
    expect(formatDelta("2026-07-25T12:00:00Z", "2026-07-25T12:00:00Z")).toBe("+0ms");
  });

  it("returns nothing rather than a negative delta when order is not monotonic", () => {
    expect(formatDelta("2026-07-25T12:00:02Z", "2026-07-25T12:00:00Z")).toBeNull();
  });

  it("returns nothing on an unparseable timestamp", () => {
    expect(formatDelta("invalid", "2026-07-25T12:00:00Z")).toBeNull();
  });
});
