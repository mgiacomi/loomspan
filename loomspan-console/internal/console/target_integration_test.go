package console

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/applicationclient"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/evidence"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/target"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/traceanalysis"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/workspace"
)

func TestApplicationCredentialNeverAppearsOutsideSelectedRequestHeader(t *testing.T) {
	secret := "LOOMSPAN_" + "TEST_APPLICATION_KEY_DO_NOT_LEAK_7f63"
	var received []string
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		received = append(received, request.Header.Values(applicationclient.APIKeyHeader)...)
		response.Header().Set(applicationclient.InstanceIDHeader, "11111111-1111-4111-8111-111111111111")
		_, _ = response.Write([]byte(`{"instanceId":"11111111-1111-4111-8111-111111111111","consoleCompatibilityVersion":"0.1.0-SNAPSHOT","observedAt":"2026-07-25T12:00:00Z","liveMonitoringAvailable":true}`))
	}))
	defer server.Close()
	policy := applicationclient.NetworkPolicy{
		ConnectTimeout: time.Second, ResponseHeaderTimeout: time.Second, RequestTimeout: time.Second,
	}
	targetContext, err := target.New(func(address applicationclient.Address) (target.ProbeClient, error) {
		return applicationclient.New(address, policy, "0.1.0-SNAPSHOT")
	}, nil, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if err := targetContext.Select(server.URL); err != nil {
		t.Fatal(err)
	}
	snapshot, domain := targetContext.SupplyCredential(context.Background(), []byte(secret))
	if domain != nil {
		t.Fatal(domain)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(encoded) + fmt.Sprintf("%#v", targetContext)
	if strings.Contains(rendered, secret) {
		t.Fatal("credential escaped into snapshot or formatted target context")
	}
	if len(received) != 1 || received[0] != secret {
		t.Fatal("credential was not confined to exactly one selected request header")
	}
	targetContext.Close()
}

// scopeOwnerRecorder records ActivateActivity and InvalidateTargetScope calls
// for verifying that the artifact service receives scope lifecycle callbacks.
type scopeOwnerRecorder struct {
	mu           sync.Mutex
	activated    []target.ScopeID
	invalidated  []target.ScopeID
	activateFn   func(target.Scope)
	invalidateFn func(target.ScopeID, context.Context)
}

func (r *scopeOwnerRecorder) ActivateActivity(scope target.Scope) {
	r.mu.Lock()
	r.activated = append(r.activated, scope.ID)
	fn := r.activateFn
	r.mu.Unlock()
	if fn != nil {
		fn(scope)
	}
}

func (r *scopeOwnerRecorder) InvalidateTargetScope(previous target.ScopeID, cancelled context.Context) {
	r.mu.Lock()
	r.invalidated = append(r.invalidated, previous)
	fn := r.invalidateFn
	r.mu.Unlock()
	if fn != nil {
		fn(previous, cancelled)
	}
}

// TestTargetContextNotifiesScopeOwnersOnActivationAndRotation verifies the
// target-context owner-notification contract that the artifact service relies
// on: registered owners receive ActivateActivity on selection and
// InvalidateTargetScope on rotation. It uses a recorder fake rather than the
// real artifact service so the contract is exercised in isolation.
func TestTargetContextNotifiesScopeOwnersOnActivationAndRotation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set(applicationclient.InstanceIDHeader, "11111111-1111-4111-8111-111111111111")
		_, _ = response.Write([]byte(`{"instanceId":"11111111-1111-4111-8111-111111111111","consoleCompatibilityVersion":"0.1.0-SNAPSHOT","observedAt":"2026-07-25T12:00:00Z","liveMonitoringAvailable":true}`))
	}))
	defer server.Close()
	policy := applicationclient.NetworkPolicy{
		ConnectTimeout: time.Second, ResponseHeaderTimeout: time.Second, RequestTimeout: time.Second,
	}
	targetContext, err := target.New(func(address applicationclient.Address) (target.ProbeClient, error) {
		return applicationclient.New(address, policy, "0.1.0-SNAPSHOT")
	}, nil, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	defer targetContext.Close()

	recorder := &scopeOwnerRecorder{}
	if err := targetContext.RegisterOwner("artifacts-test", recorder); err != nil {
		t.Fatal(err)
	}

	if err := targetContext.Select(server.URL); err != nil {
		t.Fatal(err)
	}
	if _, domain := targetContext.SupplyCredential(context.Background(), []byte(strings.Repeat("k", 32))); domain != nil {
		t.Fatal(domain)
	}

	recorder.mu.Lock()
	if len(recorder.activated) != 1 {
		t.Fatalf("expected 1 ActivateActivity call, got %d", len(recorder.activated))
	}
	recorder.mu.Unlock()

	// Rotate by re-selecting with a different instance.
	server2 := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set(applicationclient.InstanceIDHeader, "22222222-2222-4222-8222-222222222222")
		_, _ = response.Write([]byte(`{"instanceId":"22222222-2222-4222-8222-222222222222","consoleCompatibilityVersion":"0.1.0-SNAPSHOT","observedAt":"2026-07-25T12:00:00Z","liveMonitoringAvailable":true}`))
	}))
	defer server2.Close()
	if err := targetContext.Select(server2.URL); err != nil {
		t.Fatal(err)
	}
	if _, domain := targetContext.SupplyCredential(context.Background(), []byte(strings.Repeat("k", 32))); domain != nil {
		t.Fatal(domain)
	}

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.invalidated) == 0 {
		t.Fatal("expected at least 1 InvalidateTargetScope call after rotation")
	}
}

