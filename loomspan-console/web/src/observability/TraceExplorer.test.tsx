import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router";
import { beforeEach, expect, test, vi } from "vitest";

const api = vi.hoisted(() => ({
  BrowserAPIError: class BrowserAPIError extends Error { constructor(readonly code: string, message: string) { super(message); } },
  getTraceAnalysisSummary: vi.fn().mockResolvedValue({ source: "TARGET", targetScopeId: "scope-1", traceId: "trace-1", sessionId: "session-1", outcome: "FAILED", terminalFailureId: null, recordCount: 1, frameCount: 1, rootFrameIds: ["f-1"], usageComplete: false }),
  getTraceFrames: vi.fn().mockResolvedValue({ source: "TARGET", targetScopeId: "scope-1", items: [{ frameId: "f-1", parentFrameId: null, childFrameIds: [], frameType: "SKILL", route: "hello", inclusiveDurationMillis: null, selfDurationMillis: null, directUsage: { promptUnits: 0, completionUnits: 0, totalUnits: 0 }, directUsageComplete: true, descendantUsage: { promptUnits: 0, completionUnits: 0, totalUnits: 0 }, descendantUsageComplete: true, inclusiveUsage: { promptUnits: 0, completionUnits: 0, totalUnits: 0 }, inclusiveUsageComplete: true, outcome: null, attemptIds: [], retrySequenceIds: [], validationStatuses: [], failureIds: [] }], hasMore: false, nextCursor: null }),
  getTraceRecords: vi.fn().mockResolvedValue({ source: "TARGET", targetScopeId: "scope-1", items: [{ sequence: 1, type: "PAYLOAD", frameId: "f-1", route: "hello", timestampMillis: 1, representation: "LOGICAL", content: { role: "RECONSTRUCTED", contentType: "application/json", encoding: "UTF8", retainedBytes: 1, available: true, complete: true, inlineEligibility: true, contentRef: "p-1" } }], hasMore: false, nextCursor: null }),
  getTraceUsage: vi.fn().mockResolvedValue({ source: "TARGET", targetScopeId: "scope-1", attributed: { totalUnits: 4 } }),
  getTraceAttempts: vi.fn().mockResolvedValue({ source: "TARGET", targetScopeId: "scope-1", items: [], hasMore: false, nextCursor: null }),
  getTraceRetries: vi.fn().mockResolvedValue({ source: "TARGET", targetScopeId: "scope-1", items: [], hasMore: false, nextCursor: null }),
  getTraceFailures: vi.fn().mockResolvedValue({ source: "TARGET", targetScopeId: "scope-1", items: [], hasMore: false, nextCursor: null }),
  getTraceValidationLinks: vi.fn().mockResolvedValue({ source: "TARGET", targetScopeId: "scope-1", items: [], hasMore: false, nextCursor: null }),
  listSkills: vi.fn().mockResolvedValue({ source: "TARGET", targetScopeId: "scope-1", items: [], hasMore: false, nextCursor: null }),
  getTraceGaps: vi.fn().mockResolvedValue({ source: "TARGET", targetScopeId: "scope-1", items: [], hasMore: false, nextCursor: null }),
  getTraceUncertainties: vi.fn().mockResolvedValue({ source: "TARGET", targetScopeId: "scope-1", items: [], hasMore: false, nextCursor: null }),
  searchTraceEvidence: vi.fn().mockResolvedValue({ source: "TARGET", targetScopeId: "scope-1", items: [], hasMore: false, nextCursor: null }),
  getContentRange: vi.fn().mockResolvedValue({ source: "TARGET", targetScopeId: "scope-1", actualStart: 0, actualEnd: 4, totalLength: 4, contentType: "text/plain", encoding: "TEXT", content: "<a>x</a>", hasMore: false, nextCursor: null }),
  getRawRecordRange: vi.fn().mockResolvedValue({ source: "TARGET", targetScopeId: "scope-1", actualStart: 0, actualEnd: 2, totalLength: 2, contentType: "application/x-ndjson", encoding: "TEXT", content: "{}", hasMore: false, nextCursor: null }),
}));
const targetState = vi.hoisted(() => ({ target: { status: { source: "TARGET", targetScopeId: "scope-1" } }, scopeGeneration: 0, refresh: vi.fn() }));
vi.mock("../api/client", () => api);
vi.mock("../target/TargetProvider", () => ({ useTarget: () => targetState }));

