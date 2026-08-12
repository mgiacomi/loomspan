import { useEffect } from "react";
import { NavLink, Outlet } from "react-router";
import { ThemeSelect } from "./ThemeSelect";
import { PairingPage } from "../security/PairingPage";
import { useBrowserSession } from "../security/BrowserSessionProvider";
import { TargetProvider, useTarget } from "../target/TargetProvider";
import { ObservabilityProvider, useObservability } from "../observability/ObservabilityProvider";
import { ActivityProvider } from "../activity/ActivityProvider";
import type { InstanceStatus } from "../api/contracts";

export function App() {
  const session = useBrowserSession();
  return (
    <div className="app-frame">
      <header className="app-bar">
        <div className="app-bar-inner">
          <div className="brand">
            <span className="brand-mark" aria-hidden="true">ls</span>
            <span className="brand-titles">
              <h1>loomspan Console</h1>
              <span className="brand-tagline">Local developer tools</span>
            </span>
          </div>
          {session.status === "paired" && (
            <nav className="global-nav" aria-label="Console">
              <NavLink to="/" end>Overview</NavLink>
              <NavLink to="/target">Target</NavLink>
              <NavLink to="/skills">Skills</NavLink>
              <NavLink to="/active-executions">Active Executions</NavLink>
              <NavLink to="/traces">Traces</NavLink>
              <NavLink to="/trace-storage">Trace Storage</NavLink>
            </nav>
          )}
          <div className="app-bar-actions">
            <ThemeSelect />
          </div>
        </div>
      </header>
      <main className="shell-main" id="main-content">
        {session.status === "paired" ? (
          <TargetProvider initial={session.bootstrap.target} defaults={session.bootstrap.targetFormDefaults}>
            <ObservabilityProvider>
              <ActivityProvider>
                <ConsoleWorkspace />
              </ActivityProvider>
            </ObservabilityProvider>
          </TargetProvider>
        ) : (
          <PairingPage />
        )}
      </main>
      <footer className="shell-footer">
        {session.status === "paired" ? (
          <span className="workspace-path">
            Verified workspace <code>{session.bootstrap.workspacePath}</code>
          </span>
        ) : (
          <span />
        )}
        {session.status === "paired" ? <span className="build-meta">
          Console <code data-testid="console-version">{session.bootstrap.consoleVersion}</code>
        </span> : <span />}
      </footer>
    </div>
  );
}

function ConsoleWorkspace() {
  const { target } = useTarget();
  const { instance, loadInstance } = useObservability();
  const established =
    target.status.targetAuthentication === "ESTABLISHED" &&
    target.status.javaGoCompatibility === "COMPATIBLE";

  useEffect(() => {
    if (established && instance === null) void loadInstance();
  }, [established, instance, loadInstance]);

  const status = instance?.status as InstanceStatus | undefined;
  return (
    <>
      <aside className="global-context" aria-label="Current target and live context">
        <strong className="context-address">{target.address ?? "No target selected"}</strong>
        <ContextChip label="Connection" value={target.status.targetConnection} />
        <ContextChip label="Authentication" value={target.status.targetAuthentication} />
        <ContextChip label="Compatibility" value={target.status.javaGoCompatibility} />
        <ContextChip label="Runtime" value={target.status.runtimeIdentity} />
        <ContextChip
          label="Instance"
          value={status?.instanceId ?? target.status.instanceId ?? "Not established"}
        />
        {target.unencrypted && (
          <strong className="context-warning">Unencrypted target connection</strong>
        )}
        <NavLink className="context-link" to="/active-executions">
          Active executions: {status?.activeExecutionCount ?? "Unavailable"}
        </NavLink>
      </aside>
      <Outlet />
    </>
  );
}

const chipTones: Record<string, string> = {
  ESTABLISHED: "positive",
  COMPATIBLE: "positive",
  REACHABLE: "positive",
  AVAILABLE: "positive",
  SELECTED: "positive",
  INCOMPATIBLE: "negative",
  BLOCKED: "negative",
  UNAVAILABLE: "negative",
  REQUIRED: "caution",
  UNKNOWN: "caution",
  NOT_CHECKED: "caution",
};

function ContextChip({ label, value }: { label: string; value: string }) {
  return (
    <span className="context-chip" data-tone={chipTones[value] ?? "neutral"}>
      <span className="context-chip-label">{label}</span>
      {value}
    </span>
  );
}

export function NotFound() {
  return (
    <section aria-labelledby="not-found-title" className="foundation-card">
      <p className="eyebrow">Not found</p>
      <h2 id="not-found-title">This Console route does not exist</h2>
      <p>Check the address and return to the Console Overview.</p>
    </section>
  );
}
