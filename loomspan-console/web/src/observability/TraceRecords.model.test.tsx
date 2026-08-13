import { fireEvent, render, screen, within } from "@testing-library/react";
import { beforeEach, expect, test, vi } from "vitest";
import { getPayloadRange, getRawRecordRange } from "../api/client";
import type { TraceRange, TraceRecord } from "../api/contracts";
import { TraceRecords } from "./TraceRecords";

vi.mock("../api/client", () => ({
  getPayloadRange: vi.fn(),
  getRawRecordRange: vi.fn(),
  getTraceRecords: vi.fn(),
}));

const getPayloadRangeMock = vi.mocked(getPayloadRange);
const getRawRecordRangeMock = vi.mocked(getRawRecordRange);

function record(sequence: number, type: string, payloadId = ""): TraceRecord {
  return {
    sequence,
    type,
    frameId: "model-frame",
    parentFrameId: "",
    frameType: "MODEL_CALL",
    route: "skill#mission-model",
    threadName: "worker",
    timestampMillis: sequence,
    representation: "LOGICAL",
    isChunk: false,
    isEnvelope: payloadId !== "",
    payloadId,
  };
}

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

function renderRecords(records: TraceRecord[]) {
  render(<TraceRecords traceId="trace-1" records={records} failures={[]} onSelectRecord={vi.fn()} onSelectFailure={vi.fn()} onPayload={vi.fn()} />);
}

beforeEach(() => {
  getPayloadRangeMock.mockReset();
  getRawRecordRangeMock.mockReset();
});

test("reconstructs a chunked model request and presents messages by role", async () => {
  const request = JSON.stringify({
    messages: [
      { messageType: "SYSTEM", text: "Follow the system instructions." },
      { messageType: "USER", text: "Resolve this support case." },
    ],
  });
  const split = 45;
  getPayloadRangeMock
    .mockResolvedValueOnce(range(request.slice(0, split), { actualEnd: split, totalLength: request.length, hasMore: true, nextCursor: "next" }))
    .mockResolvedValueOnce(range(request.slice(split), { actualStart: split, actualEnd: request.length, totalLength: request.length }));
  renderRecords([record(9, "MODEL_REQUEST_SENT", "payload-1")]);

  expect(screen.queryByRole("button", { name: "Read payload" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Request" }));

  const detail = await screen.findByRole("region", { name: "Model request for record 9" });
  expect(within(detail).getByRole("heading", { name: "system" })).toBeVisible();
  expect(detail).toHaveTextContent("Follow the system instructions.");
  expect(within(detail).getByRole("heading", { name: "user" })).toBeVisible();
  expect(detail).toHaveTextContent("Resolve this support case.");
  expect(getPayloadRangeMock).toHaveBeenNthCalledWith(1, "trace-1", "payload-1", undefined, "TARGET");
  expect(getPayloadRangeMock).toHaveBeenNthCalledWith(2, "trace-1", "payload-1", "next", "TARGET");
  expect(getRawRecordRangeMock).not.toHaveBeenCalled();
});

test("presents a prepared request inline instead of exposing the generic payload action", async () => {
  const request = JSON.stringify({
    messages: [{ messageType: "USER", text: "Prepared model input" }],
  });
  getPayloadRangeMock.mockResolvedValue(range(request));
  renderRecords([record(8, "MODEL_REQUEST_PREPARED", "prepared-payload")]);

  expect(screen.queryByRole("button", { name: "Read payload" })).toBeNull();
  expect(screen.queryByRole("region", { name: "Prepared model request for record 8" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Prepared request" }));

  const detail = await screen.findByRole("region", { name: "Prepared model request for record 8" });
  expect(within(detail).getByRole("heading", { name: "user" })).toBeVisible();
  expect(detail).toHaveTextContent("Prepared model input");
  expect(getPayloadRangeMock).toHaveBeenCalledWith("trace-1", "prepared-payload", undefined, "TARGET");
});

test("extracts and pretty prints JSON content from an inline model response", async () => {
  const content = { stepAction: "CALL_TOOL", toolName: "understandIntent", toolArguments: { customerId: "CUST-1001" } };
  getRawRecordRangeMock.mockResolvedValue(range(JSON.stringify({ traceId: "trace-1", recordType: "MODEL_RESPONSE_RECEIVED", data: { content: `\n${JSON.stringify(content)}` } })));
  renderRecords([record(12, "MODEL_RESPONSE_RECEIVED")]);

  fireEvent.click(screen.getByRole("button", { name: "Response" }));

  const detail = await screen.findByRole("region", { name: "Model response for record 12" });
  const output = within(detail).getByText((_, element) => element?.tagName === "PRE");
  expect(output).toHaveTextContent(JSON.stringify(content, null, 2), { normalizeWhitespace: false });
  expect(detail).not.toHaveTextContent("traceId");
});

test("preserves a plain-text model response", async () => {
  getRawRecordRangeMock.mockResolvedValue(range(JSON.stringify({ data: { content: "Plain model response" } })));
  renderRecords([record(13, "MODEL_RESPONSE_RECEIVED")]);

  fireEvent.click(screen.getByRole("button", { name: "Response" }));

  expect(await screen.findByText("Plain model response")).toBeVisible();
});

test("reports malformed model content without displaying a partial fallback", async () => {
  getRawRecordRangeMock.mockResolvedValue(range("{not-json"));
  renderRecords([record(14, "MODEL_RESPONSE_RECEIVED")]);

  fireEvent.click(screen.getByRole("button", { name: "Response" }));

  expect(await screen.findByRole("alert")).toHaveTextContent("Response could not be displayed");
  expect(screen.queryByText("{not-json")).toBeNull();
});
