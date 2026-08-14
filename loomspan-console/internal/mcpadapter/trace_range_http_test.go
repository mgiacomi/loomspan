package mcpadapter

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/evidence"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/traceanalysis"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type immutableRangeAnalysis struct {
	result traceanalysis.ByteRangeResult
}

func (a *immutableRangeAnalysis) GetSummary(context.Context, evidence.Reference, traceanalysis.SummaryRequest) (traceanalysis.TraceSummary, *consolecore.Error) {
	return traceanalysis.TraceSummary{}, nil
}
func (a *immutableRangeAnalysis) QueryFrames(context.Context, evidence.Reference, traceanalysis.FrameQuery) (traceanalysis.Page[traceanalysis.FrameSummary], *consolecore.Error) {
	return traceanalysis.Page[traceanalysis.FrameSummary]{}, nil
}
func (a *immutableRangeAnalysis) QueryRecords(context.Context, evidence.Reference, traceanalysis.RecordQuery) (traceanalysis.Page[traceanalysis.RecordSummary], *consolecore.Error) {
	return traceanalysis.Page[traceanalysis.RecordSummary]{}, nil
}
func (a *immutableRangeAnalysis) ReadPayloadRange(context.Context, evidence.Reference, traceanalysis.RangeRequest) (traceanalysis.ByteRangeResult, *consolecore.Error) {
	return a.result, nil
}
func (a *immutableRangeAnalysis) ReadRawArtifactRange(ctx context.Context, _ evidence.Reference, request traceanalysis.RangeRequest) (traceanalysis.ByteRangeResult, *consolecore.Error) {
	if err := ctx.Err(); err != nil {
		return traceanalysis.ByteRangeResult{}, consolecore.NewError(consolecore.CodeTargetUnavailable, "The operation was canceled.", "", consolecore.Details{}, err)
	}
	if request.MaxBytes != int(a.result.TotalLength) {
		return traceanalysis.ByteRangeResult{}, consolecore.NewError(consolecore.CodeInvalidArgument, "The range size did not reach the analysis service.", "", consolecore.Details{}, nil)
	}
	return a.result, nil
}

func TestMCPRangeSerializationMeetsDeadlineForSelectedMaximum(t *testing.T) {
	if testing.Short() {
		t.Skip("large MCP framing gate")
	}
	for _, encoding := range []traceanalysis.RangeEncoding{traceanalysis.RangeEncodingText, traceanalysis.RangeEncodingBase64} {
		for _, size := range []int{1 << 20, 4 << 20, maxTraceRangeBytes} {
			name := fmt.Sprintf("%s-%dMiB", encoding, size>>20)
			t.Run(name, func(t *testing.T) {
				unit := []byte("loomspan")
				if encoding == traceanalysis.RangeEncodingBase64 {
					unit = []byte{0xff, 0x00, 0x80, 0x7f}
				}
				source := bytes.Repeat(unit, size/len(unit))
				content := source
				if encoding == traceanalysis.RangeEncodingBase64 {
					content = []byte(base64.StdEncoding.EncodeToString(source))
				}
				handle := artifact.Handle(strings.Repeat("a", 64))
				analysis := &immutableRangeAnalysis{result: traceanalysis.ByteRangeResult{
					Context: traceanalysis.TraceContext{Evidence: evidence.ForImported(), Handle: handle, TraceID: "large-trace", SessionID: "large-session"},
					Source:  traceanalysis.RangeSourceRawArtifact, ActualEnd: int64(len(source)), TotalLength: int64(len(source)),
					ContentType: "application/octet-stream", Encoding: encoding, Content: content,
				}}
				session := semanticFixtureSession(t, analysis)
				ctx, cancel := context.WithTimeout(context.Background(), requestTimeout+5*time.Second)
				defer cancel()
				started := time.Now()
				call, err := session.CallTool(ctx, &mcp.CallToolParams{Name: ReadTraceArtifactToolName, Arguments: map[string]any{
					"source": "IMPORTED", "artifactHandle": string(handle), "start": 0, "maxBytes": size,
				}})
				if err != nil || call == nil || call.IsError {
					t.Fatalf("large MCP range failed after %s: result=%#v err=%v", time.Since(started), call, err)
				}
				result := call.StructuredContent.(map[string]any)["result"].(map[string]any)
				wireContent := result["content"].(string)
				decoded := []byte(wireContent)
				if encoding == traceanalysis.RangeEncodingBase64 {
					decoded, err = base64.StdEncoding.DecodeString(wireContent)
					if err != nil {
						t.Fatal(err)
					}
				}
				if sha256.Sum256(decoded) != sha256.Sum256(source) || int64(result["actualEnd"].(float64)) != int64(len(source)) {
					t.Fatal("serialized MCP range changed source bytes or offsets")
				}
				fallback := call.Content[0].(*mcp.TextContent).Text
				if !strings.Contains(fallback, wireContent[:min(64, len(wireContent))]) {
					t.Fatal("fact-complete fallback omitted range content")
				}
			})
		}
	}
}

