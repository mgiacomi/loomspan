package browserapi

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/applicationclient"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/browserauth"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/evidence"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/observability"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/target"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/traceinventory"
)

// fakeArtifactService is a test double for ArtifactService that records calls
// and returns canned responses.
type fakeArtifactService struct {
	acquireResult   artifact.AcquiredArtifact
	acquireErr      *consolecore.Error
	lookupResult    artifact.LookupResult
	lookupErr       *consolecore.Error
	snapshotResult  artifact.StorageSnapshot
	snapshotErr     *consolecore.Error
	removeErr       *consolecore.Error
	clearExpiredErr *consolecore.Error
	clearAllErr     *consolecore.Error

	lastAcquireTraceID string
	lastRemoveTraceID  string
	acquireCalled      bool
	lookupCalled       bool
	snapshotCalled     bool
	removeCalled       bool
	clearExpiredCalled bool
	clearAllCalled     bool
	importResult       artifact.AcquiredArtifact
	importErr          *consolecore.Error
	importLimit        int64
	importCalled       bool
	importDeclared     int64
	importBody         []byte
}

func (f *fakeArtifactService) Acquire(_ context.Context, _ target.Scope, traceID string) (artifact.AcquiredArtifact, *consolecore.Error) {
	f.acquireCalled = true
	f.lastAcquireTraceID = traceID
	if f.acquireErr != nil {
		return artifact.AcquiredArtifact{}, f.acquireErr
	}
	return f.acquireResult, nil
}

func (f *fakeArtifactService) Import(_ context.Context, reader io.Reader, declared int64) (artifact.AcquiredArtifact, *consolecore.Error) {
	f.importCalled = true
	f.importDeclared = declared
	var readErr error
	f.importBody, readErr = io.ReadAll(reader)
	if readErr != nil {
		return artifact.AcquiredArtifact{}, consolecore.NewError(consolecore.CodeInvalidArtifact,
			"The trace artifact could not be validated.", "", consolecore.Details{}, readErr)
	}
	if f.importErr != nil {
		return artifact.AcquiredArtifact{}, f.importErr
	}
	return f.importResult, nil
}

func (f *fakeArtifactService) ImportLimit() int64 {
	if f.importLimit > 0 {
		return f.importLimit
	}
	return 4 << 30
}

func (f *fakeArtifactService) Lookup(_ evidence.Reference, _ string) (artifact.LookupResult, *consolecore.Error) {
	f.lookupCalled = true
	if f.lookupErr != nil {
		return artifact.LookupResult{}, f.lookupErr
	}
	return f.lookupResult, nil
}

func (f *fakeArtifactService) StorageSnapshot() (artifact.StorageSnapshot, *consolecore.Error) {
	f.snapshotCalled = true
	if f.snapshotErr != nil {
		return artifact.StorageSnapshot{}, f.snapshotErr
	}
	return f.snapshotResult, nil
}

func (f *fakeArtifactService) Remove(_ evidence.Reference, traceID string) *consolecore.Error {
	f.lastRemoveTraceID = traceID
	f.removeCalled = true
	return f.removeErr
}

func (f *fakeArtifactService) ClearExpired() *consolecore.Error {
	f.clearExpiredCalled = true
	return f.clearExpiredErr
}

func (f *fakeArtifactService) ClearAllUnused() *consolecore.Error {
	f.clearAllCalled = true
	return f.clearAllErr
}

func artifactTestRouter(t *testing.T, artifacts ArtifactService) (*Router, string, *http.Cookie) {
	t.Helper()
	router, tabID, _, cookie := artifactTestRouterWithCSRF(t, artifacts)
	return router, tabID, cookie
}

func artifactTestRouterWithCSRF(t *testing.T, artifacts ArtifactService) (*Router, string, string, *http.Cookie) {
	t.Helper()
	entropy := bytes.Repeat([]byte{20}, 32*16)
	pairing := browserauth.NewPairing(nil, bytes.NewReader(entropy))
	registry := browserauth.NewRegistry(nil, bytes.NewReader(entropy))
	sessionID, _ := registry.CreateSession()
	policy, _ := NewPolicy("127.0.0.1:7943", "http://127.0.0.1:7943", "")

	targetContext, _ := target.New(func(applicationclient.Address) (target.ProbeClient, error) {
		return &fakeProbeClient{}, nil
	}, func() (target.ScopeID, error) { return "scope-1", nil }, nil)
	if err := targetContext.Select("http://127.0.0.1:8080"); err != nil {
		t.Fatal(err)
	}
	if _, domain := targetContext.SupplyCredential(
		context.Background(),
		[]byte(strings.Repeat("k", 32)),
	); domain != nil {
		t.Fatal(domain)
	}

	var inventory *traceinventory.Service
	if artifacts != nil {
		inventory = traceinventory.New(artifacts, nil, targetContext, time.Now)
	}
	router, _ := New(Options{
		Policy:         policy,
		Pairing:        pairing,
		Sessions:       registry,
		PairingURL:     func(value string) string { return value },
		Target:         targetContext,
		Artifacts:      artifacts,
		TraceInventory: inventory,
	})
	bootstrapResult, _ := registry.Bootstrap(sessionID, "")
	return router, bootstrapResult.TabID, bootstrapResult.CSRF, browserauth.SessionCookie(sessionID)
}

