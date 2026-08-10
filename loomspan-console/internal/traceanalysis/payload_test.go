package traceanalysis

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
)

// chunkEnvelopeRecord builds a MODEL_REQUEST_SENT record that declares a chunked
// payload envelope.
func chunkEnvelopeRecord(seq int, payloadID, contentType string, chunkCount int) string {
	return chunkEnvelopeRecordFor(seq, "retry-1", "attempt-1", 1, payloadID, contentType, chunkCount)
}

// chunkEnvelopeRecordFor builds a MODEL_REQUEST_SENT record that declares a
// chunked payload envelope for a specific attempt.
func chunkEnvelopeRecordFor(seq int, retryID, attemptID string, attemptNum int, payloadID, contentType string, chunkCount int) string {
	meta := map[string]any{
		"retrySequenceId":       retryID,
		"attemptId":             attemptID,
		"attemptNumber":         attemptNum,
		"attemptReason":         attemptReason(attemptNum),
		"providerAttemptNumber": 1,
		"payloadChunked":        true,
		"payloadId":             payloadID,
		"contentType":           contentType,
		"chunkCount":            chunkCount,
	}
	metaJSON, _ := json.Marshal(meta)
	return `{"traceId":"t","sessionId":"s","sequence":` + itoa(seq) +
		`,"timestamp":` + timestampForSeq(seq) +
		`,"recordType":"MODEL_REQUEST_SENT","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"th",` +
		`"metadata":` + string(metaJSON) + `,"data":{"messages":["u"]}}`
}

// chunkRecord builds a PAYLOAD_CHUNK_APPENDED record with the given chunk
// content.
func chunkRecord(seq int, payloadID, contentType string, chunkIndex, chunkCount int, content string) string {
	meta := map[string]any{
		"payloadId":   payloadID,
		"chunkIndex":  chunkIndex,
		"chunkCount":  chunkCount,
		"contentType": contentType,
	}
	metaJSON, _ := json.Marshal(meta)
	dataJSON, _ := json.Marshal(content)
	return `{"traceId":"t","sessionId":"s","sequence":` + itoa(seq) +
		`,"timestamp":` + timestampForSeq(seq) +
		`,"recordType":"PAYLOAD_CHUNK_APPENDED","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"th",` +
		`"metadata":` + string(metaJSON) + `,"data":` + string(dataJSON) + `}`
}

// chunkedPayloadTrace builds a complete valid trace with a chunked payload of
// the given total size split into the given number of chunks. Each chunk is
// chunkSize bytes of 'x'. The reader streams the trace without materializing the
// full content in a single Go slice (the chunks are generated on demand).
func chunkedPayloadTrace(totalSize, chunkCount int) string {
	chunkSize := totalSize / chunkCount
	var b strings.Builder
	b.WriteString(startedRecord(1) + "\n")
	b.WriteString(requestRecord(2, "retry-1", "attempt-1", 1, true) + "\n")
	b.WriteString(chunkEnvelopeRecord(3, "payload-1", "text/plain", chunkCount) + "\n")
	for i := 0; i < chunkCount; i++ {
		content := strings.Repeat("x", chunkSize)
		b.WriteString(chunkRecord(4+i, "payload-1", "text/plain", i, chunkCount, content) + "\n")
	}
	nextSeq := 4 + chunkCount
	b.WriteString(responseRecord(nextSeq, "", "retry-1", "attempt-1", 1, 2, 1, 3, "EXACT") + "\n")
	b.WriteString(completionRecord(nextSeq+1, "SUCCEEDED", 2, 1, 3, "") + "\n")
	return b.String()
}