func TestMCPRangeSerializationRemainsResponsiveUnderConcurrentClients(t *testing.T) {
	if testing.Short() {
		t.Skip("selected-maximum concurrent MCP framing gate")
	}
	const size = maxTraceRangeBytes
	handle := artifact.Handle(strings.Repeat("b", 64))
	content := bytes.Repeat([]byte("x"), size)
	analysis := &immutableRangeAnalysis{result: traceanalysis.ByteRangeResult{
		Context: traceanalysis.TraceContext{Evidence: evidence.ForImported(), Handle: handle, TraceID: "concurrent-trace", SessionID: "concurrent-session"},
		Source:  traceanalysis.RangeSourceRawArtifact, ActualEnd: size, TotalLength: size, ContentType: "text/plain", Encoding: traceanalysis.RangeEncodingText, Content: content,
	}}
	sessions := []*mcp.ClientSession{semanticFixtureSession(t, analysis), semanticFixtureSession(t, analysis)}
	start := make(chan struct{})
	errs := make(chan error, len(sessions))
	var wg sync.WaitGroup
	for _, session := range sessions {
		wg.Add(1)
		go func(session *mcp.ClientSession) {
			defer wg.Done()
			<-start
			ctx, cancel := context.WithTimeout(context.Background(), requestTimeout+5*time.Second)
			defer cancel()
			call, err := session.CallTool(ctx, &mcp.CallToolParams{Name: ReadTraceArtifactToolName, Arguments: map[string]any{"source": "IMPORTED", "artifactHandle": string(handle), "start": 0, "maxBytes": size}})
			if err != nil || call == nil || call.IsError {
				errs <- fmt.Errorf("concurrent range: result=%#v err=%w", call, err)
			}
		}(session)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

func TestMCPRangeSchemaRejectsThirtyTwoAndSixtyFourMiBCandidates(t *testing.T) {
	handle := strings.Repeat("c", 64)
	session := semanticFixtureSession(t, &immutableRangeAnalysis{})
	for _, size := range []int{32 << 20, 64 << 20} {
		call, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: ReadTraceArtifactToolName, Arguments: map[string]any{"source": "IMPORTED", "artifactHandle": handle, "start": 0, "maxBytes": size}})
		if err == nil && call != nil && !call.IsError {
			t.Fatalf("candidate %d unexpectedly passed the selected %d-byte ceiling", size, maxTraceRangeBytes)
		}
	}
}

func TestMCPRangeAllocationBudgetSelectsSixteenMiB(t *testing.T) {
	if testing.Short() {
		t.Skip("large MCP materialization allocation gate")
	}
	// The per-call budget covers adapter fallback plus structured-result JSON
	// materialization. It is intentionally below the observed cost of the next
	// candidate, leaving memory for concurrent clients and the rest of Console.
	const allocationBudget = uint64(512 << 20)
	firstRejected := 0
	for _, size := range []int{maxTraceRangeBytes, 32 << 20, 64 << 20} {
		healthy := true
		for _, encoding := range []traceanalysis.RangeEncoding{traceanalysis.RangeEncodingText, traceanalysis.RangeEncodingBase64} {
			measurement := measureRangeMaterialization(t, size, encoding)
			t.Logf("candidate=%dMiB encoding=%s adapter-allocation=%dMiB live-heap-growth=%dMiB budget=%dMiB", size>>20, encoding, measurement.totalAlloc>>20, measurement.liveHeapGrowth>>20, allocationBudget>>20)
			healthy = healthy && measurement.totalAlloc <= allocationBudget && measurement.liveHeapGrowth <= allocationBudget
		}
		if !healthy {
			firstRejected = size
			break // Evaluate 64 MiB only when the preceding candidate is healthy.
		}
	}
	if firstRejected != 32<<20 {
		t.Fatalf("range selection is stale: first rejected candidate=%dMiB, want 32MiB", firstRejected>>20)
	}
}

