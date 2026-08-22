import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router";
import {
  ACTIVITY_KIND_LABELS,
  type ActiveExecution,
  type ActivityKind,
} from "../api/contracts";
import { useOptionalObservability } from "../observability/ObservabilityProvider";
import { scopeBoundPath } from "../observability/scope";
import { useActivity } from "./ActivityProvider";
import { ActivityNarrative } from "./ActivityNarrative";
import { formatDateTime } from "./activityPresentation";

function formatElapsedMilliseconds(elapsedMillis: number): string {
  if (elapsedMillis < 1000) return `${elapsedMillis}ms`;
  if (elapsedMillis < 60_000) return `${Math.floor(elapsedMillis / 1000)}s`;
  const minutes = Math.floor(elapsedMillis / 60_000);
  const seconds = Math.floor((elapsedMillis % 60_000) / 1000);
  return `${minutes}m ${seconds}s`;
}

export function LiveActivity() {
  const observability = useOptionalObservability();
  const activeExecutions = observability?.activeExecutions;
  const loadActiveExecutions = observability?.loadActiveExecutions;
  const {
    activities,
    recentCompletions,
    connected,
    connectionFact,
    error,
    loading,
    coverage,
    continuity,
    loadRecent,
  } = useActivity();
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    if (
      activeExecutions &&
      !activeExecutions.loaded &&
      !activeExecutions.loading &&
      !activeExecutions.error
    ) {
      void loadActiveExecutions?.();
    }
  }, [activeExecutions, loadActiveExecutions]);

  useEffect(() => {
    if (
      activeExecutions?.loaded &&
      activeExecutions.hasMore &&
      activeExecutions.nextCursor &&
      !activeExecutions.loading
    ) {
      void loadActiveExecutions?.(activeExecutions.nextCursor);
    }
  }, [activeExecutions, loadActiveExecutions]);

  useEffect(() => {
    if (!connected || (activeExecutions?.items.length ?? 0) === 0) return;
    setNow(Date.now());
    const timer = window.setInterval(() => setNow(Date.now()), 1_000);
    return () => window.clearInterval(timer);
  }, [activeExecutions?.items.length, connected]);

  const executions = (activeExecutions?.items ?? []) as ActiveExecution[];
  const activityBySession = useMemo(() => {
    const grouped = new Map<string, typeof activities>();
    for (const activity of activities) {
      const current = grouped.get(activity.sessionId);
      if (current) current.push(activity);
      else grouped.set(activity.sessionId, [activity]);
    }
    return grouped;
  }, [activities]);

  const replayGap = connectionFact?.reason === "relay_frame_limit" ||
    connectionFact?.reason === "relay_byte_limit" ||
    connectionFact?.reason === "replay_overflow" ||
    connectionFact?.reason === "subscriber_overflow";

  return (
    <section aria-labelledby="live-activity-title" className="overview-card">
      <p className="eyebrow">Real-time</p>
      <h2 id="live-activity-title">Live Activity</h2>

      <div className="status-indicator" role="status" aria-live="polite">
        <span
          className={`connection-dot ${connected ? "connected" : "disconnected"}`}
          aria-hidden="true"
        />
        {connected ? "Connected" : connectionFact?.reason ? `Disconnected: ${connectionFact.reason}` : "Disconnected"}
      </div>
      <p className="observability-note">
        {connectionFact?.at
          ? `Connection fact observed at ${connectionFact.at}.`
          : "Connection observation time is unavailable."}
        {continuity?.observedAt
          ? ` Upstream continuity observed at ${continuity.observedAt}.`
          : ""}
      </p>

      {continuity?.reset && (
        <div className="continuity-reset-notice" role="status" aria-live="polite">
          Activity window was reset ({continuity.reset.cause}). Earlier events may be unavailable.
        </div>
      )}

      {replayGap && (
        <div className="replay-gap-notice" role="alert">
          Some events were not delivered in real time.
          <button
            type="button"
            className="replay-gap-action"
            onClick={() => void loadRecent()}
          >
            Load recent
          </button>
        </div>
      )}

      {loading && <p aria-live="polite">Loading recent activity…</p>}

      {error && (
        <div className="target-error" role="alert">
          <strong>{error.message}</strong>
        </div>
      )}

      {(coverage.globalEvictedThroughCursor || coverage.sessionStartCursor ||
        coverage.sessionEvictedThroughCursor || coverage.sessionRetainedCursorRange) && (
        <div className="activity-notice" role="status">
          {coverage.globalEvictedThroughCursor && (
            <div>Global ring evicted through cursor {coverage.globalEvictedThroughCursor}.</div>
          )}
          {coverage.sessionStartCursor && (
            <div>Selected session start cursor {coverage.sessionStartCursor}.</div>
          )}
          {coverage.sessionEvictedThroughCursor && (
            <div>Selected session evicted through cursor {coverage.sessionEvictedThroughCursor}.</div>
          )}
          {coverage.sessionRetainedCursorRange && (
            <div>
              Selected session retained cursor range {coverage.sessionRetainedCursorRange.firstCursor}–{coverage.sessionRetainedCursorRange.lastCursor}.
            </div>
          )}
        </div>
      )}

      {activeExecutions?.loading && executions.length === 0 && (
        <p aria-live="polite">Loading active executions…</p>
      )}

      {activeExecutions?.error && executions.length === 0 && (
        <div className="target-error" role="alert">
          <strong>{activeExecutions.error.message}</strong>
        </div>
      )}

      {activeExecutions?.loaded && executions.length === 0 && !activeExecutions.loading && !activeExecutions.error && (
        <p className="empty-state">No active executions. New executions will appear here as they start.</p>
      )}

      {!activeExecutions && activities.length === 0 && !loading && !error && (
        <p className="empty-state">No activity yet. Events will appear here as they occur.</p>
      )}

      {executions.length > 0 && (
        <div className="live-execution-feeds" aria-label="Active execution activity feeds">
          {executions.map((execution) => {
            const executionActivities = activityBySession.get(execution.sessionId) ?? [];
            const headingId = `live-execution-${encodeURIComponent(execution.sessionId)}`;
            const observedAtMillis = activeExecutions?.observedAt
              ? Date.parse(activeExecutions.observedAt)
              : Number.NaN;
            const elapsedMillis = execution.elapsedMillis +
              (connected && Number.isFinite(observedAtMillis)
                ? Math.max(0, now - observedAtMillis)
                : 0);
            const detailPath = scopeBoundPath(
              `/active-executions/${encodeURIComponent(execution.sessionId)}`,
              activeExecutions?.targetScopeId,
            );

            return (
              <article
                key={execution.sessionId}
                className="live-execution-feed"
                aria-labelledby={headingId}
              >
                <header className="live-execution-feed-header">
                  <h3 id={headingId}>{execution.entrySkill}</h3>
                  <dl className="live-execution-feed-facts">
                    <div>
                      <dt>Started</dt>
                      <dd>{formatDateTime(execution.startedAt)}</dd>
                    </div>
                    <div>
                      <dt>Running</dt>
                      <dd>{formatElapsedMilliseconds(elapsedMillis)}</dd>
                    </div>
                  </dl>
                  <Link to={detailPath}>View active execution</Link>
                </header>
                <ActivityNarrative
                  activities={executionActivities}
                  isLive={connected}
                  alwaysFollow
                  compact
                  ariaLabel={`${execution.entrySkill} activity`}
                />
              </article>
            );
          })}
        </div>
      )}

      {recentCompletions.length > 0 && (
        <details className="recent-completions">
          <summary>Recent completions ({recentCompletions.length})</summary>
          <ol className="activity-list" aria-label="Recently completed executions">
            {recentCompletions.map((activity) => (
              <li key={`completion-${activity.cursor}`} className="activity-item">
                <span className="activity-kind" aria-label={ACTIVITY_KIND_LABELS[activity.kind as ActivityKind] ?? activity.kind}>
                  {ACTIVITY_KIND_LABELS[activity.kind as ActivityKind] ?? activity.kind}
                </span>
                <span className="activity-summary">{activity.summary}</span>
                <span className="activity-meta">
                  {activity.sessionId} · {activity.timestamp}
                </span>
              </li>
            ))}
          </ol>
        </details>
      )}
    </section>
  );
}
