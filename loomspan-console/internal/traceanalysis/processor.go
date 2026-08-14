// Package traceanalysis owns the transport-neutral trace-analysis processor
// that turns a current-scope acquired NDJSON artifact into a validated,
// immutable analysis bundle before its handle is exposed.
//
// Phase 3 expands the Phase 2 stub into full streaming parsing, chunk
// reconstruction, validation, hierarchy/timing/usage calculations, immutable
// indexes, and a manifest. The processor keeps at most one bounded physical
// line in memory; raw bytes, reconstructed payloads, and immutable query
// indexes live in the artifact bundle and share its lifecycle.
package traceanalysis

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/release"
)

// Processor is the required artifact.Processor implementation for the current
// release. It validates the raw NDJSON, reconstructs chunked payloads, builds
// immutable indexes, calculates hierarchy/timing/usage/attempt/validation/
// failure facts once, and writes a manifest before the artifact handle is
// published. On any invalidity, cancellation, or recoverable storage failure it
// returns a domain error so the service removes the staged bundle and publishes
// no handle.
type Processor struct {
	compatibilityVersion string
}

// New creates the trace-analysis processor.
func New() *Processor {
	return newProcessorForVersion(release.ProductVersion())
}

func newProcessorForVersion(version string) *Processor {
	if strings.TrimSpace(version) == "" {
		panic("trace processor compatibility version must be nonblank")
	}
	return &Processor{compatibilityVersion: version}
}

// PreflightImport reads only the first bounded physical record, validates the
// canonical start identity and compatibility marker, and returns a byte-exact
// replay stream for the normal complete processor pass.
func (processor *Processor) PreflightImport(ctx context.Context, raw io.Reader) (artifact.ImportPreflight, *consolecore.Error) {
	reader := bufio.NewReaderSize(raw, maxPhysicalLineBytes+2)
	line, terminator, readErr := readPhysicalLine(reader)
	content := line[:len(line)-int(terminator)]
	if len(content) == 0 {
		return artifact.ImportPreflight{}, invalidityError(CategoryMalformedJSON, "")
	}
	if len(content) > maxPhysicalLineBytes {
		return artifact.ImportPreflight{}, invalidityError(CategoryLineTooLarge, "")
	}
	record, domain := decodeRecord(content, RawAddress{Length: int64(len(content)), TerminatorLength: terminator})
	if domain != nil {
		return artifact.ImportPreflight{}, domain
	}
	if record.Type != RecordTraceStarted {
		return artifact.ImportPreflight{}, invalidityError(CategoryUnsupportedValue, record.TraceID)
	}
	observed, valid := extractCompatibilityVersion(record)
	if !valid {
		return artifact.ImportPreflight{}, invalidityError(CategoryUnsupportedValue, record.TraceID)
	}
	if observed != processor.compatibilityVersion {
		return artifact.ImportPreflight{}, consolecore.NewError(consolecore.CodeIncompatibleArtifact,
			"The trace artifact was produced by an incompatible Loomspan version.", record.TraceID,
			consolecore.Details{ExpectedCompatibilityVersion: processor.compatibilityVersion,
				ObservedCompatibilityVersion: observed}, nil)
	}
	if readErr != nil && readErr != io.EOF {
		return artifact.ImportPreflight{}, invalidityErrorWithCause(CategoryTruncatedInput, record.TraceID, readErr)
	}
	return artifact.ImportPreflight{
		Header: artifact.ImportHeader{TraceID: record.TraceID, SessionID: record.SessionID},
		Raw:    io.MultiReader(bytes.NewReader(line), reader),
	}, nil
}

