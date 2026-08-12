import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, expect, test, vi } from "vitest";
import { getRawRecordRange } from "../api/client";
import type { TraceRecord, TraceRange } from "../api/contracts";
import { TraceRecords } from "./TraceRecords";

vi.mock("../api/client", () => ({ getRawRecordRange: vi.fn() }));

const getRawRecordRangeMock = vi.mocked(getRawRecordRange);
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

function renderPlanRecord() {
  render(<TraceRecords traceId="trace-1" records={[record]} attempts={[]} retries={[]} failures={[]} validations={[]} gaps={[]} uncertainties={[]} payloads={[]} onSelectRecord={vi.fn()} onSelectFailure={vi.fn()} onRaw={vi.fn()} onPayload={vi.fn()} />);
  fireEvent.click(screen.getByRole("button", { name: "Show Plan" }));
}

beforeEach(() => getRawRecordRangeMock.mockReset());

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

test("decodes a JSON string stored in the plan data field", async () => {
  const plan = { planId: "plan-2", tasks: [] };
  getRawRecordRangeMock.mockResolvedValue(range(JSON.stringify({ data: JSON.stringify(plan) })));

  renderPlanRecord();

  const output = await screen.findByText((_, element) => element?.tagName === "PRE");
  expect(output).toHaveTextContent(JSON.stringify(plan, null, 2), { normalizeWhitespace: false });
});

test("reports malformed plan records without displaying a truncated fallback", async () => {
  getRawRecordRangeMock.mockResolvedValue(range("{not-json"));

  renderPlanRecord();

  expect(await screen.findByRole("alert")).toHaveTextContent("Plan could not be displayed");
  expect(screen.queryByText("{not-json")).toBeNull();
});
