package mcpadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/applicationclient"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/live"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/mcpcredential"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/profile"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type authTransport struct {
	base      http.RoundTripper
	host, key string
}

func TestCompatible2025ProtocolInitializesListsAndCallsRealRuntimeTool(t *testing.T) {
	const (
		prePR28ToolsListResponseBytes  = 34371
		expectedToolsListResponseBytes = 20304
	)
	credentials := fakeCredentials{state: mcpcredential.Snapshot{State: mcpcredential.Enabled, Generation: 4}, key: "secret"}
	options := newMCPTestOptions(t, func(endpoint string) ([]byte, error) {
		if strings.Contains(endpoint, "/skills?") {
			return []byte(`{"items":[{"registeredName":"skill-☃","sourcePath":"nested/skill.yaml"}],"hasMore":false,"nextCursor":null,"observedAt":"2026-08-13T20:30:00Z"}`), nil
		}
		if strings.Contains(endpoint, "/skills/") {
			if strings.HasSuffix(endpoint, "/missing") {
				return nil, &applicationclient.Failure{Kind: applicationclient.FailureNotFound}
			}
			return []byte(`{"registeredName":"skill-☃","sourcePath":"nested/skill.yaml","yaml":"name: skill-☃\n"}`), nil
		}
		return nil, fmt.Errorf("unexpected endpoint: %s", endpoint)
	})
	options.Credentials = credentials
	server := NewServer(options)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	lastResponseBytes := 0
	var lastResponseBody []byte
	post := func(payload string) map[string]any {
		t.Helper()
		request, err := http.NewRequest(http.MethodPost, httpServer.URL, bytes.NewBufferString(payload))
		if err != nil {
			t.Fatal(err)
		}
		request.Host = "127.0.0.1:7345"
		request.Header.Set("Authorization", "Bearer secret")
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
		lastResponseBytes = len(body)
		lastResponseBody = append(lastResponseBody[:0], body...)
		var decoded map[string]any
		if err := json.Unmarshal(body, &decoded); err != nil {
			t.Fatalf("decode %s: %v", body, err)
		}
		return decoded
	}
	initialized := post(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"compatibility-test","version":"1"}}}`)
	result := initialized["result"].(map[string]any)
	if result["protocolVersion"] != "2025-11-25" {
		t.Fatalf("negotiated protocol = %v", result["protocolVersion"])
	}
	listed := post(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	toolsListBytes := lastResponseBytes
	bodyForSnapshot := append([]byte(nil), lastResponseBody...)
	if toolsListBytes > maxToolsListResponseBytes {
		t.Fatalf("tools/list response=%d bytes discovery budget=%d", toolsListBytes, maxToolsListResponseBytes)
	}
	if toolsListBytes != expectedToolsListResponseBytes {
		t.Fatalf("tools/list serialized response=%d bytes expected=%d pre-PR28=%d", toolsListBytes, expectedToolsListResponseBytes, prePR28ToolsListResponseBytes)
	}
	snapshotPath := filepath.Join("testdata", "tools-list-response.json")
	if os.Getenv("LOOMSPAN_UPDATE_SNAPSHOTS") == "1" {
		if err := os.WriteFile(snapshotPath, bodyForSnapshot, 0o644); err != nil {
			t.Fatalf("write tools/list snapshot: %v", err)
		}
	}
	expectedSnapshot, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatalf("read tools/list snapshot: %v", err)
	}
	if !bytes.Equal(bodyForSnapshot, expectedSnapshot) {
		t.Fatalf("tools/list response does not match %s", snapshotPath)
	}
	tools := listed["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 12 || !rawToolNamesContain(tools, RuntimeToolName, ListSkillsToolName, GetSkillToolName, ListExecutionsToolName, GetExecutionToolName, GetExecutionActivityToolName, ListTracesToolName, GetTraceToolName, QueryTraceFramesToolName, QueryTraceRecordsToolName, ReadTraceContentToolName, ReadTraceArtifactToolName) {
		t.Fatalf("tools = %+v", tools)
	}
	called := post(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"LOOMSPAN_get_runtime","arguments":{}}}`)
	callResult := called["result"].(map[string]any)
	structured := callResult["structuredContent"].(map[string]any)
	capabilities := structured["capabilities"].([]any)
	if len(capabilities) != 6 || capabilities[0] != RuntimeStatusCapability || callResult["isError"] == true {
		t.Fatalf("runtime result = %+v", callResult)
	}
	skillCall := post(`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"LOOMSPAN_list_skills","arguments":{"pageSize":1}}}`)
	if skillCall["result"].(map[string]any)["isError"] == true {
		t.Fatalf("skill result = %+v", skillCall)
	}
	templateList := post(`{"jsonrpc":"2.0","id":5,"method":"resources/templates/list","params":{}}`)
	templates := templateList["result"].(map[string]any)["resourceTemplates"].([]any)
	if len(templates) != 0 {
		t.Fatalf("resource templates = %+v", templates)
	}
}

