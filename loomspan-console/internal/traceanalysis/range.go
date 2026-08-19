package traceanalysis

import (
	"context"
	"encoding/base64"
	"io"
	"mime"
	"unicode/utf8"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/evidence"
)

// RangeRequest is a bounded byte range request against a payload, raw record,
// or raw artifact. Start is the byte offset to begin reading at; MaxBytes is
// the maximum number of bytes to return (clamped to maxRangeBytes). If
// ContinueCursor is non-empty, Start is ignored and the cursor's next offset
// is used.
type RangeRequest struct {
	Handle         artifact.Handle
	ContinueCursor string
	Start          int64
	MaxBytes       int
	// Source identifies which component to read.
	Source RangeSource
	// ContentRef selects one semantic value without exposing storage details.
	ContentRef string
	// RecordSequence is required when Source == RangeSourceRawRecord.
	RecordSequence int64
}

// readPayloadRange reads a bounded byte range from a reconstructed logical
// payload stored in the payload store component. It adjusts text/JSON ranges
// to complete UTF-8 code points and returns base64 for arbitrary bytes that
// cannot be represented as a complete UTF-8 slice.
func (service *Service) readPayloadRange(ctx context.Context, lease *artifact.Lease, scopeID evidence.Reference, handle artifact.Handle, req RangeRequest, descriptor payloadDescriptor, fingerprint string) (ByteRangeResult, *consolecore.Error) {
	if e := ctx.Err(); e != nil {
		return ByteRangeResult{}, canceledError(e)
	}
	traceCtx, err := traceContextForLease(lease, scopeID, handle)
	if err != nil {
		return ByteRangeResult{}, storageError(scopeID.ID(), err)
	}
	start, maxBytes, domain := resolveRangeBounds(scopeID, req, descriptor.StoreLength)
	if domain != nil {
		return ByteRangeResult{}, domain
	}
	if start >= descriptor.StoreLength {
		// Empty range at end-of-payload.
		return ByteRangeResult{
			Context:     traceCtx,
			Source:      RangeSourceContent,
			ActualStart: start,
			ActualEnd:   start,
			TotalLength: descriptor.StoreLength,
			ContentType: descriptor.ContentType,
			Encoding:    RangeEncodingText,
			Content:     []byte{},
			HasMore:     false,
		}, nil
	}
	reader, err := lease.OpenComponent(artifact.ComponentName(ComponentPayloadStore))
	if err != nil {
		return ByteRangeResult{}, storageError(scopeID.ID(), err)
	}
	defer reader.Close()
	if _, err := reader.Seek(descriptor.StoreOffset+start, io.SeekStart); err != nil {
		return ByteRangeResult{}, storageError(scopeID.ID(), err)
	}
	buf := make([]byte, maxBytes)
	n, err := readRangeBytes(ctx, reader, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		if ctx.Err() != nil {
			return ByteRangeResult{}, canceledError(ctx.Err())
		}
		return ByteRangeResult{}, storageError(scopeID.ID(), err)
	}
	buf = buf[:n]
	actualStart := start
	actualEnd := start + int64(n)
	encoding, content, adjustedStart, adjustedEnd := encodeRangeContent(buf, descriptor.ContentType, actualStart, actualEnd, descriptor.StoreLength)
	actualStart = adjustedStart
	actualEnd = adjustedEnd
	hasMore := actualEnd < descriptor.StoreLength
	var nextCursor string
	if hasMore {
		nextCursor, err = encodeRangeCursor(cursorOpPayloadRange, ownerCursorKey(lease.Owner()), handle, fingerprint, actualEnd)
		if err != nil {
			return ByteRangeResult{}, cursorError(scopeID.ID(), err)
		}
	}
	return ByteRangeResult{
		Context:     traceCtx,
		Source:      RangeSourceContent,
		ActualStart: actualStart,
		ActualEnd:   actualEnd,
		TotalLength: descriptor.StoreLength,
		ContentType: descriptor.ContentType,
		Encoding:    encoding,
		Content:     content,
		NextCursor:  nextCursor,
		HasMore:     hasMore,
	}, nil
}

