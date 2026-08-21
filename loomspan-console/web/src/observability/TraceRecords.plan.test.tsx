import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, expect, test, vi } from "vitest";
import { getContentRange, getRawRecordRange, getTraceAttempts, getTraceRecords } from "../api/client";
import type { TraceRecord, TraceRange } from "../api/contracts";
import { TraceRecords } from "./TraceRecords";

vi.mock("../api/client", () => ({ getContentRange: vi.fn(), getRawRecordRange: vi.fn(), getTraceAttempts: vi.fn(), getTraceRecords: vi.fn() }));

const getRawRecordRangeMock = vi.mocked(getRawRecordRange);
const getTraceRecordsMock = vi.mocked(getTraceRecords);
const getTraceAttemptsMock = vi.mocked(getTraceAttempts);
const getContentRangeMock = vi.mocked(getContentRange);
const record: TraceRecord = {
  sequence: 7,
  type: "PLAN_CREATED",
  frameId: "",
  parentFrameId: "",
  frameType: "",
  route: "",
  threadName: "worker",
  timestampMillis: 0,
  representation: "LOGICAL",
  isChunk: false,
  isEnvelope: true,

};

function range(content: string, overrides: Partial<TraceRange> = {}): TraceRange {
  return {
    source: "TARGET",
    targetScopeId: "scope-1",
    actualStart: 0,
    actualEnd: content.length,
    totalLength: content.length,
    contentType: "application/json",
    encoding: "TEXT",
    content,
    hasMore: false,
    nextCursor: null,
    ...overrides,
  };
}

function renderPlanRecord(value = record) {
  render(<TraceRecords traceId="trace-1" records={[value]} failures={[]} onSelectRecord={vi.fn()} onSelectFailure={vi.fn()} onContent={vi.fn()} />);
  fireEvent.click(screen.getByRole("button", { name: "Show Plan" }));
}

beforeEach(() => {
  getRawRecordRangeMock.mockReset();
  getTraceRecordsMock.mockReset();
  getTraceAttemptsMock.mockReset();
  getContentRangeMock.mockReset();
});

test.each([
  ["primary", "trace-root", "trace-root", "planning-primary"],
  ["nested", "trace-root", "nested-mission", "planning-nested"],
])("presents actionable %s plan landmarks without trace or plan IDs", (_kind, traceRootFrameId, missionFrameId, planningFrameId) => {
  const onFrameRecord = vi.fn();
  const plan = { planId: `plan-${_kind}`, sequence: 7, traceRootFrameId, missionFrameId, planningFrameId };
  render(<TraceRecords traceId="trace-1" records={[{ ...record, plan }]} failures={[]} onSelectRecord={vi.fn()} onSelectFailure={vi.fn()} onFrameRecord={onFrameRecord} onContent={vi.fn()} />);

  fireEvent.click(screen.getByRole("button", { name: "Show Plan" }));
  const landmarks = screen.getByLabelText("Plan details for record 7");
  expect(landmarks).toHaveTextContent(missionFrameId);
  expect(landmarks).toHaveTextContent(planningFrameId);
  expect(landmarks).not.toHaveTextContent(`plan-${_kind}`);
  if (traceRootFrameId !== missionFrameId) expect(landmarks).not.toHaveTextContent(traceRootFrameId);
  fireEvent.click(screen.getByRole("button", { name: planningFrameId }));
  expect(onFrameRecord).toHaveBeenCalledWith(planningFrameId);
});

test("omits the planning frame when the plan record already belongs to it", () => {
  const plan = { planId: "plan-1", sequence: 7, traceRootFrameId: "root", missionFrameId: "mission", planningFrameId: "planning" };
  render(<TraceRecords traceId="trace-1" records={[{ ...record, frameId: "planning", plan }]} failures={[]} onSelectRecord={vi.fn()} onSelectFailure={vi.fn()} onContent={vi.fn()} />);
  fireEvent.click(screen.getByRole("button", { name: "Show Plan" }));
  expect(screen.getByLabelText("Plan details for record 7")).not.toHaveTextContent("Planning frame");
});