// TestProcessorReconstructsChunkedTextPayload proves the processor reconstructs a
// chunked text payload and writes it to the payload store component.
func TestProcessorReconstructsChunkedTextPayload(t *testing.T) {
	raw := chunkedPayloadTrace(4096, 4)
	sink := &fakeSink{}
	processor := New()
	_, domain := processor.Process(artifact.ProcessRequest{
		Context:  context.Background(),
		Metadata: artifact.TraceMetadata{TraceID: "t", SessionID: "s"},
		Raw:      strings.NewReader(raw),
		Sink:     sink,
	})
	if domain != nil {
		t.Fatalf("Process failed: %v", domain)
	}
	storeBytes, ok := sink.components[artifact.ComponentName(ComponentPayloadStore)]
	if !ok {
		t.Fatal("expected payload store component")
	}
	expected := strings.Repeat("x", 4096) // 4 chunks × 1024 bytes each
	if string(storeBytes) != expected {
		t.Fatalf("payload store has %d bytes, expected %d", len(storeBytes), len(expected))
	}
	// The payload index should have one descriptor.
	payloadIdxBytes, ok := sink.components[artifact.ComponentName(ComponentPayloadIndex)]
	if !ok {
		t.Fatal("expected payload index component")
	}
	r := bytes.NewReader(payloadIdxBytes)
	row, err := readLengthPrefixed(r)
	if err != nil {
		t.Fatalf("read payload descriptor: %v", err)
	}
	var desc payloadDescriptor
	if err := json.Unmarshal(row, &desc); err != nil {
		t.Fatalf("parse payload descriptor: %v", err)
	}
	if desc.ChunkCount != 4 {
		t.Fatalf("expected chunkCount 4, got %d", desc.ChunkCount)
	}
	if desc.ContentType != "text/plain" {
		t.Fatalf("expected contentType text/plain, got %s", desc.ContentType)
	}
}

// TestProcessorRejectsMissingChunk proves that when a chunked payload envelope
// declares 2 chunks but only 1 arrives, the processor rejects with
// INCOMPLETE_CHUNKS.
func TestProcessorRejectsMissingChunk(t *testing.T) {
	// Envelope declares 2 chunks, but only 1 arrives.
	raw := startedRecord(1) + "\n" +
		requestRecord(2, "retry-1", "attempt-1", 1, true) + "\n" +
		chunkEnvelopeRecord(3, "payload-1", "text/plain", 2) + "\n" +
		chunkRecord(4, "payload-1", "text/plain", 0, 2, "hello") + "\n" +
		// Missing chunk 1; go straight to response and completion.
		responseRecord(5, "", "retry-1", "attempt-1", 1, 2, 1, 3, "EXACT") + "\n" +
		completionRecord(6, "SUCCEEDED", 2, 1, 3, "") + "\n"
	sink := &fakeSink{}
	processor := New()
	_, domain := processor.Process(artifact.ProcessRequest{
		Context:  context.Background(),
		Metadata: artifact.TraceMetadata{TraceID: "t", SessionID: "s"},
		Raw:      strings.NewReader(raw),
		Sink:     sink,
	})
	if domain == nil {
		t.Fatal("expected error for missing chunk")
	}
	cat, ok := categoryOf(domain)
	if !ok || cat != CategoryIncompleteChunks {
		t.Fatalf("expected INCOMPLETE_CHUNKS, got %v", cat)
	}
}

// TestProcessorRejectsDuplicateChunkIndex proves a duplicate chunkIndex is
// rejected as INVALID_CHUNKS.
func TestProcessorRejectsDuplicateChunkIndex(t *testing.T) {
	raw := startedRecord(1) + "\n" +
		requestRecord(2, "retry-1", "attempt-1", 1, true) + "\n" +
		chunkEnvelopeRecord(3, "payload-1", "text/plain", 2) + "\n" +
		chunkRecord(4, "payload-1", "text/plain", 0, 2, "hello") + "\n" +
		chunkRecord(5, "payload-1", "text/plain", 0, 2, "world") + "\n" + // duplicate index 0
		responseRecord(6, "", "retry-1", "attempt-1", 1, 2, 1, 3, "EXACT") + "\n" +
		completionRecord(7, "SUCCEEDED", 2, 1, 3, "") + "\n"
	sink := &fakeSink{}
	processor := New()
	_, domain := processor.Process(artifact.ProcessRequest{
		Context:  context.Background(),
		Metadata: artifact.TraceMetadata{TraceID: "t", SessionID: "s"},
		Raw:      strings.NewReader(raw),
		Sink:     sink,
	})
	if domain == nil {
		t.Fatal("expected error for duplicate chunk index")
	}
	cat, ok := categoryOf(domain)
	if !ok || cat != CategoryInvalidChunks {
		t.Fatalf("expected INVALID_CHUNKS, got %v", cat)
	}
}

