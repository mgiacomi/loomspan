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

function record(type = "FRAME_OPENED"): TraceRecord {
  return {
    sequence: 7,
    type,
    frameId: "frame-1",
    parentFrameId: "",
    frameType: "MODEL_CALL",
    route: "skill#model",
    threadName: "worker",
    timestampMillis: 7,
    representation: "LOGICAL",
    isChunk: false,
    isEnvelope: false,
    payloadId: "",
  };
}

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

function renderRecord(value: TraceRecord) {
  render(<TraceRecords traceId="trace-1" records={[value]} attempts={[]} retries={[]} failures={[]} validations={[]} gaps={[]} uncertainties={[]} payloads={[]} onSelectRecord={vi.fn()} onSelectFailure={vi.fn()} onPayload={vi.fn()} />);
}

beforeEach(() => getRawRecordRangeMock.mockReset());

test("loads all ranges and pretty prints the complete raw record envelope", async () => {
  const rawRecord = JSON.stringify({ traceId: "trace-1", sequence: 7, recordType: "FRAME_OPENED", metadata: { route: "skill#model" }, data: null });
  const split = 38;
  getRawRecordRangeMock
    .mockResolvedValueOnce(range(rawRecord.slice(0, split), { actualEnd: split, totalLength: rawRecord.length, hasMore: true, nextCursor: "next" }))
    .mockResolvedValueOnce(range(rawRecord.slice(split), { actualStart: split, actualEnd: rawRecord.length, totalLength: rawRecord.length }));
  renderRecord(record());

  fireEvent.click(screen.getByRole("button", { name: "Read raw record" }));

  const detail = await screen.findByRole("region", { name: "Raw record 7" });
  const output = within(detail).getByText((_, element) => element?.tagName === "PRE");
  expect(output).toHaveTextContent(JSON.stringify(JSON.parse(rawRecord), null, 2), { normalizeWhitespace: false });
  expect(getRawRecordRangeMock).toHaveBeenNthCalledWith(1, "trace-1", 7, undefined);
  expect(getRawRecordRangeMock).toHaveBeenNthCalledWith(2, "trace-1", 7, "next");
});

test("switches from the response view to the raw envelope on the same row", async () => {
  const rawRecord = JSON.stringify({ traceId: "trace-1", sequence: 7, recordType: "MODEL_RESPONSE_RECEIVED", data: { content: "model output" } });
  getRawRecordRangeMock.mockResolvedValue(range(rawRecord));
  renderRecord(record("MODEL_RESPONSE_RECEIVED"));

  fireEvent.click(screen.getByRole("button", { name: "Response" }));
  expect(await screen.findByRole("region", { name: "Model response for record 7" })).toHaveTextContent("model output");
  fireEvent.click(screen.getByRole("button", { name: "Read raw record" }));

  const raw = await screen.findByRole("region", { name: "Raw record 7" });
  expect(raw).toHaveTextContent("traceId");
  expect(screen.queryByRole("region", { name: "Model response for record 7" })).toBeNull();
});

test("reports malformed raw JSON without displaying partial content", async () => {
  getRawRecordRangeMock.mockResolvedValue(range("{not-json"));
  renderRecord(record());

  fireEvent.click(screen.getByRole("button", { name: "Read raw record" }));

  expect(await screen.findByRole("alert")).toHaveTextContent("Raw record could not be displayed");
  expect(screen.queryByText("{not-json")).toBeNull();
});
