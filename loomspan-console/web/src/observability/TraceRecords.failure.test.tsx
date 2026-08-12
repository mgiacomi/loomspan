import { fireEvent, render, screen } from "@testing-library/react";
import { expect, test, vi } from "vitest";
import type { TraceFailure, TraceRecord } from "../api/contracts";
import { TraceRecords } from "./TraceRecords";

const failure: TraceFailure = {
  failureId: "failure-15",
  terminal: true,
  sequence: 15,
  timestampMillis: 15,
  recordType: "ERROR_RECORDED",
  frameId: "frame-1",
  route: "planning-model",
  attemptId: "attempt-3",
  retrySequenceId: "retry-1",
  validationStatus: "",
};

function record(sequence: number, type: string): TraceRecord {
  return {
    sequence,
    type,
    frameId: "frame-1",
    parentFrameId: "",
    frameType: "MODEL_CALL",
    route: "planning-model",
    threadName: "worker",
    timestampMillis: sequence,
    representation: "LOGICAL",
    isChunk: false,
    isEnvelope: true,
    payloadId: "",
  };
}

test("highlights the exact failure record and provides a view-error action", () => {
  const selectFailure = vi.fn();
  render(<TraceRecords records={[record(14, "MODEL_ATTEMPT_FAILED"), record(15, "ERROR_RECORDED")]} attempts={[]} retries={[]} failures={[failure]} validations={[]} gaps={[]} uncertainties={[]} payloads={[]} selectedFailureId="failure-15" onSelectRecord={vi.fn()} onSelectFailure={selectFailure} onRaw={vi.fn()} onPayload={vi.fn()} />);

  const errorRow = screen.getByRole("button", { name: "15: ERROR_RECORDED" }).closest("tr");
  const attemptRow = screen.getByRole("button", { name: "14: MODEL_ATTEMPT_FAILED" }).closest("tr");
  expect(errorRow).toHaveClass("trace-record-error");
  expect(attemptRow).not.toHaveClass("trace-record-error");

  const action = screen.getByRole("button", { name: "View error" });
  expect(action).toHaveAttribute("aria-pressed", "true");
  fireEvent.click(action);
  expect(selectFailure).toHaveBeenCalledWith("failure-15");
});
