# Runtime evidence model

Keep identities distinct: an application instance contains execution sessions;
a session records one trace; frames represent nested execution structure; and
records are ordered facts. Failure, model-attempt, retry-sequence, tool-call,
content, and continuation identifiers link specific evidence rather than
establishing cause by themselves.

Active executions and recent activity are live, bounded observations. Cite
`observedAt`, the latest canonical sequence, continuity/reset facts, and
`beginningUnavailable`. A quiet window is not proof that an execution is stuck,
and a live outcome/path/usage conclusion is provisional. Finalized trace
evidence is stable while Console can resolve its installed evidence, but an
incomplete artifact does not imply an outcome.

For finalized traces, the model-facing identity is `traceId`. Console resolves
installed target evidence, imported evidence, or safe target acquisition
internally. A unique imported trace remains inspectable without a selected
target. A known collision is reported as `AMBIGUOUS_TRACE`; Console never
silently prefers one owner. Discovery may return useful candidates with
`complete: false` when uniqueness or absence could not be established.

Target generations, evidence owners, installed handles, acquisition,
single-flight, expiry, leases, and capacity remain internal safety mechanisms.
The MCP client does not supply or compare them. `TRACE_UNAVAILABLE` is the
domain-level result when safe reuse or acquisition cannot provide evidence.

Content references and continuations are opaque, transient, and bound to their
content or query in the current Console process. On a stale continuation,
restart the query by `traceId`. On a stale content reference, re-query the
relevant record by `traceId` and use the refreshed descriptor. On
`TARGET_CHANGED`, restart the operation by `traceId`.

Application-returned registered YAML is authoritative only as the running
application's supplied representation. Exact registered names and mapping IDs
can help search a checkout. `sourcePath` is descriptive untrusted text, not a
Console path, integrity assertion, or deployment-provenance fact.
