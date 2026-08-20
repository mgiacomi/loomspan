package mcpadapter

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/evidence"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/traceanalysis"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/traceinventory"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestEncodedPageBudgetAdmitsOnlyWholeItems(t *testing.T) {
	for _, value := range []string{
		"plain",
		"☃ multibyte",
		"quotes=\" controls=\x00\n backslash=\\",
		base64.StdEncoding.EncodeToString([]byte{0xff, 0x00, 0x80, 0x7f}),
	} {
		structured := map[string]any{"value": value}
		fallback := "value=" + value + "\n"
		item, _ := json.Marshal(structured)
		line, _ := json.Marshal(fallback)
		exact := len(item) + len(line) + 2
		for _, boundary := range []struct {
			name  string
			delta int
			want  bool
		}{{"below", 1, true}, {"at", 0, true}, {"above", -1, false}} {
			t.Run(boundary.name+"/"+value, func(t *testing.T) {
				admission := &pageAdmission{budget: 10 + exact + boundary.delta, used: 10}
				if got := admission.admit(structured, fallback); got != boundary.want {
					t.Fatalf("admitted=%t want=%t exact=%d budget=%d", got, boundary.want, exact, admission.budget)
				}
			})
		}
	}
	candidate := &countedBudgetCandidate{Value: "once"}
	admission := &pageAdmission{budget: 1024}
	admitted := admission.admit(candidate, "once\n")
	if !admitted || candidate.MarshalCalls != 1 {
		t.Fatalf("candidate admitted=%t marshalCalls=%d", admitted, candidate.MarshalCalls)
	}
}

type countedBudgetCandidate struct {
	Value        string
	MarshalCalls int
}

func (candidate *countedBudgetCandidate) MarshalJSON() ([]byte, error) {
	candidate.MarshalCalls++
	return json.Marshal(struct {
		Value string `json:"value"`
	}{candidate.Value})
}

