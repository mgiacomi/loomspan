package traceanalysis

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
)

// fakeSink is a test ComponentSink that records created components in memory.
type fakeSink struct {
	components map[artifact.ComponentName][]byte
	createErr  *consolecore.Error
}

func (s *fakeSink) Create(_ context.Context, name artifact.ComponentName) (artifact.ComponentWriter, *consolecore.Error) {
	if s.createErr != nil {
		return nil, s.createErr
	}
	if s.components == nil {
		s.components = map[artifact.ComponentName][]byte{}
	}
	return &fakeComponentWriter{sink: s, name: name}, nil
}

type fakeComponentWriter struct {
	sink   *fakeSink
	name   artifact.ComponentName
	buf    bytes.Buffer
	closed bool
}

func (w *fakeComponentWriter) Write(p []byte) (int, error) {
	if w.closed {
		return 0, errors.New("writer is closed")
	}
	return w.buf.Write(p)
}

func (w *fakeComponentWriter) Sync() error { return nil }

func (w *fakeComponentWriter) Close() error {
	w.closed = true
	w.sink.components[w.name] = w.buf.Bytes()
	return nil
}

// minimalValidTrace is a single-attempt success trace matching the Java fixture
// corpus format. It is the smallest artifact the Phase 3 processor accepts.
const minimalValidTrace = `{"traceId":"trace-t","sessionId":"session-t","sequence":1,"timestamp":1784894400.000000000,"recordType":"TRACE_STARTED","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"fixture-thread","metadata":{"tracePath":"traces/t.ndjson","consoleCompatibilityVersion":"development"},"data":{"sessionId":"session-t"}}
{"traceId":"trace-t","sessionId":"session-t","sequence":2,"timestamp":1784894400.000000000,"recordType":"MODEL_THOUGHT_CAPTURED","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"fixture-thread","metadata":{"retrySequenceId":"retry-1","attemptId":"attempt-1","attemptNumber":1,"attemptReason":"INITIAL","providerAttemptNumber":1},"data":{"messages":["user"]}}
{"traceId":"trace-t","sessionId":"session-t","sequence":3,"timestamp":1784894400.000000000,"recordType":"MODEL_REQUEST_SENT","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"fixture-thread","metadata":{"retrySequenceId":"retry-1","attemptId":"attempt-1","attemptNumber":1,"attemptReason":"INITIAL","providerAttemptNumber":1},"data":{"messages":["user"]}}
{"traceId":"trace-t","sessionId":"session-t","sequence":4,"timestamp":1784894400.000000000,"recordType":"MODEL_RESPONSE_RECEIVED","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"fixture-thread","metadata":{"retrySequenceId":"retry-1","attemptId":"attempt-1","attemptNumber":1,"attemptReason":"INITIAL","providerAttemptNumber":1,"usage":{"promptUnits":10,"completionUnits":4,"totalUnits":14,"precision":"EXACT"}},"data":{"content":"fixture response"}}
{"traceId":"trace-t","sessionId":"session-t","sequence":5,"timestamp":1784894400.000000000,"recordType":"TRACE_COMPLETED","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"fixture-thread","metadata":{"outcome":"SUCCEEDED","sessionUsageSnapshot":{"promptUnits":10,"completionUnits":4,"totalUnits":14},"errored":false,"persistencePolicy":"ALWAYS"},"data":null}
`

