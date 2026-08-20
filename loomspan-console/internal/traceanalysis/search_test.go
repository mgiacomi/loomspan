package traceanalysis

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
)

func TestSearchContentDescriptorsArePageLocalAndDeduplicated(t *testing.T) {
	items, descriptors := normalizeSearchContent([]SearchResult{
		{Sequence: 1, MatchOffset: 1, contentRef: "opaque-one"},
		{Sequence: 1, MatchOffset: 9, contentRef: "opaque-one"},
		{Sequence: 2, MatchOffset: 2, contentRef: "opaque-two"},
		{Sequence: 3, MatchOffset: 3},
	})
	if len(descriptors) != 2 || descriptors[0].ContentID != "c1" || descriptors[0].ContentRef != "opaque-one" || descriptors[1].ContentID != "c2" || descriptors[1].ContentRef != "opaque-two" {
		t.Fatalf("descriptors=%+v", descriptors)
	}
	if items[0].ContentID != "c1" || items[1].ContentID != "c1" || items[2].ContentID != "c2" || items[3].ContentID != "" {
		t.Fatalf("items=%+v", items)
	}
	encoded, err := json.Marshal(SearchPage{Items: items, ContentDescriptors: descriptors})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(encoded), "opaque-one") != 1 || strings.Count(string(encoded), "opaque-two") != 1 {
		t.Fatalf("opaque references repeated: %s", encoded)
	}
}

func TestServiceSearchFindsLiteralAcrossReconstructedPayloadChunks(t *testing.T) {
	trace := strings.Join([]string{
		startedRecord(1),
		requestRecord(2, "retry-1", "attempt-1", 1, true),
		chunkEnvelopeRecord(3, "payload-1", "text/plain", 2),
		chunkRecord(4, "payload-1", "text/plain", 0, 2, "hello-"),
		chunkRecord(5, "payload-1", "text/plain", 1, 2, "world"),
		responseRecord(6, "", "retry-1", "attempt-1", 1, 2, 1, 3, "EXACT"),
		completionRecord(7, "SUCCEEDED", 2, 1, 3, ""),
	}, "\n") + "\n"
	h := newServiceTestHarness(t, "t", trace)
	page, domain := h.service.Search(context.Background(), targetEvidence(h.scopeID), SearchQuery{
		Handle: h.handle, Text: "o-wo", PageSize: 10,
	})
	if domain != nil {
		t.Fatalf("Search failed: %v", domain)
	}
	if len(page.Items) != 1 {
		t.Fatalf("expected one reconstructed-payload match, got %d", len(page.Items))
	}
	match := page.Items[0]
	if match.SearchedField != "content" || match.Sequence != 3 || match.MatchOffset != 4 || match.ContentID != "c1" || len(page.ContentDescriptors) != 1 || page.ContentDescriptors[0].ContentRef == "" {
		t.Fatalf("unexpected payload match: %+v", match)
	}
}

func TestByteAdmissionResumesRejectedPayloadMatchWithPageLocalDescriptor(t *testing.T) {
	trace := strings.Join([]string{
		startedRecord(1),
		requestRecord(2, "retry-1", "attempt-1", 1, true),
		chunkEnvelopeRecord(3, "payload-1", "text/plain", 2),
		chunkRecord(4, "payload-1", "text/plain", 0, 2, "hello-o-"),
		chunkRecord(5, "payload-1", "text/plain", 1, 2, "world-o-world"),
		responseRecord(6, "", "retry-1", "attempt-1", 1, 2, 1, 3, "EXACT"),
		completionRecord(7, "SUCCEEDED", 2, 1, 3, ""),
	}, "\n") + "\n"
	h := newServiceTestHarness(t, "t", trace)
	continuation := ""
	var offsets []int64
	var descriptorRef string
	for {
		admitted := 0
		page, domain := h.service.Search(context.Background(), targetEvidence(h.scopeID), SearchQuery{Handle: h.handle, Text: "o-wo", PageSize: 64, Cursor: continuation, Admit: func(SearchResult, string) bool { admitted++; return admitted <= 1 }})
		if domain != nil {
			t.Fatal(domain)
		}
		if len(page.Items) != 1 || len(page.ContentDescriptors) != 1 || page.Items[0].ContentID != "c1" {
			t.Fatalf("page-local search state=%+v", page)
		}
		offsets = append(offsets, page.Items[0].MatchOffset)
		if descriptorRef == "" {
			descriptorRef = page.ContentDescriptors[0].ContentRef
		} else if descriptorRef != page.ContentDescriptors[0].ContentRef {
			t.Fatalf("descriptor reference changed: %q != %q", descriptorRef, page.ContentDescriptors[0].ContentRef)
		}
		if !page.HasMore {
			break
		}
		continuation = page.NextCursor
	}
	if len(offsets) != 2 || offsets[0] == offsets[1] {
		t.Fatalf("payload match traversal=%v", offsets)
	}
}