func artifactRequestWithCSRF(handler http.Handler, path, body string, cookie *http.Cookie, tabID, csrf string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7943"+path, strings.NewReader(body))
	request.Host = "127.0.0.1:7943"
	request.Header.Set("Origin", "http://127.0.0.1:7943")
	request.Header.Set("X-loomspan-Console-Tab", tabID)
	request.Header.Set("X-loomspan-Console-CSRF", csrf)
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestArtifactRoutesRequireSession(t *testing.T) {
	entropy := bytes.Repeat([]byte{21}, 32*16)
	pairing := browserauth.NewPairing(nil, bytes.NewReader(entropy))
	registry := browserauth.NewRegistry(nil, bytes.NewReader(entropy))
	policy, _ := NewPolicy("127.0.0.1:7943", "http://127.0.0.1:7943", "")
	router, _ := New(Options{
		Policy: policy, Pairing: pairing, Sessions: registry,
		PairingURL: func(value string) string { return value },
	})
	for _, path := range []string{
		"/api/console/v1/artifacts/acquire",
		"/api/console/v1/artifacts/storage",
		"/api/console/v1/artifacts/remove",
		"/api/console/v1/artifacts/clear-expired",
		"/api/console/v1/artifacts/clear-all-unused",
	} {
		request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7943"+path, strings.NewReader(`{}`))
		request.Host = "127.0.0.1:7943"
		request.Header.Set("Origin", "http://127.0.0.1:7943")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Errorf("%s status=%d expected 401", path, response.Code)
		}
	}
}

func TestArtifactMutationRoutesRequireCSRF(t *testing.T) {
	entropy := bytes.Repeat([]byte{22}, 32*16)
	pairing := browserauth.NewPairing(nil, bytes.NewReader(entropy))
	registry := browserauth.NewRegistry(nil, bytes.NewReader(entropy))
	sessionID, _ := registry.CreateSession()
	policy, _ := NewPolicy("127.0.0.1:7943", "http://127.0.0.1:7943", "")

	targetContext, _ := target.New(func(applicationclient.Address) (target.ProbeClient, error) {
		return &fakeProbeClient{}, nil
	}, nil, nil)
	_ = targetContext.Select("http://127.0.0.1:8080")

	router, _ := New(Options{
		Policy:     policy,
		Pairing:    pairing,
		Sessions:   registry,
		PairingURL: func(value string) string { return value },
		Target:     targetContext,
		Artifacts:  &fakeArtifactService{},
	})
	cookie := browserauth.SessionCookie(sessionID)
	for _, path := range []string{
		"/api/console/v1/artifacts/acquire",
		"/api/console/v1/artifacts/remove",
		"/api/console/v1/artifacts/clear-expired",
		"/api/console/v1/artifacts/clear-all-unused",
	} {
		request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7943"+path, strings.NewReader(`{}`))
		request.Host = "127.0.0.1:7943"
		request.Header.Set("Origin", "http://127.0.0.1:7943")
		request.AddCookie(cookie)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Errorf("%s status=%d expected 403 (CSRF required)", path, response.Code)
		}
	}
}

func TestArtifactStorageDoesNotRequireCSRF(t *testing.T) {
	fake := &fakeArtifactService{
		snapshotResult: artifact.StorageSnapshot{
			WorkspaceLabel: "/test/workspace",
			MaxBytes:       1024,
			IdleTTL:        5 * time.Minute,
			ChargedBytes:   0,
			AcquiredCount:  0,
			Entries:        []artifact.StoredEntry{},
		},
	}
	router, _, cookie := artifactTestRouter(t, fake)
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7943/api/console/v1/artifacts/storage", strings.NewReader(`{}`))
	request.Host = "127.0.0.1:7943"
	request.Header.Set("Origin", "http://127.0.0.1:7943")
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code == http.StatusForbidden {
		t.Fatalf("storage route incorrectly required CSRF: %d %s", response.Code, response.Body.String())
	}
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"workspaceLabel":"/test/workspace"`) {
		t.Fatalf("body does not contain workspace label: %s", response.Body.String())
	}
}

func TestArtifactAcquireReturnsHandle(t *testing.T) {
	fake := &fakeArtifactService{
		acquireResult: artifact.AcquiredArtifact{
			Handle: artifact.Handle("test-handle-123"),
			Metadata: artifact.TraceMetadata{
				TraceID:     "trace-1",
				SessionID:   "session-1",
				Outcome:     "SUCCEEDED",
				FinalizedAt: time.Date(2026, 7, 27, 0, 0, 2, 0, time.UTC),
			},
			LocalBytes:    100,
			AcquiredAt:    time.Date(2026, 7, 27, 0, 0, 3, 0, time.UTC),
			LastUsedAt:    time.Date(2026, 7, 27, 0, 0, 3, 0, time.UTC),
			ExpiresAt:     time.Date(2026, 7, 27, 0, 5, 3, 0, time.UTC),
			HasIdleExpiry: true,
		},
	}
	entropy := bytes.Repeat([]byte{24}, 32*16)
	pairing := browserauth.NewPairing(nil, bytes.NewReader(entropy))
	reg := browserauth.NewRegistry(nil, bytes.NewReader(entropy))
	sid, _ := reg.CreateSession()
	policy, _ := NewPolicy("127.0.0.1:7943", "http://127.0.0.1:7943", "")
	tc, _ := target.New(func(applicationclient.Address) (target.ProbeClient, error) {
		return &fakeProbeClient{}, nil
	}, func() (target.ScopeID, error) { return "scope-1", nil }, nil)
	_ = tc.Select("http://127.0.0.1:8080")
	_, _ = tc.SupplyCredential(context.Background(), []byte(strings.Repeat("k", 32)))
	r, _ := New(Options{
		Policy: policy, Pairing: pairing, Sessions: reg,
		PairingURL: func(v string) string { return v },
		Target:     tc, Artifacts: fake,
	})
	ck := browserauth.SessionCookie(sid)
	result, _ := reg.Bootstrap(sid, "")
	resp := artifactRequestWithCSRF(r, "/api/console/v1/artifacts/acquire", `{"traceId":"trace-1"}`, ck, result.TabID, result.CSRF)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"artifactHandle":"test-handle-123"`) {
		t.Fatalf("body does not contain handle: %s", resp.Body.String())
	}
	if fake.lastAcquireTraceID != "trace-1" {
		t.Fatalf("expected acquire called with trace-1, got %s", fake.lastAcquireTraceID)
	}
}