func (transport authTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Host = transport.host
	clone.Header.Set("Authorization", "Bearer "+transport.key)
	return transport.base.RoundTrip(clone)
}

func TestStatelessStreamableHTTPInitializesListsAndCallsRuntime(t *testing.T) {
	owned, err := profile.Open(filepath.Join(t.TempDir(), "profile", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	defer owned.Close()
	store, err := mcpcredential.Open(owned.Directory, nil)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := store.Prepare()
	if err != nil {
		t.Fatal(err)
	}
	key, err := store.CommitEnable(prepared)
	if err != nil {
		t.Fatal(err)
	}
	tracker := NewTracker()
	options := newMCPTestOptions(t, func(endpoint string) ([]byte, error) {
		if strings.Contains(endpoint, "/skills?") {
			return []byte(`{"items":[{"registeredName":"skill-☃","sourcePath":"nested/skill.yaml"}],"hasMore":false,"nextCursor":null,"observedAt":"2026-08-13T20:30:00Z"}`), nil
		}
		if strings.Contains(endpoint, "/skills/") {
			if strings.HasSuffix(endpoint, "/missing") {
				return nil, &applicationclient.Failure{Kind: applicationclient.FailureNotFound}
			}
			return []byte(`{"registeredName":"skill-☃","sourcePath":"nested/skill.yaml","yaml":"name: skill-☃\n"}`), nil
		}
		return nil, fmt.Errorf("unexpected endpoint: %s", endpoint)
	})
	options.Port, options.Credentials, options.Tracker = 7345, store, tracker
	server := NewServer(options)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	clientHTTP := &http.Client{Transport: authTransport{base: http.DefaultTransport, host: "127.0.0.1:7345", key: key}}
	client := mcp.NewClient(&mcp.Implementation{Name: "loomspan-test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: httpServer.URL, HTTPClient: clientHTTP, DisableStandaloneSSE: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if got := session.InitializeResult().ServerInfo.Name; got != "loomspan-console" {
		t.Fatalf("server name = %q", got)
	}
	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) != 12 || !toolNamesContain(tools.Tools, RuntimeToolName, ListSkillsToolName, GetSkillToolName, ListExecutionsToolName, GetExecutionToolName, GetExecutionActivityToolName, ListTracesToolName, GetTraceToolName, QueryTraceFramesToolName, QueryTraceRecordsToolName, ReadTraceContentToolName, ReadTraceArtifactToolName) {
		t.Fatalf("tools = %+v", tools.Tools)
	}
	for _, tool := range tools.Tools {
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint || !tool.Annotations.IdempotentHint ||
			tool.Annotations.DestructiveHint == nil || *tool.Annotations.DestructiveHint ||
			tool.Annotations.OpenWorldHint == nil || *tool.Annotations.OpenWorldHint {
			t.Fatalf("tool %s annotations = %#v", tool.Name, tool.Annotations)
		}
	}
	templates, err := session.ListResourceTemplates(context.Background(), nil)
	if err != nil || len(templates.ResourceTemplates) != 0 {
		t.Fatalf("resource templates=%#v err=%v", templates, err)
	}
	var runtimeTool *mcp.Tool
	for _, tool := range tools.Tools {
		if tool.Name == RuntimeToolName {
			runtimeTool = tool
			break
		}
	}
	if runtimeTool == nil {
		t.Fatal("runtime tool was not discovered")
	}
	schema, err := json.Marshal(runtimeTool.InputSchema)
	if err != nil {
		t.Fatal(err)
	}
	if string(schema) != `{"additionalProperties":false,"type":"object"}` {
		t.Fatalf("runtime input schema = %s", schema)
	}
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: RuntimeToolName, Arguments: map[string]any{}})
	if err != nil || result.IsError || result.StructuredContent == nil || len(result.Content) != 1 {
		t.Fatalf("call = %+v, %v", result, err)
	}
	invalid, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: RuntimeToolName, Arguments: map[string]any{"unexpected": true}})
	if err != nil {
		t.Fatal(err)
	}
	if !invalid.IsError {
		t.Fatalf("runtime accepted unknown input: %+v", invalid)
	}
	skills, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: ListSkillsToolName, Arguments: map[string]any{"pageSize": 1}})
	if err != nil || skills.IsError || skills.StructuredContent == nil || len(skills.Content) != 1 {
		t.Fatalf("skills=%#v err=%v", skills, err)
	}
	missing, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: GetSkillToolName, Arguments: map[string]any{"registeredName": "missing"}})
	if err != nil || missing == nil || !missing.IsError || len(missing.Content) != 1 {
		t.Fatalf("missing skill=%#v err=%v", missing, err)
	}
	missingEnvelope, ok := missing.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("missing skill structured content type = %T", missing.StructuredContent)
	}
	missingError, ok := missingEnvelope["error"].(map[string]any)
	if !ok || missingError["code"] != string(consolecore.CodeNotFound) {
		t.Fatalf("missing skill envelope = %#v", missingEnvelope)
	}
	if text := missing.Content[0].(*mcp.TextContent).Text; text != "NOT_FOUND: The requested observability resource was not found." {
		t.Fatalf("missing skill text = %q", text)
	}
	for _, arguments := range []map[string]any{{}, {"pageSize": 0}, {"pageSize": 65}, {"pageSize": 1, "extra": true}} {
		invalid, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: ListSkillsToolName, Arguments: arguments})
		if err != nil || !invalid.IsError || invalid.StructuredContent != nil {
			t.Fatalf("invalid arguments %#v result=%#v err=%v", arguments, invalid, err)
		}
	}
}

