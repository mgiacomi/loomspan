package console

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/config"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/profile"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/workspace"
)

func TestInvalidPromptCredentialFailsBeforeListenerStartup(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "profile", "config.yaml")
	ownedProfile, err := profile.Open(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := ownedProfile.Close(); err != nil {
		t.Fatal(err)
	}
	content := config.DefaultYAML + "target:\n  address: https://application.example\n"
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	err = Run(context.Background(), Options{
		ConfigPath:              configPath,
		WorkDirectory:           filepath.Join(root, "work"),
		ListenOverride:          "127.0.0.1:0",
		NoOpenBrowser:           true,
		PromptForApplicationKey: true,
	}, Dependencies{
		PromptApplicationKey: func(context.Context) ([]byte, error) {
			return []byte("too-short"), nil
		},
	})
	var domain *consolecore.Error
	if !errors.As(err, &domain) || domain.Code != consolecore.CodeInvalidArgument {
		t.Fatalf("invalid prompt credential did not fail startup: %v", err)
	}
}

type lineWriter struct {
	mu    sync.Mutex
	lines chan string
}

func (writer *lineWriter) Write(content []byte) (int, error) {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	writer.lines <- string(content)
	return len(content), nil
}

func TestLiveConsolePairsBootstrapsAndReleasesLocks(t *testing.T) {
	root := t.TempDir()
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "profile", "config.yaml")
	workPath := filepath.Join(root, "work")
	wantWorkPath := filepath.Join(canonicalRoot, "work")
	output := &lineWriter{lines: make(chan string, 8)}
	context, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		result <- Run(context, Options{
			ConfigPath: configPath, WorkDirectory: workPath,
			ListenOverride: "127.0.0.1:0", NoOpenBrowser: true,
		}, Dependencies{
			Files: fstest.MapFS{
				"index.html":             {Data: []byte("<main>loomspan</main>")},
				"assets/app-12345678.js": {Data: []byte("export{}")},
			},
			Output: output,
		})
	}()

	var pairingURL string
	timeout := time.After(5 * time.Second)
	for pairingURL == "" {
		select {
		case line := <-output.lines:
			if strings.HasPrefix(line, "Pairing URL: ") {
				pairingURL = strings.TrimSpace(strings.TrimPrefix(line, "Pairing URL: "))
			}
		case <-timeout:
			t.Fatal("pairing URL was not printed")
		}
	}
	parsed, err := url.Parse(pairingURL)
	if err != nil {
		t.Fatal(err)
	}
	secret := strings.TrimPrefix(parsed.Fragment, "/pair/")
	origin := "http://" + parsed.Host

	exchangeRequest, _ := http.NewRequest(http.MethodPost, origin+"/api/console/v1/pairing/exchange", strings.NewReader(`{"secret":"`+secret+`"}`))
	exchangeRequest.Header.Set("Origin", origin)
	exchange, err := http.DefaultClient.Do(exchangeRequest)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, exchange.Body)
	exchange.Body.Close()
	if exchange.StatusCode != http.StatusOK || len(exchange.Cookies()) != 1 {
		t.Fatalf("exchange status=%d cookies=%v", exchange.StatusCode, exchange.Cookies())
	}

	bootstrapRequest, _ := http.NewRequest(http.MethodPost, origin+"/api/console/v1/bootstrap", strings.NewReader(`{}`))
	bootstrapRequest.Header.Set("Origin", origin)
	bootstrapRequest.AddCookie(exchange.Cookies()[0])
	bootstrap, err := http.DefaultClient.Do(bootstrapRequest)
	if err != nil {
		t.Fatal(err)
	}
	var state map[string]any
	if err := json.NewDecoder(bootstrap.Body).Decode(&state); err != nil {
		t.Fatal(err)
	}
	bootstrap.Body.Close()
	if bootstrap.StatusCode != http.StatusOK || state["workspacePath"] != wantWorkPath ||
		state["tabId"] == "" || state["csrfToken"] == "" || state["target"] == nil {
		t.Fatalf("bootstrap status=%d state=%v", bootstrap.StatusCode, state)
	}

	cancel()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("service did not shut down")
	}

	reopenedProfile, err := profile.Open(configPath)
	if err != nil {
		t.Fatalf("profile lock was not released: %v", err)
	}
	reopenedProfile.Close()
	reopenedWorkspace, err := workspace.Open(workPath)
	if err != nil {
		t.Fatalf("workspace lock was not released: %v", err)
	}
	reopenedWorkspace.Close()
}