func TestArtifactRemoveReturnsRemoved(t *testing.T) {
	fake := &fakeArtifactService{}
	entropy := bytes.Repeat([]byte{25}, 32*16)
	pairing := browserauth.NewPairing(nil, bytes.NewReader(entropy))
	reg := browserauth.NewRegistry(nil, bytes.NewReader(entropy))
	sid, _ := reg.CreateSession()
	policy, _ := NewPolicy("127.0.0.1:7943", "http://127.0.0.1:7943", "")
	tc, _ := target.New(func(applicationclient.Address) (target.ProbeClient, error) {
		return &fakeProbeClient{}, nil
	}, func() (target.ScopeID, error) { return "scope-1", nil }, nil)
	_ = tc.Select("http://127.0.0.1:8080")
	_, _ = tc.SupplyCredential(context.Background(), []byte(strings.Repeat("k", 32)))
	r, _ := New(Options{
		Policy: policy, Pairing: pairing, Sessions: reg,
		PairingURL: func(v string) string { return v },
		Target:     tc, Artifacts: fake,
	})
	ck := browserauth.SessionCookie(sid)
	result, _ := reg.Bootstrap(sid, "")
	resp := artifactRequestWithCSRF(r, "/api/console/v1/artifacts/remove", `{"source":"TARGET","traceId":"trace-1"}`, ck, result.TabID, result.CSRF)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"removed":true`) {
		t.Fatalf("body does not contain removed:true: %s", resp.Body.String())
	}
	if !fake.removeCalled || fake.lastRemoveTraceID != "trace-1" {
		t.Fatalf("expected remove called with trace-1, got called=%v traceID=%s", fake.removeCalled, fake.lastRemoveTraceID)
	}
}

func TestArtifactRemoveMapsArtifactInUse(t *testing.T) {
	fake := &fakeArtifactService{
		removeErr: consolecore.NewError(consolecore.CodeArtifactInUse,
			"Artifact is in use.", "scope-1", consolecore.Details{}, nil),
	}
	entropy := bytes.Repeat([]byte{26}, 32*16)
	pairing := browserauth.NewPairing(nil, bytes.NewReader(entropy))
	reg := browserauth.NewRegistry(nil, bytes.NewReader(entropy))
	sid, _ := reg.CreateSession()
	policy, _ := NewPolicy("127.0.0.1:7943", "http://127.0.0.1:7943", "")
	tc, _ := target.New(func(applicationclient.Address) (target.ProbeClient, error) {
		return &fakeProbeClient{}, nil
	}, func() (target.ScopeID, error) { return "scope-1", nil }, nil)
	_ = tc.Select("http://127.0.0.1:8080")
	_, _ = tc.SupplyCredential(context.Background(), []byte(strings.Repeat("k", 32)))
	r, _ := New(Options{
		Policy: policy, Pairing: pairing, Sessions: reg,
		PairingURL: func(v string) string { return v },
		Target:     tc, Artifacts: fake,
	})
	ck := browserauth.SessionCookie(sid)
	result, _ := reg.Bootstrap(sid, "")
	resp := artifactRequestWithCSRF(r, "/api/console/v1/artifacts/remove", `{"source":"TARGET","traceId":"trace-1"}`, ck, result.TabID, result.CSRF)
	if resp.Code != http.StatusConflict {
		t.Fatalf("expected 409 for ARTIFACT_IN_USE, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"code":"ARTIFACT_IN_USE"`) {
		t.Fatalf("body does not contain ARTIFACT_IN_USE: %s", resp.Body.String())
	}
}