func TestPR17ToolCancellationCancelsUpstreamAndSuppressesResult(t *testing.T) {
	started := make(chan struct{})
	client := &mcpTestTargetClient{getContext: func(ctx context.Context, _ string) ([]byte, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	options := newMCPTestOptionsWithClient(t, client)
	server := NewServer(options)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	session := connectMCPTestSession(t, httpServer.URL, "mcp-secret")
	defer session.Close()

	ctx, cancel := context.WithCancel(context.Background())
	completed := make(chan error, 1)
	go func() {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: ListSkillsToolName, Arguments: map[string]any{"pageSize": 1}})
		if result != nil {
			completed <- fmt.Errorf("canceled call published a result: %#v", result)
			return
		}
		completed <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("skill request did not reach the upstream client")
	}
	cancel()
	select {
	case err := <-completed:
		if err == nil {
			t.Fatal("canceled call returned neither a result nor an error")
		}
	case <-time.After(time.Second):
		t.Fatal("canceled skill request did not finish")
	}
}

func TestTwoMCPClientsShareLiveWindowButCancelIndependently(t *testing.T) {
	var openCalls atomic.Int32
	client := &mcpTestTargetClient{
		get: func(string) ([]byte, error) { return nil, nil },
		openActivity: func(context.Context, string, string, applicationclient.Credential) (*applicationclient.ActivityStream, error) {
			openCalls.Add(1)
			return nil, &applicationclient.Failure{Kind: applicationclient.FailureLiveMonitoringUnavailable}
		},
	}
	options := newMCPTestOptionsWithClient(t, client)
	liveService := live.NewService(context.Background())
	t.Cleanup(liveService.Close)
	scope, domain := options.Target.Capture()
	if domain != nil {
		t.Fatal(domain)
	}
	liveService.ActivateActivity(scope)
	options.Live = liveService
	deadline := time.Now().Add(time.Second)
	for !liveService.LiveUnavailable() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !liveService.LiveUnavailable() || openCalls.Load() != 1 {
		t.Fatalf("shared live service state unavailable=%t upstream opens=%d", liveService.LiveUnavailable(), openCalls.Load())
	}

	server := NewServer(options)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	first := connectMCPTestSession(t, httpServer.URL, "mcp-secret")
	second := connectMCPTestSession(t, httpServer.URL, "mcp-secret")
	defer second.Close()
	arguments := map[string]any{"sessionId": "session-1", "pageSize": 1}
	firstResult, err := first.CallTool(context.Background(), &mcp.CallToolParams{Name: GetExecutionActivityToolName, Arguments: arguments})
	if err != nil || !firstResult.IsError {
		t.Fatalf("first activity result=%#v err=%v", firstResult, err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	secondResult, err := second.CallTool(context.Background(), &mcp.CallToolParams{Name: GetExecutionActivityToolName, Arguments: arguments})
	if err != nil || !secondResult.IsError || openCalls.Load() != 1 {
		t.Fatalf("second activity result=%#v err=%v upstream opens=%d", secondResult, err, openCalls.Load())
	}
}

func connectMCPTestSession(t *testing.T, endpoint, key string) *mcp.ClientSession {
	t.Helper()
	httpClient := &http.Client{Transport: authTransport{base: http.DefaultTransport, host: "127.0.0.1:7345", key: key}}
	client := mcp.NewClient(&mcp.Implementation{Name: "loomspan-test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: endpoint, HTTPClient: httpClient, DisableStandaloneSSE: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func rawToolNamesContain(tools []any, names ...string) bool {
	found := make(map[string]bool)
	for _, tool := range tools {
		found[tool.(map[string]any)["name"].(string)] = true
	}
	for _, name := range names {
		if !found[name] {
			return false
		}
	}
	return true
}

func toolNamesContain(tools []*mcp.Tool, names ...string) bool {
	found := make(map[string]bool)
	for _, tool := range tools {
		found[tool.Name] = true
	}
	for _, name := range names {
		if !found[name] {
			return false
		}
	}
	return true
}

func rawTemplateNamesContain(templates []any, names ...string) bool {
	found := make(map[string]bool)
	for _, template := range templates {
		found[template.(map[string]any)["uriTemplate"].(string)] = true
	}
	for _, name := range names {
		if !found[name] {
			return false
		}
	}
	return true
}

func templateNamesContain(templates []*mcp.ResourceTemplate, names ...string) bool {
	found := make(map[string]bool)
	for _, template := range templates {
		found[template.URITemplate] = true
	}
	for _, name := range names {
		if !found[name] {
			return false
		}
	}
	return true
}
