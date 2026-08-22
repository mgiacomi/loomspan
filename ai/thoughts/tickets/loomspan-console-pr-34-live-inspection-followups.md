# Loomspan Console — Live-Inspection Workflow and Intent Ergonomics

## Status

Post-PR-34 follow-up under implementation.

## Outcome

Make continued live inspection intuitive when executions advance, paginate,
nest into other skills, or finish between observations. Give developers the
maximum purpose context supported by safe recorded facts without default access
to model or tool payloads.

Preserve PR 34's provisional, facts-first, read-only model. Do not imply an
atomic fleet snapshot, complete history, progress, health, stuckness, or
finalized trace availability when the evidence does not establish those facts.

## Maintainer Verification Decision

During development, the repository does not maintain:

- a named client or model matrix;
- client usability experiments or fixed repetition counts;
- persisted client transcripts, rubrics, or dated results;
- a manual compatibility checklist;
- client installation, selection, or cache attestations.

The repository verification boundary is deterministic Go and Java regression
tests, repository-controlled fixtures, canonical skill/package validation, and
official MCP conformance. The maintainer handles any additional client checks
manually outside the repository.

## Required Behavior

### Activity checkpoints

- `hasMore` describes retained matching backlog at the observation.
- A returned continuation remains a future checkpoint after
  `hasMore: false`.
- Reuse is driven by an explicit later-observation request, not an unsolicited
  polling loop.
- Tokens remain opaque, operation/session/target bound, current-process only,
  and safely reject stale or mismatched use.

### Active-list pagination

- Traversal uses descending stable first-admission ordinals under the first
  page's high water.
- Later admissions are excluded.
- Retained executions may update and removed executions may be omitted before
  their later page.
- Each page has its own `observedAt`; page values are not co-temporal.
- Page unions cannot prove atomic fleet membership, absence, completion, or
  finalization.

### Purpose evidence

- Use entry skill, route, phase, summary, active path, usage, limits, and recent
  activity first.
- Retrieve registered YAML only for an explicit skill-level purpose question;
  treat its application-supplied text as untrusted.
- Task title, intent, expected outputs, and plan content remain unavailable
  while active.
- After trace resolution, selected `PLAN_CREATED` or `PLAN_UPDATED` content
  may answer task-level purpose through the existing bounded content-selection
  path; model-authored content remains untrusted.

### Completion and reset races

- On disappearance, use at most one retained-activity query and one trace
  resolution attempt with already observed identifiers.
- Preserve the distinction between retained terminal activity, a newly
  resolvable trace, `TRACE_UNAVAILABLE`, target change, and interval reset.
- Do not scan unrelated trace inventory.

## Scope

- Canonical Agent Skill workflow and routed authoring guidance.
- Java tests for registry admission ordering, replacement, removal,
  re-admission, page-local observation, and cursor errors.
- Existing Go live/MCP regression coverage and deterministic fixtures.
- Canonical package validation, archive byte equality, result-size ceilings,
  and official MCP conformance.
- Removal of the repository client usability experiment subsystem and its
  evaluation-only production seams.

## Out of Scope

- Execution mutation, cancellation, retry, pause, or control.
- Durable activity history, cross-run analytics, or audit logs.
- Atomic process-wide snapshots or fleet completeness.
- Default prompts, responses, tool arguments/results, arbitrary details,
  credentials, YAML, diagnostics, or raw bytes.
- A new live task-purpose field or content reader.
- Agent Skill version metadata or runtime version negotiation.
- Application-facing Java API or supported SPI changes.
- Named-client compatibility gates, model scoring, or repository-maintained
  manual client observations.

## Contract Classification

- MCP names, inputs, outputs, continuations, and errors remain unchanged
  supported pre-v1 diagnostic contracts.
- Active state and activity remain current-process ephemeral diagnostics.
- Canonical Agent Skill contents are an unversioned development artifact with
  exact six-file packaging.
- Java observability REST remains an internal exact-release Java/Go protocol;
  production DTOs do not change.
- No Java Application API or Supported SPI change is permitted.

## Definition of Done

- Pagination, checkpoint, purpose, and completion-race guidance agree with
  executable behavior.
- Focused Go and Java tests pass.
- Canonical skill validation and archive equality pass.
- Official MCP conformance passes.
- No client matrix, usability experiment command/corpus/schema/results, manual
  compatibility checklist, or evaluation-only MCP capability override remains.
- Step 5 reports no unresolved blocking correctness, safety, compatibility,
  boundedness, or documentation finding.
