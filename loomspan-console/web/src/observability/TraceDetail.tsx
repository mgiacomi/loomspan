import { useCallback, useEffect, useRef, useState, type KeyboardEvent } from "react";
import { Link, useNavigate, useParams } from "react-router";
import {
  BrowserAPIError,
  acquireArtifact,
  getTraceDetail,
  rawArtifactDownloadURL,
} from "../api/client";
import type { AcquiredArtifact, Trace } from "../api/contracts";
import { useTarget } from "../target/TargetProvider";
import { useBrowserSession } from "../security/BrowserSessionProvider";
import {
  recoverObservabilityError,
  requireCurrentTargetScope,
} from "./scope";
import { useScopeBoundRoute } from "./useScopeBoundRoute";
import { TraceExplorer } from "./TraceExplorer";
import { TraceSummary } from "./TraceSummary";
import { formatDateTime } from "../activity/activityPresentation";

export function TraceDetailView() {
  const { traceId } = useParams();
  const navigate = useNavigate();
  const { target, scopeGeneration, refresh } = useTarget();
  const session = useBrowserSession();
  const [trace, setTrace] = useState<Trace | null>(null);
  const [error, setError] = useState<BrowserAPIError | null>(null);
  const [loading, setLoading] = useState(true);
  const [acquiring, setAcquiring] = useState(false);
  const [acquireError, setAcquireError] = useState<BrowserAPIError | null>(null);
  const [acquired, setAcquired] = useState<AcquiredArtifact | null>(null);
  const [confirmDownload, setConfirmDownload] = useState(false);
  const heading = useRef<HTMLHeadingElement>(null);
  const cancelDownload = useRef<HTMLButtonElement>(null);
  const downloadTrigger = useRef<HTMLButtonElement>(null);
  const downloadDialog = useRef<HTMLDivElement>(null);
  const refreshTarget = useRef(refresh);
  refreshTarget.current = refresh;
  const routeIsCurrent = useScopeBoundRoute();

  useEffect(() => {
    heading.current?.focus();
  }, []);

  useEffect(() => {
    if (confirmDownload) cancelDownload.current?.focus();
  }, [confirmDownload]);

  const closeDownload = () => {
    setConfirmDownload(false);
    requestAnimationFrame(() => downloadTrigger.current?.focus());
  };

  const handleDialogKey = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key === "Escape") {
      event.preventDefault();
      closeDownload();
      return;
    }
    if (event.key !== "Tab") return;
    const controls = [...(downloadDialog.current?.querySelectorAll<HTMLElement>('a[href], button:not([disabled])') ?? [])];
    if (controls.length === 0) return;
    const first = controls[0];
    const last = controls[controls.length - 1];
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault();
      first.focus();
    }
  };

  const handleArtifactUnavailable = useCallback((artifactError: BrowserAPIError) => {
    setTrace((current) => current ? { ...current, localAvailable: false } : current);
    setAcquired(null);
    setAcquireError(artifactError);
    const targetScopeID = target.status.targetScopeId;
    navigate(`/traces/${encodeURIComponent(traceId ?? "")}${targetScopeID ? `?targetScopeId=${encodeURIComponent(targetScopeID)}` : ""}`, { replace: true });
  }, [navigate, target.status.targetScopeId, traceId]);

  useEffect(() => {
    if (!traceId || !routeIsCurrent) return;
    let cancelled = false;
    setLoading(true);
    setError(null);
    setTrace(null);
    getTraceDetail(traceId)
      .then(async (t) => {
        await requireCurrentTargetScope(t.targetScopeId, target.status.targetScopeId, refreshTarget.current);
        if (!cancelled) setTrace(t);
      })
      .catch(async (err) => {
        const recovered = await recoverObservabilityError(err, refreshTarget.current);
        if (cancelled) return;
        setError(recovered);
      })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [routeIsCurrent, traceId, scopeGeneration, target.status.targetScopeId]);

  const handleAcquire = async () => {
    if (!traceId) return;
    const security = session.getSecurity();
    if (!security) {
      setAcquireError(new BrowserAPIError("SESSION_REQUIRED", "Pairing is required.", 401));
      return;
    }
    setAcquiring(true);
    setAcquireError(null);
    setAcquired(null);
    try {
      const result = await acquireArtifact(traceId, security);
      setAcquired(result);
      setTrace((current) => current ? { ...current, localAvailable: true } : current);
      navigate(`/traces/${encodeURIComponent(traceId)}?targetScopeId=${encodeURIComponent(target.status.targetScopeId ?? "")}`);
    } catch (err) {
      const recovered = await recoverObservabilityError(err, refreshTarget.current);
      setAcquireError(recovered);
    } finally {
      setAcquiring(false);
    }
  };

  return (
    <section aria-labelledby="trace-detail-title" className="overview-card">
      <p className="eyebrow">Operational views</p>
      <h2 id="trace-detail-title" ref={heading} tabIndex={-1}>Trace Detail</h2>

      <p>
        <Link to="/traces">Back to Trace Catalog</Link>
      </p>

      {error && (
        <div className="target-error" role="alert">
          <strong>{error.message}</strong>
        </div>
      )}

      {loading && <p>Loading trace detail…</p>}

      {trace && (
        <>
          <TraceSummary trace={trace} />

          <div className="trace-actions">
            <h3>Artifact actions</h3>
            <dl className="status-grid" aria-label="Artifact state">
              <div><dt>Local artifact</dt><dd>{trace.localAvailable ? "Available" : "Not installed"}</dd></div>
              <div><dt>Size (bytes)</dt><dd>{String(trace.sizeBytes)}</dd></div>
              <div><dt>Application availability at acquisition</dt><dd>{trace.applicationAvailability ?? "Not observed locally"}</dd></div>
            </dl>
            {!trace.localAvailable && <p>
              <button
                type="button"
                onClick={() => void handleAcquire()}
                disabled={acquiring}
              >
                {acquiring ? "Acquiring…" : "Acquire for analysis"}
              </button>
            </p>}
            <p>
              <button ref={downloadTrigger} type="button" onClick={() => setConfirmDownload(true)}>Download raw attachment</button>
            </p>
            <p className="trace-actions-note">
              Acquire installs a local analysis copy. Raw download streams the
              artifact directly from the application without installing or
              extending a local copy.
            </p>

            {acquireError && (
              <div className="target-error" role="alert">
                <strong>{acquireError.message}</strong>
              </div>
            )}

            {acquired && (
              <div role="status">
                <p>Artifact acquired successfully.</p>
                <dl className="status-grid">
                  <div><dt>Local bytes</dt><dd>{String(acquired.localBytes)}</dd></div>
                  <div><dt>Acquired at</dt><dd>{formatDateTime(acquired.acquiredAt)}</dd></div>
                  <div><dt>Expires at</dt><dd>{acquired.hasIdleExpiry ? formatDateTime(acquired.expiresAt) : "Never"}</dd></div>
                </dl>
              </div>
            )}
            {trace.localAvailable && <p><button type="button" onClick={() => navigate(`/traces/${encodeURIComponent(trace.traceId)}?targetScopeId=${encodeURIComponent(target.status.targetScopeId ?? "")}`)}>Open explorer</button></p>}
          </div>
          {trace.localAvailable && <TraceExplorer traceId={trace.traceId} onArtifactUnavailable={handleArtifactUnavailable} />}
          {confirmDownload && <div ref={downloadDialog} role="dialog" aria-modal="true" aria-labelledby="download-title" className="target-error" onKeyDown={handleDialogKey}><h3 id="download-title">Download raw attachment?</h3><p>This makes a fresh application download and may be unavailable even while the local analysis copy remains available. It does not install, refresh, or retain an analysis copy.</p><p><a href={rawArtifactDownloadURL(trace.traceId)} download onClick={closeDownload}>Confirm raw attachment download</a> <button ref={cancelDownload} type="button" onClick={closeDownload}>Cancel</button></p></div>}
        </>
      )}
    </section>
  );
}
