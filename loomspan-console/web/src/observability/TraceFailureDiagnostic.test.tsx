import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, expect, test, vi } from "vitest";
import * as api from "../api/client";
import { TraceFailureDiagnostic } from "./TraceFailureDiagnostic";

vi.mock("../api/client", () => ({ getTraceFailureDiagnostic: vi.fn() }));
const failure = { failureId: "failure-1", terminal: true, sequence: 1, timestampMillis: 1, recordType: "ERROR_RECORDED", frameId: "frame-1", route: "model", attemptId: "", retrySequenceId: "", validationStatus: "", exceptionType: "java.lang.IllegalStateException", contextSummary: "failed", diagnostics: [{ ordinal: 0, kind: "JAVA_STACK_TRACE", contentType: "text/plain; charset=utf-8", truncated: true, captureLimitBytes: 1048576, decodedBytes: 20 }] };

beforeEach(() => {
  vi.mocked(api.getTraceFailureDiagnostic).mockResolvedValue({ source: "TARGET", targetScopeId: "scope-1", failureId: "failure-1", descriptor: failure.diagnostics[0], text: "line one\n<script>x</script>" });
  Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText: vi.fn().mockResolvedValue(undefined) } });
  URL.createObjectURL = vi.fn(() => "blob:test"); URL.revokeObjectURL = vi.fn();
  vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => undefined);
});

