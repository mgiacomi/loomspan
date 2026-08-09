import { useEffect, useState } from "react";
import type { ActiveExecution, Activity } from "../api/contracts";
import { formatDateTime, presentActivity } from "./activityPresentation";

type CurrentExecutionSummaryProps = {
  execution: ActiveExecution | null;
  activities: Activity[];
  observedAt?: string | null;
  connected?: boolean;
  observationEnded?: boolean;
};

function formatElapsedMilliseconds(elapsedMillis: number): string {
  if (elapsedMillis < 1000) return `${elapsedMillis}ms`;
  if (elapsedMillis < 60_000) return `${(elapsedMillis / 1000).toFixed(1)}s`;
  const minutes = Math.floor(elapsedMillis / 60_000);
  const seconds = Math.floor((elapsedMillis % 60_000) / 1000);
  return `${minutes}m ${seconds}s`;
}

export function CurrentExecutionSummary({
  execution,
  activities,
  observedAt,
  connected = false,
  observationEnded = false,
}: CurrentExecutionSummaryProps) {
  const [now, setNow] = useState(() => Date.now());
  const latest = activities.at(-1);
  const terminal = latest?.kind === "TRACE_COMPLETED" ||
    latest?.kind === "EXECUTION_OBSERVATION_ENDED";

  useEffect(() => {
    if (!execution || observationEnded || terminal || !connected) return;
    setNow(Date.now());
    const timer = window.setInterval(() => setNow(Date.now()), 1_000);
    return () => window.clearInterval(timer);
  }, [execution, observationEnded, terminal, connected, observedAt]);

  if (!execution && !latest) {
    return (
      <div className="current-execution-summary empty" aria-live="polite">
        <p className="empty-state">The selected execution snapshot is unavailable.</p>
      </div>
    );
  }

  const presentation = latest ? presentActivity(latest) : null;
  const status = terminal
    ? (latest?.executionStatus || "TERMINAL")
    : observationEnded
      ? "OBSERVATION ENDED"
      : (execution?.status || latest?.executionStatus || "UNKNOWN");
  const sessionId = execution?.sessionId ?? latest?.sessionId;
  const traceId = execution?.traceId ?? latest?.traceId;
  const observedAtMillis = observedAt ? Date.parse(observedAt) : Number.NaN;
  const elapsedMillis = execution
    ? execution.elapsedMillis +
      (!observationEnded && !terminal && connected && Number.isFinite(observedAtMillis)
        ? Math.max(0, now - observedAtMillis)
        : 0)
    : 0;

  return (
    <div
      className="current-execution-summary"
      aria-live="polite"
      aria-label="Current execution summary"
    >
      <div className="summary-header">
        <span className={`summary-status ${status.toLowerCase()}`} aria-label={`Status: ${status}`}>
          {status}
        </span>
        {presentation?.isTerminal && (
          <span className="summary-terminal-badge" aria-label="Terminal">
            {presentation.outcome ??
              (latest?.kind === "EXECUTION_OBSERVATION_ENDED"
                ? "observation ended"
                : "completed")}
          </span>
        )}
      </div>
      <dl className="summary-facts">
        <div className="summary-fact identifier">
          <dt>Session</dt>
          <dd>{sessionId}</dd>
        </div>
        <div className="summary-fact identifier">
          <dt>Trace</dt>
          <dd>{traceId}</dd>
        </div>
        {execution && (
          <>
            <div className="summary-fact">
              <dt>Entry skill</dt>
              <dd>{execution.entrySkill}</dd>
            </div>
            <div className="summary-fact">
              <dt>Phase</dt>
              <dd>{execution.phase}</dd>
            </div>
            <div className="summary-fact">
              <dt>Started</dt>
              <dd>{formatDateTime(execution.startedAt)}</dd>
            </div>
            <div className="summary-fact">
              <dt>Elapsed</dt>
              <dd>{formatElapsedMilliseconds(elapsedMillis)}</dd>
            </div>
          </>
        )}
      </dl>
      <p className="observability-note">
        {observedAt
          ? `Snapshot observed at ${observedAt}.`
          : "Snapshot observation time is unavailable."}
        {execution ? ` Execution updated at ${execution.updatedAt}.` : ""}
        {" "}
        Live updates are {connected ? "connected" : "disconnected"}.
      </p>
      {execution && <p className="summary-latest-summary">{execution.summary}</p>}
      {latest && latest.summary !== execution?.summary && (
        <p className="summary-latest-summary" aria-label="Latest activity summary">
          Latest activity: {latest.summary}
        </p>
      )}
    </div>
  );
}
