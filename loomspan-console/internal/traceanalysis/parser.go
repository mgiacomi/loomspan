package traceanalysis

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
)

// recordFunc receives one parsed physical record. The parser reuses the Record's
// backing buffer only until the next record, so the callback must copy any bytes
// it needs to retain (metadata, data, identifiers). Returning a non-nil error
// stops parsing and is propagated unchanged.
type recordFunc func(*Record) *consolecore.Error

// parseStream reads the raw NDJSON stream, validates framing and per-record
// structure, and invokes cb for each physical record in canonical order. It
// keeps at most one bounded physical line in memory; the callback owns any
// retained bytes. recordCount is the number of physical records delivered.
func parseStream(ctx context.Context, raw io.Reader, cb recordFunc) (recordCount int, err *consolecore.Error) {
	reader := bufio.NewReaderSize(raw, maxPhysicalLineBytes+2)
	var offset int64 // bytes consumed from the stream so far
	for {
		if e := ctx.Err(); e != nil {
			return recordCount, canceledError(e)
		}
		line, consumed, readErr := readPhysicalLine(reader)
		contentStart := offset
		// content excludes the terminator; consumed is the terminator byte count.
		content := line[:len(line)-int(consumed)]
		if len(content) == 0 {
			if consumed > 0 {
				return recordCount, invalidityError(CategoryMalformedJSON, "")
			}
			offset += int64(consumed)
			if readErr != nil {
				return recordCount, finishRead(readErr)
			}
			continue
		}
		if len(content) > maxPhysicalLineBytes {
			return recordCount, invalidityError(CategoryLineTooLarge, "")
		}
		rec, domain := decodeRecord(content, RawAddress{
			Offset:           contentStart,
			Length:           int64(len(content)),
			TerminatorLength: int64(consumed),
		})
		if domain != nil {
			return recordCount, domain
		}
		if e := ctx.Err(); e != nil {
			return recordCount, canceledError(e)
		}
		if domain := cb(rec); domain != nil {
			return recordCount, domain
		}
		recordCount++
		offset += int64(len(line))
		if readErr != nil {
			return recordCount, finishRead(readErr)
		}
	}
}

// readPhysicalLine reads one NDJSON line from reader, returning the full line
// bytes (including terminator), the number of terminator bytes consumed, and any
// read error. A line exceeding the buffer without a terminator is drained so the
// caller can surface LINE_TOO_LARGE without misreading the next record; in that
// case the returned content is marked oversized.
func readPhysicalLine(reader *bufio.Reader) (line []byte, termConsumed int64, readErr error) {
	slice, err := reader.ReadSlice('\n')
	line = append([]byte(nil), slice...)
	if err == nil {
		return line, int64(terminatorLength(slice)), nil
	}
	if err == bufio.ErrBufferFull {
		// Drain the remainder of the oversized line.
		drainUntilNewline(reader)
		// Signal oversize by returning content longer than the bound.
		oversized := make([]byte, maxPhysicalLineBytes+2)
		return oversized, 0, nil
	}
	// Non-EOF error or EOF with partial content. For EOF, content (if any) is a
	// complete final line without a terminator. For other read errors the
	// partial slice may not contain a reliable terminator, so report zero in
	// both cases to avoid misaccounting content bytes as terminator bytes.
	return line, 0, err
}

// drainUntilNewline reads and discards bytes until a newline or EOF.
func drainUntilNewline(reader *bufio.Reader) {
	for {
		_, err := reader.ReadSlice('\n')
		if err == nil || err == io.EOF {
			return
		}
		if err != bufio.ErrBufferFull {
			return
		}
	}
}

// terminatorLength returns the number of trailing CR/LF bytes in line.
func terminatorLength(line []byte) int {
	n := 0
	for i := len(line) - 1; i >= 0; i-- {
		c := line[i]
		if c == '\n' || c == '\r' {
			n++
			continue
		}
		break
	}
	return n
}

