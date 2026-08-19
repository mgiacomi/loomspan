package traceanalysis

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/evidence"
)

const searchReadBufferSize = 64 << 10

// SearchQuery is a bounded, continuable literal-text search across physical
// records and reconstructed logical payloads.
type SearchQuery struct {
	Handle   artifact.Handle
	Text     string
	Filter   RecordFilter
	PageSize int
	Cursor   string
}

type searchQueryCanonical struct {
	Text     string       `json:"text"`
	Filter   RecordFilter `json:"filter"`
	PageSize int          `json:"pageSize"`
}

// Search scans physical record bytes followed by reconstructed logical
// payload bytes. Each call consumes at most maxSearchWorkBytes and
// maxSearchWorkRecords, and a cursor can stop within either kind of value.
func (service *Service) Search(ctx context.Context, scopeID evidence.Reference, query SearchQuery) (Page[SearchResult], *consolecore.Error) {
	if query.Text == "" {
		return Page[SearchResult]{}, consolecore.NewError(consolecore.CodeInvalidArgument,
			"The search text must not be empty.", scopeID.ID(), consolecore.Details{}, nil)
	}
	if domain := validateLiteralText(scopeID, query.Text); domain != nil {
		return Page[SearchResult]{}, domain
	}
	query.Filter.LiteralText = ""
	if domain := validateRecordFilter(scopeID, query.Filter); domain != nil {
		return Page[SearchResult]{}, domain
	}
	query.Filter.Types = normalizeStringSet(query.Filter.Types)
	pageSize, domain := validatePageSize(scopeID, query.PageSize)
	if domain != nil {
		return Page[SearchResult]{}, domain
	}
	fingerprint, err := canonicalizeRequest(searchQueryCanonical{Text: query.Text, Filter: query.Filter, PageSize: pageSize})
	if err != nil {
		return Page[SearchResult]{}, consolecore.NewError(consolecore.CodeConsoleError,
			"The search query could not be canonicalized.", scopeID.ID(), consolecore.Details{}, err)
	}

	lease, decoded, _, domain := service.leaseForCursor(scopeID, query.Handle, query.Cursor, cursorOpSearch)
	if domain != nil {
		return Page[SearchResult]{}, domain
	}
	success := false
	defer func() { _ = lease.Close(success) }()
	state := searchCursorState{Phase: "records"}
	if decoded.Schema != "" {
		if decoded.SearchState == nil {
			return Page[SearchResult]{}, cursorError(scopeID.ID(), fmt.Errorf("search continuation has no progress state"))
		}
		if d := validateCursorFingerprint(decoded, fingerprint, ownerCursorKey(lease.Owner()), scopeID.ID(), query.Handle); d != nil {
			return Page[SearchResult]{}, d
		}
		state = *decoded.SearchState
	}
	if err := ctx.Err(); err != nil {
		return Page[SearchResult]{}, canceledError(err)
	}

	m, err := readManifest(lease)
	if err != nil {
		return Page[SearchResult]{}, storageError(scopeID.ID(), err)
	}
	traceCtx := TraceContext{Evidence: scopeID, Handle: query.Handle, TraceID: m.TraceID, SessionID: m.SessionID}
	needle := []byte(query.Text)
	if decoded.Schema != "" {
		if state.KMPPartial >= len(needle) || state.RecordPosition > m.RecordCount ||
			state.PayloadPosition > int64(m.PayloadCount) {
			return Page[SearchResult]{}, cursorError(scopeID.ID(), fmt.Errorf("search continuation progress is out of bounds"))
		}
		if state.Phase == "records" && (state.PayloadPosition != 0 || state.PayloadIndexOffset != 0) {
			return Page[SearchResult]{}, cursorError(scopeID.ID(), fmt.Errorf("record search continuation contains payload progress"))
		}
	}
	failure := buildKMPFailureTable(needle)
	items := make([]SearchResult, 0, pageSize)
	var workBytes int64
	var workRecords int

	if state.Phase == "records" {
		indexSize, err := lease.ComponentSize(artifact.ComponentName(ComponentRecordIndex))
		if err != nil || indexSize%recordIndexRowWidth != 0 {
			if err == nil {
				err = fmt.Errorf("record index has invalid size %d", indexSize)
			}
			return Page[SearchResult]{}, storageError(scopeID.ID(), err)
		}
		recordCount := indexSize / recordIndexRowWidth
		indexReader, err := lease.OpenComponent(artifact.ComponentName(ComponentRecordIndex))
		if err != nil {
			return Page[SearchResult]{}, storageError(scopeID.ID(), err)
		}
		defer indexReader.Close()
		rawReader, err := lease.OpenComponent(artifact.ComponentRawArtifact)
		if err != nil {
			return Page[SearchResult]{}, storageError(scopeID.ID(), err)
		}
		defer rawReader.Close()

		for state.RecordPosition < recordCount {
			if err := ctx.Err(); err != nil {
				return Page[SearchResult]{}, canceledError(err)
			}
			if workBytes >= maxSearchWorkBytes || workRecords >= maxSearchWorkRecords {
				return service.finishSearchPage(scopeID, ownerCursorKey(lease.Owner()), query.Handle, fingerprint, traceCtx, state, items, true, &success)
			}
			row, err := readRecordIndexRowAt(indexReader, state.RecordPosition)
			if err != nil {
				return Page[SearchResult]{}, storageError(scopeID.ID(), err)
			}
			raw, err := readRawRecordBytesFrom(rawReader, row)
			if err != nil {
				return Page[SearchResult]{}, storageError(scopeID.ID(), err)
			}
			record, decodeDomain := decodeRecord(raw, RawAddress{Offset: row.Offset, Length: row.Length, TerminatorLength: row.TerminatorLength})
			if decodeDomain != nil {
				return Page[SearchResult]{}, storageError(scopeID.ID(), fmt.Errorf("decode record %d: %s", row.Sequence, decodeDomain.Message))
			}
			if record.IsChunk || !recordMatchesFilter(record, query.Filter) {
				state.RecordPosition++
				state.RecordField, state.ByteOffset, state.KMPPartial = "", 0, 0
				workRecords++
				continue
			}
			fields := []struct {
				name string
				data []byte
				ref  string
			}{{name: "metadata", data: record.Metadata}}
			if !record.IsEnvelope && record.Data != nil {
				contentRef, encodeErr := encodeRecordContentReference(scopeID, query.Handle, record.Sequence)
				if encodeErr != nil {
					return Page[SearchResult]{}, storageError(scopeID.ID(), encodeErr)
				}
				fields = append(fields, struct {
					name string
					data []byte
					ref  string
				}{name: "content", data: record.Data, ref: contentRef})
			}
			fieldIndex := 0
			if state.RecordField == "content" {
				fieldIndex = 1
			}
			if fieldIndex >= len(fields) {
				return Page[SearchResult]{}, cursorError(scopeID.ID(), fmt.Errorf("search record field is unavailable"))
			}
			for ; fieldIndex < len(fields); fieldIndex++ {
				field := fields[fieldIndex]
				state.RecordField = field.name
				if state.ByteOffset > int64(len(field.data)) {
					return Page[SearchResult]{}, cursorError(scopeID.ID(), fmt.Errorf("search record offset is out of bounds"))
				}
				limit := int64(len(field.data))
				if remaining := maxSearchWorkBytes - workBytes; limit-state.ByteOffset > remaining {
					limit = state.ByteOffset + remaining
				}
				for state.ByteOffset < limit {
					state.KMPPartial = advanceKMP(field.data[state.ByteOffset], needle, failure, state.KMPPartial)
					state.ByteOffset++
					workBytes++
					if state.KMPPartial == len(needle) {
						items = append(items, SearchResult{Context: traceCtx, Sequence: row.Sequence, RecordType: string(record.Type), FrameID: record.FrameID, MatchOffset: state.ByteOffset - int64(len(needle)), MatchLength: len(needle), SearchedField: field.name, ContentRef: field.ref})
						state.KMPPartial = failure[state.KMPPartial-1]
						if len(items) == pageSize {
							return service.finishSearchPage(scopeID, ownerCursorKey(lease.Owner()), query.Handle, fingerprint, traceCtx, state, items, true, &success)
						}
					}
				}
				if state.ByteOffset < int64(len(field.data)) {
					return service.finishSearchPage(scopeID, ownerCursorKey(lease.Owner()), query.Handle, fingerprint, traceCtx, state, items, true, &success)
				}
				state.ByteOffset, state.KMPPartial = 0, 0
			}
			state.RecordPosition++
			state.RecordField = ""
			workRecords++
		}
		state = searchCursorState{Phase: "payloads"}
	}

	if state.PayloadPosition < int64(m.PayloadCount) {
		payloadIndexSize, err := lease.ComponentSize(artifact.ComponentName(ComponentPayloadIndex))
		if err != nil {
			return Page[SearchResult]{}, storageError(scopeID.ID(), err)
		}
		if state.PayloadIndexOffset >= payloadIndexSize {
			if decoded.Schema != "" {
				return Page[SearchResult]{}, cursorError(scopeID.ID(), fmt.Errorf("payload index continuation offset is out of bounds"))
			}
			return Page[SearchResult]{}, storageError(scopeID.ID(), fmt.Errorf("payload index is empty or truncated"))
		}
		payloadIndex, err := lease.OpenComponent(artifact.ComponentName(ComponentPayloadIndex))
		if err != nil {
			return Page[SearchResult]{}, storageError(scopeID.ID(), err)
		}
		defer payloadIndex.Close()
		payloadStore, err := lease.OpenComponent(artifact.ComponentName(ComponentPayloadStore))
		if err != nil {
			return Page[SearchResult]{}, storageError(scopeID.ID(), err)
		}
		defer payloadStore.Close()
		var filterRecordIndex artifact.ComponentReader
		var filterRawRecords artifact.ComponentReader
		var filterRecordCount int64
		if hasRecordSelectionFilter(query.Filter) {
			recordIndexSize, sizeErr := lease.ComponentSize(artifact.ComponentName(ComponentRecordIndex))
			if sizeErr != nil || recordIndexSize%recordIndexRowWidth != 0 {
				if sizeErr == nil {
					sizeErr = fmt.Errorf("record index has invalid size %d", recordIndexSize)
				}
				return Page[SearchResult]{}, storageError(scopeID.ID(), sizeErr)
			}
			filterRecordCount = recordIndexSize / recordIndexRowWidth
			filterRecordIndex, err = lease.OpenComponent(artifact.ComponentName(ComponentRecordIndex))
			if err != nil {
				return Page[SearchResult]{}, storageError(scopeID.ID(), err)
			}
			defer filterRecordIndex.Close()
			filterRawRecords, err = lease.OpenComponent(artifact.ComponentRawArtifact)
			if err != nil {
				return Page[SearchResult]{}, storageError(scopeID.ID(), err)
			}
			defer filterRawRecords.Close()
		}
		if _, err := payloadIndex.Seek(state.PayloadIndexOffset, io.SeekStart); err != nil {
			return Page[SearchResult]{}, storageError(scopeID.ID(), err)
		}
		for position := state.PayloadPosition; position < int64(m.PayloadCount); position++ {
			if workBytes >= maxSearchWorkBytes || workRecords >= maxSearchWorkRecords {
				return service.finishSearchPage(scopeID, ownerCursorKey(lease.Owner()), query.Handle, fingerprint, traceCtx, state, items, true, &success)
			}
			rowOffset := state.PayloadIndexOffset
			encoded, err := readLengthPrefixed(payloadIndex)
			if err != nil {
				if decoded.Schema != "" {
					return Page[SearchResult]{}, cursorError(scopeID.ID(), fmt.Errorf("payload index continuation does not address a row: %w", err))
				}
				return Page[SearchResult]{}, storageError(scopeID.ID(), fmt.Errorf("read payload index: %w", err))
			}
			nextIndexOffset := rowOffset + int64(4+len(encoded))
			workBytes += int64(4 + len(encoded))
			workRecords++
			var payload payloadIndexRow
			if err := json.Unmarshal(encoded, &payload); err != nil {
				if decoded.Schema != "" {
					return Page[SearchResult]{}, cursorError(scopeID.ID(), fmt.Errorf("payload index continuation does not address a row: %w", err))
				}
				return Page[SearchResult]{}, storageError(scopeID.ID(), fmt.Errorf("parse payload index: %w", err))
			}
			if state.ByteOffset > payload.StoreLength {
				if decoded.Schema != "" {
					return Page[SearchResult]{}, cursorError(scopeID.ID(), fmt.Errorf("search payload offset is out of bounds"))
				}
				return Page[SearchResult]{}, storageError(scopeID.ID(), fmt.Errorf("search payload offset is out of bounds"))
			}
			if hasRecordSelectionFilter(query.Filter) {
				recordPosition, recordErr := lowerBoundRecordSequence(filterRecordIndex, filterRecordCount, payload.Sequence)
				if recordErr != nil || recordPosition == filterRecordCount {
					if recordErr == nil {
						recordErr = fmt.Errorf("payload record %d was not found", payload.Sequence)
					}
					return Page[SearchResult]{}, storageError(scopeID.ID(), recordErr)
				}
				recordRow, recordErr := readRecordIndexRowAt(filterRecordIndex, recordPosition)
				if recordErr != nil || recordRow.Sequence != payload.Sequence {
					if recordErr == nil {
						recordErr = fmt.Errorf("payload record %d was not found", payload.Sequence)
					}
					return Page[SearchResult]{}, storageError(scopeID.ID(), recordErr)
				}
				raw, readErr := readRawRecordBytesFrom(filterRawRecords, recordRow)
				if readErr != nil {
					return Page[SearchResult]{}, storageError(scopeID.ID(), readErr)
				}
				workBytes += int64(len(raw))
				record, decodeDomain := decodeRecord(raw, RawAddress{Offset: recordRow.Offset, Length: recordRow.Length, TerminatorLength: recordRow.TerminatorLength})
				if decodeDomain != nil {
					return Page[SearchResult]{}, storageError(scopeID.ID(), fmt.Errorf("decode record %d: %s", recordRow.Sequence, decodeDomain.Message))
				}
				if !recordMatchesFilter(record, query.Filter) {
					state.PayloadPosition++
					state.PayloadIndexOffset = nextIndexOffset
					state.ByteOffset = 0
					state.KMPPartial = 0
					continue
				}
			}
			if !isTextContentType(payload.ContentType) {
				state.ExcludedBinary = true
				state.PayloadPosition++
				state.PayloadIndexOffset = nextIndexOffset
				state.ByteOffset = 0
				state.KMPPartial = 0
				continue
			}
			// Index reads are part of the per-call work budget. If this row used
			// the remaining budget, continue it on the next call; its direct row
			// offset prevents replaying earlier descriptors.
			if workBytes >= maxSearchWorkBytes && state.ByteOffset < payload.StoreLength {
				return service.finishSearchPage(scopeID, ownerCursorKey(lease.Owner()), query.Handle, fingerprint, traceCtx, state, items, true, &success)
			}
			if _, err := payloadStore.Seek(payload.StoreOffset+state.ByteOffset, io.SeekStart); err != nil {
				return Page[SearchResult]{}, storageError(scopeID.ID(), err)
			}
			buffer := make([]byte, searchReadBufferSize)
			for state.ByteOffset < payload.StoreLength {
				if err := ctx.Err(); err != nil {
					return Page[SearchResult]{}, canceledError(err)
				}
				if workBytes >= maxSearchWorkBytes {
					return service.finishSearchPage(scopeID, ownerCursorKey(lease.Owner()), query.Handle, fingerprint, traceCtx, state, items, true, &success)
				}
				want := min(int64(len(buffer)), payload.StoreLength-state.ByteOffset, maxSearchWorkBytes-workBytes)
				n, err := io.ReadFull(payloadStore, buffer[:want])
				if err != nil {
					return Page[SearchResult]{}, storageError(scopeID.ID(), err)
				}
				for _, b := range buffer[:n] {
					state.KMPPartial = advanceKMP(b, needle, failure, state.KMPPartial)
					state.ByteOffset++
					workBytes++
					if state.KMPPartial == len(needle) {
						contentRef, encodeErr := encodeEnvelopeContentReference(scopeID, query.Handle, payload.PayloadID)
						if encodeErr != nil {
							return Page[SearchResult]{}, storageError(scopeID.ID(), encodeErr)
						}
						items = append(items, SearchResult{Context: traceCtx, Sequence: payload.Sequence,
							MatchOffset: state.ByteOffset - int64(len(needle)), MatchLength: len(needle), SearchedField: "content", ContentRef: contentRef})
						state.KMPPartial = failure[state.KMPPartial-1]
						if len(items) == pageSize {
							more := state.ByteOffset < payload.StoreLength || position+1 < int64(m.PayloadCount)
							return service.finishSearchPage(scopeID, ownerCursorKey(lease.Owner()), query.Handle, fingerprint, traceCtx, state, items, more, &success)
						}
					}
				}
			}
			state.PayloadPosition++
			state.PayloadIndexOffset = nextIndexOffset
			state.ByteOffset = 0
			state.KMPPartial = 0
		}
	}
	success = true
	return Page[SearchResult]{Context: traceCtx, Items: items, SearchLimitations: searchLimitations(state)}, nil
}

