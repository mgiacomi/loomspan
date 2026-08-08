package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/applicationclient"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/target"
)

const testTimeout = time.Second

func testCredential() []byte {
	return []byte(strings.Repeat("k", 32))
}

func setupTargetWithCredential(t *testing.T, client target.ProbeClient) *target.Context {
	t.Helper()
	targetContext, _ := target.New(func(applicationclient.Address) (target.ProbeClient, error) { return client, nil },
		func() (target.ScopeID, error) { return "scope-1", nil }, nil)
	if err := targetContext.Select("http://127.0.0.1:8080"); err != nil {
		t.Fatal(err)
	}
	if _, domain := targetContext.SupplyCredential(context.Background(), testCredential()); domain != nil {
		t.Fatal(domain)
	}
	return targetContext
}

type fakeGetClient struct {
	body       []byte
	instanceID string
	err        error
}

func (client *fakeGetClient) Probe(context.Context, applicationclient.Credential) (applicationclient.Instance, error) {
	return applicationclient.Instance{
		InstanceID:                  client.instanceID,
		ConsoleCompatibilityVersion: "0.1.0-SNAPSHOT",
		ObservedAt:                  time.Now(),
		LiveMonitoringAvailable:     true,
	}, nil
}
func (client *fakeGetClient) Get(_ context.Context, _ string, _ int64, _ applicationclient.Credential) ([]byte, string, error) {
	return client.body, client.instanceID, client.err
}
func (*fakeGetClient) Close() {}
func (c *fakeGetClient) OpenActivity(context.Context, string, string, applicationclient.Credential) (*applicationclient.ActivityStream, error) {
	return nil, nil
}
func (c *fakeGetClient) OpenArtifact(context.Context, string, string, applicationclient.Credential) (*applicationclient.ArtifactStream, error) {
	return nil, nil
}

func TestObservabilityServiceReturnsSkillPageFromFixture(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("..", "..", "..", "loomspan-console-fixtures", "application-rest", "skills-page.json"))
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeGetClient{body: fixture, instanceID: "11111111-1111-4111-8111-111111111111"}
	targetContext := setupTargetWithCredential(t, client)
	service := New()
	scope, domain := targetContext.Capture()
	if domain != nil {
		t.Fatal(domain)
	}
	page, domain := service.ListSkills(context.Background(), scope, ListRequest{})
	if domain != nil {
		t.Fatal(domain)
	}
	if len(page.Items) != 1 || page.Items[0].RegisteredName != "CheckDns" {
		t.Fatalf("unexpected page: %#v", page)
	}
	if page.HasMore {
		t.Fatal("expected hasMore=false")
	}
	if page.TargetScopeID != "scope-1" {
		t.Fatalf("expected targetScopeId=scope-1, got %q", page.TargetScopeID)
	}
}

func TestObservabilityServiceReturnsInstanceStatusFromFixture(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("..", "..", "..", "loomspan-console-fixtures", "application-rest", "instance-status.json"))
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeGetClient{body: fixture, instanceID: "11111111-1111-4111-8111-111111111111"}
	targetContext := setupTargetWithCredential(t, client)
	service := New()
	scope, domain := targetContext.Capture()
	if domain != nil {
		t.Fatal(domain)
	}
	status, domain := service.GetInstance(context.Background(), scope)
	if domain != nil {
		t.Fatal(domain)
	}
	if status.InstanceID != "11111111-1111-4111-8111-111111111111" {
		t.Fatalf("unexpected instance ID: %s", status.InstanceID)
	}
	if status.RegisteredSkillCount != 1 {
		t.Fatalf("unexpected skill count: %d", status.RegisteredSkillCount)
	}
	if status.TargetScopeID != "scope-1" {
		t.Fatalf("expected targetScopeId=scope-1, got %q", status.TargetScopeID)
	}
}

