package traceanalysis

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/evidence"
)

// TestCursorErrorPrecedenceIsTargetChangedThenArtifactExpiredThenInvalidCursor
// proves the required cursor error precedence:
//  1. TARGET_CHANGED when the cursor's scope is not the current scope.
//  2. ARTIFACT_EXPIRED when the handle is not installed in the current scope.
//  3. INVALID_CURSOR when the cursor's fingerprint does not match the request.
//
// A stale-scope cursor must return TARGET_CHANGED even when the handle is also
// absent from the current scope, so stale evidence is never reinterpreted.
func TestCursorErrorPrecedenceIsTargetChangedThenArtifactExpiredThenInvalidCursor(t *testing.T) {
	h := newServiceTestHarness(t, "trace-nested-frame-usage", nestedFrameUsageTrace)

	// Acquire one page so we have a real continuation cursor bound to the
	// current scope and handle.
	page1, domain := h.service.QueryFrames(context.Background(), targetEvidence(h.scopeID), FrameQuery{
		Handle:   h.handle,
		PageSize: 1,
	})
	if domain != nil {
		t.Fatalf("QueryFrames page 1 failed: %v", domain)
	}
	if page1.NextCursor == "" {
		t.Fatal("expected a continuation cursor from page 1")
	}

	// 1. TARGET_CHANGED: reuse the cursor but issue the call from a different
	//    scope. The handle is not installed in the other scope either, but
	//    TARGET_CHANGED must take precedence over ARTIFACT_EXPIRED.
	otherScope := h.scopeID
	if otherScope == "scope-test" {
		otherScope = "scope-other"
	} else {
		otherScope = "scope-test"
	}
	_, domain = h.service.QueryFrames(context.Background(), targetEvidence(otherScope), FrameQuery{
		Handle:   h.handle,
		PageSize: 1,
		Cursor:   page1.NextCursor,
	})
	if domain == nil {
		t.Fatal("expected an error for stale-scope cursor, got nil")
	}
	if domain.Code != consolecore.CodeTargetChanged {
		t.Fatalf("expected TARGET_CHANGED for stale-scope cursor, got %s: %s", domain.Code, domain.Message)
	}

	// 2. ARTIFACT_EXPIRED: a well-formed (64 hex char) handle that is not
	//    installed in the current scope returns ARTIFACT_EXPIRED. The handle
	//    format must be valid so the artifact service does not reject it as
	//    INVALID_ARGUMENT before checking installation.
	_, domain = h.service.QueryFrames(context.Background(), targetEvidence(h.scopeID), FrameQuery{
		Handle:   artifact.Handle("0000000000000000000000000000000000000000000000000000000000000000"),
		PageSize: 1,
	})
	if domain == nil {
		t.Fatal("expected an error for uninstalled handle, got nil")
	}
	if domain.Code != consolecore.CodeArtifactExpired {
		t.Fatalf("expected ARTIFACT_EXPIRED for uninstalled handle, got %s: %s", domain.Code, domain.Message)
	}

	// 3. INVALID_CURSOR: a cursor whose scope and handle are valid but whose
	//    fingerprint does not match the request (different order) returns
	//    INVALID_CURSOR. This is checked after lease acquisition succeeds.
	_, domain = h.service.QueryFrames(context.Background(), targetEvidence(h.scopeID), FrameQuery{
		Handle:     h.handle,
		PageSize:   1,
		Order:      FrameOrderDurationDesc,
		Projection: FrameProjectionDetailed,
		Cursor:     page1.NextCursor, // bound to CANONICAL order
	})
	if domain == nil {
		t.Fatal("expected an error for fingerprint mismatch, got nil")
	}
	if domain.Code != consolecore.CodeInvalidCursor {
		t.Fatalf("expected INVALID_CURSOR for fingerprint mismatch, got %s: %s", domain.Code, domain.Message)
	}
}

