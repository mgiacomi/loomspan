import { Fragment, useState } from "react";
import { getRawRecordRange } from "../api/client";
import type { TraceRange } from "../api/contracts";
import type { TraceAttempt, TraceFailure, TraceGap, TracePayload, TraceRecord, TraceRetry, TraceUncertainty, TraceValidation } from "../api/contracts";
import type { TraceFrameFilter } from "../api/client";

type Props = { traceId?: string; records: TraceRecord[]; attempts: TraceAttempt[]; retries: TraceRetry[]; failures: TraceFailure[]; validations: TraceValidation[]; gaps: TraceGap[]; uncertainties: TraceUncertainty[]; payloads: TracePayload[]; selectedRecordSequence?: number; selectedFailureId?: string; onSelectRecord: (record: TraceRecord) => void; onSelectFailure: (failureId: string) => void; onRelatedFrame?: (filter: TraceFrameFilter) => void; onRaw: (record: TraceRecord) => void; onPayload: (payloadId: string) => void };

type PlanCacheEntry = { loading: boolean; error?: string; json?: string };

function decodeBytes(range: TraceRange): Uint8Array {
  if (range.encoding === "BASE64") {
    try {
      const binary = atob(range.content);
      return Uint8Array.from(binary, (character) => character.charCodeAt(0));
    } catch {
      throw new Error("Plan record contained invalid base64 data.");
    }
  }
  return new TextEncoder().encode(range.content);
}

function joinBytes(parts: Uint8Array[]): Uint8Array {
  const result = new Uint8Array(parts.reduce((length, part) => length + part.length, 0));
  let offset = 0;
  for (const part of parts) {
    result.set(part, offset);
    offset += part.length;
  }
  return result;
}

async function readCompleteRecord(traceId: string, sequence: number): Promise<string> {
  const parts: Uint8Array[] = [];
  let cursor: string | undefined;
  do {
    const range = await getRawRecordRange(traceId, sequence, cursor);
    parts.push(decodeBytes(range));
    if (!range.hasMore) break;
    if (!range.nextCursor || range.nextCursor === cursor) throw new Error("Plan record continuation was invalid.");
    cursor = range.nextCursor;
  } while (true);
  return new TextDecoder("utf-8", { fatal: true }).decode(joinBytes(parts)).trim();
}

function parsePlanJson(rawRecord: string): string {
  const record: unknown = JSON.parse(rawRecord);
  if (!record || typeof record !== "object" || Array.isArray(record)) {
    throw new Error("Plan record did not contain a JSON object.");
  }
  const envelope = record as Record<string, unknown>;
  let plan = "data" in envelope ? envelope.data : envelope.Data;
  if (typeof plan === "string") {
    plan = JSON.parse(plan);
  }
  if (plan === undefined || plan === null) {
    throw new Error("Plan record did not contain plan data.");
  }
  return JSON.stringify(plan, null, 2);
}

