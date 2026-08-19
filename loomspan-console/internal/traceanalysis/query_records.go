package traceanalysis

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/evidence"
)

// RecordRepresentation selects whether a record query returns logical
// (envelope) or physical (framework) records.
type RecordRepresentation string

const (
	// RecordRepresentationLogical returns chunked-payload envelopes as single
	// logical records (chunk records are omitted).
	RecordRepresentationLogical RecordRepresentation = "LOGICAL"
	// RecordRepresentationPhysical returns every physical NDJSON record,
	// including individual chunk records.
	RecordRepresentationPhysical RecordRepresentation = "PHYSICAL"
)

// RecordFilter selects which records a record query returns.
type RecordFilter struct {
	Types              []string `json:"types,omitempty"`
	FrameID            string   `json:"frameId,omitempty"`
	Route              string   `json:"route,omitempty"`
	MinSequence        *int64   `json:"minSequence,omitempty"`
	MaxSequence        *int64   `json:"maxSequence,omitempty"`
	MinTimestampMillis *int64   `json:"minTimestampMillis,omitempty"`
	MaxTimestampMillis *int64   `json:"maxTimestampMillis,omitempty"`
	AttemptID          string   `json:"attemptId,omitempty"`
	RetrySequenceID    string   `json:"retrySequenceId,omitempty"`
	ValidationStatus   string   `json:"validationStatus,omitempty"`
	FailureID          string   `json:"failureId,omitempty"`
	LiteralText        string   `json:"literalText,omitempty"`
}

// RecordQuery is a bounded, continuable record query.
type RecordQuery struct {
	Handle         artifact.Handle
	Filter         RecordFilter         `json:"filter"`
	Representation RecordRepresentation `json:"representation"`
	InlineContent  bool                 `json:"inlineContent"`
	PageSize       int                  `json:"pageSize"`
	Cursor         string               `json:"cursor,omitempty"`
}

// recordQueryCanonical is the canonical projection for fingerprinting.
type recordQueryCanonical struct {
	Filter         RecordFilter         `json:"filter"`
	Representation RecordRepresentation `json:"representation"`
	InlineContent  bool                 `json:"inlineContent"`
	PageSize       int                  `json:"pageSize"`
}