import { TraceExplorer } from "./TraceExplorer";

beforeEach(() => {
  vi.clearAllMocks();
  targetState.target.status.targetScopeId = "scope-1";
	targetState.scopeGeneration = 0;
});

function LocationProbe() { const location = useLocation(); return <output aria-label="location">{location.search}</output>; }

test("loads hierarchy and deliberately reads inert evidence", async () => {
  render(<MemoryRouter><TraceExplorer traceId="trace-1" /></MemoryRouter>);
  await screen.findByText(/FAILED/);
  expect(api.getContentRange).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("tab", { name: "Records" }));
  await screen.findByText("1: PAYLOAD");
  fireEvent.click(screen.getByRole("button", { name: "Read content" }));
  await screen.findByText("<a>x</a>");
  expect(screen.queryByRole("link", { name: "x" })).toBeNull();
  fireEvent.click(screen.getByRole("tab", { name: "Usage" }));
  await screen.findByLabelText("Usage facts");
});

test("uses compact frames for hierarchy and detailed frames for rich views", async () => {
	render(<MemoryRouter><TraceExplorer traceId="trace-1" /></MemoryRouter>);
	await screen.findByText(/FAILED/);
	expect(api.getTraceFrames).toHaveBeenCalledWith("trace-1", undefined, {}, "CANONICAL", "TARGET");
	fireEvent.click(screen.getByRole("tab", { name: "Timeline" }));
	await vi.waitFor(() => expect(api.getTraceFrames).toHaveBeenCalledWith("trace-1", undefined, {}, "CANONICAL", "TARGET", "DETAILED"));
	fireEvent.click(screen.getByRole("tab", { name: "Hierarchy" }));
	fireEvent.click(screen.getByRole("button", { name: /SKILL: hello/ }));
	await vi.waitFor(() => expect(api.getTraceFrames).toHaveBeenCalledWith("trace-1", undefined, { frameIds: ["f-1"] }, "CANONICAL", "TARGET", "DETAILED"));
});