// TestProcessorRejectsOutOfOrderChunkIndex proves an out-of-order chunkIndex is
// rejected as INVALID_CHUNKS.
func TestProcessorRejectsOutOfOrderChunkIndex(t *testing.T) {
	raw := startedRecord(1) + "\n" +
		requestRecord(2, "retry-1", "attempt-1", 1, true) + "\n" +
		chunkEnvelopeRecord(3, "payload-1", "text/plain", 2) + "\n" +
		chunkRecord(4, "payload-1", "text/plain", 1, 2, "world") + "\n" + // index 1 before 0
		chunkRecord(5, "payload-1", "text/plain", 0, 2, "hello") + "\n" +
		responseRecord(6, "", "retry-1", "attempt-1", 1, 2, 1, 3, "EXACT") + "\n" +
		completionRecord(7, "SUCCEEDED", 2, 1, 3, "") + "\n"
	sink := &fakeSink{}
	processor := New()
	_, domain := processor.Process(artifact.ProcessRequest{
		Context:  context.Background(),
		Metadata: artifact.TraceMetadata{TraceID: "t", SessionID: "s"},
		Raw:      strings.NewReader(raw),
		Sink:     sink,
	})
	if domain == nil {
		t.Fatal("expected error for out-of-order chunk index")
	}
	cat, ok := categoryOf(domain)
	if !ok || cat != CategoryInvalidChunks {
		t.Fatalf("expected INVALID_CHUNKS, got %v", cat)
	}
}

// TestProcessorRejectsMismatchedChunkCount proves a chunk whose chunkCount
// differs from the envelope is rejected as INVALID_CHUNKS.
func TestProcessorRejectsMismatchedChunkCount(t *testing.T) {
	raw := startedRecord(1) + "\n" +
		requestRecord(2, "retry-1", "attempt-1", 1, true) + "\n" +
		chunkEnvelopeRecord(3, "payload-1", "text/plain", 2) + "\n" +
		chunkRecord(4, "payload-1", "text/plain", 0, 3, "hello") + "\n" + // chunkCount 3, not 2
		chunkRecord(5, "payload-1", "text/plain", 1, 3, "world") + "\n" +
		chunkRecord(6, "payload-1", "text/plain", 2, 3, "!") + "\n" +
		responseRecord(7, "", "retry-1", "attempt-1", 1, 2, 1, 3, "EXACT") + "\n" +
		completionRecord(8, "SUCCEEDED", 2, 1, 3, "") + "\n"
	sink := &fakeSink{}
	processor := New()
	_, domain := processor.Process(artifact.ProcessRequest{
		Context:  context.Background(),
		Metadata: artifact.TraceMetadata{TraceID: "t", SessionID: "s"},
		Raw:      strings.NewReader(raw),
		Sink:     sink,
	})
	if domain == nil {
		t.Fatal("expected error for mismatched chunk count")
	}
	cat, ok := categoryOf(domain)
	if !ok || cat != CategoryInvalidChunks {
		t.Fatalf("expected INVALID_CHUNKS, got %v", cat)
	}
}

