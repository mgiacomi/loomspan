package browserapi

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/applicationclient"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/browserauth"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/observability"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/target"
)

type fakeProbeClient struct{}

func (*fakeProbeClient) Probe(context.Context, applicationclient.Credential) (applicationclient.Instance, error) {
	return applicationclient.Instance{}, nil
}
func (*fakeProbeClient) Get(context.Context, string, int64, applicationclient.Credential) ([]byte, string, error) {
	return nil, "", nil
}
func (*fakeProbeClient) OpenActivity(context.Context, string, string, applicationclient.Credential) (*applicationclient.ActivityStream, error) {
	return nil, nil
}
func (*fakeProbeClient) OpenArtifact(context.Context, string, string, applicationclient.Credential) (*applicationclient.ArtifactStream, error) {
	return nil, nil
}
func (*fakeProbeClient) Close() {}

type fixtureObservabilityClient struct {
	failRequests bool
}

func (*fixtureObservabilityClient) Probe(context.Context, applicationclient.Credential) (applicationclient.Instance, error) {
	return applicationclient.Instance{
		InstanceID:                  "11111111-1111-4111-8111-111111111111",
		ConsoleCompatibilityVersion: "0.1.0-SNAPSHOT",
		ObservedAt:                  time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC),
		LiveMonitoringAvailable:     true,
	}, nil
}

func (client *fixtureObservabilityClient) Get(_ context.Context, endpoint string, _ int64, _ applicationclient.Credential) ([]byte, string, error) {
	if client.failRequests {
		return nil, "", errors.New("application unavailable")
	}
	const instanceID = "11111111-1111-4111-8111-111111111111"
	var body string
	switch {
	case strings.HasSuffix(endpoint, "/instance"):
		body = `{"instanceId":"` + instanceID + `","consoleCompatibilityVersion":"0.1.0-SNAPSHOT","observedAt":"2026-07-27T00:00:00Z","liveMonitoringAvailable":true,"registeredSkillCount":1,"activeExecutionCount":1,"catalogedTraceCount":1,"tracePersistencePolicy":"PERSISTENT","completionGraceTtl":"PT2M","traceCatalogMetadataTtl":"PT168H"}`
	case strings.Contains(endpoint, "/skills/"):
		body = `{"registeredName":"CheckDns","sourcePath":"classpath:/skills/check-dns.yaml","yaml":"name: CheckDns"}`
	case strings.Contains(endpoint, "/skills"):
		body = `{"items":[{"registeredName":"CheckDns","sourcePath":"classpath:/skills/check-dns.yaml"}],"hasMore":false,"nextCursor":null,"observedAt":"2026-07-27T00:00:00Z"}`
	case strings.Contains(endpoint, "/active-executions/"):
		body = activeExecutionFixture()
	case strings.Contains(endpoint, "/active-executions"):
		body = `{"items":[` + activeExecutionFixture() + `],"hasMore":false,"nextCursor":null,"observedAt":"2026-07-27T00:00:00Z","resumeCursor":"9"}`
	case strings.Contains(endpoint, "/traces/"):
		body = traceFixture()
	case strings.Contains(endpoint, "/traces"):
		body = `{"items":[` + traceFixture() + `],"hasMore":false,"nextCursor":null,"observedAt":"2026-07-27T00:00:00Z"}`
	}
	return []byte(body), instanceID, nil
}

func (*fixtureObservabilityClient) OpenActivity(context.Context, string, string, applicationclient.Credential) (*applicationclient.ActivityStream, error) {
	return nil, nil
}
func (*fixtureObservabilityClient) OpenArtifact(context.Context, string, string, applicationclient.Credential) (*applicationclient.ArtifactStream, error) {
	return nil, nil
}
func (*fixtureObservabilityClient) Close() {}