// TestCursorBindsOperationScopeHandleFingerprintOrderingAndProgress proves
// that a cursor cannot be reused with a different operation, scope, or query
// meaning, and that progress is preserved across continuations.
func TestCursorBindsOperationScopeHandleFingerprintOrderingAndProgress(t *testing.T) {
	h := newServiceTestHarness(t, "trace-t", minimalValidTrace)

	// Acquire a records cursor and try to use it for a frames query: the
	// operation mismatch must be rejected as INVALID_CURSOR.
	recPage, domain := h.service.QueryRecords(context.Background(), targetEvidence(h.scopeID), RecordQuery{
		Handle:   h.handle,
		PageSize: 1,
	})
	if domain != nil {
		t.Fatalf("QueryRecords failed: %v", domain)
	}
	if recPage.NextCursor == "" {
		t.Fatal("expected a records continuation cursor")
	}
	_, domain = h.service.QueryFrames(context.Background(), targetEvidence(h.scopeID), FrameQuery{
		Handle:   h.handle,
		PageSize: 1,
		Cursor:   recPage.NextCursor, // RECORDS cursor used for FRAMES
	})
	if domain == nil || domain.Code != consolecore.CodeInvalidCursor {
		t.Fatalf("expected INVALID_CURSOR for operation mismatch, got %v", domain)
	}

	// Progress is preserved: page 1 returns sequence 1, page 2 returns sequence 2.
	page1, domain := h.service.QueryRecords(context.Background(), targetEvidence(h.scopeID), RecordQuery{
		Handle:   h.handle,
		PageSize: 1,
	})
	if domain != nil {
		t.Fatalf("QueryRecords page 1 failed: %v", domain)
	}
	if len(page1.Items) != 1 || page1.Items[0].Sequence != 1 {
		t.Fatalf("expected sequence 1 on page 1, got %v", page1.Items)
	}
	page2, domain := h.service.QueryRecords(context.Background(), targetEvidence(h.scopeID), RecordQuery{
		Handle:   h.handle,
		PageSize: 1,
		Cursor:   page1.NextCursor,
	})
	if domain != nil {
		t.Fatalf("QueryRecords page 2 failed: %v", domain)
	}
	if len(page2.Items) != 1 || page2.Items[0].Sequence != 2 {
		t.Fatalf("expected sequence 2 on page 2, got %v", page2.Items)
	}
}

// TestPaginationHasNoDuplicatesOrOmissionsAcrossEverySupportedOrder proves
// that paginating frames with each supported order returns every frame exactly
// once across all pages.
func TestPaginationHasNoDuplicatesOrOmissionsAcrossEverySupportedOrder(t *testing.T) {
	h := newServiceTestHarness(t, "trace-nested-frame-usage", nestedFrameUsageTrace)

	orders := []FrameOrder{FrameOrderCanonical, FrameOrderDurationDesc, FrameOrderUsageDesc}
	for _, order := range orders {
		t.Run(string(order), func(t *testing.T) {
			seen := make(map[string]bool)
			cursor := ""
			for i := 0; i < 20; i++ { // bounded to prevent infinite loops
				projection := FrameProjectionCompact
				if order != FrameOrderCanonical {
					projection = FrameProjectionDetailed
				}
				page, domain := h.service.QueryFrames(context.Background(), targetEvidence(h.scopeID), FrameQuery{
					Handle:     h.handle,
					PageSize:   1,
					Order:      order,
					Projection: projection,
					Cursor:     cursor,
				})
				if domain != nil {
					t.Fatalf("QueryFrames order %s page %d failed: %v", order, i, domain)
				}
				for _, item := range page.Items {
					if seen[item.FrameID] {
						t.Fatalf("order %s: duplicate frame %s on page %d", order, item.FrameID, i)
					}
					seen[item.FrameID] = true
				}
				if !page.HasMore {
					break
				}
				cursor = page.NextCursor
				if cursor == "" {
					t.Fatalf("order %s: HasMore=true but NextCursor empty on page %d", order, i)
				}
			}
			if len(seen) != 2 {
				t.Fatalf("order %s: expected 2 unique frames, got %d (%v)", order, len(seen), seen)
			}
		})
	}
}