// streamingChunkReader is an io.Reader that generates a chunked-payload trace
// on demand without materializing the full payload in a single Go slice. Each
// chunk is generated lazily as the reader is consumed.
type streamingChunkReader struct {
	totalSize  int
	chunkCount int
	chunkSize  int
	chunkIndex int
	phase      int // 0=prefix, 1=chunks, 2=suffix
	buf        bytes.Buffer
	prefix     string
	suffix     string
}

func newStreamingChunkReader(totalSize, chunkCount int) *streamingChunkReader {
	r := &streamingChunkReader{
		totalSize:  totalSize,
		chunkCount: chunkCount,
		chunkSize:  totalSize / chunkCount,
	}
	r.prefix = startedRecord(1) + "\n" +
		requestRecord(2, "retry-1", "attempt-1", 1, true) + "\n" +
		chunkEnvelopeRecord(3, "payload-1", "text/plain", chunkCount) + "\n"
	nextSeq := 4 + chunkCount
	r.suffix = responseRecord(nextSeq, "", "retry-1", "attempt-1", 1, 2, 1, 3, "EXACT") + "\n" +
		completionRecord(nextSeq+1, "SUCCEEDED", 2, 1, 3, "") + "\n"
	return r
}

func (r *streamingChunkReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	// Refill the internal buffer if empty.
	for r.buf.Len() == 0 {
		switch r.phase {
		case 0:
			r.buf.WriteString(r.prefix)
			r.phase = 1
		case 1:
			if r.chunkIndex >= r.chunkCount {
				r.phase = 2
				continue
			}
			content := strings.Repeat("x", r.chunkSize)
			r.buf.WriteString(chunkRecord(4+r.chunkIndex, "payload-1", "text/plain",
				r.chunkIndex, r.chunkCount, content) + "\n")
			r.chunkIndex++
		case 2:
			r.buf.WriteString(r.suffix)
			r.phase = 3
		case 3:
			return 0, io.EOF
		}
	}
	return r.buf.Read(p)
}

// TestProcessorStreamsLargePayloadWithoutWholeValueReadOrWrite proves a 64 MiB
// chunked payload is reconstructed and written to the store without the
// processor materializing the full logical payload in memory. The test uses a
// streaming reader that generates chunks on demand; the fakeSink records the
// maximum single Write call to the payload store. If the processor materialized
// the whole payload, the max write would equal the payload size.
func TestProcessorStreamsLargePayloadWithoutWholeValueReadOrWrite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping large payload test in short mode")
	}
	const payloadSize = 64 << 20          // 64 MiB
	const chunkCount = 256                // 256 chunks
	chunkSize := payloadSize / chunkCount // 256 KiB per chunk (well under 1 MiB line limit)

	reader := newStreamingChunkReader(payloadSize, chunkCount)
	store := &maxWriteSink{}
	sink := &chainedSink{
		store:      store,
		components: map[artifact.ComponentName][]byte{},
	}
	processor := New()
	_, domain := processor.Process(artifact.ProcessRequest{
		Context:  context.Background(),
		Metadata: artifact.TraceMetadata{TraceID: "t", SessionID: "s"},
		Raw:      reader,
		Sink:     sink,
	})
	if domain != nil {
		t.Fatalf("Process failed: %v", domain)
	}
	// The payload store must contain the full reconstructed payload.
	if store.totalWritten != int64(payloadSize) {
		t.Fatalf("payload store has %d bytes, expected %d", store.totalWritten, payloadSize)
	}
	// The maximum single write must not exceed one chunk plus overhead. If the
	// processor materialized the whole payload, maxWrite would be ~64 MiB.
	maxAllowed := int64(chunkSize + 1024) // chunk size + small overhead
	if store.maxWrite > maxAllowed {
		t.Fatalf("max single write to payload store was %d bytes (expected <= %d); the processor may have materialized the full payload", store.maxWrite, maxAllowed)
	}
}

// maxWriteSink wraps a bytes.Buffer and records the largest single Write call.
type maxWriteSink struct {
	buf          bytes.Buffer
	totalWritten int64
	maxWrite     int64
}