test("usage operations deep-link to and focus their exact model response record", async () => {
  const rootFrame = { frameId: "root", parentFrameId: null, childFrameIds: ["model-frame"], frameType: "SKILL", route: "handleIncident", openedTimestampMillis: 1, closedTimestampMillis: 30, inclusiveDurationMillis: 29, selfDurationMillis: 1, directUsage: { promptUnits: 0, completionUnits: 0, totalUnits: 0 }, directUsageComplete: true, descendantUsage: { promptUnits: 2, completionUnits: 2, totalUnits: 4 }, descendantUsageComplete: true, inclusiveUsage: { promptUnits: 2, completionUnits: 2, totalUnits: 4 }, inclusiveUsageComplete: true, outcome: null, attemptIds: [], retrySequenceIds: [], validationStatuses: [], failureIds: [] };
  const modelFrame = { ...rootFrame, frameId: "model-frame", parentFrameId: "root", childFrameIds: [], frameType: "MODEL_CALL", route: "handleIncident#planning-model", openedTimestampMillis: 10, closedTimestampMillis: 20, directUsage: { promptUnits: 2, completionUnits: 2, totalUnits: 4 }, descendantUsage: { promptUnits: 0, completionUnits: 0, totalUnits: 0 }, inclusiveUsage: { promptUnits: 2, completionUnits: 2, totalUnits: 4 } };
  const response = { sequence: 23, type: "MODEL_RESPONSE_RECEIVED", frameId: "model-frame", route: modelFrame.route, timestampMillis: 20, representation: "LOGICAL", content: { role: "DATA", contentType: "application/json", encoding: "UTF8", retainedBytes: 1, available: true, complete: true, inlineEligibility: true, contentRef: "response-payload" } };
  const page = (items: unknown[], hasMore = false, nextCursor: string | null = null) => ({ source: "TARGET", targetScopeId: "scope-1", items, hasMore, nextCursor });
  api.getTraceFrames.mockResolvedValueOnce(page([rootFrame])).mockResolvedValueOnce(page([modelFrame]));
  api.getTraceUsage.mockResolvedValueOnce({ source: "TARGET", targetScopeId: "scope-1", attributed: modelFrame.directUsage, unattributed: rootFrame.directUsage, unframedAttributed: rootFrame.directUsage, terminal: modelFrame.directUsage });
  api.getTraceRecords.mockResolvedValueOnce(page([], true, "responses-next")).mockResolvedValueOnce(page([response])).mockResolvedValueOnce(page([response]));

  render(<MemoryRouter initialEntries={["/?view=usage"]}><TraceExplorer traceId="trace-1" /><LocationProbe /></MemoryRouter>);
  const operation = await screen.findByRole("link", { name: "handleIncident · planning · model" });
  fireEvent.click(operation);

  await vi.waitFor(() => expect(screen.getByLabelText("location")).toHaveTextContent("view=records"));
  expect(screen.getByLabelText("location")).toHaveTextContent("frameId=model-frame");
  expect(screen.getByLabelText("location")).toHaveTextContent("recordSequence=23");
  const selectedResponse = await screen.findByRole("button", { name: "23: MODEL_RESPONSE_RECEIVED" });
  await vi.waitFor(() => expect(selectedResponse).toHaveFocus());
  expect(api.getTraceRecords).toHaveBeenCalledWith("trace-1", undefined, { types: ["MODEL_RESPONSE_RECEIVED"] }, "TARGET");
  expect(api.getTraceRecords).toHaveBeenCalledWith("trace-1", "responses-next", { types: ["MODEL_RESPONSE_RECEIVED"] }, "TARGET");
});

test("continues a finite payload range using the returned opaque cursor", async () => {
  api.getContentRange.mockResolvedValueOnce({ source: "TARGET", targetScopeId: "scope-1", actualStart: 0, actualEnd: 2, totalLength: 4, contentType: "text/plain", encoding: "TEXT", content: "one", hasMore: true, nextCursor: "next-1" }).mockResolvedValueOnce({ source: "TARGET", targetScopeId: "scope-1", actualStart: 2, actualEnd: 4, totalLength: 4, contentType: "text/plain", encoding: "TEXT", content: "two", hasMore: false, nextCursor: null });
  render(<MemoryRouter><TraceExplorer traceId="trace-1" /></MemoryRouter>);
  await screen.findByText(/FAILED/);
  fireEvent.click(screen.getByRole("tab", { name: "Records" }));
  await screen.findByText("1: PAYLOAD");
  fireEvent.click(screen.getByRole("button", { name: "Read content" }));
  await screen.findByText("one");
  fireEvent.click(screen.getByRole("button", { name: "Read next range" }));
  await screen.findByText("two");
  expect(api.getContentRange).toHaveBeenLastCalledWith("trace-1", "p-1", "next-1", "TARGET");
});

test("artifact expiration during a range clears stale content and reports the precise error", async () => {
  api.getContentRange.mockResolvedValueOnce({ source: "TARGET", targetScopeId: "scope-1", actualStart: 0, actualEnd: 2, totalLength: 4, contentType: "text/plain", encoding: "TEXT", content: "one", hasMore: true, nextCursor: "next-1" }).mockRejectedValueOnce(new api.BrowserAPIError("ARTIFACT_EXPIRED", "The local artifact expired."));
  render(<MemoryRouter><TraceExplorer traceId="trace-1" /></MemoryRouter>);
  await screen.findByText(/FAILED/);
  fireEvent.click(screen.getByRole("tab", { name: "Records" }));
  await screen.findByText("1: PAYLOAD");
  fireEvent.click(screen.getByRole("button", { name: "Read content" }));
  await screen.findByText("one");
  fireEvent.click(screen.getByRole("button", { name: "Read next range" }));
  await screen.findByText("The local artifact expired.");
  expect(screen.queryByText("one")).toBeNull();
});

