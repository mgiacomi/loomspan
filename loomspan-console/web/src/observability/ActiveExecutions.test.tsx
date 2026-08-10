import { render, screen } from "@testing-library/react";
import { beforeEach, expect, test, vi } from "vitest";
import type { ReactNode } from "react";
import type { ActiveExecution, ActivePage } from "../api/contracts";
import { BrowserAPIError } from "../api/client";
import userEvent from "@testing-library/user-event";

const view = vi.hoisted(() => ({
  current: undefined as unknown as {
    activeExecutions: { targetScopeId: string | null; items: ActiveExecution[]; hasMore: boolean; nextCursor: string | null; resumeCursor: string | null; loading: boolean; loaded: boolean; error?: BrowserAPIError };
    loadActiveExecutions: ReturnType<typeof vi.fn>;
  },
}));

vi.mock("./ObservabilityProvider", () => ({
  useObservability: () => view.current,
  ObservabilityProvider: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

vi.mock("react-router", () => ({
  Link: ({ children, to }: { children: ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}));

import { ActiveExecutions } from "./ActiveExecutions";

const execution: ActiveExecution = {
  targetScopeId: "scope-1",
  sessionId: "session-1",
  traceId: "trace-1",
  lastCanonicalSequence: 42,
  startedAt: "2026-07-27T10:00:00Z",
  updatedAt: "2026-07-27T10:05:00Z",
  elapsedMillis: 300000,
  entrySkill: "CheckDns",
  status: "RUNNING",
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
  view.current = {
    activeExecutions: { targetScopeId: "scope-1", items: [], hasMore: false, nextCursor: null, resumeCursor: null, loading: false, loaded: true },
    loadActiveExecutions: vi.fn(),
  };
});

test("active executions renders items in a table", () => {
  view.current.activeExecutions.items = [execution];
  render(<ActiveExecutions />);
  expect(screen.getByText("session-1")).toBeInTheDocument();
  expect(screen.getByText("CheckDns")).toBeInTheDocument();
  expect(screen.getByText("RUNNING")).toBeInTheDocument();
  expect(screen.getByText("EXECUTING")).toBeInTheDocument();
  expect(screen.getByText("Checking DNS records")).toBeInTheDocument();
  expect(screen.getByRole("link", { name: "session-1" })).toHaveAttribute(
    "href",
    "/active-executions/session-1?targetScopeId=scope-1",
  );
});

test("active executions renders empty state", () => {
  render(<ActiveExecutions />);
  expect(screen.getByText("No active executions.")).toBeInTheDocument();
});

test("active executions renders loading state", () => {
  view.current.activeExecutions.loading = true;
  render(<ActiveExecutions />);
  expect(screen.getByText("Loading active executions…")).toBeInTheDocument();
});

test("active executions renders error state", () => {
  view.current.activeExecutions.error = new BrowserAPIError("LIVE_MONITORING_UNAVAILABLE", "Live monitoring off", 409);
  render(<ActiveExecutions />);
  expect(screen.getByText("Live monitoring off")).toBeInTheDocument();
});

test("active executions shows retry button on error", () => {
  view.current.activeExecutions.error = new BrowserAPIError("LIVE_MONITORING_UNAVAILABLE", "Live monitoring off", 409);
  render(<ActiveExecutions />);
  expect(screen.getByText("Retry")).toBeInTheDocument();
});

test("active executions preserves resume cursor without exposing it as pagination", () => {
  view.current.activeExecutions.items = [execution];
  view.current.activeExecutions.hasMore = false;
  view.current.activeExecutions.resumeCursor = "resume-cursor-1";
  render(<ActiveExecutions />);
  expect(screen.queryByText("Resume")).not.toBeInTheDocument();
});

test("active execution actions request refresh, retry, and continuation", async () => {
  view.current.activeExecutions.items = [execution];
  view.current.activeExecutions.hasMore = true;
  view.current.activeExecutions.nextCursor = "cursor-1";
  view.current.activeExecutions.error = new BrowserAPIError("LIVE_MONITORING_UNAVAILABLE", "off", 409);
  render(<ActiveExecutions />);
  await userEvent.click(screen.getByRole("button", { name: "Refresh" }));
  await userEvent.click(screen.getByRole("button", { name: "Retry" }));
  await userEvent.click(screen.getByRole("button", { name: "Load more" }));
  expect(view.current.loadActiveExecutions.mock.calls).toEqual([[], [], ["cursor-1"]]);
});