// TestLiveConsoleArtifactRoutesDoNotLeakPathsOrCredentials proves that the
// live Console's artifact JSON routes and raw download route never expose a
// filesystem path or the application credential in their response bodies,
// headers, or error envelopes (PR12-R13). It stands up the full Console
// composition root against a Java-compatible test server, acquires an artifact,
// inspects the storage snapshot, and performs a raw download, then asserts no
// path or credential appears in any rendered response.
func TestLiveConsoleArtifactRoutesDoNotLeakPathsOrCredentials(t *testing.T) {
	secret := "LOOMSPAN_" + "TEST_APPLICATION_KEY_DO_NOT_LEAK_c3d9"
	root := t.TempDir()
	configPath := filepath.Join(root, "profile", "config.yaml")
	workPath := filepath.Join(root, "work")
	ownedProfile, err := profile.Open(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := ownedProfile.Close(); err != nil {
		t.Fatal(err)
	}
	content := config.DefaultYAML
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	// Java-compatible test server that serves instance, trace metadata, and a
	// small artifact body.
	artifactBody := []byte(`{"traceId":"trace-security","sessionId":"session-1","sequence":1,"timestamp":1784894400.000000000,"recordType":"TRACE_STARTED","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"th","metadata":{},"data":null}` + "\n" +
		`{"traceId":"trace-security","sessionId":"session-1","sequence":2,"timestamp":1784894400.000000000,"recordType":"TRACE_COMPLETED","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"th","metadata":{"outcome":"SUCCEEDED","sessionUsageSnapshot":{"promptUnits":0,"completionUnits":0,"totalUnits":0},"errored":false,"persistencePolicy":"ALWAYS"},"data":null}` + "\n")
	traceMetadata := fmt.Sprintf(
		`{"targetScopeId":"scope-1","traceId":"trace-security","sessionId":"session-1","entrySkill":"CheckDns","outcome":"SUCCEEDED","finalizedAt":"2026-07-24T12:00:00Z","sizeBytes":%d,"persistencePolicy":"ALWAYS","applicationTraceExpiresAt":"2026-08-01T12:00:00Z"}`,
		len(artifactBody),
	)
	targetServer := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("X-loomspan-Instance-Id", "11111111-1111-4111-8111-111111111111")
		if strings.HasSuffix(request.URL.Path, "/instance") {
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{"instanceId":"11111111-1111-4111-8111-111111111111","consoleCompatibilityVersion":"development","observedAt":"2026-07-25T12:00:00Z","liveMonitoringAvailable":true,"registeredSkillCount":0,"activeExecutionCount":0,"catalogedTraceCount":1,"tracePersistencePolicy":"PERSISTENT","completionGraceTtl":"PT2M","traceCatalogMetadataTtl":"PT168H"}`))
			return
		}
		if strings.HasSuffix(request.URL.Path, "/traces/trace-security") && !strings.HasSuffix(request.URL.Path, "/artifact") {
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(traceMetadata))
			return
		}
		if strings.HasSuffix(request.URL.Path, "/traces/trace-security/artifact") {
			response.Header().Set("Content-Type", "application/x-ndjson")
			response.Header().Set("Content-Length", fmt.Sprintf("%d", len(artifactBody)))
			response.Header().Set("Content-Disposition", `attachment; filename="loomspan-trace-trace-security.ndjson"`)
			response.Header().Set("Cache-Control", "no-store")
			_, _ = response.Write(artifactBody)
			return
		}
		response.WriteHeader(http.StatusNotFound)
	}))
	defer targetServer.Close()

	output := &lineWriter{lines: make(chan string, 16)}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		result <- Run(ctx, Options{
			ConfigPath: configPath, WorkDirectory: workPath,
			ListenOverride: "127.0.0.1:0", NoOpenBrowser: true,
		}, Dependencies{
			Files: fstest.MapFS{
				"index.html":             {Data: []byte("<main>loomspan</main>")},
				"assets/app-12345678.js": {Data: []byte("export{}")},
			},
			Output: output,
		})
	}()

	var pairingURL string
	timeout := time.After(10 * time.Second)
	for pairingURL == "" {
		select {
		case line := <-output.lines:
			if strings.HasPrefix(line, "Pairing URL: ") {
				pairingURL = strings.TrimSpace(strings.TrimPrefix(line, "Pairing URL: "))
			}
		case <-timeout:
			cancel()
			t.Fatal("pairing URL was not printed")
		}
	}
	parsed, err := url.Parse(pairingURL)
	if err != nil {
		t.Fatal(err)
	}
	origin := "http://" + parsed.Host
	pairSecret := strings.TrimPrefix(parsed.Fragment, "/pair/")

	// Pair and bootstrap.
	exchangeReq, _ := http.NewRequest(http.MethodPost, origin+"/api/console/v1/pairing/exchange", strings.NewReader(`{"secret":"`+pairSecret+`"}`))
	exchangeReq.Header.Set("Origin", origin)
	exchange, err := http.DefaultClient.Do(exchangeReq)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, exchange.Body)
	exchange.Body.Close()
	if exchange.StatusCode != http.StatusOK || len(exchange.Cookies()) != 1 {
		t.Fatalf("exchange status=%d cookies=%v", exchange.StatusCode, exchange.Cookies())
	}
	sessionCookie := exchange.Cookies()[0]

	bootstrapReq, _ := http.NewRequest(http.MethodPost, origin+"/api/console/v1/bootstrap", strings.NewReader(`{}`))
	bootstrapReq.Header.Set("Origin", origin)
	bootstrapReq.AddCookie(sessionCookie)
	bootstrap, err := http.DefaultClient.Do(bootstrapReq)
	if err != nil {
		t.Fatal(err)
	}
	var state struct {
		TabID     string `json:"tabId"`
		CSRFToken string `json:"csrfToken"`
	}
	bodyBytes, _ := io.ReadAll(bootstrap.Body)
	bootstrap.Body.Close()
	if bootstrap.StatusCode != http.StatusOK {
		t.Fatalf("bootstrap status=%d body=%s", bootstrap.StatusCode, bodyBytes)
	}
	if err := json.Unmarshal(bodyBytes, &state); err != nil {
		t.Fatalf("decode bootstrap: %v", err)
	}

	// Connect to the test target server with the secret credential.
	connectBody := fmt.Sprintf(`{"targetAddress":%q,"applicationKey":%q}`, targetServer.URL, secret)
	connectReq, _ := http.NewRequest(http.MethodPost, origin+"/api/console/v1/target/connect", strings.NewReader(connectBody))
	connectReq.Header.Set("Origin", origin)
	connectReq.Header.Set("X-loomspan-Console-Tab", state.TabID)
	connectReq.Header.Set("X-loomspan-Console-CSRF", state.CSRFToken)
	connectReq.AddCookie(sessionCookie)
	connect, err := http.DefaultClient.Do(connectReq)
	if err != nil {
		t.Fatal(err)
	}
	connectRespBody, _ := io.ReadAll(connect.Body)
	connect.Body.Close()
	if connect.StatusCode != http.StatusOK {
		t.Fatalf("connect status=%d body=%s", connect.StatusCode, connectRespBody)
	}

	// Acquire the artifact through the browser boundary.
	acquireReq, _ := http.NewRequest(http.MethodPost, origin+"/api/console/v1/artifacts/acquire", strings.NewReader(`{"traceId":"trace-security"}`))
	acquireReq.Header.Set("Origin", origin)
	acquireReq.Header.Set("X-loomspan-Console-Tab", state.TabID)
	acquireReq.Header.Set("X-loomspan-Console-CSRF", state.CSRFToken)
	acquireReq.AddCookie(sessionCookie)
	acquire, err := http.DefaultClient.Do(acquireReq)
	if err != nil {
		t.Fatal(err)
	}
	acquireBody, _ := io.ReadAll(acquire.Body)
	acquire.Body.Close()
	if acquire.StatusCode != http.StatusOK {
		t.Fatalf("acquire status=%d body=%s", acquire.StatusCode, acquireBody)
	}
	if strings.Contains(string(acquireBody), secret) {
		t.Fatal("credential leaked into artifact acquire response")
	}
	if strings.Contains(string(acquireBody), "transient") || strings.Contains(string(acquireBody), workPath) {
		t.Fatalf("filesystem path leaked into artifact acquire response: %s", acquireBody)
	}

	// Inspect the storage snapshot.
	storageReq, _ := http.NewRequest(http.MethodPost, origin+"/api/console/v1/artifacts/storage", strings.NewReader(`{}`))
	storageReq.Header.Set("Origin", origin)
	storageReq.AddCookie(sessionCookie)
	storage, err := http.DefaultClient.Do(storageReq)
	if err != nil {
		t.Fatal(err)
	}
	storageBody, _ := io.ReadAll(storage.Body)
	storage.Body.Close()
	if storage.StatusCode != http.StatusOK {
		t.Fatalf("storage status=%d body=%s", storage.StatusCode, storageBody)
	}
	if strings.Contains(string(storageBody), secret) {
		t.Fatal("credential leaked into storage snapshot response")
	}
	if strings.Contains(string(storageBody), "transient") || strings.Contains(string(storageBody), workPath) {
		t.Fatalf("filesystem path leaked into storage snapshot response: %s", storageBody)
	}

	// Perform a raw download and verify the body contains only artifact bytes.
	rawReq, _ := http.NewRequest(http.MethodGet, origin+"/api/console/v1/artifacts/trace-security/raw", nil)
	rawReq.Header.Set("Sec-Fetch-Site", "same-origin")
	rawReq.Header.Set("Sec-Fetch-Mode", "navigate")
	rawReq.AddCookie(sessionCookie)
	raw, err := http.DefaultClient.Do(rawReq)
	if err != nil {
		t.Fatal(err)
	}
	rawBody, _ := io.ReadAll(raw.Body)
	raw.Body.Close()
	if raw.StatusCode != http.StatusOK {
		t.Fatalf("raw download status=%d body=%s", raw.StatusCode, rawBody)
	}
	if !bytes.Equal(rawBody, artifactBody) {
		t.Fatalf("raw download body mismatch: got %d bytes want %d bytes", len(rawBody), len(artifactBody))
	}
	if strings.Contains(string(rawBody), secret) {
		t.Fatal("credential leaked into raw download body")
	}
	// The raw download must not leak the workspace path in any header.
	for _, header := range raw.Header.Values("Content-Disposition") {
		if strings.Contains(header, workPath) || strings.Contains(header, "transient") {
			t.Fatalf("filesystem path leaked into Content-Disposition: %s", header)
		}
	}

	cancel()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("service did not shut down")
	}
}
