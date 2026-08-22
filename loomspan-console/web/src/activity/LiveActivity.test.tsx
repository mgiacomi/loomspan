import { render, screen, fireEvent } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { LiveActivity } from "./LiveActivity";
import { BrowserAPIError } from "../api/client";
import type {
  ActiveExecution,
  Activity,
  ActivityCoverage,
  ActivityKind,
  ConnectionFact,
  Continuity,
} from "../api/contracts";

function makeActivity(
  cursor: string,
  kind: ActivityKind,
  summary: string,
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
  };
}

type ActivityView = {
  activities: Activity[];
  recentCompletions: Activity[];
  connected: boolean;
  connectionFact: ConnectionFact | null;
  error: BrowserAPIError | null;
  loading: boolean;
  lastCursor: string | null;
  continuity: Continuity | null;
  baselineObservedAt: string | null;
  coverage: ActivityCoverage;
  reconnectAttempt: number;
  loadRecent: () => Promise<void>;
};

const baseView: ActivityView = {
  activities: [],
  recentCompletions: [],
  connected: false,
  connectionFact: null,
  error: null,
  loading: false,
  lastCursor: null,
  continuity: null,
  baselineObservedAt: null,
  coverage: {},
  reconnectAttempt: 0,
  loadRecent: vi.fn(),
};

let view: ActivityView;

function makeExecution(
  sessionId: string,
  entrySkill: string,
  overrides?: Partial<ActiveExecution>,
): ActiveExecution {
  return {
    targetScopeId: "scope-1",
    sessionId,
    traceId: `trace-${sessionId}`,
    lastCanonicalSequence: 2,
    startedAt: "2026-07-25T12:00:00Z",
    updatedAt: "2026-07-25T12:05:00Z",
    elapsedMillis: 305_000,
    entrySkill,
    status: "ACTIVE",
    phase: "EXECUTING",
    summary: "Running",
    activePath: [],
    totalFrameDepth: 0,
    activePathTruncated: false,
    usage: {
      skillInvocations: 1,
      toolInvocations: 0,
      linterRetries: 0,
      modelCalls: 0,
      providerAttempts: 0,
      promptUnits: 0,
      completionUnits: 0,
      usageUnits: 0,
      exactModelResponses: 0,
      heuristicModelResponses: 0,
      unavailableModelResponses: 0,
    },
    configuredLimits: {
      maxSkillInvocations: 10,
      maxToolInvocations: 10,
      maxLinterRetries: 10,
      maxModelCalls: 10,
      maxProviderAttempts: 10,
      maxUsageUnits: 100,
    },
    ...overrides,
  };
}

const observabilityView = vi.hoisted(() => ({
  current: {
    activeExecutions: {
      targetScopeId: "scope-1" as string | null,
      items: [] as ActiveExecution[],
      hasMore: false,
      nextCursor: null as string | null,
      resumeCursor: null as string | null,
      observedAt: "2026-07-25T12:05:00Z" as string | null,
      loading: false,
      loaded: true,
      error: undefined as BrowserAPIError | undefined,
    },
    loadActiveExecutions: vi.fn(),
  },
}));

vi.mock("./ActivityProvider", () => ({
  useActivity: () => view,
}));

vi.mock("../observability/ObservabilityProvider", () => ({
  useOptionalObservability: () => observabilityView.current,
}));