// Process streams the raw NDJSON, validates and indexes every record,
// reconstructs chunked payloads, calculates shared facts, writes all derived
// components, and returns the derived component sizes. The raw artifact size is
// tracked separately by the service.
func (processor *Processor) Process(req artifact.ProcessRequest) (result artifact.ProcessResult, domain *consolecore.Error) {
	scopeID := req.Metadata.TraceID
	ctx := req.Context

	// Log the exact invalidity category for any content rejection. The outward
	// error message is deliberately generic (see invalidityError); this defer is
	// the only place the operator-visible reason is recorded. Non-content errors
	// (cancellation, local storage) carry no category and are not logged here.
	defer func() {
		if domain == nil {
			return
		}
		if category, ok := categoryOf(domain); ok {
			slog.Warn("trace artifact rejected by analysis processor",
				"scopeId", scopeID, "category", string(category))
		}
	}()

	// Open the payload store component first so chunked payloads stream directly
	// to disk during parsing without whole-payload allocation.
	payloadStoreWriter, domain := openPayloadStore(ctx, req.Sink, scopeID)
	if domain != nil {
		return artifact.ProcessResult{}, domain
	}
	assembler := newPayloadAssembler(payloadStoreWriter)

	validator := newValidator(scopeID)
	frames := newFrameGraph()
	attempts := newAttemptGraph()
	failures := newFailureGraph()
	usage := newUsageCalculator()
	writer := newIndexWriter(req.Sink, ctx, scopeID)
	if d := writer.startRecordIndex(); d != nil {
		assembler.cleanup()
		_ = payloadStoreWriter.Close()
		return artifact.ProcessResult{}, d
	}
	defer writer.abortRecordIndex()

	var completionRec *Record
	var configuredLimits *ConfiguredLimits
	var lastSeq int64
	var sawStart bool

	// Parse and validate in one streaming pass. The callback retains only the
	// compact working state needed for calculations; raw bytes and payloads
	// stream to the bundle.
	_, domainErr := parseStream(ctx, req.Raw, func(rec *Record) *consolecore.Error {
		if !sawStart {
			if rec.Type != RecordTraceStarted {
				return invalidityError(CategoryUnsupportedValue, scopeID)
			}
			observed, valid := extractCompatibilityVersion(rec)
			if !valid {
				return invalidityError(CategoryUnsupportedValue, scopeID)
			}
			if observed != processor.compatibilityVersion {
				return consolecore.NewError(consolecore.CodeIncompatibleArtifact,
					"The trace artifact was produced by an incompatible Loomspan version.", scopeID,
					consolecore.Details{ExpectedCompatibilityVersion: processor.compatibilityVersion,
						ObservedCompatibilityVersion: observed}, nil)
			}
			sawStart = true
		}
		if d := validator.onRecord(rec); d != nil {
			return d
		}
		lastSeq = rec.Sequence
		if rec.Type == RecordTraceStarted {
			var valid bool
			configuredLimits, valid = extractConfiguredLimits(rec)
			if !valid {
				return invalidityError(CategoryUnsupportedValue, scopeID)
			}
		}

		// Record-address index row.
		if d := writer.appendRecordRow(recordIndexRow{
			Sequence:         rec.Sequence,
			Offset:           rec.Raw.Offset,
			Length:           rec.Raw.Length,
			TerminatorLength: rec.Raw.TerminatorLength,
		}); d != nil {
			return d
		}

		// Chunked payload assembly.
		if rec.IsEnvelope {
			if d := assembler.onEnvelope(rec); d != nil {
				return d
			}
		}
		if rec.IsChunk {
			if d := assembler.onChunk(rec); d != nil {
				return d
			}
			return nil // chunk records are physical-only; no further calculation.
		}

		// Frame graph.
		if rec.Type == RecordFrameOpened {
			if d := frames.onFrameOpened(rec); d != nil {
				return d
			}
		}
		if rec.Type == RecordFrameClosed {
			if d := frames.onFrameClosed(rec); d != nil {
				return d
			}
		}
		frames.associateRecord(rec)

		// Attempts and usage.
		if isModelRecord(rec.Type) {
			if d := attempts.onModelRecord(rec); d != nil {
				return d
			}
		}
		if rec.Type == RecordModelResponseReceived {
			u, complete, ok := extractResponseUsage(rec)
			if !ok {
				return invalidityError(CategoryInvalidUsage, scopeID)
			}
			unframed := rec.FrameID == ""
			if !unframed {
				exists, arithmeticOK := frames.addDirectUsageWithCompleteness(rec.FrameID, u, complete)
				if !exists {
					return invalidityError(CategoryInvalidFrameRelationship, scopeID)
				}
				if !arithmeticOK {
					return invalidityError(CategoryContradictoryUsage, scopeID)
				}
			}
			if !usage.addAttributed(u, complete, unframed) {
				return invalidityError(CategoryContradictoryUsage, scopeID)
			}
		}

		// Failures.
		if rec.Type == RecordErrorRecorded {
			if d := failures.onErrorRecord(rec); d != nil {
				return d
			}
		}

		// Validation links (advisor mutations).
		if rec.Type == RecordAdvisorRequestMutation || rec.Type == RecordAdvisorResponseMutation {
			if d := attempts.onAdvisorRecord(rec); d != nil {
				return d
			}
		}

		if rec.Type == RecordTraceCompleted {
			completionRec = rec
		}
		return nil
	})
	if domainErr != nil {
		assembler.cleanup()
		_ = payloadStoreWriter.Close()
		return artifact.ProcessResult{}, domainErr
	}

	// Finalize payload assembly (validates complete chunk sets and reconstructed
	// content) before closing the store. On failure, cleanup any remaining
	// in-flight validators so their per-build goroutines (which block on a pipe
	// reader) are unblocked and do not leak.
	if d := assembler.finalize(); d != nil {
		assembler.cleanup()
		_ = payloadStoreWriter.Close()
		return artifact.ProcessResult{}, d
	}
	if d := failures.validateDiagnostics(assembler, scopeID); d != nil {
		_ = payloadStoreWriter.Close()
		return artifact.ProcessResult{}, d
	}
	if err := payloadStoreWriter.Sync(); err != nil {
		_ = payloadStoreWriter.Close()
		return artifact.ProcessResult{}, storageError(scopeID, err)
	}
	if err := payloadStoreWriter.Close(); err != nil {
		return artifact.ProcessResult{}, storageError(scopeID, err)
	}
	writer.recordPayloadStoreSize(assembler.storeWritten)

	// Terminal validation. Check completion presence and finality before
	// validating terminal failure linkage so a non-final completion is reported
	// as NON_FINAL_COMPLETION rather than a downstream terminal-failure error.
	metaView := traceMetadataView{
		traceID:   req.Metadata.TraceID,
		sessionID: req.Metadata.SessionID,
	}
	if d := validator.finalize(metaView); d != nil {
		return artifact.ProcessResult{}, d
	}
	if completionRec == nil || completionRec.Sequence != lastSeq {
		return artifact.ProcessResult{}, invalidityError(CategoryNonFinalCompletion, scopeID)
	}

	outcome, ok := extractOutcome(completionRec)
	if !ok {
		return artifact.ProcessResult{}, invalidityError(CategoryUnsupportedValue, scopeID)
	}
	if req.Metadata.Outcome != "" && req.Metadata.Outcome != string(outcome) {
		return artifact.ProcessResult{}, invalidityError(CategoryInconsistentIdentity, scopeID)
	}
	if !req.Metadata.FinalizedAt.IsZero() && !req.Metadata.FinalizedAt.Equal(completionRec.Timestamp) {
		return artifact.ProcessResult{}, invalidityError(CategoryInconsistentIdentity, scopeID)
	}
	if req.Metadata.PersistencePolicy != "" &&
		req.Metadata.PersistencePolicy != completionRec.metadataStringOrEmpty("persistencePolicy") {
		return artifact.ProcessResult{}, invalidityError(CategoryInconsistentIdentity, scopeID)
	}
	terminalFailureID, hasTerminalFailure := extractTerminalFailureID(completionRec)
	if d := failures.validateTerminalLink(outcome, terminalFailureID, scopeID); d != nil {
		return artifact.ProcessResult{}, d
	}
	if hasTerminalFailure {
		if d := failures.validateTerminalAttemptLink(terminalFailureID, attempts, scopeID); d != nil {
			return artifact.ProcessResult{}, d
		}
	}
	terminalUsage, ok := extractTerminalUsage(completionRec)
	if !ok {
		return artifact.ProcessResult{}, invalidityError(CategoryContradictoryUsage, scopeID)
	}
	usage.setTerminal(terminalUsage)
	unattributed, ok := usage.unattributed()
	if !ok {
		return artifact.ProcessResult{}, invalidityError(CategoryContradictoryUsage, scopeID)
	}

	// Frame graph validation and results.
	if d := frames.validate(); d != nil {
		return artifact.ProcessResult{}, d
	}
	frameResults, gaps, uncertainties, ok := frames.results()
	if !ok {
		return artifact.ProcessResult{}, invalidityError(CategoryContradictoryUsage, scopeID)
	}

	// Build attempt/retry/validation results.
	attemptResults, retryResults, ok := buildAttemptResults(attempts, completionRec)
	if !ok {
		return artifact.ProcessResult{}, invalidityError(CategoryContradictoryUsage, scopeID)
	}
	validationLinks := attempts.validationLinks
	for _, attemptID := range attempts.order {
		if !attempts.attempts[attemptID].hasResponse && !attempts.attempts[attemptID].hasFailure {
			gaps = append(gaps, gapResult{Kind: "MODEL_ATTEMPT_RESPONSE_MISSING", AttemptID: attemptID})
		}
	}
	failureResults := failureResultsInOrder(failures)
	payloadResults := payloadIndexRows(assembler)
	recordFacts := buildPersistedRecordFacts(attemptResults, retryResults, validationLinks, failureResults, payloadResults)

	// Write immutable indexes.
	if d := writer.flushRecordIndex(); d != nil {
		return artifact.ProcessResult{}, d
	}
	limitations := indexFrameLimitations(gaps, uncertainties)
	canonicalFrames := persistFrameLimitations(frameResults, limitations)
	if d := writer.writeFactRows(ComponentFrameIndex, toAnySlice(canonicalFrames)); d != nil {
		return artifact.ProcessResult{}, d
	}
	durationFrames := append([]frameResult(nil), frameResults...)
	sortFrameResults(durationFrames, FrameOrderDurationDesc)
	if d := writer.writeFactRows(ComponentFrameDuration, toAnySlice(persistFrameLimitations(durationFrames, limitations))); d != nil {
		return artifact.ProcessResult{}, d
	}
	usageFrames := append([]frameResult(nil), frameResults...)
	sortFrameResults(usageFrames, FrameOrderUsageDesc)
	if d := writer.writeFactRows(ComponentFrameUsage, toAnySlice(persistFrameLimitations(usageFrames, limitations))); d != nil {
		return artifact.ProcessResult{}, d
	}
	if d := writer.writeFactRows(ComponentAttemptIndex, toAnySlice(attemptResults)); d != nil {
		return artifact.ProcessResult{}, d
	}
	if d := writer.writeFactRows(ComponentRetryIndex, toAnySlice(retryResults)); d != nil {
		return artifact.ProcessResult{}, d
	}
	if d := writer.writeFactRows(ComponentValidationIdx, toAnySlice(validationLinks)); d != nil {
		return artifact.ProcessResult{}, d
	}
	if d := writer.writeFactRows(ComponentFailureIndex, toAnySlice(failureResults)); d != nil {
		return artifact.ProcessResult{}, d
	}
	usageFacts := buildUsageFacts(usage, unattributed)
	if d := writer.writeFactRows(ComponentUsageIndex, usageFacts); d != nil {
		return artifact.ProcessResult{}, d
	}
	if d := writer.writeFactRows(ComponentGapIndex, toAnySlice(gaps)); d != nil {
		return artifact.ProcessResult{}, d
	}
	if d := writer.writeFactRows(ComponentUncertainty, toAnySlice(uncertainties)); d != nil {
		return artifact.ProcessResult{}, d
	}
	if d := writer.writeFactRows(ComponentPayloadIndex, toAnySlice(payloadResults)); d != nil {
		return artifact.ProcessResult{}, d
	}
	if d := writer.writeRecordFacts(recordFacts); d != nil {
		return artifact.ProcessResult{}, d
	}

	// Build and write the manifest.
	var terminalFailurePtr *string
	if hasTerminalFailure {
		t := terminalFailureID
		terminalFailurePtr = &t
	}
	rootFrameIDs := make([]string, 0)
	for _, frame := range frameResults {
		if frame.ParentFrameID == nil {
			rootFrameIDs = append(rootFrameIDs, frame.FrameID)
		}
	}
	usageComplete := true
	for _, attempt := range attemptResults {
		if !attempt.UsageComplete {
			usageComplete = false
			break
		}
	}
	m := manifest{
		Schema:            manifestSchemaV1,
		TraceID:           validator.traceID,
		SessionID:         validator.sessionID,
		Outcome:           string(outcome),
		TerminalFailureID: terminalFailurePtr,
		ConfiguredLimits:  configuredLimits,
		RecordCount:       writer.recordCount,
		FrameCount:        len(frameResults),
		AttemptCount:      len(attemptResults),
		RetryCount:        len(retryResults),
		ValidationCount:   len(validationLinks),
		FailureCount:      len(failures.order),
		PayloadCount:      len(assembler.descriptors),
		GapCount:          len(gaps),
		UncertaintyCount:  len(uncertainties),
		RootFrameIDs:      rootFrameIDs,
		UsageComplete:     usageComplete,
		ComponentSizes:    componentSizesMap(writer.components),
	}
	sizes, d := writeManifest(ctx, req.Sink, scopeID, m)
	if d != nil {
		return artifact.ProcessResult{}, d
	}

	return artifact.ProcessResult{
		ComponentSizes: sizes,
		Metadata: artifact.TraceMetadata{
			TraceID: validator.traceID, SessionID: validator.sessionID,
			Outcome: string(outcome), FinalizedAt: completionRec.Timestamp,
			PersistencePolicy: completionRec.metadataStringOrEmpty("persistencePolicy"),
		},
	}, nil
}