func TestArtifactClearExpiredReturnsCleared(t *testing.T) {
	fake := &fakeArtifactService{}
	entropy := bytes.Repeat([]byte{27}, 32*16)
	pairing := browserauth.NewPairing(nil, bytes.NewReader(entropy))
	reg := browserauth.NewRegistry(nil, bytes.NewReader(entropy))
	sid, _ := reg.CreateSession()
	policy, _ := NewPolicy("127.0.0.1:7943", "http://127.0.0.1:7943", "")
	tc, _ := target.New(func(applicationclient.Address) (target.ProbeClient, error) {
		return &fakeProbeClient{}, nil
	}, func() (target.ScopeID, error) { return "scope-1", nil }, nil)
	_ = tc.Select("http://127.0.0.1:8080")
	_, _ = tc.SupplyCredential(context.Background(), []byte(strings.Repeat("k", 32)))
	r, _ := New(Options{
		Policy: policy, Pairing: pairing, Sessions: reg,
		PairingURL: func(v string) string { return v },
		Target:     tc, Artifacts: fake,
	})
	ck := browserauth.SessionCookie(sid)
	result, _ := reg.Bootstrap(sid, "")
	resp := artifactRequestWithCSRF(r, "/api/console/v1/artifacts/clear-expired", `{}`, ck, result.TabID, result.CSRF)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"cleared":true`) {
		t.Fatalf("body does not contain cleared:true: %s", resp.Body.String())
	}
	if !fake.clearExpiredCalled {
		t.Fatal("expected clearExpired to be called")
	}
}

func TestArtifactClearAllUnusedReturnsCleared(t *testing.T) {
	fake := &fakeArtifactService{}
	entropy := bytes.Repeat([]byte{28}, 32*16)
	pairing := browserauth.NewPairing(nil, bytes.NewReader(entropy))
	reg := browserauth.NewRegistry(nil, bytes.NewReader(entropy))
	sid, _ := reg.CreateSession()
	policy, _ := NewPolicy("127.0.0.1:7943", "http://127.0.0.1:7943", "")
	tc, _ := target.New(func(applicationclient.Address) (target.ProbeClient, error) {
		return &fakeProbeClient{}, nil
	}, func() (target.ScopeID, error) { return "scope-1", nil }, nil)
	_ = tc.Select("http://127.0.0.1:8080")
	_, _ = tc.SupplyCredential(context.Background(), []byte(strings.Repeat("k", 32)))
	r, _ := New(Options{
		Policy: policy, Pairing: pairing, Sessions: reg,
		PairingURL: func(v string) string { return v },
		Target:     tc, Artifacts: fake,
	})
	ck := browserauth.SessionCookie(sid)
	result, _ := reg.Bootstrap(sid, "")
	resp := artifactRequestWithCSRF(r, "/api/console/v1/artifacts/clear-all-unused", `{}`, ck, result.TabID, result.CSRF)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"cleared":true`) {
		t.Fatalf("body does not contain cleared:true: %s", resp.Body.String())
	}
	if !fake.clearAllCalled {
		t.Fatal("expected clearAllUnused to be called")
	}
}

func TestArtifactClearErrorsNeverExposeArtifactOwnerAsTargetScope(t *testing.T) {
	const importedOwner = "opaque-imported-owner"
	tests := []struct {
		name      string
		path      string
		configure func(*fakeArtifactService, *consolecore.Error)
	}{
		{
			name: "expired",
			path: "/api/console/v1/artifacts/clear-expired",
			configure: func(fake *fakeArtifactService, domain *consolecore.Error) {
				fake.clearExpiredErr = domain
			},
		},
		{
			name: "all unused",
			path: "/api/console/v1/artifacts/clear-all-unused",
			configure: func(fake *fakeArtifactService, domain *consolecore.Error) {
				fake.clearAllErr = domain
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake := &fakeArtifactService{}
			domain := consolecore.NewError(
				consolecore.CodeConsoleError,
				"The Console workspace is no longer safe.",
				importedOwner,
				consolecore.Details{},
				nil,
			)
			test.configure(fake, domain)
			router, tabID, csrf, cookie := artifactTestRouterWithCSRF(t, fake)
			response := artifactRequestWithCSRF(router, test.path, `{}`, cookie, tabID, csrf)

			if response.Code != http.StatusInternalServerError {
				t.Fatalf("expected 500, got %d: %s", response.Code, response.Body.String())
			}
			body := response.Body.String()
			if !strings.Contains(body, `"code":"CONSOLE_ERROR"`) {
				t.Fatalf("body does not contain CONSOLE_ERROR: %s", body)
			}
			if strings.Contains(body, `"targetScopeId"`) || strings.Contains(body, importedOwner) {
				t.Fatalf("body exposes internal artifact owner: %s", body)
			}
		})
	}
}

func TestArtifactAcquireRejectsEmptyTraceID(t *testing.T) {
	fake := &fakeArtifactService{}
	entropy := bytes.Repeat([]byte{29}, 32*16)
	pairing := browserauth.NewPairing(nil, bytes.NewReader(entropy))
	reg := browserauth.NewRegistry(nil, bytes.NewReader(entropy))
	sid, _ := reg.CreateSession()
	policy, _ := NewPolicy("127.0.0.1:7943", "http://127.0.0.1:7943", "")
	tc, _ := target.New(func(applicationclient.Address) (target.ProbeClient, error) {
		return &fakeProbeClient{}, nil
	}, func() (target.ScopeID, error) { return "scope-1", nil }, nil)
	_ = tc.Select("http://127.0.0.1:8080")
	_, _ = tc.SupplyCredential(context.Background(), []byte(strings.Repeat("k", 32)))
	r, _ := New(Options{
		Policy: policy, Pairing: pairing, Sessions: reg,
		PairingURL: func(v string) string { return v },
		Target:     tc, Artifacts: fake,
	})
	ck := browserauth.SessionCookie(sid)
	result, _ := reg.Bootstrap(sid, "")
	resp := artifactRequestWithCSRF(r, "/api/console/v1/artifacts/acquire", `{"traceId":""}`, ck, result.TabID, result.CSRF)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty trace ID, got %d: %s", resp.Code, resp.Body.String())
	}
}