export function TraceRecords({ traceId, records, attempts, retries, failures, validations, gaps, uncertainties, payloads, selectedRecordSequence, selectedFailureId, onSelectRecord, onSelectFailure, onRelatedFrame, onRaw, onPayload }: Props) {
  const related = onRelatedFrame ?? (() => undefined);
  const [expanded, setExpanded] = useState<string | null>(null);
  const [cache, setCache] = useState<Record<string, PlanCacheEntry>>({});

  const handleTogglePlan = (record: TraceRecord) => {
    const seq = record.sequence;
    const key = `${traceId}:${seq}`;
    if (expanded === key) {
      setExpanded(null);
      return;
    }
    setExpanded(key);
    if (!traceId) return;
    const existing = cache[key];
    if (existing?.json || existing?.loading) return;
    setCache((prev) => ({ ...prev, [key]: { loading: true } }));
    void readCompleteRecord(traceId, seq)
      .then((rawRecord) => {
        const json = parsePlanJson(rawRecord);
        setCache((prev) => ({ ...prev, [key]: { loading: false, json } }));
      })
      .catch((err: unknown) => {
        const message = err instanceof Error ? `Plan could not be displayed: ${err.message}` : "Plan could not be displayed.";
        setCache((prev) => ({ ...prev, [key]: { loading: false, error: message } }));
      });
  };

  return <div aria-label="Trace records">
    <h4>Records</h4><div className="trace-table-region" role="region" aria-label="Record list" tabIndex={0}><table><thead><tr><th>Sequence</th><th>Type</th><th>Frame</th><th>Timestamp</th><th>Actions</th></tr></thead><tbody>{records.map((record) => {
      const isPlanCreated = record.type === "PLAN_CREATED";
      const key = `${traceId}:${record.sequence}`;
      const isExpanded = expanded === key;
      const entry = cache[key];
      const linkedFailure = failures.find((failure) => failure.sequence === record.sequence);
      return (
        <Fragment key={record.sequence}>
          <tr className={linkedFailure ? "trace-record-error" : undefined} aria-current={selectedRecordSequence === record.sequence ? "true" : undefined}>
            <td><button type="button" onClick={() => onSelectRecord(record)}>{record.sequence}: {record.type}</button></td>
            <td>{record.type}</td>
            <td>{record.frameId && <button type="button" onClick={() => related({ frameIds: [record.frameId] })}>{record.frameId}</button>}</td>
            <td>{record.timestampMillis}</td>
            <td>
              <button type="button" onClick={() => onRaw(record)}>Read raw record</button>
              {record.payloadId && <button type="button" onClick={() => onPayload(record.payloadId)}>Read payload</button>}
              {linkedFailure && <button className="trace-error-action" type="button" aria-pressed={selectedFailureId === linkedFailure.failureId} onClick={() => onSelectFailure(linkedFailure.failureId)}>View error</button>}
              {isPlanCreated && traceId && (
                <button type="button" aria-expanded={isExpanded} aria-controls={`plan-detail-${record.sequence}`} onClick={() => handleTogglePlan(record)}>
                  {isExpanded ? "Hide Plan" : "Show Plan"}
                </button>
              )}
            </td>
          </tr>
          {isPlanCreated && isExpanded && (
            <tr key={`${record.sequence}-plan`}>
              <td colSpan={5}>
                <div id={`plan-detail-${record.sequence}`} className="trace-plan-expanded" role="region" aria-label={`Plan for record ${record.sequence}`}>
                  {!traceId && <p role="status">Trace context unavailable.</p>}
                  {traceId && entry?.loading && <p role="status">Loading plan…</p>}
                  {entry?.error && <p role="alert">{entry.error}</p>}
                  {entry?.json && <pre style={{ maxBlockSize: "32rem", overflow: "auto" }}>{entry.json}</pre>}
                </div>
              </td>
            </tr>
          )}
        </Fragment>
      );
    })}</tbody></table></div>
    <h4>Attempts, retries, and validation</h4><ul>{attempts.map((attempt) => {
      const linkedFailure = failures.find((failure) => failure.attemptId === attempt.attemptId);
      const reason = attempt.attemptReason.toLowerCase().replaceAll("_", " ");
      return <li key={attempt.attemptId}>
        <button type="button" onClick={() => related({ attemptId: attempt.attemptId })}>{attempt.attemptId}</button>
        {` (attempt ${attempt.attemptNumber}, ${reason}, provider ${attempt.providerAttemptNumber}) — ${attempt.outcome}`}
        {attempt.failureCategory && `: ${attempt.failureCategory}`}
        {attempt.retryDecision && `; ${attempt.retryDecision}`}
        {attempt.retryDecision === "RETRY" && ` in ${attempt.retryDelayMillis} ms (${attempt.retryDelaySource})`}
        {attempt.httpStatus ? `; HTTP ${attempt.httpStatus}` : ""}
        {attempt.providerErrorType ? `; type ${attempt.providerErrorType}` : ""}
        {attempt.payloadId && <button type="button" onClick={() => onPayload(attempt.payloadId!)}>Read failed-attempt payload</button>}
        {linkedFailure && <button type="button" onClick={() => onSelectFailure(linkedFailure.failureId)}>Open linked failure</button>}
        {!attempt.usageComplete && " — usage incomplete"}
      </li>;
    })}{retries.map((retry) => <li key={retry.retrySequenceId}><button type="button" onClick={() => related({ retrySequenceId: retry.retrySequenceId })}>Retry {retry.retrySequenceId}</button>{!retry.usageComplete && " — usage incomplete"}</li>)}{validations.map((validation) => <li key={`${validation.attemptId}-${validation.status}`}><button type="button" onClick={() => related({ validationStatus: validation.status, attemptId: validation.attemptId })}>{validation.status}: {validation.attemptId}</button></li>)}</ul>
    <h4>Failures and uncertainty</h4><ul>{failures.map((failure) => <li key={failure.failureId}><button type="button" aria-pressed={selectedFailureId === failure.failureId} onClick={() => { onSelectFailure(failure.failureId); related({ failureId: failure.failureId }); }}>{failure.failureId}{failure.terminal && " (terminal)"}</button>{failure.attemptId && <button type="button" onClick={() => related({ attemptId: failure.attemptId })}>Open linked attempt</button>}</li>)}{gaps.map((gap, index) => <li key={`gap-${index}`}>{gap.kind}{gap.frameId && <>: <button type="button" onClick={() => related({ frameIds: [gap.frameId] })}>{gap.frameId}</button></>}</li>)}{uncertainties.map((uncertainty, index) => <li key={`uncertainty-${index}`}>{uncertainty.kind}{uncertainty.frameId && <>: <button type="button" onClick={() => related({ frameIds: [uncertainty.frameId] })}>{uncertainty.frameId}</button></>}</li>)}</ul>
    <h4>Payloads</h4><ul>{payloads.map((payload) => <li key={payload.payloadId}><button type="button" onClick={() => onPayload(payload.payloadId)}>{payload.payloadId}</button> ({payload.contentType}, {payload.storeLength} bytes)</li>)}</ul>
  </div>;
}