// TestProcessorValidTraceWritesBundle proves the processor accepts a valid
// trace, writes the manifest and every derived index/store component, and
// reports positive derived component sizes.
func TestProcessorValidTraceWritesBundle(t *testing.T) {
	sink := &fakeSink{}
	processor := New()

	result, domain := processor.Process(artifact.ProcessRequest{
		Context:  context.Background(),
		Metadata: artifact.TraceMetadata{TraceID: "trace-t", SessionID: "session-t", Outcome: "SUCCEEDED"},
		Raw:      strings.NewReader(minimalValidTrace),
		Sink:     sink,
	})
	if domain != nil {
		t.Fatalf("Process failed: %v", domain)
	}
	if len(result.ComponentSizes) == 0 {
		t.Fatal("expected derived component sizes")
	}
	// Every derived component must be present. Empty indexes (for example
	// failures.idx in a success trace) are valid zero-size components.
	for name := range result.ComponentSizes {
		if _, ok := sink.components[name]; !ok {
			t.Fatalf("component %s reported but not written", name)
		}
	}
	// The manifest must be present and parseable.
	manifestBytes, ok := sink.components[ComponentManifest]
	if !ok {
		t.Fatal("expected manifest component")
	}
	var m manifest
	if err := json.Unmarshal(manifestBytes, &m); err != nil {
		t.Fatalf("manifest is not valid JSON: %v", err)
	}
	if m.Schema != manifestSchemaV1 {
		t.Fatalf("expected schema %q, got %q", manifestSchemaV1, m.Schema)
	}
	if m.RecordCount != 5 {
		t.Fatalf("expected record count 5, got %d", m.RecordCount)
	}
	if m.Outcome != "SUCCEEDED" {
		t.Fatalf("expected outcome SUCCEEDED, got %q", m.Outcome)
	}
	if m.AttemptCount != 1 {
		t.Fatalf("expected attempt count 1, got %d", m.AttemptCount)
	}
}

func TestProcessorRejectsTraceWithoutCompatibilityMarker(t *testing.T) {
	raw := strings.Replace(minimalValidTrace, `,"consoleCompatibilityVersion":"development"`, "", 1)
	_, domain := New().Process(artifact.ProcessRequest{
		Context: context.Background(), Metadata: artifact.TraceMetadata{TraceID: "trace-t"},
		Raw: strings.NewReader(raw), Sink: &fakeSink{},
	})
	if domain == nil || domain.Code != consolecore.CodeInvalidArtifact {
		t.Fatalf("expected INVALID_ARTIFACT, got %v", domain)
	}
}

func TestProcessorRejectsIncompatibleCompatibilityMarker(t *testing.T) {
	raw := strings.Replace(minimalValidTrace, `"consoleCompatibilityVersion":"development"`,
		`"consoleCompatibilityVersion":"0.1.0"`, 1)
	_, domain := New().Process(artifact.ProcessRequest{
		Context: context.Background(), Metadata: artifact.TraceMetadata{TraceID: "trace-t"},
		Raw: strings.NewReader(raw), Sink: &fakeSink{},
	})
	if domain == nil || domain.Code != consolecore.CodeIncompatibleArtifact {
		t.Fatalf("expected INCOMPATIBLE_ARTIFACT, got %v", domain)
	}
	if domain.Details.ExpectedCompatibilityVersion != "development" ||
		domain.Details.ObservedCompatibilityVersion != "0.1.0" {
		t.Fatalf("unexpected compatibility details: %+v", domain.Details)
	}
}

func TestProcessorCompatibilityMarkerValidationMatrix(t *testing.T) {
	tests := []struct {
		name             string
		processorVersion string
		marker           string
		wantCode         consolecore.Code
	}{
		{name: "blank", processorVersion: "development", marker: `""`, wantCode: consolecore.CodeInvalidArtifact},
		{name: "null", processorVersion: "development", marker: `null`, wantCode: consolecore.CodeInvalidArtifact},
		{name: "number", processorVersion: "development", marker: `25`, wantCode: consolecore.CodeInvalidArtifact},
		{name: "object", processorVersion: "development", marker: `{}`, wantCode: consolecore.CodeInvalidArtifact},
		{name: "released to development", processorVersion: "development", marker: `"1.2.3"`, wantCode: consolecore.CodeIncompatibleArtifact},
		{name: "development to released", processorVersion: "1.2.3", marker: `"development"`, wantCode: consolecore.CodeIncompatibleArtifact},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw := strings.Replace(minimalValidTrace, `"consoleCompatibilityVersion":"development"`,
				`"consoleCompatibilityVersion":`+test.marker, 1)
			_, domain := newProcessorForVersion(test.processorVersion).Process(artifact.ProcessRequest{
				Context: context.Background(), Metadata: artifact.TraceMetadata{TraceID: "trace-t"},
				Raw: strings.NewReader(raw), Sink: &fakeSink{},
			})
			if domain == nil || domain.Code != test.wantCode {
				t.Fatalf("expected %s, got %v", test.wantCode, domain)
			}
		})
	}
}