// rawRecord is the strict intermediate decode target. Nullable identifier
// fields are kept as json.RawMessage so null, absent, and blank are
// distinguishable; numeric fields use json.Number for exact parsing.
type rawRecord struct {
	TraceID       string          `json:"traceId"`
	SessionID     string          `json:"sessionId"`
	Sequence      json.Number     `json:"sequence"`
	Timestamp     json.Number     `json:"timestamp"`
	RecordType    string          `json:"recordType"`
	FrameID       json.RawMessage `json:"frameId"`
	ParentFrameID json.RawMessage `json:"parentFrameId"`
	FrameType     json.RawMessage `json:"frameType"`
	Route         json.RawMessage `json:"route"`
	ThreadName    string          `json:"threadName"`
	Metadata      json.RawMessage `json:"metadata"`
	Data          json.RawMessage `json:"data"`
}

// decodeRecord validates framing, depth, and per-record structure, returning a
// populated Record or a domain error carrying the exact invalidity category.
func decodeRecord(content []byte, address RawAddress) (*Record, *consolecore.Error) {
	if depth, _ := jsonDepth(content); depth > maxJSONDepth {
		return nil, invalidityError(CategoryExcessiveJSONDepth, "")
	}
	if !isObject(content) {
		return nil, invalidityError(CategoryMalformedJSON, "")
	}
	// Reject invalid UTF-8 before the JSON decoder, which would otherwise accept
	// some invalid sequences by replacing them with U+FFFD. Invalid UTF-8 is a
	// structural violation of the NDJSON record contract.
	if !utf8.Valid(content) {
		return nil, invalidityError(CategoryMalformedJSON, "")
	}
	var raw rawRecord
	dec := json.NewDecoder(bytes.NewReader(content))
	dec.UseNumber()
	if err := dec.Decode(&raw); err != nil {
		return nil, parseErrorCategory(err)
	}
	// Reject any non-whitespace content after the single object.
	var trailing json.RawMessage
	if err := dec.Decode(&trailing); err != io.EOF {
		return nil, invalidityError(CategoryMalformedJSON, "")
	}

	rec := &Record{Raw: address}
	rec.ThreadName = raw.ThreadName
	if strings.TrimSpace(rec.ThreadName) == "" {
		rec.ThreadName = "unknown"
	}

	// Identity: traceId and sessionId must be present and non-blank.
	if strings.TrimSpace(raw.TraceID) == "" || strings.TrimSpace(raw.SessionID) == "" {
		return nil, invalidityError(CategoryInconsistentIdentity, "")
	}
	rec.TraceID = raw.TraceID
	rec.SessionID = raw.SessionID

	// Sequence: positive integer.
	seq, ok := parseInteger(raw.Sequence)
	if !ok || seq <= 0 {
		return nil, invalidityError(CategoryNonMonotonicSequence, rec.TraceID)
	}
	rec.Sequence = seq

	// Timestamp: valid decimal instant.
	ts, millis, ok := parseTimestamp(raw.Timestamp)
	if !ok {
		return nil, invalidityError(CategoryUnsupportedValue, rec.TraceID)
	}
	rec.Timestamp = ts
	rec.TimestampMillis = millis

	// Record type: known enum.
	rt, ok := knownRecordType(raw.RecordType)
	if !ok {
		return nil, invalidityError(CategoryUnsupportedValue, rec.TraceID)
	}
	rec.Type = rt
	rec.IsChunk = rt == RecordPayloadChunkAppended

	// Nullable identifier fields: null or blank becomes absent (empty).
	frameID, framePresent, valid := decodeNullableStringStrict(raw.FrameID)
	if !valid {
		return nil, invalidityError(CategoryUnsupportedValue, rec.TraceID)
	}
	rec.FrameID = frameID
	_ = framePresent
	parentFrame, parentPresent, valid := decodeNullableStringStrict(raw.ParentFrameID)
	if !valid {
		return nil, invalidityError(CategoryUnsupportedValue, rec.TraceID)
	}
	rec.ParentFrameID = parentFrame
	rec.HasParentFrame = parentPresent
	route, _, valid := decodeNullableStringStrict(raw.Route)
	if !valid {
		return nil, invalidityError(CategoryUnsupportedValue, rec.TraceID)
	}
	rec.Route = route

	// Frame type: known enum when present.
	if len(raw.FrameType) > 0 && !bytes.Equal(raw.FrameType, nullBytes) {
		var ftStr string
		if err := json.Unmarshal(raw.FrameType, &ftStr); err != nil {
			return nil, invalidityError(CategoryUnsupportedValue, rec.TraceID)
		}
		ft, ok := knownFrameType(ftStr)
		if !ok {
			return nil, invalidityError(CategoryUnsupportedValue, rec.TraceID)
		}
		rec.FrameType = ft
		rec.HasFrameType = true
	}

	// Metadata: default to {} when absent; retained verbatim.
	if len(raw.Metadata) == 0 || bytes.Equal(raw.Metadata, nullBytes) {
		rec.Metadata = []byte("{}")
	} else {
		rec.Metadata = append([]byte(nil), raw.Metadata...)
	}

	// Data: ordinary null is normally the producer's absent-value encoding.
	// Current producers mark an intentional semantic JSON null explicitly.
	explicitNull, present, boolErr := rec.metadataBool("semanticContentPresent")
	if boolErr != nil {
		return nil, invalidityError(CategoryUnsupportedValue, rec.TraceID)
	}
	if len(raw.Data) > 0 && (!bytes.Equal(raw.Data, nullBytes) || (present && explicitNull)) {
		rec.Data = append([]byte(nil), raw.Data...)
	}

	// Extract chunk/envelope payload identity from metadata for fast access.
	rec.PayloadID, rec.IsEnvelope = payloadIdentity(rec)
	if rec.PayloadID != "" && !contentReferenceIdentifierFits(rec.PayloadID) {
		return nil, invalidityError(CategoryUnsupportedValue, rec.TraceID)
	}

	return rec, nil
}

