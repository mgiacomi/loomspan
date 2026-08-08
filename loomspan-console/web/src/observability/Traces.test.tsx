import { render, screen } from "@testing-library/react";
import { beforeEach, expect, test, vi } from "vitest";
import type { ReactNode } from "react";
import type { Trace, Page } from "../api/contracts";
import { BrowserAPIError } from "../api/client";
import userEvent from "@testing-library/user-event";

const view = vi.hoisted(() => ({
  current: undefined as unknown as {
    traces: { targetScopeId: string | null; items: Trace[]; hasMore: boolean; nextCursor: string | null; loading: boolean; loaded: boolean; error?: BrowserAPIError };
    loadTraces: ReturnType<typeof vi.fn>;
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

import { Traces } from "./Traces";

const trace: Trace = {
  targetScopeId: "scope-1",
  traceId: "trace-1",
  sessionId: "session-1",
  entrySkill: "CheckDns",
  outcome: "SUCCEEDED",
  finalizedAt: "2026-07-27T10:10:00Z",
  sizeBytes: 4096,
  persistencePolicy: "PERSISTENT",
  applicationTraceExpiresAt: "2026-08-03T10:10:00Z",
  localAvailable: false,
};

beforeEach(() => {
  view.current = {
    traces: { targetScopeId: "scope-1", items: [], hasMore: false, nextCursor: null, loading: false, loaded: true },
    loadTraces: vi.fn(),
  };
});

test("traces renders items in a table", () => {
  view.current.traces.items = [trace];
  render(<Traces />);
  expect(screen.getByText("trace-1")).toBeInTheDocument();
  expect(screen.getAllByRole("columnheader")[0]).toHaveTextContent("Entry skill");
  expect(screen.getAllByRole("cell")[0]).toHaveTextContent("CheckDns");
  expect(screen.getByText("session-1")).toBeInTheDocument();
  expect(screen.getByText("SUCCEEDED")).toBeInTheDocument();
  expect(screen.getByText("4096")).toBeInTheDocument();
  expect(screen.getByText("PERSISTENT")).toBeInTheDocument();
  expect(screen.getByRole("link", { name: "trace-1" })).toHaveAttribute(
    "href",
    "/traces/trace-1?targetScopeId=scope-1",
  );
});

test("entry skill is inert text and trace ID remains the detail link", () => {
  view.current.traces.items = [{ ...trace, entrySkill: '<img src=x onerror="alert(1)">' }];
  render(<Traces />);
  expect(screen.getByText('<img src=x onerror="alert(1)">')).toBeInTheDocument();
  expect(document.querySelector("img")).toBeNull();
  expect(screen.getAllByRole("link")).toHaveLength(1);
});

test("traces states finalized and expiry times as calendar dates rather than raw ISO", () => {
  view.current.traces.items = [trace];
  render(<Traces />);
  const cells = screen
    .getAllByRole("cell")
    .map((cell) => cell.textContent ?? "")
    .filter((text) => /^\d{2}\/\d{2}\/\d{4} \d{2}:\d{2}:\d{2}$/.test(text));
  expect(cells).toHaveLength(2);
  expect(screen.queryByText(trace.finalizedAt)).toBeNull();
  expect(screen.queryByText(trace.applicationTraceExpiresAt)).toBeNull();
});

test("traces renders empty state", () => {
  render(<Traces />);
  expect(screen.getByText("No traces are cataloged.")).toBeInTheDocument();
});

test("traces renders loading state", () => {
  view.current.traces.loading = true;
  render(<Traces />);
  expect(screen.getByText("Loading traces…")).toBeInTheDocument();
});

test("traces renders error state", () => {
  view.current.traces.error = new BrowserAPIError("TARGET_UNAVAILABLE", "Target down", 503);
  render(<Traces />);
  expect(screen.getByText("Target down")).toBeInTheDocument();
});

test("traces shows retry button on error", () => {
  view.current.traces.error = new BrowserAPIError("TARGET_UNAVAILABLE", "Target down", 503);
  render(<Traces />);
  expect(screen.getByText("Retry")).toBeInTheDocument();
});

test("traces shows load more button when hasMore is true", () => {
  view.current.traces.items = [trace];
  view.current.traces.hasMore = true;
  view.current.traces.nextCursor = "cursor-1";
  render(<Traces />);
  expect(screen.getByText("Load more")).toBeInTheDocument();
});

test("trace actions request refresh, retry, and continuation", async () => {
  view.current.traces.items = [trace];
  view.current.traces.hasMore = true;
  view.current.traces.nextCursor = "cursor-1";
  view.current.traces.error = new BrowserAPIError("TARGET_UNAVAILABLE", "down", 503);
  render(<Traces />);
  await userEvent.click(screen.getByRole("button", { name: "Refresh" }));
  await userEvent.click(screen.getByRole("button", { name: "Retry" }));
  await userEvent.click(screen.getByRole("button", { name: "Load more" }));
  expect(view.current.loadTraces.mock.calls).toEqual([[], [], ["cursor-1"]]);
});