func extractCompatibilityVersion(rec *Record) (string, bool) {
	fields, ok := decodeUniqueObject(rec.Metadata)
	if !ok {
		return "", false
	}
	raw, ok := fields["consoleCompatibilityVersion"]
	if !ok || bytes.Equal(raw, nullBytes) {
		return "", false
	}
	var version string
	if json.Unmarshal(raw, &version) != nil || strings.TrimSpace(version) == "" {
		return "", false
	}
	return version, true
}

// openPayloadStore opens the payloads.store component for streaming chunk
// reconstruction.
func openPayloadStore(ctx context.Context, sink artifact.ComponentSink, scopeID string) (artifact.ComponentWriter, *consolecore.Error) {
	if e := ctx.Err(); e != nil {
		return nil, canceledError(e)
	}
	writer, domain := sink.Create(ctx, artifact.ComponentName(ComponentPayloadStore))
	if domain != nil {
		return nil, domain
	}
	return writer, nil
}

// storageError maps a storage failure to a domain error.
func storageError(scopeID string, cause error) *consolecore.Error {
	return consolecore.NewError(consolecore.CodeLocalStorageUnavailable,
		"Local artifact storage is unavailable.", scopeID, consolecore.Details{}, cause)
}

// isModelRecord reports whether a record type is a consumed model lifecycle
// record whose attempt identity must be validated.
func isModelRecord(t TraceRecordType) bool {
	return t == RecordModelRequestPrepared || t == RecordModelRequestSent || t == RecordModelResponseReceived || t == RecordModelAttemptFailed
}

