import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router";
import { beforeEach, expect, test, vi } from "vitest";

const api = vi.hoisted(() => ({
  BrowserAPIError: class BrowserAPIError extends Error { constructor(readonly code: string, message: string) { super(message); } },
  getTraceAnalysisSummary: vi.fn().mockResolvedValue({ targetScopeId: "scope-1", traceId: "trace-1", sessionId: "session-1", outcome: "FAILED", terminalFailureId: null, recordCount: 1, frameCount: 1, rootFrameIds: ["f-1"], usageComplete: false }),
  getTraceFrames: vi.fn().mockResolvedValue({ targetScopeId: "scope-1", items: [{ frameId: "f-1", parentFrameId: null, childFrameIds: [], frameType: "SKILL", route: "hello", inclusiveDurationMillis: null, selfDurationMillis: null, directUsage: { promptUnits: 0, completionUnits: 0, totalUnits: 0 }, directUsageComplete: true, descendantUsage: { promptUnits: 0, completionUnits: 0, totalUnits: 0 }, descendantUsageComplete: true, inclusiveUsage: { promptUnits: 0, completionUnits: 0, totalUnits: 0 }, inclusiveUsageComplete: true, outcomes: [], attemptIds: [], retrySequenceIds: [], validationStatuses: [], failureIds: [] }], hasMore: false, nextCursor: null }),
  getTraceRecords: vi.fn().mockResolvedValue({ targetScopeId: "scope-1", items: [{ sequence: 1, type: "PAYLOAD", frameId: "f-1", route: "hello", timestampMillis: 1, representation: "LOGICAL", payloadId: "p-1" }], hasMore: false, nextCursor: null }),
  getTraceUsage: vi.fn().mockResolvedValue({ targetScopeId: "scope-1", attributed: { totalUnits: 4 } }),
  getTraceAttempts: vi.fn().mockResolvedValue({ targetScopeId: "scope-1", items: [], hasMore: false, nextCursor: null }),
  getTraceRetries: vi.fn().mockResolvedValue({ targetScopeId: "scope-1", items: [], hasMore: false, nextCursor: null }),
  getTraceFailures: vi.fn().mockResolvedValue({ targetScopeId: "scope-1", items: [], hasMore: false, nextCursor: null }),
  getTraceValidationLinks: vi.fn().mockResolvedValue({ targetScopeId: "scope-1", items: [], hasMore: false, nextCursor: null }),
  listSkills: vi.fn().mockResolvedValue({ targetScopeId: "scope-1", items: [], hasMore: false, nextCursor: null }),
  getTraceGaps: vi.fn().mockResolvedValue({ targetScopeId: "scope-1", items: [], hasMore: false, nextCursor: null }),
  getTraceUncertainties: vi.fn().mockResolvedValue({ targetScopeId: "scope-1", items: [], hasMore: false, nextCursor: null }),
  getTracePayloads: vi.fn().mockResolvedValue({ targetScopeId: "scope-1", items: [], hasMore: false, nextCursor: null }),
  searchTraceEvidence: vi.fn().mockResolvedValue({ targetScopeId: "scope-1", items: [], hasMore: false, nextCursor: null }),
  getPayloadRange: vi.fn().mockResolvedValue({ targetScopeId: "scope-1", actualStart: 0, actualEnd: 4, totalLength: 4, contentType: "text/plain", encoding: "TEXT", content: "<a>x</a>", hasMore: false, nextCursor: null }),
  getRawRecordRange: vi.fn().mockResolvedValue({ targetScopeId: "scope-1", actualStart: 0, actualEnd: 2, totalLength: 2, contentType: "application/x-ndjson", encoding: "TEXT", content: "{}", hasMore: false, nextCursor: null }),
}));
const targetState = vi.hoisted(() => ({ target: { status: { targetScopeId: "scope-1" } }, refresh: vi.fn() }));
vi.mock("../api/client", () => api);
vi.mock("../target/TargetProvider", () => ({ useTarget: () => targetState }));

import { TraceExplorer } from "./TraceExplorer";

beforeEach(() => {
  vi.clearAllMocks();
  targetState.target.status.targetScopeId = "scope-1";
});

function LocationProbe() { const location = useLocation(); return <output aria-label="location">{location.search}</output>; }

