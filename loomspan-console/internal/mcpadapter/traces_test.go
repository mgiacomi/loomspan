package mcpadapter

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/applicationclient"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/evidence"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/mcpcredential"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/target"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/traceanalysis"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/traceinventory"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/traceresolution"
)

func TestTraceRangeTextFallbackPreservesContent(t *testing.T) {
	text := traceRangeText(rangeResult{ActualStart: 0, ActualEnd: 16, TotalLength: 32, ContentType: "application/octet-stream", Encoding: "BASE64", Content: "unique-large-content", HasMore: true})
	if !strings.Contains(text, `"content":"unique-large-content"`) || !strings.Contains(text, `"actualStart":0`) || !strings.Contains(text, `"hasMore":true`) {
		t.Fatalf("range fallback omitted diagnostic facts: %q", text)
	}
}

type fakeTraceInventory struct {
	result traceinventory.Result
	query  traceinventory.Query
	calls  int
}

func (fake *fakeTraceInventory) List(_ context.Context, query traceinventory.Query) (traceinventory.Result, *consolecore.Error) {
	fake.calls++
	fake.query = query
	return fake.result, nil
}

type fakeTraceAnalysis struct {
	summary      traceanalysis.TraceSummary
	frames       traceanalysis.Page[traceanalysis.FrameSummary]
	records      traceanalysis.Page[traceanalysis.RecordSummary]
	payload      traceanalysis.ByteRangeResult
	raw          traceanalysis.ByteRangeResult
	refs         []evidence.Reference
	frameQuery   traceanalysis.FrameQuery
	recordQuery  traceanalysis.RecordQuery
	rangeRequest traceanalysis.RangeRequest
	summaryCalls int
	frameCalls   int
	recordCalls  int
	payloadCalls int
	rawCalls     int
	onSummary    func()
}

func (fake *fakeTraceAnalysis) GetSummary(_ context.Context, ref evidence.Reference, _ traceanalysis.SummaryRequest) (traceanalysis.TraceSummary, *consolecore.Error) {
	fake.summaryCalls++
	fake.refs = append(fake.refs, ref)
	if fake.onSummary != nil {
		fake.onSummary()
	}
	return fake.summary, nil
}
func (fake *fakeTraceAnalysis) QueryFrames(_ context.Context, ref evidence.Reference, query traceanalysis.FrameQuery) (traceanalysis.Page[traceanalysis.FrameSummary], *consolecore.Error) {
	fake.frameCalls++
	fake.refs = append(fake.refs, ref)
	fake.frameQuery = query
	return fake.frames, nil
}
func (fake *fakeTraceAnalysis) QueryRecords(_ context.Context, ref evidence.Reference, query traceanalysis.RecordQuery) (traceanalysis.Page[traceanalysis.RecordSummary], *consolecore.Error) {
	fake.recordCalls++
	fake.refs = append(fake.refs, ref)
	fake.recordQuery = query
	return fake.records, nil
}
func (fake *fakeTraceAnalysis) ReadPayloadRange(_ context.Context, ref evidence.Reference, request traceanalysis.RangeRequest) (traceanalysis.ByteRangeResult, *consolecore.Error) {
	fake.payloadCalls++
	fake.refs = append(fake.refs, ref)
	fake.rangeRequest = request
	return fake.payload, nil
}
func (fake *fakeTraceAnalysis) ReadRawArtifactRange(_ context.Context, ref evidence.Reference, request traceanalysis.RangeRequest) (traceanalysis.ByteRangeResult, *consolecore.Error) {
	fake.rawCalls++
	fake.refs = append(fake.refs, ref)
	fake.rangeRequest = request
	return fake.raw, nil
}

type fakeTraceArtifacts struct {
	result artifact.AcquiredArtifact
	err    *consolecore.Error
	calls  atomic.Int32
	ref    evidence.Reference
	scope  target.Scope
}

func (fake *fakeTraceArtifacts) Resolve(_ context.Context, _ string) (traceresolution.Resolved, *consolecore.Error) {
	fake.calls.Add(1)
	if fake.err != nil {
		return traceresolution.Resolved{}, fake.err
	}
	ref := fake.ref
	if !ref.Valid() {
		ref = evidence.ForImported()
		if fake.result.Owner.Source() == evidence.SourceTarget {
			ref = evidence.ForTarget(fake.result.Owner.TargetScope())
		}
	}
	scope := fake.scope
	if ref.Source == evidence.SourceTarget && scope.ID == "" {
		scope.ID = ref.TargetScope
	}
	return traceresolution.Resolved{Reference: ref, Handle: fake.result.Handle, Scope: scope}, nil
}

