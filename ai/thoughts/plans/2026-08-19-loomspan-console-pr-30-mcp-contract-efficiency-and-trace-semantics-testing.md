# PR 30 — MCP Contract Efficiency and Trace Semantic Corrections Test Plan

## Change Summary

PR 30 changes one MCP discovery contract and five current-version trace semantics as a coordinated Java-to-Go-to-browser/UI update:

- advertise compact output schemas for all 12 MCP tools while validating every structured result against both the compact advertised schema and the complete generated result schema;
- reduce the exact serialized `tools/list` response from the current 37,788-byte baseline to no more than 20 KiB;
- classify the supplied OpenAI/OkHttp read-timeout causal chain as transient `TIMEOUT` without misclassifying cancellation, interruption, generic I/O, or an HTTP/2 `CANCEL` alone;
- replace contradictory failed `STEP_COMPLETED` records with `STEP_FAILED`, while keeping successful completion and caller-owned abort semantics distinct;
- remove `MODEL_REQUEST_PREPARED` so one physical provider attempt begins with exactly one `MODEL_REQUEST_SENT`;
- derive and persist stable `traceRootFrameId`, `missionFrameId`, and creation-owned `planningFrameId` for all records in a plan lineage, rejecting malformed lineage as `INVALID_PLAN_LINEAGE`;
- normalize literal-search results to page-local `contentId` values and one page-level `{contentId, contentRef}` descriptor per unique content value without changing offsets, work bounds, completeness, or continuation behavior; and
- make MCP text fallbacks deterministic for optional values and prove that Go pointer addresses cannot leak.

The primary implementation plan is `ai/thoughts/plans/2026-08-19-loomspan-console-pr-30-mcp-contract-efficiency-and-trace-semantics.md`. The behavioral evidence is the ticket, research report, and supplied trace `ai/thoughts/research/6777e217-03af-4a7d-bc2a-c59798fb8f36..ndjson`. The trace contains seven duplicated prepared/sent request pairs, the OpenAI wrapper → `InterruptedIOException("timeout")` → HTTP/2 `CANCEL` failure chain, a failed `STEP_COMPLETED`, and plan creation/update records on different frames.

This is a test plan only. It does not add production code, tests, or regenerate fixtures.

## Impacted Areas

| Area | Behavior under test | Primary test seams |
| --- | --- | --- |
| MCP tool registration and discovery | Compact explicit output schemas; complete internal result validation; exactly 12 tools; exact discovery bytes; ≤20 KiB budget | `loomspan-console/internal/mcpadapter/output_schemas_test.go` (new), `server_test.go`, `contracts_test.go`, `trace_contracts_test.go` |
| Provider failure translation | Exact read-timeout chain becomes transient `TIMEOUT`; cancellation/interruption and unrelated I/O remain non-timeout | `SpringAiProviderIntegrationTest` and advisor integration tests |
| Java trace production | One sent event per attempt; truthful step terminal; stable failure link; no prepared event | `ModelAttemptCallAdvisorIntegrationTest`, `StepLoopMissionExecutionEngineTest`, trace/projection tests |
| Go trace processor | Sent-first attempt state machine; closed vocabulary; plan-lineage validation and persisted landmarks | `calculations_test.go`, `processor_test.go`, new plan-focused tests |
| Literal search | Page-local IDs, deduplicated descriptors, unchanged offsets and continuations, explicit complete negative evidence | `search_test.go`, `continuation_test.go`, adapter tests |
| MCP/browser adapters | Identical neutral plan/search semantics; ordinary record pages do not grow search-only fields | MCP trace tests, browser trace-analysis tests, joined-adapter tests |
| Live and frontend presentation | `STEP_FAILED` is error evidence; `STEP_COMPLETED` is success only; prepared presentation is removed; descriptor lookup drives exact reads | live coordinator/service tests, TypeScript contract/component tests, selected E2E workflows |
| Cross-language fixture boundary | Java writer and Go reader agree on current exact release, record vocabulary, facts, failures, plans, and search evidence | `ConsoleTraceFixtureCorpusTest`, Go `fixture_corpus_test.go`, browser fixture contracts |
| Agent-facing debugging contract | Packaged skill/evals teach one-send attempts, truthful step failures, stable plan IDs, and `contentId → descriptor → contentRef → read` | Agent Skill validation, evaluation cases/harness, source audit |
| Supported compatibility surface | No supported Java API/SPI/configuration expansion or accidental internal type exposure | `LoomspanPublicSurfaceArchitectureTest`, existing provider-policy/config tests |

## Risk Assessment

### High risk

1. **MCP schemas become small by becoming inaccurate.** A compact advertised schema can accept less than the real result, omit decision-critical navigation fields, or cease to enforce the exclusive success/domain-error envelope. The test suite must separately prove advertised-schema conformance and complete generated-schema conformance, including a value that passes the compact schema but fails the complete schema.
2. **Timeout classification broadens into cancellation retry.** `InterruptedIOException` is used for more than timeouts, and OkHttp `CANCEL` can be caller-driven. Positive and negative causal-chain cases must be table-driven and must include thread interruption and nested cancellation causes.
3. **Step terminals become duplicated or misleading.** Failure may occur in the model call, validation loop, tool call, final-response path, or caller cleanup. Tests must enforce one terminal classification per started step rather than only checking that `STEP_FAILED` exists somewhere.
4. **Plan identity remains query-frame-dependent.** Creation and update records deliberately occur on different frames. Tests must compare their returned landmark triples and cover primary, nested, shared-root, and malformed lineages.
5. **Search normalization changes pagination.** Descriptor construction must be downstream of match selection. Tests must retain every offset and the same match-level cursor boundaries while deduplicating only serialized references.

### Medium risk

1. **Java and Go vocabulary drift.** The generated fixture corpus is the executable language boundary; hand-edited NDJSON could hide writer/reader disagreement.
2. **Adapters re-derive facts differently.** MCP and browser should map the same trace-analysis result. Joined-adapter assertions should compare semantic fields, descriptor relations, completeness, and continuations.
3. **Frontend follows page-local IDs as opaque refs.** A `contentId` must never be submitted to exact-content read endpoints; only its descriptor's `contentRef` is valid.
4. **Fallback formatting leaks implementation details.** A few pointer-valued fields are enough to emit `0xc...`. Test all optional field kinds and apply a response-wide address-pattern assertion.
5. **Stale prepared vocabulary survives in fixtures, filters, docs, or evaluations.** A repository audit is required in addition to compile-time enum changes.

### Low risk but compatibility-sensitive

