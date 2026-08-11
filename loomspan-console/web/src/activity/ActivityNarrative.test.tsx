import { render, screen, fireEvent } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ActivityNarrative } from "./ActivityNarrative";
import type { Activity, ActivityKind } from "../api/contracts";

function makeActivity(
  cursor: string,
  kind: ActivityKind,
  summary: string,
  overrides?: Partial<Activity>,
): Activity {
  return {
    instanceId: "11111111-1111-4111-8111-111111111111",
    cursor,
    sessionId: "session-1",
    traceId: "trace-1",
    canonicalSequence: parseInt(cursor, 10),
    timestamp: "2026-07-25T12:00:00Z",
    kind,
    executionStatus: "RUNNING",
    summary,
    details: {},
    ...overrides,
  };
}

describe("ActivityNarrative", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("renders empty state when no activities", () => {
    render(<ActivityNarrative activities={[]} isLive={true} />);
    expect(screen.getByText("No activity yet.")).toBeInTheDocument();
  });

  it("renders activity items in order", () => {
    const activities = [
      makeActivity("1", "TRACE_STARTED", "Execution began"),
      makeActivity("2", "STEP_COMPLETED", "Step done"),
    ];
    render(<ActivityNarrative activities={activities} isLive={true} />);
    expect(screen.getByText("Execution began")).toBeInTheDocument();
    expect(screen.getByText("Step done")).toBeInTheDocument();
    expect(screen.getByText("2 events")).toBeInTheDocument();
  });

  it("states a summary that merely repeats the kind label only once", () => {
    const activities = [
      makeActivity("1", "TRACE_STARTED", "Execution started"),
      makeActivity("2", "MODEL_REQUEST_SENT", "Model request sent"),
    ];
    render(<ActivityNarrative activities={activities} isLive={true} />);
    expect(screen.getAllByText("Execution started")).toHaveLength(1);
    expect(screen.getAllByText("Model request sent")).toHaveLength(1);
  });

  it("follows newest item initially (follow button shows pause)", () => {
    render(<ActivityNarrative activities={[makeActivity("1", "TRACE_STARTED", "Started")]} isLive={true} />);
    const toggle = screen.getByRole("button", { name: "Pause auto-scroll" });
    expect(toggle).toHaveAttribute("aria-pressed", "true");
  });

  it("pauses following when user scrolls backward", () => {
    const activities = [makeActivity("1", "TRACE_STARTED", "Started")];
    render(<ActivityNarrative activities={activities} isLive={true} />);
    const list = screen.getByRole("log");

    Object.defineProperty(list, "scrollHeight", { value: 1000, configurable: true });
    Object.defineProperty(list, "clientHeight", { value: 200, configurable: true });
    Object.defineProperty(list, "scrollTop", { value: 0, configurable: true });

    fireEvent.scroll(list);

    const toggle = screen.getByRole("button", { name: "Resume auto-scroll" });
    expect(toggle).toHaveAttribute("aria-pressed", "false");
  });

  it("resumes following on button click", () => {
    const activities = [makeActivity("1", "TRACE_STARTED", "Started")];
    render(<ActivityNarrative activities={activities} isLive={true} />);

    const list = screen.getByRole("log");
    Object.defineProperty(list, "scrollHeight", { value: 1000, configurable: true });
    Object.defineProperty(list, "clientHeight", { value: 200, configurable: true });
    Object.defineProperty(list, "scrollTop", { value: 0, configurable: true, writable: true });
    fireEvent.scroll(list);

    const toggle = screen.getByRole("button", { name: "Resume auto-scroll" });
    fireEvent.click(toggle);

    expect(screen.getByRole("button", { name: "Pause auto-scroll" })).toHaveAttribute("aria-pressed", "true");
  });

  it("disables following when stream goes offline", () => {
    const { rerender } = render(
      <ActivityNarrative activities={[makeActivity("1", "TRACE_STARTED", "Started")]} isLive={true} />,
    );
    expect(screen.getByRole("button", { name: "Pause auto-scroll" })).toHaveAttribute("aria-pressed", "true");

    rerender(
      <ActivityNarrative activities={[makeActivity("1", "TRACE_STARTED", "Started")]} isLive={false} />,
    );
    expect(screen.getByRole("button", { name: "Resume auto-scroll" })).toHaveAttribute("aria-pressed", "false");
  });

  it("renders untrusted summary as text only", () => {
    const activities = [
      makeActivity("1", "ERROR_RECORDED", "<script>alert('xss')</script>"),
    ];
    const { container } = render(<ActivityNarrative activities={activities} isLive={true} />);
    expect(container.querySelector("script")).toBeNull();
    expect(screen.getByText("<script>alert('xss')</script>")).toBeInTheDocument();
  });

  it("shows outcome for TRACE_COMPLETED", () => {
    const activities = [
      makeActivity("1", "TRACE_COMPLETED", "Execution finished"),
    ];
    render(<ActivityNarrative activities={activities} isLive={true} />);
    expect(screen.getByText(/Outcome:/)).toBeInTheDocument();
  });

  it("does not show outcome for EXECUTION_OBSERVATION_ENDED", () => {
    const activities = [
      makeActivity("1", "EXECUTION_OBSERVATION_ENDED", "Observation ended"),
    ];
    render(<ActivityNarrative activities={activities} isLive={true} />);
    expect(screen.queryByText(/Outcome:/)).toBeNull();
  });

  it("identifies each event by its frame route rather than the repeated session id", () => {
    const activities = [
      makeActivity("1", "STEP_STARTED", "Step started", {
        route: "support.handle_billing#step-3",
        frameId: "frame-uuid",
        details: { stepNumber: 3 },
      }),
    ];
    render(<ActivityNarrative activities={activities} isLive={true} />);
    expect(screen.getByText("support.handle_billing#step-3")).toHaveAttribute("title", "frame-uuid");
    expect(screen.queryByText(/session-1/)).toBeNull();
    expect(screen.getByText("Step 3")).toBeInTheDocument();
  });

  it("renders recorded detail facts beside the kind label", () => {
    const activities = [
      makeActivity("1", "TOOL_CALL_COMPLETED", "Tool call completed", {
        details: { capabilityName: "crm.lookupAccount", linkedTaskId: "task-2" },
      }),
    ];
    render(<ActivityNarrative activities={activities} isLive={true} />);
    expect(screen.getByText("crm.lookupAccount")).toBeInTheDocument();
    expect(screen.getByText("task")).toBeInTheDocument();
    expect(screen.getByText("task-2")).toBeInTheDocument();
  });

  it("states the elapsed time between consecutive events but not before the first", () => {
    const activities = [
      makeActivity("1", "TRACE_STARTED", "Execution started"),
      makeActivity("2", "STEP_STARTED", "Step started", { timestamp: "2026-07-25T12:00:04Z" }),
    ];
    render(<ActivityNarrative activities={activities} isLive={true} />);
    expect(screen.getByText("+4.0s")).toBeInTheDocument();
    expect(screen.queryAllByText(/^\+/)).toHaveLength(1);
  });

  it("renders untrusted detail values as text only", () => {
    const activities = [
      makeActivity("1", "ERROR_RECORDED", "Execution error recorded", {
        details: { message: "<img src=x onerror=alert(1)>" },
      }),
    ];
    const { container } = render(<ActivityNarrative activities={activities} isLive={true} />);
    expect(container.querySelector("img")).toBeNull();
    expect(screen.getByText("<img src=x onerror=alert(1)>")).toBeInTheDocument();
  });

  it("uses role=log and aria-live for accessibility", () => {
    render(<ActivityNarrative activities={[makeActivity("1", "TRACE_STARTED", "Started")]} isLive={true} />);
    const list = screen.getByRole("log");
    expect(list).toHaveAttribute("aria-live", "polite");
    expect(list).toHaveAttribute("aria-relevant", "additions");
  });
});