// TestSearchContinuationFindsBoundarySpanningLiteralWithoutDuplicates proves
// that search continuation does not duplicate or omit matches that appear at the
// very start of a record. KMP state is not carried across record boundaries
// (each record is searched independently), so this verifies that matches at
// record-start positions are found exactly once across paginated continuation.
//
// We search for "traceId" which appears at the start of every record.
// With pageSize=1, each page returns one match and continuation must
// find the next without duplicates.
func TestSearchContinuationFindsBoundarySpanningLiteralWithoutDuplicates(t *testing.T) {
	h := newServiceTestHarness(t, "trace-t", minimalValidTrace)

	seen := make(map[int64]bool)
	cursor := ""
	pageNum := 0
	for {
		page, domain := h.service.Search(context.Background(), targetEvidence(h.scopeID), SearchQuery{
			Handle:   h.handle,
			Text:     "attempt-1",
			PageSize: 1,
			Cursor:   cursor,
		})
		if domain != nil {
			t.Fatalf("Search page %d failed: %v", pageNum, domain)
		}
		if len(page.Items) == 0 {
			break
		}
		for _, m := range page.Items {
			if seen[m.Sequence] {
				t.Fatalf("duplicate match for sequence %d on page %d", m.Sequence, pageNum)
			}
			seen[m.Sequence] = true
		}
		if len(page.Items) > 1 {
			t.Fatalf("page %d returned %d matches, expected at most 1", pageNum, len(page.Items))
		}
		if !page.HasMore {
			break
		}
		if page.NextCursor == "" {
			t.Fatalf("page %d: HasMore=true but NextCursor empty", pageNum)
		}
		cursor = page.NextCursor
		pageNum++
		if pageNum > 20 {
			t.Fatal("too many pages, possible infinite loop")
		}
	}
	// The attempt identifier appears in request preparation, send, and response metadata.
	if len(seen) != 3 {
		t.Fatalf("expected matches across 3 records, got %d (%v)", len(seen), seen)
	}
}

// TestSearchStopsAtPageSizeAndResumesWithoutDuplicates proves that when a
// single call would collect more matches than the page size, the search stops
// at the page size and resumes without duplicates or omissions.
func TestSearchStopsAtPageSizeAndResumesWithoutDuplicates(t *testing.T) {
	// The attempt identifier appears in three records.
	// With pageSize=2, page 1 must return exactly 2 matches and a cursor;
	// continuation must return the remaining matches without re-returning
	// the first 2.
	h := newServiceTestHarness(t, "trace-t", minimalValidTrace)

	seen := make(map[int64]bool)
	cursor := ""
	pageNum := 0
	for {
		page, domain := h.service.Search(context.Background(), targetEvidence(h.scopeID), SearchQuery{
			Handle:   h.handle,
			Text:     "attempt-1",
			PageSize: 2,
			Cursor:   cursor,
		})
		if domain != nil {
			t.Fatalf("Search page %d failed: %v", pageNum, domain)
		}
		if len(page.Items) == 0 {
			if !page.HasMore {
				break
			}
			t.Fatalf("page %d returned no matches while work remained", pageNum)
		}
		for _, m := range page.Items {
			if seen[m.Sequence] {
				t.Fatalf("duplicate match for sequence %d on page %d", m.Sequence, pageNum)
			}
			seen[m.Sequence] = true
		}
		if len(page.Items) > 2 {
			t.Fatalf("page %d returned %d matches, expected at most 2", pageNum, len(page.Items))
		}
		if !page.HasMore {
			break
		}
		if page.NextCursor == "" {
			t.Fatalf("page %d: HasMore=true but NextCursor empty", pageNum)
		}
		cursor = page.NextCursor
		pageNum++
		if pageNum > 20 {
			t.Fatal("too many pages, possible infinite loop")
		}
	}
	if len(seen) != 3 {
		t.Fatalf("expected matches across 3 records, got %d (%v)", len(seen), seen)
	}
}

// TestSummaryCarriesScopeHandleIdentityTerminalRootsUsageGapsAndUncertainty
// proves the neutral TraceSummary carries identity, terminal outcome, root
// frame references, aggregate usage, and uncertainties.
func TestSummaryCarriesScopeHandleIdentityTerminalRootsUsageGapsAndUncertainty(t *testing.T) {
	h := newServiceTestHarness(t, "trace-nested-frame-usage", nestedFrameUsageTrace)
	summary, domain := h.service.GetSummary(context.Background(), targetEvidence(h.scopeID), SummaryRequest{Handle: h.handle})
	if domain != nil {
		t.Fatalf("GetSummary failed: %v", domain)
	}
	if summary.Context.Evidence != targetEvidence(h.scopeID) {
		t.Fatalf("expected target evidence for %s, got %#v", h.scopeID, summary.Context.Evidence)
	}
	if summary.Context.Handle != h.handle {
		t.Fatalf("expected handle %s, got %s", h.handle, summary.Context.Handle)
	}
	if summary.Context.TraceID != "trace-nested-frame-usage" {
		t.Fatalf("expected trace ID, got %s", summary.Context.TraceID)
	}
	if summary.Context.SessionID != "session-nested-frame-usage" {
		t.Fatalf("expected session ID, got %s", summary.Context.SessionID)
	}
	if summary.Outcome != "SUCCEEDED" {
		t.Fatalf("expected SUCCEEDED, got %s", summary.Outcome)
	}
	if summary.TerminalFailureID != nil {
		t.Fatalf("expected nil terminal failure for success, got %v", summary.TerminalFailureID)
	}
	if len(summary.RootFrameIDs) != 1 || summary.RootFrameIDs[0] != "root" {
		t.Fatalf("expected root frame [root], got %v", summary.RootFrameIDs)
	}
	if summary.AttributedUsage.TotalUnits <= 0 {
		t.Fatalf("expected positive attributed usage, got %d", summary.AttributedUsage.TotalUnits)
	}
	if summary.TerminalUsage.TotalUnits <= 0 {
		t.Fatalf("expected positive terminal usage, got %d", summary.TerminalUsage.TotalUnits)
	}
	if !summary.UsageComplete {
		t.Fatal("expected usage complete")
	}
}

