import { Fragment, useState } from "react";
import type { KeyboardEvent } from "react";
import { getPayloadRange, getRawRecordRange, getTraceRecords } from "../api/client";
import type { TraceRange } from "../api/contracts";
import type { TraceAttempt, TraceFailure, TraceGap, TracePayload, TraceRecord, TraceRetry, TraceUncertainty, TraceValidation } from "../api/contracts";
import type { TraceFrameFilter } from "../api/client";
import { comparePlans, toPlanSnapshot } from "./planComparison";
import type { PlanComparison, PlanSnapshot } from "./planComparison";

type Props = { traceId?: string; records: TraceRecord[]; attempts: TraceAttempt[]; retries: TraceRetry[]; failures: TraceFailure[]; validations: TraceValidation[]; gaps: TraceGap[]; uncertainties: TraceUncertainty[]; payloads: TracePayload[]; selectedRecordSequence?: number; selectedFailureId?: string; onSelectRecord: (record: TraceRecord) => void; onSelectFailure: (failureId: string) => void; onRelatedFrame?: (filter: TraceFrameFilter) => void; onRaw: (record: TraceRecord) => void; onPayload: (payloadId: string) => void };

type PlanCacheEntry = {
  loading: boolean;
  error?: string;
  json?: string;
  snapshot?: PlanSnapshot;
  comparisonLoading?: boolean;
  comparisonError?: string;
  comparisonReady?: boolean;
  comparison?: PlanComparison;
  previousSequence?: number;
};

type ModelDetail =
  | { kind: "request"; messages: { role: string; text: string }[]; fallback?: string }
  | { kind: "response"; content: string };

type ModelCacheEntry = { loading: boolean; error?: string; detail?: ModelDetail };

function decodeBytes(range: TraceRange): Uint8Array {
  if (range.encoding === "BASE64") {
    try {
      const binary = atob(range.content);
      return Uint8Array.from(binary, (character) => character.charCodeAt(0));
    } catch {
      throw new Error("Content contained invalid base64 data.");
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
    if (!range.nextCursor || range.nextCursor === cursor) throw new Error("Content continuation was invalid.");
    cursor = range.nextCursor;
  } while (true);
  return new TextDecoder("utf-8", { fatal: true }).decode(joinBytes(parts)).trim();
}

async function readCompletePayload(traceId: string, payloadId: string): Promise<string> {
  const parts: Uint8Array[] = [];
  let cursor: string | undefined;
  do {
    const range = await getPayloadRange(traceId, payloadId, cursor);
    parts.push(decodeBytes(range));
    if (!range.hasMore) break;
    if (!range.nextCursor || range.nextCursor === cursor) throw new Error("Content continuation was invalid.");
    cursor = range.nextCursor;
  } while (true);
  return new TextDecoder("utf-8", { fatal: true }).decode(joinBytes(parts)).trim();
}

function parseJsonObject(raw: string, label: string): Record<string, unknown> {
  const value: unknown = JSON.parse(raw);
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new Error(`${label} did not contain a JSON object.`);
  }
  return value as Record<string, unknown>;
}

function recordData(rawRecord: string): unknown {
  const envelope = parseJsonObject(rawRecord, "Model record");
  return "data" in envelope ? envelope.data : envelope.Data;
}

function readableValue(value: unknown): string {
  if (typeof value === "string") return value;
  return JSON.stringify(value, null, 2);
}

function parseModelDetail(kind: "request" | "response", value: unknown): ModelDetail {
  if (typeof value === "string") {
    try {
      value = JSON.parse(value);
    } catch {
      if (kind === "response") return { kind, content: String(value) };
    }
  }
  if (kind === "request") {
    if (value && typeof value === "object" && !Array.isArray(value)) {
      const request = value as Record<string, unknown>;
      if (Array.isArray(request.messages)) {
        const messages = request.messages.map((message, index) => {
          if (typeof message === "string") return { role: `Message ${index + 1}`, text: message };
          if (message && typeof message === "object" && !Array.isArray(message)) {
            const fields = message as Record<string, unknown>;
            const role = typeof fields.messageType === "string" ? fields.messageType : typeof fields.role === "string" ? fields.role : `Message ${index + 1}`;
            const content = "text" in fields ? fields.text : fields.content;
            return { role, text: readableValue(content) };
          }
          return { role: `Message ${index + 1}`, text: readableValue(message) };
        });
        return { kind, messages, fallback: JSON.stringify(value, null, 2) };
      }
    }
    return { kind, messages: [], fallback: JSON.stringify(value, null, 2) };
  }

  let content = value;
  if (value && typeof value === "object" && !Array.isArray(value) && "content" in value) {
    content = (value as Record<string, unknown>).content;
  }
  if (typeof content === "string") {
    const trimmed = content.trim();
    try {
      return { kind, content: JSON.stringify(JSON.parse(trimmed), null, 2) };
    } catch {
      return { kind, content };
    }
  }
  return { kind, content: JSON.stringify(content, null, 2) };
}