// fakeArtifactProbeClient returns a canned artifact stream for download tests.
type fakeArtifactProbeClient struct {
	artifactBody []byte
}

func (f *fakeArtifactProbeClient) Probe(context.Context, applicationclient.Credential) (applicationclient.Instance, error) {
	return applicationclient.Instance{
		InstanceID:                  "11111111-1111-4111-8111-111111111111",
		ConsoleCompatibilityVersion: "0.1.0-SNAPSHOT",
		ObservedAt:                  time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC),
		LiveMonitoringAvailable:     true,
	}, nil
}

func (f *fakeArtifactProbeClient) Get(context.Context, string, int64, applicationclient.Credential) ([]byte, string, error) {
	return nil, "", nil
}

func (f *fakeArtifactProbeClient) OpenActivity(context.Context, string, string, applicationclient.Credential) (*applicationclient.ActivityStream, error) {
	return nil, nil
}

func (f *fakeArtifactProbeClient) OpenArtifact(_ context.Context, _ string, _ string, _ applicationclient.Credential) (*applicationclient.ArtifactStream, error) {
	return applicationclient.NewTestArtifactStream(
		io.NopCloser(bytes.NewReader(f.artifactBody)),
		"11111111-1111-4111-8111-111111111111",
		int64(len(f.artifactBody)),
	), nil
}

func (f *fakeArtifactProbeClient) Close() {}

func TestArtifactRawDownloadRequiresSession(t *testing.T) {
	entropy := bytes.Repeat([]byte{30}, 32*16)
	pairing := browserauth.NewPairing(nil, bytes.NewReader(entropy))
	registry := browserauth.NewRegistry(nil, bytes.NewReader(entropy))
	policy, _ := NewPolicy("127.0.0.1:7943", "http://127.0.0.1:7943", "")
	router, _ := New(Options{
		Policy: policy, Pairing: pairing, Sessions: registry,
		PairingURL: func(value string) string { return value },
	})
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:7943/api/console/v1/artifacts/trace-1/raw", nil)
	request.Host = "127.0.0.1:7943"
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	request.Header.Set("Sec-Fetch-Mode", "navigate")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", response.Code)
	}
}

func TestArtifactRawDownloadRejectsPost(t *testing.T) {
	entropy := bytes.Repeat([]byte{31}, 32*16)
	pairing := browserauth.NewPairing(nil, bytes.NewReader(entropy))
	registry := browserauth.NewRegistry(nil, bytes.NewReader(entropy))
	sessionID, _ := registry.CreateSession()
	policy, _ := NewPolicy("127.0.0.1:7943", "http://127.0.0.1:7943", "")
	router, _ := New(Options{
		Policy: policy, Pairing: pairing, Sessions: registry,
		PairingURL: func(value string) string { return value },
	})
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7943/api/console/v1/artifacts/trace-1/raw", strings.NewReader(`{}`))
	request.Host = "127.0.0.1:7943"
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	request.Header.Set("Sec-Fetch-Mode", "navigate")
	request.AddCookie(browserauth.SessionCookie(sessionID))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", response.Code)
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", "trace"},
		{"trace-1", "trace-1"},
		{"abc123", "abc123"},
		{"a_b-c", "a_b-c"},
		{"a/b", "a_b"},
		{"a\\b", "a_b"},
		{"a.b", "a_b"},
		{"a b", "a_b"},
		{"!@#$%", "_____"},
		{strings.Repeat("x", 200), strings.Repeat("x", 128)},
		{"café", "caf_"},
		{"trace-1.json", "trace-1_json"},
	}
	for _, test := range tests {
		got := sanitizeFilename(test.input)
		if got != test.want {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", test.input, got, test.want)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		ttl         time.Duration
		neverExpire bool
		want        string
	}{
		{0, true, "Never"},
		{5 * time.Minute, true, "Never"},
		{5 * time.Minute, false, "5m0s"},
		{0, false, "0s"},
		{time.Hour + 30*time.Minute, false, "1h30m0s"},
	}
	for _, test := range tests {
		got := formatDuration(test.ttl, test.neverExpire)
		if got != test.want {
			t.Errorf("formatDuration(%v, %v) = %q, want %q", test.ttl, test.neverExpire, got, test.want)
		}
	}
}

