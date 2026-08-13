package traceanalysis

import (
	"context"
	"fmt"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/evidence"
)

// ReadPayloadRange reads a bounded byte range from a reconstructed logical
// payload identified by PayloadID. The range is bounded by the payload's total
// length; requesting beyond the payload end returns a short range. Text/JSON
// ranges are adjusted to complete UTF-8 code points; arbitrary bytes are
// returned base64-encoded.
func (service *Service) ReadPayloadRange(ctx context.Context, scopeID evidence.Reference, req RangeRequest) (ByteRangeResult, *consolecore.Error) {
	if domain := validateRangeRequestShape(scopeID, req, RangeSourcePayload); domain != nil {
		return ByteRangeResult{}, domain
	}
	if req.PayloadID == "" {
		return ByteRangeResult{}, consolecore.NewError(consolecore.CodeInvalidArgument,
			"A payload ID is required for payload ranges.",
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
		cur, _, domain = prepareCursor(req.ContinueCursor, ownerCursorKey(lease.Owner()), scopeID.ID(), cursorOpPayloadRange)
		if domain != nil {
			return ByteRangeResult{}, domain
		}
		req.Start = cur.Position
	}

	if e := ctx.Err(); e != nil {
		return ByteRangeResult{}, canceledError(e)
	}

	descPtr, err := findPayloadDescriptorInIndex(lease, req.PayloadID)
	if err != nil {
		return ByteRangeResult{}, storageError(scopeID.ID(), err)
	}
	if descPtr == nil {
		return ByteRangeResult{}, consolecore.NewError(consolecore.CodeNotFound,
			"The payload was not found in this artifact.",
			scopeID.ID(), consolecore.Details{}, fmt.Errorf("payloadId=%s", req.PayloadID))
	}
	desc := *descPtr

	fingerprint, err := canonicalizeRequest(rangeFingerprint{
		Source:    string(RangeSourcePayload),
		PayloadID: req.PayloadID,
		MaxBytes:  maxBytes,
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

	result, domain := service.readPayloadRange(ctx, lease, scopeID, req.Handle, req, desc, fingerprint)
	if domain != nil {
		return ByteRangeResult{}, domain
	}
	success = true
	return result, nil
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
	PayloadID      string `json:"payloadId,omitempty"`
	RecordSequence int64  `json:"recordSequence,omitempty"`
	MaxBytes       int    `json:"maxBytes"`
}