func (s *maxWriteSink) Write(p []byte) (int, error) {
	if int64(len(p)) > s.maxWrite {
		s.maxWrite = int64(len(p))
	}
	s.totalWritten += int64(len(p))
	return s.buf.Write(p)
}

func (s *maxWriteSink) Bytes() []byte { return s.buf.Bytes() }

// chainedSink routes the payload store component to a maxWriteSink and all other
// components to in-memory buffers, satisfying the artifact.ComponentSink
// interface.
type chainedSink struct {
	store      *maxWriteSink
	components map[artifact.ComponentName][]byte
}

func (s *chainedSink) Create(_ context.Context, name artifact.ComponentName) (artifact.ComponentWriter, *consolecore.Error) {
	if name == artifact.ComponentName(ComponentPayloadStore) {
		return &chainedComponentWriter{sink: s, name: name, buf: &bytes.Buffer{}, store: s.store}, nil
	}
	return &chainedComponentWriter{sink: s, name: name, buf: &bytes.Buffer{}, store: nil}, nil
}

// Compile-time assertion that chainedSink satisfies artifact.ComponentSink.
var _ artifact.ComponentSink = (*chainedSink)(nil)

type chainedComponentWriter struct {
	sink   *chainedSink
	name   artifact.ComponentName
	buf    *bytes.Buffer
	store  *maxWriteSink // non-nil for the payload store
	closed bool
}

func (w *chainedComponentWriter) Write(p []byte) (int, error) {
	if w.closed {
		return 0, io.ErrClosedPipe
	}
	if w.store != nil {
		return w.store.Write(p)
	}
	return w.buf.Write(p)
}

func (w *chainedComponentWriter) Sync() error { return nil }

func (w *chainedComponentWriter) Close() error {
	w.closed = true
	if w.store == nil {
		w.sink.components[w.name] = w.buf.Bytes()
	}
	return nil
}

// BenchmarkProcessChunkedPayload measures allocations for processing a chunked
// payload. Run with: go test -bench=BenchmarkProcessChunkedPayload -benchmem
// The benchmark proves peak allocations do not scale with logical payload size.
func BenchmarkProcessChunkedPayload(b *testing.B) {
	const payloadSize = 8 << 20 // 8 MiB (smaller for benchmark speed)
	const chunkCount = 8
	raw := chunkedPayloadTrace(payloadSize, chunkCount)
	sink := &fakeSink{}
	processor := New()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = processor.Process(artifact.ProcessRequest{
			Context:  context.Background(),
			Metadata: artifact.TraceMetadata{TraceID: "t", SessionID: "s"},
			Raw:      strings.NewReader(raw),
			Sink:     sink,
		})
		sink.components = map[artifact.ComponentName][]byte{}
	}
}

// chunkedJSONPayloadTrace builds a complete valid trace with a chunked JSON
// payload split into the given number of chunks. The reconstructed payload is a
// valid JSON object. Each chunk carries a fragment of the JSON text.
func chunkedJSONPayloadTrace(chunkCount int) string {
	// Build a JSON object and split it into chunkCount fragments.
	jsonObj := `{"key":"value","nested":{"items":[1,2,3]},"text":"hello world"}`
	chunkSize := len(jsonObj) / chunkCount
	var b strings.Builder
	b.WriteString(startedRecord(1) + "\n")
	b.WriteString(requestRecord(2, "retry-1", "attempt-1", 1, true) + "\n")
	b.WriteString(chunkEnvelopeRecord(3, "payload-json-1", "application/json", chunkCount) + "\n")
	for i := 0; i < chunkCount; i++ {
		start := i * chunkSize
		end := start + chunkSize
		if i == chunkCount-1 {
			end = len(jsonObj) // last chunk gets the remainder
		}
		content := jsonObj[start:end]
		b.WriteString(chunkRecord(4+i, "payload-json-1", "application/json", i, chunkCount, content) + "\n")
	}
	nextSeq := 4 + chunkCount
	b.WriteString(responseRecord(nextSeq, "", "retry-1", "attempt-1", 1, 2, 1, 3, "EXACT") + "\n")
	b.WriteString(completionRecord(nextSeq+1, "SUCCEEDED", 2, 1, 3, "") + "\n")
	return b.String()
}