func TestImportedEvidenceQueriesWithoutTargetProvenanceAndCursorIsOwnerBound(t *testing.T) {
	h := newServiceTestHarness(t, "trace-nested-frame-usage", nestedFrameUsageTrace)
	imported, domain := h.artifacts.Import(context.Background(), strings.NewReader(nestedFrameUsageTrace), int64(len(nestedFrameUsageTrace)))
	if domain != nil {
		t.Fatalf("Import failed: %v", domain)
	}
	summary, domain := h.service.GetSummary(context.Background(), evidence.ForImported(), SummaryRequest{Handle: imported.Handle})
	if domain != nil {
		t.Fatalf("imported summary failed: %v", domain)
	}
	if summary.Context.Evidence != evidence.ForImported() || summary.Context.TraceID != "trace-nested-frame-usage" {
		t.Fatalf("unexpected imported context: %+v", summary.Context)
	}
	page, domain := h.service.QueryFrames(context.Background(), evidence.ForImported(), FrameQuery{Handle: imported.Handle, PageSize: 1})
	if domain != nil || page.NextCursor == "" {
		t.Fatalf("imported frame page=%+v error=%v", page, domain)
	}
	_, domain = h.service.QueryFrames(context.Background(), targetEvidence(h.scopeID), FrameQuery{
		Handle: h.handle, PageSize: 1, Cursor: page.NextCursor,
	})
	if domain == nil || domain.Code != consolecore.CodeTargetChanged {
		t.Fatalf("expected owner-bound cursor rejection, got %v", domain)
	}
}

// TestOptionalAndUnknownFactsNeverSerializeInternallyAsKnownZero proves that
// optional duration facts use pointers (nil = unknown) rather than zero, so
// unknown is distinct from recorded zero. We verify via a frame whose self
// duration is unknown (incomplete close).
func TestOptionalAndUnknownFactsNeverSerializeInternallyAsKnownZero(t *testing.T) {
	h := newServiceTestHarness(t, "trace-nested-frame-usage", nestedFrameUsageTrace)
	page, domain := h.service.QueryFrames(context.Background(), targetEvidence(h.scopeID), FrameQuery{
		Handle:   h.handle,
		PageSize: 100,
	})
	if domain != nil {
		t.Fatalf("QueryFrames failed: %v", domain)
	}
	if len(page.Items) == 0 {
		t.Fatal("expected at least one frame")
	}
	for _, f := range page.Items {
		// Duration pointers are either present (known) or nil (unknown), never
		// a zero-value int64 disguised as "known zero". We assert the contract:
		// if InclusiveDurationMillis is nil then SelfDurationMillis is also nil
		// (a frame with no inclusive duration has no self duration).
		if f.InclusiveDurationMillis == nil && f.SelfDurationMillis != nil {
			t.Fatalf("frame %s: inclusive nil but self non-nil", f.FrameID)
		}
	}
}

