import { useEffect, useRef } from "react";
import type { TraceRange } from "../api/contracts";

export function TraceEvidenceDetail({ range, pending = false, error, label = "Evidence content", onNext, onClear }: { range?: TraceRange; pending?: boolean; error?: string; label?: string; onNext: () => void; onClear: () => void }) {
  const detail = useRef<HTMLElement>(null);
  const visible = Boolean(range || pending || error);

  useEffect(() => {
    if (visible) detail.current?.focus();
  }, [error, pending, range, visible]);

  if (!visible) return null;
  return <section ref={detail} aria-label={label} aria-busy={pending} tabIndex={-1}>
    {pending && !range && <p role="status">Reading content…</p>}
    {error && <p className="target-error" role="alert">{error}</p>}
    {range && <>
      <p>{range.encoding === "BASE64" ? "Base64-encoded" : "Text"} bytes {range.actualStart}–{range.actualEnd} of {range.totalLength} ({range.contentType})</p>
      <pre>{range.content}</pre>
      {range.hasMore && range.nextCursor && <button type="button" disabled={pending} onClick={onNext}>{pending ? "Reading…" : "Read next range"}</button>}
    </>}
    <button type="button" onClick={onClear}>Clear content</button>
  </section>;
}