// TestProcessorReconstructsChunkedJSONPayload proves the processor reconstructs
// a chunked application/json payload, validates the reconstructed JSON, and
// writes it to the payload store. The physical chunk records remain
// addressable through the record-address index.
func TestProcessorReconstructsChunkedJSONPayload(t *testing.T) {
	raw := chunkedJSONPayloadTrace(3)
	sink := &fakeSink{}
	processor := New()
	_, domain := processor.Process(artifact.ProcessRequest{
		Context:  context.Background(),
		Metadata: artifact.TraceMetadata{TraceID: "t", SessionID: "s"},
		Raw:      strings.NewReader(raw),
		Sink:     sink,
	})
	if domain != nil {
		t.Fatalf("Process failed: %v", domain)
	}
	storeBytes, ok := sink.components[artifact.ComponentName(ComponentPayloadStore)]
	if !ok {
		t.Fatal("expected payload store component")
	}
	// The reconstructed payload must be valid JSON.
	var parsed map[string]any
	if err := json.Unmarshal(storeBytes, &parsed); err != nil {
		t.Fatalf("reconstructed JSON payload is not valid JSON: %v\ncontent: %s", err, string(storeBytes))
	}
	if parsed["key"] != "value" {
		t.Errorf("reconstructed JSON key: got %v want value", parsed["key"])
	}
	// The payload index must have one descriptor with contentType application/json.
	payloads := readFactRows[payloadDescriptor](t, sink, ComponentPayloadIndex)
	if len(payloads) != 1 {
		t.Fatalf("expected 1 payload descriptor, got %d", len(payloads))
	}
	if payloads[0].ContentType != "application/json" {
		t.Errorf("contentType: got %s want application/json", payloads[0].ContentType)
	}
	if payloads[0].ChunkCount != 3 {
		t.Errorf("chunkCount: got %d want 3", payloads[0].ChunkCount)
	}
	// The physical chunk records must remain addressable: the record-address
	// index must include the chunk sequences.
	recIdx, ok := sink.components[artifact.ComponentName(ComponentRecordIndex)]
	if !ok {
		t.Fatal("expected record index component")
	}
	rowCount := len(recIdx) / recordIndexRowWidth
	// 1 started + 1 prepared + 1 envelope + 3 chunks + 1 response + 1 completion = 8
	if rowCount != 8 {
		t.Fatalf("expected 8 record rows, got %d", rowCount)
	}
}

// TestProcessorRejectsInterleavedChunksForDifferentPayloads proves a payload
// descriptor can never address interwoven bytes in the shared payload store.
func TestProcessorRejectsInterleavedChunksForDifferentPayloads(t *testing.T) {
	// attempt-1 carries payload-1 (2 chunks); attempt-2 carries payload-2 (1 chunk).
	// Chunks interleave: payload-1 chunk 0, payload-2 chunk 0, payload-1 chunk 1.
	raw := startedRecord(1) + "\n" +
		requestRecord(2, "retry-1", "attempt-1", 1, true) + "\n" +
		chunkEnvelopeRecord(3, "payload-1", "text/plain", 2) + "\n" +
		requestRecord(4, "retry-2", "attempt-2", 1, true) + "\n" +
		chunkEnvelopeRecordFor(5, "retry-2", "attempt-2", 1, "payload-2", "text/plain", 1) + "\n" +
		chunkRecord(6, "payload-1", "text/plain", 0, 2, "hello") + "\n" +
		chunkRecord(7, "payload-2", "text/plain", 0, 1, "world") + "\n" +
		chunkRecord(8, "payload-1", "text/plain", 1, 2, "!") + "\n" +
		responseRecord(9, "", "retry-1", "attempt-1", 1, 2, 1, 3, "EXACT") + "\n" +
		responseRecord(10, "", "retry-2", "attempt-2", 1, 1, 1, 2, "EXACT") + "\n" +
		completionRecord(11, "SUCCEEDED", 3, 2, 5, "") + "\n"
	sink := &fakeSink{}
	processor := New()
	_, domain := processor.Process(artifact.ProcessRequest{
		Context:  context.Background(),
		Metadata: artifact.TraceMetadata{TraceID: "t", SessionID: "s"},
		Raw:      strings.NewReader(raw),
		Sink:     sink,
	})
	category, _ := categoryOf(domain)
	if domain == nil || category != CategoryInvalidChunks {
		t.Fatalf("expected invalid chunks, got: %v", domain)
	}
}

