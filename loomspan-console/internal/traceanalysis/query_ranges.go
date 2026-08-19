package traceanalysis

import (
	"context"
	"fmt"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/evidence"
)

// ReadContentRange reads exactly one selected semantic value. The continuation
// and hasMore flag are relative to that value, never to the backing artifact.
func (service *Service) ReadContentRange(ctx context.Context, scopeID evidence.Reference, req RangeRequest) (ByteRangeResult, *consolecore.Error) {
	if domain := validateRangeRequestShape(scopeID, req, RangeSourceContent); domain != nil {
		return ByteRangeResult{}, domain
	}
	if req.ContentRef == "" {
		return ByteRangeResult{}, consolecore.NewError(consolecore.CodeInvalidArgument,
			"A content reference is required for semantic content ranges.",
			scopeID.ID(), consolecore.Details{}, nil)
	}
	contentRef, err := decodeContentReference(req.ContentRef)
	if err != nil || validateContentReference(contentRef, scopeID, req.Handle) != nil {
		return ByteRangeResult{}, consolecore.NewError(consolecore.CodeInvalidArgument, "The content reference is invalid.", scopeID.ID(), consolecore.Details{}, err)
	}
	if contentRef.ContentSource == contentSourceFailureDiagnostic {
		return service.readFailureDiagnosticReferenceRange(ctx, scopeID, req, contentRef)
	}
	maxBytes, domain := validateRangeSize(scopeID, req.MaxBytes)
	if domain != nil {
		return ByteRangeResult{}, domain
	}
	req.MaxBytes = maxBytes

	// Validate cursor before lease acquisition so INVALID_CURSOR (malformed/op)
	// and TARGET_CHANGED (scope mismatch) take precedence over ARTIFACT_EXPIRED.
	var cur cursor
	lease, domain := service.leaseForHandle(scopeID, req.Handle)
	if domain != nil {
		return ByteRangeResult{}, domain
	}
	success := false
	defer func() { _ = lease.Close(success) }()
	if req.ContinueCursor != "" {
		cur, _, domain = prepareCursor(req.ContinueCursor, ownerCursorKey(lease.Owner()), scopeID.ID(), cursorOpPayloadRange)
		if domain != nil {
			return ByteRangeResult{}, domain
		}
		req.Start = cur.Position
	}

	if e := ctx.Err(); e != nil {
		return ByteRangeResult{}, canceledError(e)
	}

	fingerprint, err := canonicalizeRequest(rangeFingerprint{
		Source:     string(RangeSourceContent),
		ContentRef: req.ContentRef,
		MaxBytes:   maxBytes,
	})
	if err != nil {
		return ByteRangeResult{}, consolecore.NewError(consolecore.CodeConsoleError,
			"The range request could not be canonicalized.", scopeID.ID(), consolecore.Details{}, err)
	}

	// Fingerprint check after lease acquisition (ARTIFACT_EXPIRED already passed).
	if req.ContinueCursor != "" {
		if d := validateCursorFingerprint(cur, fingerprint, ownerCursorKey(lease.Owner()), scopeID.ID(), req.Handle); d != nil {
			return ByteRangeResult{}, d
		}
	}

	var result ByteRangeResult
	if contentRef.ContentSource == contentSourceEnvelope {
		descPtr, readErr := findPayloadDescriptorInIndex(lease, contentRef.PayloadID)
		if readErr != nil {
			return ByteRangeResult{}, storageError(scopeID.ID(), readErr)
		}
		if descPtr == nil {
			return ByteRangeResult{}, consolecore.NewError(consolecore.CodeNotFound, "The selected content was not found in this artifact.", scopeID.ID(), consolecore.Details{}, nil)
		}
		result, domain = service.readPayloadRange(ctx, lease, scopeID, req.Handle, req, *descPtr, fingerprint)
	} else {
		result, domain = service.readRecordDataReferenceRange(ctx, lease, scopeID, req, contentRef, fingerprint)
	}
	if domain != nil {
		return ByteRangeResult{}, domain
	}
	success = true
	return result, nil
}

