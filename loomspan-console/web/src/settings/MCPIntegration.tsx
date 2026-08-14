import { useCallback, useEffect, useRef, useState } from "react";
import {
  BrowserAPIError,
  disableMCP,
  enableMCP,
  getMCPStatus,
  regenerateMCP,
  removeInvalidMCP,
  revealMCP,
} from "../api/client";
import type { MCPStatus } from "../api/contracts";
import { useBrowserSession } from "../security/BrowserSessionProvider";

type Confirmation = "regenerate" | "disable" | "remove-invalid" | null;

export function MCPIntegration() {
  const session = useBrowserSession();
  const [status, setStatus] = useState<MCPStatus | null>(null);
  const [credential, setCredential] = useState<string | null>(null);
  const [confirmation, setConfirmation] = useState<Confirmation>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const heading = useRef<HTMLHeadingElement>(null);

  useEffect(() => { heading.current?.focus(); }, []);
  useEffect(() => {
    let current = true;
    void getMCPStatus().then((value) => { if (current) setStatus(value); }).catch(() => { if (current) setError("MCP status could not be loaded."); });
    return () => { current = false; setCredential(null); };
  }, []);

  const run = useCallback(async (operation: "enable" | "reveal" | "regenerate" | "disable" | "remove-invalid") => {
    const security = session.getSecurity();
    if (!security) { setError("Pairing is required."); return; }
    setBusy(true); setError(null); setCredential(null);
    try {
      const result = operation === "enable" ? await enableMCP(security)
        : operation === "reveal" ? await revealMCP(security)
        : operation === "regenerate" ? await regenerateMCP(security)
        : operation === "disable" ? await disableMCP(security)
        : await removeInvalidMCP(security);
      setStatus(result);
      if ("credential" in result && typeof result.credential === "string") setCredential(result.credential);
      setConfirmation(null);
    } catch (caught) {
      setError(caught instanceof BrowserAPIError ? caught.message : "The MCP operation could not be completed.");
    } finally { setBusy(false); }
  }, [session]);

  return (
    <section aria-labelledby="mcp-title" className="overview-card">
      <p className="eyebrow">Settings</p>
      <h2 id="mcp-title" ref={heading} tabIndex={-1}>MCP Integration</h2>
      <div className="target-warning" role="note">
        MCP clients can send Loomspan diagnostic data to their configured model provider. Review that provider's data-handling policy before enabling access.
      </div>
      {error && <div className="target-error" role="alert"><strong>{error}</strong></div>}
      {!status && !error && <p>Loading MCP status…</p>}
      {status && <>
        <dl className="status-grid">
          <div><dt>State</dt><dd>{status.state === "DISABLED_INVALID" ? "Disabled — invalid key file" : status.state === "ENABLED" ? "Enabled" : "Disabled"}</dd></div>
          <div><dt>Endpoint</dt><dd><code>{status.endpoint}</code></dd></div>
        </dl>
        {status.diagnostic && <p role="status">{status.diagnostic}</p>}
        <div className="storage-actions">
          {status.state === "DISABLED" && <button type="button" disabled={busy} onClick={() => void run("enable")}>Enable MCP</button>}
          {status.state === "ENABLED" && <>
            <button type="button" disabled={busy} onClick={() => void run("reveal")}>Reveal access key</button>
            <button type="button" disabled={busy} onClick={() => { setCredential(null); setConfirmation("regenerate"); }}>Regenerate key</button>
            <button type="button" disabled={busy} onClick={() => { setCredential(null); setConfirmation("disable"); }}>Disable MCP</button>
          </>}
          {status.state === "DISABLED_INVALID" && <button type="button" disabled={busy} onClick={() => setConfirmation("remove-invalid")}>Remove invalid key file</button>}
        </div>
        {confirmation === "regenerate" && <Confirm text="Regenerating disconnects clients and makes every old configuration fail." action="Confirm regenerate" onConfirm={() => void run("regenerate")} onCancel={() => setConfirmation(null)} />}
        {confirmation === "disable" && <Confirm text="Disabling disconnects clients and removes the persistent access key." action="Confirm disable" onConfirm={() => void run("disable")} onCancel={() => setConfirmation(null)} />}
        {confirmation === "remove-invalid" && <Confirm text="Remove the invalid canonical key file without revealing its contents?" action="Confirm removal" onConfirm={() => void run("remove-invalid")} onCancel={() => setConfirmation(null)} />}
        {credential && <div className="credential-reveal" role="status">
          <strong>Access key</strong>
          <code>{credential}</code>
          <button type="button" onClick={() => void navigator.clipboard.writeText(credential).catch(() => setError("The access key could not be copied."))}>Copy access key</button>
          <button type="button" onClick={() => setCredential(null)}>Hide access key</button>
        </div>}
        <h3>Client setup</h3>
        <p>Use user or global client settings. Never put this key in a repository, URL, or shell command.</p>
        <div className="mcp-setup-grid">{status.setup.map((entry) => <article key={entry.client}><h4>{entry.client}</h4><p><strong>{entry.scope}</strong> scope</p><p>{entry.guidance}</p><code>Authorization: Bearer &lt;LOOMSPAN_MCP_ACCESS_KEY&gt;</code></article>)}</div>
      </>}
    </section>
  );
}

function Confirm({ text, action, onConfirm, onCancel }: { text: string; action: string; onConfirm(): void; onCancel(): void }) {
  return <div role="alertdialog" aria-label={action}><p>{text}</p><button type="button" onClick={onConfirm}>{action}</button><button type="button" onClick={onCancel}>Cancel</button></div>;
}
