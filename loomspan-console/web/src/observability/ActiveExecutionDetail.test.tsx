import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, expect, test, vi } from "vitest";
import type { ReactNode } from "react";
import type { ActiveExecution, AcquiredArtifact } from "../api/contracts";

const route = vi.hoisted(() => ({
  scope: "scope-1",
  sessionId: "session-1",
  navigate: vi.fn(),
}));
const activityView = vi.hoisted(() => ({ current: undefined as any }));

vi.mock("../api/client", () => ({
  getActiveExecutionDetail: vi.fn(),
  acquireArtifact: vi.fn(),
  BrowserAPIError: class BrowserAPIError extends Error {
    code: string;
    status: number;
    constructor(code: string, message: string, status: number) {
      super(message);
      this.code = code;
      this.status = status;
    }
  },
}));

vi.mock("react-router", () => ({
  Link: ({ children, to }: { children: ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
  useParams: () => ({ sessionId: route.sessionId }),
  useNavigate: () => route.navigate,
  useSearchParams: () => [new URLSearchParams({ targetScopeId: route.scope })],
}));

vi.mock("../target/TargetProvider", () => ({
  useTarget: () => ({
    target: { status: { targetScopeId: "scope-1" } },
    scopeGeneration: 0,
    refresh: vi.fn().mockResolvedValue(undefined),
  }),
}));

vi.mock("../security/BrowserSessionProvider", () => ({
  useBrowserSession: () => ({
    getSecurity: () => ({ tabId: "test-tab", csrfToken: "test-token" }),
  }),
}));

vi.mock("../activity/ActivityProvider", () => ({
  useOptionalActivity: () => activityView.current,
}));

import { getActiveExecutionDetail, acquireArtifact } from "../api/client";
import { ActiveExecutionDetailView } from "./ActiveExecutionDetail";

const execution: ActiveExecution = {
  targetScopeId: "scope-1",
  sessionId: "session-1",
  traceId: "trace-1",
  lastCanonicalSequence: 42,
  startedAt: "2026-07-27T10:00:00Z",
  updatedAt: "2026-07-27T10:05:00Z",
  elapsedMillis: 300000,
  entrySkill: "CheckDns",
  status: "ACTIVE",
  phase: "EXECUTING",
  summary: "Checking DNS records",
  activePath: [],
  totalFrameDepth: 1,
  activePathTruncated: false,
  usage: {
    skillInvocations: 1, toolInvocations: 2, linterRetries: 0, modelCalls: 3, providerAttempts: 4,
    promptUnits: 100, completionUnits: 50, usageUnits: 150,
    exactModelResponses: 2, heuristicModelResponses: 1, unavailableModelResponses: 0,
  },
  configuredLimits: {
    maxSkillInvocations: 10, maxToolInvocations: 20, maxLinterRetries: 5,
    maxModelCalls: 30, maxProviderAttempts: 90, maxUsageUnits: 1000,
  },
};

beforeEach(() => {
  vi.mocked(getActiveExecutionDetail).mockReset();
  vi.mocked(acquireArtifact).mockReset();
  route.scope = "scope-1";
  route.sessionId = "session-1";
  route.navigate.mockReset();
  activityView.current = undefined;
});

test("stale execution deep link resets before requesting the identifier", async () => {
  route.scope = "scope-old";
  render(<ActiveExecutionDetailView />);
  await vi.waitFor(() => expect(route.navigate).toHaveBeenCalled());
  expect(getActiveExecutionDetail).not.toHaveBeenCalled();
});

test("active execution detail renders facts when loaded", async () => {
  vi.mocked(getActiveExecutionDetail).mockResolvedValue(execution);
  render(<ActiveExecutionDetailView />);
  await vi.waitFor(() => {
    expect(screen.getAllByText("session-1").length).toBeGreaterThan(0);
  });
  expect(screen.getAllByText("trace-1").length).toBeGreaterThan(0);
  expect(screen.getByText("CheckDns")).toBeInTheDocument();
  expect(screen.getAllByText("EXECUTING").length).toBeGreaterThan(0);
});

test("usage states each observed counter beside its configured limit and proportion", async () => {
  vi.mocked(getActiveExecutionDetail).mockResolvedValue(execution);
  render(<ActiveExecutionDetailView />);
  await vi.waitFor(() => {
    expect(
      screen.getByRole("table", { name: "Observed usage against configured limits" }),
    ).toBeInTheDocument();
  });
  expect(screen.getByRole("row", { name: "Skill invocations 1 10 10%" })).toBeInTheDocument();
  expect(screen.getByRole("row", { name: "Tool invocations 2 20 10%" })).toBeInTheDocument();
  expect(screen.getByRole("row", { name: "Model calls 3 30 10%" })).toBeInTheDocument();
  expect(screen.getByRole("row", { name: "Usage units 150 1000 15%" })).toBeInTheDocument();
  expect(screen.getByRole("row", { name: "Prompt units 100" })).toBeInTheDocument();
  expect(screen.getByRole("row", { name: "Completion units 50" })).toBeInTheDocument();
});

test("usage tables adopt the shared data-table presentation and scroll region", async () => {
  vi.mocked(getActiveExecutionDetail).mockResolvedValue(execution);
  render(<ActiveExecutionDetailView />);
  const limits = await vi.waitFor(() =>
    screen.getByRole("table", { name: "Observed usage against configured limits" }),
  );
  const units = screen.getByRole("table", { name: "Observed usage units" });
  for (const table of [limits, units]) {
    expect(table).toHaveClass("observability-table");
    expect(table).toHaveClass("usage-table");
    expect(table.closest(".observability-table-region")).not.toBeNull();
  }
  expect(screen.getByRole("region", { name: "Usage against configured limits" })).toBeInTheDocument();
  expect(screen.getByRole("region", { name: "Usage unit totals" })).toBeInTheDocument();
});

test("usage reports an undefined proportion rather than dividing by a zero limit", async () => {
  vi.mocked(getActiveExecutionDetail).mockResolvedValue({
    ...execution,
    configuredLimits: { ...execution.configuredLimits, maxLinterRetries: 0 },
  });
  render(<ActiveExecutionDetailView />);
  await vi.waitFor(() => {
    expect(screen.getByRole("row", { name: "Linter retries 0 0 undefined" })).toBeInTheDocument();
  });
  expect(screen.queryByText(/NaN|Infinity/)).toBeNull();
});

test("usage keeps inexact measurement visible instead of listing response counters as facts", async () => {
  vi.mocked(getActiveExecutionDetail).mockResolvedValue(execution);
  render(<ActiveExecutionDetailView />);
  await vi.waitFor(() => {
    expect(
      screen.getByText(
        "These counts are not exact. Model responses measured: 3; heuristic: 1; usage unavailable: 0.",
      ),
    ).toBeVisible();
  });
});

test("usage confirms exact measurement without claiming responses that were never measured", async () => {
  vi.mocked(getActiveExecutionDetail).mockResolvedValue({
    ...execution,
    usage: { ...execution.usage, exactModelResponses: 3, heuristicModelResponses: 0 },
  });
  render(<ActiveExecutionDetailView />);
  await vi.waitFor(() => {
    expect(
      screen.getByText("All model responses reported exact usage (3 measured)."),
    ).toBeVisible();
  });
});

test("usage does not assert measurement quality before any response is measured", async () => {
  vi.mocked(getActiveExecutionDetail).mockResolvedValue({
    ...execution,
    usage: {
      ...execution.usage,
      exactModelResponses: 0,
      heuristicModelResponses: 0,
      unavailableModelResponses: 0,
    },
  });
  render(<ActiveExecutionDetailView />);
  await vi.waitFor(() => {
    expect(screen.getByText("No model responses have been measured yet.")).toBeVisible();
  });
  expect(screen.queryByText(/reported exact usage/)).toBeNull();
  expect(screen.queryByText(/not exact/)).toBeNull();
});

test("active execution detail states elapsed and identity once and collapses diagnostics", async () => {
  vi.mocked(getActiveExecutionDetail).mockResolvedValue(execution);
  render(<ActiveExecutionDetailView />);
  await vi.waitFor(() => {
    expect(screen.getAllByText("session-1").length).toBeGreaterThan(0);
  });
  expect(screen.getAllByText("session-1")).toHaveLength(1);
  expect(screen.getAllByText("trace-1")).toHaveLength(1);
  expect(screen.getByText("5m 0s")).toBeVisible();
  expect(screen.queryByText("300000")).toBeNull();
  expect(screen.getByText("Snapshot diagnostics")).toBeVisible();
  expect(screen.getByText("42")).not.toBeVisible();
});

test("active execution detail renders loading state", () => {
  vi.mocked(getActiveExecutionDetail).mockReturnValue(new Promise(() => {}));
  render(<ActiveExecutionDetailView />);
  expect(screen.getByText("Loading execution detail…")).toBeInTheDocument();
});

test("active execution detail renders error state", async () => {
  const { BrowserAPIError } = await import("../api/client");
  vi.mocked(getActiveExecutionDetail).mockRejectedValue(
    new BrowserAPIError("NOT_FOUND", "Execution not found", 404),
  );
  render(<ActiveExecutionDetailView />);
  await vi.waitFor(() => {
    expect(screen.getByText("Execution not found")).toBeInTheDocument();
  });
});

test("terminal activity preserves selected context when the active detail request returns not found", async () => {
  const { BrowserAPIError } = await import("../api/client");
  activityView.current = {
    activities: [{
      instanceId: "11111111-1111-4111-8111-111111111111",
      cursor: "7",
      sessionId: "session-1",
      traceId: "trace-1",
      canonicalSequence: 7,
      timestamp: "2026-07-27T10:05:00Z",
      kind: "TRACE_COMPLETED",
      executionStatus: "COMPLETED",
      summary: "Execution completed",
      details: { outcome: "succeeded" },
    }],
    connected: true,
    continuity: null,
  };
  vi.mocked(getActiveExecutionDetail).mockRejectedValue(
    new BrowserAPIError("NOT_FOUND", "Execution not found", 404),
  );

  render(<ActiveExecutionDetailView />);

  await vi.waitFor(() => {
    expect(screen.getByText("Execution completed. Context is preserved.")).toBeInTheDocument();
  });
  expect(screen.queryByText("Execution not found")).toBeNull();
  expect(screen.getByLabelText("Current execution summary")).toBeInTheDocument();
  expect(screen.getAllByText("Execution completed", { exact: true }).length).toBeGreaterThan(0);
});

test("missed terminal reconciliation preserves context without retaining an active claim", async () => {
  const { BrowserAPIError } = await import("../api/client");
  activityView.current = {
    activities: [],
    connected: true,
    continuity: null,
    baselineObservedAt: "2026-07-27T10:05:00Z",
  };
  vi.mocked(getActiveExecutionDetail)
    .mockResolvedValueOnce(execution)
    .mockRejectedValueOnce(
      new BrowserAPIError("NOT_FOUND", "Execution not found", 404),
    );

  const { rerender } = render(<ActiveExecutionDetailView />);
  await vi.waitFor(() => {
    expect(screen.getByLabelText("Status: ACTIVE")).toBeInTheDocument();
  });

  activityView.current = {
    ...activityView.current,
    baselineObservedAt: "2026-07-27T10:05:30Z",
  };
  rerender(<ActiveExecutionDetailView />);

  await vi.waitFor(() => {
    expect(
      screen.getByText(/No terminal activity was observed/),
    ).toBeInTheDocument();
    expect(
      screen.getByLabelText("Status: OBSERVATION ENDED"),
    ).toBeInTheDocument();
  });
  expect(screen.queryByLabelText("Status: ACTIVE")).toBeNull();
  expect(screen.queryByText("ACTIVE", { exact: true })).toBeNull();
  expect(screen.queryByLabelText("Terminal")).toBeNull();
  expect(screen.queryByRole("link", { name: "Inspect trace" })).toBeNull();
});

test("switching execution identifiers never renders the previous execution snapshot", async () => {
  vi.mocked(getActiveExecutionDetail)
    .mockResolvedValueOnce(execution)
    .mockReturnValueOnce(new Promise(() => {}));
  const { rerender } = render(<ActiveExecutionDetailView />);
  await vi.waitFor(() => {
    expect(screen.getAllByText("session-1").length).toBeGreaterThan(0);
  });

  route.sessionId = "session-2";
  rerender(<ActiveExecutionDetailView />);

  expect(screen.queryByText("session-1")).toBeNull();
});

test("finalization failure is distinct from a completed outcome", async () => {
  const { BrowserAPIError } = await import("../api/client");
  activityView.current = {
    activities: [{
      instanceId: "11111111-1111-4111-8111-111111111111",
      cursor: "8",
      sessionId: "session-1",
      traceId: "trace-1",
      canonicalSequence: 8,
      timestamp: "2026-07-27T10:05:01Z",
      kind: "EXECUTION_OBSERVATION_ENDED",
      executionStatus: "COMPLETED",
      summary: "Trace finalization failed",
      details: { applicationTraceAvailability: "UNAVAILABLE", applicationTraceUnavailableReason: "CORE_FINALIZATION_FAILED" },
    }],
    connected: false,
    continuity: null,
  };
  vi.mocked(getActiveExecutionDetail).mockRejectedValue(
    new BrowserAPIError("NOT_FOUND", "Execution not found", 404),
  );

  render(<ActiveExecutionDetailView />);

  await vi.waitFor(() => {
    expect(
      screen.getByText(
        "Execution observation ended without an outcome because trace finalization failed.",
      ),
    ).toBeInTheDocument();
  });
  expect(screen.getByLabelText("Terminal")).toHaveTextContent("observation ended");
  expect(screen.queryByRole("link", { name: "Inspect trace" })).toBeNull();
});

const acquiredArtifact: AcquiredArtifact = {
  artifactHandle: "handle-abc",
  traceId: "trace-1",
  sessionId: "session-1",
  outcome: "SUCCEEDED",
  finalizedAt: "2026-07-27T10:10:00Z",
  localBytes: 4096,
  acquiredAt: "2026-07-27T10:15:00Z",
  lastUsedAt: "2026-07-27T10:15:00Z",
  expiresAt: "2026-07-27T10:20:00Z",
  hasIdleExpiry: true,
};

function completedActivity(terminalFailureId?: string) {
  return {
    activities: [{
      instanceId: "11111111-1111-4111-8111-111111111111",
      cursor: "7",
      sessionId: "session-1",
      traceId: "trace-1",
      canonicalSequence: 7,
      timestamp: "2026-07-27T10:05:00Z",
      kind: "TRACE_COMPLETED",
      executionStatus: "COMPLETED",
      summary: "Execution completed",
      details: { outcome: terminalFailureId ? "failed" : "succeeded", applicationTraceAvailability: "AVAILABLE", ...(terminalFailureId ? { terminalFailureId } : {}) },
    }],
    connected: true,
    continuity: null,
  };
}

test("completed execution renders acquire button when artifact is available", async () => {
  activityView.current = completedActivity();
  vi.mocked(getActiveExecutionDetail).mockRejectedValue(
    new (await import("../api/client")).BrowserAPIError("NOT_FOUND", "Execution not found", 404),
  );
  render(<ActiveExecutionDetailView />);
  await vi.waitFor(() => {
    expect(screen.getByRole("button", { name: "Acquire for analysis" })).toBeInTheDocument();
  });
});

test("completed execution acquire button calls acquireArtifact and shows success state", async () => {
  activityView.current = completedActivity("failure-terminal");
  vi.mocked(acquireArtifact).mockResolvedValue(acquiredArtifact);
  vi.mocked(getActiveExecutionDetail).mockRejectedValue(
    new (await import("../api/client")).BrowserAPIError("NOT_FOUND", "Execution not found", 404),
  );
  render(<ActiveExecutionDetailView />);
  await vi.waitFor(() => {
    expect(screen.getByRole("button", { name: "Acquire for analysis" })).toBeInTheDocument();
  });
  fireEvent.click(screen.getByRole("button", { name: "Acquire for analysis" }));
  await vi.waitFor(() => {
    expect(screen.getByText("Artifact acquired successfully.")).toBeInTheDocument();
  });
  expect(acquireArtifact).toHaveBeenCalledWith("trace-1", { tabId: "test-tab", csrfToken: "test-token" });
  expect(screen.getByText("handle-abc")).toBeInTheDocument();
  expect(screen.getByRole("link", { name: "Open focused explorer" })).toHaveAttribute("href", "/traces/trace-1?targetScopeId=scope-1&failureId=failure-terminal");
});

test("completed execution acquire button shows error on failure", async () => {
  const { BrowserAPIError } = await import("../api/client");
  activityView.current = completedActivity();
  vi.mocked(acquireArtifact).mockRejectedValue(
    new BrowserAPIError("ARTIFACT_IN_USE", "Artifact in use", 409),
  );
  vi.mocked(getActiveExecutionDetail).mockRejectedValue(
    new BrowserAPIError("NOT_FOUND", "Execution not found", 404),
  );
  render(<ActiveExecutionDetailView />);
  await vi.waitFor(() => {
    expect(screen.getByRole("button", { name: "Acquire for analysis" })).toBeInTheDocument();
  });
  fireEvent.click(screen.getByRole("button", { name: "Acquire for analysis" }));
  await vi.waitFor(() => {
    expect(screen.getByText("Artifact in use")).toBeInTheDocument();
  });
});