func TestFullSerializedDefaultTraceResultsMeetCommittedBudgets(t *testing.T) {
	large := strings.Repeat("x", maxTraceTokenLength)
	evidence := evidenceDTO{TraceID: large, SessionID: "session", ObservedAt: time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)}
	measurements := map[string]int{}

	inventory := listTracesResult{ObservedAt: evidence.ObservedAt, Complete: true, HasMore: true, Continuation: large}
	inventoryAdmission := newPageAdmission()
	for index := range 64 {
		traceID := "trace-" + strings.Repeat("i", 240) + string(rune('a'+index%26))
		item := traceInventoryItemDTO{TraceID: traceID}
		if !inventoryAdmission.admit(item, traceInventoryFallbackLine(item)) {
			break
		}
		inventory.Items = append(inventory.Items, item)
	}
	measurements["inventory"] = assertCompleteResultBudget(t, "inventory", inventory, traceListText(inventory), defaultTraceResultBudget)

	frames := queryFramesResult{Evidence: evidence, Projection: string(traceanalysis.FrameProjectionDetailed), HasMore: true, Continuation: large}
	frameAdmission := newPageAdmission()
	for range 64 {
		item := frameDTO{FrameID: large, ChildFrameIDs: []string{}, FrameType: "MODEL_CALL", Route: "☃\\\"", Outcome: stringPointer("failed"), detailed: true}
		if !frameAdmission.admit(item, frameFallbackLine(item, traceanalysis.FrameProjectionDetailed)) {
			break
		}
		frames.Items = append(frames.Items, item)
	}
	measurements["detailed frames"] = assertCompleteResultBudget(t, "detailed frames", frames, traceFramesText(frames), defaultTraceResultBudget)
	compactFrames := frames
	compactFrames.Projection = string(traceanalysis.FrameProjectionCompact)
	for index := range compactFrames.Items {
		compactFrames.Items[index].detailed = false
	}
	measurements["compact frames"] = assertCompleteResultBudget(t, "compact frames", compactFrames, traceFramesText(compactFrames), defaultTraceResultBudget)

	records := queryRecordsResult{Evidence: evidence, HasMore: true, Continuation: large}
	recordAdmission := newPageAdmission()
	for index := range 64 {
		item := recordDTO{Sequence: int64(index + 1), Type: "MODEL_RESPONSE_RECEIVED", FrameID: strings.Repeat("f", 2048), ThreadName: "main", Representation: "logical", Facts: recordFactsDTO{Attempts: []attemptDTO{}, Retries: []retryDTO{}, Validations: []validationDTO{}, Failures: []failureDTO{}, SearchMatches: []searchMatchDTO{}}, Content: &contentDescriptorDTO{Role: "DATA", ContentType: "application/json", Encoding: "UTF8", Available: true, Complete: true, ContentRef: strings.Repeat("r", 4096)}}
		if !recordAdmission.admit(item, recordFallbackLine(item)) {
			break
		}
		records.Items = append(records.Items, item)
	}
	measurements["record descriptors"] = assertCompleteResultBudget(t, "record descriptors", records, traceRecordsText(records), defaultTraceResultBudget)

	inlineEvidence := evidenceDTO{TraceID: "trace-inline", SessionID: "session", ObservedAt: evidence.ObservedAt}
	inlineRecords := queryRecordsResult{Evidence: inlineEvidence, HasMore: true, Continuation: "opaque"}
	inlineAdmission := newInlineRecordPageAdmission(inlineEvidence.TraceID)
	for index := range 4 {
		item := recordDTO{Sequence: int64(index + 1), Type: "MODEL_RESPONSE_RECEIVED", ThreadName: "main", Representation: "logical", Facts: recordFactsDTO{Attempts: []attemptDTO{}, Retries: []retryDTO{}, Validations: []validationDTO{}, Failures: []failureDTO{}, SearchMatches: []searchMatchDTO{}}, Content: &contentDescriptorDTO{Role: "DATA", ContentType: "application/json", Encoding: "UTF8", RetainedBytes: traceanalysis.MaxInlineContentBytes, Available: true, Complete: true, InlineEligibility: true, ContentRef: "opaque", InlineContent: strings.Repeat("v", traceanalysis.MaxInlineContentBytes)}}
		if !inlineAdmission.admit(item, recordFallbackLine(item)) {
			break
		}
		inlineRecords.Items = append(inlineRecords.Items, item)
	}
	measurements["inline records"] = assertCompleteResultBudget(t, "inline records", inlineRecords, traceRecordsText(inlineRecords), defaultTraceResultBudget)

	worstText := strings.Repeat("\x00", traceanalysis.DefaultRangeBytes)
	textRange := rangeResult{Evidence: evidence, ActualEnd: int64(len(worstText)), TotalLength: int64(len(worstText) + 1), ContentType: "text/plain", Encoding: "TEXT", Content: worstText, HasMore: true, Continuation: large}
	measurements["semantic text range"] = assertCompleteResultBudget(t, "semantic text range", textRange, traceRangeText(textRange), defaultRangeResultBudget)
	binary := base64.StdEncoding.EncodeToString(make([]byte, traceanalysis.DefaultRangeBytes))
	binaryRange := rangeResult{Evidence: evidence, ActualEnd: int64(traceanalysis.DefaultRangeBytes), TotalLength: int64(traceanalysis.DefaultRangeBytes + 1), ContentType: "application/octet-stream", Encoding: "BASE64", Content: binary, HasMore: true, Continuation: large}
	measurements["raw base64 range"] = assertCompleteResultBudget(t, "raw base64 range", binaryRange, traceRangeText(binaryRange), defaultRangeResultBudget)
	want := map[string]int{
		"inventory":           21091,
		"detailed frames":     27035,
		"compact frames":      26860,
		"record descriptors":  25440,
		"inline records":      17337,
		"semantic text range": 45527,
		"raw base64 range":    36007,
	}
	for name, expected := range want {
		if measurements[name] != expected {
			t.Fatalf("%s serialized result=%d expected=%d", name, measurements[name], expected)
		}
	}
}

func assertCompleteResultBudget[T any](t *testing.T, name string, structured T, fallback string, budget int) int {
	t.Helper()
	call := &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fallback}}}
	encoded, err := json.Marshal(struct {
		Content           []mcp.Content   `json:"content"`
		StructuredContent toolEnvelope[T] `json:"structuredContent"`
	}{Content: call.Content, StructuredContent: toolEnvelope[T]{Result: &structured}})
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) > budget {
		t.Fatalf("%s serialized result=%d budget=%d", name, len(encoded), budget)
	}
	t.Logf("%s serialized result: %d bytes (budget %d)", name, len(encoded), budget)
	return len(encoded)
}

