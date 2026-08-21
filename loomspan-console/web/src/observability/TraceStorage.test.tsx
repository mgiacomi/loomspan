import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, expect, test, vi } from "vitest";
import type { ReactNode } from "react";
import type { StorageSnapshot, StoredEntry } from "../api/contracts";

const route = vi.hoisted(() => ({
  scope: "scope-1",
  navigate: vi.fn(),
}));
const browserSession = vi.hoisted(() => ({
  getSecurity: () => ({ tabId: "test-tab", csrfToken: "test-token" }),
}));

vi.mock("../api/client", () => ({
  getStorageSnapshot: vi.fn(),
  removeArtifact: vi.fn(),
  clearExpiredArtifacts: vi.fn(),
  clearAllUnusedArtifacts: vi.fn(),
	importTraceFile: vi.fn(),
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
  useBrowserSession: () => browserSession,
}));

import {
  clearAllUnusedArtifacts,
  clearExpiredArtifacts,
  getStorageSnapshot,
	importTraceFile,
  removeArtifact,
} from "../api/client";
import { TraceStorage } from "./TraceStorage";

const baseEntry: StoredEntry = {
  source: "TARGET",
  traceId: "trace-1",
      sessionId: "session-1",
      outcome: "SUCCEEDED",
      persistencePolicy: "RETAINED",
  finalizedAt: "2026-07-27T10:10:00Z",
  acquiredAt: "2026-07-27T10:05:00Z",
  lastUsedAt: "2026-07-27T10:30:00Z",
  expiresAt: "2026-07-28T10:05:00Z",
  hasIdleExpiry: true,
  localBytes: 4096,
  applicationTraceExpiresAt: "2026-08-03T10:10:00Z",
  applicationAvailability: "available",
  localAvailable: true,
  activePin: false,
};

function makeSnapshot(overrides: Partial<StorageSnapshot> = {}): StorageSnapshot {
  return {
    workspaceLabel: "My Workspace",
    maxBytes: 1048576,
    unlimited: false,
    idleTtl: "24h",
    neverExpire: false,
    chargedBytes: 2048,
    acquiredCount: 1,
    entries: [baseEntry],
    ...overrides,
  };
}

beforeEach(() => {
  vi.mocked(getStorageSnapshot).mockReset();
  vi.mocked(removeArtifact).mockReset();
  vi.mocked(clearExpiredArtifacts).mockReset();
  vi.mocked(clearAllUnusedArtifacts).mockReset();
	vi.mocked(importTraceFile).mockReset();
  route.scope = "scope-1";
  route.navigate.mockReset();
});

test("opens the file picker and imports a selected trace immediately", async () => {
  vi.mocked(getStorageSnapshot).mockResolvedValue(makeSnapshot());
  vi.mocked(importTraceFile).mockResolvedValue({ source: "IMPORTED", artifactHandle: "h", traceId: "trace-import", sessionId: "session-import", outcome: "SUCCEEDED", finalizedAt: "2026-08-12T00:00:00Z", localBytes: 12, acquiredAt: "2026-08-12T00:00:01Z", lastUsedAt: "2026-08-12T00:00:01Z", expiresAt: "2026-08-12T01:00:01Z", hasIdleExpiry: true });
  render(<TraceStorage />);
  const input = screen.getByLabelText("Trace files");
  const picker = vi.spyOn(input, "click");
  fireEvent.click(screen.getByRole("button", { name: "Import Trace File" }));
  expect(picker).toHaveBeenCalled();

  const file = new File(["{}\n"], "trace.ndjson", { type: "application/x-ndjson" });
  fireEvent.change(input, { target: { files: [file] } });
  await vi.waitFor(() => expect(importTraceFile).toHaveBeenCalledWith(file, { tabId: "test-tab", csrfToken: "test-token" }));
  expect(route.navigate).toHaveBeenCalledWith("/traces/imported/trace-import");
});