test("search is deliberate and resolves page-local content descriptors", async () => {
  api.searchTraceEvidence.mockResolvedValueOnce({ source: "TARGET", targetScopeId: "scope-1", items: [{ sequence: 1, recordType: "MODEL_RESPONSE_RECEIVED", frameId: "f-1", matchOffset: 2, matchLength: 4, searchedField: "payload", contentId: "c1" }], contentDescriptors: [{ contentId: "c1", contentRef: "opaque-match-content" }], search: { workComplete: true, caseSensitive: true, searchedFields: ["payload"], limitations: [] }, hasMore: false, nextCursor: null });
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
  fireEvent.click(screen.getByRole("button", { name: "Read match content" }));
  await vi.waitFor(() => expect(api.getContentRange).toHaveBeenCalledWith("trace-1", "opaque-match-content", undefined, "TARGET"));
  expect(api.searchTraceEvidence).toHaveBeenCalledWith("trace-1", "needle", undefined, "TARGET");
});

test("rebases repeated page-local content IDs across search continuation pages", async () => {
  const search = { workComplete: false, caseSensitive: true, searchedFields: ["content"], limitations: [] };
  api.searchTraceEvidence
    .mockResolvedValueOnce({ source: "TARGET", targetScopeId: "scope-1", items: [{ sequence: 1, recordType: "STRUCTURED_OUTPUT_RECORDED", matchOffset: 0, matchLength: 6, searchedField: "content", contentId: "c1" }], contentDescriptors: [{ contentId: "c1", contentRef: "first-page-content" }], search, hasMore: true, nextCursor: "search-next" })
    .mockResolvedValueOnce({ source: "TARGET", targetScopeId: "scope-1", items: [{ sequence: 1, recordType: "STRUCTURED_OUTPUT_RECORDED", matchOffset: 14, matchLength: 6, searchedField: "content", contentId: "c1" }], contentDescriptors: [{ contentId: "c1", contentRef: "second-page-content" }], search: { ...search, workComplete: true }, hasMore: false, nextCursor: null });

  render(<MemoryRouter><TraceExplorer traceId="trace-1" /></MemoryRouter>);
  await screen.findByText(/FAILED/);
  fireEvent.click(screen.getByRole("tab", { name: "Records" }));
  fireEvent.change(screen.getByLabelText("Literal search"), { target: { value: "needle" } });
  fireEvent.click(screen.getByRole("button", { name: "Search" }));
  fireEvent.click(await screen.findByRole("button", { name: "Load more matches" }));

  await vi.waitFor(() => expect(screen.getAllByRole("button", { name: "Read match content" })).toHaveLength(2));
  const reads = screen.getAllByRole("button", { name: "Read match content" });
  fireEvent.click(reads[0]);
  await vi.waitFor(() => expect(api.getContentRange).toHaveBeenCalledWith("trace-1", "first-page-content", undefined, "TARGET"));
  await screen.findByText("<a>x</a>");
  fireEvent.click(screen.getAllByRole("button", { name: "Read match content" })[1]);
  await vi.waitFor(() => expect(api.getContentRange).toHaveBeenCalledWith("trace-1", "second-page-content", undefined, "TARGET"));
});

