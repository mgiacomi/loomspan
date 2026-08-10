import type { TraceAttempt, TraceFailure, TraceGap, TracePayload, TraceRecord, TraceRetry, TraceUncertainty, TraceValidation } from "../api/contracts";
import type { TraceFrameFilter } from "../api/client";

type Props = { records: TraceRecord[]; attempts: TraceAttempt[]; retries: TraceRetry[]; failures: TraceFailure[]; validations: TraceValidation[]; gaps: TraceGap[]; uncertainties: TraceUncertainty[]; payloads: TracePayload[]; selectedRecordSequence?: number; selectedFailureId?: string; onSelectRecord: (record: TraceRecord) => void; onSelectFailure: (failureId: string) => void; onRelatedFrame?: (filter: TraceFrameFilter) => void; onRaw: (record: TraceRecord) => void; onPayload: (payloadId: string) => void };

export function TraceRecords({ records, attempts, retries, failures, validations, gaps, uncertainties, payloads, selectedRecordSequence, selectedFailureId, onSelectRecord, onSelectFailure, onRelatedFrame, onRaw, onPayload }: Props) {
  const related = onRelatedFrame ?? (() => undefined);
  return <div aria-label="Trace records">
    <h4>Records</h4><div className="trace-table-region" role="region" aria-label="Record list" tabIndex={0}><table><thead><tr><th>Sequence</th><th>Type</th><th>Frame</th><th>Timestamp</th><th>Actions</th></tr></thead><tbody>{records.map((record) => <tr key={record.sequence} aria-current={selectedRecordSequence === record.sequence ? "true" : undefined}><td><button type="button" onClick={() => onSelectRecord(record)}>{record.sequence}: {record.type}</button></td><td>{record.type}</td><td>{record.frameId && <button type="button" onClick={() => related({ frameIds: [record.frameId] })}>{record.frameId}</button>}</td><td>{record.timestampMillis}</td><td><button type="button" onClick={() => onRaw(record)}>Read raw record</button>{record.payloadId && <button type="button" onClick={() => onPayload(record.payloadId)}>Read payload</button>}</td></tr>)}</tbody></table></div>
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