// readRawRecordRange reads a bounded byte range from one physical record's
// bytes inside the raw NDJSON artifact. The range is bounded by the record's
// raw length; requesting beyond the record end returns a short range.
func (service *Service) readRawRecordRange(ctx context.Context, lease *artifact.Lease, scopeID evidence.Reference, handle artifact.Handle, req RangeRequest, row recordIndexRow, fingerprint string) (ByteRangeResult, *consolecore.Error) {
	if e := ctx.Err(); e != nil {
		return ByteRangeResult{}, canceledError(e)
	}
	traceCtx, err := traceContextForLease(lease, scopeID, handle)
	if err != nil {
		return ByteRangeResult{}, storageError(scopeID.ID(), err)
	}
	recordLen := row.Length
	start, maxBytes, domain := resolveRangeBounds(scopeID, req, recordLen)
	if domain != nil {
		return ByteRangeResult{}, domain
	}
	if start >= recordLen {
		return ByteRangeResult{
			Context:     traceCtx,
			Source:      RangeSourceRawRecord,
			ActualStart: start,
			ActualEnd:   start,
			TotalLength: recordLen,
			Encoding:    RangeEncodingText,
			Content:     []byte{},
			HasMore:     false,
		}, nil
	}
	reader, err := lease.OpenComponent(artifact.ComponentRawArtifact)
	if err != nil {
		return ByteRangeResult{}, storageError(scopeID.ID(), err)
	}
	defer reader.Close()
	if _, err := reader.Seek(row.Offset+start, io.SeekStart); err != nil {
		return ByteRangeResult{}, storageError(scopeID.ID(), err)
	}
	buf := make([]byte, maxBytes)
	n, err := readRangeBytes(ctx, reader, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		if ctx.Err() != nil {
			return ByteRangeResult{}, canceledError(ctx.Err())
		}
		return ByteRangeResult{}, storageError(scopeID.ID(), err)
	}
	buf = buf[:n]
	actualStart := start
	actualEnd := start + int64(n)
	// Raw records are JSON; treat as text for UTF-8 boundary adjustment.
	encoding, content, adjustedStart, adjustedEnd := encodeRangeContent(buf, "application/json", actualStart, actualEnd, recordLen)
	actualStart = adjustedStart
	actualEnd = adjustedEnd
	hasMore := actualEnd < recordLen
	var nextCursor string
	if hasMore {
		nextCursor, err = encodeRangeCursor(cursorOpRawRecordRange, ownerCursorKey(lease.Owner()), handle, fingerprint, actualEnd)
		if err != nil {
			return ByteRangeResult{}, cursorError(scopeID.ID(), err)
		}
	}
	return ByteRangeResult{
		Context:     traceCtx,
		Source:      RangeSourceRawRecord,
		ActualStart: actualStart,
		ActualEnd:   actualEnd,
		TotalLength: recordLen,
		ContentType: "application/json",
		Encoding:    encoding,
		Content:     content,
		NextCursor:  nextCursor,
		HasMore:     hasMore,
	}, nil
}

// readRawArtifactRange reads a bounded byte range from the raw NDJSON artifact
// starting at an absolute byte offset. This is the internal bounded raw-range
// primitive retained for PR 18's raw-artifact MCP adapter.
func (service *Service) readRawArtifactRange(ctx context.Context, lease *artifact.Lease, scopeID evidence.Reference, handle artifact.Handle, req RangeRequest, totalLength int64, fingerprint string) (ByteRangeResult, *consolecore.Error) {
	if e := ctx.Err(); e != nil {
		return ByteRangeResult{}, canceledError(e)
	}
	traceCtx, err := traceContextForLease(lease, scopeID, handle)
	if err != nil {
		return ByteRangeResult{}, storageError(scopeID.ID(), err)
	}
	start, maxBytes, domain := resolveRangeBounds(scopeID, req, totalLength)
	if domain != nil {
		return ByteRangeResult{}, domain
	}
	if start >= totalLength {
		return ByteRangeResult{
			Context:     traceCtx,
			Source:      RangeSourceRawArtifact,
			ActualStart: start,
			ActualEnd:   start,
			TotalLength: totalLength,
			Encoding:    RangeEncodingText,
			Content:     []byte{},
			HasMore:     false,
		}, nil
	}
	reader, err := lease.OpenComponent(artifact.ComponentRawArtifact)
	if err != nil {
		return ByteRangeResult{}, storageError(scopeID.ID(), err)
	}
	defer reader.Close()
	if _, err := reader.Seek(start, io.SeekStart); err != nil {
		return ByteRangeResult{}, storageError(scopeID.ID(), err)
	}
	buf := make([]byte, maxBytes)
	n, err := readRangeBytes(ctx, reader, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		if ctx.Err() != nil {
			return ByteRangeResult{}, canceledError(ctx.Err())
		}
		return ByteRangeResult{}, storageError(scopeID.ID(), err)
	}
	buf = buf[:n]
	actualStart := start
	actualEnd := start + int64(n)
	encoding, content, adjustedStart, adjustedEnd := encodeRangeContent(buf, "application/x-ndjson", actualStart, actualEnd, totalLength)
	actualStart = adjustedStart
	actualEnd = adjustedEnd
	hasMore := actualEnd < totalLength
	var nextCursor string
	if hasMore {
		nextCursor, err = encodeRangeCursor(cursorOpRawArtifactRange, ownerCursorKey(lease.Owner()), handle, fingerprint, actualEnd)
		if err != nil {
			return ByteRangeResult{}, cursorError(scopeID.ID(), err)
		}
	}
	return ByteRangeResult{
		Context:     traceCtx,
		Source:      RangeSourceRawArtifact,
		ActualStart: actualStart,
		ActualEnd:   actualEnd,
		TotalLength: totalLength,
		ContentType: "application/x-ndjson",
		Encoding:    encoding,
		Content:     content,
		NextCursor:  nextCursor,
		HasMore:     hasMore,
	}, nil
}

