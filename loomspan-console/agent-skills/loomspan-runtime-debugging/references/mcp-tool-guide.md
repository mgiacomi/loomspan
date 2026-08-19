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
LOOMSPAN_list_traces -> LOOMSPAN_get_trace -> compact frames -> record descriptors/search -> selected content read
```

- `LOOMSPAN_list_traces` filters by `sources`, outcome, exact entry skill,
  session, and independent finalized/acquired/imported time windows. Choose
  `FINALIZED_DESC`, `ACQUIRED_DESC`, or `IMPORTED_DESC` for the question. It
  returns one compact candidate per trace ID. `hasMore`
  means another page exists; `complete` means every expected evidence family
  was checked. Do not make a negative or uniqueness conclusion when
  `complete` is false; preserve its compact limitation.
- `LOOMSPAN_get_trace` resolves one unique available trace and returns its
  parsed summary.
- `LOOMSPAN_query_trace_frames` defaults to `COMPACT`; request `DETAILED` only
  for rich usage, duration, retry, validation, failure, gap, or uncertainty
  evidence.
- `LOOMSPAN_query_trace_records` returns content descriptors by default.
  `inlineContent` is bounded per value and across the page, so omission is not
  absence. With `filter.literalText` it returns compact case-sensitive matches
  plus fields, representation, coverage, work-completion, and limitations.
- Select plan chains by the primary root plus framework `planId`, order versions
  by record sequence, and use only recorded creation `attemptId` and
  `retrySequenceId` for acceptance. Never choose by route, first match, or
  model-authored IDs.
- `LOOMSPAN_read_trace_content` reads an exact bounded selected semantic value
  using its returned opaque `contentRef`.
- `LOOMSPAN_read_trace_artifact` optionally reads exact bounded raw source bytes
  for storage/parser forensics.

Every trace inspection tool requires `traceId` plus only question-specific
filters, pagination, projection, representation, content-reference, or range controls.
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
| stale/invalid content reference | Re-query the relevant record by `traceId`, then use its refreshed descriptor. |
| `TRACE_DISCOVERY_INCOMPLETE` limitation | Preserve returned candidates but avoid unsafe negative or uniqueness conclusions. |

A zero-match literal page proves absence only when search work is complete and
coverage limitations do not exclude relevant content. Imported evidence is not
authenticated provenance. `acquiredAt`, `importedAt`, and `finalizedAt` are
independent facts and must not substitute for one another.

Protocol negotiation or HTTP authentication is not a Loomspan domain error.
Missing capability is not `INCOMPATIBLE_TARGET`. Target authentication is not
evidence unavailability. Missing `loomspan.raw-artifact-inspection.v1` removes
only exact raw forensics; parsed trace inspection can continue.