vi.mock("react-router", () => ({
  Link: ({ children, to }: { children: ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}));

describe("LiveActivity", () => {
  beforeEach(() => {
    view = { ...baseView, loadRecent: vi.fn() };
    observabilityView.current.activeExecutions = {
      targetScopeId: "scope-1",
      items: [],
      hasMore: false,
      nextCursor: null,
      resumeCursor: null,
      observedAt: "2026-07-25T12:05:00Z",
      loading: false,
      loaded: true,
      error: undefined,
    };
    observabilityView.current.loadActiveExecutions.mockReset();
  });

  it("renders title and connection status when disconnected", () => {
    render(<LiveActivity />);
    expect(screen.getByRole("heading", { name: "Live Activity" })).toBeInTheDocument();
    expect(screen.getByText("Disconnected")).toBeInTheDocument();
  });

  it("renders connected status", () => {
    view.connected = true;
    render(<LiveActivity />);
    expect(screen.getByText("Connected")).toBeInTheDocument();
  });

  it("renders disconnected reason from connectionFact", () => {
    view.connectionFact = { connected: false, reason: "stream_closed" };
    render(<LiveActivity />);
    expect(screen.getByText("Disconnected: stream_closed")).toBeInTheDocument();
  });

  it("renders empty state when no activities and not loading", () => {
    render(<LiveActivity />);
    expect(screen.getByText("No active executions. New executions will appear here as they start.")).toBeInTheDocument();
  });

  it("renders loading indicator", () => {
    view.loading = true;
    render(<LiveActivity />);
    expect(screen.getByText("Loading recent activity…")).toBeInTheDocument();
  });

  it("renders error message", () => {
    view.error = new BrowserAPIError("CONSOLE_ERROR", "Something went wrong", 500);
    render(<LiveActivity />);
    expect(screen.getByText("Something went wrong")).toBeInTheDocument();
  });

  it("renders exact coverage cursor facts", () => {
    view.coverage = {
      globalEvictedThroughCursor: "8",
      sessionStartCursor: "9",
      sessionEvictedThroughCursor: "10",
      sessionRetainedCursorRange: { firstCursor: "11", lastCursor: "12" },
    };
    render(<LiveActivity />);
    expect(screen.getByText(/Global ring evicted through cursor 8/)).toBeInTheDocument();
    expect(screen.getByText(/Selected session start cursor 9/)).toBeInTheDocument();
    expect(screen.getByText(/Selected session evicted through cursor 10/)).toBeInTheDocument();
    expect(screen.getByText(/Selected session retained cursor range 11–12/)).toBeInTheDocument();
  });

  it("renders continuity reset notice", () => {
    view.continuity = {
      targetScopeId: "scope-1",
      instanceId: "inst-1",
      reset: { cause: "target_scope_changed", timestamp: "2026-07-25T12:00:00Z" },
    } as Continuity;
    render(<LiveActivity />);
    expect(screen.getByText(/Activity window was reset/)).toBeInTheDocument();
    expect(screen.getByText(/target_scope_changed/)).toBeInTheDocument();
  });

  it("renders replay gap notice with load recent button", () => {
    view.connectionFact = { connected: false, reason: "relay_frame_limit" };
    render(<LiveActivity />);
    expect(screen.getByRole("alert")).toHaveTextContent(/Some events were not delivered/);
    const button = screen.getByRole("button", { name: "Load recent" });
    fireEvent.click(button);
    expect(view.loadRecent).toHaveBeenCalledOnce();
  });

  it("does not render replay gap for non-gap reasons", () => {
    view.connectionFact = { connected: false, reason: "stream_closed" };
    render(<LiveActivity />);
    expect(screen.queryByText(/Some events were not delivered/)).toBeNull();
  });

  it("renders activity components when activities exist", () => {
    view.activities = [
      makeActivity("1", "TRACE_STARTED", "Started"),
      makeActivity("2", "STEP_COMPLETED", "Step done"),
    ];
    observabilityView.current.activeExecutions.items = [makeExecution("session-1", "root_skill")];
    view.connected = true;
    render(<LiveActivity />);
    expect(screen.getAllByText("Started").length).toBeGreaterThanOrEqual(1);
    expect(screen.getAllByText("Step done").length).toBeGreaterThanOrEqual(1);
  });

  it("separates activity into one compact always-following feed per active execution", () => {
    view.activities = [
      makeActivity("1", "TRACE_STARTED", "First execution started"),
      { ...makeActivity("2", "STEP_COMPLETED", "Second execution step"), sessionId: "session-2" },
    ];
    observabilityView.current.activeExecutions.items = [
      makeExecution("session-1", "first_root"),
      makeExecution("session-2", "second_root"),
    ];

    render(<LiveActivity />);

    const firstFeed = screen.getByRole("article", { name: "first_root" });
    const secondFeed = screen.getByRole("article", { name: "second_root" });
    expect(firstFeed).toHaveTextContent("First execution started");
    expect(firstFeed).not.toHaveTextContent("Second execution step");
    expect(secondFeed).toHaveTextContent("Second execution step");
    expect(secondFeed).not.toHaveTextContent("First execution started");
    expect(screen.queryByRole("button", { name: /auto-scroll/i })).toBeNull();
  });

  it("shows execution metadata, a scoped detail link, and an empty feed", () => {
    observabilityView.current.activeExecutions.items = [makeExecution("session 1", "root_skill")];
    render(<LiveActivity />);

    const feed = screen.getByRole("article", { name: "root_skill" });
    expect(feed).toHaveTextContent("Started");
    expect(feed).toHaveTextContent("Running");
    expect(feed).toHaveTextContent("5m 5s");
    expect(feed).toHaveTextContent("No activity yet.");
    expect(screen.getByRole("link", { name: "View active execution" })).toHaveAttribute(
      "href",
      "/active-executions/session%201?targetScopeId=scope-1",
    );
  });

  it("loads active executions for the overview activity feeds", () => {
    observabilityView.current.activeExecutions.loaded = false;
    render(<LiveActivity />);
    expect(observabilityView.current.loadActiveExecutions).toHaveBeenCalledWith();
  });

  it("renders recent completions section", () => {
    view.recentCompletions = [
      makeActivity("1", "TRACE_COMPLETED", "Execution finished"),
    ];
    render(<LiveActivity />);
    expect(screen.getByText("Recent completions (1)")).toBeInTheDocument();
    expect(screen.getByText("Execution finished")).toBeInTheDocument();
  });

  it("does not render recent completions when empty", () => {
    render(<LiveActivity />);
    expect(screen.queryByText(/Recent completions/)).toBeNull();
  });
});
