package traceanalysis

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/evidence"
)

func TestContentReferenceBindsSourceHandleAndKind(t *testing.T) {
	handle := artifact.Handle(strings.Repeat("a", 64))
	token, err := encodeEnvelopeContentReference(evidence.ForImported(), handle, "payload-1")
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeContentReference(token)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Source != evidence.SourceImported || decoded.Handle != string(handle) || decoded.PayloadID != "payload-1" || decoded.Kind != contentKindSemantic || decoded.ContentSource != contentSourceEnvelope {
		t.Fatalf("decoded=%#v", decoded)
	}
	if err := validateContentReference(decoded, evidence.ForTarget("scope"), handle); err == nil {
		t.Fatal("cross-source reference accepted")
	}
}

func TestContentReferenceIdentifierBoundaryIsValidatedBeforePublication(t *testing.T) {
	largest := largestFittingContentReferenceIdentifier()
	if largest == "" || !contentReferenceIdentifierFits(largest) {
		t.Fatal("failed to locate fitting content-reference identifier boundary")
	}
	tooLarge := largest + "x"
	if contentReferenceIdentifierFits(tooLarge) {
		t.Fatal("one-byte-over content-reference identifier unexpectedly fits")
	}

	metadata, err := json.Marshal(map[string]any{"payloadId": largest, "payloadChunked": true})
	if err != nil {
		t.Fatal(err)
	}
	line := `{"traceId":"t","sessionId":"s","sequence":1,"timestamp":1784894400.000000000,"recordType":"MODEL_RESPONSE_RECEIVED","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"th","metadata":` + string(metadata) + `,"data":null}`
	if _, domain := collectRecords(t, line); domain != nil {
		t.Fatalf("maximum fitting payload identifier was rejected: %v", domain)
	}

	metadata, err = json.Marshal(map[string]any{"payloadId": tooLarge, "payloadChunked": true})
	if err != nil {
		t.Fatal(err)
	}
	line = `{"traceId":"t","sessionId":"s","sequence":1,"timestamp":1784894400.000000000,"recordType":"MODEL_RESPONSE_RECEIVED","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"th","metadata":` + string(metadata) + `,"data":null}`
	if _, domain := collectRecords(t, line); domain == nil {
		t.Fatal("one-byte-over payload identifier was accepted")
	} else if category, ok := categoryOf(domain); !ok || category != CategoryUnsupportedValue {
		t.Fatalf("oversized payload category=%q ok=%v", category, ok)
	}

	failureMetadata, err := json.Marshal(map[string]any{"failureId": tooLarge})
	if err != nil {
		t.Fatal(err)
	}
	if domain := newFailureGraph().onErrorRecord(&Record{TraceID: "t", Metadata: failureMetadata}); domain == nil {
		t.Fatal("one-byte-over failure identifier was accepted")
	}

	projectionTooLarge := strings.Repeat("x", maxEncodedTokenLength)
	_, err = materializeRecordFacts(persistedRecordFacts{Payloads: []payloadIndexRow{{PayloadID: projectionTooLarge}}}, &Record{}, RecordFilter{}, evidence.ForImported(), artifact.Handle(strings.Repeat("a", 64)), TraceContext{})
	if err == nil {
		t.Fatal("defensive record projection silently discarded an oversized reference error")
	}
}

func largestFittingContentReferenceIdentifier() string {
	low, high := 0, maxEncodedTokenLength
	for low < high {
		mid := low + (high-low+1)/2
		if contentReferenceIdentifierFits(strings.Repeat("x", mid)) {
			low = mid
		} else {
			high = mid - 1
		}
	}
	return strings.Repeat("x", low)
}

func TestImportedCursorOwnerIsAdapterSafe(t *testing.T) {
	owner, _ := evidence.Imported("process-local-secret")
	if got := ownerCursorKey(owner); got != "IMPORTED" {
		t.Fatalf("owner key=%q", got)
	}
	token, err := encodePositionCursor(cursorOpRecords, ownerCursorKey(owner), artifact.Handle(strings.Repeat("b", 64)), "fingerprint", 1)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeCursor(token)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.OwnerKey != "IMPORTED" || strings.Contains(token, "process-local-secret") {
		t.Fatalf("cursor=%#v token=%s", decoded, token)
	}
}

func TestOldImportedOwnerCursorIsRejectedWithoutFallback(t *testing.T) {
	handle := artifact.Handle(strings.Repeat("c", 64))
	token, err := encodePositionCursor(cursorOpRecords, "IMPORTED:process-owner", handle, "fingerprint", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, domain := prepareCursor(token, "IMPORTED", "IMPORTED", cursorOpRecords); domain == nil {
		t.Fatal("old imported owner cursor was accepted")
	}
}
