import type { ConfiguredLimits, TraceAnalysisSummary, TraceFrame, TraceUsage as Usage, TraceUsageValue } from "../api/contracts";

function UsageRows({ rows }: { rows: Array<[string, TraceUsageValue | undefined, boolean | undefined]> }) {
  return <tbody>{rows.map(([kind, value, complete]) => <tr key={kind}><th scope="row">{kind}{complete === false && <span> (incomplete)</span>}</th><td>{value?.promptUnits ?? "unavailable"}</td><td>{value?.completionUnits ?? "unavailable"}</td><td>{value?.totalUnits ?? "unavailable"}</td></tr>)}</tbody>;
}

function percentage(numerator: number, denominator: number) {
  if (denominator === 0) return undefined;
  return `${Number(((numerator / denominator) * 100).toFixed(2))}%`;
}

function LimitRow({ label, value, limit }: { label: string; value?: number; limit: number }) {
  const proportion = value == null ? undefined : percentage(value, limit);
  return <tr><th scope="row">{label}</th><td>{value ?? "unavailable"}</td><td>{limit}</td><td>{value == null ? "unavailable" : limit === 0 ? "undefined" : proportion}</td></tr>;
}

function Limits({ limits, summary }: { limits: ConfiguredLimits | null; summary: TraceAnalysisSummary }) {
  if (!limits) return <p>Configured limit comparison unavailable.</p>;
  return <div className="observability-table-region" role="region" aria-label="Configured limit comparison" tabIndex={0}><table>
    <caption>Run-start configured limits and arithmetic proportions</caption>
    <thead><tr><th>Counter</th><th>Observed</th><th>Configured limit</th><th>Proportion</th></tr></thead>
    <tbody>
      <LimitRow label="Skill invocations" limit={limits.maxSkillInvocations} />
      <LimitRow label="Tool invocations" limit={limits.maxToolInvocations} />
      <LimitRow label="Linter retries" limit={limits.maxLinterRetries} />
      <LimitRow label="Model calls" limit={limits.maxModelCalls} />
      <LimitRow label="Provider attempts" value={summary.attemptCount} limit={limits.maxProviderAttempts} />
      <LimitRow label="Usage units" value={summary.terminalUsage.totalUnits} limit={limits.maxUsageUnits} />
    </tbody>
  </table></div>;
}

export function TraceUsage({ usage, frame, summary, contributors = [], onSelectFrame }: { usage?: Usage; frame?: TraceFrame; summary?: TraceAnalysisSummary; contributors?: TraceFrame[]; onSelectFrame?: (frameId: string) => void }) {
  if (!usage) return <p role="status">Loading usage…</p>;
  const rows: Array<[string, TraceUsageValue | undefined, boolean | undefined]> = frame ? [
    ["Selected frame direct", frame.directUsage, frame.directUsageComplete],
    ["Selected frame descendants", frame.descendantUsage, frame.descendantUsageComplete],
    ["Selected frame inclusive", frame.inclusiveUsage, frame.inclusiveUsageComplete],
  ] : [
    ["Attributed", usage.attributed, undefined], ["Unattributed", usage.unattributed, undefined],
    ["Unframed attributed", usage.unframedAttributed, undefined], ["Terminal", usage.terminal, undefined],
  ];
  return <><table aria-label="Usage facts"><caption>{frame ? `Usage for ${frame.route || frame.frameId}` : "Trace usage breakdown"}</caption><thead><tr><th>Kind</th><th>Prompt</th><th>Completion</th><th>Total</th></tr></thead><UsageRows rows={rows} /></table>
    {summary && <Limits limits={summary.configuredLimits} summary={summary} />}
    <p>Monetary cost is not calculated because this trace contains no canonical monetary value.</p>
    {contributors.some((contributor) => Boolean(contributor.inclusiveUsage)) && <div className="observability-table-region" role="region" aria-label="Usage-ordered frame contributors" tabIndex={0}><table><caption>Frames ordered by inclusive usage</caption><thead><tr><th>Frame</th><th>Path</th><th>Direct</th><th>Descendants</th><th>Inclusive</th></tr></thead><tbody>{contributors.filter((contributor) => contributor.inclusiveUsage).map((contributor) => <tr key={contributor.frameId}><th scope="row"><button type="button" onClick={() => onSelectFrame?.(contributor.frameId)}>{contributor.frameId}</button></th><td>{contributor.route || "unavailable"}</td><td>{contributor.directUsage?.totalUnits ?? "unavailable"}</td><td>{contributor.descendantUsage?.totalUnits ?? "unavailable"}</td><td>{contributor.inclusiveUsage.totalUnits}{contributor.inclusiveUsageComplete ? "" : " (incomplete)"}</td></tr>)}</tbody></table></div>}
  </>;
}