test("imports dropped files sequentially and reports batch results without navigating", async () => {
  const { BrowserAPIError } = await import("../api/client");
  vi.mocked(getStorageSnapshot).mockResolvedValue(makeSnapshot());
  let finishFirst: ((value: Awaited<ReturnType<typeof importTraceFile>>) => void) | undefined;
  vi.mocked(importTraceFile)
    .mockImplementationOnce(() => new Promise((resolve) => { finishFirst = resolve; }))
    .mockRejectedValueOnce(new BrowserAPIError("INVALID_ARTIFACT", "Invalid trace file", 400));
  render(<TraceStorage />);
  const first = new File(["{}\n"], "first.ndjson", { type: "application/x-ndjson" });
  const second = new File(["bad\n"], "second.ndjson", { type: "application/x-ndjson" });

  fireEvent.drop(screen.getByRole("group", { name: "Trace file import" }), { dataTransfer: { files: [first, second] } });
  await vi.waitFor(() => expect(importTraceFile).toHaveBeenCalledTimes(1));
  expect(screen.getByRole("status")).toHaveTextContent("Importing 1 of 2");

  finishFirst?.({ source: "IMPORTED", artifactHandle: "h", traceId: "trace-first", sessionId: "session-import", outcome: "SUCCEEDED", finalizedAt: "2026-08-12T00:00:00Z", localBytes: 12, acquiredAt: "2026-08-12T00:00:01Z", lastUsedAt: "2026-08-12T00:00:01Z", expiresAt: "2026-08-12T01:00:01Z", hasIdleExpiry: true });
  await vi.waitFor(() => expect(importTraceFile).toHaveBeenCalledTimes(2));
  expect(importTraceFile).toHaveBeenNthCalledWith(2, second, { tabId: "test-tab", csrfToken: "test-token" });
  const results = await screen.findByRole("region", { name: "Trace import results" });
  await vi.waitFor(() => expect(results).toHaveTextContent("first.ndjson: Imported as trace-first"));
  expect(results).toHaveTextContent("second.ndjson: Invalid trace file");
  expect(route.navigate).not.toHaveBeenCalled();
  await vi.waitFor(() => expect(getStorageSnapshot).toHaveBeenCalledTimes(2));
});

test("trace storage renders loading state", () => {
  vi.mocked(getStorageSnapshot).mockReturnValue(new Promise(() => {}));
  render(<TraceStorage />);
  expect(screen.getByText("Loading storage snapshot…")).toBeInTheDocument();
});

test("trace storage renders snapshot when loaded", async () => {
  vi.mocked(getStorageSnapshot).mockResolvedValue(makeSnapshot());
  render(<TraceStorage />);
  await vi.waitFor(() => {
    expect(screen.getByText("My Workspace")).toBeInTheDocument();
  });
  expect(screen.getByText("2048")).toBeInTheDocument();
  expect(screen.getByText("trace-1")).toBeInTheDocument();
  expect(screen.getByText("session-1")).toBeInTheDocument();
  expect(screen.getByText("SUCCEEDED")).toBeInTheDocument();
});

test("trace storage renders error state", async () => {
  const { BrowserAPIError } = await import("../api/client");
  vi.mocked(getStorageSnapshot).mockRejectedValue(
    new BrowserAPIError("NOT_FOUND", "Snapshot unavailable", 404),
  );
  render(<TraceStorage />);
  await vi.waitFor(() => {
    expect(screen.getByText("Snapshot unavailable")).toBeInTheDocument();
  });
  expect(screen.getByText("Snapshot unavailable").closest(".target-error")).toHaveAttribute(
    "role",
    "alert",
  );
});

test("trace storage renders empty state", async () => {
  vi.mocked(getStorageSnapshot).mockResolvedValue(makeSnapshot({ entries: [] }));
  render(<TraceStorage />);
  await vi.waitFor(() => {
    expect(screen.getByText("No artifacts are currently stored.")).toBeInTheDocument();
  });
});

test("remove requires confirmation before calling removeArtifact", async () => {
  vi.mocked(getStorageSnapshot).mockResolvedValue(makeSnapshot());
  vi.mocked(removeArtifact).mockResolvedValue({ removed: true });
  render(<TraceStorage />);
  await vi.waitFor(() => {
    expect(screen.getByText("Remove")).toBeInTheDocument();
  });
  fireEvent.click(screen.getByText("Remove"));
  expect(removeArtifact).not.toHaveBeenCalled();
  expect(screen.getByText("Confirm")).toBeInTheDocument();
  fireEvent.click(screen.getByText("Confirm"));
  await vi.waitFor(() => {
    expect(removeArtifact).toHaveBeenCalledWith("trace-1", "TARGET", {
      tabId: "test-tab",
      csrfToken: "test-token",
    });
  });
  await vi.waitFor(() => {
    expect(vi.mocked(getStorageSnapshot).mock.calls.length).toBeGreaterThan(1);
  });
});