- No supported Java API or SPI is intended to change. The architecture allowlist test remains an acceptance gate.
- Existing provider retry property names, defaults, and validation rules are unchanged; current configuration tests should remain green.
- The MCP/browser JSON and NDJSON trace vocabulary are unreleased/current-version contracts. Their deliberate breaks are tested by removing old positive expectations and adding current-format rejection tests, not by accepting both forms.
- The exact release-derived `consoleCompatibilityVersion` remains unchanged by this PR. Existing mismatch tests must continue rejecting every non-exact value.

## Existing Test Coverage

### Java producer and projections

- `SpringAiProviderIntegrationTest` already exercises typed Google/provider failures but lacks the supplied OpenAI read-timeout chain and the required cancellation/interruption negatives.
- `ModelAttemptCallAdvisorIntegrationTest` covers retry sequence identity, transient provider failures, exhausted attempts, quotas, backoff interruption, and provider throws. Its current `recordsPreparedAndSentButNoResponseWhenProviderThrows` assertion encodes the obsolete duplicate lifecycle and must be replaced.
- `StepLoopMissionExecutionEngineTest` covers tool failure, model failure, validation retry/exhaustion, timeouts, successful tool/final-response paths, and caller cleanup. Current failure assertions do not enforce the new one-of step-terminal contract.
- `LiveActivityProjectorTest`, `ExecutionJournalProjectorTest`, and `ExecutionJournalProjectionContractTest` cover visible record kinds and error/journal presentation. They need the new failed-step fact and removal of prepared-request assumptions.
- `ExecutionTraceContractTest` and `ExecutionTraceBoundaryCleanupTest` cover event order, duplicate outer model events, plan frame ownership, failure sanitization, and cleanup. Current trace cardinality still includes prepared events.
- `ConsoleTraceFixtureCorpusTest` verifies producer-generated NDJSON/expected files byte-for-byte, exact compatibility markers, frame and plan invariants, and tool lifecycle cardinality.
- `LoomspanPublicSurfaceArchitectureTest` is the executable authority for the closed supported Java API and absence of a supported SPI.

### Go analysis, adapters, and live service

- `internal/traceanalysis/calculations_test.go` has a mature attempt-lifecycle matrix: ordered/out-of-order events, explicit identities, missing response, gaps, and rejection cases. It currently models prepared as the initial attempt state.
- `processor_test.go` covers malformed traces, cancellation, exact release compatibility, and dev/release version matrices.
- `fixture_corpus_test.go` consumes the Java corpus and asserts cross-language semantic facts including attempts, plan evidence, and canonical tool lifecycle.
- `search_test.go` and `continuation_test.go` cover offsets, filters, binary exclusions, work completeness, no-match pages, KMP boundary matches, page-size continuation, cursor binding, and finite reachability. They do not cover descriptor normalization.
- `internal/mcpadapter/server_test.go` performs a real protocol initialize/list/call path and exact tool inventory, but currently blesses 37,788 bytes against an unrelated 64 KiB ceiling.
- MCP `contracts_test.go`, `trace_contracts_test.go`, and `traces_test.go` cover exclusive envelopes, strict inputs, enums, bounds, fallback budgets, and trace navigation.
- `trace_joined_adapters_test.go` and browser `trace_analysis_test.go` already exercise shared neutral analysis and opaque continuation mapping.
- Live coordinator/service tests reject unknown activity kinds and enumerate all currently valid kinds; their expected count and labels need updating.

### TypeScript/UI and workflows

- API contracts, activity presentation, `TraceRecords.*` component tests, `TraceExplorer.test.tsx`, and `TraceViews.test.tsx` cover record-specific rendering, deliberate content loading, failure focus, search continuation, and returned hierarchy facts.
- `TraceRecords.model.test.tsx` has a direct obsolete test named `presents a prepared request inline...`.
- `TraceRecords.results.test.tsx` covers successful/failed completion receipts and terminal error links but not a distinct `STEP_FAILED` record.
- `artifact-storage.spec.ts` covers failed traces, nested retries, chunked/multi-megabyte content, literal inspection, and current-version acquisition; `portable-trace-import.spec.ts` covers exact-version acceptance/rejection.

### Coverage to preserve without duplication

- Input schemas, opaque cursors, request range bounds, artifact security, and credential handling are already well covered and should not be restated in every PR 30 test.
- This PR should extend the existing focused tables and fixture corpus rather than create a second end-to-end harness.
- Race testing is conditional. It is required only if implementation introduces shared mutable registration/schema caches or other concurrently mutated validation state.

## Bug Reproduction / Failing Test First

Tests should be introduced in small red/green slices. Each slice should fail for the observed behavior, not merely because a planned type does not compile. Where a new DTO field is needed, first assert serialized JSON through `map[string]any` or an adapter response so the initial failure is a missing/wrong value; add strongly typed compile-time assertions when the DTO changes.

### Red 1 — discovery exceeds the PR budget

- Change the existing real `tools/list` protocol test to assert `len(serializedResponse) <= 20 << 10` while preserving the 12-name inventory and strict input checks.
- Expected current failure: 37,788 bytes exceeds 20,480.
- Do not commit a new exact snapshot until the compact schemas are implemented and reviewed; then make the snapshot the exact regression oracle.

### Red 2 — supplied read timeout is classified as unknown

- Add the exact causal shape from the trace to `SpringAiProviderIntegrationTest`: OpenAI response wrapper with `InterruptedIOException("timeout")`, with a nested/associated OkHttp HTTP/2 `CANCEL` detail.
- Assert failure kind `TIMEOUT`, transient retry classification, safe message, and preservation of the full stack only as diagnostic evidence.
- Expected current failure: the translator returns `UNKNOWN`/`DO_NOT_RETRY`.

### Red 3 — a failed step has no truthful terminal

- Extend the smallest existing tool-failure engine test to collect records for one step and assert exactly one `STEP_FAILED`, zero `STEP_COMPLETED`, and a stable failure identity shared with the useful recorded error.
- Expected current failure: the trace contains `STEP_COMPLETED` with failed metadata and no `STEP_FAILED`.

### Red 4 — one physical attempt emits two request records

- Replace `recordsPreparedAndSentButNoResponseWhenProviderThrows` with a sent-boundary assertion: one `MODEL_REQUEST_SENT`, no prepared kind, and one failure/terminal fact for the same attempt.
- Expected current failure: the advisor emits both prepared and sent.

### Red 5 — plan facts change identity on update

- Feed a valid create-on-`PLANNING`, update-on-`ROOT_MISSION` trace to the Go analysis API and decode the two serialized plan facts generically.
- Assert identical `traceRootFrameId`, `missionFrameId`, and `planningFrameId` for the same `planId` and absence of legacy `rootFrameId`.
- Expected current failure: update-derived root/planning identities differ and the new fields are absent.