test("loads hierarchy and deliberately reads inert evidence", async () => {
  render(<MemoryRouter><TraceExplorer traceId="trace-1" /></MemoryRouter>);
  await screen.findByText(/FAILED/);
  expect(api.getPayloadRange).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("tab", { name: "Records" }));
  await screen.findByText("1: PAYLOAD");
  fireEvent.click(screen.getByRole("button", { name: "Read payload" }));
  await screen.findByText("<a>x</a>");
  expect(screen.queryByRole("link", { name: "x" })).toBeNull();
  fireEvent.click(screen.getByRole("tab", { name: "Usage" }));
  await screen.findByLabelText("Usage facts");
});

test("usage operations deep-link to and focus their exact model response record", async () => {
  const rootFrame = { frameId: "root", parentFrameId: null, childFrameIds: ["model-frame"], frameType: "SKILL", route: "handleIncident", openedTimestampMillis: 1, closedTimestampMillis: 30, inclusiveDurationMillis: 29, selfDurationMillis: 1, directUsage: { promptUnits: 0, completionUnits: 0, totalUnits: 0 }, directUsageComplete: true, descendantUsage: { promptUnits: 2, completionUnits: 2, totalUnits: 4 }, descendantUsageComplete: true, inclusiveUsage: { promptUnits: 2, completionUnits: 2, totalUnits: 4 }, inclusiveUsageComplete: true, outcomes: [], attemptIds: [], retrySequenceIds: [], validationStatuses: [], failureIds: [] };
  const modelFrame = { ...rootFrame, frameId: "model-frame", parentFrameId: "root", childFrameIds: [], frameType: "MODEL_CALL", route: "handleIncident#planning-model", openedTimestampMillis: 10, closedTimestampMillis: 20, directUsage: { promptUnits: 2, completionUnits: 2, totalUnits: 4 }, descendantUsage: { promptUnits: 0, completionUnits: 0, totalUnits: 0 }, inclusiveUsage: { promptUnits: 2, completionUnits: 2, totalUnits: 4 } };
  const response = { sequence: 23, type: "MODEL_RESPONSE_RECEIVED", frameId: "model-frame", route: modelFrame.route, timestampMillis: 20, representation: "LOGICAL", payloadId: "response-payload" };
  const page = (items: unknown[], hasMore = false, nextCursor: string | null = null) => ({ targetScopeId: "scope-1", items, hasMore, nextCursor });
  api.getTraceFrames.mockResolvedValueOnce(page([rootFrame])).mockResolvedValueOnce(page([modelFrame]));
  api.getTraceUsage.mockResolvedValueOnce({ targetScopeId: "scope-1", attributed: modelFrame.directUsage, unattributed: rootFrame.directUsage, unframedAttributed: rootFrame.directUsage, terminal: modelFrame.directUsage });
  api.getTraceRecords.mockResolvedValueOnce(page([], true, "responses-next")).mockResolvedValueOnce(page([response])).mockResolvedValueOnce(page([response]));

  render(<MemoryRouter initialEntries={["/?view=usage"]}><TraceExplorer traceId="trace-1" /><LocationProbe /></MemoryRouter>);
  const operation = await screen.findByRole("link", { name: "handleIncident · planning · model" });
  fireEvent.click(operation);

  await vi.waitFor(() => expect(screen.getByLabelText("location")).toHaveTextContent("view=records"));
  expect(screen.getByLabelText("location")).toHaveTextContent("frameId=model-frame");
  expect(screen.getByLabelText("location")).toHaveTextContent("recordSequence=23");
  const selectedResponse = await screen.findByRole("button", { name: "23: MODEL_RESPONSE_RECEIVED" });
  await vi.waitFor(() => expect(selectedResponse).toHaveFocus());
  expect(api.getTraceRecords).toHaveBeenCalledWith("trace-1", undefined, { types: ["MODEL_RESPONSE_RECEIVED"] });
  expect(api.getTraceRecords).toHaveBeenCalledWith("trace-1", "responses-next", { types: ["MODEL_RESPONSE_RECEIVED"] });
});