func TestFullHTTPDefaultTraceResponsesMeetCommittedBudgetsAndFallbacksMatch(t *testing.T) {
	now := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	handle := artifact.Handle(strings.Repeat("a", 64))
	context := traceanalysis.TraceContext{Evidence: evidence.ForImported(), Handle: handle, TraceID: strings.Repeat("t", maxTraceTokenLength), SessionID: "session-budget"}
	inventory := &fakeTraceInventory{result: traceinventory.Result{ObservedAt: now, Complete: true}}
	analysis := &fakeTraceAnalysis{}
	for index := range 128 {
		traceID := "trace-" + strings.Repeat("i", 220) + string(rune('a'+index%26)) + string(rune('A'+index/26))
		inventory.result.Items = append(inventory.result.Items, traceinventory.Entry{TraceID: traceID, EvidenceSources: []traceinventory.EvidenceSource{traceinventory.SourceImported}})
		frameID := "frame-" + strings.Repeat("f", 170) + string(rune('a'+index%26)) + string(rune('A'+index/26))
		analysis.frames.Items = append(analysis.frames.Items, traceanalysis.FrameSummary{Context: context, FrameID: frameID, ChildFrameIDs: []string{}, FrameType: "MODEL_CALL", Route: "route-☃-\\\"", SkillNames: []string{}, AttemptIDs: []string{}, RetrySequenceIDs: []string{}, ValidationStatuses: []string{}, FailureIDs: []string{}, GapKinds: []string{}, UncertaintyKinds: []string{}})
		analysis.records.Items = append(analysis.records.Items, budgetRecord(context, index, false))
	}
	analysis.frames.Context = context
	analysis.records.Context = context
	analysis.payload = traceanalysis.ByteRangeResult{Context: context, Source: traceanalysis.RangeSourceContent, ActualEnd: traceanalysis.DefaultRangeBytes, TotalLength: traceanalysis.DefaultRangeBytes + 1, ContentType: "text/plain", Encoding: traceanalysis.RangeEncodingText, Content: bytes.Repeat([]byte{0}, traceanalysis.DefaultRangeBytes), HasMore: true, NextCursor: "content-next"}
	analysis.raw = traceanalysis.ByteRangeResult{Context: context, Source: traceanalysis.RangeSourceRawArtifact, ActualEnd: traceanalysis.DefaultRangeBytes, TotalLength: traceanalysis.DefaultRangeBytes + 1, ContentType: "application/octet-stream", Encoding: traceanalysis.RangeEncodingBase64, Content: bytes.Repeat([]byte{0xff}, traceanalysis.DefaultRangeBytes), HasMore: true, NextCursor: "raw-next"}

	options := newMCPTestOptions(t, func(string) ([]byte, error) { return nil, nil })
	options.TraceInventory = inventory
	options.TraceAnalysis = analysis
	options.TraceResolver = &fakeTraceArtifacts{result: artifact.AcquiredArtifact{Handle: handle}, ref: evidence.ForImported()}
	httpServer := httptest.NewServer(NewServer(options).Handler())
	defer httpServer.Close()
	post := newBudgetHTTPPoster(t, httpServer.URL)
	post(1, "initialize", map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "budget-test", "version": "1"}})

	measurements := map[string]int{}
	assertNavigation := func(name, tool string, arguments map[string]any, collection, fallbackMarker string) {
		t.Helper()
		body, decoded := post(len(measurements)+2, "tools/call", map[string]any{"name": tool, "arguments": arguments})
		measurements[name] = len(body)
		if len(body) > defaultTraceResultBudget {
			t.Fatalf("%s full HTTP response=%d budget=%d", name, len(body), defaultTraceResultBudget)
		}
		result := decoded["result"].(map[string]any)
		if result["isError"] == true {
			t.Fatalf("%s returned error: %s", name, body)
		}
		structured := result["structuredContent"].(map[string]any)["result"].(map[string]any)
		fallback := result["content"].([]any)[0].(map[string]any)["text"].(string)
		items := structured[collection].([]any)
		if !strings.Contains(fallback, "hasMore=") || !strings.Contains(fallback, `continuation="`) {
			t.Fatalf("%s fallback omitted pagination facts: %q", name, fallback)
		}
		if count := strings.Count(fallback, fallbackMarker); count != len(items) {
			t.Fatalf("%s fallback marker count=%d structured items=%d", name, count, len(items))
		}
	}
	assertNavigation("inventory", ListTracesToolName, map[string]any{"pageSize": 64}, "items", "traceId=")
	assertNavigation("compact frames", QueryTraceFramesToolName, map[string]any{"traceId": "trace-budget", "projection": "COMPACT", "pageSize": 64}, "items", "frameId=")
	assertNavigation("detailed frames", QueryTraceFramesToolName, map[string]any{"traceId": "trace-budget", "projection": "DETAILED", "pageSize": 64}, "items", "frameId=")
	assertNavigation("record descriptors", QueryTraceRecordsToolName, map[string]any{"traceId": "trace-budget", "pageSize": 64}, "items", "sequence=")

	analysis.records.Items = analysis.records.Items[:0]
	inlineContext := context
	inlineContext.TraceID = "trace-budget"
	analysis.records.Context = inlineContext
	for index := range 16 {
		analysis.records.Items = append(analysis.records.Items, budgetRecord(inlineContext, index, true))
	}
	assertNavigation("inline records", QueryTraceRecordsToolName, map[string]any{"traceId": "trace-budget", "inlineContent": true, "pageSize": 64}, "items", "sequence=")

	analysis.search = traceanalysis.SearchPage{Context: context}
	for index := range 128 {
		contentID := "c" + string(rune('a'+index%26)) + string(rune('A'+index/26))
		analysis.search.Items = append(analysis.search.Items, traceanalysis.SearchResult{Context: context, Sequence: int64(index + 1), RecordType: "MODEL_RESPONSE_RECEIVED", FrameID: "frame-search", MatchOffset: int64(index * 7), MatchLength: 6, SearchedField: "content", ContentID: contentID})
		analysis.search.ContentDescriptors = append(analysis.search.ContentDescriptors, traceanalysis.SearchContentDescriptor{ContentID: contentID, ContentRef: "ref-" + strings.Repeat("r", 180) + contentID})
	}
	assertNavigation("search", QueryTraceRecordsToolName, map[string]any{"traceId": "trace-budget", "filter": map[string]any{"literalText": "needle"}, "pageSize": 64}, "matches", "sequence=")

	assertRange := func(name, tool string, arguments map[string]any) {
		t.Helper()
		body, decoded := post(len(measurements)+2, "tools/call", map[string]any{"name": tool, "arguments": arguments})
		measurements[name] = len(body)
		if len(body) > defaultRangeResultBudget {
			t.Fatalf("%s full HTTP response=%d budget=%d", name, len(body), defaultRangeResultBudget)
		}
		result := decoded["result"].(map[string]any)
		structured := result["structuredContent"].(map[string]any)["result"].(map[string]any)
		fallback := result["content"].([]any)[0].(map[string]any)["text"].(string)
		if result["isError"] == true || strings.Count(fallback, structured["content"].(string)) != 1 {
			t.Fatalf("%s fallback and structured content differ: %s", name, body)
		}
	}
	assertRange("semantic text range", ReadTraceContentToolName, map[string]any{"traceId": "trace-budget", "contentRef": "content-ref"})
	if analysis.rangeRequest.Start != 0 || analysis.rangeRequest.MaxBytes != 0 {
		t.Fatalf("omitted semantic range controls became %+v", analysis.rangeRequest)
	}
	assertRange("raw base64 range", ReadTraceArtifactToolName, map[string]any{"traceId": "trace-budget"})
	if analysis.rangeRequest.Start != 0 || analysis.rangeRequest.MaxBytes != 0 {
		t.Fatalf("omitted raw range controls became %+v", analysis.rangeRequest)
	}
	want := map[string]int{
		"inventory":           12012,
		"compact frames":      21241,
		"detailed frames":     20751,
		"record descriptors":  20077,
		"inline records":      18023,
		"search":              20624,
		"semantic text range": 29225,
		"raw base64 range":    26177,
	}
	for name, size := range measurements {
		if expected := want[name]; size != expected {
			t.Fatalf("%s full HTTP response=%d expected=%d", name, size, want[name])
		}
		t.Logf("%s full HTTP response: %d bytes", name, size)
	}
}

