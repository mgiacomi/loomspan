import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, test, vi } from "vitest";
import { getRawRecordRange, getTraceRecords } from "../api/client";
import type { TraceAnalysisPage, TraceRange, TraceRecord } from "../api/contracts";
import { TraceRecords } from "./TraceRecords";

vi.mock("../api/client", () => ({
  getContentRange: vi.fn(),
  getRawRecordRange: vi.fn(),
  getTraceRecords: vi.fn(),
}));

const getRawRecordRangeMock = vi.mocked(getRawRecordRange);
const getTraceRecordsMock = vi.mocked(getTraceRecords);

function record(sequence = 100, overrides: Partial<TraceRecord> = {}): TraceRecord {
  return {
    sequence,
    type: "TOOL_CALL_STARTED",
    route: "lookupCustomer",
    frameId: "tool-frame",
    parentFrameId: "step-frame",
    frameType: "TOOL_INVOCATION",
    threadName: "worker",
    timestampMillis: sequence,
    representation: "LOGICAL",
    isChunk: false,
    isEnvelope: false,

    ...overrides,
  };
}

function planRecord(): TraceRecord {
  const plan = { planId: "plan-1", capabilityName: "handleBilling", tasks: [{ taskId: "task-customer", title: "Retrieve customer" }] };
  return { ...record(85, { type: "PLAN_CREATED", route: "handleBilling", frameId: "plan-frame" }), content: { role: "DATA", contentType: "application/json", encoding: "UTF8", retainedBytes: 1, available: true, complete: true, inlineEligibility: true, inlineContent: JSON.stringify(plan) } };
}

function rangeContent(content: string, overrides: Partial<TraceRange> = {}): TraceRange {
  return {
    source: "TARGET",
    targetScopeId: "scope-1",
    actualStart: 0,
    actualEnd: content.length,
    totalLength: content.length,
    contentType: "application/x-ndjson",
    encoding: "TEXT",
    content,
    hasMore: false,
    nextCursor: null,
    ...overrides,
  };
}

function range(value: unknown, overrides: Partial<TraceRange> = {}): TraceRange {
  return rangeContent(JSON.stringify(value), overrides);
}

function page(items: TraceRecord[], overrides: Partial<TraceAnalysisPage<TraceRecord>> = {}): TraceAnalysisPage<TraceRecord> {
  return { ...overrides, source: "TARGET", targetScopeId: "scope-1", items, hasMore: false, nextCursor: null };
}

function renderToolInput(current = record()) {
  render(<TraceRecords traceId="trace-1" records={[current]} failures={[]} onSelectRecord={vi.fn()} onSelectFailure={vi.fn()} onContent={vi.fn()} />);
}

beforeEach(() => {
  getRawRecordRangeMock.mockReset();
  getTraceRecordsMock.mockReset();
});

test("shows planned tool input only after explicit selection", async () => {
  getRawRecordRangeMock.mockImplementation(async (_traceId, sequence) => sequence === 100
    ? range({
      metadata: { capabilityName: "lookupCustomer", linkedTaskId: "task-customer" },
      data: {
        eventId: "event-1",
        capabilityName: "lookupCustomer",
        linkedTaskId: "task-customer",
        details: { arguments: { customerId: "CUST-1001", active: true } },
        note: "Use the verified customer ID.",
      },
    })
    : range({ data: { planId: "plan-1", capabilityName: "handleBilling", tasks: [{ taskId: "task-customer", title: "Retrieve customer" }] } }));
  getTraceRecordsMock.mockImplementation(async (_traceId, _cursor, filter) => filter?.types?.includes("STEP_STARTED")
    ? page([record(88, { type: "STEP_STARTED", route: "handleBilling#step-1", frameId: "step-frame", parentFrameId: "skill-frame" })])
    : page([planRecord()]));

  renderToolInput();
  const action = screen.getByRole("button", { name: "Tool input" });
  expect(action).toHaveAttribute("aria-expanded", "false");
  expect(screen.queryByText("CUST-1001")).toBeNull();
  expect(getRawRecordRangeMock).not.toHaveBeenCalled();

  fireEvent.click(action);
  const detail = await screen.findByRole("region", { name: "Tool input for record 100" });
  expect(action).toHaveAttribute("aria-expanded", "true");
  expect(detail).toHaveTextContent("Planned");
  expect(detail).toHaveTextContent("Retrieve customer");
  expect(detail).toHaveTextContent("task-customer");
  expect(detail).toHaveTextContent("event-1");
  expect(detail).toHaveTextContent("Use the verified customer ID.");
  expect(within(detail).getByText(/"customerId": "CUST-1001"/)).toBeVisible();
  expect(within(detail).getByText(/"active": true/)).toBeVisible();
});

test("resolves a planned task title through an ordinary mission model frame", async () => {
  getRawRecordRangeMock.mockImplementation(async (_traceId, sequence) => sequence === 100
    ? range({
      metadata: { capabilityName: "lookupCustomer", linkedTaskId: "task-customer" },
      data: {
        eventId: "event-mission-tool",
        capabilityName: "lookupCustomer",
        linkedTaskId: "task-customer",
        details: { arguments: { customerId: "CUST-1001" } },
      },
    })
    : range({ data: { planId: "plan-1", capabilityName: "handleBilling", tasks: [{ taskId: "task-customer", title: "Retrieve customer" }] } }));
  getTraceRecordsMock.mockImplementation(async (_traceId, _cursor, filter) => {
    if (filter?.types?.includes("STEP_STARTED")) return page([]);
    if (filter?.types?.includes("FRAME_OPENED")) {
      return page([record(90, {
        type: "FRAME_OPENED",
        route: "handleBilling#mission-model",
        frameId: "model-frame",
        parentFrameId: "root-frame",
        frameType: "MODEL_CALL",
      })]);
    }
    return page([planRecord()]);
  });

  renderToolInput(record(100, { parentFrameId: "model-frame" }));
  fireEvent.click(screen.getByRole("button", { name: "Tool input" }));

  const detail = await screen.findByRole("region", { name: "Tool input for record 100" });
  expect(detail).toHaveTextContent("Retrieve customer");
  expect(getTraceRecordsMock).toHaveBeenCalledWith("trace-1", undefined, expect.objectContaining({
    types: ["FRAME_OPENED"],
	frameId: "model-frame",
	}), "TARGET");
});

