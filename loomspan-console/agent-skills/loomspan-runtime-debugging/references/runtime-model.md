# Runtime evidence model

`targetScopeId` identifies one selected target lifetime. Target rotation makes
its handles, target resources, and continuations stale. Imported evidence has
no authenticated target ownership and survives target rotation, but remains
current-process evidence.

Keep identities distinct: an application instance contains execution sessions;
a session records one trace; frames represent nested execution structure; and
records are ordered facts. Failure, model-attempt, retry-sequence, tool-call,
payload, artifact, and continuation identifiers link specific evidence rather
than establishing cause by themselves.

Active executions and recent activity are live, bounded observations. Cite
`observedAt`, the latest canonical sequence, continuity/reset facts, and
`beginningUnavailable`. A quiet window is not proof that an execution is stuck,
and a live outcome/path/usage conclusion is provisional. Finalized trace
evidence is stable within the installed artifact, but an incomplete artifact
does not imply an outcome.

Target catalog availability, a locally installed target copy, an imported
copy, current application authentication, and the original observation are
separate facts. A retained local copy can remain readable when new target
access requires authentication. A catalog item is not queryable until acquired.

Artifact handles, payload references, resources, and continuations are opaque
and transient. Removal, expiry, shutdown, or restart invalidates them; target
rotation additionally invalidates target-owned evidence. Continue only the same
operation and scope for which a continuation was returned.

Application-returned registered YAML is authoritative only as the running
application's supplied representation. Exact registered names and mapping IDs
can help search a checkout. `sourcePath` is descriptive untrusted text, not a
Console path, integrity assertion, or deployment-provenance fact.