func TestServiceSearchAppliesRecordFiltersToOrdinaryAndEnvelopeContent(t *testing.T) {
	trace := strings.Join([]string{
		startedRecord(1),
		requestRecord(2, "retry-1", "attempt-1", 1, true),
		chunkEnvelopeRecordFor(3, "retry-1", "attempt-1", 1, "payload-1", "text/plain", 1),
		chunkRecord(4, "payload-1", "text/plain", 0, 1, "shared-hit"),
		strings.Replace(responseRecord(5, "", "retry-1", "attempt-1", 1, 1, 0, 1, "EXACT"), `"content":"r"`, `"content":"shared-hit"`, 1),
		requestRecord(6, "retry-2", "attempt-2", 1, true),
		chunkEnvelopeRecordFor(7, "retry-2", "attempt-2", 1, "payload-2", "text/plain", 1),
		chunkRecord(8, "payload-2", "text/plain", 0, 1, "shared-hit"),
		strings.Replace(responseRecord(9, "", "retry-2", "attempt-2", 1, 1, 0, 1, "EXACT"), `"content":"r"`, `"content":"shared-hit"`, 1),
		completionRecord(10, "SUCCEEDED", 2, 0, 2, ""),
	}, "\n") + "\n"
	h := newServiceTestHarness(t, "t", trace)
	query := SearchQuery{Handle: h.handle, Text: "shared-hit", Filter: RecordFilter{AttemptID: "attempt-2"}, PageSize: 1}
	sequences := []int64{}
	for {
		page, domain := h.service.Search(context.Background(), targetEvidence(h.scopeID), query)
		if domain != nil {
			t.Fatalf("Search failed: %v", domain)
		}
		for _, match := range page.Items {
			sequences = append(sequences, match.Sequence)
			if match.Sequence != 7 && match.Sequence != 9 {
				t.Fatalf("filter admitted sequence %d: %+v", match.Sequence, match)
			}
		}
		if !page.HasMore {
			break
		}
		query.Cursor = page.NextCursor
	}
	if len(sequences) != 2 {
		t.Fatalf("filtered matches=%v", sequences)
	}

	query.Filter.AttemptID = "attempt-1"
	if _, domain := h.service.Search(context.Background(), targetEvidence(h.scopeID), query); domain == nil || domain.Code != consolecore.CodeInvalidCursor {
		t.Fatalf("changed filter reused continuation: %v", domain)
	}
}

func TestServiceSearchExcludesBinarySemanticContentWithLimitation(t *testing.T) {
	trace := strings.Join([]string{
		startedRecord(1),
		requestRecord(2, "retry-1", "attempt-1", 1, true),
		chunkEnvelopeRecord(3, "payload-1", "application/octet-stream", 1),
		chunkRecord(4, "payload-1", "application/octet-stream", 0, 1, "binary-secret"),
		responseRecord(5, "", "retry-1", "attempt-1", 1, 1, 0, 1, "EXACT"),
		completionRecord(6, "SUCCEEDED", 1, 0, 1, ""),
	}, "\n") + "\n"
	h := newServiceTestHarness(t, "t", trace)
	page, domain := h.service.Search(context.Background(), targetEvidence(h.scopeID), SearchQuery{
		Handle: h.handle, Text: "binary-secret", PageSize: 10,
	})
	if domain != nil {
		t.Fatalf("Search failed: %v", domain)
	}
	if len(page.Items) != 0 {
		t.Fatalf("binary content must not produce literal matches: %+v", page.Items)
	}
	if len(page.SearchLimitations) != 1 || page.SearchLimitations[0].Code != "BINARY_CONTENT_EXCLUDED" {
		t.Fatalf("expected explicit binary exclusion limitation: %+v", page.SearchLimitations)
	}
}