### Red 6 — repeated matches repeat opaque references

- Use one content value with at least three literal matches and a `pageSize` that returns them together. Assert three offsets, one descriptor, and all matches pointing to the same page-local `contentId`.
- Expected current failure: every match carries the same full `contentRef`, and no page descriptor exists.

### Red 7 — fallback text exposes pointer addresses

- Serialize an inventory/frame fallback with non-nil optional strings, timestamps, and duration/parent fields and reject `0x[0-9a-fA-F]+` anywhere in the text.
- Expected current failure: at least one `%v`-formatted pointer address appears.

These focused red tests should be run and observed individually before the corresponding production slice. They should not be merged in a permanently failing state.

## Tests to Add or Update

Each entry names the test, type, location, proof, data/mocks, contract classification, and compatibility expectation required by the testing-plan workflow.

### 1. MCP compact schema registration and discovery

#### `TestRegisteredToolsValidateCompactAndCompleteOutputSchemas`

- **Type:** Go unit/contract test, table-driven across all 12 registrations.
- **Location:** new `loomspan-console/internal/mcpadapter/output_schemas_test.go`.
- **What it proves:** every installed tool has an explicit compact output schema; a representative success and domain-error envelope validates against it; the same serialized typed output validates against the fully generated concrete `Out` schema; registration does not silently fall back to SDK-inferred full discovery.
- **Fixtures:** minimal valid result/error values for runtime, skill, execution, activity, trace inventory, trace analysis, and content-range result families. Reuse existing builders where possible.
- **Mocks:** typed handlers returning deterministic in-memory DTOs; no HTTP or trace service.
- **Contract classification:** unreleased MCP serialized contract plus internal validation invariant.
- **Compatibility expectation:** deliberate in-place discovery break; structured JSON behavior remains coherent. No legacy schema path.

#### `TestCompleteOutputValidationRejectsFieldsHiddenByCompactDiscovery`

- **Type:** Go negative unit test.
- **Location:** new `output_schemas_test.go`.
- **What it proves:** the Loomspan wrapper performs complete validation independently of the SDK compact check. A malformed typed/result serialization that satisfies the deliberately open/compact advertised shape but violates the resolved complete schema must fail as a server defect before a response is returned.
- **Fixtures:** a test-only output type with an advertised compact subset and an invalid required/full-schema field; include one valid control.
- **Mocks:** test handler only.
- **Contract classification:** internal correctness guard protecting the advertised-vs-runtime split.
- **Compatibility expectation:** no external compatibility behavior; failure must not be converted to a Loomspan domain-error envelope.

#### `TestCompactEnvelopeRequiresExactlyOneResultOrError`

- **Type:** Go schema unit test.
- **Location:** extend `contracts_test.go` or new `output_schemas_test.go`.
- **What it proves:** the compact schema accepts success-only and error-only envelopes and rejects neither, both, or extra top-level properties; advertised error decision fields retain their real JSON names/types.
- **Fixtures:** four JSON objects plus representative domain-error details.
- **Mocks:** none.
- **Contract classification:** MCP response contract.
- **Compatibility expectation:** preserves the existing exclusive-envelope behavior while changing its advertised size.

#### `TestToolsListMatchesCompactSnapshotAndBudget`

- **Type:** Go protocol integration/snapshot test.
- **Location:** update `internal/mcpadapter/server_test.go`; add `internal/mcpadapter/testdata/tools-list-response.json`.
- **What it proves:** a real compatible MCP initialize/list flow exposes exactly 12 tools, exact names/descriptions/input/output schemas, a byte-for-byte reviewed wire response, an explicitly recorded exact byte count, and total serialized size ≤20 KiB.
- **Fixtures:** committed LF-normalized JSON snapshot.
- **Mocks:** existing real in-process server/runtime test dependencies.
- **Contract classification:** MCP discovery wire contract and performance budget.
- **Compatibility expectation:** replaces the 37,788-byte baseline; input schemas and inventory remain protected, output schema representation intentionally breaks in place.

#### `TestCompactSchemasRetainDecisionAndNavigationFields`

- **Type:** Go table-driven schema inspection test.
- **Location:** `trace_contracts_test.go` plus schema-specific tests.
- **What it proves:** each compact schema still advertises fields needed to select the next tool: identity/status, availability/completeness, pagination/continuation, frame/record identity, plan landmarks, search descriptors, exact range offsets/encoding, and domain-error decisions.
- **Fixtures:** expected required-property paths by tool, not a duplicate full DTO snapshot.
- **Mocks:** none.
- **Contract classification:** MCP authoring/usability contract.
- **Compatibility expectation:** protects useful semantics while allowing intentionally omitted descriptive/full-detail fields.

### 2. Provider timeout classification and retry facts

#### `translatesOpenAiInterruptedReadTimeoutAsTransientTimeout`

- **Type:** Java unit/integration test of the provider translator.
- **Location:** `SpringAiProviderIntegrationTest`.
- **What it proves:** the supplied OpenAI wrapper → `InterruptedIOException("timeout")` shape is classified `TIMEOUT` and transient; safe evidence is typed and diagnostic stack content is not promoted into ordinary metadata.
- **Fixtures:** construct the closest available OpenAI exception/wrapper classes and cause chain matching the supplied trace.
- **Mocks:** no network; use exception instances and current translator entry point.
- **Contract classification:** internal provider integration with observable trace semantics.
- **Compatibility expectation:** intentional behavioral correction; existing retry configuration decides the retry after classification.

#### `doesNotTreatCancellationOrGenericInterruptedIoAsTimeout`

- **Type:** Java table-driven negative test.
- **Location:** `SpringAiProviderIntegrationTest`.
- **What it proves:** timeout classification is denied for `CancellationException`, any causal `InterruptedException`, an already-interrupted thread, blank/non-timeout `InterruptedIOException`, isolated HTTP/2 `CANCEL`, and unrelated `IOException`; `SocketTimeoutException` remains a positive control.
- **Fixtures:** one exception chain per subcase; restore the test thread's interrupt flag in `finally`.
- **Mocks:** none.
- **Contract classification:** internal classification safety boundary.
- **Compatibility expectation:** protects caller cancellation and existing typed classifications from retry broadening.

#### `recordsTypedTimeoutAndPolicyDerivedRetryForOpenAiReadFailure`