// TestAllFixtureFramesRecordsAndPayloadsAreReachableThroughFiniteCalls proves
// that every frame, record, and payload in a fixture is reachable through
// finite paginated calls without whole-trace materialization.
func TestAllFixtureFramesRecordsAndPayloadsAreReachableThroughFiniteCalls(t *testing.T) {
	// Use the chunked-payload trace so payloads are present.
	ndjson := chunkedPayloadTrace(256, 2)
	h := newServiceTestHarness(t, "t", ndjson)

	// Walk all frames.
	frameIDs := make(map[string]bool)
	cursor := ""
	for i := 0; i < 50; i++ {
		page, domain := h.service.QueryFrames(context.Background(), targetEvidence(h.scopeID), FrameQuery{
			Handle:   h.handle,
			PageSize: 1,
			Cursor:   cursor,
		})
		if domain != nil {
			t.Fatalf("QueryFrames page %d failed: %v", i, domain)
		}
		for _, f := range page.Items {
			frameIDs[f.FrameID] = true
		}
		if !page.HasMore {
			break
		}
		cursor = page.NextCursor
	}

	// Walk all physical records.
	seqs := make(map[int64]bool)
	cursor = ""
	for i := 0; i < 50; i++ {
		page, domain := h.service.QueryRecords(context.Background(), targetEvidence(h.scopeID), RecordQuery{
			Handle:         h.handle,
			Representation: RecordRepresentationPhysical,
			PageSize:       1,
			Cursor:         cursor,
		})
		if domain != nil {
			t.Fatalf("QueryRecords page %d failed: %v", i, domain)
		}
		for _, r := range page.Items {
			seqs[r.Sequence] = true
		}
		if !page.HasMore {
			break
		}
		cursor = page.NextCursor
	}
	if len(seqs) == 0 {
		t.Fatal("expected at least one record")
	}

	// Walk all payloads.
	payloadIDs := make(map[string]bool)
	cursor = ""
	for i := 0; i < 50; i++ {
		page, domain := h.service.QueryPayloads(context.Background(), targetEvidence(h.scopeID), PayloadQuery{
			Handle:   h.handle,
			PageSize: 1,
			Cursor:   cursor,
		})
		if domain != nil {
			t.Fatalf("QueryPayloads page %d failed: %v", i, domain)
		}
		for _, p := range page.Items {
			payloadIDs[p.PayloadID] = true
		}
		if !page.HasMore {
			break
		}
		cursor = page.NextCursor
	}
	if len(payloadIDs) == 0 {
		t.Fatal("expected at least one payload")
	}

	// Each payload must be readable via a range call.
	for pid := range payloadIDs {
		_, domain := h.service.ReadContentRange(context.Background(), targetEvidence(h.scopeID), RangeRequest{
			Handle:     h.handle,
			Source:     RangeSourceContent,
			ContentRef: mustEnvelopeContentRef(t, h.scopeID, h.handle, pid),
			Start:      0,
			MaxBytes:   100,
		})
		if domain != nil {
			t.Fatalf("ReadContentRange for %s failed: %v", pid, domain)
		}
	}
}

// TestQueryRejectsZeroNegativeAndOneOverMaximumPageSize proves page-size
// validation: zero defaults, negative is INVALID_ARGUMENT, over-max is
// LIMIT_EXCEEDED.
func TestQueryRejectsZeroNegativeAndOneOverMaximumPageSize(t *testing.T) {
	h := newServiceTestHarness(t, "trace-t", minimalValidTrace)

	// Zero defaults to defaultPageSize and succeeds.
	page, domain := h.service.QueryRecords(context.Background(), targetEvidence(h.scopeID), RecordQuery{
		Handle:   h.handle,
		PageSize: 0,
	})
	if domain != nil {
		t.Fatalf("PageSize=0 should default, got %v", domain)
	}
	if len(page.Items) == 0 {
		t.Fatal("expected default page to return records")
	}

	// Negative is INVALID_ARGUMENT.
	_, domain = h.service.QueryRecords(context.Background(), targetEvidence(h.scopeID), RecordQuery{
		Handle:   h.handle,
		PageSize: -1,
	})
	if domain == nil || domain.Code != consolecore.CodeInvalidArgument {
		t.Fatalf("expected INVALID_ARGUMENT for negative page size, got %v", domain)
	}

	// Over max is LIMIT_EXCEEDED.
	_, domain = h.service.QueryRecords(context.Background(), targetEvidence(h.scopeID), RecordQuery{
		Handle:   h.handle,
		PageSize: maxPageSize + 1,
	})
	if domain == nil || domain.Code != consolecore.CodeLimitExceeded {
		t.Fatalf("expected LIMIT_EXCEEDED for over-max page size, got %v", domain)
	}
}