// buildAttemptResults produces the neutral attempt and retry results in
// canonical order matching the Java fixture corpus. It reports false if any
// retry usage accumulation overflows int64.
func buildAttemptResults(g *attemptGraph, completion *Record) ([]attemptResult, []retryResult, bool) {
	attempts := make([]attemptResult, 0, len(g.order))
	retryUsage := map[string]Usage{}
	retryComplete := map[string]bool{}
	retryOrder := []string{}
	for _, id := range g.order {
		a := g.attempts[id]
		attempts = append(attempts, attemptResult{
			RetrySequenceID:       a.retrySequenceID,
			AttemptID:             a.attemptID,
			AttemptNumber:         a.attemptNumber,
			AttemptReason:         a.attemptReason,
			ProviderAttemptNumber: a.providerAttemptNumber,
			Outcome: func() string {
				if a.hasResponse {
					return "SUCCEEDED"
				}
				if a.hasFailure {
					return "FAILED"
				}
				return "INCOMPLETE"
			}(),
			FailureClassification: a.failureClassification,
			FailureCategory:       a.failureCategory,
			RetryDecision:         a.retryDecision,
			RetryDelayMillis:      a.retryDelayMillis,
			RetryDelaySource:      a.retryDelaySource,
			HTTPStatus:            a.httpStatus,
			ProviderErrorType:     a.providerErrorType,
			ProviderErrorCode:     a.providerErrorCode,
			PayloadID:             a.payloadID,
			Usage:                 a.usage,
			UsageComplete:         a.usageComplete,
			ownerSequence:         a.ownerSequence,
		})
		if _, seen := retryUsage[a.retrySequenceID]; !seen {
			retryOrder = append(retryOrder, a.retrySequenceID)
			retryComplete[a.retrySequenceID] = true
		}
		var ok bool
		retryUsage[a.retrySequenceID], ok = retryUsage[a.retrySequenceID].plus(a.usage)
		if !ok {
			return nil, nil, false
		}
		retryComplete[a.retrySequenceID] = retryComplete[a.retrySequenceID] && a.usageComplete
	}
	retries := make([]retryResult, 0, len(retryOrder))
	for _, rid := range retryOrder {
		retries = append(retries, retryResult{
			RetrySequenceID: rid,
			Usage:           retryUsage[rid],
			UsageComplete:   retryComplete[rid],
		})
	}
	// Validation links are collected from advisor mutation records during
	// parsing and returned separately by the processor.
	return attempts, retries, true
}