test("continues hierarchy, record, and literal-search pages with their returned cursors", async () => {
  const page = (items: unknown[], hasMore: boolean, nextCursor: string | null) => ({ source: "TARGET", targetScopeId: "scope-1", items, hasMore, nextCursor });
  api.getTraceFrames.mockResolvedValueOnce(page([{ frameId: "f-1", parentFrameId: null, childFrameIds: [], frameType: "SKILL", route: "hello", inclusiveDurationMillis: null, selfDurationMillis: null }], true, "frames-next")).mockResolvedValueOnce(page([{ frameId: "f-2", parentFrameId: null, childFrameIds: [], frameType: "TOOL", route: "next", inclusiveDurationMillis: 1, selfDurationMillis: 1 }], false, null));
  api.getTraceRecords.mockResolvedValueOnce(page([{ sequence: 1, type: "PAYLOAD", frameId: "f-1", route: "hello", timestampMillis: 1, representation: "LOGICAL", content: { role: "RECONSTRUCTED", contentType: "application/json", encoding: "UTF8", retainedBytes: 1, available: true, complete: true, inlineEligibility: true, contentRef: "p-1" } }], true, "records-next")).mockResolvedValueOnce(page([{ sequence: 2, type: "EVENT", frameId: "f-2", route: "next", timestampMillis: 2, representation: "LOGICAL",  }], false, null));
  api.searchTraceEvidence.mockResolvedValueOnce({ ...page([], true, "search-next"), contentDescriptors: [], search: { workComplete: false, caseSensitive: true, searchedFields: ["metadata", "content"], limitations: [] } }).mockResolvedValueOnce({ ...page([], false, null), contentDescriptors: [], search: { workComplete: true, caseSensitive: true, searchedFields: ["metadata", "content"], limitations: [] } });
  render(<MemoryRouter><TraceExplorer traceId="trace-1" /></MemoryRouter>);
  await screen.findByText(/FAILED/);
  fireEvent.click(screen.getByRole("button", { name: "Load more frames" }));
  await screen.findByRole("button", { name: "TOOL: next" });
  expect(api.getTraceFrames).toHaveBeenLastCalledWith("trace-1", "frames-next", {}, "CANONICAL", "TARGET");
  fireEvent.click(screen.getByRole("tab", { name: "Records" }));
  await screen.findByRole("button", { name: "Load more records" });
  fireEvent.click(screen.getByRole("button", { name: "Load more records" }));
  await screen.findByRole("button", { name: "2: EVENT" });
  expect(api.getTraceRecords).toHaveBeenLastCalledWith("trace-1", "records-next", {}, "TARGET");
  fireEvent.change(screen.getByLabelText("Literal search"), { target: { value: "needle" } });
  fireEvent.click(screen.getByRole("button", { name: "Search" }));
  await screen.findByRole("button", { name: "Load more matches" });
  fireEvent.click(screen.getByRole("button", { name: "Load more matches" }));
  await vi.waitFor(() => expect(api.searchTraceEvidence).toHaveBeenLastCalledWith("trace-1", "needle", "search-next", "TARGET"));
});

test("invalid frame record failure and view parameters are removed", async () => {
  render(<MemoryRouter initialEntries={["/?view=invalid&recordSequence=zero&frameId=f&failureId=x"]}><TraceExplorer traceId="trace-1" /><LocationProbe /></MemoryRouter>);
  await screen.findByText(/FAILED/);
  expect(screen.getByLabelText("location")).toHaveTextContent("");
});

test("response scope mismatch clears explorer selection and refreshes target", async () => {
  api.getTraceAnalysisSummary.mockResolvedValueOnce({ source: "TARGET", targetScopeId: "scope-old" });
  render(<MemoryRouter initialEntries={["/?view=records&frameId=f-1&recordSequence=1&failureId=bad"]}><TraceExplorer traceId="trace-1" /><LocationProbe /></MemoryRouter>);
  await vi.waitFor(() => expect(targetState.refresh).toHaveBeenCalled());
  await vi.waitFor(() => expect(screen.getByLabelText("location")).not.toHaveTextContent("frameId"));
  expect(screen.getByLabelText("location")).not.toHaveTextContent("recordSequence");
  expect(screen.getByLabelText("location")).not.toHaveTextContent("failureId");
});