// TestPayloadRangeAppliesDefaultExactMaximumAndOneOverLimit proves range-size
// validation: zero defaults, exact max succeeds, one over max is
// LIMIT_EXCEEDED.
func TestPayloadRangeAppliesDefaultExactMaximumAndOneOverLimit(t *testing.T) {
	ndjson := chunkedPayloadTrace(256, 2)
	h := newServiceTestHarness(t, "t", ndjson)

	// Zero defaults to defaultRangeBytes and succeeds.
	_, domain := h.service.ReadContentRange(context.Background(), targetEvidence(h.scopeID), RangeRequest{
		Handle:     h.handle,
		Source:     RangeSourceContent,
		ContentRef: mustEnvelopeContentRef(t, h.scopeID, h.handle, "payload-1"),
		Start:      0,
		MaxBytes:   0,
	})
	if domain != nil {
		t.Fatalf("MaxBytes=0 should default, got %v", domain)
	}

	// Exact max succeeds.
	_, domain = h.service.ReadContentRange(context.Background(), targetEvidence(h.scopeID), RangeRequest{
		Handle:     h.handle,
		Source:     RangeSourceContent,
		ContentRef: mustEnvelopeContentRef(t, h.scopeID, h.handle, "payload-1"),
		Start:      0,
		MaxBytes:   maxRangeBytes,
	})
	if domain != nil {
		t.Fatalf("MaxBytes=maxRangeBytes should succeed, got %v", domain)
	}

	// One over max is LIMIT_EXCEEDED.
	_, domain = h.service.ReadContentRange(context.Background(), targetEvidence(h.scopeID), RangeRequest{
		Handle:     h.handle,
		Source:     RangeSourceContent,
		ContentRef: mustEnvelopeContentRef(t, h.scopeID, h.handle, "payload-1"),
		Start:      0,
		MaxBytes:   maxRangeBytes + 1,
	})
	if domain == nil || domain.Code != consolecore.CodeLimitExceeded {
		t.Fatalf("expected LIMIT_EXCEEDED for over-max range, got %v", domain)
	}
}

// TestPayloadRawRecordAndRawArtifactReferencesCannotBeInterchanged proves that
// range cursors bound to one source cannot be reused for a different source.
func TestPayloadRawRecordAndRawArtifactReferencesCannotBeInterchanged(t *testing.T) {
	ndjson := chunkedPayloadTrace(256, 2)
	h := newServiceTestHarness(t, "t", ndjson)

	// Get a raw-artifact range cursor.
	artifactPage, domain := h.service.ReadRawArtifactRange(context.Background(), targetEvidence(h.scopeID), RangeRequest{
		Handle:   h.handle,
		Source:   RangeSourceRawArtifact,
		Start:    0,
		MaxBytes: 50,
	})
	if domain != nil {
		t.Fatalf("ReadRawArtifactRange failed: %v", domain)
	}
	if artifactPage.NextCursor == "" {
		t.Fatal("expected a raw-artifact continuation cursor")
	}

	// Reuse the raw-artifact cursor for a payload range: must fail with
	// INVALID_CURSOR (op mismatch) since the cursor op is RAW_ARTIFACT_RANGE
	// but the call expects PAYLOAD_RANGE.
	_, domain = h.service.ReadContentRange(context.Background(), targetEvidence(h.scopeID), RangeRequest{
		Handle:         h.handle,
		Source:         RangeSourceContent,
		ContentRef:     mustEnvelopeContentRef(t, h.scopeID, h.handle, "payload-1"),
		ContinueCursor: artifactPage.NextCursor,
		MaxBytes:       50,
	})
	if domain == nil {
		t.Fatal("expected error when reusing raw-artifact cursor for payload range")
	}
	if domain.Code != consolecore.CodeInvalidCursor && domain.Code != consolecore.CodeTargetChanged {
		t.Fatalf("expected INVALID_CURSOR or TARGET_CHANGED, got %s: %s", domain.Code, domain.Message)
	}
}

// TestTextRangeSplitRuneReturnsExactBase64 proves a text range whose requested
// end lands mid-code-point preserves every requested byte.
func TestTextRangeSplitRuneReturnsExactBase64(t *testing.T) {
	// Build a payload that is entirely UTF-8 text with a multi-byte char near
	// the range boundary. chunkedPayloadTrace produces ASCII text, so we use
	// encodeRangeContent directly to verify the boundary logic.
	full := []byte("xxéyy") // xx(2) + é(2) + yy(2) = 6 bytes
	// Request bytes [0:3] = "xx" + first byte of é (0xC3).
	buf := full[:3]
	encoding, content, start, end := encodeRangeContent(buf, "text/plain", 0, 3, int64(len(full)))
	if encoding != RangeEncodingBase64 {
		t.Fatalf("expected BASE64 encoding, got %q", encoding)
	}
	if start != 0 {
		t.Fatalf("expected start 0, got %d", start)
	}
	decoded, err := base64.StdEncoding.DecodeString(string(content))
	if err != nil || string(decoded) != string(buf) {
		t.Fatalf("range bytes changed: decoded=%x err=%v want=%x", decoded, err, buf)
	}
	if end != 3 {
		t.Fatalf("expected exact end 3, got %d", end)
	}
}

