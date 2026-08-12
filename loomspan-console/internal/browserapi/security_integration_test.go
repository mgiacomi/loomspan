package browserapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/browserauth"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/release"
)

func TestPairingBootstrapAndProtectedPairingLink(t *testing.T) {
	entropy := make([]byte, 32*16)
	for index := range entropy {
		entropy[index] = byte(index/32 + 1)
	}
	pairing := browserauth.NewPairing(nil, bytes.NewReader(entropy))
	secret, _ := pairing.Create(false)
	registry := browserauth.NewRegistry(nil, bytes.NewReader(entropy[32:]))
	policy, _ := NewPolicy("127.0.0.1:7943", "http://127.0.0.1:7943", "")
	router, err := New(Options{
		Policy: policy, Pairing: pairing, Sessions: registry,
		ProcessID: "process", Workspace: "workspace",
		PairingURL: func(secret string) string { return "http://127.0.0.1:7943/#/pair/" + secret },
	})
	if err != nil {
		t.Fatal(err)
	}
	exchange := apiRequest(router, "/api/console/v1/pairing/exchange", `{"secret":"`+secret+`"}`, nil)
	if exchange.Code != http.StatusOK {
		t.Fatalf("exchange=%d %s", exchange.Code, exchange.Body.String())
	}
	cookies := exchange.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies=%v", cookies)
	}
	bootstrap := apiRequest(router, "/api/console/v1/bootstrap", `{}`, cookies[0])
	if bootstrap.Code != http.StatusOK {
		t.Fatalf("bootstrap=%d %s", bootstrap.Code, bootstrap.Body.String())
	}
	var state struct {
		ConsoleVersion string `json:"consoleVersion"`
		TabID          string `json:"tabId"`
		CSRF           string `json:"csrfToken"`
	}
	if err := json.Unmarshal(bootstrap.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if state.ConsoleVersion != release.ProductVersion() {
		t.Fatalf("consoleVersion=%q", state.ConsoleVersion)
	}
	linkRequest := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7943/api/console/v1/pairing/link", strings.NewReader(`{}`))
	linkRequest.Host = "127.0.0.1:7943"
	linkRequest.Header.Set("Origin", "http://127.0.0.1:7943")
	linkRequest.Header.Set("X-loomspan-Console-Tab", state.TabID)
	linkRequest.Header.Set(csrfHeader, state.CSRF)
	linkRequest.AddCookie(cookies[0])
	link := httptest.NewRecorder()
	router.ServeHTTP(link, linkRequest)
	if link.Code != http.StatusOK || !strings.Contains(link.Body.String(), "#/pair/") {
		t.Fatalf("link=%d %s", link.Code, link.Body.String())
	}
	if link.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("protected response is cacheable")
	}
	heartbeatRequest := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7943/api/console/v1/tabs/heartbeat", strings.NewReader(`{}`))
	heartbeatRequest.Host = "127.0.0.1:7943"
	heartbeatRequest.Header.Set("Origin", "http://127.0.0.1:7943")
	heartbeatRequest.Header.Set("X-loomspan-Console-Tab", state.TabID)
	heartbeatRequest.Header.Set(csrfHeader, state.CSRF)
	heartbeatRequest.AddCookie(cookies[0])
	heartbeat := httptest.NewRecorder()
	router.ServeHTTP(heartbeat, heartbeatRequest)
	if heartbeat.Code != http.StatusOK || !strings.Contains(heartbeat.Body.String(), `"active":true`) {
		t.Fatalf("heartbeat=%d %s", heartbeat.Code, heartbeat.Body.String())
	}
}

func TestSecurityFailureOrderPreventsBodyRead(t *testing.T) {
	pairing := browserauth.NewPairing(nil, bytes.NewReader(bytes.Repeat([]byte{1}, 32)))
	registry := browserauth.NewRegistry(nil, bytes.NewReader(bytes.Repeat([]byte{2}, 64)))
	policy, _ := NewPolicy("127.0.0.1:7943", "http://127.0.0.1:7943", "")
	router, _ := New(Options{Policy: policy, Pairing: pairing, Sessions: registry, PairingURL: func(value string) string { return value }})
	body := &readSpy{Reader: strings.NewReader(`{"secret":"candidate"}`)}
	request := httptest.NewRequest(http.MethodPost, "http://evil.test/api/console/v1/pairing/exchange", body)
	request.Host = "evil.test"
	request.Header.Set("Origin", "http://evil.test")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if body.read || response.Code != http.StatusBadRequest {
		t.Fatalf("read=%v status=%d", body.read, response.Code)
	}
}

