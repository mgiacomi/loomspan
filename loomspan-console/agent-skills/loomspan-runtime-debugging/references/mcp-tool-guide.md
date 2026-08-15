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

Trace evidence:

- `LOOMSPAN_list_traces` lists target catalog or imported installed evidence.
- `LOOMSPAN_get_trace` acquires or opens one trace and returns its summary.
- `LOOMSPAN_query_trace_frames` pages structural frame facts.
- `LOOMSPAN_query_trace_records` pages enriched ordered record facts.
- `LOOMSPAN_read_trace_payload` reads an exact bounded reconstructed payload or
  diagnostic range.
- `LOOMSPAN_read_trace_artifact` optionally reads exact bounded raw source bytes
  for storage/parser forensics.

Respect each schema's required identity and source fields. Page sizes are
bounded. A continuation belongs only to the operation, scope, installed copy,
and any session filter that produced it. Payload and raw reads use returned
source-byte offsets and lengths; continue while `hasMore` is true. Never infer
an application cursor or authority from an opaque token.

Tools provide the complete portable investigation surface. Resources can
present unchanged YAML or parsed summary/frame/record JSON, but are optional
and raw artifacts have no resource form.

Preserve the exact error class. Protocol negotiation or HTTP authentication is
not a Loomspan domain error. Missing capability is not `INCOMPATIBLE_TARGET`.
Target authentication is not evidence expiry. `TARGET_CHANGED` invalidates the
old target scope instead of authorizing remapping. `ARTIFACT_EXPIRED` and other
availability results describe the installed evidence lifecycle.

Missing `loomspan.raw-artifact-inspection.v1` removes only the last tool and
exact raw forensics. Parsed trace inspection can continue. Likewise, retained
parsed evidence can remain inspectable even when a new application acquisition
returns `TARGET_AUTHENTICATION_REQUIRED`.
