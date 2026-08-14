package mcpadapter

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/evidence"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/mcpcredential"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/target"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/traceanalysis"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/traceinventory"
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

func (f *fakeTraceInventory) List(_ context.Context, q traceinventory.Query) (traceinventory.Result, *consolecore.Error) {
	f.calls++
	f.query = q
	return f.result, nil
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
}

func (f *fakeTraceAnalysis) GetSummary(_ context.Context, r evidence.Reference, _ traceanalysis.SummaryRequest) (traceanalysis.TraceSummary, *consolecore.Error) {
	f.summaryCalls++
	f.refs = append(f.refs, r)
	return f.summary, nil
}
func (f *fakeTraceAnalysis) QueryFrames(_ context.Context, r evidence.Reference, query traceanalysis.FrameQuery) (traceanalysis.Page[traceanalysis.FrameSummary], *consolecore.Error) {
	f.frameCalls++
	f.refs = append(f.refs, r)
	f.frameQuery = query
	return f.frames, nil
}
func (f *fakeTraceAnalysis) QueryRecords(_ context.Context, r evidence.Reference, query traceanalysis.RecordQuery) (traceanalysis.Page[traceanalysis.RecordSummary], *consolecore.Error) {
	f.recordCalls++
	f.refs = append(f.refs, r)
	f.recordQuery = query
	return f.records, nil
}
func (f *fakeTraceAnalysis) ReadPayloadRange(_ context.Context, r evidence.Reference, request traceanalysis.RangeRequest) (traceanalysis.ByteRangeResult, *consolecore.Error) {
	f.payloadCalls++
	f.refs = append(f.refs, r)
	f.rangeRequest = request
	return f.payload, nil
}
func (f *fakeTraceAnalysis) ReadRawArtifactRange(_ context.Context, r evidence.Reference, request traceanalysis.RangeRequest) (traceanalysis.ByteRangeResult, *consolecore.Error) {
	f.rawCalls++
	f.refs = append(f.refs, r)
	f.rangeRequest = request
	return f.raw, nil
}

type fakeTraceArtifacts struct {
	result artifact.AcquiredArtifact
	calls  int
}

func (f *fakeTraceArtifacts) Acquire(context.Context, target.Scope, string) (artifact.AcquiredArtifact, *consolecore.Error) {
	f.calls++
	return f.result, nil
}

