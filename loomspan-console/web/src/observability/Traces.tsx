import { useEffect, useRef } from "react";
import { Link } from "react-router";
import { useObservability } from "./ObservabilityProvider";
import type { Trace } from "../api/contracts";
import { scopeBoundPath } from "./scope";
import { formatDateTime } from "../activity/activityPresentation";

export function Traces() {
  const { traces, loadTraces } = useObservability();
  const heading = useRef<HTMLHeadingElement>(null);

  useEffect(() => {
    heading.current?.focus();
  }, []);

  useEffect(() => {
    if (!traces.loaded && !traces.loading && !traces.error) {
      void loadTraces();
    }
  }, [traces, loadTraces]);

  return (
    <section aria-labelledby="traces-title" className="overview-card">
      <p className="eyebrow">Operational views</p>
      <h2 id="traces-title" ref={heading} tabIndex={-1}>Trace Catalog</h2>
      <button type="button" disabled={traces.loading} onClick={() => void loadTraces()}>
        Refresh
      </button>

      {traces.error && (
        <div className="target-error" role="alert">
          <strong>{traces.error.message}</strong>
          <div>
            <button type="button" disabled={traces.loading} onClick={() => void loadTraces()}>
              Retry
            </button>
          </div>
        </div>
      )}

      {traces.loading && <p>Loading traces…</p>}

      {traces.loaded && !traces.loading && traces.items.length === 0 && !traces.error && (
        <p>No traces are cataloged.</p>
      )}

      {traces.items.length > 0 && (
        <div className="observability-table-region" role="region" aria-label="Trace catalog table" tabIndex={0}>
          <table className="observability-table">
          <thead>
            <tr>
              <th scope="col">Entry skill</th>
              <th scope="col">Trace ID</th>
              <th scope="col">Session</th>
              <th scope="col">Outcome</th>
              <th scope="col">Finalized at</th>
              <th scope="col">Size (bytes)</th>
              <th scope="col">Persistence</th>
              <th scope="col">Expires at</th>
            </tr>
          </thead>
          <tbody>
            {traces.items.map((item) => {
              const t = item as Trace;
              return (
                <tr key={t.traceId}>
                  <td>{t.entrySkill}</td>
                  <td>
                    <Link to={scopeBoundPath(`/traces/${encodeURIComponent(t.traceId)}`, traces.targetScopeId)}>{t.traceId}</Link>
                  </td>
                  <td>{t.sessionId}</td>
                  <td>{t.outcome}</td>
                  <td>{formatDateTime(t.finalizedAt)}</td>
                  <td>{String(t.sizeBytes)}</td>
                  <td>{t.persistencePolicy}</td>
                  <td>{formatDateTime(t.applicationTraceExpiresAt)}</td>
                </tr>
              );
            })}
          </tbody>
          </table>
        </div>
      )}

      {traces.hasMore && traces.nextCursor && (
        <button type="button" disabled={traces.loading} onClick={() => void loadTraces(traces.nextCursor ?? undefined)}>
          Load more
        </button>
      )}
    </section>
  );
}
