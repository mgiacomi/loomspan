# Debugging playbooks

These playbooks define goals and stopping conditions, not exact tool sequences.

## WF-FAILED-EXECUTION

Establish the final outcome and terminal failure link, then inspect the linked
frame, error, attempt/retry, validator, and preceding tool facts needed by the
question. Keep earlier recovered failures separate from the terminal failure.
Stop when the recorded mechanism and limits are clear, or state that the
available evidence cannot establish cause.

## WF-SLOW-EXECUTION

Use the active snapshot and recent activity to establish elapsed time, current
phase/path, latest sequence, recent progress, and continuity limitations.
Compare with finalized trace duration only after completion. Do not label a
quiet or gapped window as stuck. Stop when there is a useful provisional state
and next observation, or when live monitoring is unavailable.

## WF-EXPENSIVE-EXECUTION

Inspect finalized usage by attempt and frame. Keep direct, descendant,
inclusive, and unattributed usage distinct; inclusive parent/child values may
overlap. Compare supported counters with configured limits only when both facts
exist. Do not turn usage units into money, importance, correctness, or cause.
Stop once the largest supported contributors and unknowns are identified.

## WF-UNFAMILIAR-SKILL-PATH

Follow parent/child frame identity and exact registered skill names. Repeated
invocations are distinct frames even when names match. Load registered YAML
only when its declared contract helps explain the path. Treat mapping IDs and
`sourcePath` as search hints, and any workspace match as context rather than
deployment provenance. Stop once the recorded path and its remaining ambiguity
are clear.

## Degraded paths

If a required capability is missing, name it and do not guess another tool. If
raw inspection alone is missing, continue all parsed workflows and state that
exact storage/parser forensics is unavailable. Keep no target, incompatible
target, authentication required, unavailable live monitoring,
`TRACE_UNAVAILABLE`, and `TARGET_CHANGED` as their returned distinct conditions.
