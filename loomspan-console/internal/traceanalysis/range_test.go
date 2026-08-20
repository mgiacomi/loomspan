package traceanalysis

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/target"
)

func mustEnvelopeContentRef(t *testing.T, scopeID target.ScopeID, handle artifact.Handle, payloadID string) string {
	t.Helper()
	ref, err := encodeEnvelopeContentReference(targetEvidence(scopeID), handle, payloadID)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func TestRangeCandidateExactnessAtOneFourSixteenAndThirtyTwoMiB(t *testing.T) {
	for _, size := range []int{1 << 20, 4 << 20, 16 << 20, 32 << 20} {
		t.Run(fmt.Sprintf("%dMiB", size>>20), func(t *testing.T) {
			textSource := bytes.Repeat([]byte("loomspan"), size/8)
			encoding, content, start, end := encodeRangeContent(textSource, "text/plain", 0, int64(len(textSource)), int64(len(textSource)))
			if encoding != RangeEncodingText || start != 0 || end != int64(len(textSource)) || sha256.Sum256(content) != sha256.Sum256(textSource) {
				t.Fatal("UTF-8 candidate range was not exact")
			}
			binarySource := bytes.Repeat([]byte{0xff, 0x00, 0x80, 0x7f}, size/4)
			encoding, content, start, end = encodeRangeContent(binarySource, "application/octet-stream", 0, int64(len(binarySource)), int64(len(binarySource)))
			decoded, err := base64.StdEncoding.DecodeString(string(content))
			if err != nil || encoding != RangeEncodingBase64 || start != 0 || end != int64(len(binarySource)) || sha256.Sum256(decoded) != sha256.Sum256(binarySource) {
				t.Fatalf("base64 candidate range was not exact: %v", err)
			}
		})
	}
}

func TestSixteenMiBPayloadRangeExactConcurrentCancellableAndDeadlineBound(t *testing.T) {
	if testing.Short() {
		t.Skip("large framing gate")
	}
	const size = 16 << 20
	h := newServiceTestHarness(t, "t", chunkedPayloadTrace(size, 64))
	read := func(ctx context.Context) ([]byte, *consolecore.Error) {
		result, domain := h.service.ReadContentRange(ctx, targetEvidence(h.scopeID), RangeRequest{Handle: h.handle, Source: RangeSourceContent, ContentRef: mustEnvelopeContentRef(t, h.scopeID, h.handle, "payload-1"), Start: 0, MaxBytes: size})
		return result.Content, domain
	}
	want := sha256.Sum256(bytes.Repeat([]byte("x"), size))
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			content, domain := read(context.Background())
			if domain != nil || len(content) != size || sha256.Sum256(content) != want {
				errs <- fmt.Errorf("content=%d domain=%v", len(content), domain)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, domain := read(cancelled); domain == nil {
		t.Fatal("canceled range succeeded")
	}
	deadline, cancelDeadline := context.WithDeadline(context.Background(), time.Now().Add(-time.Millisecond))
	defer cancelDeadline()
	if _, domain := read(deadline); domain == nil || !errors.Is(domain, context.DeadlineExceeded) {
		t.Fatalf("expired deadline domain=%v", domain)
	}
}

func TestRangeReadObservesCancellationAfterIOStarts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	reader := newBlockingReadCloser()
	completed := make(chan error, 1)
	go func() {
		_, err := readRangeBytes(ctx, reader, make([]byte, 1))
		completed <- err
	}()
	<-reader.started
	cancel()
	select {
	case err := <-completed:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected cancellation, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("range read did not stop after cancellation")
	}
}

func TestSixteenMiBBase64FramingHasBoundedAllocationEnvelope(t *testing.T) {
	if testing.Short() {
		t.Skip("large framing gate")
	}
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	source := bytes.Repeat([]byte{0xff, 0x00, 0x80, 0x7f}, (16<<20)/4)
	encoding, content, _, _ := encodeRangeContent(source, "application/octet-stream", 0, int64(len(source)), int64(len(source)))
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(content)
	if encoding != RangeEncodingBase64 || len(content) != base64.StdEncoding.EncodedLen(len(source)) {
		t.Fatalf("encoding=%s content=%d", encoding, len(content))
	}
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 96<<20 {
		t.Fatalf("16 MiB base64 framing allocated %d bytes; envelope is %d", allocated, 96<<20)
	}
}

func TestServiceReadRawArtifactRange(t *testing.T) {
	h := newServiceTestHarness(t, "trace-t", minimalValidTrace)
	// Read the first 100 bytes of the raw artifact.
	result, domain := h.service.ReadRawArtifactRange(context.Background(), targetEvidence(h.scopeID), RangeRequest{
		Handle:   h.handle,
		Source:   RangeSourceRawArtifact,
		Start:    0,
		MaxBytes: 100,
	})
	if domain != nil {
		t.Fatalf("ReadRawArtifactRange failed: %v", domain)
	}
	if result.Source != RangeSourceRawArtifact {
		t.Fatalf("expected source RAW_ARTIFACT, got %q", result.Source)
	}
	if result.ActualStart != 0 {
		t.Fatalf("expected actual start 0, got %d", result.ActualStart)
	}
	if result.ActualEnd > 100 {
		t.Fatalf("expected actual end <= 100, got %d", result.ActualEnd)
	}
	if result.TotalLength <= 0 {
		t.Fatalf("expected positive total length, got %d", result.TotalLength)
	}
	if len(result.Content) == 0 {
		t.Fatal("expected non-empty content")
	}
	if result.Encoding != RangeEncodingText {
		t.Fatalf("expected TEXT encoding for NDJSON, got %q", result.Encoding)
	}
}

func TestServiceReadRawArtifactRangeContinuation(t *testing.T) {
	h := newServiceTestHarness(t, "trace-t", minimalValidTrace)
	// Read the first 50 bytes.
	page1, domain := h.service.ReadRawArtifactRange(context.Background(), targetEvidence(h.scopeID), RangeRequest{
		Handle:   h.handle,
		Source:   RangeSourceRawArtifact,
		Start:    0,
		MaxBytes: 50,
	})
	if domain != nil {
		t.Fatalf("page 1 failed: %v", domain)
	}
	if !page1.HasMore {
		t.Fatal("expected hasMore on page 1")
	}
	if page1.NextCursor == "" {
		t.Fatal("expected non-empty next cursor")
	}
	// Continue from the cursor.
	page2, domain := h.service.ReadRawArtifactRange(context.Background(), targetEvidence(h.scopeID), RangeRequest{
		Handle:         h.handle,
		Source:         RangeSourceRawArtifact,
		ContinueCursor: page1.NextCursor,
		MaxBytes:       50,
	})
	if domain != nil {
		t.Fatalf("page 2 failed: %v", domain)
	}
	if page2.ActualStart != page1.ActualEnd {
		t.Fatalf("expected page 2 start %d to match page 1 end %d", page2.ActualStart, page1.ActualEnd)
	}
}

func TestServiceReadRawArtifactRangeEmptyAtEnd(t *testing.T) {
	h := newServiceTestHarness(t, "trace-t", minimalValidTrace)
	// Read past the end.
	result, domain := h.service.ReadRawArtifactRange(context.Background(), targetEvidence(h.scopeID), RangeRequest{
		Handle:   h.handle,
		Source:   RangeSourceRawArtifact,
		Start:    1 << 20, // way past the end
		MaxBytes: 100,
	})
	if domain != nil {
		t.Fatalf("ReadRawArtifactRange failed: %v", domain)
	}
	if len(result.Content) != 0 {
		t.Fatalf("expected empty content past end, got %d bytes", len(result.Content))
	}
	if result.HasMore {
		t.Fatal("expected no more pages past end")
	}
}

func TestServiceReadRawRecordRange(t *testing.T) {
	h := newServiceTestHarness(t, "trace-t", minimalValidTrace)
	// Read the first record (sequence 1).
	result, domain := h.service.ReadRawRecordRange(context.Background(), targetEvidence(h.scopeID), RangeRequest{
		Handle:         h.handle,
		Source:         RangeSourceRawRecord,
		RecordSequence: 1,
		Start:          0,
		MaxBytes:       maxRangeBytes,
	})
	if domain != nil {
		t.Fatalf("ReadRawRecordRange failed: %v", domain)
	}
	if result.Source != RangeSourceRawRecord {
		t.Fatalf("expected source RAW_RECORD, got %q", result.Source)
	}
	if result.TotalLength <= 0 {
		t.Fatalf("expected positive total length, got %d", result.TotalLength)
	}
	if len(result.Content) == 0 {
		t.Fatal("expected non-empty content")
	}
	want := minimalValidTrace[:strings.IndexByte(minimalValidTrace, '\n')]
	if got := string(result.Content); got != want {
		t.Fatalf("raw record crossed its physical boundary:\ngot:  %q\nwant: %q", got, want)
	}
	if result.ActualEnd != result.TotalLength {
		t.Fatalf("expected complete record ending at %d, got %d", result.TotalLength, result.ActualEnd)
	}
	if result.HasMore {
		t.Fatal("expected no continuation after the complete record")
	}
}

func TestServiceReadRawRecordRangeNotFound(t *testing.T) {
	h := newServiceTestHarness(t, "trace-t", minimalValidTrace)
	_, domain := h.service.ReadRawRecordRange(context.Background(), targetEvidence(h.scopeID), RangeRequest{
		Handle:         h.handle,
		Source:         RangeSourceRawRecord,
		RecordSequence: 999,
		Start:          0,
		MaxBytes:       100,
	})
	if domain == nil {
		t.Fatal("expected error for nonexistent record")
	}
}

func TestServiceReadContentRange(t *testing.T) {
	ndjson := chunkedPayloadTrace(256, 2)
	h := newServiceTestHarness(t, "t", ndjson)
	// Read the first 50 bytes of payload-1.
	result, domain := h.service.ReadContentRange(context.Background(), targetEvidence(h.scopeID), RangeRequest{
		Handle:     h.handle,
		Source:     RangeSourceContent,
		ContentRef: mustEnvelopeContentRef(t, h.scopeID, h.handle, "payload-1"),
		Start:      0,
		MaxBytes:   50,
	})
	if domain != nil {
		t.Fatalf("ReadContentRange failed: %v", domain)
	}
	if result.Source != RangeSourceContent {
		t.Fatalf("expected source PAYLOAD, got %q", result.Source)
	}
	if result.ContentType != "text/plain" {
		t.Fatalf("expected content type text/plain, got %q", result.ContentType)
	}
	if result.TotalLength != 256 {
		t.Fatalf("expected total length 256, got %d", result.TotalLength)
	}
	if len(result.Content) == 0 {
		t.Fatal("expected non-empty content")
	}
	if result.Encoding != RangeEncodingText {
		t.Fatalf("expected TEXT encoding, got %q", result.Encoding)
	}
}

func TestServiceReadContentRangeNotFound(t *testing.T) {
	h := newServiceTestHarness(t, "trace-t", minimalValidTrace)
	_, domain := h.service.ReadContentRange(context.Background(), targetEvidence(h.scopeID), RangeRequest{
		Handle:     h.handle,
		Source:     RangeSourceContent,
		ContentRef: mustEnvelopeContentRef(t, h.scopeID, h.handle, "nonexistent"),
		Start:      0,
		MaxBytes:   100,
	})
	if domain == nil {
		t.Fatal("expected error for nonexistent payload")
	}
}

func TestServiceReadContentRangeRejectsPageLocalContentID(t *testing.T) {
	h := newServiceTestHarness(t, "trace-t", minimalValidTrace)
	_, domain := h.service.ReadContentRange(context.Background(), targetEvidence(h.scopeID), RangeRequest{Handle: h.handle, Source: RangeSourceContent, ContentRef: "c1", Start: 0, MaxBytes: 100})
	if domain == nil || domain.Code != consolecore.CodeInvalidArgument {
		t.Fatalf("contentId domain=%v", domain)
	}
}

func TestServiceReadContentRangeContinuation(t *testing.T) {
	ndjson := chunkedPayloadTrace(256, 2)
	h := newServiceTestHarness(t, "t", ndjson)
	page1, domain := h.service.ReadContentRange(context.Background(), targetEvidence(h.scopeID), RangeRequest{
		Handle:     h.handle,
		Source:     RangeSourceContent,
		ContentRef: mustEnvelopeContentRef(t, h.scopeID, h.handle, "payload-1"),
		Start:      0,
		MaxBytes:   100,
	})
	if domain != nil {
		t.Fatalf("page 1 failed: %v", domain)
	}
	if !page1.HasMore {
		t.Fatal("expected hasMore on page 1")
	}
	page2, domain := h.service.ReadContentRange(context.Background(), targetEvidence(h.scopeID), RangeRequest{
		Handle:         h.handle,
		Source:         RangeSourceContent,
		ContentRef:     mustEnvelopeContentRef(t, h.scopeID, h.handle, "payload-1"),
		ContinueCursor: page1.NextCursor,
		MaxBytes:       100,
	})
	if domain != nil {
		t.Fatalf("page 2 failed: %v", domain)
	}
	if page2.ActualStart != page1.ActualEnd {
		t.Fatalf("expected page 2 start %d to match page 1 end %d", page2.ActualStart, page1.ActualEnd)
	}
}

func TestServiceReadRawArtifactRangeExceedsLimit(t *testing.T) {
	h := newServiceTestHarness(t, "trace-t", minimalValidTrace)
	_, domain := h.service.ReadRawArtifactRange(context.Background(), targetEvidence(h.scopeID), RangeRequest{
		Handle:   h.handle,
		Source:   RangeSourceRawArtifact,
		Start:    0,
		MaxBytes: maxRangeBytes + 1,
	})
	if domain == nil {
		t.Fatal("expected LIMIT_EXCEEDED error")
	}
}

func TestEncodeRangeContentUTF8Boundary(t *testing.T) {
	full := []byte("héllo")
	buf := full[:2]
	encoding, content, start, end := encodeRangeContent(buf, "text/plain", 0, 2, int64(len(full)))
	if encoding != RangeEncodingBase64 {
		t.Fatalf("expected BASE64 encoding, got %q", encoding)
	}
	decoded, err := base64.StdEncoding.DecodeString(string(content))
	if err != nil || string(decoded) != string(buf) {
		t.Fatalf("range bytes changed: decoded=%x err=%v want=%x", decoded, err, buf)
	}
	if start != 0 || end != 2 {
		t.Fatalf("expected exact offsets [0,2), got [%d,%d)", start, end)
	}
}

func TestEncodeRangeContentUTF8StartBoundary(t *testing.T) {
	// Test that a start offset landing mid-character is advanced to the next
	// complete code point. "héllo" = h(1) + é(0xC3 0xA9, 2 bytes) + llo(3).
	// Requesting bytes [1:6] gives 0xC3 (start of é) through 'o'. The start
	// is already at a rune start, so no trimming is needed.
	full := []byte("héllo")
	buf := full[1:] // 0xC3 0xA9 l l o — starts at a rune start
	encoding, _, start, end := encodeRangeContent(buf, "text/plain", 1, int64(len(full)), int64(len(full)))
	if encoding != RangeEncodingText {
		t.Fatalf("expected TEXT encoding, got %q", encoding)
	}
	if start != 1 {
		t.Fatalf("expected start 1, got %d", start)
	}
	if end != int64(len(full)) {
		t.Fatalf("expected end %d, got %d", len(full), end)
	}
	// A start that lands mid-character must preserve the exact bytes as base64.
	buf2 := full[2:] // 0xA9 l l o — starts with a continuation byte
	encoding2, content2, start2, end2 := encodeRangeContent(buf2, "text/plain", 2, int64(len(full)), int64(len(full)))
	if encoding2 != RangeEncodingBase64 {
		t.Fatalf("expected BASE64 encoding, got %q", encoding2)
	}
	decoded, err := base64.StdEncoding.DecodeString(string(content2))
	if err != nil || string(decoded) != string(buf2) {
		t.Fatalf("range bytes changed: decoded=%x err=%v want=%x", decoded, err, buf2)
	}
	if start2 != 2 {
		t.Fatalf("expected exact start 2, got %d", start2)
	}
	if end2 != int64(len(full)) {
		t.Fatalf("expected end %d, got %d", len(full), end2)
	}
}

func TestEncodeRangeContentBase64ForNonUTF8(t *testing.T) {
	// Non-UTF-8 bytes should be base64-encoded.
	input := []byte{0xFF, 0xFE, 0xFD}
	encoding, content, _, end := encodeRangeContent(input, "application/octet-stream", 0, 3, 3)
	if encoding != RangeEncodingBase64 {
		t.Fatalf("expected BASE64 encoding, got %q", encoding)
	}
	if end != 3 {
		t.Fatalf("expected end 3, got %d", end)
	}
	if len(content) == 0 {
		t.Fatal("expected non-empty base64 content")
	}
}

func TestServiceReadRawArtifactRangeExpiredHandle(t *testing.T) {
	h := newServiceTestHarness(t, "trace-t", minimalValidTrace)
	_, domain := h.service.ReadRawArtifactRange(context.Background(), targetEvidence(h.scopeID), RangeRequest{
		Handle:   artifact.Handle("nonexistent"),
		Source:   RangeSourceRawArtifact,
		Start:    0,
		MaxBytes: 100,
	})
	if domain == nil {
		t.Fatal("expected error for nonexistent handle")
	}
}

func TestServiceRawRangeOneByteTraversalIsLossless(t *testing.T) {
	raw := strings.Replace(minimalValidTrace, "traces/t.ndjson", "traces/é.ndjson", 1)
	h := newServiceTestHarness(t, "trace-t", raw)
	request := RangeRequest{Handle: h.handle, Source: RangeSourceRawArtifact, MaxBytes: 1}
	var reconstructed []byte
	for {
		result, domain := h.service.ReadRawArtifactRange(context.Background(), targetEvidence(h.scopeID), request)
		if domain != nil {
			t.Fatalf("read range: %v", domain)
		}
		chunk := result.Content
		if result.Encoding == RangeEncodingBase64 {
			var err error
			chunk, err = base64.StdEncoding.DecodeString(string(result.Content))
			if err != nil {
				t.Fatalf("decode base64 range: %v", err)
			}
		}
		reconstructed = append(reconstructed, chunk...)
		if !result.HasMore {
			break
		}
		request.ContinueCursor = result.NextCursor
	}
	if string(reconstructed) != raw {
		t.Fatalf("one-byte traversal changed artifact: got %x want %x", reconstructed, []byte(raw))
	}
}

// Suppress unused import warnings for strings (used by deriveSessionID).
var _ = strings.Contains
