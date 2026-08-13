import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, expect, test, vi } from "vitest";
import { getPayloadRange, getRawRecordRange, getTraceRecords } from "../api/client";
import type { TraceRecord, TraceRange } from "../api/contracts";
import { TraceRecords } from "./TraceRecords";

vi.mock("../api/client", () => ({ getPayloadRange: vi.fn(), getRawRecordRange: vi.fn(), getTraceRecords: vi.fn() }));

const getRawRecordRangeMock = vi.mocked(getRawRecordRange);
const getTraceRecordsMock = vi.mocked(getTraceRecords);
const getPayloadRangeMock = vi.mocked(getPayloadRange);
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
  payloadId: "",
};

function range(content: string, overrides: Partial<TraceRange> = {}): TraceRange {
  return {
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
  render(<TraceRecords traceId="trace-1" records={[value]} failures={[]} onSelectRecord={vi.fn()} onSelectFailure={vi.fn()} onPayload={vi.fn()} />);
  fireEvent.click(screen.getByRole("button", { name: "Show Plan" }));
}

beforeEach(() => {
  getRawRecordRangeMock.mockReset();
  getTraceRecordsMock.mockReset();
  getPayloadRangeMock.mockReset();
});

test("loads every raw-record range and pretty prints only the plan data", async () => {
  const plan = { planId: "plan-1", tasks: [{ taskId: "task-1", title: "Friendly title" }] };
  const raw = JSON.stringify({ traceId: "trace-1", recordType: "PLAN_CREATED", data: plan });
  const split = 40;
  getRawRecordRangeMock
    .mockResolvedValueOnce(range(raw.slice(0, split), { actualEnd: split, totalLength: raw.length, hasMore: true, nextCursor: "next" }))
    .mockResolvedValueOnce(range(raw.slice(split), { actualStart: split, actualEnd: raw.length, totalLength: raw.length }));

  renderPlanRecord();

  const output = await screen.findByText((_, element) => element?.tagName === "PRE");
  expect(output).toHaveTextContent(JSON.stringify(plan, null, 2), { normalizeWhitespace: false });
  expect(screen.getByRole("region", { name: "Plan for record 7" })).not.toHaveTextContent("traceId");
  expect(getRawRecordRangeMock).toHaveBeenNthCalledWith(1, "trace-1", 7, undefined);
  expect(getRawRecordRangeMock).toHaveBeenNthCalledWith(2, "trace-1", 7, "next");
});

test("reconstructs a chunked plan payload using the current trace contract", async () => {
  const plan = { planId: "plan-2", tasks: [] };
  const raw = JSON.stringify(plan);
  getPayloadRangeMock.mockResolvedValue(range(raw));

  renderPlanRecord({ ...record, payloadId: "payload-plan", isEnvelope: true });

  const output = await screen.findByText((_, element) => element?.tagName === "PRE");
  expect(output).toHaveTextContent(JSON.stringify(plan, null, 2), { normalizeWhitespace: false });
  expect(getPayloadRangeMock).toHaveBeenCalledWith("trace-1", "payload-plan", undefined);
  expect(getRawRecordRangeMock).not.toHaveBeenCalled();
});

test("reports malformed plan records without displaying a truncated fallback", async () => {
  getRawRecordRangeMock.mockResolvedValue(range("{not-json"));

  renderPlanRecord();

  expect(await screen.findByRole("alert")).toHaveTextContent("Plan could not be displayed");
  expect(screen.queryByText("{not-json")).toBeNull();
});

test("summarizes a plan update against the latest earlier snapshot with the same plan ID", async () => {
  const updatedRecord = { ...record, sequence: 41, type: "PLAN_UPDATED" };
  const previousRecord = { ...record, sequence: 26 };
  const interleavedRecord = { ...record, sequence: 35 };
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
  getTraceRecordsMock.mockResolvedValue({
    targetScopeId: "scope-1",
    items: [previousRecord, interleavedRecord],
    hasMore: false,
    nextCursor: null,
  });
  getRawRecordRangeMock.mockImplementation((_traceId, sequence) => {
    const plan = sequence === 41 ? currentPlan : sequence === 35 ? nestedPlan : previousPlan;
    return Promise.resolve(range(JSON.stringify({ data: plan })));
  });

  render(<TraceRecords traceId="trace-1" records={[updatedRecord]} failures={[]} onSelectRecord={vi.fn()} onSelectFailure={vi.fn()} onPayload={vi.fn()} />);
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
  });
  expect(getRawRecordRangeMock.mock.calls.map((call) => call[1])).toEqual([41, 35, 26]);

  fireEvent.click(screen.getByRole("tab", { name: "Full plan" }));
  expect(screen.getByRole("tabpanel")).toHaveTextContent(JSON.stringify(currentPlan, null, 2), { normalizeWhitespace: false });
});