func TestObservabilityServiceReturnsActivePageWithResumeCursorFromFixture(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("..", "..", "..", "loomspan-console-fixtures", "application-rest", "active-executions-page.json"))
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeGetClient{body: fixture, instanceID: "11111111-1111-4111-8111-111111111111"}
	targetContext := setupTargetWithCredential(t, client)
	service := New()
	scope, domain := targetContext.Capture()
	if domain != nil {
		t.Fatal(domain)
	}
	page, domain := service.ListActiveExecutions(context.Background(), scope, ListRequest{})
	if domain != nil {
		t.Fatal(domain)
	}
	if len(page.Items) != 1 || page.Items[0].SessionID != "session-1" {
		t.Fatalf("unexpected page: %#v", page)
	}
	if page.ResumeCursor == nil || *page.ResumeCursor != "9" {
		t.Fatalf("expected resumeCursor=9, got %v", page.ResumeCursor)
	}
	if page.Items[0].TargetScopeID != "scope-1" {
		t.Fatalf("expected nested active execution scope, got %q", page.Items[0].TargetScopeID)
	}
}

func TestObservabilityServiceReturnsTracePageFromFixture(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("..", "..", "..", "loomspan-console-fixtures", "application-rest", "traces-page.json"))
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeGetClient{body: fixture, instanceID: "11111111-1111-4111-8111-111111111111"}
	targetContext := setupTargetWithCredential(t, client)
	service := New()
	scope, domain := targetContext.Capture()
	if domain != nil {
		t.Fatal(domain)
	}
	page, domain := service.ListTraces(context.Background(), scope, ListRequest{})
	if domain != nil {
		t.Fatal(domain)
	}
	if len(page.Items) != 1 || page.Items[0].TraceID != "trace-1" {
		t.Fatalf("unexpected page: %#v", page)
	}
	if page.Items[0].TargetScopeID != "scope-1" {
		t.Fatalf("expected nested trace scope, got %q", page.Items[0].TargetScopeID)
	}
}

func TestObservabilityServiceRejectsSemanticallyInvalidSuccessResponses(t *testing.T) {
	tests := []struct {
		name string
		body string
		call func(*Service, target.Scope) *consolecore.Error
	}{
		{
			name: "empty instance",
			body: `{}`,
			call: func(service *Service, scope target.Scope) *consolecore.Error {
				_, domain := service.GetInstance(context.Background(), scope)
				return domain
			},
		},
		{
			name: "missing page fields",
			body: `{}`,
			call: func(service *Service, scope target.Scope) *consolecore.Error {
				_, domain := service.ListSkills(context.Background(), scope, ListRequest{})
				return domain
			},
		},
		{
			name: "continuing page without cursor",
			body: `{"items":[],"hasMore":true,"nextCursor":null,"observedAt":"2026-07-25T12:00:00Z"}`,
			call: func(service *Service, scope target.Scope) *consolecore.Error {
				_, domain := service.ListTraces(context.Background(), scope, ListRequest{})
				return domain
			},
		},
		{
			name: "detail identifier mismatch",
			body: `{"registeredName":"OtherSkill","sourcePath":"classpath:/other.yaml","yaml":"name: OtherSkill"}`,
			call: func(service *Service, scope target.Scope) *consolecore.Error {
				_, domain := service.GetSkill(context.Background(), scope, "CheckDns")
				return domain
			},
		},
		{
			name: "trace list missing entry skill",
			body: `{"items":[{"traceId":"trace-1","sessionId":"session-1","outcome":"SUCCEEDED","finalizedAt":"2026-07-25T12:00:00Z","sizeBytes":1,"persistencePolicy":"ALWAYS","applicationTraceExpiresAt":"2026-07-25T12:15:00Z"}],"hasMore":false,"nextCursor":null,"observedAt":"2026-07-25T12:00:00Z"}`,
			call: func(service *Service, scope target.Scope) *consolecore.Error {
				_, domain := service.ListTraces(context.Background(), scope, ListRequest{})
				return domain
			},
		},
		{
			name: "trace detail empty entry skill",
			body: `{"traceId":"trace-1","sessionId":"session-1","entrySkill":"","outcome":"SUCCEEDED","finalizedAt":"2026-07-25T12:00:00Z","sizeBytes":1,"persistencePolicy":"ALWAYS","applicationTraceExpiresAt":"2026-07-25T12:15:00Z"}`,
			call: func(service *Service, scope target.Scope) *consolecore.Error {
				_, domain := service.GetTrace(context.Background(), scope, "trace-1")
				return domain
			},
		},
		{
			name: "trace list empty entry skill",
			body: `{"items":[{"traceId":"trace-1","sessionId":"session-1","entrySkill":"","outcome":"SUCCEEDED","finalizedAt":"2026-07-25T12:00:00Z","sizeBytes":1,"persistencePolicy":"ALWAYS","applicationTraceExpiresAt":"2026-07-25T12:15:00Z"}],"hasMore":false,"nextCursor":null,"observedAt":"2026-07-25T12:00:00Z"}`,
			call: func(service *Service, scope target.Scope) *consolecore.Error {
				_, domain := service.ListTraces(context.Background(), scope, ListRequest{})
				return domain
			},
		},
		{
			name: "trace detail missing entry skill",
			body: `{"traceId":"trace-1","sessionId":"session-1","outcome":"SUCCEEDED","finalizedAt":"2026-07-25T12:00:00Z","sizeBytes":1,"persistencePolicy":"ALWAYS","applicationTraceExpiresAt":"2026-07-25T12:15:00Z"}`,
			call: func(service *Service, scope target.Scope) *consolecore.Error {
				_, domain := service.GetTrace(context.Background(), scope, "trace-1")
				return domain
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &fakeGetClient{
				body:       []byte(test.body),
				instanceID: "11111111-1111-4111-8111-111111111111",
			}
			targetContext := setupTargetWithCredential(t, client)
			scope, domain := targetContext.Capture()
			if domain != nil {
				t.Fatal(domain)
			}
			domain = test.call(New(), scope)
			if domain == nil || domain.Code != consolecore.CodeConsoleError {
				t.Fatalf("expected CONSOLE_ERROR, got %#v", domain)
			}
			if domain.TargetScopeID != "scope-1" {
				t.Fatalf("expected scope-bound error, got %#v", domain)
			}
		})
	}
}

