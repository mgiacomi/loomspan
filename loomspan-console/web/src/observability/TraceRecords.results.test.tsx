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

function record(sequence: number, type: string, route: string, frameId = "frame", parentFrameId = "parent"): TraceRecord {
  return {
    sequence, type, route, frameId, parentFrameId,
    frameType: type === "TOOL_CALL_COMPLETED" ? "TOOL_INVOCATION" : "STEP_EXECUTION",
    threadName: "worker", timestampMillis: sequence, representation: "LOGICAL",
    isChunk: false, isEnvelope: false, payloadId: "",
  };
}

function range(value: unknown): TraceRange {
  const content = JSON.stringify(value);
  return { targetScopeId: "scope-1", actualStart: 0, actualEnd: content.length, totalLength: content.length, contentType: "application/json", encoding: "TEXT", content, hasMore: false, nextCursor: null };
}

function page(items: TraceRecord[]): TraceAnalysisPage<TraceRecord> {
  return { targetScopeId: "scope-1", items, hasMore: false, nextCursor: null };
}

function renderRecord(current: TraceRecord, onSelectRecord = vi.fn(), onSelectFailure = vi.fn()) {
  render(<TraceRecords traceId="trace-1" records={[current]} failures={[]} onSelectRecord={onSelectRecord} onSelectFailure={onSelectFailure} onPayload={vi.fn()} />);
  return onSelectRecord;
}

beforeEach(() => {
  getRawRecordRangeMock.mockReset();
  getTraceRecordsMock.mockReset();
});

test("shows and pretty formats the complete owned tool result", async () => {
  getRawRecordRangeMock.mockImplementation(async (_traceId, sequence) => sequence === 100
    ? range({ metadata: { capabilityName: "lookupCustomer", linkedTaskId: "task-customer" }, data: { eventId: "event-1", capabilityName: "lookupCustomer", linkedTaskId: "task-customer", details: { result: "{\"customerId\":\"CUST-1001\",\"active\":true}" }, note: "Cached lookup" } })
    : range({ data: { planId: "plan-1", capabilityName: "handleBilling", tasks: [{ taskId: "task-customer", title: "Retrieve customer" }] } }));
  getTraceRecordsMock.mockImplementation(async (_traceId, _cursor, filter) => filter?.types?.includes("STEP_STARTED")
    ? page([record(88, "STEP_STARTED", "handleBilling#step-1", "step-frame", "skill-frame")])
    : page([record(85, "PLAN_CREATED", "handleBilling", "plan-frame", "skill-frame")]));
  renderRecord(record(100, "TOOL_CALL_COMPLETED", "lookupCustomer", "tool-frame", "step-frame"));

  fireEvent.click(screen.getByRole("button", { name: "Tool result" }));
  const detail = await screen.findByRole("region", { name: "Tool result for record 100" });
  expect(detail).toHaveTextContent("Retrieve customer");
  expect(detail).toHaveTextContent("Cached lookup");
  expect(within(detail).getByText(/"customerId": "CUST-1001"/)).toBeVisible();
  expect(within(detail).getByText(/"active": true/)).toBeVisible();
});

test("shows structured-output retries and path-oriented issues", async () => {
  getRawRecordRangeMock.mockResolvedValue(range({
    metadata: { skillName: "checkRefundPolicy", status: "RETRYING" },
    data: {
      skillName: "checkRefundPolicy", status: "RETRYING", attempt: 1, retryCount: 0, maxRetries: 2,
      failureMode: "SCHEMA_VALIDATION_FAILED",
      issues: [{ path: "$.refundAmount", message: "should be a number", canonicalField: "refundAmount" }],
    },
  }));
  renderRecord(record(175, "STRUCTURED_OUTPUT_RECORDED", "checkRefundPolicy#mission-model", "model-frame", "mission-frame"));

  fireEvent.click(screen.getByRole("button", { name: "Validation details" }));
  const detail = await screen.findByRole("region", { name: "Validation details for record 175" });
  expect(detail).toHaveTextContent("Retrying");
  expect(detail).toHaveTextContent("0 of 2");
  expect(detail).toHaveTextContent("Schema validation failed");
  expect(detail).toHaveTextContent("$.refundAmount");
  expect(detail).toHaveTextContent("should be a number");
});

