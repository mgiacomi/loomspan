import type { ReactNode } from "react";
import type { Trace } from "../api/contracts";
import { formatDateTime } from "../activity/activityPresentation";

export function TraceSummary({ trace, action }: { trace: Trace; action?: ReactNode }) {
  return (
    <div className="trace-summary" aria-label="Finalized trace summary">
      <div className="summary-header">
        <span className="summary-status outcome" aria-label={`Outcome: ${trace.outcome}`}>
          {trace.outcome}
        </span>
        <span className="summary-finalized">
          Finalized {formatDateTime(trace.finalizedAt)}
        </span>
        {action && <span className="summary-action">{action}</span>}
      </div>
      <dl className="summary-facts">
        <div className="summary-fact">
          <dt>Entry skill</dt>
          <dd>{trace.entrySkill}</dd>
        </div>
        <div className="summary-fact identifier">
          <dt>Session</dt>
          <dd>{trace.sessionId}</dd>
        </div>
        <div className="summary-fact identifier">
          <dt>Trace</dt>
          <dd>{trace.traceId}</dd>
        </div>
      </dl>
      <p className="observability-note">
        Retention <strong>{trace.persistencePolicy}</strong>; the application trace
        expires at {formatDateTime(trace.applicationTraceExpiresAt)}.
      </p>
    </div>
  );
}