test("continues a finite payload range using the returned opaque cursor", async () => {
  api.getPayloadRange.mockResolvedValueOnce({ targetScopeId: "scope-1", actualStart: 0, actualEnd: 2, totalLength: 4, contentType: "text/plain", encoding: "TEXT", content: "one", hasMore: true, nextCursor: "next-1" }).mockResolvedValueOnce({ targetScopeId: "scope-1", actualStart: 2, actualEnd: 4, totalLength: 4, contentType: "text/plain", encoding: "TEXT", content: "two", hasMore: false, nextCursor: null });
  render(<MemoryRouter><TraceExplorer traceId="trace-1" /></MemoryRouter>);
  await screen.findByText(/FAILED/);
  fireEvent.click(screen.getByRole("tab", { name: "Records" }));
  await screen.findByText("1: PAYLOAD");
  fireEvent.click(screen.getByRole("button", { name: "Read payload" }));
  await screen.findByText("one");
  fireEvent.click(screen.getByRole("button", { name: "Read next range" }));
  await screen.findByText("two");
  expect(api.getPayloadRange).toHaveBeenLastCalledWith("trace-1", "p-1", "next-1");
});

test("artifact expiration during a range clears stale content and reports the precise error", async () => {
  api.getPayloadRange.mockResolvedValueOnce({ targetScopeId: "scope-1", actualStart: 0, actualEnd: 2, totalLength: 4, contentType: "text/plain", encoding: "TEXT", content: "one", hasMore: true, nextCursor: "next-1" }).mockRejectedValueOnce(new api.BrowserAPIError("ARTIFACT_EXPIRED", "The local artifact expired."));
  render(<MemoryRouter><TraceExplorer traceId="trace-1" /></MemoryRouter>);
  await screen.findByText(/FAILED/);
  fireEvent.click(screen.getByRole("tab", { name: "Records" }));
  await screen.findByText("1: PAYLOAD");
  fireEvent.click(screen.getByRole("button", { name: "Read payload" }));
  await screen.findByText("one");
  fireEvent.click(screen.getByRole("button", { name: "Read next range" }));
  await screen.findByText("The local artifact expired.");
  expect(screen.queryByText("one")).toBeNull();
});

test("search is deliberate and forwards literal text only after submission", async () => {
  api.searchTraceEvidence.mockResolvedValueOnce({ targetScopeId: "scope-1", items: [{ sequence: 1, recordType: "MODEL_RESPONSE_RECEIVED", frameId: "f-1", matchOffset: 2, matchLength: 4, searchedField: "payload" }], hasMore: false, nextCursor: null });
  render(<MemoryRouter><TraceExplorer traceId="trace-1" /></MemoryRouter>);
  await screen.findByText(/FAILED/);
  fireEvent.click(screen.getByRole("tab", { name: "Records" }));
  await screen.findByRole("heading", { name: "Records" });
  expect(api.searchTraceEvidence).not.toHaveBeenCalled();
  fireEvent.change(screen.getByLabelText("Literal search"), { target: { value: "needle" } });
  fireEvent.click(screen.getByRole("button", { name: "Search" }));
  await screen.findByText("1 literal matches");
  fireEvent.click(screen.getByRole("button", { name: "MODEL_RESPONSE_RECEIVED record 1" }));
  expect(screen.getByRole("region", { name: "Literal search results" })).toHaveTextContent("payload bytes 2–6");
  expect(api.searchTraceEvidence).toHaveBeenCalledWith("trace-1", "needle");
});

