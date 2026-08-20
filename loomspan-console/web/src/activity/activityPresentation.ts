import { ACTIVITY_KIND_LABELS, type Activity, type ActivityKind } from "../api/contracts";

export type ActivityFact = {
  label: string;
  value: string;
  title?: string;
};

export type ActivityPresentation = {
  label: string;
  isTerminal: boolean;
  isError: boolean;
  isFrameBoundary: boolean;
  outcome: string | null;
  artifactAvailable: boolean;
  headline: string | null;
  facts: ActivityFact[];
  scope: string | null;
  scopeTitle: string | null;
};

const TERMINAL_KINDS: ReadonlySet<ActivityKind> = new Set([
  "TRACE_COMPLETED",
  "EXECUTION_OBSERVATION_ENDED",
]);

const ERROR_KINDS: ReadonlySet<ActivityKind> = new Set([
  "ERROR_RECORDED",
  "STEP_ACTION_REJECTED",
  "PLAN_VALIDATION_FAILED",
  "TOOL_CALL_FAILED",
  "MODEL_ATTEMPT_FAILED",
  "STEP_FAILED",
]);

const FRAME_BOUNDARY_KINDS: ReadonlySet<ActivityKind> = new Set([
  "FRAME_OPENED",
  "FRAME_CLOSED",
]);

export function presentActivity(activity: Activity): ActivityPresentation {
  const kind = activity.kind as ActivityKind;
  const label = ACTIVITY_KIND_LABELS[kind] ?? kind;
  const isTerminal = TERMINAL_KINDS.has(kind);
  const isError = ERROR_KINDS.has(kind);
  const isFrameBoundary = FRAME_BOUNDARY_KINDS.has(kind);

  let outcome: string | null = null;
  const details = parseDetails(activity.details);
  if (kind === "TRACE_COMPLETED") {
    outcome = typeof details?.outcome === "string" ? details.outcome : "completed";
  }

  const artifactAvailable =
    kind === "TRACE_COMPLETED" &&
    details?.applicationTraceAvailability === "AVAILABLE";

  const { headline, facts } = describeActivity(kind, details);
  const scope = activity.route || text(details, "skillName");
  const scopeTitle = activity.frameId || null;

  return {
    label,
    isTerminal,
    isError,
    isFrameBoundary,
    outcome,
    artifactAvailable,
    headline,
    facts,
    scope,
    scopeTitle,
  };
}

function describeActivity(
  kind: ActivityKind,
  details: Record<string, unknown> | null,
): { headline: string | null; facts: ActivityFact[] } {
  const facts: ActivityFact[] = [];
  let headline: string | null = null;

  switch (kind) {
    case "MODEL_REQUEST_SENT":
    case "MODEL_RESPONSE_RECEIVED":
      headline = text(details, "segment");
      addAttempt(facts, details);
      break;
    case "MODEL_ATTEMPT_FAILED":
      headline = text(details, "failureCategory") ?? text(details, "failureClassification");
      addAttempt(facts, details);
      add(facts, "decision", text(details, "retryDecision"));
      add(facts, "delay", retryDelay(details));
      break;
    case "TOOL_CALL_STARTED":
    case "TOOL_CALL_COMPLETED":
    case "TOOL_CALL_FAILED":
      headline = text(details, "capabilityName");
      add(facts, "task", text(details, "linkedTaskId"));
      if (details?.unplanned === true) add(facts, "planning", "unplanned");
      break;
    case "STEP_STARTED":
    case "STEP_COMPLETED":
    case "STEP_FAILED":
    case "STEP_ACTION_REJECTED": {
      const step = number(details, "stepNumber");
      headline = step === null ? null : `Step ${step}`;
      add(facts, "action", text(details, "stepAction"));
      add(facts, "reason", text(details, "reason"));
      break;
    }
    case "PLAN_CREATED":
    case "PLAN_UPDATED":
    case "PLAN_VALIDATION_FAILED":
    case "PLAN_RETRY_REQUESTED":
      headline = text(details, "reason");
      add(facts, "retry", integer(details, "retry"));
      if (details?.exhausted === true) add(facts, "retries", "exhausted");
      break;
    case "ERROR_RECORDED":
      headline = text(details, "message");
      add(facts, "classification", text(details, "classification"));
      add(facts, "exception", simpleName(text(details, "exceptionType")), text(details, "exceptionType"));
      break;
    default:
      break;
  }

  return { headline, facts };
}