func activeExecutionFixture() string {
	return `{"sessionId":"session-1","traceId":"trace-1","lastCanonicalSequence":7,"startedAt":"2026-07-27T00:00:00Z","updatedAt":"2026-07-27T00:00:01Z","elapsedMillis":1000,"entrySkill":"CheckDns","status":"ACTIVE","phase":"RUNNING","summary":"Checking DNS","activePath":[{"frameId":"frame-1","frameType":"SKILL_EXECUTION","route":"CheckDns"}],"totalFrameDepth":1,"activePathTruncated":false,"usage":{"skillInvocations":1,"toolInvocations":0,"linterRetries":0,"modelCalls":1,"providerAttempts":1,"promptUnits":10,"completionUnits":5,"usageUnits":15,"exactModelResponses":1,"heuristicModelResponses":0,"unavailableModelResponses":0},"configuredLimits":{"maxSkillInvocations":64,"maxToolInvocations":128,"maxLinterRetries":32,"maxModelCalls":64,"maxProviderAttempts":192,"maxUsageUnits":200000}}`
}

func traceFixture() string {
	return `{"traceId":"trace-1","sessionId":"session-1","entrySkill":"CheckDns","outcome":"SUCCEEDED","finalizedAt":"2026-07-27T00:00:02Z","sizeBytes":100,"persistencePolicy":"PERSISTENT","applicationTraceExpiresAt":"2026-07-28T00:00:02Z"}`
}