test("continues hierarchy, record, and literal-search pages with their returned cursors", async () => {
  const page = (items: unknown[], hasMore: boolean, nextCursor: string | null) => ({ targetScopeId: "scope-1", items, hasMore, nextCursor });
  api.getTraceFrames.mockResolvedValueOnce(page([{ frameId: "f-1", parentFrameId: null, childFrameIds: [], frameType: "SKILL", route: "hello", inclusiveDurationMillis: null, selfDurationMillis: null }], true, "frames-next")).mockResolvedValueOnce(page([{ frameId: "f-2", parentFrameId: null, childFrameIds: [], frameType: "TOOL", route: "next", inclusiveDurationMillis: 1, selfDurationMillis: 1 }], false, null));
  api.getTraceRecords.mockResolvedValueOnce(page([{ sequence: 1, type: "PAYLOAD", frameId: "f-1", route: "hello", timestampMillis: 1, representation: "LOGICAL", payloadId: "p-1" }], true, "records-next")).mockResolvedValueOnce(page([{ sequence: 2, type: "EVENT", frameId: "f-2", route: "next", timestampMillis: 2, representation: "LOGICAL", payloadId: "" }], false, null));
  api.searchTraceEvidence.mockResolvedValueOnce(page([], true, "search-next")).mockResolvedValueOnce(page([], false, null));
  render(<MemoryRouter><TraceExplorer traceId="trace-1" /></MemoryRouter>);
  await screen.findByText(/FAILED/);
  fireEvent.click(screen.getByRole("button", { name: "Load more frames" }));
  await screen.findByRole("button", { name: "TOOL: next" });
  expect(api.getTraceFrames).toHaveBeenLastCalledWith("trace-1", "frames-next");
  fireEvent.click(screen.getByRole("tab", { name: "Records" }));
  await screen.findByRole("button", { name: "Load more records" });
  fireEvent.click(screen.getByRole("button", { name: "Load more records" }));
  await screen.findByRole("button", { name: "2: EVENT" });
  expect(api.getTraceRecords).toHaveBeenLastCalledWith("trace-1", "records-next");
  fireEvent.change(screen.getByLabelText("Literal search"), { target: { value: "needle" } });
  fireEvent.click(screen.getByRole("button", { name: "Search" }));
  await screen.findByRole("button", { name: "Load more matches" });
  fireEvent.click(screen.getByRole("button", { name: "Load more matches" }));
  await vi.waitFor(() => expect(api.searchTraceEvidence).toHaveBeenLastCalledWith("trace-1", "needle", "search-next"));
});

test("invalid frame record failure and view parameters are removed", async () => {
  render(<MemoryRouter initialEntries={["/?view=invalid&recordSequence=zero&frameId=f&failureId=x"]}><TraceExplorer traceId="trace-1" /><LocationProbe /></MemoryRouter>);
  await screen.findByText(/FAILED/);
  expect(screen.getByLabelText("location")).toHaveTextContent("");
});

test("response scope mismatch clears explorer selection and refreshes target", async () => {
  api.getTraceAnalysisSummary.mockResolvedValueOnce({ targetScopeId: "scope-old" });
  render(<MemoryRouter initialEntries={["/?view=records&frameId=f-1&recordSequence=1&failureId=bad"]}><TraceExplorer traceId="trace-1" /><LocationProbe /></MemoryRouter>);
  await vi.waitFor(() => expect(targetState.refresh).toHaveBeenCalled());
  await vi.waitFor(() => expect(screen.getByLabelText("location")).not.toHaveTextContent("frameId"));
  expect(screen.getByLabelText("location")).not.toHaveTextContent("recordSequence");
  expect(screen.getByLabelText("location")).not.toHaveTextContent("failureId");
});

test("records omit detached fact indexes and do not load their collections", async () => {
  render(<MemoryRouter><TraceExplorer traceId="trace-1" /></MemoryRouter>);
  await screen.findByText(/FAILED/);
  fireEvent.click(screen.getByRole("tab", { name: "Records" }));
  await screen.findByRole("heading", { name: "Records" });
  expect(screen.queryByRole("heading", { name: "Attempts, retries, and validation" })).toBeNull();
  expect(screen.queryByRole("heading", { name: "Failures and uncertainty" })).toBeNull();
  expect(screen.queryByRole("heading", { name: "Payloads" })).toBeNull();
  expect(api.getTraceAttempts).not.toHaveBeenCalled();
  expect(api.getTraceRetries).not.toHaveBeenCalled();
  expect(api.getTraceValidationLinks).not.toHaveBeenCalled();
  expect(api.getTraceGaps).not.toHaveBeenCalled();
  expect(api.getTraceUncertainties).not.toHaveBeenCalled();
  expect(api.getTracePayloads).not.toHaveBeenCalled();
});