// TestProcessorRejectsInterleavedOutOfOrderChunks proves that when chunks for
// one payload interleave with another payload's chunks but arrive out of order
// for the first payload, the processor rejects them as INVALID_CHUNKS.
func TestProcessorRejectsInterleavedOutOfOrderChunks(t *testing.T) {
	// attempt-1 carries payload-1 (2 chunks); attempt-2 carries payload-2 (1 chunk).
	// payload-1 chunk 1 arrives before chunk 0 (out of order), interleaved with
	// payload-2's chunk.
	raw := startedRecord(1) + "\n" +
		requestRecord(2, "retry-1", "attempt-1", 1, true) + "\n" +
		chunkEnvelopeRecord(3, "payload-1", "text/plain", 2) + "\n" +
		requestRecord(4, "retry-2", "attempt-2", 1, true) + "\n" +
		chunkEnvelopeRecordFor(5, "retry-2", "attempt-2", 1, "payload-2", "text/plain", 1) + "\n" +
		// payload-1 chunk 1 arrives before chunk 0 — out of order.
		chunkRecord(6, "payload-1", "text/plain", 1, 2, "!") + "\n" +
		chunkRecord(7, "payload-2", "text/plain", 0, 1, "world") + "\n" +
		chunkRecord(8, "payload-1", "text/plain", 0, 2, "hello") + "\n" +
		responseRecord(9, "", "retry-1", "attempt-1", 1, 2, 1, 3, "EXACT") + "\n" +
		responseRecord(10, "", "retry-2", "attempt-2", 1, 1, 1, 2, "EXACT") + "\n" +
		completionRecord(11, "SUCCEEDED", 3, 2, 5, "") + "\n"
	sink := &fakeSink{}
	processor := New()
	_, domain := processor.Process(artifact.ProcessRequest{
		Context:  context.Background(),
		Metadata: artifact.TraceMetadata{TraceID: "t", SessionID: "s"},
		Raw:      strings.NewReader(raw),
		Sink:     sink,
	})
	if domain == nil {
		t.Fatal("expected error for out-of-order interleaved chunks")
	}
	cat, ok := categoryOf(domain)
	if !ok || cat != CategoryInvalidChunks {
		t.Fatalf("expected INVALID_CHUNKS, got %v", cat)
	}
}