type rangeMaterializationMeasurement struct {
	totalAlloc     uint64
	liveHeapGrowth uint64
}

func measureRangeMaterialization(t *testing.T, size int, encoding traceanalysis.RangeEncoding) rangeMaterializationMeasurement {
	t.Helper()
	unit := []byte("loomspan")
	if encoding == traceanalysis.RangeEncodingBase64 {
		unit = []byte{0xff, 0x00, 0x80, 0x7f}
	}
	source := bytes.Repeat(unit, size/len(unit))
	content := source
	if encoding == traceanalysis.RangeEncodingBase64 {
		content = []byte(base64.StdEncoding.EncodeToString(source))
	}
	handle := artifact.Handle(strings.Repeat("e", 64))
	analysis := &immutableRangeAnalysis{result: traceanalysis.ByteRangeResult{
		Context:   traceanalysis.TraceContext{Evidence: evidence.ForImported(), Handle: handle},
		ActualEnd: int64(size), TotalLength: int64(size), Encoding: encoding, Content: content,
	}}
	options := ServerOptions{Credentials: fakeCredentials{}, Now: time.Now, TraceAnalysis: analysis}
	start := int64(0)
	runtime.GC()
	previousGCPercent := debug.SetGCPercent(-1)
	defer debug.SetGCPercent(previousGCPercent)
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	call, envelope, err := handleTraceRange(context.Background(), options, traceRangeInput{Source: "IMPORTED", ArtifactHandle: string(handle), Start: &start, MaxBytes: size}, true)
	if err != nil || call == nil || call.IsError {
		t.Fatalf("candidate %d materialization failed: result=%#v err=%v", size, call, err)
	}
	wire, err := json.Marshal(struct {
		Call     *mcp.CallToolResult `json:"call"`
		Envelope any                 `json:"structuredContent"`
	}{call, envelope})
	if err != nil {
		t.Fatal(err)
	}
	runtime.ReadMemStats(&after)
	if !bytes.Contains(wire, content[:min(64, len(content))]) {
		t.Fatal("candidate fallback was not fact-complete")
	}
	runtime.KeepAlive(source)
	runtime.KeepAlive(content)
	return rangeMaterializationMeasurement{totalAlloc: after.TotalAlloc - before.TotalAlloc, liveHeapGrowth: after.HeapAlloc - before.HeapAlloc}
}

func BenchmarkMCPRangeMaterialization(b *testing.B) {
	for _, encoding := range []traceanalysis.RangeEncoding{traceanalysis.RangeEncodingText, traceanalysis.RangeEncodingBase64} {
		b.Run(string(encoding), func(b *testing.B) {
			source := bytes.Repeat([]byte("x"), maxTraceRangeBytes)
			content := source
			if encoding == traceanalysis.RangeEncodingBase64 {
				content = []byte(base64.StdEncoding.EncodeToString(source))
			}
			handle := artifact.Handle(strings.Repeat("d", 64))
			analysis := &immutableRangeAnalysis{result: traceanalysis.ByteRangeResult{Context: traceanalysis.TraceContext{Evidence: evidence.ForImported(), Handle: handle}, ActualEnd: maxTraceRangeBytes, TotalLength: maxTraceRangeBytes, Encoding: encoding, Content: content}}
			options := ServerOptions{Credentials: fakeCredentials{}, Now: time.Now, TraceAnalysis: analysis}
			start := int64(0)
			b.ReportAllocs()
			b.SetBytes(maxTraceRangeBytes)
			for range b.N {
				call, envelope, err := handleTraceRange(context.Background(), options, traceRangeInput{Source: "IMPORTED", ArtifactHandle: string(handle), Start: &start, MaxBytes: maxTraceRangeBytes}, true)
				if err != nil {
					b.Fatal(err)
				}
				if _, err := json.Marshal(struct {
					Call     *mcp.CallToolResult `json:"call"`
					Envelope any                 `json:"structuredContent"`
				}{call, envelope}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