// buildUsageFacts produces the neutral usage facts written to the usage index.
func buildUsageFacts(c *usageCalculator, unattributed Usage) []any {
	return []any{
		map[string]any{
			"kind":            "ATTRIBUTED",
			"promptUnits":     c.attributed.PromptUnits,
			"completionUnits": c.attributed.CompletionUnits,
			"totalUnits":      c.attributed.TotalUnits,
		},
		map[string]any{
			"kind":            "UNATTRIBUTED",
			"promptUnits":     unattributed.PromptUnits,
			"completionUnits": unattributed.CompletionUnits,
			"totalUnits":      unattributed.TotalUnits,
		},
		map[string]any{
			"kind":            "UNFRAMED_ATTRIBUTED",
			"promptUnits":     c.unframedAttributed.PromptUnits,
			"completionUnits": c.unframedAttributed.CompletionUnits,
			"totalUnits":      c.unframedAttributed.TotalUnits,
		},
		map[string]any{
			"kind":            "TERMINAL",
			"promptUnits":     c.terminal.PromptUnits,
			"completionUnits": c.terminal.CompletionUnits,
			"totalUnits":      c.terminal.TotalUnits,
		},
	}
}

func failureResultsInOrder(g *failureGraph) []failureResult {
	out := make([]failureResult, 0, len(g.order))
	for _, id := range g.order {
		out = append(out, g.failures[id])
	}
	return out
}