func (service *Service) readFailureDiagnosticReferenceRange(ctx context.Context, ref evidence.Reference, req RangeRequest, contentRef contentReference) (ByteRangeResult, *consolecore.Error) {
	maxBytes, domain := validateRangeSize(ref, req.MaxBytes)
	if domain != nil {
		return ByteRangeResult{}, domain
	}
	req.MaxBytes = maxBytes
	fingerprint, err := canonicalizeRequest(struct {
		ContentRef string `json:"contentRef"`
		MaxBytes   int    `json:"maxBytes"`
	}{req.ContentRef, maxBytes})
	if err != nil {
		return ByteRangeResult{}, consolecore.NewError(consolecore.CodeConsoleError, "The diagnostic range could not be canonicalized.", ref.ID(), consolecore.Details{}, err)
	}
	var cur cursor
	if req.ContinueCursor != "" {
		var d *consolecore.Error
		cur, _, d = prepareCursor(req.ContinueCursor, referenceCursorKey(ref), ref.ID(), cursorOpDiagnosticRange)
		if d != nil {
			return ByteRangeResult{}, d
		}
		req.Start = cur.Position
	}
	lease, domain := service.leaseForHandle(ref, req.Handle)
	if domain != nil {
		return ByteRangeResult{}, domain
	}
	success := false
	defer func() { _ = lease.Close(success) }()
	ownerKey := ownerCursorKey(lease.Owner())
	if req.ContinueCursor != "" {
		if d := validateCursorFingerprint(cur, fingerprint, ownerKey, ref.ID(), req.Handle); d != nil {
			return ByteRangeResult{}, d
		}
	}
	diagnostic, domain := service.getFailureDiagnosticWithLease(ctx, lease, ref, FailureDiagnosticRequest{Handle: req.Handle, FailureID: contentRef.FailureID, Ordinal: *contentRef.Ordinal})
	if domain != nil {
		return ByteRangeResult{}, domain
	}
	raw := []byte(diagnostic.Text)
	if req.Start > int64(len(raw)) {
		return ByteRangeResult{}, consolecore.NewError(consolecore.CodeInvalidArgument,
			"The range start exceeds the diagnostic length.", ref.ID(), consolecore.Details{}, nil)
	}
	start, count, domain := resolveRangeBounds(ref, req, int64(len(raw)))
	if domain != nil {
		return ByteRangeResult{}, domain
	}
	end := start + int64(count)
	encoding, content, actualStart, actualEnd := encodeRangeContent(raw[start:end], diagnostic.Descriptor.ContentType, start, end, int64(len(raw)))
	hasMore := end < int64(len(raw))
	next := ""
	if hasMore {
		next, err = encodeRangeCursor(cursorOpDiagnosticRange, ownerKey, req.Handle, fingerprint, end)
		if err != nil {
			return ByteRangeResult{}, cursorError(ref.ID(), err)
		}
	}
	success = true
	return ByteRangeResult{Context: diagnostic.Context, Source: RangeSourceContent, ActualStart: actualStart, ActualEnd: actualEnd, TotalLength: int64(len(raw)), ContentType: diagnostic.Descriptor.ContentType, Encoding: encoding, Content: content, HasMore: hasMore, NextCursor: next}, nil
}

func (service *Service) readRecordDataReferenceRange(ctx context.Context, lease *artifact.Lease, ref evidence.Reference, req RangeRequest, contentRef contentReference, fingerprint string) (ByteRangeResult, *consolecore.Error) {
	row, ok, err := findRecordRowInIndex(lease, contentRef.Sequence)
	if err != nil {
		return ByteRangeResult{}, storageError(ref.ID(), err)
	}
	if !ok {
		return ByteRangeResult{}, consolecore.NewError(consolecore.CodeNotFound, "The selected content was not found in this artifact.", ref.ID(), consolecore.Details{}, nil)
	}
	reader, err := lease.OpenComponent(artifact.ComponentRawArtifact)
	if err != nil {
		return ByteRangeResult{}, storageError(ref.ID(), err)
	}
	defer reader.Close()
	raw, err := readRawRecordBytesFrom(reader, row)
	if err != nil {
		return ByteRangeResult{}, storageError(ref.ID(), err)
	}
	rec, d := decodeRecord(raw, RawAddress{Offset: row.Offset, Length: row.Length, TerminatorLength: row.TerminatorLength})
	if d != nil || rec.Data == nil {
		return ByteRangeResult{}, consolecore.NewError(consolecore.CodeNotFound, "The selected content is unavailable.", ref.ID(), consolecore.Details{}, nil)
	}
	start, count, domain := resolveRangeBounds(ref, req, int64(len(rec.Data)))
	if domain != nil {
		return ByteRangeResult{}, domain
	}
	end := start + int64(count)
	encoding, content, actualStart, actualEnd := encodeRangeContent(rec.Data[start:end], "application/json", start, end, int64(len(rec.Data)))
	hasMore := actualEnd < int64(len(rec.Data))
	next := ""
	if hasMore {
		next, err = encodeRangeCursor(cursorOpPayloadRange, ownerCursorKey(lease.Owner()), req.Handle, fingerprint, actualEnd)
		if err != nil {
			return ByteRangeResult{}, cursorError(ref.ID(), err)
		}
	}
	traceCtx, err := traceContextForLease(lease, ref, req.Handle)
	if err != nil {
		return ByteRangeResult{}, storageError(ref.ID(), err)
	}
	return ByteRangeResult{Context: traceCtx, Source: RangeSourceContent, ActualStart: actualStart, ActualEnd: actualEnd, TotalLength: int64(len(rec.Data)), ContentType: "application/json", Encoding: encoding, Content: content, HasMore: hasMore, NextCursor: next}, nil
}

