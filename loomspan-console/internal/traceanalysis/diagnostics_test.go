package traceanalysis

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
)

func diagnosticRecord(seq int, failureID string, data any, metadata map[string]any) string {
	meta := map[string]any{"failureId": failureID}
	for key, value := range metadata {
		meta[key] = value
	}
	metaJSON, _ := json.Marshal(meta)
	dataJSON, _ := json.Marshal(data)
	return `{"traceId":"t","sessionId":"s","sequence":` + itoa(seq) +
		`,"timestamp":` + timestampForSeq(seq) +
		`,"recordType":"ERROR_RECORDED","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"th","metadata":` + string(metaJSON) + `,"data":` + string(dataJSON) + `}`
}

func diagnostic(kind, text string, truncated any, limit int) map[string]any {
	return map[string]any{
		"kind": kind, "contentType": "text/plain; charset=utf-8", "text": text,
		"truncated": truncated, "captureLimitBytes": limit,
	}
}

func diagnosticData(values ...map[string]any) map[string]any {
	return map[string]any{
		"exceptionType": "java.lang.IllegalStateException",
		"message":       "failed",
		"diagnostics":   values,
	}
}

func chunkedDiagnosticTrace(t *testing.T, data map[string]any) string {
	return chunkedDiagnosticTraceWithFailureID(t, "failure-1", data)
}

func chunkedDiagnosticTraceWithFailureID(t *testing.T, failureID string, data map[string]any) string {
	t.Helper()
	logical, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	const chunkSize = 512 << 10
	chunkCount := (len(logical) + chunkSize - 1) / chunkSize
	meta := map[string]any{
		"payloadChunked": true, "payloadId": "error-payload", "contentType": "application/json", "chunkCount": chunkCount,
	}
	var raw strings.Builder
	raw.WriteString(startedRecord(1) + "\n")
	raw.WriteString(diagnosticRecord(2, failureID, nil, meta) + "\n")
	for index, start := 0, 0; start < len(logical); index, start = index+1, start+chunkSize {
		end := start + chunkSize
		if end > len(logical) {
			end = len(logical)
		}
		raw.WriteString(chunkRecord(3+index, "error-payload", "application/json", index, chunkCount, string(logical[start:end])) + "\n")
	}
	raw.WriteString(completionRecord(3+chunkCount, "SUCCEEDED", 0, 0, 0, "") + "\n")
	return raw.String()
}

func TestProcessorRejectsFailureWithoutCanonicalIdentity(t *testing.T) {
	data := diagnosticData(diagnostic("K", "stack", false, 16))
	blank := startedRecord(1) + "\n" + diagnosticRecord(2, "", data, nil) + "\n" + completionRecord(3, "SUCCEEDED", 0, 0, 0, "") + "\n"
	missing := strings.Replace(blank, `"metadata":{"failureId":""}`, `"metadata":{}`, 1)
	chunked := chunkedDiagnosticTraceWithFailureID(t, "", data)
	for name, raw := range map[string]string{"blank inline": blank, "missing inline": missing, "blank chunked": chunked} {
		t.Run(name, func(t *testing.T) {
			if _, category, ok := processTrace(t, raw); ok || category != CategoryUnsupportedValue {
				t.Fatalf("valid=%v category=%s", ok, category)
			}
		})
	}
}

func TestProcessorValidatesFailureDiagnosticSchema(t *testing.T) {
	validUnknownRepeated := diagnosticData(
		diagnostic("VENDOR_TRACE", "first", false, 1<<20),
		diagnostic("VENDOR_TRACE", "second", true, 1<<20),
	)
	validSixteen := make([]map[string]any, 16)
	for i := range validSixteen {
		validSixteen[i] = diagnostic("KIND", "x", false, 1)
	}
	cases := []struct {
		name  string
		data  any
		valid bool
	}{
		{name: "unknown repeated kinds", data: validUnknownRepeated, valid: true},
		{name: "sixteen diagnostics", data: diagnosticData(validSixteen...), valid: true},
		{name: "missing diagnostics", data: map[string]any{"exceptionType": "E", "message": "m"}},
		{name: "null data", data: nil},
		{name: "blank kind", data: diagnosticData(diagnostic(" ", "x", false, 1))},
		{name: "blank content type", data: diagnosticData(map[string]any{"kind": "K", "contentType": " ", "text": "x", "truncated": false, "captureLimitBytes": 1})},
		{name: "missing truncated", data: diagnosticData(map[string]any{"kind": "K", "contentType": "text/plain", "text": "x", "captureLimitBytes": 1})},
		{name: "missing text", data: diagnosticData(map[string]any{"kind": "K", "contentType": "text/plain", "truncated": false, "captureLimitBytes": 1})},
		{name: "zero capture limit", data: diagnosticData(diagnostic("K", "", false, 0))},
		{name: "capture limit too large", data: diagnosticData(diagnostic("K", "x", false, (1<<20)+1))},
		{name: "text exceeds declared limit", data: diagnosticData(diagnostic("K", "xx", false, 1))},
		{name: "seventeen diagnostics", data: diagnosticData(append(validSixteen, diagnostic("KIND", "x", false, 1))...)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := startedRecord(1) + "\n" + diagnosticRecord(2, "failure-1", tc.data, nil) + "\n" + completionRecord(3, "SUCCEEDED", 0, 0, 0, "") + "\n"
			_, category, ok := processTrace(t, raw)
			if ok != tc.valid {
				t.Fatalf("valid=%v, category=%s", ok, category)
			}
			if !tc.valid && category != CategoryUnsupportedValue {
				t.Fatalf("category=%s, want %s", category, CategoryUnsupportedValue)
			}
		})
	}
}