func TestTraceHandlersResolveTraceIDAndPreserveQuestionSpecificRequests(t *testing.T) {
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	handle := artifact.Handle(strings.Repeat("a", 64))
	traceContext := traceanalysis.TraceContext{Evidence: evidence.ForImported(), Handle: handle, TraceID: "trace-i", SessionID: "session-i"}
	analysis := &fakeTraceAnalysis{
		summary: traceanalysis.TraceSummary{Context: traceContext, RootFrameIDs: []string{}},
		frames:  traceanalysis.Page[traceanalysis.FrameSummary]{Context: traceContext, Items: []traceanalysis.FrameSummary{}},
		records: traceanalysis.Page[traceanalysis.RecordSummary]{Context: traceContext, Items: []traceanalysis.RecordSummary{}, NextCursor: "next"},
		payload: traceanalysis.ByteRangeResult{Context: traceContext, ContentType: "text/plain", Encoding: traceanalysis.RangeEncodingText, Content: []byte("ok")},
		raw:     traceanalysis.ByteRangeResult{Context: traceContext, ContentType: "application/x-ndjson", Encoding: traceanalysis.RangeEncodingText, Content: []byte("{}\n")},
	}
	resolver := &fakeTraceArtifacts{result: artifact.AcquiredArtifact{Handle: handle}, ref: evidence.ForImported()}
	inventory := &fakeTraceInventory{result: traceinventory.Result{ObservedAt: now, Complete: true, Items: []traceinventory.Entry{{TraceID: "trace-i", SessionID: "session-i"}}}}
	options := ServerOptions{Credentials: fakeCredentials{state: mcpcredential.Snapshot{State: mcpcredential.Enabled, Generation: 1}}, Now: func() time.Time { return now }, TraceInventory: inventory, TraceAnalysis: analysis, TraceResolver: resolver}

	if result, envelope, err := handleListTraces(context.Background(), options, listTracesInput{}); err != nil || result.IsError || envelope.Result == nil || !envelope.Result.Complete {
		t.Fatalf("list result=%#v envelope=%#v err=%v", result, envelope, err)
	}
	if result, envelope, err := handleGetTrace(context.Background(), options, getTraceInput{TraceID: "trace-i"}); err != nil || result.IsError || envelope.Result == nil || envelope.Result.Evidence.TraceID != "trace-i" {
		t.Fatalf("get result=%#v envelope=%#v err=%v", result, envelope, err)
	}
	if result, _, err := handleQueryTraceFrames(context.Background(), options, queryTraceFramesInput{TraceID: "trace-i", Order: traceanalysis.FrameOrderCanonical}); err != nil || result.IsError {
		t.Fatalf("frames result=%#v err=%v", result, err)
	}
	if result, _, err := handleQueryTraceRecords(context.Background(), options, queryTraceRecordsInput{TraceID: "trace-i", Continuation: "prior", InlinePayload: true}); err != nil || result.IsError || analysis.recordQuery.Cursor != "prior" || !analysis.recordQuery.InlinePayload {
		t.Fatalf("records result=%#v request=%#v err=%v", result, analysis.recordQuery, err)
	}
	start := int64(0)
	if result, _, err := handleTraceRange(context.Background(), options, traceRangeInput{TraceID: "trace-i", PayloadRef: "opaque", Start: &start}, false); err != nil || result.IsError || analysis.rangeRequest.PayloadRef != "opaque" {
		t.Fatalf("payload result=%#v request=%#v err=%v", result, analysis.rangeRequest, err)
	}
	if result, _, err := handleTraceRange(context.Background(), options, traceRangeInput{TraceID: "trace-i", Start: &start}, true); err != nil || result.IsError {
		t.Fatalf("raw result=%#v err=%v", result, err)
	}
	if resolver.calls.Load() != 5 || analysis.summaryCalls != 1 || analysis.frameCalls != 1 || analysis.recordCalls != 1 || analysis.payloadCalls != 1 || analysis.rawCalls != 1 {
		t.Fatalf("resolver=%d summary=%d frames=%d records=%d payload=%d raw=%d", resolver.calls.Load(), analysis.summaryCalls, analysis.frameCalls, analysis.recordCalls, analysis.payloadCalls, analysis.rawCalls)
	}
}

func TestTraceOpaqueTokenErrorsGiveTraceIDRecoveryWithoutHandleTerms(t *testing.T) {
	for _, test := range []struct {
		domain       *consolecore.Error
		continuation bool
		payload      bool
		want         string
	}{
		{consolecore.NewError(consolecore.CodeInvalidCursor, "old handle mismatch", "scope", consolecore.Details{}, nil), true, false, "Restart this query by traceId."},
		{consolecore.NewError(consolecore.CodeInvalidArgument, "The payload reference is invalid.", "scope", consolecore.Details{}, nil), false, true, "Re-query the relevant record descriptor by traceId."},
		{consolecore.NewError(consolecore.CodeArtifactExpired, "old handle expired", "scope", consolecore.Details{}, nil), false, false, "Retry inspection by traceId"},
	} {
		mapped := mapTraceAnalysisError(test.domain, "trace-1", test.continuation, test.payload)
		if !strings.Contains(mapped.Message, test.want) || strings.Contains(strings.ToLower(mapped.Message), "handle") || mapped.TargetScopeID != "" {
			t.Fatalf("mapped=%#v", mapped)
		}
	}
}