test("loads a selected diagnostic deliberately and keeps text inert", async () => {
  render(<TraceFailureDiagnostic traceId="trace-1" failure={failure} />);
  expect(api.getTraceFailureDiagnostic).not.toHaveBeenCalled();
  const loadButton = screen.getByRole("button", { name: "Load diagnostic 1" });
  expect(loadButton).toHaveClass("failure-diagnostic-action");
  fireEvent.click(loadButton);
  expect(await screen.findByText(/<script>x<\/script>/)).toBeInTheDocument();
  expect(document.querySelector("script")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Disable wrapping" }));
  fireEvent.click(screen.getByRole("button", { name: "Copy diagnostic" }));
  await waitFor(() => expect(navigator.clipboard.writeText).toHaveBeenCalledWith("line one\n<script>x</script>"));
  fireEvent.click(screen.getByRole("button", { name: "Download diagnostic" }));
  expect(URL.createObjectURL).toHaveBeenCalled();
  expect(screen.getByText(/truncated during capture/)).toBeInTheDocument();
});

test("reports copy and download failures accessibly", async () => {
  vi.mocked(navigator.clipboard.writeText).mockRejectedValueOnce(new Error("denied"));
  render(<TraceFailureDiagnostic traceId="trace-1" failure={failure} />);
  fireEvent.click(screen.getByRole("button", { name: "Load diagnostic 1" }));
  await screen.findByText(/<script>x<\/script>/);

  fireEvent.click(screen.getByRole("button", { name: "Copy diagnostic" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("could not be copied");

  vi.mocked(URL.createObjectURL).mockImplementationOnce(() => { throw new Error("blocked"); });
  fireEvent.click(screen.getByRole("button", { name: "Download diagnostic" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("could not be downloaded");
});

test("does not render loaded text after failure ownership changes", async () => {
  const { rerender } = render(<TraceFailureDiagnostic traceId="trace-1" failure={failure} scopeGeneration={1} />);
  fireEvent.click(screen.getByRole("button", { name: "Load diagnostic 1" }));
  expect(await screen.findByText(/<script>x<\/script>/)).toBeInTheDocument();

  rerender(<TraceFailureDiagnostic traceId="trace-1" failure={{ ...failure, failureId: "failure-2" }} scopeGeneration={1} />);
  expect(screen.queryByText(/<script>x<\/script>/)).toBeNull();
});

test("labels unknown diagnostics generically and handles load errors", async () => {
  vi.mocked(api.getTraceFailureDiagnostic).mockRejectedValueOnce(new Error("load failed"));
  render(<TraceFailureDiagnostic traceId="trace-1" failure={{ ...failure, terminal: false, diagnostics: [{ ...failure.diagnostics[0], kind: "FUTURE_KIND", truncated: false }] }} />);
  expect(screen.getByText(/Text diagnostic \(FUTURE_KIND\)/)).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Load diagnostic 1" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("load failed");
});

test("shows a bounded deliberate loading state", async () => {
  let resolve!: (value: Awaited<ReturnType<typeof api.getTraceFailureDiagnostic>>) => void;
  vi.mocked(api.getTraceFailureDiagnostic).mockReturnValueOnce(new Promise((done) => { resolve = done; }));
  render(<TraceFailureDiagnostic traceId="trace-1" failure={failure} />);
  fireEvent.click(screen.getByRole("button", { name: "Load diagnostic 1" }));
  expect(screen.getByRole("button", { name: "Loading diagnostic..." })).toBeDisabled();
  resolve({ source: "TARGET", targetScopeId: "scope-1", failureId: "failure-1", descriptor: failure.diagnostics[0], text: "stack" });
  expect(await screen.findByText("stack")).toBeInTheDocument();
});

test("discards stale responses after the selected failure changes", async () => {
  let resolveOld!: (value: Awaited<ReturnType<typeof api.getTraceFailureDiagnostic>>) => void;
  vi.mocked(api.getTraceFailureDiagnostic)
    .mockReturnValueOnce(new Promise((done) => { resolveOld = done; }))
    .mockResolvedValueOnce({
      source: "TARGET",
      targetScopeId: "scope-1",
      failureId: "failure-2",
      descriptor: failure.diagnostics[0],
      text: "new diagnostic",
    });
  const { rerender } = render(<TraceFailureDiagnostic traceId="trace-1" failure={failure} scopeGeneration={1} />);
  fireEvent.click(screen.getByRole("button", { name: "Load diagnostic 1" }));

  const nextFailure = { ...failure, failureId: "failure-2" };
  rerender(<TraceFailureDiagnostic traceId="trace-1" failure={nextFailure} scopeGeneration={1} />);
  fireEvent.click(screen.getByRole("button", { name: "Load diagnostic 1" }));
  expect(await screen.findByText("new diagnostic")).toBeInTheDocument();

  resolveOld({ source: "TARGET", targetScopeId: "scope-1", failureId: "failure-1", descriptor: failure.diagnostics[0], text: "old diagnostic" });
  await waitFor(() => expect(screen.queryByText("old diagnostic")).toBeNull());
  expect(screen.getByText("new diagnostic")).toBeInTheDocument();
});

test("discards a response from an earlier scope before scope verification", async () => {
  let resolveOld!: (value: Awaited<ReturnType<typeof api.getTraceFailureDiagnostic>>) => void;
  vi.mocked(api.getTraceFailureDiagnostic).mockReturnValueOnce(new Promise((done) => { resolveOld = done; }));
  const verifyScope = vi.fn(async (response: Awaited<ReturnType<typeof api.getTraceFailureDiagnostic>>) => response);
  const { rerender } = render(
    <TraceFailureDiagnostic traceId="trace-1" failure={failure} scopeGeneration={1} verifyScope={verifyScope} />,
  );
  fireEvent.click(screen.getByRole("button", { name: "Load diagnostic 1" }));

  rerender(<TraceFailureDiagnostic traceId="trace-1" failure={failure} scopeGeneration={2} verifyScope={verifyScope} />);
  await act(async () => {
    resolveOld({ source: "TARGET", targetScopeId: "scope-1", failureId: "failure-1", descriptor: failure.diagnostics[0], text: "old scope" });
  });

  expect(screen.queryByText("old scope")).toBeNull();
  expect(verifyScope).not.toHaveBeenCalled();
});

test("keeps imported diagnostic state across unrelated target generations", async () => {
	vi.mocked(api.getTraceFailureDiagnostic).mockResolvedValueOnce({ source: "IMPORTED", failureId: "failure-1", descriptor: failure.diagnostics[0], text: "imported diagnostic" });
	const { rerender } = render(<TraceFailureDiagnostic traceId="trace-1" source="IMPORTED" failure={failure} scopeGeneration={1} />);
	fireEvent.click(screen.getByRole("button", { name: "Load diagnostic 1" }));
	expect(await screen.findByText("imported diagnostic")).toBeInTheDocument();

	rerender(<TraceFailureDiagnostic traceId="trace-1" source="IMPORTED" failure={failure} scopeGeneration={2} />);
	expect(screen.getByText("imported diagnostic")).toBeInTheDocument();
});
