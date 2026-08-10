import { describe, expect, it } from "vitest";
import { presentActivity, formatDateTime, formatTimestamp, formatElapsed } from "./activityPresentation";
import type { Activity, ActivityKind } from "../api/contracts";
import { ACTIVITY_KIND_LABELS } from "../api/contracts";

function makeActivity(kind: ActivityKind, details?: Record<string, unknown>): Activity {
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
  };
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