func TestServiceSearchFindsLiteral(t *testing.T) {
	h := newServiceTestHarness(t, "trace-t", minimalValidTrace)
	// Search for "fixture" which appears in the trace.
	page, domain := h.service.Search(context.Background(), targetEvidence(h.scopeID), SearchQuery{
		Handle:   h.handle,
		Text:     "fixture",
		PageSize: 10,
	})
	if domain != nil {
		t.Fatalf("Search failed: %v", domain)
	}
	if len(page.Items) == 0 {
		t.Fatal("expected at least one match for 'fixture'")
	}
	for _, m := range page.Items {
		if m.MatchLength != len("fixture") {
			t.Fatalf("expected match length %d, got %d", len("fixture"), m.MatchLength)
		}
	}
}

func TestServiceSearchNoMatch(t *testing.T) {
	h := newServiceTestHarness(t, "trace-t", minimalValidTrace)
	page, domain := h.service.Search(context.Background(), targetEvidence(h.scopeID), SearchQuery{
		Handle:   h.handle,
		Text:     "nonexistent-text-that-does-not-appear",
		PageSize: 10,
	})
	if domain != nil {
		t.Fatalf("Search failed: %v", domain)
	}
	if len(page.Items) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(page.Items))
	}
	if page.HasMore {
		t.Fatal("expected no more pages")
	}
}

func TestServiceSearchEmptyUnfinishedPageCannotBeMistakenForCompleteNegative(t *testing.T) {
	trace := chunkedPayloadTrace(maxSearchWorkBytes+1024, 16)
	h := newServiceTestHarness(t, "t", trace)
	query := SearchQuery{Handle: h.handle, Text: "not-present", PageSize: 10}
	first, domain := h.service.Search(context.Background(), targetEvidence(h.scopeID), query)
	if domain != nil || len(first.Items) != 0 || !first.HasMore || first.NextCursor == "" {
		t.Fatalf("unfinished page=%+v domain=%v", first, domain)
	}
	query.Cursor = first.NextCursor
	final, domain := h.service.Search(context.Background(), targetEvidence(h.scopeID), query)
	if domain != nil || len(final.Items) != 0 || final.HasMore || final.NextCursor != "" {
		t.Fatalf("final page=%+v domain=%v", final, domain)
	}
	query.Text = "changed"
	if _, domain = h.service.Search(context.Background(), targetEvidence(h.scopeID), query); domain == nil || domain.Code != consolecore.CodeInvalidCursor {
		t.Fatalf("changed-query cursor domain=%v", domain)
	}
}

func TestServiceSearchEmptyText(t *testing.T) {
	h := newServiceTestHarness(t, "trace-t", minimalValidTrace)
	_, domain := h.service.Search(context.Background(), targetEvidence(h.scopeID), SearchQuery{
		Handle:   h.handle,
		Text:     "",
		PageSize: 10,
	})
	if domain == nil {
		t.Fatal("expected error for empty search text")
	}
}

func TestServiceSearchExceedsLiteralLimit(t *testing.T) {
	h := newServiceTestHarness(t, "trace-t", minimalValidTrace)
	// Search with a literal exceeding the byte limit.
	longText := make([]byte, maxLiteralTextBytes+1)
	for i := range longText {
		longText[i] = 'a'
	}
	_, domain := h.service.Search(context.Background(), targetEvidence(h.scopeID), SearchQuery{
		Handle:   h.handle,
		Text:     string(longText),
		PageSize: 10,
	})
	if domain == nil {
		t.Fatal("expected error for oversized literal")
	}
}

func TestServiceSearchPagination(t *testing.T) {
	h := newServiceTestHarness(t, "trace-t", minimalValidTrace)
	// Search metadata shared by the request/response records.
	page1, domain := h.service.Search(context.Background(), targetEvidence(h.scopeID), SearchQuery{
		Handle:   h.handle,
		Text:     "attempt-1",
		PageSize: 2,
	})
	if domain != nil {
		t.Fatalf("Search page 1 failed: %v", domain)
	}
	if len(page1.Items) != 2 {
		t.Fatalf("expected 2 matches on page 1, got %d", len(page1.Items))
	}
	if !page1.HasMore {
		t.Fatal("expected hasMore on page 1")
	}
	// Continue.
	page2, domain := h.service.Search(context.Background(), targetEvidence(h.scopeID), SearchQuery{
		Handle:   h.handle,
		Text:     "attempt-1",
		PageSize: 2,
		Cursor:   page1.NextCursor,
	})
	if domain != nil {
		t.Fatalf("Search page 2 failed: %v", domain)
	}
	if len(page2.Items) == 0 {
		t.Fatal("expected at least 1 match on page 2")
	}
}