test("remove cancel does not call removeArtifact", async () => {
  vi.mocked(getStorageSnapshot).mockResolvedValue(makeSnapshot());
  render(<TraceStorage />);
  await vi.waitFor(() => {
    expect(screen.getByText("Remove")).toBeInTheDocument();
  });
  fireEvent.click(screen.getByText("Remove"));
  fireEvent.click(screen.getByText("Cancel"));
  expect(removeArtifact).not.toHaveBeenCalled();
  expect(screen.getByText("Remove")).toBeInTheDocument();
});

test("pinned entry shows In use instead of Remove", async () => {
  vi.mocked(getStorageSnapshot).mockResolvedValue(
    makeSnapshot({ entries: [{ ...baseEntry, activePin: true }] }),
  );
  render(<TraceStorage />);
  await vi.waitFor(() => {
    expect(screen.getByText("In use")).toBeInTheDocument();
  });
  expect(screen.queryByText("Remove")).not.toBeInTheDocument();
});

test("clear expired requires confirmation", async () => {
  vi.mocked(getStorageSnapshot).mockResolvedValue(makeSnapshot());
  vi.mocked(clearExpiredArtifacts).mockResolvedValue({ cleared: true });
  render(<TraceStorage />);
  await vi.waitFor(() => {
    expect(screen.getByText("Clear expired")).toBeInTheDocument();
  });
  fireEvent.click(screen.getByText("Clear expired"));
  expect(clearExpiredArtifacts).not.toHaveBeenCalled();
  fireEvent.click(screen.getByText("Confirm clear expired"));
  await vi.waitFor(() => {
    expect(clearExpiredArtifacts).toHaveBeenCalledWith({
      tabId: "test-tab",
      csrfToken: "test-token",
    });
  });
});

test("clear all unused requires confirmation", async () => {
  vi.mocked(getStorageSnapshot).mockResolvedValue(makeSnapshot());
  vi.mocked(clearAllUnusedArtifacts).mockResolvedValue({ cleared: true });
  render(<TraceStorage />);
  await vi.waitFor(() => {
    expect(screen.getByText("Clear all unused")).toBeInTheDocument();
  });
  fireEvent.click(screen.getByText("Clear all unused"));
  expect(clearAllUnusedArtifacts).not.toHaveBeenCalled();
  fireEvent.click(screen.getByText("Confirm clear all unused"));
  await vi.waitFor(() => {
    expect(clearAllUnusedArtifacts).toHaveBeenCalledWith({
      tabId: "test-tab",
      csrfToken: "test-token",
    });
  });
});

test("clear cancel does not call the API", async () => {
  vi.mocked(getStorageSnapshot).mockResolvedValue(makeSnapshot());
  render(<TraceStorage />);
  await vi.waitFor(() => {
    expect(screen.getByText("Clear expired")).toBeInTheDocument();
  });
  fireEvent.click(screen.getByText("Clear expired"));
  fireEvent.click(screen.getByText("Cancel"));
  expect(clearExpiredArtifacts).not.toHaveBeenCalled();
  expect(screen.getByText("Clear expired")).toBeInTheDocument();
});

test("action error is displayed", async () => {
  const { BrowserAPIError } = await import("../api/client");
  vi.mocked(getStorageSnapshot).mockResolvedValue(makeSnapshot());
  vi.mocked(removeArtifact).mockRejectedValue(
    new BrowserAPIError("ARTIFACT_IN_USE", "Remove failed", 409),
  );
  render(<TraceStorage />);
  await vi.waitFor(() => {
    expect(screen.getByText("Remove")).toBeInTheDocument();
  });
  fireEvent.click(screen.getByText("Remove"));
  fireEvent.click(screen.getByText("Confirm"));
  await vi.waitFor(() => {
    expect(screen.getByText("Remove failed")).toBeInTheDocument();
  });
  expect(screen.getByText("Remove failed").closest(".target-error")).toHaveAttribute(
    "role",
    "alert",
  );
});

test("unlimited and never expire sentinels display correctly", async () => {
  vi.mocked(getStorageSnapshot).mockResolvedValue(
    makeSnapshot({ unlimited: true, neverExpire: true }),
  );
  render(<TraceStorage />);
  await vi.waitFor(() => {
    expect(screen.getByText("Unlimited")).toBeInTheDocument();
  });
  expect(screen.getByText("Never")).toBeInTheDocument();
});