func TestEnrichTracePageAddsArtifactAvailability(t *testing.T) {
	fake := &fakeArtifactService{
		lookupResult: artifact.LookupResult{
			LocalAvailable:          true,
			Handle:                  artifact.Handle("handle-1"),
			ApplicationAvailability: artifact.ApplicationAvailable,
		},
	}
	router, _, _ := artifactTestRouter(t, fake)
	page := observability.Page[observability.Trace]{
		Items: []observability.Trace{
			{TraceID: "trace-1"},
			{TraceID: "trace-2"},
		},
	}
	enriched := router.enrichTracePage("scope-1", page)
	for i, item := range enriched.Items {
		if !item.LocalAvailable {
			t.Fatalf("item %d: expected locally available", i)
		}
		if item.ArtifactHandle != "handle-1" {
			t.Fatalf("item %d: expected handle, got %q", i, item.ArtifactHandle)
		}
		if item.ApplicationAvailability != "AVAILABLE" {
			t.Fatalf("item %d: expected availability, got %q", i, item.ApplicationAvailability)
		}
	}
}

func TestRouterDoesNotConstructCompatibilityTraceInventory(t *testing.T) {
	router, _, _ := artifactTestRouter(t, nil)
	if router.options.TraceInventory != nil {
		t.Fatal("router constructed a compatibility trace inventory")
	}
}

func TestEnrichTracePageSkipsWhenNotAvailable(t *testing.T) {
	fake := &fakeArtifactService{
		lookupResult: artifact.LookupResult{LocalAvailable: false},
	}
	router, _, _ := artifactTestRouter(t, fake)
	page := observability.Page[observability.Trace]{
		Items: []observability.Trace{{TraceID: "trace-1"}},
	}
	enriched := router.enrichTracePage("scope-1", page)
	if enriched.Items[0].LocalAvailable {
		t.Fatalf("expected trace to remain unenriched when lookup returns not available")
	}
}

func TestEnrichTraceAddsArtifactAvailability(t *testing.T) {
	fake := &fakeArtifactService{
		lookupResult: artifact.LookupResult{
			LocalAvailable:          true,
			Handle:                  artifact.Handle("handle-2"),
			ApplicationAvailability: artifact.ApplicationUnavailable,
		},
	}
	router, _, _ := artifactTestRouter(t, fake)
	trace := observability.Trace{TraceID: "trace-1"}
	enriched := router.enrichTrace("scope-1", trace)
	if !enriched.LocalAvailable {
		t.Fatalf("expected trace to be enriched as locally available")
	}
	if enriched.ArtifactHandle != "handle-2" {
		t.Fatalf("expected artifact handle, got %q", enriched.ArtifactHandle)
	}
	if enriched.ApplicationAvailability != "UNAVAILABLE" {
		t.Fatalf("expected application availability, got %q", enriched.ApplicationAvailability)
	}
}

func TestEnrichTraceNoArtifactsServiceSkipsEnrichment(t *testing.T) {
	router, _, _ := artifactTestRouter(t, nil)
	trace := observability.Trace{TraceID: "trace-1"}
	enriched := router.enrichTrace("scope-1", trace)
	if enriched.LocalAvailable {
		t.Fatalf("expected trace to remain unenriched when artifact service is nil")
	}
}

