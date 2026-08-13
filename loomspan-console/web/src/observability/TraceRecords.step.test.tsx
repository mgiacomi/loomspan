import { fireEvent, render, screen, within } from "@testing-library/react";
import { beforeEach, expect, test, vi } from "vitest";
import { getRawRecordRange } from "../api/client";
import type { TraceRange, TraceRecord } from "../api/contracts";
import { TraceRecords } from "./TraceRecords";

vi.mock("../api/client", () => ({
  getPayloadRange: vi.fn(),
  getRawRecordRange: vi.fn(),
  getTraceRecords: vi.fn(),
}));

const getRawRecordRangeMock = vi.mocked(getRawRecordRange);

const stepRecord: TraceRecord = {
  sequence: 88,
  type: "STEP_STARTED",
  frameId: "step-frame",
  parentFrameId: "root-frame",
  frameType: "STEP_EXECUTION",
  route: "handleBilling#step-1",
  threadName: "worker",
  timestampMillis: 88,
  representation: "LOGICAL",
  isChunk: false,
  isEnvelope: false,
  payloadId: "",
};

function range(content: string): TraceRange {
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
  };
}

function renderStep() {
  render(<TraceRecords traceId="trace-1" records={[stepRecord]} failures={[]} onSelectRecord={vi.fn()} onSelectFailure={vi.fn()} onPayload={vi.fn()} />);
}

beforeEach(() => getRawRecordRangeMock.mockReset());

test("shows authoritative step-start facts without attributing a future task", async () => {
  getRawRecordRangeMock.mockResolvedValue(range(JSON.stringify({
    recordType: "STEP_STARTED",
    route: "handleBilling#step-1",
    metadata: { stepNumber: 1, readyTasks: 3 },
    data: { planStatus: "VALID" },
  })));
  renderStep();

  expect(screen.queryByRole("region", { name: "Step details for record 88" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Step details" }));

  const details = await screen.findByRole("region", { name: "Step details for record 88" });
  expect(within(details).getByText("handleBilling")).toBeVisible();
  expect(within(details).getByText("3")).toBeVisible();
  expect(details).toHaveTextContent("Plan status");
  expect(details).toHaveTextContent("valid");
  expect(details).toHaveTextContent("No task or action has been selected yet");
  expect(details).toHaveTextContent("STEP_ACTION_PROPOSED");
  expect(getRawRecordRangeMock).toHaveBeenCalledWith("trace-1", 88, undefined, "TARGET");
});

test("reports invalid step facts instead of guessing", async () => {
  getRawRecordRangeMock.mockResolvedValue(range(JSON.stringify({
    metadata: { stepNumber: 1 },
    data: { planStatus: "VALID" },
  })));
  renderStep();

  fireEvent.click(screen.getByRole("button", { name: "Step details" }));

  expect(await screen.findByRole("alert")).toHaveTextContent("Step details could not be displayed");
  expect(screen.queryByText("No task or action has been selected yet")).toBeNull();
});