func TestObservabilityRoutesRequireSession(t *testing.T) {
	entropy := bytes.Repeat([]byte{5}, 32*16)
	pairing := browserauth.NewPairing(nil, bytes.NewReader(entropy))
	registry := browserauth.NewRegistry(nil, bytes.NewReader(entropy))
	policy, _ := NewPolicy("127.0.0.1:7943", "http://127.0.0.1:7943", "")
	router, _ := New(Options{
		Policy: policy, Pairing: pairing, Sessions: registry,
		PairingURL: func(value string) string { return value },
	})
	for _, path := range []string{
		"/api/console/v1/observability/instance",
		"/api/console/v1/skills/list",
		"/api/console/v1/skills/detail",
		"/api/console/v1/active-executions/list",
		"/api/console/v1/active-executions/detail",
		"/api/console/v1/traces/list",
		"/api/console/v1/traces/detail",
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

func TestObservabilityRoutesDoNotRequireCSRF(t *testing.T) {
	entropy := bytes.Repeat([]byte{6}, 32*16)
	pairing := browserauth.NewPairing(nil, bytes.NewReader(entropy))
	registry := browserauth.NewRegistry(nil, bytes.NewReader(entropy))
	sessionID, _ := registry.CreateSession()
	policy, _ := NewPolicy("127.0.0.1:7943", "http://127.0.0.1:7943", "")

	router, _ := New(Options{
		Policy:        policy,
		Pairing:       pairing,
		Sessions:      registry,
		PairingURL:    func(value string) string { return value },
		Observability: observability.New(),
	})

	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7943/api/console/v1/observability/instance", strings.NewReader(`{}`))
	request.Host = "127.0.0.1:7943"
	request.Header.Set("Origin", "http://127.0.0.1:7943")
	request.AddCookie(browserauth.SessionCookie(sessionID))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code == http.StatusForbidden {
		t.Fatalf("observability route incorrectly required CSRF: %d %s", response.Code, response.Body.String())
	}
}

func TestObservabilityRoutesRejectInvalidJSON(t *testing.T) {
	entropy := bytes.Repeat([]byte{8}, 32*16)
	pairing := browserauth.NewPairing(nil, bytes.NewReader(entropy))
	registry := browserauth.NewRegistry(nil, bytes.NewReader(entropy))
	sessionID, _ := registry.CreateSession()
	policy, _ := NewPolicy("127.0.0.1:7943", "http://127.0.0.1:7943", "")

	targetContext, _ := target.New(func(applicationclient.Address) (target.ProbeClient, error) {
		return &fakeProbeClient{}, nil
	}, nil, nil)
	_ = targetContext.Select("http://127.0.0.1:8080")

	router, _ := New(Options{
		Policy:        policy,
		Pairing:       pairing,
		Sessions:      registry,
		PairingURL:    func(value string) string { return value },
		Target:        targetContext,
		Observability: observability.New(),
	})

	for _, path := range []string{
		"/api/console/v1/observability/instance",
		"/api/console/v1/skills/list",
		"/api/console/v1/skills/detail",
		"/api/console/v1/active-executions/list",
		"/api/console/v1/active-executions/detail",
		"/api/console/v1/traces/list",
		"/api/console/v1/traces/detail",
	} {
		request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7943"+path, strings.NewReader(`{"unexpected":true}`))
		request.Host = "127.0.0.1:7943"
		request.Header.Set("Origin", "http://127.0.0.1:7943")
		request.AddCookie(browserauth.SessionCookie(sessionID))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest ||
			!strings.Contains(response.Body.String(), `"code":"INVALID_REQUEST"`) {
			t.Errorf("%s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
}

func TestObservabilityRoutesApplySecurityHeadersAndCacheControl(t *testing.T) {
	entropy := bytes.Repeat([]byte{10}, 32*16)
	pairing := browserauth.NewPairing(nil, bytes.NewReader(entropy))
	registry := browserauth.NewRegistry(nil, bytes.NewReader(entropy))
	sessionID, _ := registry.CreateSession()
	policy, _ := NewPolicy("127.0.0.1:7943", "http://127.0.0.1:7943", "")

	targetContext, _ := target.New(func(applicationclient.Address) (target.ProbeClient, error) {
		return &fakeProbeClient{}, nil
	}, nil, nil)
	_ = targetContext.Select("http://127.0.0.1:8080")

	router, _ := New(Options{
		Policy:        policy,
		Pairing:       pairing,
		Sessions:      registry,
		PairingURL:    func(value string) string { return value },
		Target:        targetContext,
		Observability: observability.New(),
	})

	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7943/api/console/v1/observability/instance", strings.NewReader(`{}`))
	request.Host = "127.0.0.1:7943"
	request.Header.Set("Origin", "http://127.0.0.1:7943")
	request.AddCookie(browserauth.SessionCookie(sessionID))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("expected Cache-Control: no-store, got %q", response.Header().Get("Cache-Control"))
	}
	if response.Header().Get("X-Frame-Options") != "DENY" {
		t.Errorf("expected X-Frame-Options: DENY, got %q", response.Header().Get("X-Frame-Options"))
	}
	if response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("expected X-Content-Type-Options: nosniff, got %q", response.Header().Get("X-Content-Type-Options"))
	}
	if response.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Errorf("expected Referrer-Policy: no-referrer, got %q", response.Header().Get("Referrer-Policy"))
	}
}

func TestObservabilityRoutesRejectOversizedBody(t *testing.T) {
	entropy := bytes.Repeat([]byte{11}, 32*16)
	pairing := browserauth.NewPairing(nil, bytes.NewReader(entropy))
	registry := browserauth.NewRegistry(nil, bytes.NewReader(entropy))
	sessionID, _ := registry.CreateSession()
	policy, _ := NewPolicy("127.0.0.1:7943", "http://127.0.0.1:7943", "")

	targetContext, _ := target.New(func(applicationclient.Address) (target.ProbeClient, error) {
		return &fakeProbeClient{}, nil
	}, nil, nil)
	_ = targetContext.Select("http://127.0.0.1:8080")

	router, _ := New(Options{
		Policy:        policy,
		Pairing:       pairing,
		Sessions:      registry,
		PairingURL:    func(value string) string { return value },
		Target:        targetContext,
		Observability: observability.New(),
	})

	oversized := strings.Repeat(" ", maxObservabilityJSONBody+1)
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7943/api/console/v1/skills/list", strings.NewReader(oversized))
	request.Host = "127.0.0.1:7943"
	request.Header.Set("Origin", "http://127.0.0.1:7943")
	request.AddCookie(browserauth.SessionCookie(sessionID))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for oversized body, got %d", response.Code)
	}
}

func TestObservabilityRoutesReturnCanonicalDTOs(t *testing.T) {
	entropy := bytes.Repeat([]byte{12}, 32*16)
	pairing := browserauth.NewPairing(nil, bytes.NewReader(entropy))
	registry := browserauth.NewRegistry(nil, bytes.NewReader(entropy))
	sessionID, _ := registry.CreateSession()
	policy, _ := NewPolicy("127.0.0.1:7943", "http://127.0.0.1:7943", "")
	client := &fixtureObservabilityClient{}
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
		Policy:        policy,
		Pairing:       pairing,
		Sessions:      registry,
		PairingURL:    func(value string) string { return value },
		Target:        targetContext,
		Observability: observability.New(),
	})
	tests := []struct {
		path string
		body string
		want string
	}{
		{"/api/console/v1/observability/instance", `{}`, `"registeredSkillCount":1`},
		{"/api/console/v1/skills/list", `{"cursor":"","pageSize":10}`, `"registeredName":"CheckDns"`},
		{"/api/console/v1/skills/detail", `{"registeredName":"CheckDns"}`, `"yaml":"name: CheckDns"`},
		{"/api/console/v1/active-executions/list", `{"cursor":"","pageSize":10}`, `"resumeCursor":"9"`},
		{"/api/console/v1/active-executions/detail", `{"sessionId":"session-1"}`, `"frameId":"frame-1"`},
		{"/api/console/v1/traces/list", `{"cursor":"","pageSize":10}`, `"traceId":"trace-1"`},
		{"/api/console/v1/traces/detail", `{"traceId":"trace-1"}`, `"outcome":"SUCCEEDED"`},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			response := apiRequest(router, test.path, test.body, browserauth.SessionCookie(sessionID))
			if response.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if !strings.Contains(response.Body.String(), test.want) {
				t.Fatalf("body does not contain %s: %s", test.want, response.Body.String())
			}
			if response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("expected Cache-Control: no-store, got %q", response.Header().Get("Cache-Control"))
			}
		})
	}
}