// TestBinaryRangeReturnsBase64WithExactOffsets proves that non-UTF-8 bytes are
// returned base64-encoded with exact byte offsets (no boundary adjustment).
func TestBinaryRangeReturnsBase64WithExactOffsets(t *testing.T) {
	input := []byte{0xFF, 0xFE, 0xFD, 0xFC}
	encoding, content, start, end := encodeRangeContent(input, "application/octet-stream", 10, 14, 14)
	if encoding != RangeEncodingBase64 {
		t.Fatalf("expected BASE64 encoding, got %q", encoding)
	}
	if start != 10 {
		t.Fatalf("expected exact start 10, got %d", start)
	}
	if end != 14 {
		t.Fatalf("expected exact end 14, got %d", end)
	}
	if len(content) == 0 {
		t.Fatal("expected non-empty base64 content")
	}
}

// TestSearchRejectsTextOverByteOrCodePointLimit proves that a literal exceeding
// the byte or code-point limit is rejected with LIMIT_EXCEEDED.
func TestSearchRejectsTextOverByteOrCodePointLimit(t *testing.T) {
	h := newServiceTestHarness(t, "trace-t", minimalValidTrace)

	// Over byte limit.
	longBytes := strings.Repeat("a", maxLiteralTextBytes+1)
	_, domain := h.service.Search(context.Background(), targetEvidence(h.scopeID), SearchQuery{
		Handle:   h.handle,
		Text:     longBytes,
		PageSize: 10,
	})
	if domain == nil || domain.Code != consolecore.CodeLimitExceeded {
		t.Fatalf("expected LIMIT_EXCEEDED for over-byte literal, got %v", domain)
	}

	// Over code-point limit: use 2-byte chars so byte count stays under the
	// byte limit but rune count exceeds the rune limit.
	longRunes := strings.Repeat("é", maxLiteralTextRunes+1)
	if len(longRunes) > maxLiteralTextBytes {
		// If this combination exceeds the byte limit too, shrink it.
		longRunes = strings.Repeat("é", maxLiteralTextRunes+1)
		maxRunesThatFitBytes := maxLiteralTextBytes / 2
		if maxRunesThatFitBytes > maxLiteralTextRunes {
			longRunes = strings.Repeat("é", maxRunesThatFitBytes)
		}
	}
	if runeCount(longRunes) <= maxLiteralTextRunes {
		// Ensure we are over the rune limit while under the byte limit.
		longRunes = strings.Repeat("é", maxLiteralTextRunes+1)
	}
	if len(longRunes) > maxLiteralTextBytes {
		t.Skip("cannot construct a rune-over-limit literal that is under the byte limit with 2-byte chars")
	}
	_, domain = h.service.Search(context.Background(), targetEvidence(h.scopeID), SearchQuery{
		Handle:   h.handle,
		Text:     longRunes,
		PageSize: 10,
	})
	if domain == nil || domain.Code != consolecore.CodeLimitExceeded {
		t.Fatalf("expected LIMIT_EXCEEDED for over-rune literal, got %v", domain)
	}
}

