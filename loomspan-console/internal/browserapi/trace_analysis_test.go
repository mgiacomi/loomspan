package browserapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/evidence"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/target"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/traceanalysis"
)

type fakeTraceAnalysisService struct {
	summary           traceanalysis.TraceSummary
	summaryErr        *consolecore.Error
	frameQuery        traceanalysis.FrameQuery
	framePage         traceanalysis.Page[traceanalysis.FrameSummary]
	recordPage        traceanalysis.Page[traceanalysis.RecordSummary]
	failurePage       traceanalysis.Page[traceanalysis.FailureSummary]
	diagnosticScope   evidence.Reference
	diagnosticRequest traceanalysis.FailureDiagnosticRequest
	diagnosticResult  traceanalysis.FailureDiagnostic
	diagnosticErr     *consolecore.Error
	searchQuery       traceanalysis.SearchQuery
	searchPage        traceanalysis.SearchPage
	payloadRangeQuery traceanalysis.RangeRequest
	rawRangeQuery     traceanalysis.RangeRequest
	rangeResult       traceanalysis.ByteRangeResult
}

func (f *fakeTraceAnalysisService) GetSummary(_ context.Context, _ evidence.Reference, _ traceanalysis.SummaryRequest) (traceanalysis.TraceSummary, *consolecore.Error) {
	return f.summary, f.summaryErr
}
func (f *fakeTraceAnalysisService) QueryFrames(_ context.Context, _ evidence.Reference, q traceanalysis.FrameQuery) (traceanalysis.Page[traceanalysis.FrameSummary], *consolecore.Error) {
	f.frameQuery = q
	if f.framePage.Items != nil {
		return f.framePage, nil
	}
	return traceanalysis.Page[traceanalysis.FrameSummary]{Items: []traceanalysis.FrameSummary{}}, nil
}

func (f *fakeTraceAnalysisService) QueryRecords(context.Context, evidence.Reference, traceanalysis.RecordQuery) (traceanalysis.Page[traceanalysis.RecordSummary], *consolecore.Error) {
	if f.recordPage.Items != nil {
		return f.recordPage, nil
	}
	return traceanalysis.Page[traceanalysis.RecordSummary]{Items: []traceanalysis.RecordSummary{}}, nil
}
func (f *fakeTraceAnalysisService) QueryAttempts(context.Context, evidence.Reference, traceanalysis.AttemptQuery) (traceanalysis.Page[traceanalysis.AttemptSummary], *consolecore.Error) {
	return traceanalysis.Page[traceanalysis.AttemptSummary]{Items: []traceanalysis.AttemptSummary{}}, nil
}
func (f *fakeTraceAnalysisService) QueryRetries(context.Context, evidence.Reference, traceanalysis.RetryQuery) (traceanalysis.Page[traceanalysis.RetrySummary], *consolecore.Error) {
	return traceanalysis.Page[traceanalysis.RetrySummary]{Items: []traceanalysis.RetrySummary{}}, nil
}
func (f *fakeTraceAnalysisService) QueryValidationLinks(context.Context, evidence.Reference, traceanalysis.ValidationQuery) (traceanalysis.Page[traceanalysis.ValidationSummary], *consolecore.Error) {
	return traceanalysis.Page[traceanalysis.ValidationSummary]{Items: []traceanalysis.ValidationSummary{}}, nil
}
func (f *fakeTraceAnalysisService) QueryFailures(context.Context, evidence.Reference, traceanalysis.FailureQuery) (traceanalysis.Page[traceanalysis.FailureSummary], *consolecore.Error) {
	if f.failurePage.Items != nil {
		return f.failurePage, nil
	}
	return traceanalysis.Page[traceanalysis.FailureSummary]{Items: []traceanalysis.FailureSummary{}}, nil
}
func (f *fakeTraceAnalysisService) GetFailureDiagnostic(_ context.Context, scope evidence.Reference, request traceanalysis.FailureDiagnosticRequest) (traceanalysis.FailureDiagnostic, *consolecore.Error) {
	f.diagnosticScope = scope
	f.diagnosticRequest = request
	return f.diagnosticResult, f.diagnosticErr
}
func (f *fakeTraceAnalysisService) QueryPayloads(context.Context, evidence.Reference, traceanalysis.PayloadQuery) (traceanalysis.Page[traceanalysis.PayloadDescriptor], *consolecore.Error) {
	return traceanalysis.Page[traceanalysis.PayloadDescriptor]{Items: []traceanalysis.PayloadDescriptor{}}, nil
}
func (f *fakeTraceAnalysisService) QueryGaps(context.Context, evidence.Reference, traceanalysis.GapQuery) (traceanalysis.Page[traceanalysis.Gap], *consolecore.Error) {
	return traceanalysis.Page[traceanalysis.Gap]{Items: []traceanalysis.Gap{}}, nil
}
func (f *fakeTraceAnalysisService) QueryUncertainties(context.Context, evidence.Reference, traceanalysis.UncertaintyQuery) (traceanalysis.Page[traceanalysis.Uncertainty], *consolecore.Error) {
	return traceanalysis.Page[traceanalysis.Uncertainty]{Items: []traceanalysis.Uncertainty{}}, nil
}
func (f *fakeTraceAnalysisService) GetUsageBreakdown(context.Context, evidence.Reference, artifact.Handle) (traceanalysis.UsageBreakdown, *consolecore.Error) {
	return traceanalysis.UsageBreakdown{}, nil
}
func (f *fakeTraceAnalysisService) Search(_ context.Context, _ evidence.Reference, q traceanalysis.SearchQuery) (traceanalysis.SearchPage, *consolecore.Error) {
	f.searchQuery = q
	if f.searchPage.Items != nil {
		return f.searchPage, nil
	}
	return traceanalysis.SearchPage{Items: []traceanalysis.SearchResult{}, ContentDescriptors: []traceanalysis.SearchContentDescriptor{}}, nil
}
func (f *fakeTraceAnalysisService) ReadContentRange(_ context.Context, _ evidence.Reference, q traceanalysis.RangeRequest) (traceanalysis.ByteRangeResult, *consolecore.Error) {
	f.payloadRangeQuery = q
	return f.rangeResult, nil
}
func (f *fakeTraceAnalysisService) ReadRawRecordRange(_ context.Context, _ evidence.Reference, q traceanalysis.RangeRequest) (traceanalysis.ByteRangeResult, *consolecore.Error) {
	f.rawRangeQuery = q
	return f.rangeResult, nil
}