test("summarizes the accepted attempt and links every version in plan history", async () => {
  const onSelectRecord = vi.fn();
  const plan = { planId: "plan-1", sequence: 7, traceRootFrameId: "root", missionFrameId: "mission", planningFrameId: "planning", attemptId: "accepted", retrySequenceId: "retry-1" };
  const content = { role: "DATA" as const, contentType: "application/json", encoding: "UTF8" as const, retainedBytes: 1, available: true, complete: true, inlineEligibility: true, inlineContent: JSON.stringify({ planId: "plan-1", tasks: [] }) };
  const created = { ...record, frameId: "planning", plan, content };
  const updated = { ...record, sequence: 19, type: "PLAN_UPDATED", frameId: "step", plan: { ...plan, sequence: 19 }, content };
  const response = { ...record, sequence: 6, type: "MODEL_RESPONSE_RECEIVED", frameId: "planning" };
  getTraceRecordsMock
    .mockResolvedValueOnce({ source: "TARGET", targetScopeId: "scope-1", items: [updated, created], hasMore: false, nextCursor: null })
    .mockResolvedValueOnce({ source: "TARGET", targetScopeId: "scope-1", items: [response], hasMore: false, nextCursor: null });
  getTraceAttemptsMock.mockResolvedValue({
    source: "TARGET", targetScopeId: "scope-1", hasMore: false, nextCursor: null,
    items: [
      { retrySequenceId: "retry-1", attemptId: "first", attemptNumber: 1 },
      { retrySequenceId: "retry-1", attemptId: "accepted", attemptNumber: 2 },
      { retrySequenceId: "retry-1", attemptId: "third", attemptNumber: 3 },
    ],
  } as Awaited<ReturnType<typeof getTraceAttempts>>);
  render(<TraceRecords traceId="trace-1" records={[created]} failures={[]} onSelectRecord={onSelectRecord} onSelectFailure={vi.fn()} onContent={vi.fn()} />);

  fireEvent.click(screen.getByRole("button", { name: "Show Plan" }));
  fireEvent.click(await screen.findByRole("button", { name: "Attempt 2 of 3" }));
  expect(onSelectRecord).toHaveBeenCalledWith(response);
  const history = await screen.findByRole("region", { name: "Plan history" });
  expect(history).toHaveTextContent("Plan History: 7 Created / 19 Updated");
  fireEvent.click(screen.getByRole("button", { name: "19 Updated" }));
  expect(onSelectRecord).toHaveBeenCalledWith(updated);
  expect(history).not.toHaveTextContent("plan-1");
});

test("loads every content range and pretty prints only the plan data", async () => {
  const plan = { planId: "plan-1", tasks: [{ taskId: "task-1", title: "Friendly title" }] };
  const raw = JSON.stringify(plan);
  const split = 40;
  getContentRangeMock
    .mockResolvedValueOnce(range(raw.slice(0, split), { actualEnd: split, totalLength: raw.length, hasMore: true, nextCursor: "next" }))
    .mockResolvedValueOnce(range(raw.slice(split), { actualStart: split, actualEnd: raw.length, totalLength: raw.length }));

  renderPlanRecord({ ...record, content: { role: "DATA", contentType: "application/json", encoding: "UTF8", retainedBytes: raw.length, available: true, complete: true, inlineEligibility: true, contentRef: "plan-content" } });

  const output = await screen.findByText((_, element) => element?.tagName === "PRE");
  expect(output).toHaveTextContent(JSON.stringify(plan, null, 2), { normalizeWhitespace: false });
  expect(getContentRangeMock).toHaveBeenNthCalledWith(1, "trace-1", "plan-content", undefined, "TARGET");
  expect(getContentRangeMock).toHaveBeenNthCalledWith(2, "trace-1", "plan-content", "next", "TARGET");
});

