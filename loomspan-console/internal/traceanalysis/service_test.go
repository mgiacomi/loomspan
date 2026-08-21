package traceanalysis

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/applicationclient"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/evidence"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/target"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/workspace"
)

func targetEvidence(scopeID target.ScopeID) evidence.Reference { return evidence.ForTarget(scopeID) }
func targetCursorKey(scopeID target.ScopeID) string            { return ownerCursorKey(evidence.Target(scopeID)) }

// serviceTestHarness wires a real traceanalysis.Service against a real
// artifact.Service and workspace, processes a fixture trace through an httptest
// server, and exposes the scope/handle for query tests.
type serviceTestHarness struct {
	t         *testing.T
	service   *Service
	artifacts *artifact.Service
	scope     target.Scope
	scopeID   target.ScopeID
	handle    artifact.Handle
	traceID   string
}

// javaCompatibleInstanceJSON is the exact instance probe body a Java adapter at
// consoleCompatibilityVersion "0.1.0-SNAPSHOT" emits. Reused from the console
// integration tests so the scope's compatibility gate is established exactly
// as in production.
const javaCompatibleInstanceJSON = `{"instanceId":"11111111-1111-4111-8111-111111111111","consoleCompatibilityVersion":"0.1.0-SNAPSHOT","observedAt":"2026-07-25T12:00:00Z","liveMonitoringAvailable":true,"registeredSkillCount":0,"activeExecutionCount":0,"catalogedTraceCount":1,"tracePersistencePolicy":"PERSISTENT","completionGraceTtl":"PT2M","traceCatalogMetadataTtl":"PT168H"}`

// deriveSessionID returns the session ID that matches the NDJSON content for
// the given trace ID. Test traces use different conventions:
//   - minimalValidTrace (trace-t) uses "session-t"
//   - nestedFrameUsageTrace (trace-nested-frame-usage) uses "session-nested-frame-usage"
//   - chunkedPayloadTrace (t) uses "s"
func deriveSessionID(traceID string) string {
	switch traceID {
	case "trace-t":
		return "session-t"
	case "t":
		return "s"
	default:
		return "session-" + strings.TrimPrefix(traceID, "trace-")
	}
}

// newServiceTestHarness processes the given NDJSON body through the real
// traceanalysis.Service + artifact.Service composition and returns a harness
// ready for query tests.
func newServiceTestHarness(t *testing.T, traceID, ndjson string) *serviceTestHarness {
	return newServiceTestHarnessForVersion(t, traceID, ndjson, "")
}