func TestImportedTraceToolsWorkWithoutSelectedTarget(t *testing.T) {
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	handle := artifact.Handle("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	ctxValue := traceanalysis.TraceContext{Evidence: evidence.ForImported(), Handle: handle, TraceID: "trace-i", SessionID: "session-i"}
	analysis := &fakeTraceAnalysis{summary: traceanalysis.TraceSummary{Context: ctxValue, RootFrameIDs: []string{}}, frames: traceanalysis.Page[traceanalysis.FrameSummary]{Context: ctxValue, Items: []traceanalysis.FrameSummary{}}, records: traceanalysis.Page[traceanalysis.RecordSummary]{Context: ctxValue, Items: []traceanalysis.RecordSummary{}}, payload: traceanalysis.ByteRangeResult{Context: ctxValue, ContentType: "text/plain", Encoding: traceanalysis.RangeEncodingText, Content: []byte("ok")}, raw: traceanalysis.ByteRangeResult{Context: ctxValue, ContentType: "application/x-ndjson", Encoding: traceanalysis.RangeEncodingText, Content: []byte("{}\n")}}
	inventory := &fakeTraceInventory{result: traceinventory.Result{ObservedAt: now, Items: []traceinventory.Entry{{Source: evidence.SourceImported, TraceID: "trace-i", SessionID: "session-i", ArtifactHandle: handle, LocalAvailable: true}}}}
	options := ServerOptions{Credentials: fakeCredentials{state: mcpcredential.Snapshot{State: mcpcredential.Enabled, Generation: 1}}, Now: func() time.Time { return now }, TraceInventory: inventory, TraceAnalysis: analysis}
	if result, envelope, err := handleListTraces(context.Background(), options, listTracesInput{SourceFilter: traceinventory.SourceFilterImported}); err != nil || result.IsError || envelope.Result == nil {
		t.Fatalf("list result=%#v envelope=%#v err=%v", result, envelope, err)
	}
	if result, envelope, err := handleGetTrace(context.Background(), options, getTraceInput{Source: "IMPORTED", ArtifactHandle: string(handle)}); err != nil || result.IsError || envelope.Result == nil || envelope.Result.Evidence.TargetScopeID != "" {
		t.Fatalf("get result=%#v envelope=%#v err=%v", result, envelope, err)
	}
	if result, envelope, err := handleQueryTraceFrames(context.Background(), options, queryTraceFramesInput{Source: "IMPORTED", ArtifactHandle: string(handle)}); err != nil || result.IsError || envelope.Result == nil || envelope.Result.Evidence.Source != "IMPORTED" {
		t.Fatalf("frames result=%#v envelope=%#v err=%v", result, envelope, err)
	}
	if result, envelope, err := handleQueryTraceRecords(context.Background(), options, queryTraceRecordsInput{Source: "IMPORTED", ArtifactHandle: string(handle)}); err != nil || result.IsError || envelope.Result == nil || envelope.Result.Evidence.Source != "IMPORTED" {
		t.Fatalf("records result=%#v envelope=%#v err=%v", result, envelope, err)
	}
	start := int64(0)
	if result, envelope, err := handleTraceRange(context.Background(), options, traceRangeInput{Source: "IMPORTED", ArtifactHandle: string(handle), PayloadRef: "opaque", Start: &start}, false); err != nil || result.IsError || envelope.Result == nil {
		t.Fatalf("payload result=%#v envelope=%#v err=%v", result, envelope, err)
	}
	if result, envelope, err := handleTraceRange(context.Background(), options, traceRangeInput{Source: "IMPORTED", ArtifactHandle: string(handle), Start: &start}, true); err != nil || result.IsError || envelope.Result == nil {
		t.Fatalf("raw result=%#v envelope=%#v err=%v", result, envelope, err)
	}
	for _, ref := range analysis.refs {
		if ref.Source != evidence.SourceImported || ref.TargetScope != "" {
			t.Fatalf("invented target reference: %#v", ref)
		}
	}
}

func TestGetTraceRejectsInvalidSourceIdentifierBranches(t *testing.T) {
	options := ServerOptions{Credentials: fakeCredentials{state: mcpcredential.Snapshot{State: mcpcredential.Enabled}}, Now: time.Now, TraceAnalysis: &fakeTraceAnalysis{}}
	for _, input := range []getTraceInput{{Source: "IMPORTED", TraceID: "trace"}, {Source: "IMPORTED"}, {Source: "IMPORTED", TraceID: "trace", ArtifactHandle: "handle"}, {Source: "OTHER", ArtifactHandle: "handle"}} {
		result, envelope, err := handleGetTrace(context.Background(), options, input)
		if err != nil || result == nil || !result.IsError || envelope.Error == nil || envelope.Error.Code != consolecore.CodeInvalidArgument {
			t.Fatalf("input=%#v result=%#v envelope=%#v err=%v", input, result, envelope, err)
		}
	}
}

func TestTraceCapabilityTargetAcquisitionFactsContinuationsAndBothSources(t *testing.T) {
	options := newMCPTestOptions(t, func(string) ([]byte, error) { return nil, errors.New("unused") })
	handle := artifact.Handle(strings.Repeat("c", 64))
	targetContext := traceanalysis.TraceContext{Evidence: evidence.ForTarget("scope-1"), Handle: handle, TraceID: "target-trace", SessionID: "session"}
	analysis := &fakeTraceAnalysis{
		summary: traceanalysis.TraceSummary{Context: targetContext, RootFrameIDs: []string{}},
		records: traceanalysis.Page[traceanalysis.RecordSummary]{Context: targetContext, HasMore: true, NextCursor: "next-record", Items: []traceanalysis.RecordSummary{{Context: targetContext, Sequence: 4, Type: "MODEL_RESPONSE_RECEIVED", Facts: traceanalysis.RecordFacts{Attempts: []traceanalysis.AttemptSummary{{AttemptID: "attempt-1"}}, Retries: []traceanalysis.RetrySummary{}, Validations: []traceanalysis.ValidationSummary{}, Failures: []traceanalysis.FailureSummary{}, Payloads: []traceanalysis.PayloadDescriptor{}, SearchMatches: []traceanalysis.SearchResult{}}}}},
		raw:     traceanalysis.ByteRangeResult{Context: targetContext, Source: traceanalysis.RangeSourceRawArtifact, ActualEnd: 4, TotalLength: 4, Encoding: traceanalysis.RangeEncodingText, Content: []byte("data")},
	}
	artifacts := &fakeTraceArtifacts{result: artifact.AcquiredArtifact{Handle: handle, Owner: evidence.Target("scope-1")}}
	options.TraceAnalysis, options.Artifacts = analysis, artifacts
	if result, envelope, err := handleGetTrace(context.Background(), options, getTraceInput{Source: "TARGET", TraceID: "target-trace"}); err != nil || result.IsError || envelope.Result == nil || artifacts.calls != 1 || envelope.Result.Evidence.Source != "TARGET" {
		t.Fatalf("target acquisition result=%#v envelope=%#v calls=%d err=%v", result, envelope, artifacts.calls, err)
	}
	if result, envelope, err := handleQueryTraceRecords(context.Background(), options, queryTraceRecordsInput{Source: "TARGET", ArtifactHandle: string(handle), Continuation: "prior-record"}); err != nil || result.IsError || envelope.Result == nil || envelope.Result.Continuation != "next-record" || analysis.recordQuery.Cursor != "prior-record" || len(envelope.Result.Items) != 1 || len(envelope.Result.Items[0].Facts.Attempts) != 1 {
		t.Fatalf("record facts/continuation result=%#v envelope=%#v query=%#v err=%v", result, envelope, analysis.recordQuery, err)
	}
	start := int64(0)
	if result, _, err := handleTraceRange(context.Background(), options, traceRangeInput{Source: "TARGET", ArtifactHandle: string(handle), Start: &start}, true); err != nil || result.IsError || analysis.rangeRequest.Start != 0 {
		t.Fatalf("target raw result=%#v request=%#v err=%v", result, analysis.rangeRequest, err)
	}
	analysis.raw.Context.Evidence = evidence.ForImported()
	if result, envelope, err := handleTraceRange(context.Background(), options, traceRangeInput{Source: "IMPORTED", ArtifactHandle: string(handle), Continuation: "raw-next"}, true); err != nil || result.IsError || envelope.Result == nil || envelope.Result.Evidence.Source != "IMPORTED" || analysis.rangeRequest.ContinueCursor != "raw-next" {
		t.Fatalf("imported raw result=%#v envelope=%#v request=%#v err=%v", result, envelope, analysis.rangeRequest, err)
	}
}