// TestArtifactServiceAcquiresAndUsesThroughTargetScope verifies that the
// artifact service can acquire and use an artifact through a real target scope
// with a real workspace.
func TestArtifactServiceAcquiresAndUsesThroughTargetScope(t *testing.T) {
	data := validNDJSONArtifact()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set(applicationclient.InstanceIDHeader, "11111111-1111-4111-8111-111111111111")
		if strings.HasSuffix(request.URL.Path, "/instance") {
			_, _ = response.Write([]byte(`{"instanceId":"11111111-1111-4111-8111-111111111111","consoleCompatibilityVersion":"0.1.0-SNAPSHOT","observedAt":"2026-07-25T12:00:00Z","liveMonitoringAvailable":true,"registeredSkillCount":1,"activeExecutionCount":0,"catalogedTraceCount":0,"tracePersistencePolicy":"PERSISTENT","completionGraceTtl":"PT2M","traceCatalogMetadataTtl":"PT168H"}`))
			return
		}
		if strings.HasSuffix(request.URL.Path, "/traces/trace-1") {
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{"targetScopeId":"scope-1","traceId":"trace-1","sessionId":"session-1","entrySkill":"CheckDns","outcome":"SUCCEEDED","finalizedAt":"2026-07-24T12:00:00Z","sizeBytes":` + fmt.Sprintf("%d", len(data)) + `,"persistencePolicy":"ALWAYS","applicationTraceExpiresAt":"2026-07-26T12:00:00Z"}`))
			return
		}
		if strings.HasSuffix(request.URL.Path, "/traces/trace-1/artifact") {
			response.Header().Set("Content-Type", applicationclient.ArtifactMediaType)
			response.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
			_, _ = response.Write(data)
			return
		}
		response.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	policy := applicationclient.NetworkPolicy{
		ConnectTimeout: time.Second, ResponseHeaderTimeout: time.Second, RequestTimeout: 10 * time.Second,
	}
	targetContext, err := target.New(func(address applicationclient.Address) (target.ProbeClient, error) {
		return applicationclient.New(address, policy, "0.1.0-SNAPSHOT")
	}, nil, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	defer targetContext.Close()

	ws, err := workspace.Open(filepath.Join(t.TempDir(), "work"))
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()

	// Create the artifact service with real trace loading and stream opening.
	artifactSvc, err := artifact.New(artifact.Config{
		MaxBytes:  1 << 20,
		IdleTTL:   time.Hour,
		Unlimited: false,
	}, artifact.Dependencies{
		Lifetime:  context.Background(),
		Workspace: ws,
		TraceLoader: func(ctx context.Context, scope target.Scope, traceID string) (artifact.TraceMetadata, *consolecore.Error) {
			endpoint := scope.Target.TraceEndpoint(traceID)
			body, domain := scope.Upstream(ctx, endpoint, 1<<20)
			if domain != nil {
				return artifact.TraceMetadata{}, domain
			}
			trace, perr := parseTraceJSON(body)
			if perr != nil {
				return artifact.TraceMetadata{}, consolecore.NewError(consolecore.CodeConsoleError, "Failed to parse trace metadata.", string(scope.ID), consolecore.Details{}, perr)
			}
			return artifact.TraceMetadata{
				TraceID:                   trace.TraceID,
				SessionID:                 trace.SessionID,
				Outcome:                   trace.Outcome,
				FinalizedAt:               trace.FinalizedAt,
				SizeBytes:                 trace.SizeBytes,
				PersistencePolicy:         trace.PersistencePolicy,
				ApplicationTraceExpiresAt: trace.ApplicationTraceExpiresAt,
			}, nil
		},
		StreamOpener: func(ctx context.Context, scope target.Scope, traceID string) (*applicationclient.ArtifactStream, *consolecore.Error) {
			return scope.OpenArtifact(ctx, traceID)
		},
		Processor: traceanalysis.New(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer artifactSvc.Close()

	if err := targetContext.RegisterOwner("artifacts", artifactSvc); err != nil {
		t.Fatal(err)
	}
	if err := targetContext.Select(server.URL); err != nil {
		t.Fatal(err)
	}
	if _, domain := targetContext.SupplyCredential(context.Background(), []byte(strings.Repeat("k", 32))); domain != nil {
		t.Fatal(domain)
	}

	scope, domain := targetContext.Capture()
	if domain != nil {
		t.Fatal(domain)
	}

	artifactResult, domain := artifactSvc.Acquire(context.Background(), scope, "trace-1")
	if domain != nil {
		t.Fatalf("Acquire failed: %v", domain)
	}
	if artifactResult.Handle == "" {
		t.Fatal("expected non-empty handle")
	}

	// Use the artifact.
	lease, domain := artifactSvc.Use(evidence.ForTarget(scope.ID), artifactResult.Handle)
	if domain != nil {
		t.Fatalf("Use failed: %v", domain)
	}
	reader, err := lease.OpenComponent(artifact.ComponentRawArtifact)
	if err != nil {
		t.Fatalf("OpenComponent failed: %v", err)
	}
	got := make([]byte, len(data))
	if _, err := io.ReadFull(reader, got); err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	reader.Close()
	lease.Close(true)

	if string(got) != string(data) {
		t.Fatalf("expected %q, got %q", data, got)
	}
}

// traceJSON is a minimal DTO for parsing trace metadata in integration tests.
type traceJSON struct {
	TraceID                   string    `json:"traceId"`
	SessionID                 string    `json:"sessionId"`
	EntrySkill                string    `json:"entrySkill"`
	Outcome                   string    `json:"outcome"`
	FinalizedAt               time.Time `json:"finalizedAt"`
	SizeBytes                 int64     `json:"sizeBytes"`
	PersistencePolicy         string    `json:"persistencePolicy"`
	ApplicationTraceExpiresAt time.Time `json:"applicationTraceExpiresAt"`
}

func parseTraceJSON(data []byte) (traceJSON, error) {
	var trace traceJSON
	if err := json.Unmarshal(data, &trace); err != nil {
		return traceJSON{}, err
	}
	return trace, nil
}

// validNDJSONArtifact returns a minimal valid NDJSON artifact body that the real
// traceanalysis.Processor accepts: a TRACE_STARTED record followed by a
// TRACE_COMPLETED record with matching identity and a zero terminal usage
// snapshot. Used by target-scope integration tests that wire the production
// processor.
func validNDJSONArtifact() []byte {
	return []byte(`{"traceId":"trace-1","sessionId":"session-1","sequence":1,"timestamp":1784894400.000000000,"recordType":"TRACE_STARTED","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"th","metadata":{"consoleCompatibilityVersion":"development"},"data":null}` + "\n" +
		`{"traceId":"trace-1","sessionId":"session-1","sequence":2,"timestamp":1784894400.000000000,"recordType":"TRACE_COMPLETED","frameId":null,"parentFrameId":null,"frameType":null,"route":null,"threadName":"th","metadata":{"outcome":"SUCCEEDED","sessionUsageSnapshot":{"promptUnits":0,"completionUnits":0,"totalUnits":0},"errored":false,"persistencePolicy":"ALWAYS"},"data":null}` + "\n")
}

// TestArtifactScopeRotationDuringMetadataFetchReturnsTargetChanged proves
// that scope rotation during the trace metadata load surfaces TARGET_CHANGED
// rather than installing a stale-scope artifact (PR12-R03, PR12-R09).
func TestArtifactScopeRotationDuringMetadataFetchReturnsTargetChanged(t *testing.T) {
	data := validNDJSONArtifact()
	metadataReleased := make(chan struct{})
	traceLoaded := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set(applicationclient.InstanceIDHeader, "11111111-1111-4111-8111-111111111111")
		if strings.HasSuffix(request.URL.Path, "/instance") {
			_, _ = response.Write([]byte(`{"instanceId":"11111111-1111-4111-8111-111111111111","consoleCompatibilityVersion":"0.1.0-SNAPSHOT","observedAt":"2026-07-25T12:00:00Z","liveMonitoringAvailable":true,"registeredSkillCount":1,"activeExecutionCount":0,"catalogedTraceCount":0,"tracePersistencePolicy":"PERSISTENT","completionGraceTtl":"PT2M","traceCatalogMetadataTtl":"PT168H"}`))
			return
		}
		if strings.HasSuffix(request.URL.Path, "/traces/trace-1") && !strings.HasSuffix(request.URL.Path, "/artifact") {
			// Hold the metadata response until the test rotates the scope.
			close(traceLoaded)
			<-metadataReleased
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{"targetScopeId":"scope-1","traceId":"trace-1","sessionId":"session-1","entrySkill":"CheckDns","outcome":"SUCCEEDED","finalizedAt":"2026-07-24T12:00:00Z","sizeBytes":` + fmt.Sprintf("%d", len(data)) + `,"persistencePolicy":"ALWAYS","applicationTraceExpiresAt":"2026-07-26T12:00:00Z"}`))
			return
		}
		response.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	policy := applicationclient.NetworkPolicy{
		ConnectTimeout: time.Second, ResponseHeaderTimeout: time.Second, RequestTimeout: 10 * time.Second,
	}
	targetContext, err := target.New(func(address applicationclient.Address) (target.ProbeClient, error) {
		return applicationclient.New(address, policy, "0.1.0-SNAPSHOT")
	}, nil, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	defer targetContext.Close()
	ws, err := workspace.Open(filepath.Join(t.TempDir(), "work"))
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	artifactSvc, err := artifact.New(artifact.Config{
		MaxBytes: 1 << 20, IdleTTL: time.Hour,
	}, artifact.Dependencies{
		Lifetime:  context.Background(),
		Workspace: ws,
		TraceLoader: func(ctx context.Context, scope target.Scope, traceID string) (artifact.TraceMetadata, *consolecore.Error) {
			endpoint := scope.Target.TraceEndpoint(traceID)
			body, domain := scope.Upstream(ctx, endpoint, 1<<20)
			if domain != nil {
				return artifact.TraceMetadata{}, domain
			}
			trace, perr := parseTraceJSON(body)
			if perr != nil {
				return artifact.TraceMetadata{}, consolecore.NewError(consolecore.CodeConsoleError, "Failed to parse trace metadata.", string(scope.ID), consolecore.Details{}, perr)
			}
			return artifact.TraceMetadata{
				TraceID:                   trace.TraceID,
				SessionID:                 trace.SessionID,
				Outcome:                   trace.Outcome,
				FinalizedAt:               trace.FinalizedAt,
				SizeBytes:                 trace.SizeBytes,
				PersistencePolicy:         trace.PersistencePolicy,
				ApplicationTraceExpiresAt: trace.ApplicationTraceExpiresAt,
			}, nil
		},
		StreamOpener: func(ctx context.Context, scope target.Scope, traceID string) (*applicationclient.ArtifactStream, *consolecore.Error) {
			return scope.OpenArtifact(ctx, traceID)
		},
		Processor: traceanalysis.New(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer artifactSvc.Close()
	if err := targetContext.RegisterOwner("artifacts", artifactSvc); err != nil {
		t.Fatal(err)
	}
	if err := targetContext.Select(server.URL); err != nil {
		t.Fatal(err)
	}
	if _, domain := targetContext.SupplyCredential(context.Background(), []byte(strings.Repeat("k", 32))); domain != nil {
		t.Fatal(domain)
	}
	scope, domain := targetContext.Capture()
	if domain != nil {
		t.Fatal(domain)
	}

	// Start an acquisition that will block on the metadata fetch.
	acquireDone := make(chan error, 1)
	go func() {
		_, domain := artifactSvc.Acquire(context.Background(), scope, "trace-1")
		if domain != nil {
			acquireDone <- errors.New(domain.Message)
			return
		}
		acquireDone <- nil
	}()

	// Wait for the metadata request to arrive, then rotate the scope by
	// selecting a server with a different instance ID.
	<-traceLoaded
	server2 := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set(applicationclient.InstanceIDHeader, "22222222-2222-4222-8222-222222222222")
		_, _ = response.Write([]byte(`{"instanceId":"22222222-2222-4222-8222-222222222222","consoleCompatibilityVersion":"0.1.0-SNAPSHOT","observedAt":"2026-07-25T12:00:00Z","liveMonitoringAvailable":true,"registeredSkillCount":1,"activeExecutionCount":0,"catalogedTraceCount":0,"tracePersistencePolicy":"PERSISTENT","completionGraceTtl":"PT2M","traceCatalogMetadataTtl":"PT168H"}`))
	}))
	defer server2.Close()
	if err := targetContext.Select(server2.URL); err != nil {
		t.Fatal(err)
	}
	if _, domain := targetContext.SupplyCredential(context.Background(), []byte(strings.Repeat("k", 32))); domain != nil {
		t.Fatal(domain)
	}

	// Release the metadata response. The acquisition must observe scope
	// cancellation and return a TARGET_CHANGED error, not install a stale copy.
	close(metadataReleased)
	select {
	case err := <-acquireDone:
		if err == nil {
			t.Fatal("expected acquisition to fail after scope rotation")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("acquisition did not return after scope rotation")
	}

	// The old scope's storage must report TARGET_CHANGED.
	_, domain = artifactSvc.StorageSnapshot()
	if domain != nil {
		t.Fatalf("global storage snapshot failed after rotation: %v", domain)
	}
}

// TestArtifactScopeRotationDuringLeaseUseReturnsTargetChanged proves that a
// lease issued against a scope becomes invalid after scope rotation: Use on the
// stale scope returns TARGET_CHANGED (PR12-R09).
func TestArtifactScopeRotationDuringLeaseUseReturnsTargetChanged(t *testing.T) {
	data := validNDJSONArtifact()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set(applicationclient.InstanceIDHeader, "11111111-1111-4111-8111-111111111111")
		if strings.HasSuffix(request.URL.Path, "/instance") {
			_, _ = response.Write([]byte(`{"instanceId":"11111111-1111-4111-8111-111111111111","consoleCompatibilityVersion":"0.1.0-SNAPSHOT","observedAt":"2026-07-25T12:00:00Z","liveMonitoringAvailable":true,"registeredSkillCount":1,"activeExecutionCount":0,"catalogedTraceCount":0,"tracePersistencePolicy":"PERSISTENT","completionGraceTtl":"PT2M","traceCatalogMetadataTtl":"PT168H"}`))
			return
		}
		if strings.HasSuffix(request.URL.Path, "/traces/trace-1") && !strings.HasSuffix(request.URL.Path, "/artifact") {
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{"targetScopeId":"scope-1","traceId":"trace-1","sessionId":"session-1","entrySkill":"CheckDns","outcome":"SUCCEEDED","finalizedAt":"2026-07-24T12:00:00Z","sizeBytes":` + fmt.Sprintf("%d", len(data)) + `,"persistencePolicy":"ALWAYS","applicationTraceExpiresAt":"2026-07-26T12:00:00Z"}`))
			return
		}
		if strings.HasSuffix(request.URL.Path, "/traces/trace-1/artifact") {
			response.Header().Set("Content-Type", applicationclient.ArtifactMediaType)
			response.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
			_, _ = response.Write(data)
			return
		}
		response.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	policy := applicationclient.NetworkPolicy{
		ConnectTimeout: time.Second, ResponseHeaderTimeout: time.Second, RequestTimeout: 10 * time.Second,
	}
	targetContext, err := target.New(func(address applicationclient.Address) (target.ProbeClient, error) {
		return applicationclient.New(address, policy, "0.1.0-SNAPSHOT")
	}, nil, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	defer targetContext.Close()
	ws, err := workspace.Open(filepath.Join(t.TempDir(), "work"))
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	artifactSvc, err := artifact.New(artifact.Config{
		MaxBytes: 1 << 20, IdleTTL: time.Hour,
	}, artifact.Dependencies{
		Lifetime:  context.Background(),
		Workspace: ws,
		TraceLoader: func(ctx context.Context, scope target.Scope, traceID string) (artifact.TraceMetadata, *consolecore.Error) {
			endpoint := scope.Target.TraceEndpoint(traceID)
			body, domain := scope.Upstream(ctx, endpoint, 1<<20)
			if domain != nil {
				return artifact.TraceMetadata{}, domain
			}
			trace, perr := parseTraceJSON(body)
			if perr != nil {
				return artifact.TraceMetadata{}, consolecore.NewError(consolecore.CodeConsoleError, "Failed to parse trace metadata.", string(scope.ID), consolecore.Details{}, perr)
			}
			return artifact.TraceMetadata{
				TraceID:                   trace.TraceID,
				SessionID:                 trace.SessionID,
				Outcome:                   trace.Outcome,
				FinalizedAt:               trace.FinalizedAt,
				SizeBytes:                 trace.SizeBytes,
				PersistencePolicy:         trace.PersistencePolicy,
				ApplicationTraceExpiresAt: trace.ApplicationTraceExpiresAt,
			}, nil
		},
		StreamOpener: func(ctx context.Context, scope target.Scope, traceID string) (*applicationclient.ArtifactStream, *consolecore.Error) {
			return scope.OpenArtifact(ctx, traceID)
		},
		Processor: traceanalysis.New(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer artifactSvc.Close()
	if err := targetContext.RegisterOwner("artifacts", artifactSvc); err != nil {
		t.Fatal(err)
	}
	if err := targetContext.Select(server.URL); err != nil {
		t.Fatal(err)
	}
	if _, domain := targetContext.SupplyCredential(context.Background(), []byte(strings.Repeat("k", 32))); domain != nil {
		t.Fatal(domain)
	}
	scope, domain := targetContext.Capture()
	if domain != nil {
		t.Fatal(domain)
	}

	acquired, domain := artifactSvc.Acquire(context.Background(), scope, "trace-1")
	if domain != nil {
		t.Fatalf("Acquire failed: %v", domain)
	}
	staleScopeID := scope.ID

	// Rotate the scope.
	server2 := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set(applicationclient.InstanceIDHeader, "22222222-2222-4222-8222-222222222222")
		_, _ = response.Write([]byte(`{"instanceId":"22222222-2222-4222-8222-222222222222","consoleCompatibilityVersion":"0.1.0-SNAPSHOT","observedAt":"2026-07-25T12:00:00Z","liveMonitoringAvailable":true,"registeredSkillCount":1,"activeExecutionCount":0,"catalogedTraceCount":0,"tracePersistencePolicy":"PERSISTENT","completionGraceTtl":"PT2M","traceCatalogMetadataTtl":"PT168H"}`))
	}))
	defer server2.Close()
	if err := targetContext.Select(server2.URL); err != nil {
		t.Fatal(err)
	}
	if _, domain := targetContext.SupplyCredential(context.Background(), []byte(strings.Repeat("k", 32))); domain != nil {
		t.Fatal(domain)
	}

	// Use with the stale scope must return TARGET_CHANGED.
	_, domain = artifactSvc.Use(evidence.ForTarget(staleScopeID), acquired.Handle)
	if domain == nil || domain.Code != consolecore.CodeTargetChanged {
		t.Fatalf("expected TARGET_CHANGED for stale scope Use, got %v", domain)
	}
}