func budgetRecord(context traceanalysis.TraceContext, index int, inline bool) traceanalysis.RecordSummary {
	descriptor := &traceanalysis.ContentDescriptor{Role: traceanalysis.ContentRoleData, ContentType: "application/json", Encoding: traceanalysis.ContentEncodingUTF8, RetainedBytes: 8192, Available: true, Complete: true, InlineEligibility: true, ContentRef: "ref-" + strings.Repeat("r", 300) + string(rune('a'+index%26))}
	if inline {
		descriptor.InlineContent = bytes.Repeat([]byte("v"), traceanalysis.MaxInlineContentBytes)
	}
	return traceanalysis.RecordSummary{Context: context, Sequence: int64(index + 1), Type: "MODEL_RESPONSE_RECEIVED", FrameID: "frame-record", ThreadName: "main", Representation: "logical", Content: descriptor, Facts: traceanalysis.RecordFacts{Attempts: []traceanalysis.AttemptSummary{}, Retries: []traceanalysis.RetrySummary{}, Validations: []traceanalysis.ValidationSummary{}, Failures: []traceanalysis.FailureSummary{}, SearchMatches: []traceanalysis.SearchResult{}}}
}

func newBudgetHTTPPoster(t *testing.T, endpoint string) func(int, string, map[string]any) ([]byte, map[string]any) {
	t.Helper()
	return func(id int, method string, params map[string]any) ([]byte, map[string]any) {
		t.Helper()
		payload, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
		if err != nil {
			t.Fatal(err)
		}
		request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		request.Host = "127.0.0.1:7345"
		request.Header.Set("Authorization", "Bearer mcp-secret")
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", "application/json, text/event-stream")
		request.Header.Set("MCP-Protocol-Version", "2025-11-25")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		if err != nil || response.StatusCode != http.StatusOK {
			t.Fatalf("status=%d body=%s err=%v", response.StatusCode, body, err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(body, &decoded); err != nil {
			t.Fatalf("decode %s: %v", body, err)
		}
		return body, decoded
	}
}
