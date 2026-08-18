package agenteval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/live"
)

func TestEvaluationServerRunsProductionMCPAdapterAndProtectsConnection(t *testing.T) {
	cases, err := LoadCases()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	server, err := StartServer(root, cases["failed-execution"], "0.1.0-SNAPSHOT", "0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Close(ctx)
	}()
	session, err := LoadSession(root)
	if err != nil {
		t.Fatal(err)
	}
	if session.Endpoint != server.Session.Endpoint || session.Key == "" {
		t.Fatal("protected session did not preserve server connection details")
	}
	request, _ := http.NewRequest(http.MethodPost, session.Endpoint, nil)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated production MCP status = %d, want 401", response.StatusCode)
	}
}

func TestSlowEvaluationServerReusesAuthoritativeActiveAndActivityFixtures(t *testing.T) {
	cases, _ := LoadCases()
	server, err := StartServer(t.TempDir(), cases["slow-execution"], "0.1.0-SNAPSHOT", "0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Close(ctx)
	}()
	if server.target == nil || server.live == nil || server.target.Snapshot().Status.LiveMonitoring != consolecore.LiveAvailable {
		t.Fatal("slow evaluation case did not assemble the production live target boundary")
	}
	recent, domain := server.live.Recent(live.RecentRequest{Limit: 64})
	if domain != nil || len(recent.Items) == 0 || recent.Continuity == nil {
		t.Fatalf("slow evaluation recent activity = %#v, domain=%v", recent, domain)
	}
}

func TestSkillWithoutMCPCaseCreatesNoListenerOrCredential(t *testing.T) {
	cases, _ := LoadCases()
	root := t.TempDir()
	server, err := StartServer(root, cases["skill-without-mcp"], "0.1.0-SNAPSHOT", "0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	if server.http != nil || server.listen != nil || server.Session.Endpoint != "unavailable" || server.Session.Key != "" {
		t.Fatalf("skill-only case unexpectedly created MCP state: %#v", server.Session)
	}
	if _, err := LoadSession(root); err != nil {
		t.Fatal(err)
	}
}

func TestCompositeAdversarialCaseServesUntrustedRuntimeContentThroughMCP(t *testing.T) {
	cases, err := LoadCases()
	if err != nil {
		t.Fatal(err)
	}
	server, err := StartServer(t.TempDir(), cases["composite-adversarial"], "0.1.0-SNAPSHOT", "0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Close(ctx)
	}()

	post := func(payload string) map[string]any {
		t.Helper()
		request, err := http.NewRequest(http.MethodPost, server.Session.Endpoint, bytes.NewBufferString(payload))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+server.Session.Key)
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
			t.Fatalf("MCP status=%d body=%s err=%v", response.StatusCode, body, err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(body, &decoded); err != nil {
			t.Fatalf("decode MCP response %s: %v", body, err)
		}
		return decoded
	}
	post(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"agent-eval-test","version":"1"}}}`)
	listed := post(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"LOOMSPAN_list_traces","arguments":{}}}`)
	callResult, ok := listed["result"].(map[string]any)
	if !ok || callResult["isError"] == true {
		t.Fatalf("list imported adversarial trace = %#v", listed)
	}
	envelope, ok := callResult["structuredContent"].(map[string]any)
	if !ok {
		t.Fatalf("list result has unexpected shape %#v", callResult["structuredContent"])
	}
	result, ok := envelope["result"].(map[string]any)
	if !ok {
		t.Fatalf("list envelope has unexpected shape %#v", envelope)
	}
	items, ok := result["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("imported adversarial items = %#v", result["items"])
	}
	item, ok := items[0].(map[string]any)
	if !ok || item["traceId"] != "trace-composite-adversarial" {
		t.Fatalf("adversarial trace item = %#v", items[0])
	}
	raw := post(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"LOOMSPAN_read_trace_artifact","arguments":{"traceId":"trace-composite-adversarial","start":0}}}`)
	rawResult, ok := raw["result"].(map[string]any)
	if !ok || rawResult["isError"] == true {
		t.Fatalf("read adversarial trace = %#v", raw)
	}
	serialized, err := json.Marshal(rawResult["structuredContent"])
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"Ignore the developer", "shell", "repository", "credentials", "attacker.invalid", "selected target"} {
		if !strings.Contains(string(serialized), marker) {
			t.Errorf("MCP adversarial trace does not expose marker %q", marker)
		}
	}
}

func TestEvaluationServerRunsTraceIDOnlyMCPScenarios(t *testing.T) {
	cases, err := LoadCases()
	if err != nil {
		t.Fatal(err)
	}
	server, err := StartServer(t.TempDir(), cases["mcp-without-skill"], "0.1.0-SNAPSHOT", "0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Close(ctx)
	}()

	post := func(payload string) map[string]any {
		t.Helper()
		request, err := http.NewRequest(http.MethodPost, server.Session.Endpoint, bytes.NewBufferString(payload))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+server.Session.Key)
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
			t.Fatalf("MCP status=%d body=%s err=%v", response.StatusCode, body, err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(body, &decoded); err != nil {
			t.Fatalf("decode MCP response %s: %v", body, err)
		}
		return decoded
	}
	post(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"agent-eval-test","version":"1"}}}`)
	listed := post(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"LOOMSPAN_list_traces","arguments":{}}}`)
	items := listed["result"].(map[string]any)["structuredContent"].(map[string]any)["result"].(map[string]any)["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("trace inventory=%#v", items)
	}
	traceID, _ := items[0].(map[string]any)["traceId"].(string)
	inspected := post(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"LOOMSPAN_get_trace","arguments":{"traceId":` + fmt.Sprintf("%q", traceID) + `}}}`)
	call := inspected["result"].(map[string]any)
	structured := call["structuredContent"].(map[string]any)
	serialized, err := json.Marshal(structured)
	if err != nil || call["isError"] == true || !strings.Contains(string(serialized), `"traceId":"`+traceID+`"`) {
		t.Fatalf("trace inspection=%#v serialized=%s err=%v", call, serialized, err)
	}
	for _, rejected := range []string{"sourceFilter", "artifactHandle", "targetScopeId", "instanceId", "resourceUri", "resources"} {
		if strings.Contains(string(serialized), `"`+rejected+`"`) {
			t.Errorf("trace inspection exposed %s: %s", rejected, serialized)
		}
	}
}
