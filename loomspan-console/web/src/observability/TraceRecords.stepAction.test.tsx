import { fireEvent, render, screen, within } from "@testing-library/react";
import { beforeEach, expect, test, vi } from "vitest";
import { getRawRecordRange, getTraceRecords } from "../api/client";
import type { TraceAnalysisPage, TraceRange, TraceRecord } from "../api/contracts";
import { TraceRecords } from "./TraceRecords";

vi.mock("../api/client", () => ({
  getPayloadRange: vi.fn(),
  getRawRecordRange: vi.fn(),
  getTraceRecords: vi.fn(),
}));

const getRawRecordRangeMock = vi.mocked(getRawRecordRange);
const getTraceRecordsMock = vi.mocked(getTraceRecords);

function record(sequence: number, type: string): TraceRecord {
  return {
    sequence,
    type,
    frameId: "step-frame",
    parentFrameId: "root-frame",
    frameType: "STEP_EXECUTION",
    route: "handleBilling#step-1",
    threadName: "worker",
    timestampMillis: sequence,
    representation: "LOGICAL",
    isChunk: false,
    isEnvelope: false,
    payloadId: "",
  };
}

function range(value: unknown): TraceRange {
  const content = JSON.stringify(value);
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
  };
}

function page(items: TraceRecord[]): TraceAnalysisPage<TraceRecord> {
  return { targetScopeId: "scope-1", items, hasMore: false, nextCursor: null };
}

function renderRecord(current: TraceRecord) {
  render(<TraceRecords traceId="trace-1" records={[current]} attempts={[]} retries={[]} failures={[]} validations={[]} gaps={[]} uncertainties={[]} payloads={[]} onSelectRecord={vi.fn()} onSelectFailure={vi.fn()} onPayload={vi.fn()} />);
  fireEvent.click(screen.getByRole("button", { name: "Action details" }));
}

beforeEach(() => {
  getRawRecordRangeMock.mockReset();
  getTraceRecordsMock.mockReset();
});

test("shows the proposed task and tool without claiming execution", async () => {
  getRawRecordRangeMock.mockResolvedValueOnce(range({
    metadata: { taskId: "task-lookup-customer", stepAction: "CALL_TOOL", toolName: "lookupCustomer" },
    data: {},
  })).mockResolvedValueOnce(range({
    data: {
      planId: "plan-1",
      capabilityName: "handleBilling",
      tasks: [{ taskId: "task-lookup-customer", title: "Retrieve Customer Account Details" }],
    },
  }));
  getTraceRecordsMock.mockResolvedValue(page([record(85, "PLAN_CREATED")]));

  renderRecord(record(94, "STEP_ACTION_PROPOSED"));

  const details = await screen.findByRole("region", { name: "Action details for record 94" });
  expect(details).toHaveTextContent("Proposed");
  expect(details).toHaveTextContent("Call tool");
  expect(details).toHaveTextContent("Retrieve Customer Account Details");
  expect(details).toHaveTextContent("task-lookup-customer");
  expect(details).toHaveTextContent("lookupCustomer");
  expect(details).toHaveTextContent("not accepted or executed");
});

test("joins a validated action to its proposal without claiming tool success", async () => {
  getRawRecordRangeMock.mockImplementation(async (_traceId, sequence) => {
    if (sequence === 95) return range({ metadata: { stepAction: "CALL_TOOL" }, data: {} });
    if (sequence === 94) return range({ metadata: { taskId: "task-lookup-customer", stepAction: "CALL_TOOL", toolName: "lookupCustomer" }, data: {} });
    return range({ data: { planId: "plan-1", capabilityName: "handleBilling", tasks: [{ taskId: "task-lookup-customer", title: "Retrieve Customer Account Details" }] } });
  });
  getTraceRecordsMock.mockImplementation(async (_traceId, _cursor, filter) => {
    if (filter?.types?.includes("STEP_ACTION_PROPOSED")) return page([record(94, "STEP_ACTION_PROPOSED")]);
    return page([record(85, "PLAN_CREATED")]);
  });

  renderRecord(record(95, "STEP_ACTION_VALIDATED"));

  const details = await screen.findByRole("region", { name: "Action details for record 95" });
  expect(details).toHaveTextContent("Accepted");
  expect(details).toHaveTextContent("Retrieve Customer Account Details");
  expect(details).toHaveTextContent("record 94");
  expect(details).toHaveTextContent("does not mean the tool ran or succeeded");
});

test("shows a rejected action reason and retry facts", async () => {
  getRawRecordRangeMock.mockResolvedValue(range({
    metadata: { retry: 1, reason: "Task 'task-missing' does not exist in the plan." },
    data: { stepAction: "CALL_TOOL" },
  }));
  getTraceRecordsMock.mockResolvedValue(page([]));

  renderRecord(record(93, "STEP_ACTION_REJECTED"));

  const details = await screen.findByRole("region", { name: "Action details for record 93" });
  expect(details).toHaveTextContent("Rejected");
  expect(details).toHaveTextContent("Earlier rejected attempts");
  expect(details).toHaveTextContent("Task 'task-missing' does not exist in the plan.");
  expect(details).toHaveTextContent("before execution");
});

test("shows parse rejection response excerpts without inventing an action", async () => {
  getRawRecordRangeMock.mockResolvedValue(range({
    metadata: { retry: 0, reason: "Failed to parse model response as StepAction" },
    data: { rawResponse: "not valid json" },
  }));
  getTraceRecordsMock.mockResolvedValue(page([]));

  renderRecord(record(93, "STEP_ACTION_REJECTED"));

  const details = await screen.findByRole("region", { name: "Action details for record 93" });
  expect(details).toHaveTextContent("Model response excerpt");
  expect(details).toHaveTextContent("not valid json");
  expect(within(details).queryByText("Action")).toBeNull();
});
