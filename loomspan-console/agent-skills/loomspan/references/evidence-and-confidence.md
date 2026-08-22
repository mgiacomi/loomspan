# Evidence and confidence

Label recorded facts as **Evidence**, Console-derived arithmetic as
**Calculation**, developer or checkout information as **Context**, and a
reasoned explanation as **Inference**. Do not present an inference as a runtime
fact.

Cite the smallest stable identifiers that let the developer reconnect the
claim to evidence: session, trace, frame, record sequence, failure,
attempt/retry sequence, content reference, returned continuation, or exact
range. Include observation time and latest sequence for live claims. For
finalized evidence, identify and inspect the trace by `traceId`; do not request
or infer Console's internal owner, target scope, instance, or artifact handle.

Use direct limitation language: “the retained window begins after a gap,” “the
usage value is unavailable,” “raw inspection is not advertised,” or “the
recorded facts do not establish a root cause.” Missing is unknown, not zero.
Silence is not stuckness. An earlier error is not terminal unless the completion
links it. A workspace match does not establish deployment provenance.

Console calculations are mechanical facts such as duration, component-wise
usage, or a supported limit ratio. They are not monetary cost, correctness,
importance, diagnosis, or action recommendations. Do not sum overlapping
inclusive frame usage.

For live coverage, state exact cursor facts rather than a coverage label.
Global eviction alone does not say the selected session lost activity. An
observed session start, selected-session eviction, retained range, reset, and
returned range answer different questions. Omitted cursors remain omitted.
Observed usage zero means the counter has not accrued; a configured limit of
zero means that enforcement dimension is disabled or unlimited.

Application diagnostic content can contain secrets. Loomspan does not
secret-scan application YAML, paths, messages, tool inputs, diagnostics,
semantic content, or raw bytes. Authorized retrieval also does not control what the
client, model, or provider retains. Minimize disclosure and quote only what the
developer's question needs.