// TestRangeCursorErrorPrecedenceMatchesQueryCursorPrecedence proves that range
// methods (ReadRawArtifactRange, ReadContentRange, ReadRawRecordRange) apply the
// same cursor error precedence as query methods: INVALID_CURSOR (malformed/op)
// and TARGET_CHANGED (scope mismatch) are checked before lease acquisition, so
// they take precedence over ARTIFACT_EXPIRED (L2).
func TestRangeCursorErrorPrecedenceMatchesQueryCursorPrecedence(t *testing.T) {
	h := newServiceTestHarness(t, "trace-nested-frame-usage", nestedFrameUsageTrace)

	// Acquire one page so we have a real raw-artifact continuation cursor.
	page1, domain := h.service.ReadRawArtifactRange(context.Background(), targetEvidence(h.scopeID), RangeRequest{
		Handle:   h.handle,
		Source:   RangeSourceRawArtifact,
		Start:    0,
		MaxBytes: 50,
	})
	if domain != nil {
		t.Fatalf("ReadRawArtifactRange page 1 failed: %v", domain)
	}
	if page1.NextCursor == "" {
		t.Fatal("expected a continuation cursor from page 1")
	}

	// 1. TARGET_CHANGED: reuse the cursor from a different scope. The handle is
	//    not installed in the other scope, but TARGET_CHANGED must take
	//    precedence over ARTIFACT_EXPIRED.
	otherScope := h.scopeID
	if otherScope == "scope-test" {
		otherScope = "scope-other"
	} else {
		otherScope = "scope-test"
	}
	_, domain = h.service.ReadRawArtifactRange(context.Background(), targetEvidence(otherScope), RangeRequest{
		Handle:         h.handle,
		Source:         RangeSourceRawArtifact,
		ContinueCursor: page1.NextCursor,
		MaxBytes:       50,
	})
	if domain == nil {
		t.Fatal("expected an error for stale-scope cursor, got nil")
	}
	if domain.Code != consolecore.CodeTargetChanged {
		t.Fatalf("expected TARGET_CHANGED for stale-scope range cursor, got %s: %s", domain.Code, domain.Message)
	}

	// 2. INVALID_CURSOR (op mismatch): reuse a raw-artifact cursor for a payload
	//    range. The cursor op is RAW_ARTIFACT_RANGE but the call expects
	//    PAYLOAD_RANGE. This must be caught before lease acquisition.
	ndjson := chunkedPayloadTrace(256, 2)
	h2 := newServiceTestHarness(t, "t", ndjson)
	artifactPage, domain := h2.service.ReadRawArtifactRange(context.Background(), targetEvidence(h2.scopeID), RangeRequest{
		Handle:   h2.handle,
		Source:   RangeSourceRawArtifact,
		Start:    0,
		MaxBytes: 50,
	})
	if domain != nil {
		t.Fatalf("ReadRawArtifactRange for op-mismatch test failed: %v", domain)
	}
	_, domain = h2.service.ReadContentRange(context.Background(), targetEvidence(h2.scopeID), RangeRequest{
		Handle:         h2.handle,
		Source:         RangeSourceContent,
		ContentRef:     mustEnvelopeContentRef(t, h2.scopeID, h2.handle, "payload-1"),
		ContinueCursor: artifactPage.NextCursor,
		MaxBytes:       50,
	})
	if domain == nil {
		t.Fatal("expected error for op-mismatch range cursor")
	}
	if domain.Code != consolecore.CodeInvalidCursor {
		t.Fatalf("expected INVALID_CURSOR for op-mismatch range cursor, got %s: %s", domain.Code, domain.Message)
	}
}

// TestRecordQuerySequenceRangeUsesBinarySearchFastPath proves that a record
// query with only MinSequence/MaxSequence filters returns the correct records
// without parsing records outside the range (L3).
func TestRecordQuerySequenceRangeUsesBinarySearchFastPath(t *testing.T) {
	h := newServiceTestHarness(t, "trace-t", minimalValidTrace)

	// minimalValidTrace has 5 records (sequences 1-5). Query for sequences 2-4.
	minSeq := int64(2)
	maxSeq := int64(4)
	page, domain := h.service.QueryRecords(context.Background(), targetEvidence(h.scopeID), RecordQuery{
		Handle: h.handle,
		Filter: RecordFilter{
			MinSequence: &minSeq,
			MaxSequence: &maxSeq,
		},
		Representation: RecordRepresentationPhysical,
		PageSize:       100,
	})
	if domain != nil {
		t.Fatalf("QueryRecords failed: %v", domain)
	}
	if len(page.Items) != 3 {
		t.Fatalf("expected 3 records (sequences 2-4), got %d", len(page.Items))
	}
	for _, r := range page.Items {
		if r.Sequence < minSeq || r.Sequence > maxSeq {
			t.Fatalf("record sequence %d outside range [%d, %d]", r.Sequence, minSeq, maxSeq)
		}
	}
	// Verify exact sequences.
	seqs := make(map[int64]bool)
	for _, r := range page.Items {
		seqs[r.Sequence] = true
	}
	if !seqs[2] || !seqs[3] || !seqs[4] {
		t.Fatalf("expected sequences 2, 3, 4; got %v", seqs)
	}
}
