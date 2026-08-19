package mcpadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/applicationclient"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/browserapi"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/browserauth"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/evidence"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/target"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/traceanalysis"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/traceresolution"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/workspace"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type passThroughProcessor struct{}

func (passThroughProcessor) Process(request artifact.ProcessRequest) (artifact.ProcessResult, *consolecore.Error) {
	if _, err := io.Copy(io.Discard, request.Raw); err != nil {
		return artifact.ProcessResult{}, consolecore.NewError(consolecore.CodeConsoleError, "The test artifact could not be read.", "", consolecore.Details{}, err)
	}
	return artifact.ProcessResult{ComponentSizes: map[artifact.ComponentName]int64{}, Metadata: request.Metadata}, nil
}

type echoSummaryAnalysis struct{}

func (echoSummaryAnalysis) GetSummary(_ context.Context, ref evidence.Reference, request traceanalysis.SummaryRequest) (traceanalysis.TraceSummary, *consolecore.Error) {
	return traceanalysis.TraceSummary{Context: traceanalysis.TraceContext{Evidence: ref, Handle: request.Handle, TraceID: "trace-joined", SessionID: "session-joined"}, RootFrameIDs: []string{}}, nil
}
func (echoSummaryAnalysis) QueryFrames(context.Context, evidence.Reference, traceanalysis.FrameQuery) (traceanalysis.Page[traceanalysis.FrameSummary], *consolecore.Error) {
	return traceanalysis.Page[traceanalysis.FrameSummary]{}, nil
}
func (echoSummaryAnalysis) QueryRecords(context.Context, evidence.Reference, traceanalysis.RecordQuery) (traceanalysis.Page[traceanalysis.RecordSummary], *consolecore.Error) {
	return traceanalysis.Page[traceanalysis.RecordSummary]{}, nil
}
func (echoSummaryAnalysis) ReadContentRange(context.Context, evidence.Reference, traceanalysis.RangeRequest) (traceanalysis.ByteRangeResult, *consolecore.Error) {
	return traceanalysis.ByteRangeResult{}, nil
}
func (echoSummaryAnalysis) ReadRawArtifactRange(context.Context, evidence.Reference, traceanalysis.RangeRequest) (traceanalysis.ByteRangeResult, *consolecore.Error) {
	return traceanalysis.ByteRangeResult{}, nil
}

type leasingSummaryAnalysis struct {
	echoSummaryAnalysis
	artifacts *artifact.Service
	entered   chan *artifact.Lease
	release   <-chan struct{}
	active    atomic.Int32
	maximum   atomic.Int32
}

func (analysis *leasingSummaryAnalysis) GetSummary(_ context.Context, ref evidence.Reference, request traceanalysis.SummaryRequest) (traceanalysis.TraceSummary, *consolecore.Error) {
	lease, domain := analysis.artifacts.Use(ref, request.Handle)
	if domain != nil {
		analysis.entered <- nil
		return traceanalysis.TraceSummary{}, domain
	}
	active := analysis.active.Add(1)
	for {
		maximum := analysis.maximum.Load()
		if active <= maximum || analysis.maximum.CompareAndSwap(maximum, active) {
			break
		}
	}
	analysis.entered <- lease
	<-analysis.release
	analysis.active.Add(-1)
	_ = lease.Close(true)
	return traceanalysis.TraceSummary{Context: traceanalysis.TraceContext{Evidence: ref, Handle: request.Handle, TraceID: "trace-joined", SessionID: "session-joined"}, RootFrameIDs: []string{}}, nil
}

func TestBrowserAndMCPJoinOneAcquisitionHandleAndCapacityCharge(t *testing.T) {
	runJoinedAdapterAcquisitionFixture(t)
}

