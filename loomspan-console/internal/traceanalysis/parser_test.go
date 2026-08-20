package traceanalysis

import (
	"context"
	"strings"
	"testing"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
)

// collectRecords runs the parser over raw and returns the parsed records or the
// domain error.
func collectRecords(t *testing.T, raw string) ([]*Record, *consolecore.Error) {
	t.Helper()
	var records []*Record
	_, domain := parseStream(context.Background(), strings.NewReader(raw), func(rec *Record) *consolecore.Error {
		// Copy retained bytes since the parser reuses buffers.
		cp := *rec
		cp.Metadata = append([]byte(nil), rec.Metadata...)
		if rec.Data != nil {
			cp.Data = append([]byte(nil), rec.Data...)
		}
		records = append(records, &cp)
		return nil
	})
	return records, domain
}

// validLine builds a minimal valid TraceRecord line with the given sequence.
func validLine(seq int) string {
	return `{"traceId":"t","sessionId":"s","sequence":` + itoa(seq) +
		`,"timestamp":1784894400.000000000,"recordType":"TRACE_STARTED","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"th","metadata":{"consoleCompatibilityVersion":"development"},"data":null}`
}

// itoa converts an int to a string without importing strconv at the test top.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// TestParserAcceptsLFAndCompleteFinalLine proves the parser accepts LF-terminated
// lines and a complete final line without a trailing newline.
func TestParserAcceptsLFAndCompleteFinalLine(t *testing.T) {
	raw := validLine(1) + "\n" + validLine(2) // no trailing newline
	records, domain := collectRecords(t, raw)
	if domain != nil {
		t.Fatalf("unexpected error: %v", domain)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	if records[1].Sequence != 2 {
		t.Fatalf("expected second record sequence 2, got %d", records[1].Sequence)
	}
}

// TestParserAcceptsCRLFLineEndings proves the parser accepts CRLF terminators.
func TestParserAcceptsCRLFLineEndings(t *testing.T) {
	raw := validLine(1) + "\r\n" + validLine(2) + "\r\n"
	records, domain := collectRecords(t, raw)
	if domain != nil {
		t.Fatalf("unexpected error: %v", domain)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
}

// TestParserRejectsBlankLines proves every physical NDJSON line must contain a
// JSON record; blank lines are not silently discarded.
func TestParserRejectsBlankLines(t *testing.T) {
	raw := validLine(1) + "\n\n" + validLine(2) + "\n"
	_, domain := collectRecords(t, raw)
	if domain == nil {
		t.Fatal("expected an error for a blank NDJSON line")
	}
	category, ok := categoryOf(domain)
	if !ok || category != CategoryMalformedJSON {
		t.Fatalf("expected MALFORMED_JSON, got %v", category)
	}
}

// TestParserRejectsMalformedJSON proves a malformed line is rejected as
// MALFORMED_JSON.
func TestParserRejectsMalformedJSON(t *testing.T) {
	raw := validLine(1) + "\n{not-json}\n"
	_, domain := collectRecords(t, raw)
	if domain == nil {
		t.Fatal("expected error for malformed JSON")
	}
	cat, ok := categoryOf(domain)
	if !ok || cat != CategoryMalformedJSON {
		t.Fatalf("expected MALFORMED_JSON, got %v", cat)
	}
}

// TestParserRejectsTruncatedFinalInput proves a truncated final line is rejected
// as TRUNCATED_INPUT.
func TestParserRejectsTruncatedFinalInput(t *testing.T) {
	line := validLine(1)
	raw := line + "\n" + line[:len(line)-5] // truncated final line
	_, domain := collectRecords(t, raw)
	if domain == nil {
		t.Fatal("expected error for truncated input")
	}
	cat, ok := categoryOf(domain)
	if !ok || cat != CategoryTruncatedInput {
		t.Fatalf("expected TRUNCATED_INPUT, got %v", cat)
	}
}

// TestParserRejectsNonObjectTopLevel proves a non-object top-level JSON value is
// rejected as MALFORMED_JSON.
func TestParserRejectsNonObjectTopLevel(t *testing.T) {
	raw := validLine(1) + "\n[1,2,3]\n"
	_, domain := collectRecords(t, raw)
	if domain == nil {
		t.Fatal("expected error for non-object top level")
	}
	cat, ok := categoryOf(domain)
	if !ok || cat != CategoryMalformedJSON {
		t.Fatalf("expected MALFORMED_JSON, got %v", cat)
	}
}

// TestParserRejectsBlankIdentity proves a record with a blank traceId or
// sessionId is rejected as INCONSISTENT_IDENTITY at the per-record level.
// Cross-record identity consistency is enforced by the processor's validator.
func TestParserRejectsBlankIdentity(t *testing.T) {
	raw := `{"traceId":"","sessionId":"s","sequence":1,"timestamp":1784894400.000000000,"recordType":"TRACE_STARTED","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"th","metadata":{},"data":null}` + "\n"
	_, domain := collectRecords(t, raw)
	if domain == nil {
		t.Fatal("expected error for blank identity")
	}
	cat, ok := categoryOf(domain)
	if !ok || cat != CategoryInconsistentIdentity {
		t.Fatalf("expected INCONSISTENT_IDENTITY, got %v", cat)
	}
}

// TestParserRejectsNonPositiveSequence proves a non-positive sequence is rejected
// as NON_MONOTONIC_SEQUENCE.
func TestParserRejectsNonPositiveSequence(t *testing.T) {
	raw := `{"traceId":"t","sessionId":"s","sequence":0,"timestamp":1784894400.000000000,"recordType":"TRACE_STARTED","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"th","metadata":{},"data":null}` + "\n"
	_, domain := collectRecords(t, raw)
	if domain == nil {
		t.Fatal("expected error for non-positive sequence")
	}
	cat, ok := categoryOf(domain)
	if !ok || cat != CategoryNonMonotonicSequence {
		t.Fatalf("expected NON_MONOTONIC_SEQUENCE, got %v", cat)
	}
}

// TestParserRejectsRemovedPreparedRequest proves obsolete current-version
// vocabulary is rejected rather than silently normalized into a sent request.
func TestParserRejectsRemovedPreparedRequest(t *testing.T) {
	raw := `{"traceId":"t","sessionId":"s","sequence":1,"timestamp":1784894400.000000000,"recordType":"MODEL_REQUEST_PREPARED","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"th","metadata":{},"data":null}` + "\n"
	_, domain := collectRecords(t, raw)
	if domain == nil {
		t.Fatal("expected error for unsupported enum")
	}
	cat, ok := categoryOf(domain)
	if !ok || cat != CategoryUnsupportedValue {
		t.Fatalf("expected UNSUPPORTED_VALUE, got %v", cat)
	}
}

// TestParserEnforcesJSONDepthAt128 proves the parser accepts total JSON depth
// 128 and rejects depth 129 as EXCESSIVE_JSON_DEPTH. The depth scanner counts
// every object/array nesting level, including the record's outer object and the
// metadata object.
func TestParserEnforcesJSONDepthAt128(t *testing.T) {
	// The record outer object is depth 1. REPLACE is the metadata value, so the
	// first {"v":} is depth 2. Nest 127 levels for total depth 128 (accepted),
	// and 128 for total depth 129 (rejected).
	base := `{"traceId":"t","sessionId":"s","sequence":1,"timestamp":1784894400.000000000,"recordType":"TRACE_STARTED","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"th","metadata":REPLACE,"data":null}`
	depth127 := strings.Repeat(`{"v":`, 127) + "null" + strings.Repeat(`}`, 127)
	depth128 := strings.Repeat(`{"v":`, 128) + "null" + strings.Repeat(`}`, 128)

	records, domain := collectRecords(t, strings.Replace(base, "REPLACE", depth127, 1)+"\n")
	if domain != nil {
		t.Fatalf("expected total depth 128 to be accepted, got %v", domain)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}

	_, domain = collectRecords(t, strings.Replace(base, "REPLACE", depth128, 1)+"\n")
	if domain == nil {
		t.Fatal("expected total depth 129 to be rejected")
	}
	cat, ok := categoryOf(domain)
	if !ok || cat != CategoryExcessiveJSONDepth {
		t.Fatalf("expected EXCESSIVE_JSON_DEPTH, got %v", cat)
	}
}

// TestParserEnforcesPhysicalLineLimitAtExactBoundary proves the parser accepts a
// line of exactly maxPhysicalLineBytes bytes and rejects one byte over as
// LINE_TOO_LARGE.
func TestParserEnforcesPhysicalLineLimitAtExactBoundary(t *testing.T) {
	// Build a valid record and pad the threadName to reach exactly the limit.
	prefix := `{"traceId":"t","sessionId":"s","sequence":1,"timestamp":1784894400.000000000,"recordType":"TRACE_STARTED","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"`
	suffix := `","metadata":{},"data":null}`
	needed := maxPhysicalLineBytes - len(prefix) - len(suffix)
	if needed < 0 {
		t.Fatalf("test setup error: prefix+suffix exceed %d bytes", maxPhysicalLineBytes)
	}
	atLimit := prefix + strings.Repeat("x", needed) + suffix
	if len(atLimit) != maxPhysicalLineBytes {
		t.Fatalf("test setup error: at-limit line is %d bytes, want %d", len(atLimit), maxPhysicalLineBytes)
	}
	records, domain := collectRecords(t, atLimit+"\n")
	if domain != nil {
		t.Fatalf("expected %d-byte line to be accepted, got %v", maxPhysicalLineBytes, domain)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}

	overLimit := prefix + strings.Repeat("x", needed+1) + suffix
	_, domain = collectRecords(t, overLimit+"\n")
	if domain == nil {
		t.Fatal("expected one-byte-over line to be rejected")
	}
	cat, ok := categoryOf(domain)
	if !ok || cat != CategoryLineTooLarge {
		t.Fatalf("expected LINE_TOO_LARGE, got %v", cat)
	}
}

// TestParserPreservesUnconsumedMetadataAndDataAsOpaqueJSON proves unconsumed
// metadata and data fields are retained verbatim without requiring a Go field.
func TestParserPreservesUnconsumedMetadataAndDataAsOpaqueJSON(t *testing.T) {
	raw := `{"traceId":"t","sessionId":"s","sequence":1,"timestamp":1784894400.000000000,"recordType":"TRACE_STARTED","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"th","metadata":{"customField":42,"nested":{"a":"b"}},"data":{"opaque":[1,2,3]}}` + "\n"
	records, domain := collectRecords(t, raw)
	if domain != nil {
		t.Fatalf("unexpected error: %v", domain)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if string(records[0].Metadata) != `{"customField":42,"nested":{"a":"b"}}` {
		t.Fatalf("metadata not preserved verbatim: %s", records[0].Metadata)
	}
	if string(records[0].Data) != `{"opaque":[1,2,3]}` {
		t.Fatalf("data not preserved verbatim: %s", records[0].Data)
	}
}

// TestParserStopsPromptlyWhenContextIsCanceled proves the parser respects
// context cancellation.
func TestParserStopsPromptlyWhenContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, domain := parseStream(ctx, strings.NewReader(validLine(1)+"\n"), func(*Record) *consolecore.Error { return nil })
	if domain == nil {
		t.Fatal("expected error for canceled context")
	}
	if domain.Code != consolecore.CodeTargetUnavailable {
		t.Fatalf("expected TARGET_UNAVAILABLE, got %v", domain.Code)
	}
}

// TestParserRejectsTrailingContentAfterObject proves a line with valid JSON
// followed by trailing content is rejected as MALFORMED_JSON.
func TestParserRejectsTrailingContentAfterObject(t *testing.T) {
	raw := validLine(1) + "extra" + "\n"
	_, domain := collectRecords(t, raw)
	if domain == nil {
		t.Fatal("expected error for trailing content")
	}
	cat, ok := categoryOf(domain)
	if !ok || cat != CategoryMalformedJSON {
		t.Fatalf("expected MALFORMED_JSON, got %v", cat)
	}
}

func TestParserRejectsInvalidNullableFieldTypes(t *testing.T) {
	for _, field := range []string{"frameId", "parentFrameId", "frameType", "route"} {
		t.Run(field, func(t *testing.T) {
			line := strings.Replace(validLine(1), `"`+field+`":null`, `"`+field+`":123`, 1)
			_, domain := collectRecords(t, line+"\n")
			if domain == nil {
				t.Fatalf("expected non-string %s to be rejected", field)
			}
			category, ok := categoryOf(domain)
			if !ok || category != CategoryUnsupportedValue {
				t.Fatalf("expected UNSUPPORTED_VALUE, got %v", category)
			}
		})
	}
}

func TestParserRejectsTimestampPrecisionAndOverflow(t *testing.T) {
	for name, timestamp := range map[string]string{
		"more than nine fractional digits": "1784894400.1234567890",
		"millisecond overflow":             "9223372036854776",
	} {
		t.Run(name, func(t *testing.T) {
			line := strings.Replace(validLine(1), "1784894400.000000000", timestamp, 1)
			_, domain := collectRecords(t, line+"\n")
			if domain == nil {
				t.Fatal("expected invalid timestamp to be rejected")
			}
			category, ok := categoryOf(domain)
			if !ok || category != CategoryUnsupportedValue {
				t.Fatalf("expected UNSUPPORTED_VALUE, got %v", category)
			}
		})
	}
}

// TestParserRecordsCorrectRawByteOffsets proves the raw byte offset, length, and
// terminator length recorded for each record are correct, including for records
// after the first. This protects raw-record addressing and binary-search lookup
// over the fixed-width record-address index.
func TestParserRecordsCorrectRawByteOffsets(t *testing.T) {
	raw := validLine(1) + "\n" + validLine(2) + "\n" + validLine(3) + "\r\n"
	records, domain := collectRecords(t, raw)
	if domain != nil {
		t.Fatalf("unexpected error: %v", domain)
	}
	if len(records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(records))
	}
	// Record 1 starts at offset 0.
	if records[0].Raw.Offset != 0 {
		t.Errorf("record 0 offset: got %d want 0", records[0].Raw.Offset)
	}
	if records[0].Raw.Length != int64(len(validLine(1))) {
		t.Errorf("record 0 length: got %d want %d", records[0].Raw.Length, int64(len(validLine(1))))
	}
	if records[0].Raw.TerminatorLength != 1 {
		t.Errorf("record 0 terminator: got %d want 1", records[0].Raw.TerminatorLength)
	}
	// Record 2 starts after record 1's content + LF terminator.
	rec1Start := int64(len(validLine(1)) + 1)
	if records[1].Raw.Offset != rec1Start {
		t.Errorf("record 1 offset: got %d want %d", records[1].Raw.Offset, rec1Start)
	}
	if records[1].Raw.Length != int64(len(validLine(2))) {
		t.Errorf("record 1 length: got %d want %d", records[1].Raw.Length, int64(len(validLine(2))))
	}
	if records[1].Raw.TerminatorLength != 1 {
		t.Errorf("record 1 terminator: got %d want 1", records[1].Raw.TerminatorLength)
	}
	// Record 3 starts after record 2's content + LF terminator, uses CRLF.
	rec2Start := rec1Start + int64(len(validLine(2))) + 1
	if records[2].Raw.Offset != rec2Start {
		t.Errorf("record 2 offset: got %d want %d", records[2].Raw.Offset, rec2Start)
	}
	if records[2].Raw.Length != int64(len(validLine(3))) {
		t.Errorf("record 2 length: got %d want %d", records[2].Raw.Length, int64(len(validLine(3))))
	}
	if records[2].Raw.TerminatorLength != 2 {
		t.Errorf("record 2 terminator: got %d want 2 (CRLF)", records[2].Raw.TerminatorLength)
	}
}

// TestParserRejectsInvalidUTF8Records proves a record containing invalid UTF-8
// bytes is rejected. The JSON decoder rejects bytes that are not valid UTF-8
// inside string values.
func TestParserRejectsInvalidUTF8Records(t *testing.T) {
	// Build a record with an invalid UTF-8 byte sequence inside the threadName
	// string value. The byte 0xFF is not valid UTF-8 in this context.
	raw := validLine(1) + "\n" +
		`{"traceId":"t","sessionId":"s","sequence":2,"timestamp":1784894400.000000000,"recordType":"TRACE_STARTED","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"` + "\xff\xfe" + `","metadata":{},"data":null}` + "\n"
	_, domain := collectRecords(t, raw)
	if domain == nil {
		t.Fatal("expected error for invalid UTF-8")
	}
	// The JSON decoder rejects invalid UTF-8 as a malformed JSON error.
	cat, ok := categoryOf(domain)
	if !ok || (cat != CategoryMalformedJSON && cat != CategoryUnsupportedValue) {
		t.Fatalf("expected MALFORMED_JSON or UNSUPPORTED_VALUE, got %v", cat)
	}
}

// TestParserRejectsIdentitySequenceTimestampEnumAndIntegerContradictions
// proves the parser rejects records with contradictory identity, sequence,
// timestamp, enum, and integer values. Each subtest targets one contradiction.
func TestParserRejectsIdentitySequenceTimestampEnumAndIntegerContradictions(t *testing.T) {
	t.Run("non_monotonic_sequence", func(t *testing.T) {
		// Sequence 2 followed by sequence 1 (decreasing). The parser itself
		// accepts both records; the validator (run by the processor) rejects
		// the non-monotonic sequence.
		raw := validLine(2) + "\n" +
			`{"traceId":"t","sessionId":"s","sequence":1,"timestamp":1784894400.000000000,"recordType":"TRACE_STARTED","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"th","metadata":{},"data":null}` + "\n" +
			completionRecord(3, "SUCCEEDED", 0, 0, 0, "") + "\n"
		_, cat, ok := processTrace(t, raw)
		if ok {
			t.Fatal("expected error for non-monotonic sequence")
		}
		if cat != CategoryNonMonotonicSequence {
			t.Fatalf("expected NON_MONOTONIC_SEQUENCE, got %v", cat)
		}
	})

	t.Run("invalid_timestamp_format", func(t *testing.T) {
		// Timestamp is a string, not a number. The JSON decoder rejects this as
		// a type mismatch (MALFORMED_JSON), not UNSUPPORTED_VALUE.
		raw := `{"traceId":"t","sessionId":"s","sequence":1,"timestamp":"not-a-number","recordType":"TRACE_STARTED","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"th","metadata":{},"data":null}` + "\n"
		_, domain := collectRecords(t, raw)
		if domain == nil {
			t.Fatal("expected error for invalid timestamp")
		}
		cat, ok := categoryOf(domain)
		if !ok || (cat != CategoryUnsupportedValue && cat != CategoryMalformedJSON) {
			t.Fatalf("expected UNSUPPORTED_VALUE or MALFORMED_JSON, got %v", cat)
		}
	})

	t.Run("negative_timestamp", func(t *testing.T) {
		// Negative timestamp (before epoch) is rejected by parseTimestamp.
		raw := `{"traceId":"t","sessionId":"s","sequence":1,"timestamp":-1784894400.000000000,"recordType":"TRACE_STARTED","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"th","metadata":{},"data":null}` + "\n"
		_, domain := collectRecords(t, raw)
		if domain == nil {
			t.Fatal("expected error for negative timestamp")
		}
		cat, ok := categoryOf(domain)
		if !ok || cat != CategoryUnsupportedValue {
			t.Fatalf("expected UNSUPPORTED_VALUE, got %v", cat)
		}
	})

	t.Run("unknown_frame_type", func(t *testing.T) {
		// An unknown frameType enum value.
		raw := `{"traceId":"t","sessionId":"s","sequence":1,"timestamp":1784894400.000000000,"recordType":"FRAME_OPENED","frameId":"f","parentFrameId":null,"frameType":"FUTURE_FRAME_TYPE","route":null,"threadName":"th","metadata":{},"data":null}` + "\n"
		_, domain := collectRecords(t, raw)
		if domain == nil {
			t.Fatal("expected error for unknown frame type")
		}
		cat, ok := categoryOf(domain)
		if !ok || cat != CategoryUnsupportedValue {
			t.Fatalf("expected UNSUPPORTED_VALUE, got %v", cat)
		}
	})

	t.Run("sequence_overflow", func(t *testing.T) {
		// A sequence number that overflows int64.
		raw := `{"traceId":"t","sessionId":"s","sequence":99999999999999999999999,"timestamp":1784894400.000000000,"recordType":"TRACE_STARTED","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"th","metadata":{},"data":null}` + "\n"
		_, domain := collectRecords(t, raw)
		if domain == nil {
			t.Fatal("expected error for sequence overflow")
		}
		// An overflowed integer is either NON_MONOTONIC_SEQUENCE (parseInteger
		// fails) or MALFORMED_JSON depending on the decoder path.
		cat, ok := categoryOf(domain)
		if !ok || (cat != CategoryNonMonotonicSequence && cat != CategoryMalformedJSON) {
			t.Fatalf("expected NON_MONOTONIC_SEQUENCE or MALFORMED_JSON, got %v", cat)
		}
	})

	t.Run("decimal_timestamp_with_fractional_seconds", func(t *testing.T) {
		// A valid decimal-second timestamp with fractional seconds must be
		// accepted and truncated to milliseconds.
		raw := `{"traceId":"t","sessionId":"s","sequence":1,"timestamp":1784894400.123456789,"recordType":"TRACE_STARTED","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"th","metadata":{},"data":null}` + "\n"
		records, domain := collectRecords(t, raw)
		if domain != nil {
			t.Fatalf("expected valid decimal timestamp, got error: %v", domain)
		}
		if len(records) != 1 {
			t.Fatalf("expected 1 record, got %d", len(records))
		}
		// 1784894400.123456789 → millis = 1784894400*1000 + 123 = 1784894400123
		if records[0].TimestampMillis != 1784894400123 {
			t.Errorf("timestamp millis: got %d want 1784894400123", records[0].TimestampMillis)
		}
	})
}
