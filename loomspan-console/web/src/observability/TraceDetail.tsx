import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useNavigate, useParams } from "react-router";
import {
  BrowserAPIError,
  acquireArtifact,
  getTraceDetail,
  rawArtifactDownloadURL,
} from "../api/client";
import type { Trace } from "../api/contracts";
import { useTarget } from "../target/TargetProvider";
import { useBrowserSession } from "../security/BrowserSessionProvider";
import {
  recoverObservabilityError,
  requireCurrentTargetScope,
} from "./scope";
import { useScopeBoundRoute } from "./useScopeBoundRoute";
import { TraceExplorer } from "./TraceExplorer";
import { TraceSummary } from "./TraceSummary";

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
  const heading = useRef<HTMLHeadingElement>(null);
  const refreshTarget = useRef(refresh);
  refreshTarget.current = refresh;
  const routeIsCurrent = useScopeBoundRoute();

  useEffect(() => {
    heading.current?.focus();
  }, []);

  const handleArtifactUnavailable = useCallback((artifactError: BrowserAPIError) => {
    setTrace((current) => current ? { ...current, localAvailable: false } : current);
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
    try {
      await acquireArtifact(traceId, security);
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
          <TraceSummary trace={trace} action={<a className="trace-download-link" href={rawArtifactDownloadURL(trace.traceId)} download>Download Trace</a>} />

          {!trace.localAvailable && <div className="trace-analysis-unavailable">
            <p>Analysis is not available locally.</p>
            <p>
              <button
                type="button"
                onClick={() => void handleAcquire()}
                disabled={acquiring}
              >
                {acquiring ? "Acquiring…" : "Acquire for analysis"}
              </button>
            </p>

            {acquireError && (
              <div className="target-error" role="alert">
                <strong>{acquireError.message}</strong>
                {acquireError.details?.rawDownloadAvailable && <p>The analysis copy was rejected, but the trace remains available through Download Trace above.</p>}
              </div>
            )}
          </div>}
          {trace.localAvailable && <TraceExplorer traceId={trace.traceId} onArtifactUnavailable={handleArtifactUnavailable} />}
        </>
      )}
    </section>
  );
}
