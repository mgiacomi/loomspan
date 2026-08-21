import { render, screen, waitFor, act } from "@testing-library/react";
import { type ReactNode } from "react";
import { MemoryRouter } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { TargetResponse, Activity, RecentActivityResponse } from "../api/contracts";

const targetView = {
  target: {
    address: "https://app.example",
    unencrypted: false,
    status: {
      observedAt: "2026-07-27T00:00:00Z",
      targetScopeId: "scope-1",
      targetSelection: "SELECTED",
      targetConnection: "REACHABLE",
      targetAuthentication: "ESTABLISHED",
      javaGoCompatibility: "COMPATIBLE",
      runtimeIdentity: "ESTABLISHED",
      liveMonitoring: "AVAILABLE",
    },
  } as TargetResponse,
  error: undefined,
  scopeGeneration: 0,
  connect: vi.fn(),
  credential: vi.fn(),
  recheck: vi.fn(),
  refresh: vi.fn(),
};

const sessionView = {
  status: "paired" as const,
  bootstrap: {
    processId: "p1",
    workspacePath: "/ws",
    tabId: "tab-1",
    csrfToken: "csrf-1",
    target: targetView.target,
  },
  pair: vi.fn(),
  requestManualChallenge: vi.fn(),
  readTargetStatus: vi.fn(),
  connectTarget: vi.fn(),
  supplyTargetCredential: vi.fn(),
  recheckTarget: vi.fn(),
};

const mockOpenActivityStream = vi.hoisted(() => vi.fn());
const mockFetchRecentActivities = vi.hoisted(() => vi.fn());
const observabilityView = vi.hoisted(() => ({
  loadInstance: vi.fn(),
  loadActiveExecutions: vi.fn(),
  loadTraces: vi.fn(),
}));

vi.mock("../target/TargetProvider", () => ({
  useTarget: () => targetView,
}));

vi.mock("../security/BrowserSessionProvider", () => ({
  useBrowserSession: () => sessionView,
}));

vi.mock("../api/client", () => ({
  BrowserAPIError: class BrowserAPIError extends Error {
    code: string;
    status: number;
    constructor(code: string, message: string, status: number) {
      super(message);
      this.code = code;
      this.status = status;
    }
  },
  openActivityStream: mockOpenActivityStream,
  fetchRecentActivities: mockFetchRecentActivities,
}));

vi.mock("../observability/ObservabilityProvider", () => ({
  useOptionalObservability: () => observabilityView,
}));

import { ActivityProvider, useActivity } from "./ActivityProvider";

function makeActivity(cursor: string, kind: string, summary: string): Activity {
  return {
    instanceId: "11111111-1111-4111-8111-111111111111",
    cursor,
    sessionId: "session-1",
    traceId: "trace-1",
    canonicalSequence: parseInt(cursor, 10),
    timestamp: "2026-07-25T12:00:00Z",
    kind: kind as Activity["kind"],
    executionStatus: "RUNNING",
    summary,
    details: {},
  };
}

function withProvider(children: ReactNode) {
  return render(
    <MemoryRouter>
      <ActivityProvider>{children}</ActivityProvider>
    </MemoryRouter>,
  );
}

function Consumer() {
  const ctx = useActivity();
  return (
    <div>
      <span data-testid="connected">{String(ctx.connected)}</span>
      <span data-testid="activity-count">{ctx.activities.length}</span>
      <span data-testid="loading">{String(ctx.loading)}</span>
      <span data-testid="error">{ctx.error?.message ?? "none"}</span>
      <span data-testid="reconnect">{ctx.reconnectAttempt}</span>
      <span data-testid="last-cursor">{ctx.lastCursor ?? "none"}</span>
      <span data-testid="baseline-observed">{ctx.baselineObservedAt ?? "none"}</span>
      <button onClick={() => void ctx.loadRecent()}>load-recent</button>
    </div>
  );
}