func TestObservabilityServiceClampsPageSize(t *testing.T) {
	size, domain := clampPageSize(0)
	if domain != nil || size != defaultPageSize {
		t.Fatalf("expected default page size, got %d", size)
	}
	size, domain = clampPageSize(maxPageSize + 100)
	if domain != nil || size != maxPageSize {
		t.Fatalf("expected clamped page size, got %d", size)
	}
	_, domain = clampPageSize(-1)
	if domain == nil {
		t.Fatal("expected error for negative page size")
	}
}

func TestObservabilityServiceReturnsSkillDetailFromFixture(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("..", "..", "..", "loomspan-console-fixtures", "application-rest", "skill-detail.json"))
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeGetClient{body: fixture, instanceID: "11111111-1111-4111-8111-111111111111"}
	targetContext := setupTargetWithCredential(t, client)
	service := New()
	scope, domain := targetContext.Capture()
	if domain != nil {
		t.Fatal(domain)
	}
	detail, domain := service.GetSkill(context.Background(), scope, "CheckDns")
	if domain != nil {
		t.Fatal(domain)
	}
	if detail.RegisteredName != "CheckDns" || detail.Yaml == "" {
		t.Fatalf("unexpected detail: %#v", detail)
	}
}

func TestObservabilityServiceReturnsActiveExecutionDetailFromFixture(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("..", "..", "..", "loomspan-console-fixtures", "application-rest", "active-execution-detail.json"))
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeGetClient{body: fixture, instanceID: "11111111-1111-4111-8111-111111111111"}
	targetContext := setupTargetWithCredential(t, client)
	service := New()
	scope, domain := targetContext.Capture()
	if domain != nil {
		t.Fatal(domain)
	}
	execution, domain := service.GetActiveExecution(context.Background(), scope, "session-1")
	if domain != nil {
		t.Fatal(domain)
	}
	if execution.SessionID != "session-1" || execution.EntrySkill != "CheckDns" {
		t.Fatalf("unexpected execution: %#v", execution)
	}
}

func TestObservabilityServiceReturnsTraceDetailFromFixture(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("..", "..", "..", "loomspan-console-fixtures", "application-rest", "trace-detail.json"))
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeGetClient{body: fixture, instanceID: "11111111-1111-4111-8111-111111111111"}
	targetContext := setupTargetWithCredential(t, client)
	service := New()
	scope, domain := targetContext.Capture()
	if domain != nil {
		t.Fatal(domain)
	}
	trace, domain := service.GetTrace(context.Background(), scope, "trace-1")
	if domain != nil {
		t.Fatal(domain)
	}
	if trace.TraceID != "trace-1" || trace.Outcome != "SUCCEEDED" {
		t.Fatalf("unexpected trace: %#v", trace)
	}
}