test("failure deep links continue pages until the selected fact is found", async () => {
  api.getTraceFailures
    .mockResolvedValueOnce({ targetScopeId: "scope-1", items: [], hasMore: true, nextCursor: "failures-next" })
    .mockResolvedValueOnce({ targetScopeId: "scope-1", items: [{ failureId: "failure-late", terminal: false }], hasMore: false, nextCursor: null });
  render(<MemoryRouter initialEntries={["/?view=records&failureId=failure-late"]}><TraceExplorer traceId="trace-1" /></MemoryRouter>);
  await screen.findByRole("heading", { name: "Recovered failure" });
  expect(api.getTraceFailures).toHaveBeenLastCalledWith("trace-1", "failures-next");
});

test("failure selection follows its linked frame", async () => {
  api.getTraceFailures.mockResolvedValueOnce({ targetScopeId: "scope-1", items: [{ failureId: "failure-1", terminal: false, sequence: 2, frameId: "f-1" }], hasMore: false, nextCursor: null });
  render(<MemoryRouter initialEntries={["/?failureId=failure-1"]}><TraceExplorer traceId="trace-1" /><LocationProbe /></MemoryRouter>);
  await vi.waitFor(() => expect(screen.getByLabelText("location")).toHaveTextContent("failureId=failure-1"));
  await vi.waitFor(() => expect(screen.getByLabelText("location")).toHaveTextContent("frameId=f-1"));
});

test("frame selection follows its first linked failure", async () => {
  api.getTraceFrames.mockResolvedValueOnce({ targetScopeId: "scope-1", items: [{ frameId: "f-1", parentFrameId: null, childFrameIds: [], frameType: "SKILL", route: "hello", inclusiveDurationMillis: null, selfDurationMillis: null, failureIds: ["failure-1"] }], hasMore: false, nextCursor: null });
  api.getTraceFailures.mockResolvedValueOnce({ targetScopeId: "scope-1", items: [{ failureId: "failure-1", terminal: false, sequence: 2, frameId: "f-1" }], hasMore: false, nextCursor: null });
  render(<MemoryRouter><TraceExplorer traceId="trace-1" /><LocationProbe /></MemoryRouter>);
  fireEvent.click(await screen.findByRole("button", { name: /SKILL: hello/ }));
  await vi.waitFor(() => expect(screen.getByLabelText("location")).toHaveTextContent("frameId=f-1"));
  expect(screen.getByLabelText("location")).toHaveTextContent("failureId=failure-1");
});

test("a terminal failure does not pin navigation to its frame", async () => {
  api.getTraceAnalysisSummary.mockResolvedValueOnce({ targetScopeId: "scope-1", traceId: "trace-1", sessionId: "session-1", outcome: "FAILED", terminalFailureId: "terminal-1", recordCount: 5, frameCount: 2, rootFrameIds: ["root"], usageComplete: false });
  api.getTraceFrames.mockResolvedValueOnce({ targetScopeId: "scope-1", items: [
    { frameId: "root", parentFrameId: null, childFrameIds: ["model"], frameType: "ROOT", route: "root", inclusiveDurationMillis: 5, selfDurationMillis: 4, failureIds: [] },
    { frameId: "model", parentFrameId: "root", childFrameIds: [], frameType: "MODEL_CALL", route: "model", inclusiveDurationMillis: 1, selfDurationMillis: 1, failureIds: ["terminal-1"] },
  ], hasMore: false, nextCursor: null });
  api.getTraceFailures.mockResolvedValueOnce({ targetScopeId: "scope-1", items: [{ failureId: "terminal-1", terminal: true, sequence: 4, frameId: "model" }], hasMore: false, nextCursor: null });

  render(<MemoryRouter><TraceExplorer traceId="trace-1" /><LocationProbe /></MemoryRouter>);
  await vi.waitFor(() => expect(screen.getByLabelText("location")).toHaveTextContent("frameId=model"));
  fireEvent.click(screen.getByRole("button", { name: /^ROOT: root/ }));
  await vi.waitFor(() => expect(screen.getByLabelText("location")).toHaveTextContent("frameId=root"));
  expect(screen.getByLabelText("location")).not.toHaveTextContent("failureId=");
  await new Promise((resolve) => setTimeout(resolve, 0));
  expect(screen.getByLabelText("location")).toHaveTextContent("frameId=root");
});