func readRangeBytes(ctx context.Context, reader io.ReadCloser, buf []byte) (int, error) {
	stopCancellationClose := closeReaderOnCancellation(ctx, reader)
	defer stopCancellationClose()
	n, err := io.ReadFull(&contextChunkReader{ctx: ctx, reader: reader}, buf)
	if contextErr := ctx.Err(); contextErr != nil {
		return n, contextErr
	}
	return n, err
}

// resolveRangeBounds resolves the start offset and max bytes for a range
// request, applying defaults and clamping to both maxRangeBytes and the
// remaining bytes in the selected source. Cursor validation
// (op, scope, fingerprint) is handled by the caller before this function is
// reached: the public Read*Range methods call prepareCursor before lease
// acquisition and validateCursorFingerprint after, following the required
// precedence: INVALID_CURSOR (malformed/op), TARGET_CHANGED (scope mismatch),
// ARTIFACT_EXPIRED (handle not installed), then INVALID_CURSOR (fingerprint).
func resolveRangeBounds(scopeID evidence.Reference, req RangeRequest, totalLength int64) (int64, int, *consolecore.Error) {
	start := req.Start
	if start < 0 {
		return 0, 0, consolecore.NewError(consolecore.CodeInvalidArgument,
			"The range start must not be negative.", scopeID.ID(), consolecore.Details{}, nil)
	}
	maxBytes := req.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultRangeBytes
	}
	if maxBytes > maxRangeBytes {
		return 0, 0, consolecore.NewError(consolecore.CodeLimitExceeded,
			"The requested range exceeds the maximum allowed size.",
			scopeID.ID(), consolecore.Details{LimitName: "rangeBytes", LimitValue: int64(maxRangeBytes)}, nil)
	}
	if start > totalLength {
		start = totalLength
	}
	remaining := totalLength - start
	if int64(maxBytes) > remaining {
		maxBytes = int(remaining)
	}
	return start, maxBytes, nil
}

// encodeRangeContent returns text only when the complete requested slice is
// valid UTF-8. A slice that starts or ends inside a rune is returned as exact
// base64 bytes so continuation traversal never discards source bytes.
func encodeRangeContent(buf []byte, contentType string, start, end, total int64) (RangeEncoding, []byte, int64, int64) {
	if isTextContentType(contentType) && utf8.Valid(buf) {
		return RangeEncodingText, buf, start, end
	}
	// For arbitrary bytes or non-UTF-8 content, return base64 with exact
	// byte offsets. The caller can request the next range to continue.
	encoded := base64.StdEncoding.EncodeToString(buf)
	return RangeEncodingBase64, []byte(encoded), start, end
}

// isTextContentType reports whether a content type should be treated as text
// for UTF-8 boundary adjustment.
func isTextContentType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = contentType
	}
	switch mediaType {
	case "text/plain", "application/json", "application/x-ndjson":
		return true
	default:
		return false
	}
}
