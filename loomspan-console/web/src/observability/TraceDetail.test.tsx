import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, expect, test, vi } from "vitest";
import type { ReactNode } from "react";
import type { AcquiredArtifact, Trace } from "../api/contracts";

const route = vi.hoisted(() => ({
  scope: "scope-1",
  navigate: vi.fn(),
}));

vi.mock("../api/client", () => ({
  getTraceDetail: vi.fn(),
  acquireArtifact: vi.fn(),
  getTraceAnalysisSummary: vi.fn().mockResolvedValue({ targetScopeId: "scope-1", traceId: "trace-1", sessionId: "session-1", outcome: "SUCCEEDED", terminalFailureId: null, recordCount: 0, frameCount: 0, rootFrameIds: [], usageComplete: true }),
  getTraceFrames: vi.fn().mockResolvedValue({ targetScopeId: "scope-1", items: [], hasMore: false, nextCursor: null }),
  getTraceRecords: vi.fn(), getTraceUsage: vi.fn(), getPayloadRange: vi.fn(), getRawRecordRange: vi.fn(),
  getTraceAttempts: vi.fn(), getTraceFailures: vi.fn(), getTraceValidationLinks: vi.fn(), getTraceGaps: vi.fn(), getTraceUncertainties: vi.fn(), getTracePayloads: vi.fn(),
  searchTraceEvidence: vi.fn(),
  rawArtifactDownloadURL: (traceId: string) => `/api/console/v1/artifacts/${encodeURIComponent(traceId)}/raw`,
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
  useParams: () => ({ traceId: "trace-1" }),
  useNavigate: () => route.navigate,
  useSearchParams: () => [new URLSearchParams({ targetScopeId: route.scope }), vi.fn()],
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

import { getTraceDetail, acquireArtifact, getTraceAnalysisSummary } from "../api/client";
import { TraceDetailView } from "./TraceDetail";

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
  vi.mocked(getTraceDetail).mockReset();
  vi.mocked(acquireArtifact).mockReset();
  route.scope = "scope-1";
  route.navigate.mockReset();
});

test("stale trace deep link resets before requesting the identifier", async () => {
  route.scope = "scope-old";
  render(<TraceDetailView />);
  await vi.waitFor(() => expect(route.navigate).toHaveBeenCalled());
  expect(getTraceDetail).not.toHaveBeenCalled();
});

test("trace detail renders facts when loaded", async () => {
  vi.mocked(getTraceDetail).mockResolvedValue(trace);
  render(<TraceDetailView />);
  await vi.waitFor(() => {
    expect(screen.getByText("trace-1")).toBeInTheDocument();
  });
  expect(screen.getByText("session-1")).toBeInTheDocument();
  expect(screen.getByText("CheckDns")).toBeInTheDocument();
  expect(screen.getByText("SUCCEEDED")).toBeInTheDocument();
  expect(screen.getByText("PERSISTENT")).toBeInTheDocument();
});

test("trace detail leads with a terminal outcome rather than a live status", async () => {
  vi.mocked(getTraceDetail).mockResolvedValue(trace);
  render(<TraceDetailView />);
  const outcome = await screen.findByLabelText("Outcome: SUCCEEDED");
  expect(outcome).toHaveClass("outcome");
  expect(outcome).not.toHaveClass("running");
  expect(screen.getByText(/^Finalized \d{2}\/\d{2}\/\d{4} \d{2}:\d{2}:\d{2}$/)).toBeVisible();
  expect(screen.queryByText(trace.finalizedAt)).toBeNull();
  expect(screen.queryByText(trace.applicationTraceExpiresAt)).toBeNull();
});

test("trace detail states artifact state beside the actions that change it", async () => {
  vi.mocked(getTraceDetail).mockResolvedValue(trace);
  render(<TraceDetailView />);
  await screen.findByText("trace-1");
  const artifactState = screen.getByLabelText("Artifact state");
  const actions = artifactState.closest(".trace-actions");
  expect(actions).not.toBeNull();
  expect(actions?.querySelector("h3")?.textContent).toBe("Artifact actions");
  for (const label of ["Local artifact", "Size (bytes)", "Application availability at acquisition"]) {
    expect(artifactState).toHaveTextContent(label);
  }
  const summary = screen.getByLabelText("Finalized trace summary");
  expect(summary).not.toHaveTextContent("Local artifact");
  expect(summary).not.toHaveTextContent("Application availability at acquisition");
});

test("trace detail renders entry skill as inert text before acquisition", async () => {
  vi.mocked(getTraceDetail).mockResolvedValue({ ...trace, entrySkill: "<script>bad()</script>" });
  render(<TraceDetailView />);
  expect(await screen.findByText("<script>bad()</script>")).toBeInTheDocument();
  expect(document.querySelector("script")).toBeNull();
  expect(acquireArtifact).not.toHaveBeenCalled();
});

test("trace detail renders loading state", () => {
  vi.mocked(getTraceDetail).mockReturnValue(new Promise(() => {}));
  render(<TraceDetailView />);
  expect(screen.getByText("Loading trace detail…")).toBeInTheDocument();
});

test("trace detail renders error state", async () => {
  const { BrowserAPIError } = await import("../api/client");
  vi.mocked(getTraceDetail).mockRejectedValue(
    new BrowserAPIError("NOT_FOUND", "Trace not found", 404),
  );
  render(<TraceDetailView />);
  await vi.waitFor(() => {
    expect(screen.getByText("Trace not found")).toBeInTheDocument();
  });
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

test("trace detail requires confirmation before raw download", async () => {
  vi.mocked(getTraceDetail).mockResolvedValue(trace);
  render(<TraceDetailView />);
  await vi.waitFor(() => {
    expect(screen.getByText("trace-1")).toBeInTheDocument();
  });
  expect(screen.getByRole("button", { name: "Acquire for analysis" })).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Download raw attachment" }));
  const link = screen.getByRole("link", { name: "Confirm raw attachment download" });
  expect(link).toHaveAttribute("download");
  expect(link.getAttribute("href")).toContain(encodeURIComponent("trace-1"));
});

test("raw download cancellation does not navigate", async () => {
  vi.mocked(getTraceDetail).mockResolvedValue(trace);
  render(<TraceDetailView />);
  await screen.findByText("trace-1");
  fireEvent.click(screen.getByRole("button", { name: "Download raw attachment" }));
  const cancel = screen.getByRole("button", { name: "Cancel" });
  expect(cancel).toHaveFocus();
  fireEvent.keyDown(cancel, { key: "Tab" });
  expect(screen.getByRole("link", { name: "Confirm raw attachment download" })).toHaveFocus();
  fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
  expect(screen.queryByRole("dialog")).toBeNull();
  await vi.waitFor(() => expect(screen.getByRole("button", { name: "Download raw attachment" })).toHaveFocus());
  expect(route.navigate).not.toHaveBeenCalled();
});

test("acquire button calls acquireArtifact and shows success state", async () => {
  vi.mocked(getTraceDetail).mockResolvedValue(trace);
  vi.mocked(acquireArtifact).mockResolvedValue(acquiredArtifact);
  render(<TraceDetailView />);
  await vi.waitFor(() => {
    expect(screen.getByText("trace-1")).toBeInTheDocument();
  });
  fireEvent.click(screen.getByRole("button", { name: "Acquire for analysis" }));
  await vi.waitFor(() => {
    expect(screen.getByText("Artifact acquired successfully.")).toBeInTheDocument();
  });
  expect(acquireArtifact).toHaveBeenCalledWith("trace-1", { tabId: "test-tab", csrfToken: "test-token" });
  expect(screen.queryByText("handle-abc")).not.toBeInTheDocument();
  expect(screen.getAllByText("4096").length).toBeGreaterThan(0);
});

test("expired explorer artifact returns trace detail to reacquisition state", async () => {
  const { BrowserAPIError } = await import("../api/client");
  vi.mocked(getTraceDetail).mockResolvedValue({ ...trace, localAvailable: true });
  vi.mocked(getTraceAnalysisSummary).mockRejectedValueOnce(new BrowserAPIError("ARTIFACT_EXPIRED", "The local artifact expired.", 409));
  render(<TraceDetailView />);
  await screen.findByText("The local artifact expired.");
  expect(screen.getByRole("button", { name: "Acquire for analysis" })).toBeInTheDocument();
  expect(screen.queryByRole("heading", { name: "Trace explorer" })).toBeNull();
  expect(route.navigate).toHaveBeenCalledWith("/traces/trace-1?targetScopeId=scope-1", { replace: true });
});

test("acquire button shows error on failure", async () => {
  const { BrowserAPIError } = await import("../api/client");
  vi.mocked(getTraceDetail).mockResolvedValue(trace);
  vi.mocked(acquireArtifact).mockRejectedValue(
    new BrowserAPIError("ARTIFACT_IN_USE", "Artifact in use", 409),
  );
  render(<TraceDetailView />);
  await vi.waitFor(() => {
    expect(screen.getByText("trace-1")).toBeInTheDocument();
  });
  fireEvent.click(screen.getByRole("button", { name: "Acquire for analysis" }));
  await vi.waitFor(() => {
    expect(screen.getByText("Artifact in use")).toBeInTheDocument();
  });
  expect(screen.getByRole("alert")).toBeInTheDocument();
});

test("trace detail shows application availability and local artifact status", async () => {
  const traceWithAvailability: Trace = {
    ...trace,
    applicationAvailability: "AVAILABLE",
    localAvailable: true,
  };
  vi.mocked(getTraceDetail).mockResolvedValue(traceWithAvailability);
  render(<TraceDetailView />);
  await vi.waitFor(() => {
    expect(screen.getByText("AVAILABLE")).toBeInTheDocument();
  });
  expect(screen.getByText("Available")).toBeInTheDocument();
});

test("trace detail shows that local acquisition availability was not observed", async () => {
  vi.mocked(getTraceDetail).mockResolvedValue(trace);
  render(<TraceDetailView />);
  await vi.waitFor(() => {
    expect(screen.getByText("Not observed locally")).toBeInTheDocument();
  });
});
