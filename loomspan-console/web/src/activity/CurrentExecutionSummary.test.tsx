import { act, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { CurrentExecutionSummary } from "./CurrentExecutionSummary";
import type { ActiveExecution, Activity } from "../api/contracts";

const execution: ActiveExecution = {
  targetScopeId: "scope-1",
  sessionId: "session-1",
  traceId: "trace-1",
  lastCanonicalSequence: 42,
  startedAt: "2026-07-25T12:00:00Z",
  updatedAt: "2026-07-25T12:05:00Z",
  elapsedMillis: 305_000,
  entrySkill: "entry",
  status: "ACTIVE",
  phase: "EXECUTING",
  summary: "Authoritative snapshot summary",
  activePath: [],
  totalFrameDepth: 0,
  activePathTruncated: false,
  usage: {
    skillInvocations: 4,
    toolInvocations: 7,
    linterRetries: 1,
    modelCalls: 3,
    promptUnits: 100,
    completionUnits: 50,
    usageUnits: 150,
    exactModelResponses: 2,
    heuristicModelResponses: 1,
    unavailableModelResponses: 0,
  },
  configuredLimits: {
    maxSkillInvocations: 10,
    maxToolInvocations: 20,
    maxLinterRetries: 5,
    maxModelCalls: 30,
    maxUsageUnits: 1000,
  },
};

function activity(kind: Activity["kind"] = "STEP_COMPLETED"): Activity {
  return {
    instanceId: "11111111-1111-4111-8111-111111111111",
    cursor: "42",
    sessionId: "session-1",
    traceId: "trace-1",
    canonicalSequence: 42,
    timestamp: "2026-07-25T12:05:00Z",
    kind,
    executionStatus: kind === "TRACE_COMPLETED" ? "COMPLETED" : "ACTIVE",
    summary: "Latest bounded activity",
    details: kind === "TRACE_COMPLETED" ? { outcome: "succeeded" } : {},
  };
}

describe("CurrentExecutionSummary", () => {
  it("renders an unavailable snapshot without claiming no selection", () => {
    render(<CurrentExecutionSummary execution={null} activities={[]} />);
    expect(screen.getByText("The selected execution snapshot is unavailable.")).toBeInTheDocument();
    expect(screen.queryByText("No active execution selected.")).toBeNull();
  });

  it("uses authoritative snapshot status and elapsed time", () => {
    render(<CurrentExecutionSummary execution={execution} activities={[activity()]} />);
    expect(screen.getByLabelText("Status: ACTIVE")).toBeInTheDocument();
    expect(screen.getByText("5m 5s")).toBeInTheDocument();
    expect(screen.getByText("Authoritative snapshot summary")).toBeInTheDocument();
  });

  it("leaves invocation counters to the usage section, even with an activity suffix", () => {
    const suffix = [activity("STEP_STARTED"), { ...activity("TOOL_CALL_STARTED"), cursor: "43" }];
    render(<CurrentExecutionSummary execution={execution} activities={suffix} />);
    for (const label of ["Skill invocations", "Tool invocations", "Model calls"]) {
      expect(screen.queryByText(label)).toBeNull();
    }
    for (const count of ["4", "7", "3"]) {
      expect(screen.queryByText(count)).toBeNull();
    }
  });

  it("preserves terminal facts when the active snapshot is unavailable", () => {
    render(<CurrentExecutionSummary execution={null} activities={[activity("TRACE_COMPLETED")]} />);
    expect(screen.getByLabelText("Terminal")).toHaveTextContent("succeeded");
    expect(screen.getByText("session-1")).toBeInTheDocument();
    expect(screen.getByText("trace-1")).toBeInTheDocument();
  });

  it("does not invent completion for observation-ended", () => {
    render(
      <CurrentExecutionSummary
        execution={null}
        activities={[activity("EXECUTION_OBSERVATION_ENDED")]}
      />,
    );
    expect(screen.getByLabelText("Terminal")).toHaveTextContent("observation ended");
    expect(screen.getByLabelText("Terminal")).not.toHaveTextContent(/^completed$/);
  });

  it("renders the latest activity separately from the snapshot summary", () => {
    render(<CurrentExecutionSummary execution={execution} activities={[activity()]} />);
    expect(screen.getByLabelText("Latest activity summary")).toHaveTextContent(
      "Latest activity: Latest bounded activity",
    );
  });

  it("does not restate the latest activity when it repeats the snapshot summary", () => {
    const repeated = { ...activity(), summary: execution.summary };
    render(<CurrentExecutionSummary execution={execution} activities={[repeated]} />);
    expect(screen.getAllByText("Authoritative snapshot summary")).toHaveLength(1);
    expect(screen.queryByLabelText("Latest activity summary")).toBeNull();
  });

  it("states the entry skill and snapshot update time with the observation note", () => {
    render(
      <CurrentExecutionSummary
        execution={execution}
        activities={[activity()]}
        observedAt="2026-07-25T12:05:00Z"
      />,
    );
    expect(screen.getByText("entry")).toBeInTheDocument();
    expect(screen.getByText(/^\d{2}\/\d{2}\/\d{4} \d{2}:\d{2}:\d{2}$/)).toBeInTheDocument();
    expect(screen.queryByText(execution.startedAt)).toBeNull();
    expect(
      screen.getByText(/Execution updated at 2026-07-25T12:05:00Z\./),
    ).toBeInTheDocument();
  });

  it("lets a terminal activity override the retained active snapshot status", () => {
    render(
      <CurrentExecutionSummary
        execution={execution}
        activities={[activity("TRACE_COMPLETED")]}
        observationEnded
      />,
    );
    expect(screen.getByLabelText("Status: COMPLETED")).toBeInTheDocument();
    expect(screen.queryByLabelText("Status: ACTIVE")).toBeNull();
  });

  it("marks a missed terminal observation without inventing an outcome", () => {
    render(
      <CurrentExecutionSummary
        execution={execution}
        activities={[]}
        observationEnded
      />,
    );
    expect(screen.getByLabelText("Status: OBSERVATION ENDED")).toBeInTheDocument();
    expect(screen.queryByLabelText("Terminal")).toBeNull();
  });

  it("advances elapsed time from the authoritative observation while connected", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-07-25T12:05:00Z"));
    render(
      <CurrentExecutionSummary
        execution={execution}
        activities={[activity()]}
        observedAt="2026-07-25T12:05:00Z"
        connected
      />,
    );
    expect(screen.getByText("5m 5s")).toBeInTheDocument();
    act(() => {
      vi.advanceTimersByTime(2_000);
    });
    expect(screen.getByText("5m 7s")).toBeInTheDocument();
    vi.useRealTimers();
  });
});