test("reconstructs a chunked plan payload using the current trace contract", async () => {
  const plan = { planId: "plan-2", tasks: [] };
  const raw = JSON.stringify(plan);
  getContentRangeMock.mockResolvedValue(range(raw));

  renderPlanRecord({ ...record, content: { role: "RECONSTRUCTED", contentType: "application/json", encoding: "UTF8", retainedBytes: raw.length, available: true, complete: true, inlineEligibility: true, contentRef: "payload-plan" }, isEnvelope: true });

  const output = await screen.findByText((_, element) => element?.tagName === "PRE");
  expect(output).toHaveTextContent(JSON.stringify(plan, null, 2), { normalizeWhitespace: false });
  expect(getContentRangeMock).toHaveBeenCalledWith("trace-1", "payload-plan", undefined, "TARGET");
  expect(getRawRecordRangeMock).not.toHaveBeenCalled();
});

test("reports malformed plan records without displaying a truncated fallback", async () => {
  renderPlanRecord({ ...record, content: { role: "DATA", contentType: "application/json", encoding: "UTF8", retainedBytes: 9, available: true, complete: true, inlineEligibility: true, inlineContent: "{not-json" } });

  expect(await screen.findByRole("alert")).toHaveTextContent("Plan could not be displayed");
  expect(screen.queryByText("{not-json")).toBeNull();
});

test("summarizes a plan update against the latest earlier snapshot with the same plan ID", async () => {
  const previousPlan = {
    planId: "plan-1",
    capabilityName: "resolveSupportCase",
    status: "VALID",
    activeTaskId: null,
    tasks: [{ taskId: "task-understand", title: "Understand intent", intent: "Extract support intent from the customer message.", status: "PENDING", note: null }],
  };
  const currentPlan = {
    ...previousPlan,
    activeTaskId: "task-understand",
    tasks: [{ taskId: "task-understand", title: "Understand intent", status: "IN_PROGRESS", note: "Starting tool understandIntent" }],
  };
  const nestedPlan = { planId: "nested-plan", tasks: [] };
  const withPlan = (sequence: number, type: string, plan: unknown): TraceRecord => ({ ...record, sequence, type, content: { role: "DATA", contentType: "application/json", encoding: "UTF8", retainedBytes: 1, available: true, complete: true, inlineEligibility: true, inlineContent: JSON.stringify(plan) } });
  const updatedRecord = withPlan(41, "PLAN_UPDATED", currentPlan);
  const previousRecord = withPlan(26, "PLAN_CREATED", previousPlan);
  const interleavedRecord = withPlan(35, "PLAN_UPDATED", nestedPlan);
  getTraceRecordsMock.mockResolvedValue({
    source: "TARGET",
    targetScopeId: "scope-1",
    items: [previousRecord, interleavedRecord],
    hasMore: false,
    nextCursor: null,
  });

  render(<TraceRecords traceId="trace-1" records={[updatedRecord]} failures={[]} onSelectRecord={vi.fn()} onSelectFailure={vi.fn()} onContent={vi.fn()} />);
  fireEvent.click(screen.getByRole("button", { name: "View changes" }));

  const changes = await screen.findByRole("region", { name: "Plan changes" });
  expect(changes).toHaveTextContent("Changes since record 26");
  expect(changes).toHaveTextContent(/Active task:.*None.*Understand intent/);
  expect(changes).toHaveTextContent("Extract support intent from the customer message.");
  expect(changes).toHaveTextContent(/Status:.*Pending.*In progress/);
  expect(changes).toHaveTextContent(/Note:.*None.*Starting tool understandIntent/);
  expect(getTraceRecordsMock).toHaveBeenCalledWith("trace-1", undefined, {
    types: ["PLAN_CREATED", "PLAN_UPDATED"],
	maxSequence: 40,
  }, "TARGET");
  expect(getRawRecordRangeMock).not.toHaveBeenCalled();

  fireEvent.click(screen.getByRole("tab", { name: "Full plan" }));
  expect(screen.getByRole("tabpanel")).toHaveTextContent(JSON.stringify(currentPlan, null, 2), { normalizeWhitespace: false });
});