// TestArtifactRawPassThroughDuringScopeRotationReturnsTargetChanged proves
// that a raw pass-through OpenArtifact against a rotated scope returns
// TARGET_CHANGED rather than streaming stale bytes (PR12-R09, PR12-R12).
func TestArtifactRawPassThroughDuringScopeRotationReturnsTargetChanged(t *testing.T) {
	data := []byte("raw-passthrough-scope-rotation")
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set(applicationclient.InstanceIDHeader, "11111111-1111-4111-8111-111111111111")
		if strings.HasSuffix(request.URL.Path, "/instance") {
			_, _ = response.Write([]byte(`{"instanceId":"11111111-1111-4111-8111-111111111111","consoleCompatibilityVersion":"0.1.0-SNAPSHOT","observedAt":"2026-07-25T12:00:00Z","liveMonitoringAvailable":true,"registeredSkillCount":1,"activeExecutionCount":0,"catalogedTraceCount":0,"tracePersistencePolicy":"PERSISTENT","completionGraceTtl":"PT2M","traceCatalogMetadataTtl":"PT168H"}`))
			return
		}
		if strings.HasSuffix(request.URL.Path, "/traces/trace-1/artifact") {
			response.Header().Set("Content-Type", applicationclient.ArtifactMediaType)
			response.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
			_, _ = response.Write(data)
			return
		}
		response.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	policy := applicationclient.NetworkPolicy{
		ConnectTimeout: time.Second, ResponseHeaderTimeout: time.Second, RequestTimeout: 10 * time.Second,
	}
	targetContext, err := target.New(func(address applicationclient.Address) (target.ProbeClient, error) {
		return applicationclient.New(address, policy, "0.1.0-SNAPSHOT")
	}, nil, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	defer targetContext.Close()
	if err := targetContext.Select(server.URL); err != nil {
		t.Fatal(err)
	}
	if _, domain := targetContext.SupplyCredential(context.Background(), []byte(strings.Repeat("k", 32))); domain != nil {
		t.Fatal(domain)
	}
	scope, domain := targetContext.Capture()
	if domain != nil {
		t.Fatal(domain)
	}

	// Rotate the scope before the raw pass-through.
	server2 := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set(applicationclient.InstanceIDHeader, "22222222-2222-4222-8222-222222222222")
		_, _ = response.Write([]byte(`{"instanceId":"22222222-2222-4222-8222-222222222222","consoleCompatibilityVersion":"0.1.0-SNAPSHOT","observedAt":"2026-07-25T12:00:00Z","liveMonitoringAvailable":true,"registeredSkillCount":1,"activeExecutionCount":0,"catalogedTraceCount":0,"tracePersistencePolicy":"PERSISTENT","completionGraceTtl":"PT2M","traceCatalogMetadataTtl":"PT168H"}`))
	}))
	defer server2.Close()
	if err := targetContext.Select(server2.URL); err != nil {
		t.Fatal(err)
	}
	if _, domain := targetContext.SupplyCredential(context.Background(), []byte(strings.Repeat("k", 32))); domain != nil {
		t.Fatal(domain)
	}

	// OpenArtifact on the stale scope must fail with TARGET_CHANGED.
	_, domain = scope.OpenArtifact(context.Background(), "trace-1")
	if domain == nil || domain.Code != consolecore.CodeTargetChanged {
		t.Fatalf("expected TARGET_CHANGED for raw pass-through on stale scope, got %v", domain)
	}
}
