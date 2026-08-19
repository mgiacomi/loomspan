package traceanalysis

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/evidence"
)

// persistedRecordFacts contains the neutral, processor-owned relationships for
// one canonical record. Search matches remain query-specific and are computed
// from the already selected record rather than persisted here.
type persistedRecordFacts struct {
	Attempts    []attemptResult   `json:"attempts,omitempty"`
	Retries     []retryResult     `json:"retries,omitempty"`
	Validations []validationLink  `json:"validations,omitempty"`
	Failures    []failureResult   `json:"failures,omitempty"`
	Payloads    []payloadIndexRow `json:"payloads,omitempty"`
}

func (facts persistedRecordFacts) empty() bool {
	return len(facts.Attempts) == 0 && len(facts.Retries) == 0 && len(facts.Validations) == 0 && len(facts.Failures) == 0 && len(facts.Payloads) == 0
}

func buildPersistedRecordFacts(attempts []attemptResult, retries []retryResult, validations []validationLink, failures []failureResult, payloads []payloadIndexRow) map[int64]persistedRecordFacts {
	out := map[int64]persistedRecordFacts{}
	retryOwnerSequence := make(map[string]int64, len(retries))
	for _, attempt := range attempts {
		if attempt.ownerSequence <= 0 {
			continue
		}
		facts := out[attempt.ownerSequence]
		facts.Attempts = append(facts.Attempts, attempt)
		out[attempt.ownerSequence] = facts
		retryOwnerSequence[attempt.RetrySequenceID] = attempt.ownerSequence
	}
	for _, retry := range retries {
		sequence := retryOwnerSequence[retry.RetrySequenceID]
		if sequence <= 0 {
			continue
		}
		facts := out[sequence]
		facts.Retries = append(facts.Retries, retry)
		out[sequence] = facts
	}
	for _, validation := range validations {
		facts := out[validation.sequence]
		facts.Validations = append(facts.Validations, validation)
		out[validation.sequence] = facts
	}
	for _, failure := range failures {
		facts := out[failure.Sequence]
		facts.Failures = append(facts.Failures, failure)
		out[failure.Sequence] = facts
	}
	for _, payload := range payloads {
		facts := out[payload.Sequence]
		facts.Payloads = append(facts.Payloads, payload)
		out[payload.Sequence] = facts
	}
	return out
}

type recordFactReader struct {
	index     artifact.ComponentReader
	store     artifact.ComponentReader
	stopIndex func() bool
	stopStore func() bool
}

func openRecordFactReader(ctx context.Context, lease *artifact.Lease) (*recordFactReader, error) {
	index, err := lease.OpenComponent(artifact.ComponentName(ComponentRecordFactIdx))
	if err != nil {
		return nil, err
	}
	store, err := lease.OpenComponent(artifact.ComponentName(ComponentRecordFacts))
	if err != nil {
		_ = index.Close()
		return nil, err
	}
	return &recordFactReader{index: index, store: store, stopIndex: closeReaderOnCancellation(ctx, index), stopStore: closeReaderOnCancellation(ctx, store)}, nil
}

func (reader *recordFactReader) Close() {
	reader.stopIndex()
	reader.stopStore()
	_ = reader.index.Close()
	_ = reader.store.Close()
}