function ModelDetailView({ detail }: { detail: ModelDetail }) {
  if (detail.kind === "response") return <pre>{detail.content}</pre>;
  if (detail.messages.length === 0) return <pre>{detail.fallback}</pre>;
  return <div className="trace-model-messages">
    {detail.messages.map((message, index) => <section className="trace-model-message" key={`${message.role}-${index}`}>
      <h5>{message.role.toLowerCase().replaceAll("_", " ")}</h5>
      <pre>{message.text}</pre>
    </section>)}
  </div>;
}

function parsePlanRecord(rawRecord: string): PlanSnapshot {
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
  return toPlanSnapshot(plan);
}

async function findPreviousPlan(traceId: string, sequence: number, planId: string): Promise<{ sequence: number; snapshot: PlanSnapshot } | undefined> {
  if (sequence <= 1) return undefined;
  const candidates: TraceRecord[] = [];
  let cursor: string | undefined;
  do {
    const page = await getTraceRecords(traceId, cursor, {
      types: ["PLAN_CREATED", "PLAN_UPDATED"],
      maxSequence: sequence - 1,
    });
    candidates.push(...page.items);
    if (!page.hasMore) break;
    if (!page.nextCursor || page.nextCursor === cursor) throw new Error("Plan history continuation was invalid.");
    cursor = page.nextCursor;
  } while (true);

  candidates.sort((left, right) => right.sequence - left.sequence);
  for (const candidate of candidates) {
    const rawRecord = await readCompleteRecord(traceId, candidate.sequence);
    let snapshot: PlanSnapshot;
    try {
      snapshot = parsePlanRecord(rawRecord);
    } catch {
      continue;
    }
    if (snapshot.planId === planId) return { sequence: candidate.sequence, snapshot };
  }
  return undefined;
}

function PlanChanges({ comparison, previousSequence }: { comparison: PlanComparison; previousSequence: number }) {
  const hasChanges = comparison.plan.length > 0 || comparison.tasks.length > 0;
  return <section className="trace-plan-changes" aria-label="Plan changes">
    <p className="trace-plan-comparison-source">Changes since record {previousSequence}</p>
    {!hasChanges && <p>No task or plan-state changes were detected. Check Full plan for other fields.</p>}
    {comparison.plan.length > 0 && <div>
      <h5>Plan</h5>
      <ul>{comparison.plan.map((change) => <li key={change.label}><strong>{change.label}:</strong> {change.before} <span aria-label="changed to">{"\u2192"}</span> {change.after}</li>)}</ul>
    </div>}
    {comparison.tasks.map((task) => <div className="trace-plan-task-change" key={task.taskId}>
      <h5>{task.intent}</h5>
      {task.kind === "added" && <p>Task added</p>}
      {task.kind === "removed" && <p>Task removed</p>}
      {task.fields.length > 0 && <ul>{task.fields.map((change) => <li key={change.label}><strong>{change.label}:</strong> {change.before} <span aria-label="changed to">{"\u2192"}</span> {change.after}</li>)}</ul>}
    </div>)}
  </section>;
}