type frameLimitationIndex struct {
	gapKindsByFrameID    map[string][]string
	gapKindsByAttemptID  map[string][]string
	uncertaintyKindsByID map[string][]string
}

func indexFrameLimitations(gaps []gapResult, uncertainties []uncertaintyResult) frameLimitationIndex {
	index := frameLimitationIndex{
		gapKindsByFrameID:    make(map[string][]string),
		gapKindsByAttemptID:  make(map[string][]string),
		uncertaintyKindsByID: make(map[string][]string),
	}
	for _, gap := range gaps {
		if gap.FrameID != "" {
			index.gapKindsByFrameID[gap.FrameID] = appendUnique(index.gapKindsByFrameID[gap.FrameID], gap.Kind)
		}
		if gap.AttemptID != "" {
			index.gapKindsByAttemptID[gap.AttemptID] = appendUnique(index.gapKindsByAttemptID[gap.AttemptID], gap.Kind)
		}
	}
	for _, uncertainty := range uncertainties {
		if uncertainty.FrameID != "" {
			index.uncertaintyKindsByID[uncertainty.FrameID] = appendUnique(index.uncertaintyKindsByID[uncertainty.FrameID], uncertainty.Kind)
		}
	}
	return index
}

func persistFrameLimitations(frames []frameResult, limitations frameLimitationIndex) []persistedFrameResult {
	out := make([]persistedFrameResult, 0, len(frames))
	for _, frame := range frames {
		gapKinds := append([]string{}, limitations.gapKindsByFrameID[frame.FrameID]...)
		for _, attemptID := range frame.AttemptIDs {
			for _, kind := range limitations.gapKindsByAttemptID[attemptID] {
				gapKinds = appendUnique(gapKinds, kind)
			}
		}
		out = append(out, persistedFrameResult{frameResult: frame, GapKinds: gapKinds, UncertaintyKinds: limitations.uncertaintyKindsByID[frame.FrameID]})
	}
	return out
}