func (reader *recordFactReader) Read(ctx context.Context, position int64) (persistedRecordFacts, error) {
	if err := ctx.Err(); err != nil {
		return persistedRecordFacts{}, err
	}
	if _, err := reader.index.Seek(position*recordFactIndexRowWidth, io.SeekStart); err != nil {
		return persistedRecordFacts{}, err
	}
	var addressBytes [recordFactIndexRowWidth]byte
	if _, err := io.ReadFull(&contextChunkReader{ctx: ctx, reader: reader.index}, addressBytes[:]); err != nil {
		return persistedRecordFacts{}, err
	}
	address := readRecordFactIndexRow(addressBytes[:])
	if address.Offset < 0 || address.Length < 0 || address.Length > maxFactRowBytes {
		return persistedRecordFacts{}, fmt.Errorf("invalid record fact address at position %d", position)
	}
	if address.Length == 0 {
		return persistedRecordFacts{}, nil
	}
	if _, err := reader.store.Seek(address.Offset, io.SeekStart); err != nil {
		return persistedRecordFacts{}, err
	}
	body := make([]byte, address.Length)
	if _, err := io.ReadFull(&contextChunkReader{ctx: ctx, reader: reader.store}, body); err != nil {
		return persistedRecordFacts{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var facts persistedRecordFacts
	if err := decoder.Decode(&facts); err != nil {
		return persistedRecordFacts{}, err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return persistedRecordFacts{}, fmt.Errorf("record fact row has trailing data")
	}
	return facts, nil
}

func materializeRecordFacts(stored persistedRecordFacts, rec *Record, filter RecordFilter, ref evidence.Reference, handle artifact.Handle, traceCtx TraceContext) (RecordFacts, error) {
	facts := RecordFacts{Attempts: []AttemptSummary{}, Retries: []RetrySummary{}, Validations: []ValidationSummary{}, Failures: []FailureSummary{}, Payloads: []PayloadDescriptor{}, SearchMatches: []SearchResult{}}
	if rec.Type == RecordPlanCreated || rec.Type == RecordPlanUpdated {
		if planID, present, err := rec.metadataString("planId"); present && err == nil && planID != "" {
			facts.Plan = &PlanLandmark{PlanID: planID, Sequence: rec.Sequence, PlanningFrameID: rec.FrameID}
			if rec.Type == RecordPlanCreated {
				facts.Plan.AttemptID, _, _ = rec.metadataString("attemptId")
				facts.Plan.RetrySequenceID, _, _ = rec.metadataString("retrySequenceId")
			}
		}
	}
	for _, a := range stored.Attempts {
		contentRef := ""
		if a.PayloadID != "" {
			var err error
			contentRef, err = encodeEnvelopeContentReference(ref, handle, a.PayloadID)
			if err != nil {
				return RecordFacts{}, err
			}
		}
		facts.Attempts = append(facts.Attempts, AttemptSummary{Context: traceCtx, RetrySequenceID: a.RetrySequenceID, AttemptID: a.AttemptID, AttemptNumber: a.AttemptNumber, AttemptReason: a.AttemptReason, ProviderAttemptNumber: a.ProviderAttemptNumber, Outcome: a.Outcome, FailureClassification: a.FailureClassification, FailureCategory: a.FailureCategory, RetryDecision: a.RetryDecision, RetryDelayMillis: a.RetryDelayMillis, RetryDelaySource: a.RetryDelaySource, HTTPStatus: a.HTTPStatus, ProviderErrorType: a.ProviderErrorType, ProviderErrorCode: a.ProviderErrorCode, ContentRef: contentRef, Usage: a.Usage, UsageComplete: a.UsageComplete})
	}
	for _, r := range stored.Retries {
		facts.Retries = append(facts.Retries, RetrySummary{Context: traceCtx, RetrySequenceID: r.RetrySequenceID, Usage: r.Usage, UsageComplete: r.UsageComplete})
	}
	for _, v := range stored.Validations {
		facts.Validations = append(facts.Validations, ValidationSummary{Context: traceCtx, Status: v.Status, RetrySequenceID: v.RetrySequenceID, AttemptID: v.AttemptID, AttemptNumber: v.AttemptNumber})
	}
	for _, f := range stored.Failures {
		diagnostics := append([]DiagnosticDescriptor(nil), f.Diagnostics...)
		for i := range diagnostics {
			ordinal := diagnostics[i].Ordinal
			var err error
			diagnostics[i].ContentRef, err = encodeDiagnosticContentReference(ref, handle, f.FailureID, ordinal)
			if err != nil {
				return RecordFacts{}, err
			}
		}
		facts.Failures = append(facts.Failures, FailureSummary{Context: traceCtx, FailureID: f.FailureID, Terminal: f.Terminal, Sequence: f.Sequence, TimestampMillis: f.TimestampMillis, RecordType: f.RecordType, FrameID: f.FrameID, Route: f.Route, AttemptID: f.AttemptID, RetrySequenceID: f.RetrySequenceID, ValidationStatus: f.ValidationStatus, ExceptionType: f.ExceptionType, ContextSummary: f.ContextSummary, Diagnostics: diagnostics})
	}
	for _, p := range stored.Payloads {
		contentRef, err := encodeEnvelopeContentReference(ref, handle, p.PayloadID)
		if err != nil {
			return RecordFacts{}, err
		}
		facts.Payloads = append(facts.Payloads, PayloadDescriptor{Context: traceCtx, PayloadID: p.PayloadID, ContentRef: contentRef, Sequence: p.Sequence, ContentType: p.ContentType, ChunkCount: p.ChunkCount, StoreOffset: p.StoreOffset, StoreLength: p.StoreLength})
	}
	if filter.LiteralText != "" {
		needle := []byte(filter.LiteralText)
		appendMatches := func(field string, haystack []byte) {
			for start := 0; start <= len(haystack)-len(needle); {
				offset := bytes.Index(haystack[start:], needle)
				if offset < 0 {
					break
				}
				offset += start
				facts.SearchMatches = append(facts.SearchMatches, SearchResult{Context: traceCtx, Sequence: rec.Sequence, RecordType: string(rec.Type), FrameID: rec.FrameID, MatchOffset: int64(offset), MatchLength: len(needle), SearchedField: field})
				start = offset + len(needle)
			}
		}
		appendMatches("metadata", rec.Metadata)
		appendMatches("data", rec.Data)
	}
	return facts, nil
}