- **Type:** Java advisor/provider integration test.
- **Location:** `ModelAttemptCallAdvisorIntegrationTest` or the nearest existing provider-attempt integration fixture.
- **What it proves:** the translated timeout becomes an observable failed physical attempt with `failureKind=TIMEOUT`, the configured policy's retry decision, stable attempt/retry IDs, and exactly one request-sent record per physical attempt.
- **Fixtures:** provider chain fails once with the supplied shape, then either succeeds or exhausts according to an explicit small retry policy.
- **Mocks:** deterministic fake call chain; no sleeps/network.
- **Contract classification:** user-visible current trace semantics plus preserved configuration behavior.
- **Compatibility expectation:** retry keys/defaults remain unchanged; only previously unknown timeout behavior changes.

### 3. Truthful step terminals and projections

#### `toolFailureEmitsOneStepFailedAndNoCompletion`

- **Type:** Java engine integration test; first step-terminal red test.
- **Location:** update `StepLoopMissionExecutionEngineTest.surfacesToolFailureAsExplicitTerminalFailure`.
- **What it proves:** a started tool step that fails non-cancellably emits exactly one `STEP_FAILED`, zero `STEP_COMPLETED`, closes the step frame as failed, and links the step terminal to stable useful failure evidence.
- **Fixtures:** existing throwing tool fixture.
- **Mocks:** current deterministic model/tool stubs.
- **Contract classification:** ephemeral diagnostic writer contract.
- **Compatibility expectation:** failed `STEP_COMPLETED` is removed, not accepted alongside the new record.

#### `modelAndValidationFailuresEmitOneStepFailed`

- **Type:** Java parameterized engine integration test.
- **Location:** `StepLoopMissionExecutionEngineTest`.
- **What it proves:** model-call failure, structured-output/validation exhaustion, and invalid-action exhaustion each produce one failed terminal for the active step with the correct frame/failure identity and no completion.
- **Fixtures:** reuse existing model-failure and exhaustion scenarios.
- **Mocks:** existing scripted model responses/exceptions.
- **Contract classification:** current trace semantics.
- **Compatibility expectation:** adds truthful terminal evidence without changing supported Java API.

#### `successfulStepEmitsOneCompletionAndNoFailure`

- **Type:** Java parameterized regression test.
- **Location:** `StepLoopMissionExecutionEngineTest`.
- **What it proves:** successful tool and final-response paths still emit exactly one `STEP_COMPLETED`, no `STEP_FAILED`, and preserve their existing success metadata.
- **Fixtures:** existing successful tool and final-response scenarios.
- **Mocks:** existing scripted model/tool fixtures.
- **Contract classification:** protected behavioral regression.
- **Compatibility expectation:** successful semantics are preserved.

#### `callerCancellationEmitsNeitherFailedNorCompletedTerminal`

- **Type:** Java cancellation/cleanup integration test.
- **Location:** `StepLoopMissionExecutionEngineTest` and/or `ExecutionTraceBoundaryCleanupTest`.
- **What it proves:** caller-owned timeout/interruption cleanup retains abort semantics, does not invent `STEP_FAILED`, does not emit success, and records only the owner-appropriate failure/cleanup facts.
- **Fixtures:** existing `timeoutRecordsOnlyTheCallerOwnedFailureWhenInterruptedModelThrows` setup and interruption-before-send cases.
- **Mocks:** blocking/interrupting model stub with deterministic synchronization.
- **Contract classification:** cancellation ownership and trace boundary contract.
- **Compatibility expectation:** explicitly protected from the new failure terminal.

#### `projectsStepFailedAsErrorWithoutCallingItCompleted`

- **Type:** Java projection unit tests.
- **Location:** `LiveActivityProjectorTest`, `ExecutionJournalProjectorTest`, `ExecutionJournalProjectionContractTest`.
- **What it proves:** `STEP_FAILED` is visible as error activity and an error-level `STEP_FAILURE` journal entry; `ERROR_RECORDED` remains separate diagnostic evidence; settled visible-kind counts and exact projection snapshots are updated; no failed path renders “Step completed.”
- **Fixtures:** minimal failed-step record plus linked error and successful control.
- **Mocks:** none.
- **Contract classification:** live/browser-facing projection semantics.
- **Compatibility expectation:** deliberate current-format vocabulary replacement.

### 4. One-send model-attempt lifecycle

#### `recordsOneSentAndNoResponseWhenProviderThrows`

- **Type:** Java advisor integration test; replacement for the prepared/sent test.
- **Location:** `ModelAttemptCallAdvisorIntegrationTest`.
- **What it proves:** quota reservation and identity allocation lead to exactly one `MODEL_REQUEST_SENT` immediately before the provider chain; a throw produces the appropriate failure with no response and no duplicate request record.
- **Fixtures:** existing provider-throws scenario.
- **Mocks:** existing failing chain.
- **Contract classification:** Java trace writer semantics.
- **Compatibility expectation:** obsolete prepared event is removed completely.

#### `everyPhysicalRetryHasExactlyOneSentAndOneTerminal`

- **Type:** Java parameterized integration regression.
- **Location:** existing retry/quota tests in `ModelAttemptCallAdvisorIntegrationTest` and `ExecutionTraceContractTest`.
- **What it proves:** transient retry, exhausted retry, semantic retry, OpenRouter success, and quota failure retain stable attempt/retry identities; every physical provider call has one sent and exactly one response/failed terminal; interruption before send has no attempt.
- **Fixtures:** reuse current retry scenarios.
- **Mocks:** current deterministic provider chains and quota policies.
- **Contract classification:** trace cardinality and quota semantics.
- **Compatibility expectation:** retry behavior preserved except for the timeout reclassification.

#### `TestAttemptLifecycleStartsAtSent`

- **Type:** Go attempt-state unit test.
- **Location:** update `internal/traceanalysis/calculations_test.go`.
- **What it proves:** `SENT → RESPONSE` and `SENT → FAILED` are valid; response/failure before sent, duplicate sent, duplicate terminal, mismatched identity, and sent-without-terminal/gap conditions keep precise outcomes.
- **Fixtures:** minimal in-memory records for each transition.
- **Mocks:** none.
- **Contract classification:** current-version NDJSON reader/derived model.
- **Compatibility expectation:** no prepared-first alternative is retained.

#### `TestProcessorRejectsRemovedPreparedRecordKind`

- **Type:** Go parser/processor negative contract test.
- **Location:** `processor_test.go`.
- **What it proves:** a current-format trace containing `MODEL_REQUEST_PREPARED` is rejected as unsupported/invalid vocabulary rather than ignored or normalized into sent.
- **Fixtures:** smallest otherwise-valid NDJSON trace with the obsolete kind.
- **Mocks:** none.
- **Contract classification:** ephemeral current-version diagnostic format.
- **Compatibility expectation:** intentional rejection; historical readability is not required.