// parseErrorCategory maps a JSON decode error to the right invalidity category:
// an unexpected end of input is a truncated record; anything else is malformed.
func parseErrorCategory(err error) *consolecore.Error {
	msg := err.Error()
	if strings.Contains(msg, "unexpected end of JSON input") || strings.Contains(msg, "unexpected EOF") {
		return invalidityError(CategoryTruncatedInput, "")
	}
	return invalidityError(CategoryMalformedJSON, "")
}

// jsonDepth scans content outside string literals and returns the maximum
// object/array nesting depth. It is a bounded pre-scan used to enforce the
// depth-128 structural limit before encoding/json decodes the value.
func jsonDepth(content []byte) (int, bool) {
	depth, max := 0, 0
	inString := false
	escape := false
	for i := 0; i < len(content); i++ {
		c := content[i]
		if escape {
			escape = false
			continue
		}
		if inString {
			if c == '\\' {
				escape = true
			} else if c == '"' {
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{', '[':
			depth++
			if depth > max {
				max = depth
			}
		case '}', ']':
			depth--
		}
	}
	return max, !inString
}

// isObject reports whether content's first non-whitespace byte begins a JSON
// object.
func isObject(content []byte) bool {
	for i := 0; i < len(content); i++ {
		c := content[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			continue
		}
		return c == '{'
	}
	return false
}

func decodeNullableStringStrict(raw json.RawMessage) (string, bool, bool) {
	if len(raw) == 0 || bytes.Equal(raw, nullBytes) {
		return "", false, true
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false, false
	}
	normalized := normalizeNullable(s)
	if normalized == "" {
		return "", false, true
	}
	return normalized, true, true
}

// parseInteger parses a json.Number as a positive-allowing int64.
func parseInteger(num json.Number) (int64, bool) {
	if num == "" {
		return 0, false
	}
	v, err := num.Int64()
	if err != nil {
		return 0, false
	}
	return v, true
}

// parseTimestamp parses a numeric Instant (decimal seconds since epoch) into a
// time.Time and the millisecond truncation used by duration calculations. It
// matches the Java fixture corpus arithmetic (decimal movePointRight(3)).
func parseTimestamp(num json.Number) (time.Time, int64, bool) {
	s := string(num)
	if s == "" {
		return time.Time{}, 0, false
	}
	negative := false
	if strings.HasPrefix(s, "-") {
		negative = true
		s = s[1:]
	}
	intPart := s
	fracPart := ""
	if dot := strings.IndexByte(s, '.'); dot >= 0 {
		intPart = s[:dot]
		fracPart = s[dot+1:]
	}
	if intPart == "" || !allDigits(intPart) || (fracPart != "" && !allDigits(fracPart)) {
		return time.Time{}, 0, false
	}
	seconds, err := strconv.ParseInt(intPart, 10, 64)
	if err != nil {
		return time.Time{}, 0, false
	}
	if negative {
		return time.Time{}, 0, false
	}
	// java.time.Instant has nanosecond precision. Reject rather than truncate a
	// value that cannot be represented exactly by the current trace contract.
	if len(fracPart) > 9 {
		return time.Time{}, 0, false
	}
	for len(fracPart) < 9 {
		fracPart += "0"
	}
	nanos, err := strconv.ParseInt(fracPart, 10, 64)
	if err != nil {
		return time.Time{}, 0, false
	}
	t := time.Unix(seconds, nanos).UTC()
	millis, ok := multiplyChecked(seconds, 1000)
	if !ok {
		return time.Time{}, 0, false
	}
	millis, ok = addChecked(millis, nanos/1_000_000)
	if !ok {
		return time.Time{}, 0, false
	}
	return t, millis, true
}

func multiplyChecked(a, b int64) (int64, bool) {
	if a == 0 || b == 0 {
		return 0, true
	}
	result := a * b
	if result/b != a {
		return 0, false
	}
	return result, true
}

// allDigits reports whether s consists only of ASCII digits.
func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// payloadIdentity extracts the payloadId and payloadChunked flag from a
// record's metadata, returning whether this is a chunked-payload envelope.
func payloadIdentity(rec *Record) (string, bool) {
	if len(rec.Metadata) == 0 {
		return "", false
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(rec.Metadata, &m); err != nil {
		return "", false
	}
	idRaw, ok := m["payloadId"]
	if !ok || bytes.Equal(idRaw, nullBytes) {
		return "", false
	}
	var id string
	if err := json.Unmarshal(idRaw, &id); err != nil {
		return "", false
	}
	chunked := false
	if flagRaw, ok := m["payloadChunked"]; ok {
		var b bool
		if err := json.Unmarshal(flagRaw, &b); err == nil {
			chunked = b
		}
	}
	return id, chunked
}

// canceledError maps a context cancellation to a domain error. Cancellation is
// surfaced as TARGET_UNAVAILABLE so the service removes the staged bundle
// without publishing a handle.
func canceledError(cause error) *consolecore.Error {
	return consolecore.NewError(consolecore.CodeTargetUnavailable,
		"The operation was canceled.", "", consolecore.Details{}, cause)
}

// readErrorToDomain maps a non-EOF read error to a domain error. A canceled
// context is cancellation; otherwise the raw stream is unreadable.
func readErrorToDomain(cause error) *consolecore.Error {
	return consolecore.NewError(consolecore.CodeLocalStorageUnavailable,
		"The trace artifact could not be read.", "", consolecore.Details{}, cause)
}

// finishRead maps a non-nil read error to a domain error. EOF is success.
func finishRead(readErr error) *consolecore.Error {
	if readErr == nil || readErr == io.EOF {
		return nil
	}
	return readErrorToDomain(readErr)
}