func extractConfiguredLimits(rec *Record) (*ConfiguredLimits, bool) {
	metadata, err := rec.metadataObject()
	if err != nil {
		return nil, false
	}
	raw, present := metadata["configuredLimits"]
	if !present {
		return nil, true
	}
	if bytes.Equal(raw, nullBytes) {
		return nil, false
	}
	fields, ok := decodeUniqueObject(raw)
	if !ok || len(fields) != 6 {
		return nil, false
	}
	read := func(name string) (int64, bool) {
		value, ok := fields[name]
		if !ok || bytes.Equal(value, nullBytes) {
			return 0, false
		}
		decoder := json.NewDecoder(bytes.NewReader(value))
		decoder.UseNumber()
		var number json.Number
		if decoder.Decode(&number) != nil {
			return 0, false
		}
		parsed, err := number.Int64()
		return parsed, err == nil && parsed >= 0 && parsed <= 2147483647
	}
	maxSkills, ok1 := read("maxSkillInvocations")
	maxTools, ok2 := read("maxToolInvocations")
	maxRetries, ok3 := read("maxLinterRetries")
	maxModels, ok4 := read("maxModelCalls")
	maxProviderAttempts, ok5 := read("maxProviderAttempts")
	maxUsage, ok6 := read("maxUsageUnits")
	if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 || !ok6 {
		return nil, false
	}
	return &ConfiguredLimits{MaxSkillInvocations: maxSkills, MaxToolInvocations: maxTools,
		MaxLinterRetries: maxRetries, MaxModelCalls: maxModels, MaxProviderAttempts: maxProviderAttempts, MaxUsageUnits: maxUsage}, true
}

func decodeUniqueObject(raw json.RawMessage) (map[string]json.RawMessage, bool) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return nil, false
	}
	fields := make(map[string]json.RawMessage)
	for decoder.More() {
		name, err := decoder.Token()
		if err != nil {
			return nil, false
		}
		key, ok := name.(string)
		if !ok {
			return nil, false
		}
		if _, duplicate := fields[key]; duplicate {
			return nil, false
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, false
		}
		fields[key] = value
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') {
		return nil, false
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, false
	}
	return fields, true
}

func payloadIndexRows(a *payloadAssembler) []payloadIndexRow {
	descs := a.descriptorsInOrder()
	out := make([]payloadIndexRow, 0, len(descs))
	for _, d := range descs {
		out = append(out, payloadIndexRow{
			PayloadID:   d.PayloadID,
			Sequence:    d.Sequence,
			ContentType: d.ContentType,
			ChunkCount:  d.ChunkCount,
			StoreOffset: d.StoreOffset,
			StoreLength: d.StoreLength,
		})
	}
	return out
}

// toAnySlice converts a typed slice to []any for the fact-row writer.
func toAnySlice[T any](in []T) []any {
	out := make([]any, len(in))
	for i, v := range in {
		out[i] = v
	}
	return out
}

// componentSizesMap converts the internal component map to the artifact
// ComponentName-keyed map.
func componentSizesMap(in map[component]int64) map[artifact.ComponentName]int64 {
	out := make(map[artifact.ComponentName]int64, len(in))
	for k, v := range in {
		out[artifact.ComponentName(k)] = v
	}
	return out
}

// Compile-time assertion that Processor satisfies the artifact.Processor
// interface.
var _ artifact.Processor = (*Processor)(nil)
var _ artifact.ImportProcessor = (*Processor)(nil)