func TestProcessorAcceptsExactReleaseAndMatchingDevelopmentOnlyThroughFullValidation(t *testing.T) {
	released := strings.Replace(minimalValidTrace, `"consoleCompatibilityVersion":"development"`,
		`"consoleCompatibilityVersion":"1.2.3"`, 1)
	if _, domain := newProcessorForVersion("1.2.3").Process(artifact.ProcessRequest{
		Context: context.Background(), Metadata: artifact.TraceMetadata{TraceID: "trace-t"},
		Raw: strings.NewReader(released), Sink: &fakeSink{},
	}); domain != nil {
		t.Fatalf("exact release was rejected: %v", domain)
	}

	startOnly := strings.SplitN(minimalValidTrace, "\n", 2)[0] + "\n"
	_, domain := newProcessorForVersion("development").Process(artifact.ProcessRequest{
		Context: context.Background(), Metadata: artifact.TraceMetadata{TraceID: "trace-t"},
		Raw: strings.NewReader(startOnly), Sink: &fakeSink{},
	})
	if domain == nil || domain.Code != consolecore.CodeInvalidArtifact {
		t.Fatalf("matching development bypassed semantic validation: %v", domain)
	}
	if category, ok := categoryOf(domain); !ok || category != CategoryMissingCompletion {
		t.Fatalf("expected MISSING_COMPLETION after compatibility passed, got %v", domain)
	}
}

func TestImportPreflightReplaysConsumedHeaderBytesExactly(t *testing.T) {
	processor := newProcessorForVersion("development")
	preflight, domain := processor.PreflightImport(context.Background(), strings.NewReader(minimalValidTrace))
	if domain != nil {
		t.Fatalf("PreflightImport failed: %v", domain)
	}
	replayed, err := io.ReadAll(preflight.Raw)
	if err != nil {
		t.Fatal(err)
	}
	if string(replayed) != minimalValidTrace || preflight.Header.TraceID != "trace-t" || preflight.Header.SessionID != "session-t" {
		t.Fatalf("preflight did not preserve exact bytes or identity: header=%+v", preflight.Header)
	}
}

// TestProcessorRejectsMalformedNDJSON proves the processor rejects a malformed
// line with INVALID_ARTIFACT so the service removes the staged bundle.
func TestProcessorRejectsMalformedNDJSON(t *testing.T) {
	raw := strings.NewReader(minimalValidTrace + "{not valid json}\n")
	sink := &fakeSink{}
	processor := New()

	_, domain := processor.Process(artifact.ProcessRequest{
		Context:  context.Background(),
		Metadata: artifact.TraceMetadata{TraceID: "trace-t"},
		Raw:      raw,
		Sink:     sink,
	})
	if domain == nil || domain.Code != consolecore.CodeInvalidArtifact {
		t.Fatalf("expected INVALID_ARTIFACT, got %v", domain)
	}
	cat, ok := categoryOf(domain)
	if !ok || cat != CategoryMalformedJSON {
		t.Fatalf("expected category MALFORMED_JSON, got %v", cat)
	}
}

// TestProcessorRejectsBlankArtifact proves the processor rejects a zero-record
// artifact because it has no terminal completion.
func TestProcessorRejectsBlankArtifact(t *testing.T) {
	raw := strings.NewReader("")
	sink := &fakeSink{}
	processor := New()

	_, domain := processor.Process(artifact.ProcessRequest{
		Context:  context.Background(),
		Metadata: artifact.TraceMetadata{TraceID: "trace-t"},
		Raw:      raw,
		Sink:     sink,
	})
	if domain == nil || domain.Code != consolecore.CodeInvalidArtifact {
		t.Fatalf("expected INVALID_ARTIFACT, got %v", domain)
	}
	cat, ok := categoryOf(domain)
	if !ok || cat != CategoryMissingCompletion {
		t.Fatalf("expected category MISSING_COMPLETION, got %v", cat)
	}
}

