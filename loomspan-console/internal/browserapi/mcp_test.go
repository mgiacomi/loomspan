package browserapi

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/browserauth"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/mcpcredential"
)

type fakeMCPManager struct {
	snapshot   mcpcredential.Snapshot
	credential string
	calls      []string
}

func (fake *fakeMCPManager) Status() mcpcredential.Snapshot { return fake.snapshot }
func (fake *fakeMCPManager) Enable(context.Context) (string, error) {
	fake.calls = append(fake.calls, "enable")
	fake.snapshot.State = mcpcredential.Enabled
	return fake.credential, nil
}
func (fake *fakeMCPManager) Reveal() (string, error) {
	fake.calls = append(fake.calls, "reveal")
	return fake.credential, nil
}
func (fake *fakeMCPManager) Regenerate(context.Context) (string, error) {
	fake.calls = append(fake.calls, "regenerate")
	return fake.credential, nil
}
func (fake *fakeMCPManager) Disable(context.Context) error {
	fake.calls = append(fake.calls, "disable")
	fake.snapshot.State = mcpcredential.Disabled
	return nil
}
func (fake *fakeMCPManager) RemoveInvalid(context.Context) error {
	fake.calls = append(fake.calls, "remove-invalid")
	fake.snapshot = mcpcredential.Snapshot{State: mcpcredential.Disabled}
	return nil
}

func mcpTestRouter(t *testing.T, manager MCPManager) (*Router, *http.Cookie, string, string) {
	t.Helper()
	entropy := bytes.Repeat([]byte{11}, 32*16)
	pairing := browserauth.NewPairing(nil, bytes.NewReader(entropy))
	registry := browserauth.NewRegistry(nil, bytes.NewReader(entropy))
	sessionID, _ := registry.CreateSession()
	security, _ := registry.Bootstrap(sessionID, "")
	policy, _ := NewPolicy("127.0.0.1:7943", "http://127.0.0.1:7943", "")
	router, err := New(Options{Policy: policy, Pairing: pairing, Sessions: registry, PairingURL: func(value string) string { return value }, MCP: manager, MCPEndpoint: "http://127.0.0.1:7943/mcp"})
	if err != nil {
		t.Fatal(err)
	}
	return router, browserauth.SessionCookie(sessionID), security.TabID, security.CSRF
}

func mcpBrowserRequest(router http.Handler, path, body string, cookie *http.Cookie, tab, csrf string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7943"+path, strings.NewReader(body))
	request.Host = "127.0.0.1:7943"
	request.Header.Set("Origin", "http://127.0.0.1:7943")
	if cookie != nil {
		request.AddCookie(cookie)
	}
	if tab != "" {
		request.Header.Set("X-loomspan-Console-Tab", tab)
	}
	if csrf != "" {
		request.Header.Set(csrfHeader, csrf)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestMCPStatusNeverReturnsCredential(t *testing.T) {
	fake := &fakeMCPManager{snapshot: mcpcredential.Snapshot{State: mcpcredential.Enabled}, credential: "lsmcp_SECRET_SENTINEL"}
	router, cookie, _, _ := mcpTestRouter(t, fake)
	response := mcpBrowserRequest(router, "/api/console/v1/mcp/status", "{}", cookie, "", "")
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), fake.credential) || !strings.Contains(response.Body.String(), `"state":"ENABLED"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestMCPRevealRequiresPairedCSRFAndExplicitResponseIsNoStore(t *testing.T) {
	fake := &fakeMCPManager{snapshot: mcpcredential.Snapshot{State: mcpcredential.Enabled}, credential: "lsmcp_SECRET_SENTINEL"}
	router, cookie, tab, csrf := mcpTestRouter(t, fake)
	for _, test := range []struct {
		name   string
		cookie *http.Cookie
		csrf   string
		want   int
	}{{"session", nil, csrf, 401}, {"csrf", cookie, "wrong", 403}, {"valid", cookie, csrf, 200}} {
		t.Run(test.name, func(t *testing.T) {
			response := mcpBrowserRequest(router, "/api/console/v1/mcp/reveal", "{}", test.cookie, tab, test.csrf)
			if response.Code != test.want {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if test.want == 200 && (!strings.Contains(response.Body.String(), fake.credential) || response.Header().Get("Cache-Control") != "no-store") {
				t.Fatal("explicit credential response contract failed")
			}
		})
	}
}

func TestDisruptiveMCPOperationsRequireExactConfirmation(t *testing.T) {
	fake := &fakeMCPManager{snapshot: mcpcredential.Snapshot{State: mcpcredential.Enabled}, credential: "key"}
	router, cookie, tab, csrf := mcpTestRouter(t, fake)
	for _, item := range []struct{ path, confirmation string }{{"/api/console/v1/mcp/regenerate", "REGENERATE"}, {"/api/console/v1/mcp/disable", "DISABLE"}} {
		bad := mcpBrowserRequest(router, item.path, `{"confirmation":"wrong"}`, cookie, tab, csrf)
		if bad.Code != 400 {
			t.Fatalf("%s accepted wrong confirmation", item.path)
		}
		good := mcpBrowserRequest(router, item.path, `{"confirmation":"`+item.confirmation+`"}`, cookie, tab, csrf)
		if good.Code != 200 {
			t.Fatalf("%s status=%d body=%s", item.path, good.Code, good.Body.String())
		}
		if item.path == "/api/console/v1/mcp/disable" {
			fake.snapshot.State = mcpcredential.Enabled
		}
	}
}

func TestMCPInvalidRemovalRequiresExactConfirmationAndNeverRevealsContents(t *testing.T) {
	fake := &fakeMCPManager{snapshot: mcpcredential.Snapshot{State: mcpcredential.DisabledInvalid, Diagnostic: "canonical access key format is invalid"}, credential: "invalid-file-secret"}
	router, cookie, tab, csrf := mcpTestRouter(t, fake)
	bad := mcpBrowserRequest(router, "/api/console/v1/mcp/remove-invalid", `{"confirmation":"REMOVE"}`, cookie, tab, csrf)
	if bad.Code != http.StatusBadRequest || len(fake.calls) != 0 {
		t.Fatalf("invalid confirmation status=%d calls=%v", bad.Code, fake.calls)
	}
	good := mcpBrowserRequest(router, "/api/console/v1/mcp/remove-invalid", `{"confirmation":"REMOVE_INVALID"}`, cookie, tab, csrf)
	if good.Code != http.StatusOK || !strings.Contains(good.Body.String(), `"state":"DISABLED"`) || strings.Contains(good.Body.String(), fake.credential) {
		t.Fatalf("removal status=%d body=%s", good.Code, good.Body.String())
	}
}

func TestMCPBrowserOperationsRejectUnknownTrailingAndOversizedJSON(t *testing.T) {
	fake := &fakeMCPManager{snapshot: mcpcredential.Snapshot{State: mcpcredential.Disabled}}
	router, cookie, tab, csrf := mcpTestRouter(t, fake)
	for name, body := range map[string]string{
		"unknown":   `{"unknown":true}`,
		"trailing":  `{} {}`,
		"oversized": `{"padding":"` + strings.Repeat("x", maxMCPJSONBody) + `"}`,
	} {
		t.Run(name, func(t *testing.T) {
			response := mcpBrowserRequest(router, "/api/console/v1/mcp/enable", body, cookie, tab, csrf)
			if response.Code != http.StatusBadRequest || len(fake.calls) != 0 {
				t.Fatalf("status=%d calls=%v body=%s", response.Code, fake.calls, response.Body.String())
			}
		})
	}
}