func TestServiceSearchExpiredHandle(t *testing.T) {
	h := newServiceTestHarness(t, "trace-t", minimalValidTrace)
	_, domain := h.service.Search(context.Background(), targetEvidence(h.scopeID), SearchQuery{
		Handle:   artifact.Handle("nonexistent"),
		Text:     "test",
		PageSize: 10,
	})
	if domain == nil {
		t.Fatal("expected error for nonexistent handle")
	}
}

func TestServiceSearchRejectsOutOfRangeKMPContinuation(t *testing.T) {
	h := newServiceTestHarness(t, "trace-t", minimalValidTrace)
	query := SearchQuery{Handle: h.handle, Text: "attempt-1", PageSize: 1}
	page, domain := h.service.Search(context.Background(), targetEvidence(h.scopeID), query)
	if domain != nil || page.NextCursor == "" {
		t.Fatalf("create search continuation: page=%+v domain=%v", page, domain)
	}
	decoded, err := decodeCursor(page.NextCursor)
	if err != nil {
		t.Fatalf("decode search continuation: %v", err)
	}
	decoded.SearchState.KMPPartial = len(query.Text)
	query.Cursor, err = encodeCursor(decoded)
	if err != nil {
		t.Fatalf("encode malformed search continuation: %v", err)
	}
	_, domain = h.service.Search(context.Background(), targetEvidence(h.scopeID), query)
	if domain == nil || domain.Code != consolecore.CodeInvalidCursor {
		t.Fatalf("expected INVALID_CURSOR for out-of-range KMP progress, got %v", domain)
	}
}

func TestServicePayloadSearchContinuationCarriesDirectIndexOffset(t *testing.T) {
	trace := strings.Join([]string{
		startedRecord(1),
		requestRecord(2, "retry-1", "attempt-1", 1, true),
		chunkEnvelopeRecordFor(3, "retry-1", "attempt-1", 1, "payload-1", "text/plain", 1),
		chunkRecord(4, "payload-1", "text/plain", 0, 1, "payload-hit-a"),
		responseRecord(5, "", "retry-1", "attempt-1", 1, 1, 0, 1, "EXACT"),
		requestRecord(6, "retry-2", "attempt-2", 1, true),
		chunkEnvelopeRecordFor(7, "retry-2", "attempt-2", 1, "payload-2", "text/plain", 1),
		chunkRecord(8, "payload-2", "text/plain", 0, 1, "payload-hit-b"),
		responseRecord(9, "", "retry-2", "attempt-2", 1, 1, 0, 1, "EXACT"),
		completionRecord(10, "SUCCEEDED", 2, 0, 2, ""),
	}, "\n") + "\n"
	h := newServiceTestHarness(t, "t", trace)
	query := SearchQuery{Handle: h.handle, Text: "payload-hit", PageSize: 1}
	fingerprint, err := canonicalizeRequest(searchQueryCanonical{Text: query.Text, Filter: query.Filter, PageSize: query.PageSize})
	if err != nil {
		t.Fatalf("canonicalize search: %v", err)
	}
	query.Cursor, err = encodeSearchCursor(targetCursorKey(h.scopeID), h.handle, fingerprint, searchCursorState{Phase: "payloads"})
	if err != nil {
		t.Fatalf("encode payload search cursor: %v", err)
	}

	first, domain := h.service.Search(context.Background(), targetEvidence(h.scopeID), query)
	if domain != nil || len(first.Items) != 1 || first.NextCursor == "" {
		t.Fatalf("first payload match: page=%+v domain=%v", first, domain)
	}
	query.Cursor = first.NextCursor
	second, domain := h.service.Search(context.Background(), targetEvidence(h.scopeID), query)
	if domain != nil || len(second.Items) != 1 || second.NextCursor == "" {
		t.Fatalf("second payload match: page=%+v domain=%v", second, domain)
	}
	decoded, err := decodeCursor(second.NextCursor)
	if err != nil {
		t.Fatalf("decode second payload continuation: %v", err)
	}
	if decoded.SearchState.PayloadPosition != 1 || decoded.SearchState.PayloadIndexOffset <= 0 {
		t.Fatalf("payload continuation did not retain a direct index offset: %+v", decoded.SearchState)
	}
}