// QueryRecords returns a finite, continuable page of record summaries. In
// logical representation, chunked-payload envelopes are returned as single
// logical records and chunk records are omitted. In physical representation,
// every physical NDJSON record is returned.
func (service *Service) QueryRecords(ctx context.Context, scopeID evidence.Reference, query RecordQuery) (Page[RecordSummary], *consolecore.Error) {
	pageSize, domain := validatePageSize(scopeID, query.PageSize)
	if domain != nil {
		return Page[RecordSummary]{}, domain
	}
	if query.Representation == "" {
		query.Representation = RecordRepresentationLogical
	}
	if query.Representation != RecordRepresentationPhysical && query.Representation != RecordRepresentationLogical {
		return Page[RecordSummary]{}, consolecore.NewError(consolecore.CodeInvalidArgument,
			"The record representation is not supported.", scopeID.ID(), consolecore.Details{}, nil)
	}
	if domain := validateRecordFilter(scopeID, query.Filter); domain != nil {
		return Page[RecordSummary]{}, domain
	}
	query.Filter.Types = normalizeStringSet(query.Filter.Types)
	fingerprint, err := canonicalizeRequest(recordQueryCanonical{
		Filter:         query.Filter,
		Representation: query.Representation,
		InlineContent:  query.InlineContent,
		PageSize:       pageSize,
	})
	if err != nil {
		return Page[RecordSummary]{}, consolecore.NewError(consolecore.CodeConsoleError,
			"The record query could not be canonicalized.", scopeID.ID(), consolecore.Details{}, err)
	}

	startIdx := 0
	var decodedCursor cursor

	lease, domain := service.leaseForHandle(scopeID, query.Handle)
	if domain != nil {
		return Page[RecordSummary]{}, domain
	}
	success := false
	defer func() { _ = lease.Close(success) }()
	if query.Cursor != "" {
		c, start, cursorDomain := prepareCursor(query.Cursor, ownerCursorKey(lease.Owner()), scopeID.ID(), cursorOpRecords)
		if cursorDomain != nil {
			return Page[RecordSummary]{}, cursorDomain
		}
		decodedCursor = c
		startIdx = start
	}

	if decodedCursor.Schema != "" {
		if d := validateCursorFingerprint(decodedCursor, fingerprint, ownerCursorKey(lease.Owner()), scopeID.ID(), query.Handle); d != nil {
			return Page[RecordSummary]{}, d
		}
	}

	if e := ctx.Err(); e != nil {
		return Page[RecordSummary]{}, canceledError(e)
	}

	indexSize, err := lease.ComponentSize(artifact.ComponentName(ComponentRecordIndex))
	if err != nil {
		return Page[RecordSummary]{}, storageError(scopeID.ID(), err)
	}
	if indexSize%recordIndexRowWidth != 0 {
		return Page[RecordSummary]{}, storageError(scopeID.ID(), fmt.Errorf("record index has invalid size %d", indexSize))
	}
	recordCount := indexSize / recordIndexRowWidth
	indexReader, err := lease.OpenComponent(artifact.ComponentName(ComponentRecordIndex))
	if err != nil {
		return Page[RecordSummary]{}, storageError(scopeID.ID(), err)
	}
	defer indexReader.Close()
	rawReader, err := lease.OpenComponent(artifact.ComponentRawArtifact)
	if err != nil {
		return Page[RecordSummary]{}, storageError(scopeID.ID(), err)
	}
	defer rawReader.Close()
	scanStart := int64(startIdx)
	if query.Cursor == "" && query.Filter.MinSequence != nil {
		scanStart, err = lowerBoundRecordSequence(indexReader, recordCount, *query.Filter.MinSequence)
		if err != nil {
			return Page[RecordSummary]{}, storageError(scopeID.ID(), err)
		}
	}

	traceCtx, err := traceContextForLease(lease, scopeID, query.Handle)
	if err != nil {
		return Page[RecordSummary]{}, storageError(scopeID.ID(), err)
	}
	frameRoots, err := loadFrameRoots(lease)
	if err != nil {
		return Page[RecordSummary]{}, storageError(scopeID.ID(), err)
	}
	items := make([]RecordSummary, 0, pageSize)
	pageRecords := make([]*Record, 0, pageSize)
	pagePositions := make([]int64, 0, pageSize)
	var nextPosition int64
	hasMore := false
	for position := scanStart; position < recordCount; position++ {
		if e := ctx.Err(); e != nil {
			return Page[RecordSummary]{}, canceledError(e)
		}
		row, readErr := readRecordIndexRowAt(indexReader, position)
		if readErr != nil {
			return Page[RecordSummary]{}, storageError(scopeID.ID(), readErr)
		}
		if query.Filter.MaxSequence != nil && row.Sequence > *query.Filter.MaxSequence {
			break
		}
		raw, readErr := readRawRecordBytesFrom(rawReader, row)
		if readErr != nil {
			return Page[RecordSummary]{}, storageError(scopeID.ID(), readErr)
		}
		rec, decodeDomain := decodeRecord(raw, RawAddress{Offset: row.Offset, Length: row.Length, TerminatorLength: row.TerminatorLength})
		if decodeDomain != nil {
			return Page[RecordSummary]{}, storageError(scopeID.ID(), fmt.Errorf("decode record %d: %s", row.Sequence, decodeDomain.Message))
		}
		if query.Representation == RecordRepresentationLogical && rec.IsChunk {
			continue
		}
		if !recordMatchesFilter(rec, query.Filter) {
			continue
		}
		if len(items) == pageSize {
			hasMore = true
			break
		}
		summary := recordToSummary(rec, row, traceCtx, query.Representation)
		items = append(items, summary)
		pageRecords = append(pageRecords, rec)
		pagePositions = append(pagePositions, position)
		nextPosition = position + 1
	}

	var factReader *recordFactReader
	if len(pageRecords) > 0 {
		factReader, err = openRecordFactReader(ctx, lease)
		if err != nil {
			return Page[RecordSummary]{}, storageError(scopeID.ID(), err)
		}
		defer factReader.Close()
	}
	var aggregateInlineBytes int64
	for index, rec := range pageRecords {
		storedFacts, readErr := factReader.Read(ctx, pagePositions[index])
		if readErr != nil {
			if ctx.Err() != nil {
				return Page[RecordSummary]{}, canceledError(ctx.Err())
			}
			return Page[RecordSummary]{}, storageError(scopeID.ID(), readErr)
		}
		items[index].Facts, err = materializeRecordFacts(storedFacts, rec, query.Filter, scopeID, query.Handle, traceCtx)
		if err != nil {
			return Page[RecordSummary]{}, invalidityErrorWithCause(CategoryUnsupportedValue, scopeID.ID(), err)
		}
		if items[index].Facts.Plan != nil {
			items[index].Facts.Plan.RootFrameID = frameRoots[rec.FrameID]
		}
		if query.Representation == RecordRepresentationLogical && rec.IsEnvelope {
			for _, desc := range storedFacts.Payloads {
				if desc.PayloadID != rec.PayloadID {
					continue
				}
				contentRef, refErr := encodeEnvelopeContentReference(scopeID, query.Handle, desc.PayloadID)
				if refErr != nil {
					return Page[RecordSummary]{}, storageError(scopeID.ID(), refErr)
				}
				items[index].Content = &ContentDescriptor{Role: ContentRoleReconstructed, ContentType: desc.ContentType, Encoding: descriptorEncoding(desc.ContentType), RetainedBytes: desc.StoreLength, Available: true, Complete: true, InlineEligibility: desc.StoreLength <= MaxInlineContentBytes, ContentRef: contentRef}
				if query.InlineContent {
					if d := inlineDescriptor(ctx, lease, items[index].Content, desc, &aggregateInlineBytes); d != nil {
						return Page[RecordSummary]{}, d
					}
				}
				break
			}
		} else if query.Representation == RecordRepresentationLogical && rec.Data != nil {
			contentRef, refErr := encodeRecordContentReference(scopeID, query.Handle, rec.Sequence)
			if refErr != nil {
				return Page[RecordSummary]{}, storageError(scopeID.ID(), refErr)
			}
			items[index].Content = &ContentDescriptor{Role: ContentRoleData, ContentType: "application/json", Encoding: ContentEncodingUTF8, RetainedBytes: int64(len(rec.Data)), Available: true, Complete: true, InlineEligibility: len(rec.Data) <= MaxInlineContentBytes, ContentRef: contentRef}
			if query.InlineContent {
				inlineOrdinaryDescriptor(items[index].Content, rec.Data, &aggregateInlineBytes)
			}
		}
	}

	var nextCursor string
	if hasMore {
		nextCursor, err = encodePositionCursor(cursorOpRecords, ownerCursorKey(lease.Owner()), query.Handle, fingerprint, nextPosition)
		if err != nil {
			return Page[RecordSummary]{}, cursorError(scopeID.ID(), err)
		}
	}
	success = true
	return Page[RecordSummary]{
		Context:    traceCtx,
		Items:      items,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

func loadFrameRoots(lease *artifact.Lease) (map[string]string, error) {
	parents := map[string]string{}
	err := scanFactRows[persistedFrameResult](lease, ComponentFrameIndex, 0, func(row persistedFrameResult, _ int64) bool {
		if row.ParentFrameID != nil {
			parents[row.FrameID] = *row.ParentFrameID
		} else {
			parents[row.FrameID] = ""
		}
		return false
	})
	if err != nil {
		return nil, err
	}
	roots := map[string]string{}
	for id := range parents {
		seen := map[string]bool{}
		current := id
		for current != "" && !seen[current] {
			seen[current] = true
			parent, ok := parents[current]
			if !ok || parent == "" {
				roots[id] = current
				break
			}
			current = parent
		}
	}
	return roots, nil
}

// recordMatchesFilter reports whether a record matches all set filter fields.
func recordMatchesFilter(rec *Record, f RecordFilter) bool {
	if len(f.Types) > 0 {
		matched := false
		for _, t := range f.Types {
			if string(rec.Type) == t {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if f.FrameID != "" && rec.FrameID != f.FrameID {
		return false
	}
	if f.Route != "" && rec.Route != f.Route {
		return false
	}
	if f.MinSequence != nil && rec.Sequence < *f.MinSequence {
		return false
	}
	if f.MaxSequence != nil && rec.Sequence > *f.MaxSequence {
		return false
	}
	if f.MinTimestampMillis != nil && rec.TimestampMillis < *f.MinTimestampMillis {
		return false
	}
	if f.MaxTimestampMillis != nil && rec.TimestampMillis > *f.MaxTimestampMillis {
		return false
	}
	if f.AttemptID != "" {
		aid, _, _ := rec.metadataString("attemptId")
		if aid != f.AttemptID {
			return false
		}
	}
	if f.RetrySequenceID != "" {
		rid, _, _ := rec.metadataString("retrySequenceId")
		if rid != f.RetrySequenceID {
			return false
		}
	}
	if f.ValidationStatus != "" {
		status, _, _ := rec.metadataString("status")
		if status != f.ValidationStatus {
			return false
		}
	}
	if f.FailureID != "" {
		fid, _, _ := rec.metadataString("failureId")
		if fid != f.FailureID {
			return false
		}
	}
	if f.LiteralText != "" && !bytes.Contains(rec.Metadata, []byte(f.LiteralText)) && !bytes.Contains(rec.Data, []byte(f.LiteralText)) {
		return false
	}
	return true
}

func validateRecordFilter(scopeID evidence.Reference, filter RecordFilter) *consolecore.Error {
	if filter.MinSequence != nil && *filter.MinSequence <= 0 {
		return consolecore.NewError(consolecore.CodeInvalidArgument, "The minimum sequence must be positive.", scopeID.ID(), consolecore.Details{}, nil)
	}
	if filter.MaxSequence != nil && *filter.MaxSequence <= 0 {
		return consolecore.NewError(consolecore.CodeInvalidArgument, "The maximum sequence must be positive.", scopeID.ID(), consolecore.Details{}, nil)
	}
	if filter.MinSequence != nil && filter.MaxSequence != nil && *filter.MinSequence > *filter.MaxSequence {
		return consolecore.NewError(consolecore.CodeInvalidArgument, "The record sequence range is reversed.", scopeID.ID(), consolecore.Details{}, nil)
	}
	if filter.MinTimestampMillis != nil && filter.MaxTimestampMillis != nil && *filter.MinTimestampMillis > *filter.MaxTimestampMillis {
		return consolecore.NewError(consolecore.CodeInvalidArgument, "The record timestamp range is reversed.", scopeID.ID(), consolecore.Details{}, nil)
	}
	for _, value := range filter.Types {
		if _, ok := knownRecordType(value); !ok {
			return consolecore.NewError(consolecore.CodeInvalidArgument, "The record type filter is not supported.", scopeID.ID(), consolecore.Details{}, nil)
		}
	}
	if filter.ValidationStatus != "" && !knownValidationStatus(filter.ValidationStatus) {
		return consolecore.NewError(consolecore.CodeInvalidArgument, "The validation status filter is not supported.", scopeID.ID(), consolecore.Details{}, nil)
	}
	if filter.LiteralText != "" {
		return validateLiteralText(scopeID, filter.LiteralText)
	}
	return nil
}

func knownValidationStatus(value string) bool {
	return containsClosedValue(ValidationStatusValues(), value)
}

// recordToSummary converts a parsed Record to a RecordSummary.
func recordToSummary(rec *Record, row recordIndexRow, ctx TraceContext, rep RecordRepresentation) RecordSummary {
	repStr := "physical"
	if rep == RecordRepresentationLogical {
		repStr = "logical"
	}
	return RecordSummary{
		Context:         ctx,
		Sequence:        rec.Sequence,
		Type:            string(rec.Type),
		FrameID:         rec.FrameID,
		ParentFrameID:   rec.ParentFrameID,
		FrameType:       string(rec.FrameType),
		Route:           rec.Route,
		ThreadName:      rec.ThreadName,
		TimestampMillis: rec.TimestampMillis,
		Representation:  repStr,
		IsChunk:         rec.IsChunk,
		IsEnvelope:      rec.IsEnvelope,
		PayloadID:       rec.PayloadID,
		Raw:             rec.Raw,
		Facts:           RecordFacts{Attempts: []AttemptSummary{}, Retries: []RetrySummary{}, Validations: []ValidationSummary{}, Failures: []FailureSummary{}, Payloads: []PayloadDescriptor{}, SearchMatches: []SearchResult{}},
	}
}

func descriptorEncoding(contentType string) ContentEncoding {
	if isTextContentType(contentType) {
		return ContentEncodingUTF8
	}
	return ContentEncodingBinary
}

func inlineOrdinaryDescriptor(desc *ContentDescriptor, raw []byte, aggregate *int64) {
	if len(raw) > MaxInlineContentBytes {
		desc.InlineOmission = InlineOmissionPerValue
		return
	}
	if *aggregate+int64(len(raw)) > MaxAggregateInlineContentBytes {
		desc.InlineOmission = InlineOmissionAggregate
		return
	}
	desc.InlineContent = append([]byte(nil), raw...)
	*aggregate += int64(len(raw))
}

func inlineDescriptor(ctx context.Context, lease *artifact.Lease, desc *ContentDescriptor, stored payloadIndexRow, aggregate *int64) *consolecore.Error {
	if stored.StoreLength > MaxInlineContentBytes {
		desc.InlineOmission = InlineOmissionPerValue
		return nil
	}
	if *aggregate+stored.StoreLength > MaxAggregateInlineContentBytes {
		desc.InlineOmission = InlineOmissionAggregate
		return nil
	}
	payload, err := readInlinePayload(ctx, lease, payloadDescriptor{PayloadID: stored.PayloadID, Sequence: stored.Sequence, ContentType: stored.ContentType, ChunkCount: stored.ChunkCount, StoreOffset: stored.StoreOffset, StoreLength: stored.StoreLength})
	if err != nil {
		return storageError(desc.ContentRef, err)
	}
	desc.InlineContent = payload
	*aggregate += stored.StoreLength
	return nil
}

func findPayloadDescriptorInIndex(lease *artifact.Lease, payloadID string) (*payloadDescriptor, error) {
	var found *payloadDescriptor
	err := scanFactRows[payloadIndexRow](lease, ComponentPayloadIndex, 0, func(row payloadIndexRow, _ int64) bool {
		if row.PayloadID != payloadID {
			return false
		}
		desc := payloadDescriptor{
			PayloadID: row.PayloadID, Sequence: row.Sequence, ContentType: row.ContentType,
			ChunkCount: row.ChunkCount, StoreOffset: row.StoreOffset, StoreLength: row.StoreLength,
		}
		found = &desc
		return true
	})
	return found, err
}

// readInlinePayload reads a complete small payload from the payload store.
func readInlinePayload(ctx context.Context, lease *artifact.Lease, desc payloadDescriptor) ([]byte, error) {
	reader, err := lease.OpenComponent(artifact.ComponentName(ComponentPayloadStore))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	stopCancellationClose := closeReaderOnCancellation(ctx, reader)
	defer stopCancellationClose()
	if _, err := reader.Seek(desc.StoreOffset, 0); err != nil {
		return nil, err
	}
	buf := make([]byte, desc.StoreLength)
	if _, err := io.ReadFull(&contextChunkReader{ctx: ctx, reader: reader}, buf); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return buf, nil
}

func closeReaderOnCancellation(ctx context.Context, reader io.Closer) func() bool {
	return context.AfterFunc(ctx, func() { _ = reader.Close() })
}

const contextReadChunkBytes = 64 << 10

type contextChunkReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader *contextChunkReader) Read(buf []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	if len(buf) > contextReadChunkBytes {
		buf = buf[:contextReadChunkBytes]
	}
	n, err := reader.reader.Read(buf)
	if contextErr := reader.ctx.Err(); contextErr != nil {
		return n, contextErr
	}
	return n, err
}
