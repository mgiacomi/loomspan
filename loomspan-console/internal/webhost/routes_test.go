package webhost

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/browserapi"
)

func TestRoutesDispatchesExactMCPRealmBeforeBrowserPolicy(t *testing.T) {
	policy, err := browserapi.NewPolicy("127.0.0.1:7345", "http://127.0.0.1:7345", "")
	if err != nil {
		t.Fatal(err)
	}
	called := 0
	mcp := http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		called++
		response.WriteHeader(http.StatusAccepted)
	})
	handler := Routes(policy, http.NotFoundHandler(), mcp, testFiles())
	request := httptest.NewRequest(http.MethodPost, "http://foreign.invalid/mcp", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || called != 1 {
		t.Fatalf("exact /mcp response = %d, calls = %d", response.Code, called)
	}

	for _, path := range []string{"/mcp/", "/api/mcp/"} {
		called = 0
		response = httptest.NewRecorder()
		request = httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7345"+path, nil)
		handler.ServeHTTP(response, request)
		if called != 0 {
			t.Fatalf("%s called MCP handler", path)
		}
		if response.Code == http.StatusTemporaryRedirect || response.Code == http.StatusPermanentRedirect {
			t.Fatalf("%s redirected to MCP", path)
		}
	}
}