### 5. Processor-owned plan lineage

#### `TestPlanLandmarksRemainStableAcrossCreationAndUpdates`

- **Type:** Go processor/query unit integration, table-driven.
- **Location:** new `internal/traceanalysis/plans_test.go` or focused additions to processor/query tests.
- **What it proves:** creation and every update for one `planId` expose identical `traceRootFrameId`, nearest owning `missionFrameId`, and creation-owned `planningFrameId`; only creation retains recorded attempt/retry IDs.
- **Fixtures:** create on `PLANNING`, updates on owning `ROOT_MISSION` and a descendant where allowed, with complete frame graph.
- **Mocks:** none; use temporary trace/index storage helpers.
- **Contract classification:** current-version derived trace fact.
- **Compatibility expectation:** replaces query-record-derived `rootFrameId`; legacy field is absent.

#### `TestPlanLandmarksDistinguishPrimaryNestedAndMultiplePlans`

- **Type:** Go processor/query table test.
- **Location:** `plans_test.go` and `fixture_corpus_test.go`.
- **What it proves:** a primary plan and nested plan can share a top trace root while having different mission/planning frames; multiple IDs remain independent; updates never borrow another plan's creation landmarks.
- **Fixtures:** generated corpus plan evidence plus a focused small synthetic case.
- **Mocks:** none.
- **Contract classification:** plan lineage semantics.
- **Compatibility expectation:** new stable contract; no inference fallback.

#### `TestInvalidPlanLineageIsRejected`

- **Type:** Go processor negative table test.
- **Location:** `plans_test.go` or `processor_test.go`.
- **What it proves:** update before create, duplicate creation for an ID, blank/mismatched ID, creation outside `PLANNING`, missing `ROOT_MISSION` ancestor, missing trace root, and wrong-frame lineage fail with `INVALID_PLAN_LINEAGE` and sanitized details.
- **Fixtures:** one minimal trace mutation per subcase.
- **Mocks:** none.
- **Contract classification:** current-format validation category.
- **Compatibility expectation:** malformed current traces are rejected, not repaired.

#### `TestPlanFactsDoNotDependOnQueryPageOrRecordFrame`

- **Type:** Go query/continuation regression test.
- **Location:** query-record/continuation tests.
- **What it proves:** querying creation/update separately, in different page sizes/orders, returns the same persisted landmarks and does not require page-local hierarchy reconstruction.
- **Fixtures:** one plan with records split across pages.
- **Mocks:** none.
- **Contract classification:** query determinism.
- **Compatibility expectation:** protects the processor-owned design from regression to query-time derivation.

### 6. Page-local literal-search descriptors

#### `TestSearchPageDeduplicatesContentDescriptorsAndPreservesOffsets`

- **Type:** Go search unit test; first search red test.
- **Location:** `internal/traceanalysis/search_test.go`.
- **What it proves:** repeated literal matches in one content value retain all exact offsets and match order, share one `contentId`, and produce one descriptor containing the opaque ref.
- **Fixtures:** one text payload with at least three matches.
- **Mocks:** existing indexed trace test helper.
- **Contract classification:** neutral search result contract.
- **Compatibility expectation:** match-level `contentRef` is removed.

#### `TestSearchContentIdsFollowFirstOccurrencePerPage`

- **Type:** Go table-driven search unit test.
- **Location:** `search_test.go`.
- **What it proves:** several content values receive deterministic `c1`, `c2`, … in first-match order; repeated values reuse IDs; metadata matches have no content ID; each descriptor appears exactly once; IDs restart on each page and are not stable artifact handles.
- **Fixtures:** mixed metadata/content records with interleaved matches.
- **Mocks:** existing search fixture builder.
- **Contract classification:** page-local search serialization.
- **Compatibility expectation:** intentional new current response contract.

#### `TestSearchDescriptorNormalizationDoesNotChangeContinuation`

- **Type:** Go continuation regression test.
- **Location:** `continuation_test.go`.
- **What it proves:** page size still counts matches, not descriptors; offsets at KMP/content boundaries are unchanged; resuming returns each match exactly once; work limits, fingerprints, case options, and finite reachability remain unchanged.
- **Fixtures:** repeated value spanning two pages and a boundary-spanning literal.
- **Mocks:** none.
- **Contract classification:** protected opaque-continuation and bounded-work behavior.
- **Compatibility expectation:** continuation semantics preserved.

#### `TestSearchNoMatchPageHasExplicitEmptyDescriptorsAndCoverage`

- **Type:** Go search contract test.
- **Location:** `search_test.go`.
- **What it proves:** a completed negative search returns zero matches, non-nil empty `contentDescriptors`, explicit coverage, and `workComplete=true`; an incomplete zero-result page remains honest and continuable.
- **Fixtures:** one absent literal with sufficient work budget and one with an intentionally small budget.
- **Mocks:** none.
- **Contract classification:** negative evidence/completeness contract.
- **Compatibility expectation:** preserves current honesty requirements.

#### `TestContentIdCannotBeUsedAsExactReadReference`

- **Type:** Go service/adapter security regression.
- **Location:** traceanalysis service tests and/or joined adapter tests.
- **What it proves:** passing page-local `c1` to exact content read is rejected; using the associated opaque descriptor `contentRef` succeeds and returns the exact range.
- **Fixtures:** one positive search result and its descriptor.
- **Mocks:** real in-process analysis service.
- **Contract classification:** opaque-reference security boundary.
- **Compatibility expectation:** preserves reference opacity while introducing a display/join ID.

### 7. MCP/browser mapping and fallback formatting

#### `TestPlanAndSearchContractsMatchAcrossMCPAndBrowser`

- **Type:** Go joined-adapter contract test.
- **Location:** `internal/mcpadapter/trace_joined_adapters_test.go`, with focused assertions in MCP/browser tests.
- **What it proves:** both adapters expose the same plan landmark triples, creation-only attempt fields, match `contentId` assignments, descriptor maps, offsets, completeness, and continuation from one neutral result.
- **Fixtures:** primary+nested plan trace and repeated-search trace, preferably from the generated corpus.
- **Mocks:** shared real traceanalysis service; adapter transports only.
- **Contract classification:** unreleased MCP/browser serialized contracts.
- **Compatibility expectation:** both break atomically; no adapter-specific aliases.

#### `TestOrdinaryRecordPagesOmitSearchDescriptors`