test("summarizes a tool step and links to its authoritative full result", async () => {
  const toolResult = record(100, "TOOL_CALL_COMPLETED", "lookupCustomer", "tool-frame", "step-frame");
  getRawRecordRangeMock.mockImplementation(async (_traceId, sequence) => sequence === 104
    ? range({ metadata: { taskId: "task-customer", stepNumber: 1, toolName: "lookupCustomer", stepAction: "CALL_TOOL" }, data: { resultPreview: "{\"customerId\":\"CUST-1001\"..." } })
    : range({ data: { planId: "plan-1", capabilityName: "handleBilling", tasks: [{ taskId: "task-customer", title: "Retrieve customer" }] } }));
  getTraceRecordsMock.mockImplementation(async (_traceId, _cursor, filter) => filter?.types?.includes("TOOL_CALL_COMPLETED")
    ? page([toolResult])
    : page([record(85, "PLAN_CREATED", "handleBilling", "plan-frame", "skill-frame")]));
  const onSelectRecord = renderRecord(record(104, "STEP_COMPLETED", "handleBilling#step-1", "step-frame", "skill-frame"));

  fireEvent.click(screen.getByRole("button", { name: "Step result" }));
  const detail = await screen.findByRole("region", { name: "Step result for record 104" });
  expect(detail).toHaveTextContent("Retrieve customer");
  expect(detail).toHaveTextContent("The preview may be truncated");
  fireEvent.click(within(detail).getByRole("button", { name: "View full tool result" }));
  expect(onSelectRecord).toHaveBeenCalledWith(toolResult);
});

test("describes a final-response completion without inventing its body", async () => {
  const response = record(193, "MODEL_RESPONSE_RECEIVED", "handleBilling#step-5-model", "model-frame", "step-frame");
  getRawRecordRangeMock.mockResolvedValue(range({ metadata: { stepAction: "FINAL_RESPONSE", stepNumber: 5 }, data: {} }));
  getTraceRecordsMock.mockResolvedValue(page([response]));
  renderRecord(record(198, "STEP_COMPLETED", "handleBilling#step-5", "step-frame", "skill-frame"));

  fireEvent.click(screen.getByRole("button", { name: "Step result" }));
  const detail = await screen.findByRole("region", { name: "Step result for record 198" });
  expect(detail).toHaveTextContent("does not contain the response body");
  expect(within(detail).getByRole("button", { name: "View model response record" })).toBeVisible();
});

test("explains a recorded evidence source and links to its authoritative tool result", async () => {
  const sourceResult = record(178, "TOOL_CALL_COMPLETED", "checkRefundPolicy", "tool-frame", "step-frame");
  getRawRecordRangeMock.mockImplementation(async (_traceId, sequence) => sequence === 181
    ? range({
      metadata: { capabilityName: "checkRefundPolicy", linkedTaskId: "task-check-refund-policy", unplanned: false },
      data: {
        successfulSkill: "checkRefundPolicy",
        successfulDirectSkills: ["lookupCustomer", "lookupInvoices", "lookupRefundPolicy", "checkRefundPolicy"],
      },
    })
    : range({ data: { planId: "plan-1", capabilityName: "handleBilling", tasks: [{ taskId: "task-check-refund-policy", title: "Evaluate Refund Eligibility" }] } }));
  getTraceRecordsMock.mockImplementation(async (_traceId, _cursor, filter) => filter?.types?.includes("TOOL_CALL_COMPLETED")
    ? page([sourceResult])
    : page([record(85, "PLAN_CREATED", "handleBilling", "plan-frame", "skill-frame")]));
  const onSelectRecord = renderRecord(record(181, "EVIDENCE_RECORDED", "handleBilling#step-4", "step-frame", "skill-frame"));

  fireEvent.click(screen.getByRole("button", { name: "Evidence details" }));
  const detail = await screen.findByRole("region", { name: "Evidence details for record 181" });
  expect(detail).toHaveTextContent("checkRefundPolicy");
  expect(detail).toHaveTextContent("Evaluate Refund Eligibility");
  expect(detail).toHaveTextContent("Planned");
  expect(detail).toHaveTextContent("Available evidence sources after this record");
  expect(detail).toHaveTextContent("lookupCustomer");
  expect(detail).toHaveTextContent("does not determine whether a particular final-response claim is supported");
  fireEvent.click(within(detail).getByRole("button", { name: "View source result" }));
  expect(onSelectRecord).toHaveBeenCalledWith(sourceResult);
});