type mutableTraceCredentials struct{ generation atomic.Uint64 }

func (credentials *mutableTraceCredentials) Snapshot() mcpcredential.Snapshot {
	return mcpcredential.Snapshot{State: mcpcredential.Enabled, Generation: credentials.generation.Load()}
}
func (credentials *mutableTraceCredentials) Authenticate(string) (uint64, bool) {
	return credentials.generation.Load(), true
}

func TestTraceHandlerSuppressesAmbiguityAndChangedPublicationAuthority(t *testing.T) {
	handle := artifact.Handle(strings.Repeat("f", 64))

	t.Run("ambiguity stops before analysis", func(t *testing.T) {
		analysis := &fakeTraceAnalysis{}
		options := ServerOptions{
			Credentials:   fakeCredentials{state: mcpcredential.Snapshot{State: mcpcredential.Enabled, Generation: 1}},
			TraceResolver: &fakeTraceArtifacts{err: consolecore.NewError(consolecore.CodeAmbiguousTrace, "ambiguous", "", consolecore.Details{}, nil)},
			TraceAnalysis: analysis,
			Now:           time.Now,
		}
		result, envelope, err := handleGetTrace(context.Background(), options, getTraceInput{TraceID: "trace-1"})
		if err != nil || result == nil || !result.IsError || envelope.Error == nil || envelope.Error.Code != consolecore.CodeAmbiguousTrace || analysis.summaryCalls != 0 {
			t.Fatalf("result=%#v envelope=%#v err=%v summaryCalls=%d", result, envelope, err, analysis.summaryCalls)
		}
	})

	t.Run("selected-target imported fallback rotates after analysis", func(t *testing.T) {
		ids := []target.ScopeID{"scope-1", "scope-2"}
		client := &mcpTestTargetClient{}
		targetContext, err := target.New(
			func(applicationclient.Address) (target.ProbeClient, error) { return client, nil },
			func() (target.ScopeID, error) { id := ids[0]; ids = ids[1:]; return id, nil },
			time.Now,
		)
		if err != nil {
			t.Fatal(err)
		}
		defer targetContext.Close()
		if domain := targetContext.Select("http://127.0.0.1:8080"); domain != nil {
			t.Fatal(domain)
		}
		if _, domain := targetContext.SupplyCredential(context.Background(), []byte(strings.Repeat("k", 32))); domain != nil {
			t.Fatal(domain)
		}
		scope, domain := targetContext.Capture()
		if domain != nil {
			t.Fatal(domain)
		}
		analysis := &fakeTraceAnalysis{summary: traceanalysis.TraceSummary{Context: traceanalysis.TraceContext{TraceID: "trace-1"}, RootFrameIDs: []string{}}}
		analysis.onSummary = func() {
			if domain := targetContext.Select("http://127.0.0.1:8081"); domain != nil {
				t.Fatal(domain)
			}
		}
		options := ServerOptions{
			Credentials:   fakeCredentials{state: mcpcredential.Snapshot{State: mcpcredential.Enabled, Generation: 1}},
			Target:        targetContext,
			TraceResolver: &fakeTraceArtifacts{result: artifact.AcquiredArtifact{Handle: handle}, ref: evidence.ForImported(), scope: scope},
			TraceAnalysis: analysis,
			Now:           time.Now,
		}
		result, envelope, callErr := handleGetTrace(context.Background(), options, getTraceInput{TraceID: "trace-1"})
		if callErr != nil || result == nil || !result.IsError || envelope.Error == nil || envelope.Error.Code != consolecore.CodeTargetChanged {
			t.Fatalf("result=%#v envelope=%#v err=%v", result, envelope, callErr)
		}
	})

	t.Run("authentication generation changes after analysis", func(t *testing.T) {
		credentials := &mutableTraceCredentials{}
		credentials.generation.Store(1)
		tracker := NewTracker()
		ctx, done, err := tracker.Admit(context.Background(), 1)
		if err != nil {
			t.Fatal(err)
		}
		defer done()
		analysis := &fakeTraceAnalysis{summary: traceanalysis.TraceSummary{Context: traceanalysis.TraceContext{TraceID: "trace-1"}, RootFrameIDs: []string{}}}
		analysis.onSummary = func() { credentials.generation.Store(2) }
		options := ServerOptions{
			Credentials:   credentials,
			TraceResolver: &fakeTraceArtifacts{result: artifact.AcquiredArtifact{Handle: handle}, ref: evidence.ForImported()},
			TraceAnalysis: analysis,
			Now:           time.Now,
		}
		result, envelope, callErr := handleGetTrace(ctx, options, getTraceInput{TraceID: "trace-1"})
		if callErr == nil || result != nil || envelope.Result != nil || envelope.Error != nil {
			t.Fatalf("result=%#v envelope=%#v err=%v", result, envelope, callErr)
		}
	})
}