- **Type:** Go MCP/browser DTO contract test.
- **Location:** `mcpadapter/traces_test.go` and `browserapi/trace_analysis_test.go`.
- **What it proves:** search responses include descriptors (including an empty array when applicable), but ordinary record-query pages do not gain a meaningless `contentDescriptors` field.
- **Fixtures:** one ordinary record page and one literal page.
- **Mocks:** trace service stub or real small trace.
- **Contract classification:** serialized response shape.
- **Compatibility expectation:** exact new shape, no dual fields.

#### `TestOptionalFallbackFormattingIsDeterministic`

- **Type:** Go table-driven unit test.
- **Location:** `mcpadapter/traces_test.go` or `contracts_test.go`.
- **What it proves:** nil renders `-`; present blank string and zero timestamp render `unknown`; present strings are bounded/escaped; timestamps render UTC RFC3339Nano; optional parent/closed/duration fields show values rather than pointer identities.
- **Fixtures:** nil, blank/zero, Unicode/control-containing/boundary-length string, non-UTC timestamp, zero/nonzero duration.
- **Mocks:** none.
- **Contract classification:** MCP text fallback contract.
- **Compatibility expectation:** behavioral correction only; structured JSON is unchanged by formatting.

#### `TestMCPFallbacksNeverContainPointerAddresses`

- **Type:** Go response-level regression test.
- **Location:** `mcpadapter/traces_test.go` and possibly real tool protocol test.
- **What it proves:** inventory, compact-frame, record, and range fallback strings never match `0x[0-9a-fA-F]+`; fallback budgets and escaping remain intact.
- **Fixtures:** all representative DTOs with every optional pointer populated.
- **Mocks:** deterministic service results.
- **Contract classification:** safety/usability invariant.
- **Compatibility expectation:** no pointer-format compatibility is preserved.

### 8. Live and frontend presentation

#### `TestStepFailedIsAValidErrorActivityKind`

- **Type:** Go live DTO/coordinator/service tests.
- **Location:** existing live kind enumeration, label, round-trip, and unknown-kind tests.
- **What it proves:** `STEP_FAILED` is accepted, labeled as failure/error evidence, serialized/streamed unchanged, and included in the exact valid-kind set; `STEP_COMPLETED` remains a non-error success kind; unknown kinds still fail.
- **Fixtures:** one activity of each step terminal kind.
- **Mocks:** existing coordinator/service harness.
- **Contract classification:** live browser contract.
- **Compatibility expectation:** current valid-kind count changes intentionally.

#### `activityPresentationDistinguishesFailedAndCompletedSteps`

- **Type:** TypeScript unit/component tests.
- **Location:** activity presentation tests and `LiveActivity.test.tsx`.
- **What it proves:** failed and completed steps have distinct accessible labels/severity; failed activity never says completed; recent-completion routing continues to follow execution terminal rules rather than treating every step failure as execution completion.
- **Fixtures:** typed activities for both kinds.
- **Mocks:** none beyond existing render setup.
- **Contract classification:** user-visible UI behavior.
- **Compatibility expectation:** deliberate vocabulary update.

#### `presentsOneSentRequestAndRejectsPreparedVocabulary`

- **Type:** TypeScript component/contract test update.
- **Location:** replace the prepared assertion in `TraceRecords.model.test.tsx`; update `api/contracts.ts` compile-time unions and any raw-record tests.
- **What it proves:** sent request content remains inspectable; no specialized prepared label/component exists; a prepared value cannot enter the typed record-kind union.
- **Fixtures:** current sent request record and chunked content.
- **Mocks:** existing content reader mocks.
- **Contract classification:** frontend projection of current trace vocabulary.
- **Compatibility expectation:** remove, do not hide or alias, the obsolete kind.

#### `rendersStepFailedWithLinkedFailureEvidence`

- **Type:** TypeScript component test.
- **Location:** `TraceRecords.results.test.tsx` or a new focused `TraceRecords.stepFailure.test.tsx`.
- **What it proves:** `STEP_FAILED` renders as a failed terminal, exposes the recorded failure link deliberately, and cannot be presented as a successful receipt; successful `STEP_COMPLETED` test remains green.
- **Fixtures:** one failed-step fact and linked failure; one success control.
- **Mocks:** existing trace view/failure-selection callbacks.
- **Contract classification:** browser UI semantic contract.
- **Compatibility expectation:** intentional presentation replacement.

#### `searchUsesDescriptorRefForExactContentReads`

- **Type:** TypeScript component/API interaction test.
- **Location:** `TraceExplorer.test.tsx`, `TraceViews.test.tsx`, and relevant contract fixtures.
- **What it proves:** matches render/page using short `contentId`, lookup resolves the page descriptor, and the exact-read request receives the opaque `contentRef`; missing descriptor is reported as invalid evidence rather than sending `contentId`.
- **Fixtures:** repeated matches sharing `c1`, one descriptor, and a malformed missing-descriptor case.
- **Mocks:** API client spies already used by explorer tests.
- **Contract classification:** browser response consumption and reference security.
- **Compatibility expectation:** old per-match ref path is removed.

#### `planPresentationUsesStableTraceMissionPlanningLandmarks`

- **Type:** TypeScript component/contract test.
- **Location:** `TraceRecords.plan.test.tsx` and any plan fact fixtures.
- **What it proves:** creation/update summaries use `traceRootFrameId`, `missionFrameId`, and `planningFrameId`; update comparison remains keyed by `planId`; legacy `rootFrameId` is absent.
- **Fixtures:** primary and nested creation/update facts.
- **Mocks:** existing paged record/content helpers.
- **Contract classification:** browser UI current contract.
- **Compatibility expectation:** exact in-place field replacement.

### 9. Cross-language fixtures, exact versioning, and authoring/evaluation evidence

#### `ConsoleTraceFixtureCorpusTest` producer cases

- **Type:** Java generated contract fixture test.
- **Location:** `loomspan-spring-boot-starter/src/test/java/com/lokiscale/loomspan/internal/runtime/trace/ConsoleTraceFixtureCorpusTest.java` and generated `loomspan-console-fixtures` files.
- **What it proves:** producer-owned current traces contain one sent event, truthful success/failure step terminals, typed timeout/retry facts, and primary/nested/multiple plan lineages; invalid cases include malformed plan lineage; each fixture carries the exact existing release-derived compatibility marker.
- **Fixtures:** generated NDJSON and expected JSON. Add repeated searchable content if it belongs naturally in corpus behavior; do not hand-edit outputs.
- **Mocks:** deterministic Java runtime/model/tool fixtures.
- **Contract classification:** Java-to-Go ephemeral diagnostic boundary.
- **Compatibility expectation:** regenerate in place; historical development traces need not remain readable.