func hasRecordSelectionFilter(filter RecordFilter) bool {
	return len(filter.Types) > 0 || filter.FrameID != "" || filter.Route != "" ||
		filter.MinSequence != nil || filter.MaxSequence != nil ||
		filter.MinTimestampMillis != nil || filter.MaxTimestampMillis != nil ||
		filter.AttemptID != "" || filter.RetrySequenceID != "" ||
		filter.ValidationStatus != "" || filter.FailureID != ""
}

func (service *Service) finishSearchPage(scopeID evidence.Reference, ownerKey string, handle artifact.Handle, fingerprint string,
	traceCtx TraceContext, state searchCursorState, items []SearchResult, hasMore bool, success *bool) (Page[SearchResult], *consolecore.Error) {
	var next string
	var err error
	if hasMore {
		next, err = encodeSearchCursor(ownerKey, handle, fingerprint, state)
		if err != nil {
			return Page[SearchResult]{}, cursorError(scopeID.ID(), err)
		}
	}
	*success = true
	return Page[SearchResult]{Context: traceCtx, Items: items, NextCursor: next, HasMore: hasMore, SearchLimitations: searchLimitations(state)}, nil
}

func searchLimitations(state searchCursorState) []SearchLimitation {
	limitations := []SearchLimitation{}
	if state.ExcludedBinary {
		limitations = append(limitations, SearchLimitation{
			Code:    "BINARY_CONTENT_EXCLUDED",
			Message: "Binary semantic content was excluded from exact literal-text search.",
		})
	}
	return limitations
}

func advanceKMP(b byte, needle []byte, failure []int, partial int) int {
	for partial > 0 && b != needle[partial] {
		partial = failure[partial-1]
	}
	if b == needle[partial] {
		partial++
	}
	return partial
}

func buildKMPFailureTable(needle []byte) []int {
	failure := make([]int, len(needle))
	for i := 1; i < len(needle); i++ {
		j := failure[i-1]
		for j > 0 && needle[i] != needle[j] {
			j = failure[j-1]
		}
		if needle[i] == needle[j] {
			j++
		}
		failure[i] = j
	}
	return failure
}

func kmpSearch(haystack, needle []byte, failure []int, partial int) ([]int64, int) {
	var matches []int64
	for i, b := range haystack {
		partial = advanceKMP(b, needle, failure, partial)
		if partial == len(needle) {
			matches = append(matches, int64(i-partial+1))
			partial = failure[partial-1]
		}
	}
	return matches, partial
}