func TestProcessorValidatesChunkedDiagnosticBoundsAfterReconstruction(t *testing.T) {
	t.Run("one mebibyte item accepted", func(t *testing.T) {
		raw := chunkedDiagnosticTrace(t, diagnosticData(diagnostic("K", strings.Repeat("x", 1<<20), false, 1<<20)))
		if _, category, ok := processTrace(t, raw); !ok {
			t.Fatalf("category=%s", category)
		}
	})
	t.Run("item over one mebibyte rejected", func(t *testing.T) {
		raw := chunkedDiagnosticTrace(t, diagnosticData(diagnostic("K", strings.Repeat("x", (1<<20)+1), false, 1<<20)))
		if _, category, ok := processTrace(t, raw); ok || category != CategoryUnsupportedValue {
			t.Fatalf("valid=%v category=%s", ok, category)
		}
	})
	t.Run("aggregate over four mebibytes rejected", func(t *testing.T) {
		values := make([]map[string]any, 5)
		for i := range values {
			values[i] = diagnostic("K", strings.Repeat("x", 900<<10), false, 1<<20)
		}
		raw := chunkedDiagnosticTrace(t, diagnosticData(values...))
		if _, category, ok := processTrace(t, raw); ok || category != CategoryUnsupportedValue {
			t.Fatalf("valid=%v category=%s", ok, category)
		}
	})
	t.Run("escaped one mebibyte item accepted", func(t *testing.T) {
		raw := chunkedDiagnosticTrace(t, diagnosticData(diagnostic("K", strings.Repeat("\x00", 1<<20), false, 1<<20)))
		if _, category, ok := processTrace(t, raw); !ok {
			t.Fatalf("category=%s", category)
		}
	})
}

func TestLargeDiagnosticTextExistsOnlyInPayloadStoreDerivedArtifact(t *testing.T) {
	const sentinel = "UNIQUE_LARGE_DIAGNOSTIC_SENTINEL"
	raw := chunkedDiagnosticTrace(t, diagnosticData(
		diagnostic("JVM_STACK_TRACE", sentinel+strings.Repeat("x", 900<<10), false, 1<<20),
	))
	sink := &fakeSink{}
	_, domain := New().Process(artifact.ProcessRequest{
		Context:  context.Background(),
		Metadata: artifact.TraceMetadata{TraceID: "t", SessionID: "s"},
		Raw:      strings.NewReader(raw),
		Sink:     sink,
	})
	if domain != nil {
		t.Fatalf("process large diagnostic: %v", domain)
	}

	for name, content := range sink.components {
		occurrences := bytes.Count(content, []byte(sentinel))
		if name == artifact.ComponentName(ComponentPayloadStore) {
			if occurrences != 1 {
				t.Fatalf("payload store sentinel occurrences=%d, want 1", occurrences)
			}
			continue
		}
		if occurrences != 0 {
			t.Fatalf("derived component %s duplicated diagnostic text", name)
		}
	}
}

func TestFailuresRejectDuplicateIDsAndDeriveTerminalityOnlyFromCompletion(t *testing.T) {
	duplicate := startedRecord(1) + "\n" + errorRecord(2, "same", false) + "\n" + errorRecord(3, "same", false) + "\n" + completionRecord(4, "SUCCEEDED", 0, 0, 0, "") + "\n"
	if _, category, ok := processTrace(t, duplicate); ok || category != CategoryInvalidTerminalFailure {
		t.Fatalf("duplicate valid=%v category=%s", ok, category)
	}
	recoveredWithLegacyFlag := startedRecord(1) + "\n" + diagnosticRecord(2, "recovered", diagnosticData(diagnostic("K", "x", false, 1)), map[string]any{"terminal": true}) + "\n" + completionRecord(3, "SUCCEEDED", 0, 0, 0, "") + "\n"
	if _, category, ok := processTrace(t, recoveredWithLegacyFlag); !ok {
		t.Fatalf("legacy flag incorrectly controlled terminality: %s", category)
	}
}