func referenceCursorKey(ref evidence.Reference) string {
	if ref.Source == evidence.SourceImported {
		return string(evidence.SourceImported)
	}
	return string(evidence.SourceTarget) + ":" + string(ref.TargetScope)
}

// ReadRawRecordRange reads a bounded byte range from one physical record's
// bytes inside the raw NDJSON artifact. The range is bounded by the record's
// raw length.
func (service *Service) ReadRawRecordRange(ctx context.Context, scopeID evidence.Reference, req RangeRequest) (ByteRangeResult, *consolecore.Error) {
	if domain := validateRangeRequestShape(scopeID, req, RangeSourceRawRecord); domain != nil {
		return ByteRangeResult{}, domain
	}
	if req.RecordSequence <= 0 {
		return ByteRangeResult{}, consolecore.NewError(consolecore.CodeInvalidArgument,
			"A positive record sequence is required for raw record ranges.",
			scopeID.ID(), consolecore.Details{}, nil)
	}
	maxBytes, domain := validateRangeSize(scopeID, req.MaxBytes)
	if domain != nil {
		return ByteRangeResult{}, domain
	}
	req.MaxBytes = maxBytes

	// Validate cursor before lease acquisition so INVALID_CURSOR (malformed/op)
	// and TARGET_CHANGED (scope mismatch) take precedence over ARTIFACT_EXPIRED.
	var cur cursor
	lease, domain := service.leaseForHandle(scopeID, req.Handle)
	if domain != nil {
		return ByteRangeResult{}, domain
	}
	success := false
	defer func() { _ = lease.Close(success) }()
	if req.ContinueCursor != "" {
		cur, _, domain = prepareCursor(req.ContinueCursor, ownerCursorKey(lease.Owner()), scopeID.ID(), cursorOpRawRecordRange)
		if domain != nil {
			return ByteRangeResult{}, domain
		}
		req.Start = cur.Position
	}

	if e := ctx.Err(); e != nil {
		return ByteRangeResult{}, canceledError(e)
	}

	row, ok, err := findRecordRowInIndex(lease, req.RecordSequence)
	if err != nil {
		return ByteRangeResult{}, storageError(scopeID.ID(), err)
	}
	if !ok {
		return ByteRangeResult{}, consolecore.NewError(consolecore.CodeNotFound,
			"The record was not found in this artifact.",
			scopeID.ID(), consolecore.Details{}, fmt.Errorf("sequence=%d", req.RecordSequence))
	}

	fingerprint, err := canonicalizeRequest(rangeFingerprint{
		Source:         string(RangeSourceRawRecord),
		RecordSequence: req.RecordSequence,
		MaxBytes:       maxBytes,
	})
	if err != nil {
		return ByteRangeResult{}, consolecore.NewError(consolecore.CodeConsoleError,
			"The range request could not be canonicalized.", scopeID.ID(), consolecore.Details{}, err)
	}

	// Fingerprint check after lease acquisition (ARTIFACT_EXPIRED already passed).
	if req.ContinueCursor != "" {
		if d := validateCursorFingerprint(cur, fingerprint, ownerCursorKey(lease.Owner()), scopeID.ID(), req.Handle); d != nil {
			return ByteRangeResult{}, d
		}
	}

	result, domain := service.readRawRecordRange(ctx, lease, scopeID, req.Handle, req, row, fingerprint)
	if domain != nil {
		return ByteRangeResult{}, domain
	}
	success = true
	return result, nil
}