#### `TestCurrentFixtureCorpusHasPR30Semantics`

- **Type:** Go cross-language contract test.
- **Location:** `internal/traceanalysis/fixture_corpus_test.go` and MCP semantic fixture tests.
- **What it proves:** Go accepts every valid Java-generated fixture, rejects named invalid fixtures with the exact category, and returns the expected timeout, attempt, step, plan, and search semantics. It must explicitly assert absence of prepared records and failed completions.
- **Fixtures:** only Java-generated corpus and expected files.
- **Mocks:** none.
- **Contract classification:** executable writer/reader/projector coherence.
- **Compatibility expectation:** one exact current format; no historical compatibility assertion.

#### `TestCompatibilityVersionMustMatchExactly` (preserve/update)

- **Type:** Go processor/import contract test and E2E regression.
- **Location:** existing `processor_test.go`, browser contract tests, and `portable-trace-import.spec.ts`.
- **What it proves:** the packaged exact marker is accepted; older, newer, blank, malformed, and same-looking development alternatives are rejected. PR 30 does not add a schema marker, range, or migration path.
- **Fixtures:** existing compatibility matrix plus regenerated current fixture.
- **Mocks:** existing import/process helpers.
- **Contract classification:** protected release compatibility gate.
- **Compatibility expectation:** exact behavior preserved.

#### Agent Skill validation and evaluation cases

- **Type:** documentation contract validation plus agent evaluation.
- **Location:** `loomspan-console/agent-skills/loomspan-runtime-debugging`, `loomspan-console/agent-evals`, and `ai/skill-authoring/traces-and-debugging.md` source audit.
- **What it proves:** guidance and executable cases use one-send attempts, `STEP_FAILED`, typed timeout/retry decisions, the three stable plan landmarks, and `contentId → descriptor → contentRef → read`; no evaluation rewards legacy fields or passing `contentId` to read.
- **Fixtures:** update `final-primary-plan.json`, `composite-adversarial.ndjson`, and add a focused nested-plan/search/timeout case only if current cases cannot express all decisions.
- **Mocks:** existing evaluation harness/fake MCP transcript.
- **Contract classification:** author-facing debugging workflow; source-verified skill documentation.
- **Compatibility expectation:** packaged skill remains `1.0.0` for the unreleased in-place correction; no legacy instruction branch.

### 10. Compatibility and obsolete-vocabulary guards

#### `LoomspanPublicSurfaceArchitectureTest` (unchanged acceptance gate)

- **Type:** Java architecture contract test.
- **Location:** existing architecture test.
- **What it proves:** no supported `com.lokiscale.loomspan.api` type changes, no internal/autoconfigure type leaks through supported signatures, and no accidental SPI/bean override contract is introduced.
- **Fixtures/mocks:** none.
- **Contract classification:** protected supported Java API.
- **Compatibility expectation:** zero delta.

#### Existing retry configuration tests (unchanged regression gate)

- **Type:** Java configuration/policy tests.
- **Location:** existing provider retry policy and configuration-binding suites selected by source changes.
- **What it proves:** property names, defaults, bounds, and policy decisions are unchanged; only the classified failure kind feeding the policy changes.
- **Fixtures/mocks:** existing configuration contexts.
- **Contract classification:** user-visible configuration behavior.
- **Compatibility expectation:** preserved.

#### `PR30ObsoleteVocabularyAudit`

- **Type:** repository source/fixture audit performed as an acceptance command, not a historical-document rewrite.
- **Location:** all nonhistorical production, tests, generated current fixtures, packaged skill, and eval files.
- **What it proves:** no positive use of `MODEL_REQUEST_PREPARED`, failed `STEP_COMPLETED`, plan-fact `rootFrameId`, or match-level search `contentRef` remains. Intentional negative tests may contain the removed strings and must be easy to identify.
- **Fixtures/mocks:** none.
- **Contract classification:** atomic contract cleanup.
- **Compatibility expectation:** removed current behavior stays removed; dated ticket/research/plan evidence is excluded from the ban.

## How to Run

Use repository-root PowerShell commands unless a command changes directory explicitly. During implementation, run the smallest owning test first, then the area suite, then full acceptance.

### Focused Java red/green loop

```powershell
.\mvnw.cmd -pl loomspan-spring-boot-starter test "-Dtest=SpringAiProviderIntegrationTest,ModelAttemptCallAdvisorIntegrationTest,StepLoopMissionExecutionEngineTest,LiveActivityProjectorTest,ExecutionJournalProjectorTest,ExecutionJournalProjectionContractTest,ExecutionTraceContractTest,ExecutionTraceBoundaryCleanupTest" -DfailIfNoTests=false
```

Run the supported-surface gate after production type changes:

```powershell
.\mvnw.cmd -pl loomspan-spring-boot-starter test -Dtest=LoomspanPublicSurfaceArchitectureTest -DfailIfNoTests=false
```

If provider configuration/policy files change, add their existing focused test classes to the first command; do not create a new configuration behavior merely for this PR.

### Focused Go red/green loops

```powershell
Push-Location loomspan-console
go test ./internal/mcpadapter
go test ./internal/traceanalysis
go test ./internal/browserapi ./internal/live
Pop-Location
```

Use `-run` during the initial red/green slices, for example:

```powershell
Push-Location loomspan-console
go test ./internal/mcpadapter -run 'Test(ToolsListMatchesCompactSnapshotAndBudget|RegisteredToolsValidateCompactAndCompleteOutputSchemas|OptionalFallback)'
go test ./internal/traceanalysis -run 'Test(AttemptLifecycleStartsAtSent|Plan|Search)'
Pop-Location
```

Exact names may be adjusted to existing Go naming conventions, but the behaviors and subcase matrices above must remain.

### Frontend focused tests

Use the repository-pinned Node/npm workflow. After `npm ci` has been established by the build tool, run affected Vitest files through the package scripts, then use the canonical verifier:

```powershell
Push-Location loomspan-console\web
npm test -- --run src/activity src/observability/TraceRecords.model.test.tsx src/observability/TraceRecords.results.test.tsx src/observability/TraceRecords.plan.test.tsx src/observability/TraceExplorer.test.tsx src/observability/TraceViews.test.tsx
Pop-Location
```

If that script's argument forwarding differs in the pinned package version, use the exact `package.json` Vitest command rather than installing or upgrading tooling.

### Fixture regeneration and verification

Regenerate only through the intentional Java writer workflow:

