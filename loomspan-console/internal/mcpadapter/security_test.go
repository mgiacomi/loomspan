package mcpadapter

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/mcpcredential"
)

type fakeCredentials struct {
	state mcpcredential.Snapshot
	key   string
}

type barrierCredentials struct {
	mu            sync.Mutex
	snapshot      mcpcredential.Snapshot
	authenticated chan struct{}
	release       chan struct{}
}

func (credentials *barrierCredentials) Snapshot() mcpcredential.Snapshot {
	credentials.mu.Lock()
	defer credentials.mu.Unlock()
	return credentials.snapshot
}

func (credentials *barrierCredentials) Authenticate(string) (uint64, bool) {
	credentials.mu.Lock()
	generation := credentials.snapshot.Generation
	credentials.mu.Unlock()
	close(credentials.authenticated)
	<-credentials.release
	return generation, true
}

func (credentials *barrierCredentials) rotate() {
	credentials.mu.Lock()
	credentials.snapshot.Generation++
	credentials.mu.Unlock()
}

type panicReader struct{}

func (panicReader) Read([]byte) (int, error) { panic("protected body was read") }
func (panicReader) Close() error             { return nil }

func (fake fakeCredentials) Snapshot() mcpcredential.Snapshot { return fake.state }
func (fake fakeCredentials) Authenticate(value string) (uint64, bool) {
	return fake.state.Generation, value == fake.key
}

func TestMCPSecurityPolicyAndFailureOrder(t *testing.T) {
	credentials := fakeCredentials{state: mcpcredential.Snapshot{State: mcpcredential.Enabled, Generation: 2}, key: "secret"}
	called := 0
	handler := SecurityHandler(7345, credentials, NewTracker(), http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { called++; w.WriteHeader(http.StatusNoContent) }))
	tests := []struct {
		name, host, origin, authorization string
		want                              int
	}{
		{"ipv4", "127.0.0.1:7345", "", "Bearer secret", 204},
		{"localhost", "localhost:7345", "http://localhost:7345", "Bearer secret", 204},
		{"foreign host before auth", "foreign.invalid:7345", "", "Bearer secret", 400},
		{"wrong port", "127.0.0.1:9", "", "Bearer secret", 400},
		{"ipv6", "[::1]:7345", "", "Bearer secret", 400},
		{"foreign origin", "127.0.0.1:7345", "https://foreign.invalid", "Bearer secret", 403},
		{"missing bearer", "127.0.0.1:7345", "", "", 401},
		{"wrong bearer", "127.0.0.1:7345", "", "Bearer wrong", 401},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader("{}"))
			request.Host = test.host
			if test.origin != "" {
				request.Header.Set("Origin", test.origin)
			}
			if test.authorization != "" {
				request.Header.Set("Authorization", test.authorization)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.want {
				t.Fatalf("status = %d body=%q", response.Code, response.Body.String())
			}
			if response.Header().Get("Cache-Control") != "no-store" && test.want != 204 {
				t.Fatal("failure was cacheable")
			}
		})
	}
	if called != 2 {
		t.Fatalf("SDK called %d times", called)
	}
}

func TestMCPDisabledFailsBeforeAuthentication(t *testing.T) {
	handler := SecurityHandler(7345, fakeCredentials{state: mcpcredential.Snapshot{State: mcpcredential.Disabled}}, NewTracker(), http.NotFoundHandler())
	request := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	request.Body = panicReader{}
	request.Host = "127.0.0.1:7345"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", response.Code)
	}
}

func TestMCPSecurityRejectsAmbiguousHeadersAndOversizedBodies(t *testing.T) {
	credentials := fakeCredentials{state: mcpcredential.Snapshot{State: mcpcredential.Enabled, Generation: 2}, key: "secret"}
	handler := SecurityHandler(7345, credentials, NewTracker(), http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		_, _ = io.Copy(io.Discard, request.Body)
		response.WriteHeader(http.StatusNoContent)
	}))
	newRequest := func() *http.Request {
		request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader("{}"))
		request.Host = "127.0.0.1:7345"
		request.Header.Set("Authorization", "Bearer secret")
		return request
	}
	tests := []struct {
		name   string
		mutate func(*http.Request)
		want   int
	}{
		{"multiple authorization", func(request *http.Request) {
			request.Header["Authorization"] = []string{"Bearer secret", "Bearer secret"}
		}, 401},
		{"comma authorization", func(request *http.Request) { request.Header.Set("Authorization", "Bearer secret,Bearer secret") }, 401},
		{"multiple origin", func(request *http.Request) {
			request.Header["Origin"] = []string{"http://127.0.0.1:7345", "http://127.0.0.1:7345"}
		}, 403},
		{"absolute target", func(request *http.Request) { request.URL.Scheme = "http"; request.URL.Host = "127.0.0.1:7345" }, 400},
		{"declared oversize", func(request *http.Request) { request.ContentLength = maxRequestBody + 1 }, 413},
		{"forwarded ignored", func(request *http.Request) {
			request.Header.Set("X-Forwarded-Host", "foreign.invalid")
			request.Header.Set("X-Forwarded-Proto", "https")
		}, 204},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := newRequest()
			test.mutate(request)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.want {
				t.Fatalf("status = %d body=%q", response.Code, response.Body.String())
			}
		})
	}
}

func TestRevokedGenerationCannotDispatchInitializeListOrToolCall(t *testing.T) {
	for name, body := range map[string]string{
		"initialize": `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2026-07-28","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`,
		"list":       `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
		"tool":       `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"LOOMSPAN_get_runtime","arguments":{}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			credentials := &barrierCredentials{
				snapshot:      mcpcredential.Snapshot{State: mcpcredential.Enabled, Generation: 7},
				authenticated: make(chan struct{}),
				release:       make(chan struct{}),
			}
			tracker := NewTracker()
			dispatched := 0
			handler := SecurityHandler(7345, credentials, tracker, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { dispatched++ }))
			request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
			request.Host = "127.0.0.1:7345"
			request.Header.Set("Authorization", "Bearer old-key")
			response := httptest.NewRecorder()
			finished := make(chan struct{})
			go func() {
				handler.ServeHTTP(response, request)
				close(finished)
			}()
			<-credentials.authenticated
			if err := tracker.Freeze(context.Background(), false, nil); err != nil {
				t.Fatal(err)
			}
			credentials.rotate()
			if err := tracker.Reopen(); err != nil {
				t.Fatal(err)
			}
			close(credentials.release)
			<-finished
			if response.Code != http.StatusUnauthorized || dispatched != 0 {
				t.Fatalf("status=%d dispatched=%d body=%q", response.Code, dispatched, response.Body.String())
			}
		})
	}
}

func TestSuccessfulAuthenticatedResponseIsNoStore(t *testing.T) {
	credentials := fakeCredentials{state: mcpcredential.Snapshot{State: mcpcredential.Enabled, Generation: 2}, key: "secret"}
	handler := SecurityHandler(7345, credentials, NewTracker(), http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Cache-Control", "no-cache, no-transform")
		response.WriteHeader(http.StatusOK)
		_, _ = response.Write([]byte("response"))
	}))
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader("{}"))
	request.Host = "127.0.0.1:7345"
	request.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status=%d cache-control=%q", response.Code, response.Header().Get("Cache-Control"))
	}
}
