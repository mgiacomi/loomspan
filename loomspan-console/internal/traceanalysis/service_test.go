package traceanalysis

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
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
	artifactSvc, err := artifact.New(artifact.Config{
		MaxBytes: 10 << 20,
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
	if summary.AttemptCount != 1 {
		t.Fatalf("expected 1 attempt, got %d", summary.AttemptCount)
	}
	if summary.RetryCount != 1 {
		t.Fatalf("expected 1 retry, got %d", summary.RetryCount)
	}
	if !summary.UsageComplete {
		t.Fatal("expected usage complete")
	}
	if summary.AttributedUsage.TotalUnits != 14 {
		t.Fatalf("expected attributed total 14, got %d", summary.AttributedUsage.TotalUnits)
	}
}

func TestServiceQueryFramesEmpty(t *testing.T) {
	h := newServiceTestHarness(t, "trace-t", minimalValidTrace)
	page, domain := h.service.QueryFrames(context.Background(), targetEvidence(h.scopeID), FrameQuery{
		Handle:   h.handle,
		PageSize: 10,
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
		Handle:   h.handle,
		PageSize: 10,
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
		Handle: h.handle,
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
		!reflect.DeepEqual(child.Outcomes, []string{"completed"}) ||
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

	all, domain := h.service.QueryFrames(context.Background(), targetEvidence(h.scopeID), FrameQuery{Handle: h.handle, PageSize: 1})
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
		Handle: h.handle, PageSize: 1, Cursor: all.NextCursor,
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
		Handle:   h.handle,
		PageSize: 1,
		Order:    FrameOrderDurationDesc,
		Cursor:   page1.NextCursor,
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
		Handle: h.handle,
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

// nestedFrameUsageTrace is a trace with two frames (root + skill) matching the
// Java fixture corpus nested-frame-usage case.
const nestedFrameUsageTrace = `{"traceId":"trace-nested-frame-usage","sessionId":"session-nested-frame-usage","sequence":1,"timestamp":1784894400.000000000,"recordType":"TRACE_STARTED","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"fixture-thread","metadata":{"tracePath":"traces/nested-frame-usage.ndjson","consoleCompatibilityVersion":"development"},"data":{"sessionId":"session-nested-frame-usage"}}
{"traceId":"trace-nested-frame-usage","sessionId":"session-nested-frame-usage","sequence":2,"timestamp":1784894400.000000000,"recordType":"TRACE_CAPTURE_POLICY_RECORDED","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"fixture-thread","metadata":{"persistencePolicy":"ALWAYS"},"data":null}
{"traceId":"trace-nested-frame-usage","sessionId":"session-nested-frame-usage","sequence":3,"timestamp":1784894400.000000000,"recordType":"FRAME_OPENED","frameId":"root","parentFrameId":null,"frameType":"ROOT_MISSION","route":"root.skill","threadName":"fixture-thread","metadata":{"timestampOverride":"2026-07-24T12:00:00Z"},"data":null}
{"traceId":"trace-nested-frame-usage","sessionId":"session-nested-frame-usage","sequence":4,"timestamp":1784894401.000000000,"recordType":"FRAME_OPENED","frameId":"skill","parentFrameId":"root","frameType":"SKILL_EXECUTION","route":"root.skill","threadName":"fixture-thread","metadata":{"timestampOverride":"2026-07-24T12:00:01Z"},"data":null}
{"traceId":"trace-nested-frame-usage","sessionId":"session-nested-frame-usage","sequence":5,"timestamp":1784894400.000000000,"recordType":"MODEL_REQUEST_PREPARED","frameId":"skill","parentFrameId":"root","frameType":"SKILL_EXECUTION","route":"root.skill","threadName":"fixture-thread","metadata":{"retrySequenceId":"retry-framed","attemptId":"attempt-framed","attemptNumber":1,"attemptReason":"INITIAL","providerAttemptNumber":1},"data":{"messages":["user"]}}
{"traceId":"trace-nested-frame-usage","sessionId":"session-nested-frame-usage","sequence":6,"timestamp":1784894400.000000000,"recordType":"MODEL_REQUEST_SENT","frameId":"skill","parentFrameId":"root","frameType":"SKILL_EXECUTION","route":"root.skill","threadName":"fixture-thread","metadata":{"retrySequenceId":"retry-framed","attemptId":"attempt-framed","attemptNumber":1,"attemptReason":"INITIAL","providerAttemptNumber":1},"data":{"messages":["user"]}}
{"traceId":"trace-nested-frame-usage","sessionId":"session-nested-frame-usage","sequence":7,"timestamp":1784894402.000000000,"recordType":"MODEL_RESPONSE_RECEIVED","frameId":"skill","parentFrameId":"root","frameType":"SKILL_EXECUTION","route":"root.skill","threadName":"fixture-thread","metadata":{"retrySequenceId":"retry-framed","attemptId":"attempt-framed","attemptNumber":1,"attemptReason":"INITIAL","providerAttemptNumber":1,"usage":{"promptUnits":4,"completionUnits":2,"totalUnits":6,"precision":"EXACT"}},"data":{"content":"fixture response"}}
{"traceId":"trace-nested-frame-usage","sessionId":"session-nested-frame-usage","sequence":8,"timestamp":1784894403.000000000,"recordType":"FRAME_CLOSED","frameId":"skill","parentFrameId":"root","frameType":"SKILL_EXECUTION","route":"root.skill","threadName":"fixture-thread","metadata":{},"data":null}
{"traceId":"trace-nested-frame-usage","sessionId":"session-nested-frame-usage","sequence":9,"timestamp":1784894404.000000000,"recordType":"FRAME_CLOSED","frameId":"root","parentFrameId":null,"frameType":"ROOT_MISSION","route":"root.skill","threadName":"fixture-thread","metadata":{},"data":null}
{"traceId":"trace-nested-frame-usage","sessionId":"session-nested-frame-usage","sequence":10,"timestamp":1784894400.000000000,"recordType":"MODEL_REQUEST_PREPARED","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"fixture-thread","metadata":{"retrySequenceId":"retry-unframed","attemptId":"attempt-unframed","attemptNumber":1,"attemptReason":"INITIAL","providerAttemptNumber":1},"data":{"messages":["user"]}}
{"traceId":"trace-nested-frame-usage","sessionId":"session-nested-frame-usage","sequence":11,"timestamp":1784894400.000000000,"recordType":"MODEL_REQUEST_SENT","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"fixture-thread","metadata":{"retrySequenceId":"retry-unframed","attemptId":"attempt-unframed","attemptNumber":1,"attemptReason":"INITIAL","providerAttemptNumber":1},"data":{"messages":["user"]}}
{"traceId":"trace-nested-frame-usage","sessionId":"session-nested-frame-usage","sequence":12,"timestamp":1784894400.000000000,"recordType":"MODEL_RESPONSE_RECEIVED","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"fixture-thread","metadata":{"retrySequenceId":"retry-unframed","attemptId":"attempt-unframed","attemptNumber":1,"attemptReason":"INITIAL","providerAttemptNumber":1,"usage":{"promptUnits":1,"completionUnits":1,"totalUnits":2,"precision":"EXACT"}},"data":{"content":"fixture response"}}
{"traceId":"trace-nested-frame-usage","sessionId":"session-nested-frame-usage","sequence":13,"timestamp":1784894400.000000000,"recordType":"TRACE_COMPLETED","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"fixture-thread","metadata":{"outcome":"SUCCEEDED","sessionUsageSnapshot":{"promptUnits":5,"completionUnits":3,"totalUnits":8},"errored":false,"persistencePolicy":"ALWAYS"},"data":null}
`

// chunkedPayloadTraceForService is reserved for future use; the existing
// chunkedPayloadTrace test helper is used instead.