func runJoinedAdapterAcquisitionFixture(t *testing.T) {
	t.Helper()
	options := newMCPTestOptions(t, nil)
	raw := []byte("joined adapter artifact")
	metadata := artifact.TraceMetadata{TraceID: "trace-joined", SessionID: "session-joined", Outcome: "SUCCEEDED", FinalizedAt: time.Date(2026, 8, 14, 20, 0, 0, 0, time.UTC), SizeBytes: int64(len(raw)), PersistencePolicy: "ALWAYS"}
	loaderEntered := make(chan struct{})
	releaseLoader := make(chan struct{})
	var loadOnce sync.Once
	var loaderCalls, streamCalls atomic.Int32
	ws, err := workspace.Open(filepath.Join(t.TempDir(), "workspace"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ws.Close() })
	artifacts, err := artifact.New(artifact.Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}, artifact.Dependencies{
		Workspace: ws,
		TraceLoader: func(context.Context, target.Scope, string) (artifact.TraceMetadata, *consolecore.Error) {
			loaderCalls.Add(1)
			loadOnce.Do(func() { close(loaderEntered) })
			<-releaseLoader
			return metadata, nil
		},
		StreamOpener: func(_ context.Context, scope target.Scope, _ string) (*applicationclient.ArtifactStream, *consolecore.Error) {
			streamCalls.Add(1)
			return applicationclient.NewTestArtifactStream(io.NopCloser(bytes.NewReader(raw)), scope.InstanceID, int64(len(raw))), nil
		},
		Processor: passThroughProcessor{},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(artifacts.Close)
	if err := options.Target.RegisterOwner("joined-artifacts", artifacts); err != nil {
		t.Fatal(err)
	}
	artifacts.ActivateActivity(mustCaptureScope(t, options.Target))
	releaseAnalysis := make(chan struct{})
	analysis := &leasingSummaryAnalysis{artifacts: artifacts, entered: make(chan *artifact.Lease, 2), release: releaseAnalysis}
	options.TraceResolver = traceresolution.New(artifacts, options.Observability, options.Target)
	options.TraceAnalysis = analysis

	router, cookie, tabID, csrf := joinedBrowserRouter(t, options.Target, artifacts)
	type browserResult struct {
		code   int
		body   []byte
		handle string
	}
	browserDone := make(chan browserResult, 1)
	go func() {
		request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7943/api/console/v1/artifacts/acquire", strings.NewReader(`{"traceId":"trace-joined"}`))
		request.Host = "127.0.0.1:7943"
		request.Header.Set("Origin", "http://127.0.0.1:7943")
		request.Header.Set("X-loomspan-Console-Tab", tabID)
		request.Header.Set("X-loomspan-Console-CSRF", csrf)
		request.AddCookie(cookie)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		var body map[string]any
		_ = json.Unmarshal(response.Body.Bytes(), &body)
		handle, _ := body["artifactHandle"].(string)
		browserDone <- browserResult{code: response.Code, body: response.Body.Bytes(), handle: handle}
	}()
	<-loaderEntered
	type mcpResult struct {
		result   *mcp.CallToolResult
		envelope toolEnvelope[getTraceResult]
		err      error
	}
	callMCP := func(ctx context.Context) <-chan mcpResult {
		done := make(chan mcpResult, 1)
		go func() {
			result, envelope, err := handleGetTrace(ctx, options, getTraceInput{TraceID: "trace-joined"})
			done <- mcpResult{result: result, envelope: envelope, err: err}
		}()
		return done
	}
	firstMCP := callMCP(context.Background())
	secondMCP := callMCP(context.Background())
	canceledContext, cancel := context.WithCancel(context.Background())
	canceledMCP := callMCP(canceledContext)
	waitForJoinedAdapterWaiters(t, artifacts, 4)
	cancel()
	canceled := <-canceledMCP
	if canceled.err == nil && canceled.result != nil && !canceled.result.IsError {
		t.Fatalf("canceled acquisition published success: %#v", canceled)
	}
	close(releaseLoader)
	browser := <-browserDone
	firstLease, secondLease := <-analysis.entered, <-analysis.entered
	if firstLease == nil || secondLease == nil || firstLease == secondLease || analysis.maximum.Load() != 2 {
		t.Fatalf("leases first=%p second=%p maximum=%d", firstLease, secondLease, analysis.maximum.Load())
	}
	pinned, domain := artifacts.StorageSnapshot()
	if domain != nil || len(pinned.Entries) != 1 || !pinned.Entries[0].ActivePin {
		t.Fatalf("concurrent analysis did not pin shared evidence: snapshot=%#v domain=%v", pinned, domain)
	}
	close(releaseAnalysis)
	first, second := <-firstMCP, <-secondMCP
	for index, call := range []mcpResult{first, second} {
		if call.err != nil || call.result == nil || call.result.IsError || call.envelope.Result == nil || call.envelope.Result.Evidence.TraceID != "trace-joined" {
			t.Fatalf("MCP call %d=%#v", index, call)
		}
	}
	if browser.code != http.StatusOK || browser.handle == "" {
		t.Fatalf("browser code=%d body=%s handle=%q", browser.code, browser.body, browser.handle)
	}
	snapshot, domain := artifacts.StorageSnapshot()
	if domain != nil || loaderCalls.Load() != 1 || streamCalls.Load() != 1 || snapshot.AcquiredCount != 1 || len(snapshot.Entries) != 1 {
		t.Fatalf("loader=%d streams=%d snapshot=%#v domain=%v", loaderCalls.Load(), streamCalls.Load(), snapshot, domain)
	}
	if domain := options.Target.Select("http://127.0.0.1:8081"); domain != nil {
		t.Fatal(domain)
	}
	rotated, domain := artifacts.StorageSnapshot()
	if domain != nil || len(rotated.Entries) != 0 {
		t.Fatalf("target rotation retained target evidence: snapshot=%#v domain=%v", rotated, domain)
	}
}

func waitForJoinedAdapterWaiters(t *testing.T, artifacts *artifact.Service, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		snapshot, domain := artifacts.StorageSnapshot()
		if domain != nil {
			t.Fatal(domain)
		}
		if snapshot.PendingAcquisitionCount == 1 && snapshot.PendingWaiterCount >= want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	snapshot, domain := artifacts.StorageSnapshot()
	t.Fatalf("timed out waiting for %d joined adapter waiters: snapshot=%#v domain=%v", want, snapshot, domain)
}

func mustCaptureScope(t *testing.T, context *target.Context) target.Scope {
	t.Helper()
	scope, domain := context.Capture()
	if domain != nil {
		t.Fatal(domain)
	}
	return scope
}

func joinedBrowserRouter(t *testing.T, targetContext *target.Context, artifacts *artifact.Service) (http.Handler, *http.Cookie, string, string) {
	t.Helper()
	entropy := bytes.Repeat([]byte{31}, 32*16)
	registry := browserauth.NewRegistry(nil, bytes.NewReader(entropy))
	t.Cleanup(registry.Close)
	sessionID, err := registry.CreateSession()
	if err != nil {
		t.Fatal(err)
	}
	policy, err := browserapi.NewPolicy("127.0.0.1:7943", "http://127.0.0.1:7943", "")
	if err != nil {
		t.Fatal(err)
	}
	router, err := browserapi.New(browserapi.Options{Policy: policy, Pairing: browserauth.NewPairing(nil, bytes.NewReader(entropy)), Sessions: registry, PairingURL: func(value string) string { return value }, Target: targetContext, Artifacts: artifacts})
	if err != nil {
		t.Fatal(err)
	}
	bootstrap, err := registry.Bootstrap(sessionID, "")
	if err != nil {
		t.Fatal(err)
	}
	return router, browserauth.SessionCookie(sessionID), bootstrap.TabID, bootstrap.CSRF
}
