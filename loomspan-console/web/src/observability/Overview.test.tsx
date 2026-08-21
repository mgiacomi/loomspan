import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, expect, test, vi } from "vitest";
import type { ReactNode } from "react";
import type { InstanceStatus, SkillSummary } from "../api/contracts";
import { BrowserAPIError } from "../api/client";

const observabilityView = vi.hoisted(() => ({
  current: undefined as unknown as {
    instance: { status: InstanceStatus | undefined; error: BrowserAPIError | undefined };
    skills: { items: SkillSummary[]; hasMore: boolean; nextCursor: string | null; loading: boolean; error?: BrowserAPIError };
    activeExecutions: { targetScopeId: string | null; items: unknown[]; hasMore: boolean; nextCursor: string | null; observedAt: string | null; loading: boolean; loaded: boolean; error?: BrowserAPIError };
    loadInstance: ReturnType<typeof vi.fn>;
    loadSkills: ReturnType<typeof vi.fn>;
    loadActiveExecutions: ReturnType<typeof vi.fn>;
    loadTraces: ReturnType<typeof vi.fn>;
  },
}));
const routerView = vi.hoisted(() => ({
  state: null as { staleTargetScope?: boolean } | null,
}));

vi.mock("./ObservabilityProvider", () => ({
  useObservability: () => observabilityView.current,
  useOptionalObservability: () => observabilityView.current,
  ObservabilityProvider: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

vi.mock("../target/TargetProvider", () => ({
  useTarget: () => ({
    target: {
      address: "https://application.example",
      status: {
        targetScopeId: "scope-1",
        targetSelection: "SELECTED",
        targetConnection: "REACHABLE",
        targetAuthentication: "ESTABLISHED",
        javaGoCompatibility: "COMPATIBLE",
        runtimeIdentity: "ESTABLISHED",
        liveMonitoring: "AVAILABLE",
      },
    },
  }),
}));

const activityView = vi.hoisted(() => ({
  current: {
    activities: [],
    recentCompletions: [],
    connected: false,
    connectionFact: null,
    error: null,
    loading: false,
    beginningUnavailable: false,
    continuity: null,
    loadRecent: vi.fn(),
  },
}));

vi.mock("../activity/ActivityProvider", () => ({
  useActivity: () => activityView.current,
  ActivityProvider: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

vi.mock("react-router", () => ({
  Link: ({ children, to, ...rest }: { children: ReactNode; to: string }) => (
    <a href={to} {...rest}>{children}</a>
  ),
  useLocation: () => ({ state: routerView.state }),
}));

import { ObservabilityOverview } from "./Overview";

const instanceStatus: InstanceStatus = {
  targetScopeId: "scope-1",
  instanceId: "11111111-1111-4111-8111-111111111111",
  consoleCompatibilityVersion: "0.1.0-SNAPSHOT",
  observedAt: "2026-07-27T00:00:00Z",
  liveMonitoringAvailable: true,
  registeredSkillCount: 3,
  activeExecutionCount: 1,
  catalogedTraceCount: 5,
  tracePersistencePolicy: "PERSISTENT",
  completionGraceTtl: "PT2M",
  traceCatalogMetadataTtl: "PT168H",
};

beforeEach(() => {
  routerView.state = null;
  observabilityView.current = {
    instance: { status: undefined, error: undefined },
    skills: { items: [], hasMore: false, nextCursor: null, loading: false },
    activeExecutions: { targetScopeId: "scope-1", items: [], hasMore: false, nextCursor: null, observedAt: null, loading: false, loaded: true },
    loadInstance: vi.fn(),
    loadSkills: vi.fn(),
    loadActiveExecutions: vi.fn(),
    loadTraces: vi.fn(),
  };
});

test("overview does not repeat facts the global context banner already states", () => {
  observabilityView.current.instance.status = instanceStatus;
  render(<ObservabilityOverview />);
  expect(screen.queryByText("11111111-1111-4111-8111-111111111111")).not.toBeInTheDocument();
  expect(screen.queryByText("https://application.example")).not.toBeInTheDocument();
  expect(screen.queryByText("REACHABLE")).not.toBeInTheDocument();
  expect(screen.queryByText("ESTABLISHED")).not.toBeInTheDocument();
});

test("overview reports instance observation time with the control that refreshes it", () => {
  observabilityView.current.instance.status = instanceStatus;
  render(<ObservabilityOverview />);
  expect(screen.getByText("Observed 2026-07-27T00:00:00Z")).toBeVisible();
  expect(screen.getByRole("button", { name: "Refresh" })).toBeVisible();
});

test("overview states catalog counts on the links that navigate to them", () => {
  observabilityView.current.instance.status = instanceStatus;
  render(<ObservabilityOverview />);
  expect(screen.getByRole("link", { name: "Skill Catalog, 3 registered skills" })).toBeVisible();
  expect(screen.getByRole("link", { name: "Active Executions, 1 active execution" })).toBeVisible();
  expect(screen.getByRole("link", { name: "Trace Catalog, 5 cataloged traces" })).toBeVisible();
});

test("overview collapses static configuration facts behind a disclosure", () => {
  observabilityView.current.instance.status = instanceStatus;
  render(<ObservabilityOverview />);
  const disclosure = screen.getByText("Instance configuration");
  expect(disclosure).toBeVisible();
  for (const value of ["0.1.0-SNAPSHOT", "PERSISTENT", "PT2M", "PT168H"]) {
    expect(screen.getByText(value)).not.toBeVisible();
  }
  expect(
    screen.getByText(
      "Catalog metadata TTL and core file retention are independent. Neither provides cross-restart history.",
    ),
  ).not.toBeVisible();

  fireEvent.click(disclosure);

  for (const value of ["0.1.0-SNAPSHOT", "PERSISTENT", "PT2M", "PT168H"]) {
    expect(screen.getByText(value)).toBeVisible();
  }
});

test("overview keeps unavailable live monitoring visible rather than collapsed", () => {
  observabilityView.current.instance.status = { ...instanceStatus, liveMonitoringAvailable: false };
  render(<ObservabilityOverview />);
  expect(screen.getByText("Live monitoring is unavailable for this instance.")).toBeVisible();
});

test("overview renders error message when instance load fails", () => {
  const error = new BrowserAPIError("TARGET_UNAVAILABLE", "Target is down", 503);
  observabilityView.current.instance.error = error;
  render(<ObservabilityOverview />);
  expect(screen.getByText("Target is down")).toBeInTheDocument();
});

test("overview does not duplicate the workspace instance request", () => {
  const loadInstance = vi.fn();
  observabilityView.current.instance = null as never;
  observabilityView.current.loadInstance = loadInstance;
  render(<ObservabilityOverview />);
  expect(loadInstance).not.toHaveBeenCalled();
});

test("overview explains that a stale target-bound view was discarded", () => {
  routerView.state = { staleTargetScope: true };
  render(<ObservabilityOverview />);
  expect(
    screen.getByText("The selected target changed. The previous view was discarded."),
  ).toBeInTheDocument();
});

test("overview renders loading state when instance is loading", () => {
  observabilityView.current.instance = { status: undefined, error: undefined, loading: true } as never;
  render(<ObservabilityOverview />);
  expect(screen.getByText("Loading instance overview…")).toBeInTheDocument();
});