test("imported evidence does not reset or refetch when the target rotates", async () => {
	api.getTraceAnalysisSummary.mockResolvedValueOnce({ source: "IMPORTED", traceId: "trace-1", sessionId: "session-1", outcome: "FAILED", terminalFailureId: null, recordCount: 1, frameCount: 1, rootFrameIds: ["f-1"], usageComplete: false });
	api.getTraceFrames.mockResolvedValueOnce({ source: "IMPORTED", items: [{ frameId: "f-1", parentFrameId: null, childFrameIds: [], frameType: "SKILL", route: "hello", inclusiveDurationMillis: null, selfDurationMillis: null, directUsage: { promptUnits: 0, completionUnits: 0, totalUnits: 0 }, directUsageComplete: true, descendantUsage: { promptUnits: 0, completionUnits: 0, totalUnits: 0 }, descendantUsageComplete: true, inclusiveUsage: { promptUnits: 0, completionUnits: 0, totalUnits: 0 }, inclusiveUsageComplete: true, outcome: null, attemptIds: [], retrySequenceIds: [], validationStatuses: [], failureIds: [] }], hasMore: false, nextCursor: null });
	const view = render(<MemoryRouter><TraceExplorer traceId="trace-1" source="IMPORTED" /></MemoryRouter>);
	await screen.findByText(/FAILED/);
	expect(api.getTraceAnalysisSummary).toHaveBeenCalledTimes(1);
	expect(api.getTraceFrames).toHaveBeenCalledTimes(1);

	targetState.target.status.targetScopeId = "scope-2";
	targetState.scopeGeneration = 1;
	view.rerender(<MemoryRouter><TraceExplorer traceId="trace-1" source="IMPORTED" /></MemoryRouter>);
	await vi.waitFor(() => expect(screen.getByText(/FAILED/)).toBeInTheDocument());
	expect(api.getTraceAnalysisSummary).toHaveBeenCalledTimes(1);
	expect(api.getTraceFrames).toHaveBeenCalledTimes(1);
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
});

test("failure deep links continue pages until the selected fact is found", async () => {
  api.getTraceFailures
    .mockResolvedValueOnce({ source: "TARGET", targetScopeId: "scope-1", items: [], hasMore: true, nextCursor: "failures-next" })
    .mockResolvedValueOnce({ source: "TARGET", targetScopeId: "scope-1", items: [{ failureId: "failure-late", terminal: false }], hasMore: false, nextCursor: null });
  render(<MemoryRouter initialEntries={["/?view=records&failureId=failure-late"]}><TraceExplorer traceId="trace-1" /></MemoryRouter>);
  await screen.findByRole("heading", { name: "Recovered failure" });
  expect(api.getTraceFailures).toHaveBeenLastCalledWith("trace-1", "failures-next", "TARGET");
});

test("failure selection follows its linked frame", async () => {
  api.getTraceFailures.mockResolvedValueOnce({ source: "TARGET", targetScopeId: "scope-1", items: [{ failureId: "failure-1", terminal: false, sequence: 2, frameId: "f-1" }], hasMore: false, nextCursor: null });
  render(<MemoryRouter initialEntries={["/?failureId=failure-1"]}><TraceExplorer traceId="trace-1" /><LocationProbe /></MemoryRouter>);
  await vi.waitFor(() => expect(screen.getByLabelText("location")).toHaveTextContent("failureId=failure-1"));
  await vi.waitFor(() => expect(screen.getByLabelText("location")).toHaveTextContent("frameId=f-1"));
});

test("frame selection follows its first linked failure", async () => {
  api.getTraceFrames.mockResolvedValueOnce({ source: "TARGET", targetScopeId: "scope-1", items: [{ frameId: "f-1", parentFrameId: null, childFrameIds: [], frameType: "SKILL", route: "hello", inclusiveDurationMillis: null, selfDurationMillis: null, failureIds: ["failure-1"] }], hasMore: false, nextCursor: null });
  api.getTraceFailures.mockResolvedValueOnce({ source: "TARGET", targetScopeId: "scope-1", items: [{ failureId: "failure-1", terminal: false, sequence: 2, frameId: "f-1" }], hasMore: false, nextCursor: null });
  render(<MemoryRouter><TraceExplorer traceId="trace-1" /><LocationProbe /></MemoryRouter>);
  fireEvent.click(await screen.findByRole("button", { name: /SKILL: hello/ }));
  await vi.waitFor(() => expect(screen.getByLabelText("location")).toHaveTextContent("frameId=f-1"));
  expect(screen.getByLabelText("location")).toHaveTextContent("failureId=failure-1");
});