test.each([
  ["JSON scalar", 42, "42"],
  ["JSON encoded string", "\"hello\"", '"hello"'],
  ["plain text", "hello there", "hello there"],
])("shows unplanned %s input without inventing an outcome or task", async (_label, argumentsValue, expected) => {
  getRawRecordRangeMock.mockResolvedValue(range({
    metadata: { capabilityName: "lookupPolicy", unplanned: true },
    data: { eventId: "event-unplanned", capabilityName: "lookupPolicy", details: { arguments: argumentsValue }, note: "Not present in the plan." },
  }));
  renderToolInput(record(25, { route: "lookupPolicy", parentFrameId: "root-frame" }));

  fireEvent.click(screen.getByRole("button", { name: "Tool input" }));
  const detail = await screen.findByRole("region", { name: "Tool input for record 25" });
  expect(detail).toHaveTextContent("Unplanned");
  expect(detail).toHaveTextContent("No plan task was linked");
  expect(detail).toHaveTextContent(expected);
  expect(detail).not.toHaveTextContent("Succeeded");
  expect(detail).not.toHaveTextContent("Failed");
  expect(within(detail).queryByText("Task ID")).toBeNull();
  expect(getTraceRecordsMock).not.toHaveBeenCalled();
});

test("loads complete input across ranges and renders malicious-looking arguments inertly", async () => {
  const malicious = '<script>window.hacked=true</script> **not markdown**';
  const raw = JSON.stringify({
    metadata: { capabilityName: "unsafeLooking", unplanned: true },
    data: { eventId: "event-inert", capabilityName: "unsafeLooking", details: { arguments: malicious } },
  });
  const split = Math.floor(raw.length / 2);
  getRawRecordRangeMock
    .mockResolvedValueOnce(rangeContent(raw.slice(0, split), { actualEnd: split, totalLength: raw.length, hasMore: true, nextCursor: "next" }))
    .mockResolvedValueOnce(rangeContent(raw.slice(split), { actualStart: split, actualEnd: raw.length, totalLength: raw.length }));
  renderToolInput();

  const action = screen.getByRole("button", { name: "Tool input" });
  action.focus();
  await userEvent.keyboard("{Enter}");
  const detail = await screen.findByRole("region", { name: "Tool input for record 100" });
  expect(within(detail).getByText(malicious)).toBeVisible();
  expect(detail.querySelector("script")).toBeNull();
  expect(getRawRecordRangeMock).toHaveBeenNthCalledWith(1, "trace-1", 100, undefined, "TARGET");
  expect(getRawRecordRangeMock).toHaveBeenNthCalledWith(2, "trace-1", 100, "next", "TARGET");
});

test("keeps the recorded planned task ID when no title can be resolved", async () => {
  getRawRecordRangeMock.mockResolvedValue(range({
    metadata: { capabilityName: "lookupCustomer", linkedTaskId: "task-missing-title" },
    data: { eventId: "event-no-title", capabilityName: "lookupCustomer", linkedTaskId: "task-missing-title", details: { arguments: null } },
  }));
  getTraceRecordsMock.mockResolvedValue(page([]));
  renderToolInput();

  fireEvent.click(screen.getByRole("button", { name: "Tool input" }));
  const detail = await screen.findByRole("region", { name: "Tool input for record 100" });
  expect(detail).toHaveTextContent("task-missing-title");
  expect(within(detail).queryByText("Task", { exact: true })).toBeNull();
  expect(within(detail).getByText("null")).toBeVisible();
});

test("reports invalid tool input content instead of showing partial detail", async () => {
  getRawRecordRangeMock.mockResolvedValue(range({
    metadata: { capabilityName: "lookupCustomer", linkedTaskId: "task-1" },
    data: { eventId: "event-invalid", capabilityName: "lookupCustomer", linkedTaskId: "task-1", details: {} },
  }));
  renderToolInput();

  fireEvent.click(screen.getByRole("button", { name: "Tool input" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("did not contain arguments");
  expect(screen.queryByRole("heading", { name: "Arguments" })).toBeNull();
});

test("rejects repeated raw continuation for tool input", async () => {
  getRawRecordRangeMock.mockResolvedValue(rangeContent("{", { hasMore: true, nextCursor: "same" }));
  renderToolInput();

  fireEvent.click(screen.getByRole("button", { name: "Tool input" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("Content continuation was invalid");
  expect(getRawRecordRangeMock).toHaveBeenNthCalledWith(1, "trace-1", 100, undefined, "TARGET");
  expect(getRawRecordRangeMock).toHaveBeenNthCalledWith(2, "trace-1", 100, "same", "TARGET");
});

test("presents loading and invalid input as accessible status and alert", async () => {
  let reject!: (reason: Error) => void;
  getRawRecordRangeMock.mockReturnValue(new Promise((_resolve, rejectPromise) => { reject = rejectPromise; }));
  renderToolInput();

  fireEvent.click(screen.getByRole("button", { name: "Tool input" }));
  expect(screen.getByRole("status")).toHaveTextContent("Loading details");
  reject(new Error("Content continuation was invalid."));
  expect(await screen.findByRole("alert")).toHaveTextContent("Content continuation was invalid");
});
