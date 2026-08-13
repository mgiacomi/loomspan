import { Link, useParams } from "react-router";
import { TraceExplorer } from "./TraceExplorer";

export function ImportedTraceView() {
	const { traceId = "" } = useParams();
	return <section className="overview-card" aria-labelledby="imported-trace-title">
		<p className="eyebrow">Imported evidence</p>
		<h2 id="imported-trace-title">Imported trace</h2>
		<p><Link to="/trace-storage">Back to Trace Storage</Link></p>
		<p>This is transient evidence from a same-version trace file. The compatibility marker does not establish authenticity or provenance.</p>
		{traceId && <TraceExplorer traceId={traceId} source="IMPORTED" />}
	</section>;
}
