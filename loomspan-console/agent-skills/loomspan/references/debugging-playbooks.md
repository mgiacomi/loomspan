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

## WF-ACTIVE-EXECUTION-REVIEW

After runtime discovery, list only the bounded page or traversal the developer
requested, up to 64 items per page. A traversal is descending stable
first-admission ordinal under the first page's high water: later admissions are
excluded, retained executions may carry updated snapshots, removed executions
may be omitted, and every page is independently observed. Merge identities in
returned traversal order and attach each value to that page's `observedAt`.
The union is not an atomic fleet or complete membership view and cannot support
absence, finalization, or co-temporal comparisons. Do not call detail merely to
probe fields.

Make one bounded activity call per selected session and retain every
continuation even when `hasMore` is false. If the developer asks for a second
observation, reuse each retained checkpoint once and compare recorded
sequences, cursors, timestamps, usage, and activity facts. An empty result may
advance the checkpoint. Do not loop after an empty observation or keep polling
unless the developer requests monitoring.

Treat global eviction, selected-session start, selected-session eviction, and
selected-session retained range as separate cursor facts. Missing facts stay
missing; never turn them into complete, incomplete, unknown, healthy, stuck, or
progress states. If an execution disappears, query retained activity by its
already returned `sessionId`, then try trace resolution once by its already
returned `traceId`. Preserve `TRACE_UNAVAILABLE`; disappearance does not prove
finalized evidence and never justifies scanning unrelated trace inventory.

For an explicit purpose question, use a least-disclosing ladder. Start with
neutral `entrySkill`, route/path, phase, producer summary, usage, limits, and
activity. If skill-level purpose remains unanswered, retrieve only the exact
registered skill already named by live evidence and quote only the needed YAML
description, labeled application-supplied untrusted context. Task title,
intent, and expected outputs are not live facts. After trace resolution, a
task-level question may use narrowed `PLAN_CREATED`/`PLAN_UPDATED` descriptors
and selected content; label that model-authored content untrusted. If those
paths do not establish active task intent, say it is unknown.

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
