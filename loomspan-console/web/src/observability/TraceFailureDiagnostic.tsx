import { useEffect, useRef, useState } from "react";
import { getTraceFailureDiagnostic } from "../api/client";
import type { TraceFailure, TraceFailureDiagnostic as Diagnostic } from "../api/contracts";

export function TraceFailureDiagnostic({ traceId, failure, scopeGeneration = 0, verifyScope }: {
  traceId: string;
  failure?: TraceFailure;
  scopeGeneration?: number;
  verifyScope?: (response: Diagnostic) => Promise<Diagnostic>;
}) {
  const [selected, setSelected] = useState<number>();
  const [loaded, setLoaded] = useState<{ key: string; diagnostic: Diagnostic }>();
  const [wrap, setWrap] = useState(true);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string>();
  const [status, setStatus] = useState<string>();
  const requestGeneration = useRef(0);
  useEffect(() => {
    requestGeneration.current += 1;
    setSelected(undefined);
    setLoaded(undefined);
    setLoading(false);
    setError(undefined);
    setStatus(undefined);
  }, [failure?.failureId, scopeGeneration, traceId]);
  if (!failure) return null;
  const selectionKey = `${traceId}\u0000${failure.failureId}\u0000${scopeGeneration}`;
  const activeLoaded = loaded?.key === selectionKey ? loaded.diagnostic : undefined;
  const diagnostics = failure.diagnostics ?? [];
  const load = async (ordinal: number) => {
    const generation = ++requestGeneration.current;
    const expectedFailureId = failure.failureId;
    const expectedKey = selectionKey;
    setSelected(ordinal);
    setLoaded(undefined);
    setError(undefined);
    setStatus(undefined);
    setLoading(true);
    try {
      const response = await getTraceFailureDiagnostic(traceId, expectedFailureId, ordinal);
      if (generation !== requestGeneration.current) return;
      const verified = verifyScope ? await verifyScope(response) : response;
      if (generation !== requestGeneration.current) return;
      if (verified.failureId !== expectedFailureId || verified.descriptor.ordinal !== ordinal) {
        throw new Error("The diagnostic response did not match the selected failure.");
      }
      setLoaded({ key: expectedKey, diagnostic: verified });
    } catch (value) {
      if (generation !== requestGeneration.current) return;
      setError(value instanceof Error ? value.message : "Diagnostic could not be loaded.");
    } finally {
      if (generation === requestGeneration.current) setLoading(false);
    }
  };
  const download = () => {
    if (!activeLoaded) return;
    let url: string | undefined;
    try {
      setError(undefined);
      url = URL.createObjectURL(new Blob([activeLoaded.text], { type: "text/plain;charset=utf-8" }));
      const anchor = document.createElement("a");
      anchor.href = url;
      anchor.download = `${failure.failureId}-${activeLoaded.descriptor.ordinal}.txt`;
      anchor.click();
      setStatus("Diagnostic download started.");
    } catch {
      setStatus(undefined);
      setError("Diagnostic could not be downloaded.");
    } finally {
      if (url) URL.revokeObjectURL(url);
    }
  };
  const copy = async () => {
    if (!activeLoaded) return;
    try {
      setError(undefined);
      await navigator.clipboard.writeText(activeLoaded.text);
      setStatus("Diagnostic copied to the clipboard.");
    } catch {
      setStatus(undefined);
      setError("Diagnostic could not be copied.");
    }
  };
  return <section aria-label="Failure diagnostics">
    <h4>{failure.terminal ? "Terminal failure" : "Recovered failure"}</h4>
    {failure.exceptionType && <p><code>{failure.exceptionType}</code>{failure.contextSummary && ` - ${failure.contextSummary}`}</p>}
    {diagnostics.length === 0 ? <p>No diagnostic text was recorded.</p> : <ul>{diagnostics.map((descriptor) => <li key={descriptor.ordinal}>
      <span>{descriptor.kind === "JAVA_STACK_TRACE" ? "Java stack trace" : `Text diagnostic (${descriptor.kind})`} / {descriptor.contentType} / {descriptor.decodedBytes} bytes</span>
      {descriptor.truncated && <strong> / truncated</strong>}
      {" "}<button type="button" disabled={loading} onClick={() => void load(descriptor.ordinal)}>{loading && selected === descriptor.ordinal ? "Loading diagnostic..." : `Load diagnostic ${descriptor.ordinal + 1}`}</button>
    </li>)}</ul>}
    {error && <p role="alert">{error}</p>}
    {status && <p role="status">{status}</p>}
    {activeLoaded && selected === activeLoaded.descriptor.ordinal && <div>
      {activeLoaded.descriptor.truncated && <p role="status">This diagnostic was truncated during capture at {activeLoaded.descriptor.captureLimitBytes} bytes.</p>}
      <button type="button" onClick={() => setWrap((value) => !value)}>{wrap ? "Disable wrapping" : "Enable wrapping"}</button>
      <button type="button" onClick={() => void copy()}>Copy diagnostic</button>
      <button type="button" onClick={download}>Download diagnostic</button>
      <pre style={{ whiteSpace: wrap ? "pre-wrap" : "pre", overflowX: "auto" }}>{activeLoaded.text}</pre>
    </div>}
  </section>;
}