```powershell
.\mvnw.cmd -pl loomspan-spring-boot-starter test -Dtest=ConsoleTraceFixtureCorpusTest -Dloomspan.console.fixtures.regenerate=true -DfailIfNoTests=false
.\mvnw.cmd -pl loomspan-spring-boot-starter test -Dtest=ConsoleTraceFixtureCorpusTest -DfailIfNoTests=false
```

Then immediately run the Go consumer and adapter fixture tests:

```powershell
Push-Location loomspan-console
go test ./internal/traceanalysis ./internal/mcpadapter ./internal/browserapi
Pop-Location
```

Review generated NDJSON/expected diffs rather than hand-editing them. Verify LF endings and exact release markers.

### Full Console acceptance

```powershell
Push-Location loomspan-console
go test ./...
go run ./internal/buildtool verify
Pop-Location
```

`go run ./internal/buildtool verify` is the canonical gate for toolchain checks, pinned frontend installation, Agent Skill validation, TypeScript typecheck, frontend coverage, production build/assets, and Go tests.

Run race detection only if the implementation introduces shared mutable schema/registration/validation state:

```powershell
Push-Location loomspan-console
$env:CGO_ENABLED = '1'
go test -race ./...
Remove-Item Env:CGO_ENABLED
Pop-Location
```

On Windows, use the MSYS2/CGO path required by `loomspan-console/AGENTS.md` if the conditional race run is necessary. Prefer immutable registration-time schemas so it is not necessary merely because schemas are validated.

### Obsolete vocabulary audit

Run targeted searches and inspect every nonhistorical hit:

```powershell
rg -n "MODEL_REQUEST_PREPARED|rootFrameId|contentRef|STEP_COMPLETED" loomspan-spring-boot-starter loomspan-console loomspan-console-fixtures ai/skill-authoring
```

Interpretation matters:

- `MODEL_REQUEST_PREPARED` may remain only in intentional rejection tests and dated `ai/thoughts` evidence.
- `rootFrameId` may remain for unrelated non-plan concepts only; plan facts must use `traceRootFrameId` and `missionFrameId`.
- `contentRef` remains valid in page-level descriptors and exact-read requests, but not on individual search matches.
- `STEP_COMPLETED` remains valid for success, but no producer, projector, fixture, or guidance may use it for a failed step.

### Manual acceptance walkthrough

After automated gates pass:

1. Restart the development Java target and Console so both use the same current checkout.
2. Connect a fresh stateless MCP client without seeding a trace ID.
3. Capture the exact serialized `tools/list` byte count and the client's approximate token observation; confirm all 12 tools are present and discovery is ≤20 KiB.
4. Discover the newest trace using inventory and compact orientation tools rather than raw NDJSON.
5. Inspect primary and nested plan facts and verify stable trace-root, mission, and planning-frame IDs across creation/update.
6. Inspect the supplied/reproduced timeout attempt and verify typed `TIMEOUT` plus policy-derived retry decision without opening diagnostic stack content.
7. Inspect a failed tool/model step and verify `STEP_FAILED`, a failed frame, useful linked failure evidence, and no completion claim.
8. Inspect each physical provider attempt and verify one sent request and no prepared request.
9. Run a repeated literal search; verify all offsets remain, one descriptor exists per content value, and exact read succeeds only with the descriptor's opaque ref.
10. Inspect inventory/frame fallback text for nil, unknown, and present values and confirm no pointer address.
11. Run a complete negative search and verify explicit coverage with `workComplete=true`.

Do not save credentials, absolute private paths, raw diagnostics, or unrelated sensitive returned content in acceptance evidence.

## Exit Criteria

PR 30 is test-complete only when all of the following are true:

- [ ] Each of the seven observed defects has a focused test that was demonstrably red before its production change and green afterward.
- [x] All 12 MCP tools have explicit compact output schemas and representative success/error results pass both compact advertised and complete generated-schema validation.
- [x] A compact-schema/full-schema divergence test proves the complete validation path is real and independent.
- [x] The real serialized `tools/list` response matches the reviewed exact snapshot, records its exact byte count, contains exactly 12 tools, preserves strict inputs, and is ≤20 KiB.
- [x] The supplied OpenAI read-timeout shape is transient `TIMEOUT`; cancellation, interruption, generic `InterruptedIOException`, isolated HTTP/2 `CANCEL`, and unrelated I/O negative cases remain non-timeout.
- [x] Retry integration records the policy-derived decision and one sent event per physical attempt without changing configuration keys/defaults.
- [x] Every started successful step has exactly one `STEP_COMPLETED`; every non-cancellation failed step has exactly one `STEP_FAILED`; caller-owned abort paths have neither false success nor false failure terminal.
- [x] Live activity, journal, browser, MCP, and frontend projections call a failed step failed and preserve distinct diagnostic evidence.
- [x] Java emits no prepared request; Go starts attempts at sent and rejects prepared as obsolete current-format vocabulary; UI/docs/evals contain no positive prepared behavior.
- [x] Plan creation and updates expose identical `traceRootFrameId`, `missionFrameId`, and `planningFrameId` for a lineage; primary/nested/multiple plans are correct; malformed lineage fails as `INVALID_PLAN_LINEAGE`.
- [x] Search preserves every match offset, order, page-size boundary, work/completeness fact, and continuation while serializing each opaque reference once per page.
- [x] Page-local `contentId` is never accepted as an exact-read reference; UI and agents resolve the descriptor's opaque `contentRef` first.
- [x] MCP and browser joined-adapter tests return equivalent plan/search semantics, and ordinary record pages omit search-only descriptor fields.
- [x] All optional fallback subcases are deterministic, bounded, escaped, and free of `0x...` pointer-address patterns.
- [x] Java-generated current fixtures are reviewed and verified byte-for-byte; Go accepts valid fixtures and rejects named invalid fixtures with exact categories.
- [x] Exact `consoleCompatibilityVersion` equality tests remain green for packaged current fixtures and reject all non-exact variants; no PR-specific version or legacy reader is added.
- [x] `LoomspanPublicSurfaceArchitectureTest` reports zero supported API/SPI expansion, and existing configuration-policy tests remain green.
- [x] Packaged Agent Skill validation and affected agent evaluations pass with the new one-send, failed-step, plan-lineage, timeout, and descriptor workflows.
- [x] Focused Java, Go, adapter/live, and frontend tests pass; `go test ./...` and `go run ./internal/buildtool verify` pass from `loomspan-console`.
- [ ] Conditional `go test -race ./...` passes if and only if shared mutable schema/registration state was introduced.
- [x] The obsolete-vocabulary audit has no unexplained nonhistorical hits.
- [ ] The fresh-client manual walkthrough completes without raw-NDJSON dependence and records only safe acceptance evidence.
