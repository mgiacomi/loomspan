import { ACTIVITY_KIND_LABELS, type Activity, type ActivityKind } from "../api/contracts";

export type ActivityPresentation = {
  label: string;
  isTerminal: boolean;
  isError: boolean;
  isFrameBoundary: boolean;
  outcome: string | null;
  artifactAvailable: boolean;
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

  return { label, isTerminal, isError, isFrameBoundary, outcome, artifactAvailable };
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

function parseDetails(details: unknown): Record<string, unknown> | null {
  if (details && typeof details === "object") {
    return details as Record<string, unknown>;
  }
  return null;
}