function addAttempt(facts: ActivityFact[], details: Record<string, unknown> | null) {
  const attemptNumber = number(details, "attemptNumber");
  const attemptId = text(details, "attemptId");
  if (attemptNumber !== null) {
    facts.push({
      label: "attempt",
      value: String(attemptNumber),
      ...(attemptId ? { title: attemptId } : {}),
    });
  }
  const reason = text(details, "attemptReason");
  if (reason && reason !== "INITIAL") add(facts, "reason", reason);
  const providerAttempt = number(details, "providerAttemptNumber");
  if (providerAttempt !== null && providerAttempt !== attemptNumber) {
    add(facts, "provider attempt", String(providerAttempt));
  }
}

function add(facts: ActivityFact[], label: string, value: string | null, title?: string | null) {
  if (!value) return;
  facts.push({ label, value, ...(title ? { title } : {}) });
}

function retryDelay(details: Record<string, unknown> | null): string | null {
  const millis = number(details, "retryDelayMillis");
  if (millis === null) return null;
  const formatted = millis < 1000 ? `${millis}ms` : `${(millis / 1000).toFixed(1)}s`;
  const source = text(details, "retryDelaySource");
  return source ? `${formatted} (${source})` : formatted;
}

function simpleName(qualified: string | null): string | null {
  if (!qualified) return null;
  const separator = qualified.lastIndexOf(".");
  return separator === -1 ? qualified : qualified.slice(separator + 1);
}

export function formatTimestamp(iso: string): string {
  const date = new Date(iso);
  if (isNaN(date.getTime())) return iso;
  return date.toLocaleTimeString(undefined, {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

export function formatDateTime(iso: string): string {
  const date = new Date(iso);
  if (isNaN(date.getTime())) return iso;
  const pad = (value: number) => String(value).padStart(2, "0");
  return `${pad(date.getMonth() + 1)}/${pad(date.getDate())}/${date.getFullYear()} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;
}

export function formatElapsed(startIso: string, endIso: string): string {
  const start = new Date(startIso).getTime();
  const end = new Date(endIso).getTime();
  if (isNaN(start) || isNaN(end)) return "—";
  const elapsedMs = end - start;
  if (elapsedMs < 1000) return `${elapsedMs}ms`;
  if (elapsedMs < 60_000) return `${(elapsedMs / 1000).toFixed(1)}s`;
  const minutes = Math.floor(elapsedMs / 60_000);
  const seconds = Math.floor((elapsedMs % 60_000) / 1000);
  return `${minutes}m ${seconds}s`;
}

export function formatDelta(previousIso: string, iso: string): string | null {
  const previous = new Date(previousIso).getTime();
  const current = new Date(iso).getTime();
  if (isNaN(previous) || isNaN(current) || current < previous) return null;
  return `+${formatElapsed(previousIso, iso)}`;
}

function parseDetails(details: unknown): Record<string, unknown> | null {
  if (details && typeof details === "object") {
    return details as Record<string, unknown>;
  }
  return null;
}

function text(details: Record<string, unknown> | null, key: string): string | null {
  const value = details?.[key];
  return typeof value === "string" && value !== "" ? value : null;
}

function number(details: Record<string, unknown> | null, key: string): number | null {
  const value = details?.[key];
  return typeof value === "number" && Number.isFinite(value) ? value : null;
}

function integer(details: Record<string, unknown> | null, key: string): string | null {
  const value = number(details, key);
  return value === null ? null : String(value);
}