func traceAnalysisRouter(t *testing.T) (*Router, string, *http.Cookie, *fakeTraceAnalysisService) {
	t.Helper()
	fake := &fakeTraceAnalysisService{summary: traceanalysis.TraceSummary{Context: traceanalysis.TraceContext{TraceID: "trace-1", SessionID: "session-1"}, Outcome: "FAILED", RootFrameIDs: []string{}, UsageComplete: true}}
	router, tab, cookie := artifactTestRouter(t, &fakeArtifactService{lookupResult: artifact.LookupResult{Handle: "opaque-handle", LocalAvailable: true}})
	router.options.TraceAnalysis = fake
	return router, tab, cookie, fake
}
func traceAnalysisRequest(router *Router, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7943"+path, strings.NewReader(body))
	r.Host = "127.0.0.1:7943"
	r.Header.Set("Origin", "http://127.0.0.1:7943")
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}

func TestTraceAnalysisSummaryReturnsScopedProjectionForInstalledArtifact(t *testing.T) {
	router, _, cookie, _ := traceAnalysisRouter(t)
	w := traceAnalysisRequest(router, "/api/console/v1/traces/analysis/summary", `{"source":"TARGET","traceId":"trace-1"}`, cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, forbidden := range []string{"opaque-handle", "artifactHandle", "installedDir"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("response leaked %q: %s", forbidden, body)
		}
	}
	if !strings.Contains(body, `"targetScopeId":"scope-1"`) {
		t.Errorf("missing scope: %s", body)
	}
}
func TestTraceAnalysisRoutesRequireSessionButNotCSRF(t *testing.T) {
	router, _, cookie, _ := traceAnalysisRouter(t)
	w := traceAnalysisRequest(router, "/api/console/v1/traces/analysis/summary", `{"source":"TARGET","traceId":"trace-1"}`, cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("read route status=%d", w.Code)
	}
	unauth := traceAnalysisRequest(router, "/api/console/v1/traces/analysis/summary", `{"source":"TARGET","traceId":"trace-1"}`, nil)
	if unauth.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d", unauth.Code)
	}
}
func TestTraceAnalysisFramesForwardCursorAndRejectMalformedBody(t *testing.T) {
	router, _, cookie, fake := traceAnalysisRouter(t)
	w := traceAnalysisRequest(router, "/api/console/v1/traces/analysis/frames", `{"source":"TARGET","traceId":"trace-1","pageSize":25,"cursor":"cursor-1","order":"CANONICAL"}`, cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	if fake.frameQuery.Cursor != "cursor-1" || fake.frameQuery.PageSize != 25 {
		t.Fatalf("query=%+v", fake.frameQuery)
	}
	bad := traceAnalysisRequest(router, "/api/console/v1/traces/analysis/frames", `{"source":"TARGET","traceId":"trace-1","unknown":true}`, cookie)
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("malformed status=%d", bad.Code)
	}
}
func TestTraceAnalysisDoesNotRegisterRawArtifactRange(t *testing.T) {
	router, _, cookie, _ := traceAnalysisRouter(t)
	w := traceAnalysisRequest(router, "/api/console/v1/traces/analysis/raw-artifact-range", `{"source":"TARGET","traceId":"trace-1"}`, cookie)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestTraceAnalysisRejectsUnavailableLocalArtifact(t *testing.T) {
	fake := &fakeArtifactService{lookupResult: artifact.LookupResult{LocalAvailable: false}}
	router, _, cookie := artifactTestRouter(t, fake)
	router.options.TraceAnalysis = &fakeTraceAnalysisService{}
	w := traceAnalysisRequest(router, "/api/console/v1/traces/analysis/summary", `{"source":"TARGET","traceId":"trace-1"}`, cookie)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestTraceAnalysisFramesPreserveTimingUsageAndUnknownValues(t *testing.T) {
	router, _, cookie, fake := traceAnalysisRouter(t)
	opened, closed, duration := int64(100), int64(112), int64(12)
	fake.framePage = traceanalysis.Page[traceanalysis.FrameSummary]{Items: []traceanalysis.FrameSummary{{
		FrameID: "frame-1", ChildFrameIDs: []string{}, FrameType: "SKILL", OpenedTimestampMillis: opened,
		ClosedTimestampMillis: &closed, InclusiveDurationMillis: &duration, SelfDurationMillis: nil,
		DirectUsage: traceanalysis.Usage{PromptUnits: 3, CompletionUnits: 2, TotalUnits: 5}, DirectUsageComplete: false,
		InclusiveUsage: traceanalysis.Usage{PromptUnits: 3, CompletionUnits: 2, TotalUnits: 5}, InclusiveUsageComplete: false,
		SkillNames: []string{"registered.skill"}, Outcomes: []string{"FAILED"}, AttemptIDs: []string{"attempt-1"},
		RetrySequenceIDs: []string{"retry-1"}, ValidationStatuses: []string{"exhausted"}, FailureIDs: []string{"failure-1"},
	}}}
	w := traceAnalysisRequest(router, "/api/console/v1/traces/analysis/frames", `{"source":"TARGET","traceId":"trace-1","pageSize":10,"projection":"DETAILED"}`, cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	for _, expected := range []string{`"openedTimestampMillis":100`, `"closedTimestampMillis":112`, `"selfDurationMillis":null`, `"directUsage":{"promptUnits":3,"completionUnits":2,"totalUnits":5}`, `"directUsageComplete":false`, `"skillNames":["registered.skill"]`, `"attemptIds":["attempt-1"]`, `"failureIds":["failure-1"]`} {
		if !strings.Contains(w.Body.String(), expected) {
			t.Errorf("missing %s in %s", expected, w.Body.String())
		}
	}
}

func TestTraceAnalysisCompactFramesRetainHierarchyAndCountsOnly(t *testing.T) {
	router, _, cookie, fake := traceAnalysisRouter(t)
	duration := int64(12)
	fake.framePage = traceanalysis.Page[traceanalysis.FrameSummary]{Items: []traceanalysis.FrameSummary{{
		FrameID: "root", ChildFrameIDs: []string{"child"}, FrameType: "ROOT_MISSION", Route: "root",
		InclusiveDurationMillis: &duration, DirectUsage: traceanalysis.Usage{TotalUnits: 5}, SkillNames: []string{"skill"},
		AttemptIDs: []string{"attempt"}, FailureIDs: []string{"failure"}, DirectAttemptCount: 1, DirectFailureCount: 1,
	}}}
	w := traceAnalysisRequest(router, "/api/console/v1/traces/analysis/frames", `{"source":"TARGET","traceId":"trace-1","pageSize":10}`, cookie)
	if w.Code != http.StatusOK || fake.frameQuery.Projection != "" {
		t.Fatalf("status=%d query=%+v body=%s", w.Code, fake.frameQuery, w.Body.String())
	}
	for _, want := range []string{`"childFrameIds":["child"]`, `"directAttemptCount":1`, `"directFailureCount":1`} {
		if !strings.Contains(w.Body.String(), want) {
			t.Fatalf("compact frame missing %s: %s", want, w.Body.String())
		}
	}
	for _, forbidden := range []string{`"inclusiveDurationMillis"`, `"directUsage"`, `"skillNames"`, `"attemptIds"`, `"failureIds"`} {
		if strings.Contains(w.Body.String(), forbidden) {
			t.Fatalf("compact frame retained %s: %s", forbidden, w.Body.String())
		}
	}
}

func TestTraceAnalysisMapsConfiguredLimitsAndDirectFailureRelationships(t *testing.T) {
	router, _, cookie, fake := traceAnalysisRouter(t)
	fake.summary.ConfiguredLimits = &traceanalysis.ConfiguredLimits{MaxSkillInvocations: 7, MaxToolInvocations: 11, MaxLinterRetries: 3, MaxModelCalls: 5, MaxProviderAttempts: 15, MaxUsageUnits: 1234}
	fake.failurePage = traceanalysis.Page[traceanalysis.FailureSummary]{Items: []traceanalysis.FailureSummary{{
		FailureID: "failure-1", Terminal: true, Sequence: 42, TimestampMillis: 1000,
		RecordType: "ERROR_RECORDED", FrameID: "frame-1", Route: "root.child",
		AttemptID: "attempt-1", RetrySequenceID: "retry-1", ValidationStatus: "exhausted",
	}}}
	summary := traceAnalysisRequest(router, "/api/console/v1/traces/analysis/summary", `{"source":"TARGET","traceId":"trace-1"}`, cookie)
	if summary.Code != http.StatusOK || !strings.Contains(summary.Body.String(), `"configuredLimits":{"maxSkillInvocations":7,"maxToolInvocations":11,"maxLinterRetries":3,"maxModelCalls":5,"maxProviderAttempts":15,"maxUsageUnits":1234}`) {
		t.Fatalf("summary=%s", summary.Body.String())
	}
	failures := traceAnalysisRequest(router, "/api/console/v1/traces/analysis/failures", `{"source":"TARGET","traceId":"trace-1"}`, cookie)
	for _, expected := range []string{`"sequence":42`, `"recordType":"ERROR_RECORDED"`, `"frameId":"frame-1"`, `"attemptId":"attempt-1"`, `"validationStatus":"exhausted"`} {
		if !strings.Contains(failures.Body.String(), expected) {
			t.Errorf("missing %s in %s", expected, failures.Body.String())
		}
	}
}

func TestTraceAnalysisMapsCompactFailureDescriptorsAndSelectedDiagnostic(t *testing.T) {
	router, _, cookie, fake := traceAnalysisRouter(t)
	descriptor := traceanalysis.DiagnosticDescriptor{Ordinal: 0, Kind: "JAVA_STACK_TRACE", ContentType: "text/plain; charset=utf-8", Truncated: true, CaptureLimitBytes: 1048576, DecodedBytes: 10}
	fake.failurePage = traceanalysis.Page[traceanalysis.FailureSummary]{Items: []traceanalysis.FailureSummary{{
		FailureID: "failure-1", Terminal: true, ExceptionType: "java.lang.IllegalStateException", ContextSummary: "provider failed", Diagnostics: []traceanalysis.DiagnosticDescriptor{descriptor},
	}}}
	failures := traceAnalysisRequest(router, "/api/console/v1/traces/analysis/failures", `{"source":"TARGET","traceId":"trace-1"}`, cookie)
	if failures.Code != http.StatusOK {
		t.Fatalf("failures status=%d body=%s", failures.Code, failures.Body.String())
	}
	for _, expected := range []string{`"exceptionType":"java.lang.IllegalStateException"`, `"contextSummary":"provider failed"`, `"kind":"JAVA_STACK_TRACE"`, `"decodedBytes":10`} {
		if !strings.Contains(failures.Body.String(), expected) {
			t.Errorf("missing %s in %s", expected, failures.Body.String())
		}
	}
	if strings.Contains(failures.Body.String(), `"text"`) {
		t.Fatalf("failure list leaked diagnostic text: %s", failures.Body.String())
	}

	fake.diagnosticResult = traceanalysis.FailureDiagnostic{FailureID: "failure-1", Descriptor: descriptor, Text: "stack-line"}
	diagnostic := traceAnalysisRequest(router, "/api/console/v1/traces/analysis/failure-diagnostic", `{"source":"TARGET","traceId":"trace-1","failureId":"failure-1","ordinal":0}`, cookie)
	if diagnostic.Code != http.StatusOK || !strings.Contains(diagnostic.Body.String(), `"text":"stack-line"`) {
		t.Fatalf("diagnostic status=%d body=%s", diagnostic.Code, diagnostic.Body.String())
	}
	if fake.diagnosticScope != evidence.ForTarget(target.ScopeID("scope-1")) || fake.diagnosticRequest.Handle != artifact.Handle("opaque-handle") || fake.diagnosticRequest.FailureID != "failure-1" || fake.diagnosticRequest.Ordinal != 0 {
		t.Fatalf("scope=%s request=%+v", fake.diagnosticScope, fake.diagnosticRequest)
	}
	unauthorized := traceAnalysisRequest(router, "/api/console/v1/traces/analysis/failure-diagnostic", `{"source":"TARGET","traceId":"trace-1","failureId":"failure-1","ordinal":0}`, nil)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status=%d", unauthorized.Code)
	}
	malformed := traceAnalysisRequest(router, "/api/console/v1/traces/analysis/failure-diagnostic", `{"source":"TARGET","traceId":"trace-1","failureId":"failure-1","ordinal":0,"unknown":true}`, cookie)
	if malformed.Code != http.StatusBadRequest {
		t.Fatalf("malformed status=%d body=%s", malformed.Code, malformed.Body.String())
	}
	missingOrdinal := traceAnalysisRequest(router, "/api/console/v1/traces/analysis/failure-diagnostic", `{"source":"TARGET","traceId":"trace-1","failureId":"failure-1"}`, cookie)
	if missingOrdinal.Code != http.StatusBadRequest {
		t.Fatalf("missing ordinal status=%d body=%s", missingOrdinal.Code, missingOrdinal.Body.String())
	}
}

func TestTraceAnalysisRangeAndSearchPreserveOpaqueContinuations(t *testing.T) {
	router, _, cookie, fake := traceAnalysisRouter(t)
	next := "next-range"
	fake.rangeResult = traceanalysis.ByteRangeResult{ActualStart: 4, ActualEnd: 8, TotalLength: 12, ContentType: "application/octet-stream", Encoding: traceanalysis.RangeEncodingBase64, Content: []byte("AQIDBA=="), HasMore: true, NextCursor: next}
	fake.searchPage = traceanalysis.SearchPage{Items: []traceanalysis.SearchResult{{Sequence: 7, RecordType: "STRUCTURED_OUTPUT_RECORDED", SearchedField: "content", ContentID: "c1"}}, ContentDescriptors: []traceanalysis.SearchContentDescriptor{{ContentID: "c1", ContentRef: "opaque-content"}}, HasMore: true, NextCursor: "search-next", SearchLimitations: []traceanalysis.SearchLimitation{{Code: "BINARY_CONTENT_EXCLUDED", Message: "binary excluded"}}}
	w := traceAnalysisRequest(router, "/api/console/v1/traces/analysis/content-range", `{"source":"TARGET","traceId":"trace-1","contentRef":"opaque-content","maxBytes":64,"continueCursor":"range-cursor"}`, cookie)
	if w.Code != http.StatusOK || fake.payloadRangeQuery.ContinueCursor != "range-cursor" || fake.payloadRangeQuery.ContentRef != "opaque-content" {
		t.Fatalf("content range status=%d query=%+v body=%s", w.Code, fake.payloadRangeQuery, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"encoding":"BASE64"`) || !strings.Contains(w.Body.String(), `"nextCursor":"next-range"`) {
		t.Fatalf("range projection=%s", w.Body.String())
	}
	badSource := traceAnalysisRequest(router, "/api/console/v1/traces/analysis/raw-record-range", `{"source":"TARGET","traceId":"trace-1","recordSequence":2,"contentRef":"opaque-content"}`, cookie)
	if badSource.Code != http.StatusBadRequest {
		t.Fatalf("mixed range source status=%d", badSource.Code)
	}
	search := traceAnalysisRequest(router, "/api/console/v1/traces/analysis/search", `{"source":"TARGET","traceId":"trace-1","text":"needle","pageSize":5,"cursor":"search-cursor"}`, cookie)
	if search.Code != http.StatusOK || fake.searchQuery.Cursor != "search-cursor" || fake.searchQuery.Text != "needle" {
		t.Fatalf("search status=%d query=%+v", search.Code, fake.searchQuery)
	}
	for _, expected := range []string{`"contentId":"c1"`, `"contentRef":"opaque-content"`, `"workComplete":false`, `"code":"BINARY_CONTENT_EXCLUDED"`, `"nextCursor":"search-next"`} {
		if !strings.Contains(search.Body.String(), expected) {
			t.Fatalf("search projection missing %s: %s", expected, search.Body.String())
		}
	}
}

func TestTraceAnalysisBinaryInlineContentUsesBase64(t *testing.T) {
	router, _, cookie, fake := traceAnalysisRouter(t)
	fake.recordPage = traceanalysis.Page[traceanalysis.RecordSummary]{Items: []traceanalysis.RecordSummary{{Content: &traceanalysis.ContentDescriptor{Encoding: traceanalysis.ContentEncodingBinary, InlineContent: []byte{0xff, 0x00, 0x80}}}}}
	response := traceAnalysisRequest(router, "/api/console/v1/traces/analysis/records", `{"source":"TARGET","traceId":"trace-1","pageSize":1,"representation":"LOGICAL"}`, cookie)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"inlineContent":"/wCA"`) {
		t.Fatalf("response=%d %s", response.Code, response.Body.String())
	}
}

func TestTraceAnalysisRecordsExposeFailureIdentity(t *testing.T) {
	router, _, cookie, fake := traceAnalysisRouter(t)
	fake.recordPage = traceanalysis.Page[traceanalysis.RecordSummary]{Items: []traceanalysis.RecordSummary{{Sequence: 12, Type: "STEP_FAILED", FailureID: "failure-step"}}}
	response := traceAnalysisRequest(router, "/api/console/v1/traces/analysis/records", `{"source":"TARGET","traceId":"trace-1","pageSize":1,"representation":"LOGICAL"}`, cookie)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"failureId":"failure-step"`) {
		t.Fatalf("response=%d %s", response.Code, response.Body.String())
	}
}

func TestTraceAnalysisRejectsMissingExpiredAndOversizedRequests(t *testing.T) {
	router, _, cookie, _ := traceAnalysisRouter(t)
	missing := traceAnalysisRequest(router, "/api/console/v1/traces/analysis/summary", `{}`, cookie)
	if missing.Code != http.StatusBadRequest {
		t.Fatalf("missing trace status=%d", missing.Code)
	}
	oversized := traceAnalysisRequest(router, "/api/console/v1/traces/analysis/summary", `{"source":"TARGET","traceId":"trace-1","padding":"`+strings.Repeat("x", maxTraceAnalysisJSONBody)+`"}`, cookie)
	if oversized.Code != http.StatusBadRequest {
		t.Fatalf("oversized status=%d", oversized.Code)
	}
	expiredArtifacts := &fakeArtifactService{lookupErr: consolecore.NewError(consolecore.CodeArtifactExpired, "The local artifact expired.", "scope-1", consolecore.Details{}, nil)}
	expiredRouter, _, expiredCookie := artifactTestRouter(t, expiredArtifacts)
	expiredRouter.options.TraceAnalysis = &fakeTraceAnalysisService{}
	expired := traceAnalysisRequest(expiredRouter, "/api/console/v1/traces/analysis/summary", `{"source":"TARGET","traceId":"trace-1"}`, expiredCookie)
	if expired.Code != http.StatusConflict || !strings.Contains(expired.Body.String(), `"ARTIFACT_EXPIRED"`) {
		t.Fatalf("expired status=%d body=%s", expired.Code, expired.Body.String())
	}
}