test("shows an unplanned evidence source without inventing a task or result", async () => {
  getRawRecordRangeMock.mockResolvedValue(range({
    metadata: { capabilityName: "lookupPolicy", unplanned: true },
    data: { successfulSkill: "lookupPolicy", successfulDirectSkills: ["lookupPolicy"] },
  }));
  getTraceRecordsMock.mockResolvedValue(page([]));
  renderRecord(record(25, "EVIDENCE_RECORDED", "rootSkill", "root-frame", ""));

  fireEvent.click(screen.getByRole("button", { name: "Evidence details" }));
  const detail = await screen.findByRole("region", { name: "Evidence details for record 25" });
  expect(detail).toHaveTextContent("Unplanned");
  expect(within(detail).queryByText("Task ID")).toBeNull();
  expect(within(detail).queryByRole("button", { name: "View source result" })).toBeNull();
});

test("shows the authoritative successful completion receipt", async () => {
  getRawRecordRangeMock.mockResolvedValue(range({
    metadata: {
      remainingFrames: 0,
      skillName: "resolveSupportCase",
      objective: "Execute YAML skill 'resolveSupportCase' using the provided mission input object.",
      outcome: "SUCCEEDED",
      sessionUsageSnapshot: {
        skillInvocations: 5, toolInvocations: 7, linterRetries: 0, modelCalls: 15, providerAttempts: 15,
        promptUnits: 15760, completionUnits: 25456, totalUnits: 41216,
        exactModelResponses: 15, heuristicModelResponses: 0, unavailableModelResponses: 0,
      },
      errored: false,
      persistencePolicy: "ALWAYS",
    },
    data: null,
  }));
  renderRecord(record(257, "TRACE_COMPLETED", "", "", ""));

  fireEvent.click(screen.getByRole("button", { name: "Completion details" }));
  const detail = await screen.findByRole("region", { name: "Completion details for record 257" });
  expect(detail).toHaveTextContent("Succeeded");
  expect(detail).toHaveTextContent("resolveSupportCase");
  expect(detail).toHaveTextContent("Remaining open frames");
  expect(within(detail).getByRole("table", { name: "Final execution counters" })).toHaveTextContent("15");
  expect(within(detail).getByRole("table", { name: "Final usage" })).toHaveTextContent("41,216");
  expect(within(detail).getByRole("table", { name: "Usage precision" })).toHaveTextContent("Exact responses15");
  expect(detail).toHaveTextContent("does not make a broader claim about every cleanup operation");
  expect(detail).not.toHaveTextContent("one or more failure");
});

test("highlights failed completion and opens the terminal error", async () => {
  const onSelectFailure = vi.fn();
  getRawRecordRangeMock.mockResolvedValue(range({
    metadata: {
      remainingFrames: 0,
      skillName: "resolveSupportCase",
      outcome: "FAILED",
      terminalFailureId: "failure-1",
      sessionUsageSnapshot: {
        skillInvocations: 1, toolInvocations: 0, linterRetries: 0, modelCalls: 1, providerAttempts: 1,
        promptUnits: 10, completionUnits: 0, totalUnits: 10,
        exactModelResponses: 0, heuristicModelResponses: 0, unavailableModelResponses: 1,
      },
      errored: true,
      persistencePolicy: "ONERROR",
    },
    data: null,
  }));
  renderRecord(record(21, "TRACE_COMPLETED", "", "", ""), vi.fn(), onSelectFailure);

  fireEvent.click(screen.getByRole("button", { name: "Completion details" }));
  const detail = await screen.findByRole("region", { name: "Completion details for record 21" });
  expect(detail).toHaveTextContent("Failed");
  expect(detail).toHaveTextContent("failure-1");
  expect(detail).toHaveTextContent("one or more failure");
  fireEvent.click(within(detail).getByRole("button", { name: "View terminal error" }));
  expect(onSelectFailure).toHaveBeenCalledWith("failure-1");
});

test("reports invalid completion totals instead of presenting a misleading receipt", async () => {
  getRawRecordRangeMock.mockResolvedValue(range({
    metadata: {
      remainingFrames: 0, outcome: "SUCCEEDED", errored: false, persistencePolicy: "ALWAYS",
      sessionUsageSnapshot: {
        skillInvocations: 1, toolInvocations: 0, linterRetries: 0, modelCalls: 1, providerAttempts: 1,
        promptUnits: 10, completionUnits: 5, totalUnits: 99,
        exactModelResponses: 1, heuristicModelResponses: 0, unavailableModelResponses: 0,
      },
    },
    data: null,
  }));
  renderRecord(record(21, "TRACE_COMPLETED", "", "", ""));

  fireEvent.click(screen.getByRole("button", { name: "Completion details" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("terminal usage totals did not reconcile");
});