test("a terminal failure does not pin navigation to its frame", async () => {
  api.getTraceAnalysisSummary.mockResolvedValueOnce({ source: "TARGET", targetScopeId: "scope-1", traceId: "trace-1", sessionId: "session-1", outcome: "FAILED", terminalFailureId: "terminal-1", recordCount: 5, frameCount: 2, rootFrameIds: ["root"], usageComplete: false });
  api.getTraceFrames.mockResolvedValueOnce({ source: "TARGET", targetScopeId: "scope-1", items: [
    { frameId: "root", parentFrameId: null, childFrameIds: ["model"], frameType: "ROOT", route: "root", inclusiveDurationMillis: 5, selfDurationMillis: 4, failureIds: [] },
    { frameId: "model", parentFrameId: "root", childFrameIds: [], frameType: "MODEL_CALL", route: "model", inclusiveDurationMillis: 1, selfDurationMillis: 1, failureIds: ["terminal-1"] },
  ], hasMore: false, nextCursor: null });
  api.getTraceFailures.mockResolvedValueOnce({ source: "TARGET", targetScopeId: "scope-1", items: [{ failureId: "terminal-1", terminal: true, sequence: 4, frameId: "model" }], hasMore: false, nextCursor: null });

  render(<MemoryRouter><TraceExplorer traceId="trace-1" /><LocationProbe /></MemoryRouter>);
  await vi.waitFor(() => expect(screen.getByLabelText("location")).toHaveTextContent("frameId=model"));
  fireEvent.click(screen.getByRole("button", { name: /^ROOT: root/ }));
  await vi.waitFor(() => expect(screen.getByLabelText("location")).toHaveTextContent("frameId=root"));
  expect(screen.getByLabelText("location")).not.toHaveTextContent("failureId=");
  await new Promise((resolve) => setTimeout(resolve, 0));
  expect(screen.getByLabelText("location")).toHaveTextContent("frameId=root");
});

test("deep frame selection loads complete ancestry for hierarchy and breadcrumbs", async () => {
  const result = (items: unknown[]) => ({ source: "TARGET", targetScopeId: "scope-1", items, hasMore: false, nextCursor: null });
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
  api.getTraceAnalysisSummary.mockResolvedValueOnce({ source: "TARGET", targetScopeId: "scope-1", traceId: "trace-1", sessionId: "session-1", outcome: "FAILED", terminalFailureId: "terminal-1", recordCount: 120, frameCount: 1, attemptCount: 1, retryCount: 1, validationCount: 1, failureCount: 2, payloadCount: 1, gapCount: 1, uncertaintyCount: 1, rootFrameIds: ["f-1"], usageComplete: false, configuredLimits: null });
  api.getTraceFailures.mockResolvedValueOnce({ source: "TARGET", targetScopeId: "scope-1", items: [{ failureId: "recovered", terminal: false, sequence: 3, timestampMillis: 3, recordType: "ERROR_RECORDED", frameId: "", route: "", attemptId: "", retrySequenceId: "", validationStatus: "" }], hasMore: true, nextCursor: "failure-next" }).mockResolvedValueOnce({ source: "TARGET", targetScopeId: "scope-1", items: [{ failureId: "terminal-1", terminal: true, sequence: 119, timestampMillis: 119, recordType: "ERROR_RECORDED", frameId: "f-1", route: "hello", attemptId: "a-1", retrySequenceId: "r-1", validationStatus: "exhausted" }], hasMore: false, nextCursor: null });
  render(<MemoryRouter><TraceExplorer traceId="trace-1" /><LocationProbe /></MemoryRouter>);
  expect(await screen.findByRole("heading", { name: "Terminal failure evidence" })).toBeInTheDocument();
  expect(screen.getByRole("region", { name: "Trace failure details" })).toHaveClass("trace-failure-panel");
  await screen.findByText("ERROR_RECORDED sequence 119");
  await vi.waitFor(() => expect(screen.getByLabelText("location")).toHaveTextContent("frameId=f-1"));
  expect(screen.getByText(/does not identify root cause/)).toBeInTheDocument();
  expect(api.getContentRange).not.toHaveBeenCalled();
  expect(api.getRawRecordRange).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "Show in hierarchy" }));
  await vi.waitFor(() => expect(screen.getByRole("button", { name: /SKILL: hello/ })).toHaveFocus());
});