test("deep frame selection loads complete ancestry for hierarchy and breadcrumbs", async () => {
  const result = (items: unknown[]) => ({ targetScopeId: "scope-1", items, hasMore: false, nextCursor: null });
  api.getTraceFrames
    .mockResolvedValueOnce(result([]))
    .mockResolvedValueOnce(result([{ frameId: "leaf", parentFrameId: "middle", childFrameIds: [], frameType: "TOOL", route: "leaf", openedTimestampMillis: 3, closedTimestampMillis: 4, inclusiveDurationMillis: 1, selfDurationMillis: 1 }]))
    .mockResolvedValueOnce(result([{ frameId: "middle", parentFrameId: "root", childFrameIds: ["leaf"], frameType: "SKILL", route: "middle", openedTimestampMillis: 2, closedTimestampMillis: 5, inclusiveDurationMillis: 3, selfDurationMillis: 2 }]))
    .mockResolvedValueOnce(result([{ frameId: "root", parentFrameId: null, childFrameIds: ["middle"], frameType: "ROOT", route: "root", openedTimestampMillis: 1, closedTimestampMillis: 6, inclusiveDurationMillis: 5, selfDurationMillis: 2 }]));
  render(<MemoryRouter initialEntries={["/?frameId=leaf"]}><TraceExplorer traceId="trace-1" /></MemoryRouter>);
  const breadcrumbs = await screen.findByRole("navigation", { name: "Selected frame breadcrumbs" });
  expect(breadcrumbs).toHaveTextContent("root / middle / leaf");
  expect(screen.getByRole("button", { name: "ROOT: root" })).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "TOOL: leaf" })).toBeInTheDocument();
});

test("tabs use roving focus and arrow key activation", async () => {
  render(<MemoryRouter><TraceExplorer traceId="trace-1" /><LocationProbe /></MemoryRouter>);
  await screen.findByText(/FAILED/);
  const hierarchy = screen.getByRole("tab", { name: "Hierarchy" });
  const timeline = screen.getByRole("tab", { name: "Timeline" });
  expect(hierarchy).toHaveAttribute("tabindex", "0");
  expect(timeline).toHaveAttribute("tabindex", "-1");
  hierarchy.focus();
  fireEvent.keyDown(hierarchy, { key: "ArrowRight" });
  expect(timeline).toHaveFocus();
  expect(timeline).toHaveAttribute("aria-selected", "true");
  expect(screen.getByRole("tabpanel")).toHaveAttribute("aria-labelledby", "trace-tab-timeline");
});

test("expired local artifact clears explorer state and requests reacquisition", async () => {
  const unavailable = vi.fn();
  api.getTraceAnalysisSummary.mockRejectedValueOnce(new api.BrowserAPIError("ARTIFACT_EXPIRED", "The local artifact expired."));
  render(<MemoryRouter initialEntries={["/?view=records&frameId=f-1&recordSequence=1&failureId=bad"]}><TraceExplorer traceId="trace-1" onArtifactUnavailable={unavailable} /><LocationProbe /></MemoryRouter>);
  await vi.waitFor(() => expect(unavailable).toHaveBeenCalledWith(expect.objectContaining({ code: "ARTIFACT_EXPIRED" })));
  expect(screen.getByLabelText("location")).not.toHaveTextContent("frameId");
  expect(screen.getByLabelText("location")).not.toHaveTextContent("recordSequence");
  expect(screen.queryByText(/FAILED/)).toBeNull();
});