func findRecordRowInIndex(lease *artifact.Lease, sequence int64) (recordIndexRow, bool, error) {
	size, err := lease.ComponentSize(artifact.ComponentName(ComponentRecordIndex))
	if err != nil {
		return recordIndexRow{}, false, err
	}
	if size%recordIndexRowWidth != 0 {
		return recordIndexRow{}, false, fmt.Errorf("record index has invalid size %d", size)
	}
	reader, err := lease.OpenComponent(artifact.ComponentName(ComponentRecordIndex))
	if err != nil {
		return recordIndexRow{}, false, err
	}
	defer reader.Close()
	count := size / recordIndexRowWidth
	position, err := lowerBoundRecordSequence(reader, count, sequence)
	if err != nil || position == count {
		return recordIndexRow{}, false, err
	}
	row, err := readRecordIndexRowAt(reader, position)
	if err != nil {
		return recordIndexRow{}, false, err
	}
	return row, row.Sequence == sequence, nil
}

// ReadRawArtifactRange reads a bounded byte range from the raw NDJSON artifact
// starting at an absolute byte offset. This is the bounded raw-range primitive
// retained for PR 18's raw-artifact MCP adapter.
func (service *Service) ReadRawArtifactRange(ctx context.Context, scopeID evidence.Reference, req RangeRequest) (ByteRangeResult, *consolecore.Error) {
	if domain := validateRangeRequestShape(scopeID, req, RangeSourceRawArtifact); domain != nil {
		return ByteRangeResult{}, domain
	}
	maxBytes, domain := validateRangeSize(scopeID, req.MaxBytes)
	if domain != nil {
		return ByteRangeResult{}, domain
	}
	req.MaxBytes = maxBytes

	// Validate cursor before lease acquisition so INVALID_CURSOR (malformed/op)
	// and TARGET_CHANGED (scope mismatch) take precedence over ARTIFACT_EXPIRED.
	var cur cursor
	lease, domain := service.leaseForHandle(scopeID, req.Handle)
	if domain != nil {
		return ByteRangeResult{}, domain
	}
	success := false
	defer func() { _ = lease.Close(success) }()
	if req.ContinueCursor != "" {
		cur, _, domain = prepareCursor(req.ContinueCursor, ownerCursorKey(lease.Owner()), scopeID.ID(), cursorOpRawArtifactRange)
		if domain != nil {
			return ByteRangeResult{}, domain
		}
		req.Start = cur.Position
	}

	if e := ctx.Err(); e != nil {
		return ByteRangeResult{}, canceledError(e)
	}

	size, err := lease.ComponentSize(artifact.ComponentRawArtifact)
	if err != nil {
		return ByteRangeResult{}, storageError(scopeID.ID(), err)
	}

	fingerprint, err := canonicalizeRequest(rangeFingerprint{
		Source:   string(RangeSourceRawArtifact),
		MaxBytes: maxBytes,
	})
	if err != nil {
		return ByteRangeResult{}, consolecore.NewError(consolecore.CodeConsoleError,
			"The range request could not be canonicalized.", scopeID.ID(), consolecore.Details{}, err)
	}

	// Fingerprint check after lease acquisition (ARTIFACT_EXPIRED already passed).
	if req.ContinueCursor != "" {
		if d := validateCursorFingerprint(cur, fingerprint, ownerCursorKey(lease.Owner()), scopeID.ID(), req.Handle); d != nil {
			return ByteRangeResult{}, d
		}
	}

	result, domain := service.readRawArtifactRange(ctx, lease, scopeID, req.Handle, req, size, fingerprint)
	if domain != nil {
		return ByteRangeResult{}, domain
	}
	success = true
	return result, nil
}

func validateRangeRequestShape(scopeID evidence.Reference, req RangeRequest, expected RangeSource) *consolecore.Error {
	if req.Source != "" && req.Source != expected {
		return consolecore.NewError(consolecore.CodeInvalidArgument,
			"The range source does not match the requested operation.", scopeID.ID(), consolecore.Details{}, nil)
	}
	if req.Start < 0 {
		return consolecore.NewError(consolecore.CodeInvalidArgument,
			"The range start must not be negative.", scopeID.ID(), consolecore.Details{}, nil)
	}
	if req.ContinueCursor != "" && req.Start != 0 {
		return consolecore.NewError(consolecore.CodeInvalidArgument,
			"A range start and continuation cannot be supplied together.", scopeID.ID(), consolecore.Details{}, nil)
	}
	return nil
}

// rangeFingerprint is the canonical projection of a range request used for
// cursor fingerprinting.
type rangeFingerprint struct {
	Source         string `json:"source"`
	ContentRef     string `json:"contentRef,omitempty"`
	RecordSequence int64  `json:"recordSequence,omitempty"`
	MaxBytes       int    `json:"maxBytes"`
}
