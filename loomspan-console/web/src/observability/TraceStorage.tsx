import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useNavigate } from "react-router";
import {
  BrowserAPIError,
  clearAllUnusedArtifacts,
  clearExpiredArtifacts,
  getStorageSnapshot,
	importTraceFile,
  removeArtifact,
} from "../api/client";
import type { StorageSnapshot } from "../api/contracts";
import { useBrowserSession } from "../security/BrowserSessionProvider";

export function TraceStorage() {
  const session = useBrowserSession();
	const navigate = useNavigate();
  const [snapshot, setSnapshot] = useState<StorageSnapshot | null>(null);
  const [error, setError] = useState<BrowserAPIError | null>(null);
  const [loading, setLoading] = useState(true);
  const [actionError, setActionError] = useState<BrowserAPIError | null>(null);
	const [confirmRemove, setConfirmRemove] = useState<string | null>(null);
	const [selectedFile, setSelectedFile] = useState<File | null>(null);
	const [importing, setImporting] = useState(false);
  const [confirmClear, setConfirmClear] = useState<"expired" | "all-unused" | null>(null);
  const heading = useRef<HTMLHeadingElement>(null);

  useEffect(() => {
    heading.current?.focus();
  }, []);

  const loadSnapshot = useCallback(async () => {
    const security = session.getSecurity();
    if (!security) {
      setError(new BrowserAPIError("SESSION_REQUIRED", "Pairing is required.", 401));
      setLoading(false);
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const result = await getStorageSnapshot(security);
      setSnapshot(result);
    } catch (err) {
		setError(err instanceof BrowserAPIError ? err : new BrowserAPIError("CONSOLE_ERROR", "Storage could not be loaded.", 500));
    } finally {
      setLoading(false);
    }
  }, [session]);

  useEffect(() => {
    void loadSnapshot();
  }, [loadSnapshot]);

  const handleRemove = useCallback(async (traceId: string, source: "TARGET" | "IMPORTED") => {
    const security = session.getSecurity();
    if (!security) return;
    setActionError(null);
    try {
		await removeArtifact(traceId, source, security);
      setConfirmRemove(null);
      await loadSnapshot();
    } catch (err) {
		setActionError(err instanceof BrowserAPIError ? err : new BrowserAPIError("CONSOLE_ERROR", "The artifact could not be removed.", 500));
    }
  }, [session, loadSnapshot]);

  const handleClearExpired = useCallback(async () => {
    const security = session.getSecurity();
    if (!security) return;
    setActionError(null);
    try {
      await clearExpiredArtifacts(security);
      setConfirmClear(null);
      await loadSnapshot();
    } catch (err) {
		setActionError(err instanceof BrowserAPIError ? err : new BrowserAPIError("CONSOLE_ERROR", "Artifacts could not be cleared.", 500));
    }
  }, [session, loadSnapshot]);

  const handleClearAllUnused = useCallback(async () => {
    const security = session.getSecurity();
    if (!security) return;
    setActionError(null);
    try {
      await clearAllUnusedArtifacts(security);
      setConfirmClear(null);
      await loadSnapshot();
    } catch (err) {
		setActionError(err instanceof BrowserAPIError ? err : new BrowserAPIError("CONSOLE_ERROR", "Artifacts could not be cleared.", 500));
    }
  }, [session, loadSnapshot]);

	const handleImport = useCallback(async () => {
		const security = session.getSecurity();
		if (!security || !selectedFile) return;
		setImporting(true);
		setActionError(null);
		try {
			const result = await importTraceFile(selectedFile, security);
			navigate(`/traces/imported/${encodeURIComponent(result.traceId)}`);
		} catch (err) {
			setActionError(err instanceof BrowserAPIError ? err : new BrowserAPIError("CONSOLE_ERROR", "The trace file could not be opened.", 500));
		} finally {
			setImporting(false);
		}
	}, [navigate, selectedFile, session]);

  return (
    <section aria-labelledby="trace-storage-title" className="overview-card">
      <p className="eyebrow">Artifact cache</p>
      <h2 id="trace-storage-title" ref={heading} tabIndex={-1}>Trace Storage</h2>

	  <div className="trace-import">
		<p>Trace files may contain sensitive diagnostics and application paths. Only complete files from this exact Loomspan version can be opened.</p>
		<label htmlFor="trace-file">Trace file</label>
		<input id="trace-file" type="file" accept="application/x-ndjson,.ndjson" onChange={(event) => setSelectedFile(event.target.files?.[0] ?? null)} />
		<button type="button" disabled={!selectedFile || importing} onClick={() => void handleImport()}>{importing ? "Opening trace file…" : "Open trace file"}</button>
	  </div>

      <p>
        <Link to="/traces">Back to Trace Catalog</Link>
      </p>

      {error && (
        <div className="target-error" role="alert">
          <strong>{error.message}</strong>
        </div>
      )}

      {actionError && (
        <div className="target-error" role="alert">
          <strong>{actionError.message}</strong>
		  {actionError.details?.expectedCompatibilityVersion && actionError.details?.observedCompatibilityVersion && (
			<p>Expected {actionError.details.expectedCompatibilityVersion}; observed {actionError.details.observedCompatibilityVersion}.</p>
		  )}
        </div>
      )}

      {loading && <p>Loading storage snapshot…</p>}

      {snapshot && (
        <>
          <dl className="status-grid">
            <div><dt>Workspace</dt><dd>{snapshot.workspaceLabel}</dd></div>
            <div><dt>Maximum bytes</dt><dd>{snapshot.unlimited ? "Unlimited" : String(snapshot.maxBytes)}</dd></div>
            <div><dt>Idle TTL</dt><dd>{snapshot.neverExpire ? "Never" : snapshot.idleTtl}</dd></div>
            <div><dt>Charged bytes</dt><dd>{String(snapshot.chargedBytes)}</dd></div>
            <div><dt>Acquired count</dt><dd>{String(snapshot.acquiredCount)}</dd></div>
          </dl>

          <div className="storage-actions">
            {confirmClear === "expired" ? (
              <>
                <button
                  type="button"
                  onClick={() => void handleClearExpired()}
                  aria-label="Confirm clearing expired artifacts"
                >
                  Confirm clear expired
                </button>
                <button type="button" onClick={() => setConfirmClear(null)}>
                  Cancel
                </button>
              </>
            ) : confirmClear === "all-unused" ? (
              <>
                <button
                  type="button"
                  onClick={() => void handleClearAllUnused()}
                  aria-label="Confirm clearing all unused artifacts"
                >
                  Confirm clear all unused
                </button>
                <button type="button" onClick={() => setConfirmClear(null)}>
                  Cancel
                </button>
              </>
            ) : (
              <>
                <button type="button" onClick={() => setConfirmClear("expired")}>
                  Clear expired
                </button>
                <button type="button" onClick={() => setConfirmClear("all-unused")}>
                  Clear all unused
                </button>
              </>
            )}
          </div>

          {snapshot.entries.length === 0 ? (
            <p>No artifacts are currently stored.</p>
          ) : (
            <table className="storage-table">
              <thead>
                <tr>
				  <th scope="col">Trace ID</th>
				  <th scope="col">Source</th>
                  <th scope="col">Session ID</th>
                  <th scope="col">Outcome</th>
                  <th scope="col">Local bytes</th>
                  <th scope="col">App availability at acquisition</th>
                  <th scope="col">Local</th>
                  <th scope="col">Active pin</th>
                  <th scope="col">Acquired at</th>
                  <th scope="col">Last used</th>
                  <th scope="col">Expires at</th>
                  <th scope="col">Actions</th>
                </tr>
              </thead>
              <tbody>
                {snapshot.entries.map((entry) => (
				  <tr key={`${entry.source}:${entry.traceId}`}>
                    <td>
					  <Link to={entry.source === "IMPORTED" ? `/traces/imported/${encodeURIComponent(entry.traceId)}` : `/traces/${encodeURIComponent(entry.traceId)}`}>
                        {entry.traceId}
                      </Link>
					</td>
					<td>{entry.source === "IMPORTED" ? "Imported" : "Target"}</td>
                    <td>{entry.sessionId}</td>
                    <td>{entry.outcome}</td>
                    <td>{String(entry.localBytes)}</td>
					<td>{entry.applicationAvailability ?? "Not applicable"}</td>
                    <td>{entry.localAvailable ? "Yes" : "No"}</td>
                    <td>{entry.activePin ? "Yes" : "No"}</td>
                    <td>{entry.acquiredAt}</td>
                    <td>{entry.lastUsedAt}</td>
                    <td>{entry.hasIdleExpiry ? entry.expiresAt : "Never"}</td>
                    <td>
                      {entry.activePin ? (
                        <span aria-label="Cannot remove: artifact is in use">In use</span>
					  ) : confirmRemove === `${entry.source}:${entry.traceId}` ? (
                        <>
                          <button
                            type="button"
							onClick={() => void handleRemove(entry.traceId, entry.source)}
                            aria-label={`Confirm removal of ${entry.traceId}`}
                          >
                            Confirm
                          </button>
                          <button
                            type="button"
                            onClick={() => setConfirmRemove(null)}
                          >
                            Cancel
                          </button>
                        </>
                      ) : (
                        <button
                          type="button"
						  onClick={() => setConfirmRemove(`${entry.source}:${entry.traceId}`)}
                        >
                          Remove
                        </button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </>
      )}
    </section>
  );
}