func TestKMPSearchBasic(t *testing.T) {
	needle := []byte("abc")
	failure := buildKMPFailureTable(needle)
	matches, partial := kmpSearch([]byte("xabcyabcz"), needle, failure, 0)
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
	if matches[0] != 1 || matches[1] != 5 {
		t.Fatalf("expected matches at 1 and 5, got %v", matches)
	}
	if partial != 0 {
		t.Fatalf("expected partial 0, got %d", partial)
	}
}

func TestKMPSearchBoundarySpanning(t *testing.T) {
	needle := []byte("abcdef")
	failure := buildKMPFailureTable(needle)
	// First chunk ends with "abc" — partial match of 3.
	matches1, partial := kmpSearch([]byte("xxabc"), needle, failure, 0)
	if len(matches1) != 0 {
		t.Fatalf("expected 0 matches in first chunk, got %d", len(matches1))
	}
	if partial != 3 {
		t.Fatalf("expected partial 3, got %d", partial)
	}
	// Second chunk starts with "def" — should complete the match.
	// The match started 3 bytes before this chunk (in the previous chunk),
	// so the offset is -3 relative to the current chunk.
	matches2, partial2 := kmpSearch([]byte("defxx"), needle, failure, partial)
	if len(matches2) != 1 {
		t.Fatalf("expected 1 match in second chunk, got %d", len(matches2))
	}
	if matches2[0] != -3 {
		t.Fatalf("expected match at offset -3 (spanning boundary), got %d", matches2[0])
	}
	if partial2 != 0 {
		t.Fatalf("expected partial 0 after complete match, got %d", partial2)
	}
}

func TestKMPFailureTable(t *testing.T) {
	needle := []byte("abab")
	failure := buildKMPFailureTable(needle)
	// Expected failure table for "abab": [0, 0, 1, 2]
	expected := []int{0, 0, 1, 2}
	for i, v := range expected {
		if failure[i] != v {
			t.Fatalf("failure[%d] = %d, expected %d", i, failure[i], v)
		}
	}
}

// TestServiceSearchReturnsNonNilItemsOnNoMatch proves that Search returns a
// non-nil Items slice even when no matches are found, honoring the Page
// contract that Items is always non-nil (L1).
func TestServiceSearchReturnsNonNilItemsOnNoMatch(t *testing.T) {
	h := newServiceTestHarness(t, "trace-t", minimalValidTrace)
	page, domain := h.service.Search(context.Background(), targetEvidence(h.scopeID), SearchQuery{
		Handle:   h.handle,
		Text:     "nonexistent-text-that-does-not-appear",
		PageSize: 10,
	})
	if domain != nil {
		t.Fatalf("Search failed: %v", domain)
	}
	if page.Items == nil {
		t.Fatal("expected non-nil Items slice on no match, got nil")
	}
	if len(page.Items) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(page.Items))
	}
}

// TestServiceSearchMatchOffsetsAreNonNegative proves that all search match
// offsets are non-negative. KMP state is not carried across record boundaries,
// so a match that would span two records (producing a negative offset) is never
// returned (M3).
func TestServiceSearchMatchOffsetsAreNonNegative(t *testing.T) {
	h := newServiceTestHarness(t, "trace-t", minimalValidTrace)
	// Search for a literal that appears in every record. All matches must have
	// non-negative offsets because KMP state is reset at each record boundary.
	page, domain := h.service.Search(context.Background(), targetEvidence(h.scopeID), SearchQuery{
		Handle:   h.handle,
		Text:     "attempt-1",
		PageSize: 100,
	})
	if domain != nil {
		t.Fatalf("Search failed: %v", domain)
	}
	if len(page.Items) == 0 {
		t.Fatal("expected at least one match")
	}
	for _, m := range page.Items {
		if m.MatchOffset < 0 {
			t.Fatalf("negative match offset %d for sequence %d — KMP state leaked across record boundary", m.MatchOffset, m.Sequence)
		}
	}
}