// TestProcessorCancellationReturnsError proves the processor respects context
// cancellation and returns a domain error.
func TestProcessorCancellationReturnsError(t *testing.T) {
	raw := &slowReader{delay: 100 * time.Millisecond}
	sink := &fakeSink{}
	processor := New()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, domain := processor.Process(artifact.ProcessRequest{
		Context:  ctx,
		Metadata: artifact.TraceMetadata{TraceID: "trace-t"},
		Raw:      raw,
		Sink:     sink,
	})
	if domain == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// TestProcessorSinkCreateFailurePropagates proves a sink Create failure is
// propagated as a domain error.
func TestProcessorSinkCreateFailurePropagates(t *testing.T) {
	sink := &fakeSink{
		createErr: consolecore.NewError(consolecore.CodeLocalStorageUnavailable,
			"storage unavailable", "trace-t", consolecore.Details{}, nil),
	}
	processor := New()

	_, domain := processor.Process(artifact.ProcessRequest{
		Context:  context.Background(),
		Metadata: artifact.TraceMetadata{TraceID: "trace-t"},
		Raw:      strings.NewReader(minimalValidTrace),
		Sink:     sink,
	})
	if domain == nil || domain.Code != consolecore.CodeLocalStorageUnavailable {
		t.Fatalf("expected LOCAL_STORAGE_UNAVAILABLE, got %v", domain)
	}
}

// slowReader is a reader that delays each Read call, used to test cancellation.
type slowReader struct {
	delay time.Duration
}

func (r *slowReader) Read(p []byte) (int, error) {
	time.Sleep(r.delay)
	return 0, io.EOF
}

// TestProcessorErrorPathDoesNotLeakValidatorGoroutines proves that when the
// processor rejects a trace after at least one chunked-payload envelope has been
// opened, the per-build JSON stream validator goroutines are cleaned up and do
// not leak. The goroutine count is compared before and after a failing run that
// opens a JSON stream validator (which blocks on a pipe reader until cleanup).
func TestProcessorErrorPathDoesNotLeakValidatorGoroutines(t *testing.T) {
	// Envelope declares a 2-chunk JSON payload, but only the first chunk
	// arrives before a malformed record aborts the parse. The validator
	// goroutine is started and must be cleaned up by the error path.
	raw := `{"traceId":"t","sessionId":"s","sequence":1,"timestamp":1784894400.000000000,"recordType":"TRACE_STARTED","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"th","metadata":{"consoleCompatibilityVersion":"development"},"data":null}` + "\n" +
		`{"traceId":"t","sessionId":"s","sequence":2,"timestamp":1784894400.000000000,"recordType":"STEP_STARTED","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"th","metadata":{"payloadChunked":true,"payloadId":"p1","contentType":"application/json","chunkCount":2},"data":null}` + "\n" +
		`{"traceId":"t","sessionId":"s","sequence":3,"timestamp":1784894400.000000000,"recordType":"PAYLOAD_CHUNK_APPENDED","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"th","metadata":{"payloadId":"p1","chunkIndex":0,"chunkCount":2,"contentType":"application/json"},"data":"{\"a\":"}` + "\n" +
		`{not-json}` + "\n"

	before := runtime.NumGoroutine()
	sink := &fakeSink{}
	processor := New()
	_, domain := processor.Process(artifact.ProcessRequest{
		Context:  context.Background(),
		Metadata: artifact.TraceMetadata{TraceID: "t"},
		Raw:      strings.NewReader(raw),
		Sink:     sink,
	})
	if domain == nil {
		t.Fatal("expected error for malformed trailing record")
	}
	t.Logf("got error: %v", domain)
	// Allow the cleanup goroutines to settle.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= before {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	after := runtime.NumGoroutine()
	if after > before {
		t.Fatalf("goroutine leak: before=%d after=%d", before, after)
	}
}