func TestObservabilityServiceAgainstHTTPTestServer(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("..", "..", "..", "loomspan-console-fixtures", "application-rest", "skills-page.json"))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		response.Header().Set(applicationclient.InstanceIDHeader, "11111111-1111-4111-8111-111111111111")
		if strings.HasSuffix(request.URL.Path, "/instance") {
			_, _ = response.Write([]byte(`{"instanceId":"11111111-1111-4111-8111-111111111111","consoleCompatibilityVersion":"0.1.0-SNAPSHOT","observedAt":"2026-07-25T12:00:00Z","liveMonitoringAvailable":true,"registeredSkillCount":1,"activeExecutionCount":0,"catalogedTraceCount":0,"tracePersistencePolicy":"PERSISTENT","completionGraceTtl":"PT2M","traceCatalogMetadataTtl":"PT168H"}`))
			return
		}
		_, _ = response.Write(fixture)
	}))
	defer server.Close()
	policy := applicationclient.NetworkPolicy{
		ConnectTimeout: testTimeout, ResponseHeaderTimeout: testTimeout, RequestTimeout: testTimeout,
	}
	targetContext, _ := target.New(func(addr applicationclient.Address) (target.ProbeClient, error) {
		return applicationclient.New(addr, policy, "0.1.0-SNAPSHOT")
	}, nil, nil)
	if err := targetContext.Select(server.URL); err != nil {
		t.Fatal(err)
	}
	if _, domain := targetContext.SupplyCredential(context.Background(), testCredential()); domain != nil {
		t.Fatal(domain)
	}
	service := New()
	scope, domain := targetContext.Capture()
	if domain != nil {
		t.Fatal(domain)
	}
	page, domain := service.ListSkills(context.Background(), scope, ListRequest{})
	if domain != nil {
		t.Fatal(domain)
	}
	if len(page.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(page.Items))
	}
}

func TestObservabilityServiceMapsStaleCursorFromUpstream(t *testing.T) {
	staleFixture := []byte(`{"status":410,"code":"STALE_CURSOR","message":"The continuation belongs to another application instance"}`)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		response.Header().Set(applicationclient.InstanceIDHeader, "11111111-1111-4111-8111-111111111111")
		if strings.HasSuffix(request.URL.Path, "/instance") {
			_, _ = response.Write([]byte(`{"instanceId":"11111111-1111-4111-8111-111111111111","consoleCompatibilityVersion":"0.1.0-SNAPSHOT","observedAt":"2026-07-25T12:00:00Z","liveMonitoringAvailable":true,"registeredSkillCount":1,"activeExecutionCount":0,"catalogedTraceCount":0,"tracePersistencePolicy":"PERSISTENT","completionGraceTtl":"PT2M","traceCatalogMetadataTtl":"PT168H"}`))
			return
		}
		response.WriteHeader(410)
		_, _ = response.Write(staleFixture)
	}))
	defer server.Close()
	policy := applicationclient.NetworkPolicy{
		ConnectTimeout: testTimeout, ResponseHeaderTimeout: testTimeout, RequestTimeout: testTimeout,
	}
	targetContext, _ := target.New(func(addr applicationclient.Address) (target.ProbeClient, error) {
		return applicationclient.New(addr, policy, "0.1.0-SNAPSHOT")
	}, nil, nil)
	if err := targetContext.Select(server.URL); err != nil {
		t.Fatal(err)
	}
	if _, domain := targetContext.SupplyCredential(context.Background(), testCredential()); domain != nil {
		t.Fatal(domain)
	}
	service := New()
	scope, domain := targetContext.Capture()
	if domain != nil {
		t.Fatal(domain)
	}
	_, domain = service.ListSkills(context.Background(), scope, ListRequest{})
	if domain == nil {
		t.Fatal("expected error for stale cursor")
	}
	if domain.Code != consolecore.CodeStaleCursor {
		t.Fatalf("expected STALE_CURSOR, got %s", domain.Code)
	}
}