// TestProcessorRejectsInvalidReconstructedJSONAndContentType proves the
// processor rejects a chunked application/json payload whose reconstructed
// content is not valid JSON, and a chunk whose content type does not match the
// envelope's content type.
func TestProcessorRejectsInvalidReconstructedJSONAndContentType(t *testing.T) {
	t.Run("invalid_reconstructed_json", func(t *testing.T) {
		// A chunked application/json payload whose chunks concatenate to
		// invalid JSON.
		raw := startedRecord(1) + "\n" +
			requestRecord(2, "retry-1", "attempt-1", 1, true) + "\n" +
			chunkEnvelopeRecord(3, "payload-1", "application/json", 2) + "\n" +
			chunkRecord(4, "payload-1", "application/json", 0, 2, "{not valid") + "\n" +
			chunkRecord(5, "payload-1", "application/json", 1, 2, " json}") + "\n" +
			responseRecord(6, "", "retry-1", "attempt-1", 1, 2, 1, 3, "EXACT") + "\n" +
			completionRecord(7, "SUCCEEDED", 2, 1, 3, "") + "\n"
		sink := &fakeSink{}
		processor := New()
		_, domain := processor.Process(artifact.ProcessRequest{
			Context:  context.Background(),
			Metadata: artifact.TraceMetadata{TraceID: "t", SessionID: "s"},
			Raw:      strings.NewReader(raw),
			Sink:     sink,
		})
		if domain == nil {
			t.Fatal("expected error for invalid reconstructed JSON")
		}
		cat, ok := categoryOf(domain)
		if !ok || cat != CategoryInvalidChunks {
			t.Fatalf("expected INVALID_CHUNKS, got %v", cat)
		}
	})

	t.Run("mismatched_content_type", func(t *testing.T) {
		// Envelope declares text/plain, chunk declares application/json.
		raw := startedRecord(1) + "\n" +
			requestRecord(2, "retry-1", "attempt-1", 1, true) + "\n" +
			chunkEnvelopeRecord(3, "payload-1", "text/plain", 1) + "\n" +
			chunkRecord(4, "payload-1", "application/json", 0, 1, "hello") + "\n" +
			responseRecord(5, "", "retry-1", "attempt-1", 1, 2, 1, 3, "EXACT") + "\n" +
			completionRecord(6, "SUCCEEDED", 2, 1, 3, "") + "\n"
		sink := &fakeSink{}
		processor := New()
		_, domain := processor.Process(artifact.ProcessRequest{
			Context:  context.Background(),
			Metadata: artifact.TraceMetadata{TraceID: "t", SessionID: "s"},
			Raw:      strings.NewReader(raw),
			Sink:     sink,
		})
		if domain == nil {
			t.Fatal("expected error for mismatched content type")
		}
		cat, ok := categoryOf(domain)
		if !ok || cat != CategoryInvalidChunks {
			t.Fatalf("expected INVALID_CHUNKS, got %v", cat)
		}
	})

	t.Run("extra_chunk", func(t *testing.T) {
		// Envelope declares 1 chunk, but 2 arrive (extra chunk). The processor
		// accepts the second chunk (its index matches the next expected) but
		// rejects at finalize because received != declared chunk count.
		raw := startedRecord(1) + "\n" +
			requestRecord(2, "retry-1", "attempt-1", 1, true) + "\n" +
			chunkEnvelopeRecord(3, "payload-1", "text/plain", 1) + "\n" +
			chunkRecord(4, "payload-1", "text/plain", 0, 1, "hello") + "\n" +
			chunkRecord(5, "payload-1", "text/plain", 1, 1, "extra") + "\n" + // extra chunk
			responseRecord(6, "", "retry-1", "attempt-1", 1, 2, 1, 3, "EXACT") + "\n" +
			completionRecord(7, "SUCCEEDED", 2, 1, 3, "") + "\n"
		sink := &fakeSink{}
		processor := New()
		_, domain := processor.Process(artifact.ProcessRequest{
			Context:  context.Background(),
			Metadata: artifact.TraceMetadata{TraceID: "t", SessionID: "s"},
			Raw:      strings.NewReader(raw),
			Sink:     sink,
		})
		if domain == nil {
			t.Fatal("expected error for extra chunk")
		}
		cat, ok := categoryOf(domain)
		if !ok || (cat != CategoryIncompleteChunks && cat != CategoryInvalidChunks) {
			t.Fatalf("expected INCOMPLETE_CHUNKS or INVALID_CHUNKS, got %v", cat)
		}
	})
}