export function TraceRecords({ traceId, records, attempts, retries, failures, validations, gaps, uncertainties, payloads, selectedRecordSequence, selectedFailureId, onSelectRecord, onSelectFailure, onRelatedFrame, onRaw, onPayload }: Props) {
  const related = onRelatedFrame ?? (() => undefined);
  const [expanded, setExpanded] = useState<string | null>(null);
  const [cache, setCache] = useState<Record<string, PlanCacheEntry>>({});
  const [modelCache, setModelCache] = useState<Record<string, ModelCacheEntry>>({});
  const [planView, setPlanView] = useState<"changes" | "full">("full");

  const handleTogglePlan = (record: TraceRecord) => {
    const seq = record.sequence;
    const key = `${traceId}:${seq}`;
    if (expanded === key) {
      setExpanded(null);
      return;
    }
    const isUpdate = record.type === "PLAN_UPDATED";
    setPlanView(isUpdate ? "changes" : "full");
    setExpanded(key);
    if (!traceId) return;
    const existing = cache[key];
    if (existing?.json || existing?.loading) return;
    setCache((prev) => ({ ...prev, [key]: { loading: true } }));
    void readCompleteRecord(traceId, seq)
      .then(async (rawRecord) => {
        const snapshot = parsePlanRecord(rawRecord);
        const json = JSON.stringify(snapshot.value, null, 2);
        setCache((prev) => ({ ...prev, [key]: { loading: false, json, snapshot, comparisonLoading: isUpdate } }));
        if (!isUpdate) return;
        if (!snapshot.planId) {
          setCache((prev) => ({ ...prev, [key]: { ...prev[key], comparisonLoading: false, comparisonError: "The plan update did not contain a plan ID." } }));
          return;
        }
        try {
          const previous = await findPreviousPlan(traceId, seq, snapshot.planId);
          const comparison = previous ? comparePlans(previous.snapshot, snapshot) : undefined;
          setCache((prev) => ({ ...prev, [key]: { ...prev[key], comparisonLoading: false, comparisonReady: true, comparison, previousSequence: previous?.sequence } }));
        } catch (err: unknown) {
          const message = err instanceof Error ? err.message : "The previous plan version could not be loaded.";
          setCache((prev) => ({ ...prev, [key]: { ...prev[key], comparisonLoading: false, comparisonError: message } }));
        }
      })
      .catch((err: unknown) => {
        const message = err instanceof Error ? `Plan could not be displayed: ${err.message}` : "Plan could not be displayed.";
        setCache((prev) => ({ ...prev, [key]: { loading: false, error: message } }));
      });
  };

  const handleToggleModel = (record: TraceRecord) => {
    const key = `${traceId}:${record.sequence}`;
    if (expanded === key) {
      setExpanded(null);
      return;
    }
    setExpanded(key);
    if (!traceId) return;
    const existing = modelCache[key];
    if (existing?.detail || existing?.loading) return;
    const kind = record.type === "MODEL_REQUEST_SENT" ? "request" : "response";
    setModelCache((previous) => ({ ...previous, [key]: { loading: true } }));
    const source = record.payloadId
      ? readCompletePayload(traceId, record.payloadId).then((raw) => parseJsonObject(raw, "Model payload"))
      : readCompleteRecord(traceId, record.sequence).then(recordData);
    void source
      .then((value) => {
        const detail = parseModelDetail(kind, value);
        setModelCache((previous) => ({ ...previous, [key]: { loading: false, detail } }));
      })
      .catch((error: unknown) => {
        const message = error instanceof Error ? `${kind === "request" ? "Request" : "Response"} could not be displayed: ${error.message}` : `${kind === "request" ? "Request" : "Response"} could not be displayed.`;
        setModelCache((previous) => ({ ...previous, [key]: { loading: false, error: message } }));
      });
  };

  return <div aria-label="Trace records">
    <h4>Records</h4><div className="trace-table-region" role="region" aria-label="Record list" tabIndex={0}><table><thead><tr><th>Sequence</th><th>Type</th><th>Frame</th><th>Timestamp</th><th>Actions</th></tr></thead><tbody>{records.map((record) => {
      const isPlanCreated = record.type === "PLAN_CREATED";
      const isPlanUpdated = record.type === "PLAN_UPDATED";
      const isPlanRecord = isPlanCreated || isPlanUpdated;
      const isModelRequest = record.type === "MODEL_REQUEST_SENT";
      const isModelResponse = record.type === "MODEL_RESPONSE_RECEIVED";
      const isModelRecord = isModelRequest || isModelResponse;
      const key = `${traceId}:${record.sequence}`;
      const isExpanded = expanded === key;
      const entry = cache[key];
      const modelEntry = modelCache[key];
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
              {record.payloadId && !isModelRecord && <button type="button" onClick={() => onPayload(record.payloadId)}>Read payload</button>}
              {linkedFailure && <button className="trace-error-action" type="button" aria-pressed={selectedFailureId === linkedFailure.failureId} onClick={() => onSelectFailure(linkedFailure.failureId)}>View error</button>}
              {isPlanRecord && traceId && (
                <button type="button" aria-expanded={isExpanded} aria-controls={`plan-detail-${record.sequence}`} onClick={() => handleTogglePlan(record)}>
                  {isExpanded ? (isPlanUpdated ? "Hide changes" : "Hide Plan") : (isPlanUpdated ? "View changes" : "Show Plan")}
                </button>
              )}
              {isModelRecord && traceId && (
                <button type="button" aria-expanded={isExpanded} aria-controls={`model-detail-${record.sequence}`} onClick={() => handleToggleModel(record)}>
                  {isExpanded ? `Hide ${isModelRequest ? "request" : "response"}` : (isModelRequest ? "Request" : "Response")}
                </button>
              )}
            </td>
          </tr>
          {isPlanRecord && isExpanded && (
            <tr key={`${record.sequence}-plan`}>
              <td colSpan={5}>
                <div id={`plan-detail-${record.sequence}`} className="trace-plan-expanded" role="region" aria-label={`${isPlanUpdated ? "Plan update" : "Plan"} for record ${record.sequence}`}>
                  {!traceId && <p role="status">Trace context unavailable.</p>}
                  {traceId && entry?.loading && <p role="status">Loading plan…</p>}
                  {entry?.error && <p role="alert">{entry.error}</p>}
                  {entry?.json && isPlanUpdated && <>
                    <div role="tablist" aria-label={`Plan record ${record.sequence} views`}>
                      {(["changes", "full"] as const).map((view) => <button
                        id={`plan-${record.sequence}-tab-${view}`}
                        aria-controls={`plan-${record.sequence}-panel-${view}`}
                        aria-selected={planView === view}
                        key={view}
                        onClick={() => setPlanView(view)}
                        onKeyDown={(event: KeyboardEvent<HTMLButtonElement>) => {
                          if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;
                          event.preventDefault();
                          const next = event.key === "ArrowLeft" || event.key === "Home" ? "changes" : "full";
                          setPlanView(next);
                          document.getElementById(`plan-${record.sequence}-tab-${next}`)?.focus();
                        }}
                        role="tab"
                        tabIndex={planView === view ? 0 : -1}
                        type="button"
                      >{view === "changes" ? "Changes" : "Full plan"}</button>)}
                    </div>
                    {planView === "changes" && <div id={`plan-${record.sequence}-panel-changes`} aria-labelledby={`plan-${record.sequence}-tab-changes`} role="tabpanel">
                      {entry.comparisonLoading && <p role="status">Finding the previous plan versionâ€¦</p>}
                      {entry.comparisonError && <p role="alert">Changes could not be determined: {entry.comparisonError}</p>}
                      {entry.comparisonReady && entry.comparison && entry.previousSequence !== undefined && <PlanChanges comparison={entry.comparison} previousSequence={entry.previousSequence} />}
                      {entry.comparisonReady && !entry.comparison && <p>The earlier version of this plan is not available in the trace.</p>}
                    </div>}
                    {planView === "full" && <div id={`plan-${record.sequence}-panel-full`} aria-labelledby={`plan-${record.sequence}-tab-full`} role="tabpanel"><pre>{entry.json}</pre></div>}
                  </>}
                  {entry?.json && isPlanCreated && <pre>{entry.json}</pre>}
                </div>
              </td>
            </tr>
          )}
          {isModelRecord && isExpanded && (
            <tr key={`${record.sequence}-model`}>
              <td colSpan={5}>
                <div id={`model-detail-${record.sequence}`} className="trace-model-expanded" role="region" aria-label={`${isModelRequest ? "Model request" : "Model response"} for record ${record.sequence}`}>
                  {!traceId && <p role="status">Trace context unavailable.</p>}
                  {traceId && modelEntry?.loading && <p role="status">Loading {isModelRequest ? "request" : "response"}â€¦</p>}
                  {modelEntry?.error && <p role="alert">{modelEntry.error}</p>}
                  {modelEntry?.detail && <ModelDetailView detail={modelEntry.detail} />}
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