func TestObservabilityServiceMapsNotFoundFromUpstream(t *testing.T) {
	notFoundFixture := []byte(`{"status":404,"code":"NOT_FOUND","message":"The requested observability resource was not found"}`)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		response.Header().Set(applicationclient.InstanceIDHeader, "11111111-1111-4111-8111-111111111111")
		if strings.HasSuffix(request.URL.Path, "/instance") {
			_, _ = response.Write([]byte(`{"instanceId":"11111111-1111-4111-8111-111111111111","consoleCompatibilityVersion":"0.1.0-SNAPSHOT","observedAt":"2026-07-25T12:00:00Z","liveMonitoringAvailable":true,"registeredSkillCount":1,"activeExecutionCount":0,"catalogedTraceCount":0,"tracePersistencePolicy":"PERSISTENT","completionGraceTtl":"PT2M","traceCatalogMetadataTtl":"PT168H"}`))
			return
		}
		response.WriteHeader(404)
		_, _ = response.Write(notFoundFixture)
	}))
	defer server.Close()
	policy := applicationclient.NetworkPolicy{
		ConnectTimeout: testTimeout, ResponseHeaderTimeout: testTimeout, RequestTimeout: testTimeout,
	}
	targetContext, _ := target.New(func(addr applicationclient.Address) (target.ProbeClient, error) {
		return applicationclient.New(addr, policy, "0.1.0-SNAPSHOT")
	}, nil, nil)
	if err := targetContext.Select(server.URL); err != nil {
		t.Fatal(err)
	}
	if _, domain := targetContext.SupplyCredential(context.Background(), testCredential()); domain != nil {
		t.Fatal(domain)
	}
	service := New()
	scope, domain := targetContext.Capture()
	if domain != nil {
		t.Fatal(domain)
	}
	_, domain = service.GetSkill(context.Background(), scope, "missing-skill")
	if domain == nil {
		t.Fatal("expected error for not found")
	}
	if domain.Code != consolecore.CodeNotFound {
		t.Fatalf("expected NOT_FOUND, got %s", domain.Code)
	}
}

func TestObservabilityServiceMapsLiveMonitoringUnavailableFromUpstream(t *testing.T) {
	liveMonitoringFixture := []byte(`{"status":503,"code":"LIVE_MONITORING_UNAVAILABLE","message":"Live execution monitoring is unavailable"}`)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		response.Header().Set(applicationclient.InstanceIDHeader, "11111111-1111-4111-8111-111111111111")
		if strings.HasSuffix(request.URL.Path, "/instance") {
			_, _ = response.Write([]byte(`{"instanceId":"11111111-1111-4111-8111-111111111111","consoleCompatibilityVersion":"0.1.0-SNAPSHOT","observedAt":"2026-07-25T12:00:00Z","liveMonitoringAvailable":true,"registeredSkillCount":1,"activeExecutionCount":0,"catalogedTraceCount":0,"tracePersistencePolicy":"PERSISTENT","completionGraceTtl":"PT2M","traceCatalogMetadataTtl":"PT168H"}`))
			return
		}
		response.WriteHeader(503)
		_, _ = response.Write(liveMonitoringFixture)
	}))
	defer server.Close()
	policy := applicationclient.NetworkPolicy{
		ConnectTimeout: testTimeout, ResponseHeaderTimeout: testTimeout, RequestTimeout: testTimeout,
	}
	targetContext, _ := target.New(func(addr applicationclient.Address) (target.ProbeClient, error) {
		return applicationclient.New(addr, policy, "0.1.0-SNAPSHOT")
	}, nil, nil)
	if err := targetContext.Select(server.URL); err != nil {
		t.Fatal(err)
	}
	if _, domain := targetContext.SupplyCredential(context.Background(), testCredential()); domain != nil {
		t.Fatal(domain)
	}
	service := New()
	scope, domain := targetContext.Capture()
	if domain != nil {
		t.Fatal(domain)
	}
	_, domain = service.GetActiveExecution(context.Background(), scope, "session-1")
	if domain == nil {
		t.Fatal("expected error for live monitoring unavailable")
	}
	if domain.Code != consolecore.CodeLiveMonitoringUnavailable {
		t.Fatalf("expected LIVE_MONITORING_UNAVAILABLE, got %s", domain.Code)
	}
}
