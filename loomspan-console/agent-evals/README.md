# Agent evaluation harness

This directory contains repository release evidence for the portable
`loomspan-runtime-debugging` Agent Skill. Cases link to existing authoritative
fixtures rather than copying trace NDJSON. Records are sanitized development
evidence, not a Loomspan runtime interchange format.

PR-28 cases cover primary-plan and accepted-attempt lineage, failure before
acceptance, tool/model/structured semantic content, positive and complete
negative literal search, imported-time discovery, and compact large-trace
orientation. PR-30 cases cover the one-send provider-attempt lifecycle, typed
read-timeout evidence, failed-step failure joins, and page-local descriptor
resolution for repeated content. They forbid ordinary raw reads for semantic
questions and unsafe conclusions from incomplete discovery or search work.
PR-31 adds paired tools-only and skill-assisted cases for actual-retry
cardinality, lifecycle times, COMPACT/DETAILED projection, physical record-type
histogram discovery, semantic content selection, and complete continuation
traversal. Results are committed only after actual client executions.
The paired cases use the same sanitized current-checkout trace under
`fixtures/`; it is replaced when an intentional pre-1.0 format change requires
it rather than retained as a historical compatibility promise. Synthetic tests
remain responsible for exact boundary and malformed-trace coverage.

From `loomspan-console/`, start one isolated production-adapter MCP case:

```text
go run ./internal/buildtool agent-eval serve --case CASE_ID --output TEMP_DIR
```

The protected `TEMP_DIR/session.json` contains the temporary loopback endpoint
and key. Do not commit, print, screenshot, or pass that key on a command line.
Install the canonical skill by an explicit copy or filesystem link into the
client's temporary user/global skill location, then configure the MCP endpoint
and header through that client's protected mechanism.

Use a fresh conversation and the case's exact `developerPrompt`. Capture the
client's native event stream, client/model builds, MCP calls as hashes plus
stable evidence references, and the unedited final answer. The client-event
JSON uses schema version 1 and supplies the record metadata, classified events,
operations, and completed human rubric. It must not contain headers, keys,
absolute machine paths, unrelated repository content, or full payloads.

```text
go run ./internal/buildtool agent-eval record --session TEMP_DIR --client-events CLIENT_EVENTS.json --answer ANSWER.txt --output RECORD.json
go run ./internal/buildtool agent-eval score --record RECORD.json
go run ./internal/buildtool agent-eval summarize --results agent-evals/results/DATE
```

`record` fails closed on incomplete headless event visibility or sensitive
content. `score` checks fixture facts, forbidden claims/actions, and every human
rubric threshold without requiring exact prose or call order. `summarize`
requires and preserves the selected 28-run Codex CLI/Claude Code matrix; a
completed unfavorable run cannot be dropped or replaced. Infrastructure
failures are retained separately and rerun, while completed model failures
remain release failures.

MCP-without-skill and protocol/service degradation are deterministic or single
observation coverage rather than repeated model diagnoses. GUI rows remain
`Not run` when no executable local build is available. Adversarial results are
defense-in-depth observations only; they do not claim Console controls client
tools, model behavior, or provider retention.