func TestPairingLinkRequiresOriginSessionAndCSRFIndependently(t *testing.T) {
	entropy := bytes.Repeat([]byte{7}, 32*16)
	pairing := browserauth.NewPairing(nil, bytes.NewReader(entropy))
	registry := browserauth.NewRegistry(nil, bytes.NewReader(entropy))
	sessionID, _ := registry.CreateSession()
	state, _ := registry.Bootstrap(sessionID, "")
	policy, _ := NewPolicy("127.0.0.1:7943", "http://127.0.0.1:7943", "")
	router, _ := New(Options{Policy: policy, Pairing: pairing, Sessions: registry, PairingURL: func(value string) string { return value }})
	validCookie := browserauth.SessionCookie(sessionID)

	tests := []struct {
		name   string
		origin string
		cookie *http.Cookie
		csrf   string
		status int
	}{
		{"missing origin", "", validCookie, state.CSRF, http.StatusForbidden},
		{"missing session", "http://127.0.0.1:7943", nil, state.CSRF, http.StatusUnauthorized},
		{"wrong csrf", "http://127.0.0.1:7943", validCookie, "wrong", http.StatusForbidden},
		{"valid", "http://127.0.0.1:7943", validCookie, state.CSRF, http.StatusOK},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7943/api/console/v1/pairing/link", strings.NewReader(`{}`))
			request.Host = "127.0.0.1:7943"
			if test.origin != "" {
				request.Header.Set("Origin", test.origin)
			}
			request.Header.Set("X-loomspan-Console-Tab", state.TabID)
			request.Header.Set(csrfHeader, test.csrf)
			if test.cookie != nil {
				request.AddCookie(test.cookie)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestEveryEmptyBodyOperationRejectsInvalidJSON(t *testing.T) {
	entropy := bytes.Repeat([]byte{9}, 32*16)
	pairing := browserauth.NewPairing(nil, bytes.NewReader(entropy))
	registry := browserauth.NewRegistry(nil, bytes.NewReader(entropy))
	sessionID, _ := registry.CreateSession()
	state, _ := registry.Bootstrap(sessionID, "")
	policy, _ := NewPolicy("127.0.0.1:7943", "http://127.0.0.1:7943", "")
	router, _ := New(Options{
		Policy: policy, Pairing: pairing, Sessions: registry,
		PairingURL: func(value string) string { return value },
	})

	for _, path := range []string{
		"/api/console/v1/pairing/challenge",
		"/api/console/v1/pairing/link",
		"/api/console/v1/tabs/release",
	} {
		t.Run(path, func(t *testing.T) {
			for name, body := range map[string]string{
				"unknown field": `{"unexpected":true}`,
				"oversized":     strings.Repeat(" ", maxJSONBody+1),
			} {
				t.Run(name, func(t *testing.T) {
					request := httptest.NewRequest(
						http.MethodPost,
						"http://127.0.0.1:7943"+path,
						strings.NewReader(body),
					)
					request.Host = "127.0.0.1:7943"
					request.Header.Set("Origin", "http://127.0.0.1:7943")
					if path != "/api/console/v1/pairing/challenge" {
						request.AddCookie(browserauth.SessionCookie(sessionID))
						request.Header.Set("X-loomspan-Console-Tab", state.TabID)
						request.Header.Set(csrfHeader, state.CSRF)
					}
					response := httptest.NewRecorder()
					router.ServeHTTP(response, request)
					if response.Code != http.StatusBadRequest ||
						!strings.Contains(response.Body.String(), `"code":"INVALID_REQUEST"`) {
						t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
					}
				})
			}
		})
	}
}

type readSpy struct {
	*strings.Reader
	read bool
}

func (reader *readSpy) Read(buffer []byte) (int, error) {
	reader.read = true
	return reader.Reader.Read(buffer)
}

func apiRequest(handler http.Handler, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7943"+path, strings.NewReader(body))
	request.Host = "127.0.0.1:7943"
	request.Header.Set("Origin", "http://127.0.0.1:7943")
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestRawDownloadRequiresSameSitePairedNavigation(t *testing.T) {
	entropy := bytes.Repeat([]byte{40}, 32*16)
	pairing := browserauth.NewPairing(nil, bytes.NewReader(entropy))
	registry := browserauth.NewRegistry(nil, bytes.NewReader(entropy))
	sessionID, _ := registry.CreateSession()
	policy, _ := NewPolicy("127.0.0.1:7943", "http://127.0.0.1:7943", "")
	router, _ := New(Options{
		Policy: policy, Pairing: pairing, Sessions: registry,
		PairingURL: func(value string) string { return value },
	})
	cookie := browserauth.SessionCookie(sessionID)

	tests := []struct {
		name        string
		origin      string
		fetchSite   string
		cookie      *http.Cookie
		host        string
		wantStatus  int
		wantBlocked bool
	}{
		{
			name:       "same-origin navigation without Origin header passes security",
			cookie:     cookie,
			host:       "127.0.0.1:7943",
			wantStatus: http.StatusInternalServerError, // no target context, but security passes
		},
		{
			name:       "same-origin with Sec-Fetch-Site passes security",
			fetchSite:  "same-origin",
			cookie:     cookie,
			host:       "127.0.0.1:7943",
			wantStatus: http.StatusInternalServerError, // no target context, but security passes
		},
		{
			name:        "cross-site fetch metadata rejected",
			fetchSite:   "cross-site",
			cookie:      cookie,
			host:        "127.0.0.1:7943",
			wantStatus:  http.StatusForbidden,
			wantBlocked: true,
		},
		{
			name:        "same-site fetch metadata rejected",
			fetchSite:   "same-site",
			cookie:      cookie,
			host:        "127.0.0.1:7943",
			wantStatus:  http.StatusForbidden,
			wantBlocked: true,
		},
		{
			name:        "missing session cookie rejected",
			host:        "127.0.0.1:7943",
			wantStatus:  http.StatusUnauthorized,
			wantBlocked: true,
		},
		{
			name:        "wrong host rejected",
			cookie:      cookie,
			host:        "evil.test",
			wantStatus:  http.StatusBadRequest,
			wantBlocked: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:7943/api/console/v1/artifacts/trace-1/raw", nil)
			request.Host = test.host
			if test.origin != "" {
				request.Header.Set("Origin", test.origin)
			}
			if test.fetchSite != "" {
				request.Header.Set("Sec-Fetch-Site", test.fetchSite)
				request.Header.Set("Sec-Fetch-Mode", "navigate")
			}
			if test.cookie != nil {
				request.AddCookie(test.cookie)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status=%d expected %d, body=%s", response.Code, test.wantStatus, response.Body.String())
			}
			if test.wantBlocked {
				body := response.Body.String()
				if strings.Contains(body, "trace-1") && strings.Contains(body, "ndjson") {
					t.Fatalf("blocked request leaked artifact content: %s", body)
				}
			}
		})
	}
}