test("failure focus selects the recorded terminal failure and never loads raw payloads", async () => {
  api.getTraceAnalysisSummary.mockResolvedValueOnce({ targetScopeId: "scope-1", traceId: "trace-1", sessionId: "session-1", outcome: "FAILED", terminalFailureId: "terminal-1", recordCount: 120, frameCount: 1, attemptCount: 1, retryCount: 1, validationCount: 1, failureCount: 2, payloadCount: 1, gapCount: 1, uncertaintyCount: 1, rootFrameIds: ["f-1"], usageComplete: false, configuredLimits: null });
  api.getTraceFailures.mockResolvedValueOnce({ targetScopeId: "scope-1", items: [{ failureId: "recovered", terminal: false, sequence: 3, timestampMillis: 3, recordType: "ERROR_RECORDED", frameId: "", route: "", attemptId: "", retrySequenceId: "", validationStatus: "" }], hasMore: true, nextCursor: "failure-next" }).mockResolvedValueOnce({ targetScopeId: "scope-1", items: [{ failureId: "terminal-1", terminal: true, sequence: 119, timestampMillis: 119, recordType: "ERROR_RECORDED", frameId: "f-1", route: "hello", attemptId: "a-1", retrySequenceId: "r-1", validationStatus: "exhausted" }], hasMore: false, nextCursor: null });
  render(<MemoryRouter><TraceExplorer traceId="trace-1" /><LocationProbe /></MemoryRouter>);
  expect(await screen.findByRole("heading", { name: "Terminal failure evidence" })).toBeInTheDocument();
  expect(screen.getByRole("region", { name: "Trace failure details" })).toHaveClass("trace-failure-panel");
  await screen.findByText("ERROR_RECORDED sequence 119");
  await vi.waitFor(() => expect(screen.getByLabelText("location")).toHaveTextContent("frameId=f-1"));
  expect(screen.getByText(/does not identify root cause/)).toBeInTheDocument();
  expect(api.getPayloadRange).not.toHaveBeenCalled();
  expect(api.getRawRecordRange).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "Show in hierarchy" }));
  await vi.waitFor(() => expect(screen.getByRole("button", { name: /SKILL: hello/ })).toHaveFocus());
});

test("view error from the exact failure record focuses the failure panel", async () => {
  api.getTraceAnalysisSummary.mockResolvedValueOnce({ targetScopeId: "scope-1", traceId: "trace-1", sessionId: "session-1", outcome: "FAILED", terminalFailureId: "terminal-1", recordCount: 15, frameCount: 1, attemptCount: 1, retryCount: 1, validationCount: 0, failureCount: 1, payloadCount: 1, gapCount: 0, uncertaintyCount: 0, rootFrameIds: ["f-1"], usageComplete: false, configuredLimits: null });
  api.getTraceRecords.mockResolvedValueOnce({ targetScopeId: "scope-1", items: [{ sequence: 15, type: "ERROR_RECORDED", frameId: "f-1", route: "hello", timestampMillis: 15, representation: "LOGICAL", payloadId: "p-1" }], hasMore: false, nextCursor: null });
  api.getTraceFailures.mockResolvedValueOnce({ targetScopeId: "scope-1", items: [{ failureId: "terminal-1", terminal: true, sequence: 15, timestampMillis: 15, recordType: "ERROR_RECORDED", frameId: "f-1", route: "hello", attemptId: "a-1", retrySequenceId: "r-1", validationStatus: "" }], hasMore: false, nextCursor: null });
  render(<MemoryRouter initialEntries={["/?view=records"]}><TraceExplorer traceId="trace-1" /></MemoryRouter>);

  fireEvent.click(await screen.findByRole("button", { name: "View error" }));

  expect(screen.getByRole("region", { name: "Trace failure details" })).toHaveFocus();
});

test("selected frames link only exact current registered skill names", async () => {
  api.getTraceFrames.mockResolvedValueOnce({ targetScopeId: "scope-1", items: [{ frameId: "f-1", parentFrameId: null, childFrameIds: [], frameType: "SKILL", route: "root.child", inclusiveDurationMillis: 1, selfDurationMillis: 1, skillNames: ["exact.skill", "Missing.Skill"] }], hasMore: false, nextCursor: null });
  api.listSkills.mockResolvedValueOnce({ targetScopeId: "scope-1", items: [{ registeredName: "exact.skill", sourcePath: "<unsafe>" }, { registeredName: "missing.skill", sourcePath: "other" }], hasMore: false, nextCursor: null });
  render(<MemoryRouter initialEntries={["/?frameId=f-1"]}><TraceExplorer traceId="trace-1" /></MemoryRouter>);
  expect(await screen.findByRole("link", { name: "exact.skill" })).toHaveAttribute("href", "/skills/exact.skill?targetScopeId=scope-1");
  expect(screen.getByText("Missing.Skill").closest("li")).toHaveTextContent("not in current registered catalog");
  expect(screen.queryByRole("link", { name: "Missing.Skill" })).toBeNull();
  expect(screen.queryByText("<unsafe>")).toBeNull();
});