func TestArtifactAcquireMapsTargetChanged(t *testing.T) {
	fake := &fakeArtifactService{
		acquireErr: consolecore.NewError(consolecore.CodeTargetChanged,
			"The selected target changed.", "scope-1", consolecore.Details{}, nil),
	}
	entropy := bytes.Repeat([]byte{40}, 32*16)
	pairing := browserauth.NewPairing(nil, bytes.NewReader(entropy))
	reg := browserauth.NewRegistry(nil, bytes.NewReader(entropy))
	sid, _ := reg.CreateSession()
	policy, _ := NewPolicy("127.0.0.1:7943", "http://127.0.0.1:7943", "")
	tc, _ := target.New(func(applicationclient.Address) (target.ProbeClient, error) {
		return &fakeProbeClient{}, nil
	}, func() (target.ScopeID, error) { return "scope-1", nil }, nil)
	_ = tc.Select("http://127.0.0.1:8080")
	_, _ = tc.SupplyCredential(context.Background(), []byte(strings.Repeat("k", 32)))
	r, _ := New(Options{
		Policy: policy, Pairing: pairing, Sessions: reg,
		PairingURL: func(v string) string { return v },
		Target:     tc, Artifacts: fake,
	})
	ck := browserauth.SessionCookie(sid)
	result, _ := reg.Bootstrap(sid, "")
	resp := artifactRequestWithCSRF(r, "/api/console/v1/artifacts/acquire", `{"traceId":"trace-1"}`, ck, result.TabID, result.CSRF)
	if resp.Code != http.StatusConflict {
		t.Fatalf("expected 409 for TARGET_CHANGED, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"code":"TARGET_CHANGED"`) {
		t.Fatalf("body does not contain TARGET_CHANGED: %s", resp.Body.String())
	}
}

func TestArtifactRemoveMapsTargetChanged(t *testing.T) {
	fake := &fakeArtifactService{
		removeErr: consolecore.NewError(consolecore.CodeTargetChanged,
			"The selected target changed.", "scope-1", consolecore.Details{}, nil),
	}
	entropy := bytes.Repeat([]byte{41}, 32*16)
	pairing := browserauth.NewPairing(nil, bytes.NewReader(entropy))
	reg := browserauth.NewRegistry(nil, bytes.NewReader(entropy))
	sid, _ := reg.CreateSession()
	policy, _ := NewPolicy("127.0.0.1:7943", "http://127.0.0.1:7943", "")
	tc, _ := target.New(func(applicationclient.Address) (target.ProbeClient, error) {
		return &fakeProbeClient{}, nil
	}, func() (target.ScopeID, error) { return "scope-1", nil }, nil)
	_ = tc.Select("http://127.0.0.1:8080")
	_, _ = tc.SupplyCredential(context.Background(), []byte(strings.Repeat("k", 32)))
	r, _ := New(Options{
		Policy: policy, Pairing: pairing, Sessions: reg,
		PairingURL: func(v string) string { return v },
		Target:     tc, Artifacts: fake,
	})
	ck := browserauth.SessionCookie(sid)
	result, _ := reg.Bootstrap(sid, "")
	resp := artifactRequestWithCSRF(r, "/api/console/v1/artifacts/remove", `{"source":"TARGET","traceId":"trace-1"}`, ck, result.TabID, result.CSRF)
	if resp.Code != http.StatusConflict {
		t.Fatalf("expected 409 for TARGET_CHANGED, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"code":"TARGET_CHANGED"`) {
		t.Fatalf("body does not contain TARGET_CHANGED: %s", resp.Body.String())
	}
}

func TestArtifactStorageMapsTargetChanged(t *testing.T) {
	fake := &fakeArtifactService{
		snapshotErr: consolecore.NewError(consolecore.CodeTargetChanged,
			"The selected target changed.", "scope-1", consolecore.Details{}, nil),
	}
	entropy := bytes.Repeat([]byte{42}, 32*16)
	pairing := browserauth.NewPairing(nil, bytes.NewReader(entropy))
	reg := browserauth.NewRegistry(nil, bytes.NewReader(entropy))
	sid, _ := reg.CreateSession()
	policy, _ := NewPolicy("127.0.0.1:7943", "http://127.0.0.1:7943", "")
	tc, _ := target.New(func(applicationclient.Address) (target.ProbeClient, error) {
		return &fakeProbeClient{}, nil
	}, func() (target.ScopeID, error) { return "scope-1", nil }, nil)
	_ = tc.Select("http://127.0.0.1:8080")
	_, _ = tc.SupplyCredential(context.Background(), []byte(strings.Repeat("k", 32)))
	r, _ := New(Options{
		Policy: policy, Pairing: pairing, Sessions: reg,
		PairingURL: func(v string) string { return v },
		Target:     tc, Artifacts: fake,
	})
	ck := browserauth.SessionCookie(sid)
	resp := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7943/api/console/v1/artifacts/storage", strings.NewReader(`{}`))
	resp.Host = "127.0.0.1:7943"
	resp.Header.Set("Origin", "http://127.0.0.1:7943")
	resp.AddCookie(ck)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, resp)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for TARGET_CHANGED, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"code":"TARGET_CHANGED"`) {
		t.Fatalf("body does not contain TARGET_CHANGED: %s", rec.Body.String())
	}
}

// undeclaredLengthArtifactProbeClient returns an ArtifactStream with
// DeclaredLength() == -1, simulating a chunked transfer with no
// Content-Length header from the upstream.
type undeclaredLengthArtifactProbeClient struct {
	artifactBody []byte
}

func (c *undeclaredLengthArtifactProbeClient) Probe(context.Context, applicationclient.Credential) (applicationclient.Instance, error) {
	return applicationclient.Instance{
		InstanceID:                  "11111111-1111-4111-8111-111111111111",
		ConsoleCompatibilityVersion: "0.1.0-SNAPSHOT",
	}, nil
}

func (c *undeclaredLengthArtifactProbeClient) Get(context.Context, string, int64, applicationclient.Credential) ([]byte, string, error) {
	return nil, "", nil
}

func (c *undeclaredLengthArtifactProbeClient) OpenActivity(context.Context, string, string, applicationclient.Credential) (*applicationclient.ActivityStream, error) {
	return nil, nil
}

func (c *undeclaredLengthArtifactProbeClient) OpenArtifact(_ context.Context, _, _ string, _ applicationclient.Credential) (*applicationclient.ArtifactStream, error) {
	return applicationclient.NewTestArtifactStream(
		io.NopCloser(bytes.NewReader(c.artifactBody)),
		"11111111-1111-4111-8111-111111111111",
		-1,
	), nil
}

func (c *undeclaredLengthArtifactProbeClient) Close() {}

func TestArtifactRawDownloadStreamsWithUndeclaredLength(t *testing.T) {
	artifactBody := `{"kind":"TRACE_STARTED"}
`
	entropy := bytes.Repeat([]byte{43}, 32*16)
	pairing := browserauth.NewPairing(nil, bytes.NewReader(entropy))
	registry := browserauth.NewRegistry(nil, bytes.NewReader(entropy))
	sessionID, _ := registry.CreateSession()
	policy, _ := NewPolicy("127.0.0.1:7943", "http://127.0.0.1:7943", "")
	client := &undeclaredLengthArtifactProbeClient{artifactBody: []byte(artifactBody)}
	targetContext, _ := target.New(func(applicationclient.Address) (target.ProbeClient, error) {
		return client, nil
	}, func() (target.ScopeID, error) { return "scope-1", nil }, nil)
	if err := targetContext.Select("http://127.0.0.1:8080"); err != nil {
		t.Fatal(err)
	}
	if _, domain := targetContext.SupplyCredential(
		context.Background(),
		[]byte(strings.Repeat("k", 32)),
	); domain != nil {
		t.Fatal(domain)
	}
	router, _ := New(Options{
		Policy: policy, Pairing: pairing, Sessions: registry,
		PairingURL: func(value string) string { return value },
		Target:     targetContext,
	})
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:7943/api/console/v1/artifacts/trace-1/raw", nil)
	request.Host = "127.0.0.1:7943"
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	request.Header.Set("Sec-Fetch-Mode", "navigate")
	request.AddCookie(browserauth.SessionCookie(sessionID))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	if response.Body.String() != artifactBody {
		t.Fatalf("body mismatch: expected %q, got %q", artifactBody, response.Body.String())
	}
	if response.Header().Get("Content-Length") != "" {
		t.Fatalf("expected no Content-Length for undeclared stream, got %q", response.Header().Get("Content-Length"))
	}
}

// ctxBlockedReader blocks on Read until the context is done, then returns
// the context error. Used to test that raw download stops on client
// cancellation. It tracks Close calls so tests can assert the upstream
// stream was released.
type ctxBlockedReader struct {
	ctx    context.Context
	closed atomic.Bool
}

func (r *ctxBlockedReader) Read(_ []byte) (int, error) {
	<-r.ctx.Done()
	return 0, r.ctx.Err()
}

func (r *ctxBlockedReader) Close() error {
	r.closed.Store(true)
	return nil
}

// blockingArtifactProbeClient returns an ArtifactStream whose body blocks on
// the operation context, so cancellation interrupts the read. It exposes the
// last stream's reader so tests can assert Close was called.
type blockingArtifactProbeClient struct {
	lastReader atomic.Pointer[ctxBlockedReader]
}

func (*blockingArtifactProbeClient) Probe(context.Context, applicationclient.Credential) (applicationclient.Instance, error) {
	return applicationclient.Instance{
		InstanceID:                  "11111111-1111-4111-8111-111111111111",
		ConsoleCompatibilityVersion: "0.1.0-SNAPSHOT",
	}, nil
}

func (*blockingArtifactProbeClient) Get(context.Context, string, int64, applicationclient.Credential) ([]byte, string, error) {
	return nil, "", nil
}

func (*blockingArtifactProbeClient) OpenActivity(context.Context, string, string, applicationclient.Credential) (*applicationclient.ActivityStream, error) {
	return nil, nil
}

func (c *blockingArtifactProbeClient) OpenArtifact(ctx context.Context, _, _ string, _ applicationclient.Credential) (*applicationclient.ArtifactStream, error) {
	reader := &ctxBlockedReader{ctx: ctx}
	c.lastReader.Store(reader)
	return applicationclient.NewTestArtifactStream(
		reader,
		"11111111-1111-4111-8111-111111111111",
		-1,
	), nil
}

func (*blockingArtifactProbeClient) Close() {}

func TestArtifactRawDownloadStopsOnClientCancellation(t *testing.T) {
	entropy := bytes.Repeat([]byte{44}, 32*16)
	pairing := browserauth.NewPairing(nil, bytes.NewReader(entropy))
	registry := browserauth.NewRegistry(nil, bytes.NewReader(entropy))
	sessionID, _ := registry.CreateSession()
	policy, _ := NewPolicy("127.0.0.1:7943", "http://127.0.0.1:7943", "")
	probeClient := &blockingArtifactProbeClient{}
	targetContext, _ := target.New(func(applicationclient.Address) (target.ProbeClient, error) {
		return probeClient, nil
	}, func() (target.ScopeID, error) { return "scope-1", nil }, nil)
	if err := targetContext.Select("http://127.0.0.1:8080"); err != nil {
		t.Fatal(err)
	}
	if _, domain := targetContext.SupplyCredential(
		context.Background(),
		[]byte(strings.Repeat("k", 32)),
	); domain != nil {
		t.Fatal(domain)
	}
	router, _ := New(Options{
		Policy: policy, Pairing: pairing, Sessions: registry,
		PairingURL: func(value string) string { return value },
		Target:     targetContext,
	})
	ctx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:7943/api/console/v1/artifacts/trace-1/raw", nil)
	request = request.WithContext(ctx)
	request.Host = "127.0.0.1:7943"
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	request.Header.Set("Sec-Fetch-Mode", "navigate")
	request.AddCookie(browserauth.SessionCookie(sessionID))
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		router.ServeHTTP(response, request)
		close(done)
	}()
	// Wait briefly for the handler to open the upstream stream before
	// cancelling, so we can assert the stream is closed on cancellation.
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("raw download did not stop after client cancellation")
	}
	// The handler's defer stream.Close() must release the upstream stream on
	// cancellation, not leave it blocked forever.
	reader := probeClient.lastReader.Load()
	if reader == nil {
		t.Fatal("expected the probe client to have opened a stream")
	}
	if !reader.closed.Load() {
		t.Fatal("upstream stream was not closed after client cancellation")
	}
}