describe("ActivityProvider", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    observabilityView.loadInstance.mockResolvedValue(undefined);
    observabilityView.loadActiveExecutions.mockResolvedValue(undefined);
    observabilityView.loadTraces.mockResolvedValue(undefined);
    mockOpenActivityStream.mockReturnValue(() => {});
    mockFetchRecentActivities.mockResolvedValue({
      items: [],
      hasMore: false,
      nextCursor: "",
      continuity: undefined,
      beginningUnavailable: false,
    } as RecentActivityResponse);
  });

  it("opens stream when target is connected and session is paired", () => {
    withProvider(<Consumer />);
    expect(mockOpenActivityStream).toHaveBeenCalledTimes(1);
  });

  it("does not open stream when target is not connected", () => {
    targetView.target.status.targetConnection = "UNAVAILABLE";
    targetView.target.status.targetAuthentication = "UNKNOWN";
    withProvider(<Consumer />);
    expect(mockOpenActivityStream).not.toHaveBeenCalled();
    targetView.target.status.targetConnection = "REACHABLE";
    targetView.target.status.targetAuthentication = "ESTABLISHED";
  });

  it("dispatches stream-connected on successful connection callback", async () => {
    mockOpenActivityStream.mockImplementation((callbacks: any) => {
      callbacks.onConnection({ connected: true });
      return () => {};
    });
    withProvider(<Consumer />);
    await waitFor(() => {
      expect(screen.getByTestId("connected")).toHaveTextContent("true");
    });
  });

  it("does not dispatch stream-connected when connection fact is false", async () => {
    mockOpenActivityStream.mockImplementation((callbacks: any) => {
      callbacks.onConnection({ connected: false, reason: "live_unavailable" });
      return () => {};
    });
    withProvider(<Consumer />);
    await waitFor(() => {
      expect(screen.getByTestId("connected")).toHaveTextContent("false");
    });
  });

  it("dispatches stream-activity when activity callback fires", async () => {
    mockOpenActivityStream.mockImplementation((callbacks: any) => {
      callbacks.onConnection({ connected: true });
      callbacks.onActivity(makeActivity("1", "STEP_STARTED", "Step 1"));
      return () => {};
    });
    withProvider(<Consumer />);
    await waitFor(() => {
      expect(screen.getByTestId("activity-count")).toHaveTextContent("1");
    });
  });

  it("dispatches stream-error and increments reconnect on error callback", async () => {
    mockOpenActivityStream.mockImplementation((callbacks: any) => {
      callbacks.onError(new Error("Network failure"));
      return () => {};
    });
    withProvider(<Consumer />);
    await waitFor(() => {
      expect(screen.getByTestId("error")).toHaveTextContent("Network failure");
      expect(screen.getByTestId("reconnect")).toHaveTextContent("1");
    });
  });

  it("dispatches stream-closed and increments reconnect on close callback", async () => {
    mockOpenActivityStream.mockImplementation((callbacks: any) => {
      callbacks.onClose();
      return () => {};
    });
    withProvider(<Consumer />);
    await waitFor(() => {
      expect(screen.getByTestId("connected")).toHaveTextContent("false");
      expect(screen.getByTestId("reconnect")).toHaveTextContent("1");
    });
  });

  it("loadRecent fetches and dispatches recent-success", async () => {
    const items = [makeActivity("1", "TRACE_STARTED", "Started")];
    mockFetchRecentActivities.mockResolvedValue({
      items,
      hasMore: false,
      nextCursor: "1",
      continuity: undefined,
      beginningUnavailable: false,
    } as RecentActivityResponse);

    withProvider(<Consumer />);
    await act(async () => {
      screen.getByText("load-recent").click();
    });
    await waitFor(() => {
      expect(screen.getByTestId("activity-count")).toHaveTextContent("1");
      expect(screen.getByTestId("loading")).toHaveTextContent("false");
    });
  });

  it("loadRecent dispatches recent-error on fetch failure", async () => {
    const { BrowserAPIError } = await import("../api/client");
    mockFetchRecentActivities.mockRejectedValue(
      new BrowserAPIError("CONSOLE_ERROR", "Fetch failed", 500),
    );

    withProvider(<Consumer />);
    await act(async () => {
      screen.getByText("load-recent").click();
    });
    await waitFor(() => {
      expect(screen.getByTestId("error")).toHaveTextContent("Fetch failed");
      expect(screen.getByTestId("loading")).toHaveTextContent("false");
    });
  });

  it("resets state and reopens stream when scopeGeneration changes", async () => {
    let callCount = 0;
    mockOpenActivityStream.mockImplementation((callbacks: any) => {
      callCount++;
      callbacks.onConnection({ connected: true });
      if (callCount === 1) {
        callbacks.onActivity(makeActivity("1", "STEP_STARTED", "Step 1"));
      }
      return () => {};
    });

    const { rerender } = withProvider(<Consumer />);
    await waitFor(() => {
      expect(screen.getByTestId("activity-count")).toHaveTextContent("1");
    });

    targetView.scopeGeneration = 1;
    rerender(
      <MemoryRouter>
        <ActivityProvider>
          <Consumer />
        </ActivityProvider>
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(mockOpenActivityStream).toHaveBeenCalledTimes(2);
      expect(screen.getByTestId("activity-count")).toHaveTextContent("0");
    });
    targetView.scopeGeneration = 0;
  });

  it("reconnects with exponential backoff after stream closes", () => {
    vi.useFakeTimers();
    mockOpenActivityStream.mockImplementation((callbacks: any) => {
      callbacks.onClose();
      return () => {};
    });

    withProvider(<Consumer />);
    expect(screen.getByTestId("reconnect")).toHaveTextContent("1");
    expect(mockOpenActivityStream).toHaveBeenCalledTimes(1);

    vi.advanceTimersByTime(1000);
    expect(mockOpenActivityStream).toHaveBeenCalledTimes(2);
    vi.useRealTimers();
  });

  it("reconnects after the last cursor when the same reset fact is repeated", async () => {
    vi.useFakeTimers();
    let call = 0;
    const continuity = {
      intervalId: "interval-2",
      targetScopeId: "scope-1",
      instanceId: "11111111-1111-4111-8111-111111111111",
      reset: { cause: "upstream_stale_cursor", timestamp: "2026-07-25T12:01:00Z" },
    };
    mockOpenActivityStream.mockImplementation((callbacks: any) => {
      call++;
      callbacks.onContinuity(continuity);
      if (call === 1) {
        callbacks.onActivity(makeActivity("7", "STEP_STARTED", "Step 7"));
        callbacks.onClose();
      }
      return () => {};
    });

    withProvider(<Consumer />);
    await act(async () => {});
    expect(screen.getByTestId("activity-count")).toHaveTextContent("1");

    await act(async () => {
      vi.advanceTimersByTime(1000);
    });

    expect(mockOpenActivityStream).toHaveBeenCalledTimes(2);
    expect(mockOpenActivityStream.mock.calls[1]?.[2]).toBe("7");
    expect(screen.getByTestId("activity-count")).toHaveTextContent("1");
    vi.useRealTimers();
  });

  it("closes a gapped relay, replaces from recent state, and resumes after the resynced cursor", async () => {
    let callbacks: any;
    const close = vi.fn();
    const continuity = {
      intervalId: "interval-1",
      targetScopeId: "scope-1",
      instanceId: "11111111-1111-4111-8111-111111111111",
      lastCursor: "258",
    };
    mockOpenActivityStream.mockImplementation((nextCallbacks: any) => {
      callbacks = nextCallbacks;
      return close;
    });
    mockFetchRecentActivities.mockResolvedValue({
      items: [
        makeActivity("257", "STEP_STARTED", "Step 257"),
        makeActivity("258", "STEP_COMPLETED", "Step 258"),
      ],
      hasMore: false,
      nextCursor: "",
      continuity,
      beginningUnavailable: true,
    } as RecentActivityResponse);

    withProvider(<Consumer />);
    await act(async () => {
      callbacks.onContinuity(continuity);
      callbacks.onActivity(makeActivity("1", "STEP_STARTED", "Old replay"));
      callbacks.onReplayGap("replay_overflow");
    });

    await waitFor(() => {
      expect(close).toHaveBeenCalledTimes(1);
      expect(mockOpenActivityStream).toHaveBeenCalledTimes(2);
      expect(mockOpenActivityStream.mock.calls[1]?.[2]).toBe("258");
      expect(screen.getByTestId("activity-count")).toHaveTextContent("2");
      expect(screen.getByTestId("last-cursor")).toHaveTextContent("258");
    });
  });

  it("uses the authoritative baseline observation timestamp from the relay", async () => {
    let callbacks: any;
    mockOpenActivityStream.mockImplementation((nextCallbacks: any) => {
      callbacks = nextCallbacks;
      return () => {};
    });
    withProvider(<Consumer />);
    act(() => callbacks.onBaselineRefreshed("2026-07-25T12:01:00Z"));
    await waitFor(() => {
      expect(screen.getByTestId("baseline-observed")).toHaveTextContent(
        "2026-07-25T12:01:00Z",
      );
    });
  });

  it("coalesces activity into authoritative collection and header refreshes", async () => {
    vi.useFakeTimers();
    let callbacks: any;
    mockOpenActivityStream.mockImplementation((nextCallbacks: any) => {
      callbacks = nextCallbacks;
      return () => {};
    });
    withProvider(<Consumer />);

    act(() => {
      callbacks.onActivity(makeActivity("1", "TRACE_STARTED", "Started"));
      callbacks.onActivity(makeActivity("2", "STEP_STARTED", "Step started"));
      callbacks.onActivity(makeActivity("3", "TRACE_COMPLETED", "Completed"));
      callbacks.onActivity(makeActivity("4", "EXECUTION_OBSERVATION_ENDED", "Ended"));
      vi.advanceTimersByTime(250);
    });

    expect(observabilityView.loadInstance).toHaveBeenCalledTimes(1);
    expect(observabilityView.loadActiveExecutions).toHaveBeenCalledTimes(1);
    expect(observabilityView.loadTraces).toHaveBeenCalledTimes(1);
    vi.useRealTimers();
  });

  it("refreshes all authoritative views after each periodic baseline", () => {
    vi.useFakeTimers();
    let callbacks: any;
    mockOpenActivityStream.mockImplementation((nextCallbacks: any) => {
      callbacks = nextCallbacks;
      return () => {};
    });
    withProvider(<Consumer />);

    act(() => {
      callbacks.onBaselineRefreshed("2026-07-25T12:01:00Z");
      vi.advanceTimersByTime(250);
    });

    expect(observabilityView.loadInstance).toHaveBeenCalledTimes(1);
    expect(observabilityView.loadActiveExecutions).toHaveBeenCalledTimes(1);
    expect(observabilityView.loadTraces).toHaveBeenCalledTimes(1);
    vi.useRealTimers();
  });
});