test("view error from the exact failure record focuses the failure panel", async () => {
  api.getTraceAnalysisSummary.mockResolvedValueOnce({ source: "TARGET", targetScopeId: "scope-1", traceId: "trace-1", sessionId: "session-1", outcome: "FAILED", terminalFailureId: "terminal-1", recordCount: 15, frameCount: 1, attemptCount: 1, retryCount: 1, validationCount: 0, failureCount: 1, payloadCount: 1, gapCount: 0, uncertaintyCount: 0, rootFrameIds: ["f-1"], usageComplete: false, configuredLimits: null });
  api.getTraceRecords.mockResolvedValueOnce({ source: "TARGET", targetScopeId: "scope-1", items: [{ sequence: 15, type: "ERROR_RECORDED", frameId: "f-1", route: "hello", timestampMillis: 15, representation: "LOGICAL", content: { role: "RECONSTRUCTED", contentType: "application/json", encoding: "UTF8", retainedBytes: 1, available: true, complete: true, inlineEligibility: true, contentRef: "p-1" } }], hasMore: false, nextCursor: null });
  api.getTraceFailures.mockResolvedValueOnce({ source: "TARGET", targetScopeId: "scope-1", items: [{ failureId: "terminal-1", terminal: true, sequence: 15, timestampMillis: 15, recordType: "ERROR_RECORDED", frameId: "f-1", route: "hello", attemptId: "a-1", retrySequenceId: "r-1", validationStatus: "" }], hasMore: false, nextCursor: null });
  render(<MemoryRouter initialEntries={["/?view=records"]}><TraceExplorer traceId="trace-1" /></MemoryRouter>);

  fireEvent.click(await screen.findByRole("button", { name: "View error" }));

  expect(screen.getByRole("region", { name: "Trace failure details" })).toHaveFocus();
});

test("selected frames link only exact current registered skill names", async () => {
	const framePage = { source: "TARGET", targetScopeId: "scope-1", items: [{ frameId: "f-1", parentFrameId: null, childFrameIds: [], frameType: "SKILL", route: "root.child", inclusiveDurationMillis: 1, selfDurationMillis: 1, skillNames: ["exact.skill", "Missing.Skill"] }], hasMore: false, nextCursor: null };
	api.getTraceFrames.mockResolvedValueOnce(framePage).mockResolvedValueOnce(framePage);
  api.listSkills.mockResolvedValueOnce({ source: "TARGET", targetScopeId: "scope-1", items: [{ registeredName: "exact.skill", sourcePath: "<unsafe>" }, { registeredName: "missing.skill", sourcePath: "other" }], hasMore: false, nextCursor: null });
  render(<MemoryRouter initialEntries={["/?frameId=f-1"]}><TraceExplorer traceId="trace-1" /></MemoryRouter>);
  expect(await screen.findByRole("link", { name: "exact.skill" })).toHaveAttribute("href", "/skills/exact.skill?targetScopeId=scope-1");
  expect(screen.getByText("Missing.Skill").closest("li")).toHaveTextContent("not in current registered catalog");
  expect(screen.queryByRole("link", { name: "Missing.Skill" })).toBeNull();
  expect(screen.queryByText("<unsafe>")).toBeNull();
});