func TestTraceRoutesFallBackToInstalledAcquisitionFacts(t *testing.T) {
	entropy := bytes.Repeat([]byte{13}, 32*16)
	pairing := browserauth.NewPairing(nil, bytes.NewReader(entropy))
	registry := browserauth.NewRegistry(nil, bytes.NewReader(entropy))
	sessionID, _ := registry.CreateSession()
	policy, _ := NewPolicy("127.0.0.1:7943", "http://127.0.0.1:7943", "")
	client := &fixtureObservabilityClient{}
	targetContext, _ := target.New(func(applicationclient.Address) (target.ProbeClient, error) {
		return client, nil
	}, func() (target.ScopeID, error) { return "scope-1", nil }, nil)
	if err := targetContext.Select("http://127.0.0.1:8080"); err != nil {
		t.Fatal(err)
	}
	if _, domain := targetContext.SupplyCredential(context.Background(), []byte(strings.Repeat("k", 32))); domain != nil {
		t.Fatal(domain)
	}
	acquiredAt := time.Date(2026, 7, 27, 0, 0, 3, 0, time.UTC)
	metadata := artifact.TraceMetadata{
		TraceID: "trace-1", SessionID: "session-1", EntrySkill: "CheckDns", Outcome: "SUCCEEDED",
		FinalizedAt: acquiredAt.Add(-time.Minute), SizeBytes: 100,
		PersistencePolicy: "RETAINED", ApplicationTraceExpiresAt: acquiredAt.Add(time.Hour),
	}
	artifacts := &fakeArtifactService{
		lookupResult: artifact.LookupResult{
			Handle: artifact.Handle(strings.Repeat("a", 64)), Metadata: metadata,
			LocalAvailable: true, ApplicationAvailability: artifact.ApplicationAvailable,
			AcquiredAt: acquiredAt, LastUsedAt: acquiredAt, LocalBytes: 100,
		},
		snapshotResult: artifact.StorageSnapshot{Entries: []artifact.StoredEntry{{
			TraceID: "trace-1", AcquiredAt: acquiredAt,
		}}},
	}
	router, _ := New(Options{
		Policy: policy, Pairing: pairing, Sessions: registry,
		PairingURL: func(value string) string { return value },
		Target:     targetContext, Observability: observability.New(), Artifacts: artifacts,
	})
	client.failRequests = true

	for _, test := range []struct {
		path string
		body string
	}{
		{"/api/console/v1/traces/detail", `{"traceId":"trace-1"}`},
		{"/api/console/v1/traces/list", `{"pageSize":10}`},
	} {
		response := apiRequest(router, test.path, test.body, browserauth.SessionCookie(sessionID))
		if response.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", test.path, response.Code, response.Body.String())
		}
		body := response.Body.String()
		for _, want := range []string{
			`"traceId":"trace-1"`, `"entrySkill":"CheckDns"`, `"localAvailable":true`,
			`"applicationAvailability":"AVAILABLE"`, `"persistencePolicy":"RETAINED"`,
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("%s body missing %s: %s", test.path, want, body)
			}
		}
	}
}
