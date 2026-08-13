import { Link } from "react-router";
import type { ConfiguredLimits, TraceAnalysisSummary, TraceFrame, TraceRecord, TraceUsage as Usage, TraceUsageValue } from "../api/contracts";

function UsageRows({ rows }: { rows: Array<[string, TraceUsageValue | undefined, boolean | undefined]> }) {
  return <tbody>{rows.map(([kind, value, complete]) => <tr key={kind}><th scope="row">{kind}{complete === false && <span> (incomplete)</span>}</th><td>{value?.promptUnits ?? "unavailable"}</td><td>{value?.completionUnits ?? "unavailable"}</td><td>{value?.totalUnits ?? "unavailable"}</td></tr>)}</tbody>;
}

function percentage(numerator: number, denominator: number) {
  if (denominator === 0) return undefined;
  return `${Math.round((numerator / denominator) * 100)}%`;
}

function operationName(frame: TraceFrame) {
  if (!frame.route) return frame.frameType || "Unknown operation";
  const [entry, detail] = frame.route.split("#", 2);
  if (!detail) return entry;
  const parts = detail.split("-");
  const readableParts: string[] = [];
  for (let index = 0; index < parts.length; index++) {
    if (parts[index] === "step" && /^\d+$/.test(parts[index + 1] ?? "")) {
      readableParts.push(`step ${parts[++index]}`);
    } else {
      readableParts.push(parts[index]);
    }
  }
  return `${entry} · ${readableParts.join(" · ")}`;
}

function operationLabels(contributors: TraceFrame[]) {
  const groups = new Map<string, TraceFrame[]>();
  for (const contributor of contributors) {
    const name = operationName(contributor);
    groups.set(name, [...(groups.get(name) ?? []), contributor]);
  }
  const labels = new Map<string, string>();
  for (const [name, group] of groups) {
    const chronological = group.toSorted((left, right) => left.openedTimestampMillis - right.openedTimestampMillis || left.frameId.localeCompare(right.frameId));
    chronological.forEach((contributor, index) => {
      labels.set(contributor.frameId, group.length === 1 ? name : `${name} · call ${index + 1} of ${group.length}`);
    });
  }
  return labels;
}

function LimitRow({ label, value, limit }: { label: string; value: number; limit: number }) {
  return <tr><th scope="row">{label}</th><td>{value}</td><td>{limit}</td><td>{limit === 0 ? "undefined" : percentage(value, limit)}</td></tr>;
}

function Limits({ limits, summary }: { limits: ConfiguredLimits | null; summary: TraceAnalysisSummary }) {
  if (!limits) return <p>Configured limit comparison unavailable.</p>;
  return <div className="observability-table-region" role="region" aria-label="Configured limit comparison" tabIndex={0}><table>
    <caption>Observed usage against run-start limits; only counters with authoritative trace totals are shown</caption>
    <thead><tr><th>Counter</th><th>Observed</th><th>Configured limit</th><th>Proportion</th></tr></thead>
    <tbody>
      <LimitRow label="Provider attempts" value={summary.attemptCount} limit={limits.maxProviderAttempts} />
      <LimitRow label="Usage units" value={summary.terminalUsage.totalUnits} limit={limits.maxUsageUnits} />
    </tbody>
  </table></div>;
}

export function TraceUsage({ usage, frame, summary, contributors = [], responseRecords = [], recordHref }: { usage?: Usage; frame?: TraceFrame; summary?: TraceAnalysisSummary; contributors?: TraceFrame[]; responseRecords?: TraceRecord[]; recordHref?: (record: TraceRecord) => string }) {
  if (!usage) return <p role="status">Loading usage…</p>;
  const directContributors = contributors
    .filter((contributor) => contributor.directUsage.totalUnits > 0)
    .toSorted((left, right) => right.directUsage.totalUnits - left.directUsage.totalUnits || left.frameId.localeCompare(right.frameId));
  const contributorLabels = operationLabels(directContributors);
  const responseByFrame = new Map<string, TraceRecord | undefined>();
  for (const record of responseRecords.toSorted((left, right) => left.sequence - right.sequence)) {
    if (!record.frameId) continue;
    responseByFrame.set(record.frameId, responseByFrame.has(record.frameId) ? undefined : record);
  }
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
    {directContributors.length > 0 && <section className="trace-usage-contributors" aria-labelledby="trace-usage-contributors-title"><h4 id="trace-usage-contributors-title">Where model usage went</h4><p>Each row is usage attributed directly to one operation. Rows do not overlap and can be compared or added together. Repeated operations are numbered in execution order. Select an operation to open its model response in Records. Share is measured against the trace total; visible shares may total less than 100% when usage was unframed or unattributed.</p><div className="observability-table-region" tabIndex={0}><table className="trace-usage-contributor-table"><caption>Operations with directly attributed model usage</caption><thead><tr><th>Operation</th><th>Prompt</th><th>Completion</th><th>Total</th><th>Share of trace</th></tr></thead><tbody>{directContributors.map((contributor) => {
      const response = responseByFrame.get(contributor.frameId);
      const label = contributorLabels.get(contributor.frameId);
      return <tr key={contributor.frameId}><th scope="row">{response && recordHref ? <Link to={recordHref(response)}>{label}</Link> : label}</th><td>{contributor.directUsage.promptUnits}</td><td>{contributor.directUsage.completionUnits}</td><td>{contributor.directUsage.totalUnits}{contributor.directUsageComplete ? "" : " (incomplete)"}</td><td>{percentage(contributor.directUsage.totalUnits, usage.terminal.totalUnits) ?? "undefined"}</td></tr>;
    })}</tbody></table></div></section>}
  </>;
}
