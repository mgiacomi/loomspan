package mcpadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/mcpcredential"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/profile"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type authTransport struct {
	base      http.RoundTripper
	host, key string
}

func TestCompatible2025ProtocolInitializesListsAndCallsRealRuntimeTool(t *testing.T) {
	credentials := fakeCredentials{state: mcpcredential.Snapshot{State: mcpcredential.Enabled, Generation: 4}, key: "secret"}
	server := NewServer(7345, credentials, NewTracker(), func() consolecore.StatusSnapshot {
		return consolecore.NoTargetStatus(time.Unix(1, 0).UTC())
	})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
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
	tools := listed["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 1 || tools[0].(map[string]any)["name"] != RuntimeToolName {
		t.Fatalf("tools = %+v", tools)
	}
	called := post(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"LOOMSPAN_get_runtime","arguments":{}}}`)
	callResult := called["result"].(map[string]any)
	structured := callResult["structuredContent"].(map[string]any)
	capabilities := structured["capabilities"].([]any)
	if len(capabilities) != 1 || capabilities[0] != RuntimeStatusCapability || callResult["isError"] == true {
		t.Fatalf("runtime result = %+v", callResult)
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
	server := NewServer(7345, store, tracker, func() consolecore.StatusSnapshot { return consolecore.NoTargetStatus(time.Unix(1, 0).UTC()) })
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
	if len(tools.Tools) != 1 || tools.Tools[0].Name != RuntimeToolName {
		t.Fatalf("tools = %+v", tools.Tools)
	}
	schema, err := json.Marshal(tools.Tools[0].InputSchema)
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
}