func newServiceTestHarnessForVersion(t *testing.T, traceID, ndjson, compatibilityVersion string) *serviceTestHarness {
	t.Helper()
	body := []byte(ndjson)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(applicationclient.InstanceIDHeader, "11111111-1111-4111-8111-111111111111")
		switch {
		case strings.HasSuffix(r.URL.Path, "/instance"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(javaCompatibleInstanceJSON))
		case strings.HasSuffix(r.URL.Path, "/traces/"+traceID) && !strings.HasSuffix(r.URL.Path, "/artifact"):
			w.Header().Set("Content-Type", "application/json")
			// The session ID must match the NDJSON content's sessionId.
			// All test traces use "session-" + (traceID without "trace-" prefix).
			sessionID := deriveSessionID(traceID)
			_, _ = fmt.Fprintf(w, `{"targetScopeId":"scope-test","traceId":"%s","sessionId":"%s","entrySkill":"CheckDns","outcome":"SUCCEEDED","finalizedAt":"2026-07-24T12:00:00Z","sizeBytes":%d,"persistencePolicy":"ALWAYS","applicationTraceExpiresAt":"2026-08-01T12:00:00Z"}`,
				traceID, sessionID, len(body))
		case strings.HasSuffix(r.URL.Path, "/traces/"+traceID+"/artifact"):
			w.Header().Set("Content-Type", applicationclient.ArtifactMediaType)
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
			w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="loomspan-trace-%s.ndjson"`, traceID))
			w.Header().Set("Cache-Control", "no-store")
			_, _ = w.Write(body)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	ws, err := workspace.Open(filepath.Join(t.TempDir(), "work"))
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	t.Cleanup(func() { _ = ws.Close() })

	policy := applicationclient.NetworkPolicy{
		ConnectTimeout: time.Second, ResponseHeaderTimeout: time.Second, RequestTimeout: 30 * time.Second,
	}
	targetContext, err := target.New(func(address applicationclient.Address) (target.ProbeClient, error) {
		return applicationclient.New(address, policy, "0.1.0-SNAPSHOT")
	}, nil, time.Now)
	if err != nil {
		t.Fatalf("create target context: %v", err)
	}
	t.Cleanup(targetContext.Close)

	traceAnalysisService := NewService(nil)
	if compatibilityVersion != "" {
		traceAnalysisService = NewServiceForCompatibilityVersion(nil, compatibilityVersion)
	}
	artifactSvc, err := artifact.New(artifact.Config{
		MaxBytes: 128 << 20,
		IdleTTL:  time.Hour,
	}, artifact.Dependencies{
		Lifetime:  context.Background(),
		Workspace: ws,
		TraceLoader: func(ctx context.Context, scope target.Scope, tid string) (artifact.TraceMetadata, *consolecore.Error) {
			endpoint := scope.Target.TraceEndpoint(tid)
			_, domain := scope.Upstream(ctx, endpoint, 1<<20)
			if domain != nil {
				return artifact.TraceMetadata{}, domain
			}
			return artifact.TraceMetadata{
				TraceID:           traceID,
				SessionID:         deriveSessionID(traceID),
				Outcome:           "SUCCEEDED",
				SizeBytes:         int64(len(body)),
				PersistencePolicy: "ALWAYS",
			}, nil
		},
		StreamOpener: func(ctx context.Context, scope target.Scope, tid string) (*applicationclient.ArtifactStream, *consolecore.Error) {
			return scope.OpenArtifact(ctx, tid)
		},
		Processor: traceAnalysisService,
	})
	if err != nil {
		t.Fatalf("create artifact service: %v", err)
	}
	t.Cleanup(artifactSvc.Close)
	traceAnalysisService.SetArtifactService(artifactSvc)

	if err := targetContext.RegisterOwner("artifacts", artifactSvc); err != nil {
		t.Fatalf("register artifact owner: %v", err)
	}
	if err := targetContext.Select(server.URL); err != nil {
		t.Fatalf("select target: %v", err)
	}
	if _, domain := targetContext.SupplyCredential(context.Background(), []byte(strings.Repeat("k", 32))); domain != nil {
		t.Fatalf("supply credential: %v", domain)
	}
	scope, domain := targetContext.Capture()
	if domain != nil {
		t.Fatalf("capture scope: %v", domain)
	}

	handle, domain := artifactSvc.Acquire(context.Background(), scope, traceID)
	if domain != nil {
		t.Fatalf("Acquire failed: %v", domain)
	}
	return &serviceTestHarness{
		t:         t,
		service:   traceAnalysisService,
		artifacts: artifactSvc,
		scope:     scope,
		scopeID:   scope.ID,
		handle:    handle.Handle,
		traceID:   traceID,
	}
}

func TestServiceGetSummary(t *testing.T) {
	h := newServiceTestHarness(t, "trace-t", minimalValidTrace)
	summary, domain := h.service.GetSummary(context.Background(), targetEvidence(h.scopeID), SummaryRequest{Handle: h.handle})
	if domain != nil {
		t.Fatalf("GetSummary failed: %v", domain)
	}
	if summary.Context.TraceID != "trace-t" {
		t.Fatalf("expected trace ID %q, got %q", "trace-t", summary.Context.TraceID)
	}
	if summary.Outcome != "SUCCEEDED" {
		t.Fatalf("expected outcome SUCCEEDED, got %q", summary.Outcome)
	}
	if summary.RecordCount != 5 {
		t.Fatalf("expected 5 records, got %d", summary.RecordCount)
	}
	var histogramTotal int64
	for _, count := range summary.RecordCountsByType {
		histogramTotal += count
	}
	if histogramTotal != summary.RecordCount || summary.RecordCountsByType[RecordTraceStarted] != 1 || summary.RecordCountsByType[RecordTraceCompleted] != 1 {
		t.Fatalf("record histogram=%v recordCount=%d", summary.RecordCountsByType, summary.RecordCount)
	}
	if summary.AttemptCount != 1 {
		t.Fatalf("expected 1 attempt, got %d", summary.AttemptCount)
	}
	if summary.RetryCount != 0 {
		t.Fatalf("expected 0 retries, got %d", summary.RetryCount)
	}
	if !summary.UsageComplete {
		t.Fatal("expected usage complete")
	}
	if summary.AttributedUsage.TotalUnits != 14 {
		t.Fatalf("expected attributed total 14, got %d", summary.AttributedUsage.TotalUnits)
	}
}

func TestManifestRejectsInvalidRecordHistogram(t *testing.T) {
	valid := manifest{Schema: manifestSchemaV1, TraceID: "t", SessionID: "s", RecordCount: 1, RecordCountsByType: map[TraceRecordType]int64{RecordTraceStarted: 1}}
	for name, mutate := range map[string]func(*manifest){
		"unknown key": func(m *manifest) { m.RecordCountsByType = map[TraceRecordType]int64{"UNKNOWN": 1} },
		"zero value":  func(m *manifest) { m.RecordCountsByType = map[TraceRecordType]int64{RecordTraceStarted: 0} },
		"negative":    func(m *manifest) { m.RecordCountsByType = map[TraceRecordType]int64{RecordTraceStarted: -1} },
		"wrong sum":   func(m *manifest) { m.RecordCount = 2 },
		"overflow": func(m *manifest) {
			m.RecordCount = int64(^uint64(0) >> 1)
			m.RecordCountsByType = map[TraceRecordType]int64{RecordTraceStarted: int64(^uint64(0) >> 1), RecordTraceCompleted: 1}
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			mutate(&candidate)
			if err := validateManifest(candidate); err == nil {
				t.Fatal("expected invalid histogram")
			}
		})
	}
}

func TestFrameDirectRetryCountUsesAttributedLaterAttempts(t *testing.T) {
	withFrame := func(record, frameID string) string {
		return strings.Replace(record, `"frameId":null`, `"frameId":"`+frameID+`"`, 1)
	}
	raw := strings.Join([]string{
		startedRecord(1),
		frameRecord(2, "initial-frame", "", false, "MODEL_CALL", true),
		withFrame(requestRecord(3, "retry-1", "attempt-1", 1, false), "initial-frame"),
		responseRecord(4, "initial-frame", "retry-1", "attempt-1", 1, 0, 0, 0, "EXACT"),
		frameRecord(5, "initial-frame", "", false, "MODEL_CALL", false),
		frameRecord(6, "retry-frame", "", false, "MODEL_CALL", true),
		withFrame(requestRecord(7, "retry-1", "attempt-2", 2, false), "retry-frame"),
		responseRecord(8, "retry-frame", "retry-1", "attempt-2", 2, 0, 0, 0, "EXACT"),
		frameRecord(9, "retry-frame", "", false, "MODEL_CALL", false),
		completionRecord(10, "SUCCEEDED", 0, 0, 0, ""),
	}, "\n") + "\n"
	h := newServiceTestHarness(t, "t", raw)
	page, domain := h.service.QueryFrames(context.Background(), targetEvidence(h.scopeID), FrameQuery{Handle: h.handle, Projection: FrameProjectionDetailed, PageSize: 10})
	if domain != nil || len(page.Items) != 2 {
		t.Fatalf("page=%+v domain=%v", page, domain)
	}
	if page.Items[0].DirectRetryCount != 0 || page.Items[1].DirectRetryCount != 1 {
		t.Fatalf("cross-frame retry counts=%d/%d", page.Items[0].DirectRetryCount, page.Items[1].DirectRetryCount)
	}
}

func TestFrameDirectRetryCountCountsLaterAttemptsInSameFrame(t *testing.T) {
	withFrame := func(record string) string {
		return strings.Replace(record, `"frameId":null`, `"frameId":"frame"`, 1)
	}
	raw := strings.Join([]string{
		startedRecord(1),
		frameRecord(2, "frame", "", false, "MODEL_CALL", true),
		withFrame(requestRecord(3, "retry-1", "attempt-1", 1, false)),
		responseRecord(4, "frame", "retry-1", "attempt-1", 1, 0, 0, 0, "EXACT"),
		withFrame(requestRecord(5, "retry-1", "attempt-2", 2, false)),
		responseRecord(6, "frame", "retry-1", "attempt-2", 2, 0, 0, 0, "EXACT"),
		frameRecord(7, "frame", "", false, "MODEL_CALL", false),
		completionRecord(8, "SUCCEEDED", 0, 0, 0, ""),
	}, "\n") + "\n"
	h := newServiceTestHarness(t, "t", raw)
	page, domain := h.service.QueryFrames(context.Background(), targetEvidence(h.scopeID), FrameQuery{Handle: h.handle, Projection: FrameProjectionDetailed, PageSize: 10})
	if domain != nil || len(page.Items) != 1 || page.Items[0].DirectAttemptCount != 2 || page.Items[0].DirectRetryCount != 1 {
		t.Fatalf("same-frame retries page=%+v domain=%v", page, domain)
	}
}

func TestFrameMinDirectRetriesFiltersExistingExactFrameCount(t *testing.T) {
	withFrame := func(record, frameID string) string {
		return strings.Replace(record, `"frameId":null`, `"frameId":"`+frameID+`"`, 1)
	}
	lines := []string{startedRecord(1), frameRecord(2, "root", "", false, "ROOT_MISSION", true)}
	sequence := 3
	addFrame := func(frameID string, attempts int) {
		lines = append(lines, frameRecord(sequence, frameID, "root", true, "MODEL_CALL", true))
		sequence++
		for attempt := 1; attempt <= attempts; attempt++ {
			attemptID := frameID + "-attempt-" + itoa(attempt)
			lines = append(lines,
				withFrame(requestRecord(sequence, frameID+"-retry", attemptID, attempt, false), frameID),
				responseRecord(sequence+1, frameID, frameID+"-retry", attemptID, attempt, 0, 0, 0, "EXACT"),
			)
			sequence += 2
		}
		lines = append(lines, frameRecord(sequence, frameID, "root", true, "MODEL_CALL", false))
		sequence++
	}
	addFrame("zero", 1)
	addFrame("one", 2)
	addFrame("two", 3)
	lines = append(lines, frameRecord(sequence, "root", "", false, "ROOT_MISSION", false), completionRecord(sequence+1, "SUCCEEDED", 0, 0, 0, ""))
	h := newServiceTestHarness(t, "t", strings.Join(lines, "\n")+"\n")

	query := func(filter FrameFilter, projection FrameProjection, order FrameOrder, pageSize int, cursor string) (Page[FrameSummary], *consolecore.Error) {
		return h.service.QueryFrames(context.Background(), targetEvidence(h.scopeID), FrameQuery{
			Handle: h.handle, Filter: filter, Projection: projection, Order: order, PageSize: pageSize, Cursor: cursor,
		})
	}
	assertIDs := func(page Page[FrameSummary], domain *consolecore.Error, want ...string) {
		t.Helper()
		if domain != nil {
			t.Fatalf("QueryFrames failed: %v", domain)
		}
		got := make([]string, 0, len(page.Items))
		for _, item := range page.Items {
			got = append(got, item.FrameID)
		}
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Fatalf("frame IDs=%v want=%v", got, want)
		}
	}

	page, domain := query(FrameFilter{MinDirectRetries: 1}, FrameProjectionCompact, FrameOrderCanonical, 10, "")
	assertIDs(page, domain, "one", "two")
	page, domain = query(FrameFilter{MinDirectRetries: 2}, FrameProjectionDetailed, FrameOrderCanonical, 10, "")
	assertIDs(page, domain, "two")
	page, domain = query(FrameFilter{MinDirectRetries: 1, FrameIDs: []string{"zero", "one"}}, FrameProjectionDetailed, FrameOrderUsageDesc, 10, "")
	assertIDs(page, domain, "one")
	page, domain = query(FrameFilter{MinDirectRetries: 1, FrameType: "ROOT_MISSION"}, FrameProjectionDetailed, FrameOrderDurationDesc, 10, "")
	assertIDs(page, domain)

	first, domain := query(FrameFilter{MinDirectRetries: 1}, FrameProjectionDetailed, FrameOrderCanonical, 1, "")
	if domain != nil || !first.HasMore || first.NextCursor == "" {
		t.Fatalf("first retry page=%+v domain=%v", first, domain)
	}
	_, domain = query(FrameFilter{MinDirectRetries: 2}, FrameProjectionDetailed, FrameOrderCanonical, 1, first.NextCursor)
	if domain == nil || domain.Code != consolecore.CodeInvalidCursor {
		t.Fatalf("changed threshold domain=%v", domain)
	}
}

func TestFrameMinDirectRetriesRejectsNegativeInternalInput(t *testing.T) {
	h := newServiceTestHarness(t, "trace-nested-frame-usage", nestedFrameUsageTrace)
	_, domain := h.service.QueryFrames(context.Background(), targetEvidence(h.scopeID), FrameQuery{
		Handle: h.handle, Filter: FrameFilter{MinDirectRetries: -1}, PageSize: 10,
	})
	if domain == nil || domain.Code != consolecore.CodeInvalidArgument {
		t.Fatalf("domain=%v", domain)
	}
}

func TestFrameOutcomeIsOptionalScalarAndFiltersSingularly(t *testing.T) {
	closeWithStatus := func(sequence int, status string) string {
		close := frameRecord(sequence, "frame", "", false, "MODEL_CALL", false)
		return strings.Replace(close, `"metadata":{}`, `"metadata":{"status":"`+status+`"}`, 1)
	}
	failed := "failed"
	for _, test := range []struct {
		name       string
		frameLines []string
		want       *string
	}{
		{name: "open", frameLines: []string{frameRecord(2, "frame", "", false, "MODEL_CALL", true)}},
		{name: "blank close", frameLines: []string{frameRecord(2, "frame", "", false, "MODEL_CALL", true), closeWithStatus(3, "")}},
		{name: "failed close", frameLines: []string{frameRecord(2, "frame", "", false, "MODEL_CALL", true), closeWithStatus(3, "failed")}, want: &failed},
	} {
		t.Run(test.name, func(t *testing.T) {
			lines := append([]string{startedRecord(1)}, test.frameLines...)
			lines = append(lines, completionRecord(len(lines)+1, "SUCCEEDED", 0, 0, 0, ""))
			h := newServiceTestHarness(t, "t", strings.Join(lines, "\n")+"\n")
			page, domain := h.service.QueryFrames(context.Background(), targetEvidence(h.scopeID), FrameQuery{Handle: h.handle, Projection: FrameProjectionDetailed, PageSize: 10})
			if domain != nil || len(page.Items) != 1 || !reflect.DeepEqual(page.Items[0].Outcome, test.want) {
				t.Fatalf("page=%+v domain=%v want=%v", page, domain, test.want)
			}
			filtered, domain := h.service.QueryFrames(context.Background(), targetEvidence(h.scopeID), FrameQuery{Handle: h.handle, Projection: FrameProjectionDetailed, Filter: FrameFilter{Outcome: "failed"}, PageSize: 10})
			wantMatches := 0
			if test.want != nil {
				wantMatches = 1
			}
			if domain != nil || len(filtered.Items) != wantMatches {
				t.Fatalf("filtered=%+v domain=%v wantMatches=%d", filtered, domain, wantMatches)
			}
		})
	}
}

func TestServiceQueryFramesEmpty(t *testing.T) {
	h := newServiceTestHarness(t, "trace-t", minimalValidTrace)
	page, domain := h.service.QueryFrames(context.Background(), targetEvidence(h.scopeID), FrameQuery{
		Handle:     h.handle,
		Projection: FrameProjectionDetailed,
		PageSize:   10,
	})
	if domain != nil {
		t.Fatalf("QueryFrames failed: %v", domain)
	}
	if len(page.Items) != 0 {
		t.Fatalf("expected 0 frames, got %d", len(page.Items))
	}
	if page.HasMore {
		t.Fatal("expected no more pages")
	}
}

func TestServiceQueryFramesWithFrames(t *testing.T) {
	h := newServiceTestHarness(t, "trace-nested-frame-usage", nestedFrameUsageTrace)
	page, domain := h.service.QueryFrames(context.Background(), targetEvidence(h.scopeID), FrameQuery{
		Handle: h.handle, Projection: FrameProjectionDetailed, PageSize: 10,
	})
	if domain != nil {
		t.Fatalf("QueryFrames failed: %v", domain)
	}
	if len(page.Items) != 2 {
		t.Fatalf("expected 2 frames, got %d", len(page.Items))
	}
	if page.Items[0].FrameID != "root" {
		t.Fatalf("expected first frame ID 'root', got %q", page.Items[0].FrameID)
	}
	if page.Items[0].Route != "root.skill" {
		t.Fatalf("expected route 'root.skill', got %q", page.Items[0].Route)
	}
	if page.Items[1].FrameID != "skill" {
		t.Fatalf("expected second frame ID 'skill', got %q", page.Items[1].FrameID)
	}
	for _, frame := range page.Items {
		if frame.OpenedTimestampMillis == 0 || frame.ClosedTimestampMillis == nil {
			t.Fatalf("frame boundary timestamps were not projected: %+v", frame)
		}
		if got := *frame.ClosedTimestampMillis - frame.OpenedTimestampMillis; frame.InclusiveDurationMillis == nil || got != *frame.InclusiveDurationMillis {
			t.Fatalf("returned frame boundaries do not match processor duration: %+v", frame)
		}
	}
}

func TestLogicalRecordDataIsRepresentedAsReadableSemanticContent(t *testing.T) {
	h := newServiceTestHarness(t, "trace-t", minimalValidTrace)
	page, domain := h.service.QueryRecords(context.Background(), targetEvidence(h.scopeID), RecordQuery{
		Handle: h.handle, Representation: RecordRepresentationLogical,
		Filter: RecordFilter{Types: []string{string(RecordModelResponseReceived)}}, PageSize: 10,
	})
	if domain != nil || len(page.Items) != 1 {
		t.Fatalf("query: page=%+v domain=%v", page, domain)
	}
	record := page.Items[0]
	if record.Representation != "logical" || record.Content == nil || !record.Content.Available || !record.Content.Complete || record.Content.ContentType != "application/json" || record.Content.ContentRef == "" {
		t.Fatalf("ordinary data descriptor is not truthful: %+v", record)
	}
	first, domain := h.service.ReadContentRange(context.Background(), targetEvidence(h.scopeID), RangeRequest{Handle: h.handle, ContentRef: record.Content.ContentRef, Start: 0, MaxBytes: 7})
	if domain != nil || !first.HasMore || first.TotalLength != record.Content.RetainedBytes {
		t.Fatalf("first range=%+v domain=%v", first, domain)
	}
	content := append([]byte{}, first.Content...)
	next := first
	for next.HasMore {
		next, domain = h.service.ReadContentRange(context.Background(), targetEvidence(h.scopeID), RangeRequest{Handle: h.handle, ContentRef: record.Content.ContentRef, ContinueCursor: next.NextCursor, MaxBytes: 7})
		if domain != nil {
			t.Fatalf("continued range=%+v domain=%v", next, domain)
		}
		content = append(content, next.Content...)
	}
	if got := string(content); got != `{"content":"fixture response"}` {
		t.Fatalf("selected value=%q", got)
	}
}

func TestOrdinarySemanticContentPreservesAbsentNullAndEveryJSONShape(t *testing.T) {
	original := `"data":{"content":"fixture response"}`
	for _, test := range []struct {
		name        string
		replacement string
		want        string
		available   bool
	}{
		{name: "absent", replacement: ``, available: false},
		{name: "explicit-null", replacement: `"data":null`, want: `null`, available: true},
		{name: "scalar-number", replacement: `"data":42`, want: `42`, available: true},
		{name: "scalar-boolean", replacement: `"data":true`, want: `true`, available: true},
		{name: "string", replacement: `"data":"value"`, want: `"value"`, available: true},
		{name: "array", replacement: `"data":[1,"two"]`, want: `[1,"two"]`, available: true},
		{name: "object", replacement: `"data":{"value":1}`, want: `{"value":1}`, available: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			replacement := test.replacement
			trace := minimalValidTrace
			if test.name == "absent" {
				trace = strings.Replace(trace, `,`+original, ``, 1)
			} else {
				trace = strings.Replace(trace, original, replacement, 1)
			}
			if test.name == "explicit-null" {
				trace = strings.Replace(trace, `"usage":{"promptUnits"`, `"semanticContentPresent":true,"usage":{"promptUnits"`, 1)
			}
			h := newServiceTestHarness(t, "trace-t", trace)
			page, domain := h.service.QueryRecords(context.Background(), targetEvidence(h.scopeID), RecordQuery{
				Handle: h.handle, Representation: RecordRepresentationLogical, InlineContent: true,
				Filter: RecordFilter{Types: []string{string(RecordModelResponseReceived)}}, PageSize: 1,
			})
			if domain != nil || len(page.Items) != 1 {
				t.Fatalf("page=%+v domain=%v", page, domain)
			}
			descriptor := page.Items[0].Content
			if !test.available {
				if descriptor != nil {
					t.Fatalf("absent data produced descriptor=%+v", descriptor)
				}
				return
			}
			if descriptor == nil || !descriptor.Available || !descriptor.Complete || descriptor.ContentRef == "" || string(descriptor.InlineContent) != test.want {
				t.Fatalf("descriptor=%+v want=%q", descriptor, test.want)
			}
			read, readDomain := h.service.ReadContentRange(context.Background(), targetEvidence(h.scopeID), RangeRequest{Handle: h.handle, ContentRef: descriptor.ContentRef, MaxBytes: MaxRangeBytes})
			if readDomain != nil || read.HasMore || string(read.Content) != test.want {
				t.Fatalf("read=%+v domain=%v want=%q", read, readDomain, test.want)
			}
		})
	}
}

func TestInlineContentPerValueAndAggregateBoundaries(t *testing.T) {
	for _, test := range []struct {
		name string
		size int
		want InlineOmissionReason
	}{
		{"below", MaxInlineContentBytes - 1, ""},
		{"at", MaxInlineContentBytes, ""},
		{"above", MaxInlineContentBytes + 1, InlineOmissionPerValue},
	} {
		t.Run("per-value-"+test.name, func(t *testing.T) {
			desc := &ContentDescriptor{ContentRef: "still-readable"}
			aggregate := int64(0)
			inlineOrdinaryDescriptor(desc, bytes.Repeat([]byte{'x'}, test.size), &aggregate)
			if desc.InlineOmission != test.want || desc.ContentRef == "" || (test.want == "" && len(desc.InlineContent) != test.size) || (test.want != "" && len(desc.InlineContent) != 0) {
				t.Fatalf("descriptor=%+v inline=%d aggregate=%d", desc, len(desc.InlineContent), aggregate)
			}
		})
	}
	for _, test := range []struct {
		name  string
		start int64
		want  InlineOmissionReason
	}{
		{"below", MaxAggregateInlineContentBytes - 2, ""},
		{"at", MaxAggregateInlineContentBytes - 1, ""},
		{"above", MaxAggregateInlineContentBytes, InlineOmissionAggregate},
	} {
		t.Run("aggregate-"+test.name, func(t *testing.T) {
			desc := &ContentDescriptor{ContentRef: "still-readable"}
			aggregate := test.start
			inlineOrdinaryDescriptor(desc, []byte{'x'}, &aggregate)
			if desc.InlineOmission != test.want || desc.ContentRef == "" || (test.want != "" && len(desc.InlineContent) != 0) {
				t.Fatalf("descriptor=%+v aggregate=%d", desc, aggregate)
			}
		})
	}
}

func TestByteShortenedRecordPagesRecomputeInlineAggregateForAcceptedPage(t *testing.T) {
	content, err := json.Marshal(strings.Repeat("x", 8000))
	if err != nil {
		t.Fatal(err)
	}
	lines := []string{startedRecord(1)}
	for sequence := 2; sequence <= 6; sequence++ {
		lines = append(lines, fmt.Sprintf(`{"traceId":"t","sessionId":"s","sequence":%d,"timestamp":%s,"recordType":"PLAN_QUALITY_WARNING","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"th","metadata":{},"data":%s}`, sequence, timestampForSeq(sequence), content))
	}
	lines = append(lines, completionRecord(7, "SUCCEEDED", 0, 0, 0, ""))
	h := newServiceTestHarness(t, "t", strings.Join(lines, "\n")+"\n")
	continuation := ""
	seenWarnings := 0
	for {
		admitted := 0
		page, domain := h.service.QueryRecords(context.Background(), targetEvidence(h.scopeID), RecordQuery{Handle: h.handle, Representation: RecordRepresentationLogical, InlineContent: true, PageSize: 64, Cursor: continuation, Admit: func(RecordSummary) bool { admitted++; return admitted <= 2 }})
		if domain != nil {
			t.Fatal(domain)
		}
		for _, record := range page.Items {
			if record.Type != string(RecordPlanQualityWarning) {
				continue
			}
			seenWarnings++
			if record.Content == nil || len(record.Content.InlineContent) == 0 || record.Content.InlineOmission != "" {
				t.Fatalf("warning content was not reconsidered for its accepted page: %+v", record.Content)
			}
		}
		if !page.HasMore {
			break
		}
		continuation = page.NextCursor
	}
	if seenWarnings != 5 {
		t.Fatalf("seen warnings=%d, want 5", seenWarnings)
	}
}

func TestFrameContinuationCannotCrossProjection(t *testing.T) {
	h := newServiceTestHarness(t, "trace-nested-frame-usage", nestedFrameUsageTrace)
	first, domain := h.service.QueryFrames(context.Background(), targetEvidence(h.scopeID), FrameQuery{Handle: h.handle, Projection: FrameProjectionCompact, PageSize: 1})
	if domain != nil || first.NextCursor == "" {
		t.Fatalf("first=%+v domain=%v", first, domain)
	}
	_, domain = h.service.QueryFrames(context.Background(), targetEvidence(h.scopeID), FrameQuery{Handle: h.handle, Projection: FrameProjectionDetailed, PageSize: 1, Cursor: first.NextCursor})
	if domain == nil || domain.Code != consolecore.CodeInvalidCursor {
		t.Fatalf("domain=%v", domain)
	}
}

func TestCompactFrameProjectionRetainsHierarchyAndOmitsDetailedEvidence(t *testing.T) {
	h := newServiceTestHarness(t, "trace-nested-frame-usage", nestedFrameUsageTrace)
	page, domain := h.service.QueryFrames(context.Background(), targetEvidence(h.scopeID), FrameQuery{Handle: h.handle, PageSize: 10})
	if domain != nil || len(page.Items) < 2 {
		t.Fatalf("page=%+v domain=%v", page, domain)
	}
	root := page.Items[0]
	if !reflect.DeepEqual(root.ChildFrameIDs, []string{"skill"}) {
		t.Fatalf("compact hierarchy lost children: %+v", root.ChildFrameIDs)
	}
	if root.InclusiveDurationMillis == nil {
		t.Fatalf("compact hierarchy lost inclusive duration: %+v", root)
	}
	for _, frame := range page.Items {
		if frame.SelfDurationMillis != nil || frame.DirectUsage != (Usage{}) || frame.DescendantUsage != (Usage{}) || frame.InclusiveUsage != (Usage{}) || len(frame.SkillNames) != 0 || len(frame.AttemptIDs) != 0 || len(frame.RetrySequenceIDs) != 0 || len(frame.ValidationStatuses) != 0 || len(frame.FailureIDs) != 0 || len(frame.GapKinds) != 0 || len(frame.UncertaintyKinds) != 0 {
			t.Fatalf("compact frame retained detailed evidence: %+v", frame)
		}
	}
}

func TestFrameQueriesExposeUsageCompletenessAndRecordedCrossReferences(t *testing.T) {
	withChildFrame := func(record string) string {
		return strings.Replace(record,
			`"frameId":null,"parentFrameId":null,"frameType":null,"route":null`,
			`"frameId":"child","parentFrameId":"root","frameType":"SKILL_EXECUTION","route":"root.skill"`, 1)
	}
	response := strings.Replace(responseRecord(6, "child", "retry-1", "attempt-1", 1, 0, 0, 0, "UNAVAILABLE"),
		`"usage":`, `"skillName":"skill.alpha","usage":`, 1)
	closeChild := strings.Replace(frameRecord(9, "child", "root", true, "SKILL_EXECUTION", false),
		`"metadata":{}`, `"metadata":{"status":"completed"}`, 1)
	closeRoot := strings.Replace(frameRecord(10, "root", "", false, "ROOT_MISSION", false),
		`"metadata":{}`, `"metadata":{"status":"completed"}`, 1)
	raw := startedRecord(1) + "\n" +
		frameRecord(2, "root", "", false, "ROOT_MISSION", true) + "\n" +
		frameRecord(3, "child", "root", true, "SKILL_EXECUTION", true) + "\n" +
		requestRecord(4, "retry-1", "attempt-1", 1, true) + "\n" +
		requestRecord(5, "retry-1", "attempt-1", 1, false) + "\n" +
		response + "\n" +
		withChildFrame(advisorRecord(7, true, "retry-1", "attempt-1", 1, "passed")) + "\n" +
		withChildFrame(errorRecord(8, "failure-1", false)) + "\n" +
		closeChild + "\n" + closeRoot + "\n" + completionRecord(11, "SUCCEEDED", 0, 0, 0, "") + "\n"
	h := newServiceTestHarness(t, "t", raw)

	page, domain := h.service.QueryFrames(context.Background(), targetEvidence(h.scopeID), FrameQuery{
		Handle: h.handle, Projection: FrameProjectionDetailed,
		Filter: FrameFilter{
			SkillName: "skill.alpha", Outcome: "completed", AttemptID: "attempt-1",
			RetrySequenceID: "retry-1", ValidationStatus: "passed", FailureID: "failure-1",
		},
		PageSize: 10,
	})
	if domain != nil {
		t.Fatalf("QueryFrames failed: %v", domain)
	}
	if len(page.Items) != 1 || page.Items[0].FrameID != "child" {
		t.Fatalf("expected only child frame, got %+v", page.Items)
	}
	child := page.Items[0]
	if child.DirectUsageComplete || child.InclusiveUsageComplete {
		t.Fatalf("unavailable direct usage reported complete: %+v", child)
	}
	if !reflect.DeepEqual(child.SkillNames, []string{"skill.alpha"}) ||
		child.Outcome == nil || *child.Outcome != "completed" ||
		!reflect.DeepEqual(child.AttemptIDs, []string{"attempt-1"}) ||
		!reflect.DeepEqual(child.RetrySequenceIDs, []string{"retry-1"}) ||
		!reflect.DeepEqual(child.ValidationStatuses, []string{"passed"}) ||
		!reflect.DeepEqual(child.FailureIDs, []string{"failure-1"}) {
		t.Fatalf("recorded frame relationships were not preserved: %+v", child)
	}
	failurePage, failureDomain := h.service.QueryFailures(context.Background(), targetEvidence(h.scopeID), FailureQuery{Handle: h.handle, PageSize: 10})
	if failureDomain != nil || len(failurePage.Items) != 1 {
		t.Fatalf("failure query: page=%+v domain=%v", failurePage, failureDomain)
	}
	failure := failurePage.Items[0]
	if failure.Sequence != 8 || failure.RecordType != "ERROR_RECORDED" || failure.FrameID != "child" ||
		failure.Route != "root.skill" || failure.FailureID != "failure-1" {
		t.Fatalf("direct failure relationships were not preserved: %+v", failure)
	}

	all, domain := h.service.QueryFrames(context.Background(), targetEvidence(h.scopeID), FrameQuery{Handle: h.handle, Projection: FrameProjectionDetailed, PageSize: 1})
	if domain != nil || !all.HasMore {
		t.Fatalf("first frame page: page=%+v domain=%v", all, domain)
	}
	decoded, err := decodeCursor(all.NextCursor)
	if err != nil {
		t.Fatalf("decode frame cursor: %v", err)
	}
	if decoded.Position <= 1 {
		t.Fatalf("fact continuation must store a byte offset, got %d", decoded.Position)
	}
	second, domain := h.service.QueryFrames(context.Background(), targetEvidence(h.scopeID), FrameQuery{
		Handle: h.handle, Projection: FrameProjectionDetailed, PageSize: 1, Cursor: all.NextCursor,
	})
	if domain != nil || len(second.Items) != 1 || second.Items[0].FrameID != "child" {
		t.Fatalf("second frame page: page=%+v domain=%v", second, domain)
	}
	if second.Items[0].DirectUsageComplete || second.Items[0].InclusiveUsageComplete {
		t.Fatalf("child usage completeness lost after persistence: %+v", second.Items[0])
	}
	if !all.Items[0].DirectUsageComplete || all.Items[0].DescendantUsageComplete || all.Items[0].InclusiveUsageComplete {
		t.Fatalf("root completeness did not propagate child uncertainty: %+v", all.Items[0])
	}
	if !reflect.DeepEqual(all.Items[0].ChildFrameIDs, []string{"child"}) {
		t.Fatalf("root immediate-child references were not exposed: %+v", all.Items[0].ChildFrameIDs)
	}
	if len(second.Items[0].ChildFrameIDs) != 0 {
		t.Fatalf("leaf frame unexpectedly exposed children: %+v", second.Items[0].ChildFrameIDs)
	}
}

func TestServiceQueryFramesPagination(t *testing.T) {
	h := newServiceTestHarness(t, "trace-nested-frame-usage", nestedFrameUsageTrace)
	page1, domain := h.service.QueryFrames(context.Background(), targetEvidence(h.scopeID), FrameQuery{
		Handle:   h.handle,
		PageSize: 1,
	})
	if domain != nil {
		t.Fatalf("QueryFrames page 1 failed: %v", domain)
	}
	if len(page1.Items) != 1 {
		t.Fatalf("expected 1 item on page 1, got %d", len(page1.Items))
	}
	if !page1.HasMore {
		t.Fatal("expected hasMore on page 1")
	}
	if page1.NextCursor == "" {
		t.Fatal("expected non-empty next cursor")
	}
	page2, domain := h.service.QueryFrames(context.Background(), targetEvidence(h.scopeID), FrameQuery{
		Handle:   h.handle,
		PageSize: 1,
		Cursor:   page1.NextCursor,
	})
	if domain != nil {
		t.Fatalf("QueryFrames page 2 failed: %v", domain)
	}
	if len(page2.Items) != 1 {
		t.Fatalf("expected 1 item on page 2, got %d", len(page2.Items))
	}
	if page2.HasMore {
		t.Fatal("expected no more pages on page 2")
	}
}

func TestServiceQueryFramesCursorFingerprintMismatch(t *testing.T) {
	h := newServiceTestHarness(t, "trace-nested-frame-usage", nestedFrameUsageTrace)
	page1, _ := h.service.QueryFrames(context.Background(), targetEvidence(h.scopeID), FrameQuery{
		Handle:   h.handle,
		PageSize: 1,
	})
	_, domain := h.service.QueryFrames(context.Background(), targetEvidence(h.scopeID), FrameQuery{
		Handle:     h.handle,
		PageSize:   1,
		Order:      FrameOrderDurationDesc,
		Projection: FrameProjectionDetailed,
		Cursor:     page1.NextCursor,
	})
	if domain == nil {
		t.Fatal("expected fingerprint mismatch error")
	}
	if domain.Code != consolecore.CodeInvalidCursor {
		t.Fatalf("expected INVALID_CURSOR, got %s", domain.Code)
	}
}

func TestServiceQueryFramesFilterByFrameType(t *testing.T) {
	h := newServiceTestHarness(t, "trace-nested-frame-usage", nestedFrameUsageTrace)
	page, domain := h.service.QueryFrames(context.Background(), targetEvidence(h.scopeID), FrameQuery{
		Handle: h.handle, Projection: FrameProjectionDetailed,
		Filter: FrameFilter{FrameType: "SKILL_EXECUTION"},
	})
	if domain != nil {
		t.Fatalf("QueryFrames failed: %v", domain)
	}
	if len(page.Items) != 1 {
		t.Fatalf("expected 1 SKILL_EXECUTION frame, got %d", len(page.Items))
	}
	if page.Items[0].FrameID != "skill" {
		t.Fatalf("expected frame ID 'skill', got %q", page.Items[0].FrameID)
	}
}

func TestRecordSummaryExposesFailureIdentity(t *testing.T) {
	record := &Record{Sequence: 12, Type: RecordStepFailed, Metadata: []byte(`{"failureId":"failure-step"}`)}
	summary := recordToSummary(record, recordIndexRow{}, TraceContext{}, RecordRepresentationLogical)
	if summary.FailureID != "failure-step" {
		t.Fatalf("failureId=%q", summary.FailureID)
	}
}

func TestServiceQueryRecordsPhysical(t *testing.T) {
	h := newServiceTestHarness(t, "trace-t", minimalValidTrace)
	page, domain := h.service.QueryRecords(context.Background(), targetEvidence(h.scopeID), RecordQuery{
		Handle:         h.handle,
		Representation: RecordRepresentationPhysical,
		PageSize:       10,
	})
	if domain != nil {
		t.Fatalf("QueryRecords failed: %v", domain)
	}
	if len(page.Items) != 5 {
		t.Fatalf("expected 5 physical records, got %d", len(page.Items))
	}
	if page.Items[0].Sequence != 1 {
		t.Fatalf("expected first sequence 1, got %d", page.Items[0].Sequence)
	}
	if page.Items[0].Type != "TRACE_STARTED" {
		t.Fatalf("expected first type TRACE_STARTED, got %q", page.Items[0].Type)
	}
}

func TestServiceQueryRecordsFilterByType(t *testing.T) {
	h := newServiceTestHarness(t, "trace-t", minimalValidTrace)
	page, domain := h.service.QueryRecords(context.Background(), targetEvidence(h.scopeID), RecordQuery{
		Handle: h.handle,
		Filter: RecordFilter{Types: []string{"MODEL_RESPONSE_RECEIVED"}},
	})
	if domain != nil {
		t.Fatalf("QueryRecords failed: %v", domain)
	}
	if len(page.Items) != 1 {
		t.Fatalf("expected 1 MODEL_RESPONSE_RECEIVED record, got %d", len(page.Items))
	}
	if page.Items[0].Sequence != 4 {
		t.Fatalf("expected sequence 4, got %d", page.Items[0].Sequence)
	}
}

func TestServiceQueryRecordsPagination(t *testing.T) {
	h := newServiceTestHarness(t, "trace-t", minimalValidTrace)
	page1, domain := h.service.QueryRecords(context.Background(), targetEvidence(h.scopeID), RecordQuery{
		Handle:   h.handle,
		PageSize: 2,
	})
	if domain != nil {
		t.Fatalf("QueryRecords page 1 failed: %v", domain)
	}
	if len(page1.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(page1.Items))
	}
	if !page1.HasMore {
		t.Fatal("expected hasMore")
	}
	page2, domain := h.service.QueryRecords(context.Background(), targetEvidence(h.scopeID), RecordQuery{
		Handle:   h.handle,
		PageSize: 2,
		Cursor:   page1.NextCursor,
	})
	if domain != nil {
		t.Fatalf("QueryRecords page 2 failed: %v", domain)
	}
	if len(page2.Items) != 2 {
		t.Fatalf("expected 2 items on page 2, got %d", len(page2.Items))
	}
	if !page2.HasMore {
		t.Fatal("expected hasMore on page 2")
	}
	page3, domain := h.service.QueryRecords(context.Background(), targetEvidence(h.scopeID), RecordQuery{
		Handle:   h.handle,
		PageSize: 2,
		Cursor:   page2.NextCursor,
	})
	if domain != nil {
		t.Fatalf("QueryRecords page 3 failed: %v", domain)
	}
	if len(page3.Items) != 1 {
		t.Fatalf("expected 1 item on page 3, got %d", len(page3.Items))
	}
	if page3.HasMore {
		t.Fatal("expected no more pages on page 3")
	}
}

func TestServiceQueryAttempts(t *testing.T) {
	h := newServiceTestHarness(t, "trace-t", minimalValidTrace)
	page, domain := h.service.QueryAttempts(context.Background(), targetEvidence(h.scopeID), AttemptQuery{
		Handle:   h.handle,
		PageSize: 10,
	})
	if domain != nil {
		t.Fatalf("QueryAttempts failed: %v", domain)
	}
	if len(page.Items) != 1 {
		t.Fatalf("expected 1 attempt, got %d", len(page.Items))
	}
	if page.Items[0].AttemptID != "attempt-1" {
		t.Fatalf("expected attempt ID 'attempt-1', got %q", page.Items[0].AttemptID)
	}
	if page.Items[0].Usage.TotalUnits != 14 {
		t.Fatalf("expected total 14, got %d", page.Items[0].Usage.TotalUnits)
	}
}

func TestServiceQueryRetries(t *testing.T) {
	h := newServiceTestHarness(t, "trace-t", minimalValidTrace)
	page, domain := h.service.QueryRetries(context.Background(), targetEvidence(h.scopeID), RetryQuery{
		Handle:   h.handle,
		PageSize: 10,
	})
	if domain != nil {
		t.Fatalf("QueryRetries failed: %v", domain)
	}
	if len(page.Items) != 1 {
		t.Fatalf("expected 1 retry, got %d", len(page.Items))
	}
	if page.Items[0].RetrySequenceID != "retry-1" {
		t.Fatalf("expected retry ID 'retry-1', got %q", page.Items[0].RetrySequenceID)
	}
}

func TestServiceQueryValidationLinks(t *testing.T) {
	h := newServiceTestHarness(t, "trace-t", minimalValidTrace)
	page, domain := h.service.QueryValidationLinks(context.Background(), targetEvidence(h.scopeID), ValidationQuery{
		Handle:   h.handle,
		PageSize: 10,
	})
	if domain != nil {
		t.Fatalf("QueryValidationLinks failed: %v", domain)
	}
	if len(page.Items) != 0 {
		t.Fatalf("expected 0 validation links, got %d", len(page.Items))
	}
}

func TestServiceQueryFailures(t *testing.T) {
	h := newServiceTestHarness(t, "trace-t", minimalValidTrace)
	page, domain := h.service.QueryFailures(context.Background(), targetEvidence(h.scopeID), FailureQuery{
		Handle:   h.handle,
		PageSize: 10,
	})
	if domain != nil {
		t.Fatalf("QueryFailures failed: %v", domain)
	}
	if len(page.Items) != 0 {
		t.Fatalf("expected 0 failures, got %d", len(page.Items))
	}
}

func TestServiceGetFailureDiagnostic(t *testing.T) {
	raw := startedRecord(1) + "\n" + errorRecord(2, "failure-diagnostic", false) + "\n" + completionRecord(3, "SUCCEEDED", 0, 0, 0, "") + "\n"
	h := newServiceTestHarness(t, "t", raw)
	page, domain := h.service.QueryFailures(context.Background(), targetEvidence(h.scopeID), FailureQuery{Handle: h.handle, PageSize: 10})
	if domain != nil || len(page.Items) != 1 || len(page.Items[0].Diagnostics) != 1 {
		t.Fatalf("failure descriptors: page=%+v domain=%v", page, domain)
	}
	ref := page.Items[0].Diagnostics[0].ContentRef
	if ref == "" {
		t.Fatal("diagnostic payload reference is empty")
	}
	first, domain := h.service.ReadContentRange(context.Background(), targetEvidence(h.scopeID), RangeRequest{Handle: h.handle, ContentRef: ref, Start: 0, MaxBytes: 2})
	if domain != nil || string(first.Content) != "st" || !first.HasMore || first.NextCursor == "" {
		t.Fatalf("first diagnostic range=%#v domain=%v", first, domain)
	}
	second, domain := h.service.ReadContentRange(context.Background(), targetEvidence(h.scopeID), RangeRequest{Handle: h.handle, ContentRef: ref, ContinueCursor: first.NextCursor, MaxBytes: 2})
	if domain != nil || string(second.Content) != "ac" || !second.HasMore {
		t.Fatalf("second diagnostic range=%#v domain=%v", second, domain)
	}
	third, domain := h.service.ReadContentRange(context.Background(), targetEvidence(h.scopeID), RangeRequest{Handle: h.handle, ContentRef: ref, ContinueCursor: second.NextCursor, MaxBytes: 2})
	if domain != nil || string(third.Content) != "k" || third.HasMore {
		t.Fatalf("third diagnostic range=%#v domain=%v", third, domain)
	}
	result, domain := h.service.GetFailureDiagnostic(context.Background(), targetEvidence(h.scopeID), FailureDiagnosticRequest{Handle: h.handle, FailureID: "failure-diagnostic", Ordinal: 0})
	if domain != nil {
		t.Fatalf("GetFailureDiagnostic: %v", domain)
	}
	if result.Text != "stack" || result.Descriptor.DecodedBytes != len([]byte(result.Text)) {
		t.Fatalf("unexpected diagnostic: %+v", result)
	}
	for _, tc := range []struct {
		name  string
		scope target.ScopeID
		req   FailureDiagnosticRequest
		ctx   context.Context
	}{
		{name: "unknown failure", scope: h.scopeID, req: FailureDiagnosticRequest{Handle: h.handle, FailureID: "missing", Ordinal: 0}, ctx: context.Background()},
		{name: "negative ordinal", scope: h.scopeID, req: FailureDiagnosticRequest{Handle: h.handle, FailureID: "failure-diagnostic", Ordinal: -1}, ctx: context.Background()},
		{name: "out of range ordinal", scope: h.scopeID, req: FailureDiagnosticRequest{Handle: h.handle, FailureID: "failure-diagnostic", Ordinal: 1}, ctx: context.Background()},
		{name: "wrong scope", scope: target.ScopeID("other-scope"), req: FailureDiagnosticRequest{Handle: h.handle, FailureID: "failure-diagnostic", Ordinal: 0}, ctx: context.Background()},
		{name: "wrong handle", scope: h.scopeID, req: FailureDiagnosticRequest{Handle: artifact.Handle("other-handle"), FailureID: "failure-diagnostic", Ordinal: 0}, ctx: context.Background()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, domain := h.service.GetFailureDiagnostic(tc.ctx, targetEvidence(tc.scope), tc.req); domain == nil {
				t.Fatal("expected domain error")
			}
		})
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, domain := h.service.GetFailureDiagnostic(cancelled, targetEvidence(h.scopeID), FailureDiagnosticRequest{Handle: h.handle, FailureID: "failure-diagnostic", Ordinal: 0}); domain == nil {
		t.Fatal("expected cancellation error")
	}
}

func TestDiagnosticRangeFailureDoesNotRefreshArtifactLastUse(t *testing.T) {
	raw := startedRecord(1) + "\n" + errorRecord(2, "failure-diagnostic", false) + "\n" + completionRecord(3, "SUCCEEDED", 0, 0, 0, "") + "\n"
	h := newServiceTestHarness(t, "t", raw)
	failures, domain := h.service.QueryFailures(context.Background(), targetEvidence(h.scopeID), FailureQuery{Handle: h.handle, PageSize: 10})
	if domain != nil || len(failures.Items) == 0 || len(failures.Items[0].Diagnostics) == 0 {
		t.Fatalf("failures=%#v domain=%v", failures, domain)
	}
	ref := failures.Items[0].Diagnostics[0].ContentRef
	before, snapshotDomain := h.artifacts.StorageSnapshot()
	if snapshotDomain != nil || len(before.Entries) != 1 {
		t.Fatalf("before=%#v domain=%v", before, snapshotDomain)
	}
	time.Sleep(time.Millisecond)
	_, domain = h.service.ReadContentRange(context.Background(), targetEvidence(h.scopeID), RangeRequest{Handle: h.handle, ContentRef: ref, Start: 1 << 20, MaxBytes: 1})
	if domain == nil || domain.Code != consolecore.CodeInvalidArgument {
		t.Fatalf("out-of-bounds diagnostic range domain=%v", domain)
	}
	after, snapshotDomain := h.artifacts.StorageSnapshot()
	if snapshotDomain != nil || len(after.Entries) != 1 {
		t.Fatalf("after=%#v domain=%v", after, snapshotDomain)
	}
	if !after.Entries[0].LastUsedAt.Equal(before.Entries[0].LastUsedAt) {
		t.Fatalf("rejected range refreshed last use: before=%s after=%s", before.Entries[0].LastUsedAt, after.Entries[0].LastUsedAt)
	}
}

func TestServiceQueryPayloads(t *testing.T) {
	h := newServiceTestHarness(t, "trace-t", minimalValidTrace)
	page, domain := h.service.QueryPayloads(context.Background(), targetEvidence(h.scopeID), PayloadQuery{
		Handle:   h.handle,
		PageSize: 10,
	})
	if domain != nil {
		t.Fatalf("QueryPayloads failed: %v", domain)
	}
	if len(page.Items) != 0 {
		t.Fatalf("expected 0 payloads, got %d", len(page.Items))
	}
}

func TestServiceQueryGaps(t *testing.T) {
	h := newServiceTestHarness(t, "trace-t", minimalValidTrace)
	page, domain := h.service.QueryGaps(context.Background(), targetEvidence(h.scopeID), GapQuery{
		Handle:   h.handle,
		PageSize: 10,
	})
	if domain != nil {
		t.Fatalf("QueryGaps failed: %v", domain)
	}
	if len(page.Items) != 0 {
		t.Fatalf("expected 0 gaps, got %d", len(page.Items))
	}
}

func TestServiceGetUsageBreakdown(t *testing.T) {
	h := newServiceTestHarness(t, "trace-t", minimalValidTrace)
	breakdown, domain := h.service.GetUsageBreakdown(context.Background(), targetEvidence(h.scopeID), h.handle)
	if domain != nil {
		t.Fatalf("GetUsageBreakdown failed: %v", domain)
	}
	if breakdown.Attributed.TotalUnits != 14 {
		t.Fatalf("expected attributed total 14, got %d", breakdown.Attributed.TotalUnits)
	}
	if breakdown.Terminal.TotalUnits != 14 {
		t.Fatalf("expected terminal total 14, got %d", breakdown.Terminal.TotalUnits)
	}
}

func TestServiceQueryFramesExpiredHandle(t *testing.T) {
	h := newServiceTestHarness(t, "trace-t", minimalValidTrace)
	_, domain := h.service.QueryFrames(context.Background(), targetEvidence(h.scopeID), FrameQuery{
		Handle: artifact.Handle("nonexistent-handle"),
	})
	if domain == nil {
		t.Fatal("expected error for nonexistent handle")
	}
}

func TestServiceQueryRecordsLogicalSkipsChunks(t *testing.T) {
	// Use the existing chunkedPayloadTrace test helper which produces a valid
	// chunked-payload trace with trace ID "t" and session ID "s".
	ndjson := chunkedPayloadTrace(256, 2)
	h := newServiceTestHarness(t, "t", ndjson)
	physicalPage, domain := h.service.QueryRecords(context.Background(), targetEvidence(h.scopeID), RecordQuery{
		Handle:         h.handle,
		Representation: RecordRepresentationPhysical,
		PageSize:       100,
	})
	if domain != nil {
		t.Fatalf("QueryRecords physical failed: %v", domain)
	}
	hasChunk := false
	for _, r := range physicalPage.Items {
		if r.IsChunk {
			hasChunk = true
			break
		}
	}
	if !hasChunk {
		t.Fatal("expected at least one chunk record in physical representation")
	}
	logicalPage, domain := h.service.QueryRecords(context.Background(), targetEvidence(h.scopeID), RecordQuery{
		Handle:         h.handle,
		Representation: RecordRepresentationLogical,
		PageSize:       100,
	})
	if domain != nil {
		t.Fatalf("QueryRecords logical failed: %v", domain)
	}
	for _, r := range logicalPage.Items {
		if r.IsChunk {
			t.Fatal("expected no chunk records in logical representation")
		}
	}
}

func TestEnrichedRecordsAttachOnlyOwnedFactsAndKeepArraysNonNil(t *testing.T) {
	h := newServiceTestHarness(t, "trace-t", minimalValidTrace)
	page, domain := h.service.QueryRecords(context.Background(), targetEvidence(h.scopeID), RecordQuery{Handle: h.handle, Representation: RecordRepresentationPhysical, PageSize: 100})
	if domain != nil {
		t.Fatal(domain)
	}
	var response *RecordSummary
	for index := range page.Items {
		record := &page.Items[index]
		if record.Facts.Attempts == nil || record.Facts.Retries == nil || record.Facts.Validations == nil || record.Facts.Failures == nil || record.Facts.Payloads == nil || record.Facts.SearchMatches == nil {
			t.Fatalf("nil facts on sequence %d: %#v", record.Sequence, record.Facts)
		}
		if record.Type == string(RecordModelResponseReceived) {
			response = record
		}
		if record.Type == string(RecordTraceStarted) && (len(record.Facts.Attempts)+len(record.Facts.Retries)+len(record.Facts.Validations)+len(record.Facts.Failures)+len(record.Facts.Payloads)) != 0 {
			t.Fatalf("unowned facts on trace start: %#v", record.Facts)
		}
	}
	if response == nil || len(response.Facts.Attempts) != 1 || len(response.Facts.Retries) != 1 || response.Facts.Attempts[0].AttemptID == "" {
		t.Fatalf("response facts=%#v", response)
	}
}

func TestEnrichedRecordTraversalEmitsRetryAggregateOnce(t *testing.T) {
	framed := func(record string) string { return strings.Replace(record, `"frameId":null`, `"frameId":"frame"`, 1) }
	raw := strings.Join([]string{
		startedRecord(1), frameRecord(2, "frame", "", false, "ROOT_MISSION", true),
		framed(requestRecord(3, "retry-1", "attempt-1", 1, true)),
		framed(requestRecord(4, "retry-1", "attempt-1", 1, false)),
		responseRecord(5, "frame", "retry-1", "attempt-1", 1, 1, 1, 2, "EXACT"),
		framed(requestRecord(6, "retry-1", "attempt-2", 2, true)),
		framed(requestRecord(7, "retry-1", "attempt-2", 2, false)),
		responseRecord(8, "frame", "retry-1", "attempt-2", 2, 1, 1, 2, "EXACT"),
		frameRecord(9, "frame", "", false, "ROOT_MISSION", false), completionRecord(10, "SUCCEEDED", 2, 2, 4, ""),
	}, "\n") + "\n"
	h := newServiceTestHarness(t, "t", raw)
	page, domain := h.service.QueryRecords(context.Background(), targetEvidence(h.scopeID), RecordQuery{Handle: h.handle, Representation: RecordRepresentationPhysical, PageSize: 100})
	if domain != nil {
		t.Fatal(domain)
	}
	attempts, retries := 0, 0
	var retryOwner int64
	for _, record := range page.Items {
		attempts += len(record.Facts.Attempts)
		if len(record.Facts.Retries) > 0 {
			retries += len(record.Facts.Retries)
			retryOwner = record.Sequence
		}
	}
	if attempts != 2 || retries != 1 || retryOwner != 8 {
		t.Fatalf("attempts=%d retries=%d retryOwner=%d", attempts, retries, retryOwner)
	}
}

func TestFramesAttachAttemptAttributedMissingResponseGap(t *testing.T) {
	framed := func(record string) string { return strings.Replace(record, `"frameId":null`, `"frameId":"frame"`, 1) }
	raw := strings.Join([]string{
		startedRecord(1), frameRecord(2, "frame", "", false, "ROOT_MISSION", true),
		framed(requestRecord(3, "retry-1", "attempt-1", 1, true)),
		framed(requestRecord(4, "retry-1", "attempt-1", 1, false)),
		frameRecord(5, "frame", "", false, "ROOT_MISSION", false), completionRecord(6, "SUCCEEDED", 0, 0, 0, ""),
	}, "\n") + "\n"
	h := newServiceTestHarness(t, "t", raw)
	page, domain := h.service.QueryFrames(context.Background(), targetEvidence(h.scopeID), FrameQuery{Handle: h.handle, Projection: FrameProjectionDetailed, PageSize: 10})
	if domain != nil {
		t.Fatal(domain)
	}
	if len(page.Items) != 1 || !containsString(page.Items[0].GapKinds, "MODEL_ATTEMPT_RESPONSE_MISSING") {
		t.Fatalf("frames=%#v", page.Items)
	}
}

func TestRecordFactAddressIndexSupportsPageSizeOneTraversal(t *testing.T) {
	h := newServiceTestHarness(t, "trace-t", minimalValidTrace)
	continuation := ""
	foundAttempt := false
	visited := 0
	for {
		page, domain := h.service.QueryRecords(context.Background(), targetEvidence(h.scopeID), RecordQuery{Handle: h.handle, Representation: RecordRepresentationPhysical, PageSize: 1, Cursor: continuation})
		if domain != nil {
			t.Fatal(domain)
		}
		if len(page.Items) != 1 {
			t.Fatalf("page=%#v", page)
		}
		visited++
		if len(page.Items[0].Facts.Attempts) > 0 {
			foundAttempt = true
		}
		if !page.HasMore {
			break
		}
		continuation = page.NextCursor
	}
	lease, domain := h.artifacts.Use(targetEvidence(h.scopeID), h.handle)
	if domain != nil {
		t.Fatal(domain)
	}
	defer lease.Close(false)
	recordIndexSize, err := lease.ComponentSize(artifact.ComponentName(ComponentRecordIndex))
	if err != nil {
		t.Fatal(err)
	}
	factIndexSize, err := lease.ComponentSize(artifact.ComponentName(ComponentRecordFactIdx))
	if err != nil {
		t.Fatal(err)
	}
	recordCount := recordIndexSize / recordIndexRowWidth
	if !foundAttempt || int64(visited) != recordCount || factIndexSize != recordCount*recordFactIndexRowWidth {
		t.Fatalf("visited=%d records=%d factIndexSize=%d foundAttempt=%v", visited, recordCount, factIndexSize, foundAttempt)
	}
}

func TestFactRowScanObservesCancellationBetweenRows(t *testing.T) {
	h := newServiceTestHarness(t, "trace-t", minimalValidTrace)
	lease, domain := h.artifacts.Use(targetEvidence(h.scopeID), h.handle)
	if domain != nil {
		t.Fatal(domain)
	}
	defer lease.Close(false)
	ctx, cancel := context.WithCancel(context.Background())
	visits := 0
	err := scanFactRowsContext[attemptResult](ctx, lease, ComponentAttemptIndex, 0, func(attemptResult, int64) bool {
		visits++
		cancel()
		return false
	})
	if !errors.Is(err, context.Canceled) || visits != 1 {
		t.Fatalf("err=%v visits=%d", err, visits)
	}
}

func TestFramesAttachDirectGapKindsWithoutPropagation(t *testing.T) {
	raw := startedRecord(1) + "\n" + frameRecord(2, "open", "", false, "ROOT_MISSION", true) + "\n" + completionRecord(3, "SUCCEEDED", 0, 0, 0, "") + "\n"
	h := newServiceTestHarness(t, "t", raw)
	page, domain := h.service.QueryFrames(context.Background(), targetEvidence(h.scopeID), FrameQuery{Handle: h.handle, Projection: FrameProjectionDetailed, PageSize: 10})
	if domain != nil {
		t.Fatal(domain)
	}
	if len(page.Items) != 1 || page.Items[0].FrameID != "open" || len(page.Items[0].GapKinds) == 0 {
		t.Fatalf("frames=%#v", page.Items)
	}
	if page.Items[0].UncertaintyKinds == nil {
		t.Fatalf("uncertainty kinds are nil: %#v", page.Items[0])
	}
}

// nestedFrameUsageTrace is a trace with two frames (root + skill) matching the
// Java fixture corpus nested-frame-usage case.
const nestedFrameUsageTrace = `{"traceId":"trace-nested-frame-usage","sessionId":"session-nested-frame-usage","sequence":1,"timestamp":1784894400.000000000,"recordType":"TRACE_STARTED","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"fixture-thread","metadata":{"tracePath":"traces/nested-frame-usage.ndjson","consoleCompatibilityVersion":"development"},"data":{"sessionId":"session-nested-frame-usage"}}
{"traceId":"trace-nested-frame-usage","sessionId":"session-nested-frame-usage","sequence":2,"timestamp":1784894400.000000000,"recordType":"TRACE_CAPTURE_POLICY_RECORDED","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"fixture-thread","metadata":{"persistencePolicy":"ALWAYS"},"data":null}
{"traceId":"trace-nested-frame-usage","sessionId":"session-nested-frame-usage","sequence":3,"timestamp":1784894400.000000000,"recordType":"FRAME_OPENED","frameId":"root","parentFrameId":null,"frameType":"ROOT_MISSION","route":"root.skill","threadName":"fixture-thread","metadata":{"timestampOverride":"2026-07-24T12:00:00Z"},"data":null}
{"traceId":"trace-nested-frame-usage","sessionId":"session-nested-frame-usage","sequence":4,"timestamp":1784894401.000000000,"recordType":"FRAME_OPENED","frameId":"skill","parentFrameId":"root","frameType":"SKILL_EXECUTION","route":"root.skill","threadName":"fixture-thread","metadata":{"timestampOverride":"2026-07-24T12:00:01Z"},"data":null}
{"traceId":"trace-nested-frame-usage","sessionId":"session-nested-frame-usage","sequence":5,"timestamp":1784894400.000000000,"recordType":"MODEL_THOUGHT_CAPTURED","frameId":"skill","parentFrameId":"root","frameType":"SKILL_EXECUTION","route":"root.skill","threadName":"fixture-thread","metadata":{"retrySequenceId":"retry-framed","attemptId":"attempt-framed","attemptNumber":1,"attemptReason":"INITIAL","providerAttemptNumber":1},"data":{"messages":["user"]}}
{"traceId":"trace-nested-frame-usage","sessionId":"session-nested-frame-usage","sequence":6,"timestamp":1784894400.000000000,"recordType":"MODEL_REQUEST_SENT","frameId":"skill","parentFrameId":"root","frameType":"SKILL_EXECUTION","route":"root.skill","threadName":"fixture-thread","metadata":{"retrySequenceId":"retry-framed","attemptId":"attempt-framed","attemptNumber":1,"attemptReason":"INITIAL","providerAttemptNumber":1},"data":{"messages":["user"]}}
{"traceId":"trace-nested-frame-usage","sessionId":"session-nested-frame-usage","sequence":7,"timestamp":1784894402.000000000,"recordType":"MODEL_RESPONSE_RECEIVED","frameId":"skill","parentFrameId":"root","frameType":"SKILL_EXECUTION","route":"root.skill","threadName":"fixture-thread","metadata":{"retrySequenceId":"retry-framed","attemptId":"attempt-framed","attemptNumber":1,"attemptReason":"INITIAL","providerAttemptNumber":1,"usage":{"promptUnits":4,"completionUnits":2,"totalUnits":6,"precision":"EXACT"}},"data":{"content":"fixture response"}}
{"traceId":"trace-nested-frame-usage","sessionId":"session-nested-frame-usage","sequence":8,"timestamp":1784894403.000000000,"recordType":"FRAME_CLOSED","frameId":"skill","parentFrameId":"root","frameType":"SKILL_EXECUTION","route":"root.skill","threadName":"fixture-thread","metadata":{},"data":null}
{"traceId":"trace-nested-frame-usage","sessionId":"session-nested-frame-usage","sequence":9,"timestamp":1784894404.000000000,"recordType":"FRAME_CLOSED","frameId":"root","parentFrameId":null,"frameType":"ROOT_MISSION","route":"root.skill","threadName":"fixture-thread","metadata":{},"data":null}
{"traceId":"trace-nested-frame-usage","sessionId":"session-nested-frame-usage","sequence":10,"timestamp":1784894400.000000000,"recordType":"MODEL_THOUGHT_CAPTURED","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"fixture-thread","metadata":{"retrySequenceId":"retry-unframed","attemptId":"attempt-unframed","attemptNumber":1,"attemptReason":"INITIAL","providerAttemptNumber":1},"data":{"messages":["user"]}}
{"traceId":"trace-nested-frame-usage","sessionId":"session-nested-frame-usage","sequence":11,"timestamp":1784894400.000000000,"recordType":"MODEL_REQUEST_SENT","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"fixture-thread","metadata":{"retrySequenceId":"retry-unframed","attemptId":"attempt-unframed","attemptNumber":1,"attemptReason":"INITIAL","providerAttemptNumber":1},"data":{"messages":["user"]}}
{"traceId":"trace-nested-frame-usage","sessionId":"session-nested-frame-usage","sequence":12,"timestamp":1784894400.000000000,"recordType":"MODEL_RESPONSE_RECEIVED","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"fixture-thread","metadata":{"retrySequenceId":"retry-unframed","attemptId":"attempt-unframed","attemptNumber":1,"attemptReason":"INITIAL","providerAttemptNumber":1,"usage":{"promptUnits":1,"completionUnits":1,"totalUnits":2,"precision":"EXACT"}},"data":{"content":"fixture response"}}
{"traceId":"trace-nested-frame-usage","sessionId":"session-nested-frame-usage","sequence":13,"timestamp":1784894400.000000000,"recordType":"TRACE_COMPLETED","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"fixture-thread","metadata":{"outcome":"SUCCEEDED","sessionUsageSnapshot":{"promptUnits":5,"completionUnits":3,"totalUnits":8},"errored":false,"persistencePolicy":"ALWAYS"},"data":null}
`

// chunkedPayloadTraceForService is reserved for future use; the existing
// chunkedPayloadTrace test helper is used instead.
