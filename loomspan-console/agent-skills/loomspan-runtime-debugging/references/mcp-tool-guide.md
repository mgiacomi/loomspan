# MCP tool guide

Bootstrap with `LOOMSPAN_get_runtime`.

Target skill evidence:

- `LOOMSPAN_list_skills` lists registered names and descriptive source paths.
- `LOOMSPAN_get_skill` returns one application's unchanged registered YAML.

Live execution evidence:

- `LOOMSPAN_list_executions` lists bounded provisional execution summaries.
- `LOOMSPAN_get_execution` returns one provisional active execution.
- `LOOMSPAN_get_execution_activity` returns bounded ordered recent activity
  with observation and continuity facts.

Finalized trace evidence follows one workflow:

```text
LOOMSPAN_list_traces -> select traceId -> inspect/query/read by traceId
```

- `LOOMSPAN_list_traces` returns one compact candidate per trace ID. `hasMore`
  means another page exists; `complete` means every expected evidence family
  was checked. Do not make a negative or uniqueness conclusion when
  `complete` is false; preserve its compact limitation.
- `LOOMSPAN_get_trace` resolves one unique available trace and returns its
  parsed summary.
- `LOOMSPAN_query_trace_frames` and `LOOMSPAN_query_trace_records` page parsed
  structural and ordered facts.
- `LOOMSPAN_read_trace_payload` reads an exact bounded reconstructed payload or
  diagnostic range using a returned opaque `payloadRef`.
- `LOOMSPAN_read_trace_artifact` optionally reads exact bounded raw source bytes
  for storage/parser forensics.

Every trace inspection tool requires `traceId` plus only question-specific
filters, pagination, representation, payload-reference, or range controls.
Page sizes and ranges are bounded. A continuation is opaque and belongs only
to its query. Continue while `hasMore` is true; never infer authority or
identity from an opaque token. Tools are the complete MCP investigation path;
no custom Loomspan resources are advertised.

Preserve exact domain errors and recovery:

| Code or condition | Recovery |
| --- | --- |
| `AMBIGUOUS_TRACE` | Resolve the conflicting trace identity in Console; never choose an evidence owner silently. |
| `TRACE_UNAVAILABLE` | Retry inspection by `traceId` after evidence or target availability changes. |
| `TARGET_CHANGED` | Restart the operation by `traceId`. |
| stale/invalid continuation | Restart the same query by `traceId`. |
| stale/invalid payload reference | Re-query the relevant record by `traceId`, then use its refreshed descriptor. |
| `TRACE_DISCOVERY_INCOMPLETE` limitation | Preserve returned candidates but avoid unsafe negative or uniqueness conclusions. |

Protocol negotiation or HTTP authentication is not a Loomspan domain error.
Missing capability is not `INCOMPATIBLE_TARGET`. Target authentication is not
evidence unavailability. Missing `loomspan.raw-artifact-inspection.v1` removes
only exact raw forensics; parsed trace inspection can continue.
