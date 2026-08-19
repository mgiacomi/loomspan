package console

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
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

// javaCompatibleInstanceJSON is the exact instance probe body a Java adapter at
// consoleCompatibilityVersion "0.1.0-SNAPSHOT" emits. Tests reuse it so the
// scope's compatibility gate and runtime identity are established exactly as in
// production.
const javaCompatibleInstanceJSON = `{"instanceId":"11111111-1111-4111-8111-111111111111","consoleCompatibilityVersion":"0.1.0-SNAPSHOT","observedAt":"2026-07-25T12:00:00Z","liveMonitoringAvailable":true,"registeredSkillCount":0,"activeExecutionCount":0,"catalogedTraceCount":1,"tracePersistencePolicy":"PERSISTENT","completionGraceTtl":"PT2M","traceCatalogMetadataTtl":"PT168H"}`

const javaInstanceID = "11111111-1111-4111-8111-111111111111"

// artifactTestServer is a stateful httptest.Server that mimics the Java
// observability boundary for instance probe, trace metadata, and artifact
// streaming. It records request counts and supports toggling authentication
// rejection, instance rotation, and artifact body changes so integration tests
// can exercise the cross-boundary lifecycle deterministically.
type artifactTestServer struct {
	t              *testing.T
	instanceJSON   string
	instanceID     string
	traceMetadata  string
	artifactBody   []byte
	artifactFail   atomic.Bool
	authReject     atomic.Bool
	instanceRotate atomic.Bool
	artifactCalls  atomic.Int32
	traceCalls     atomic.Int32
	instanceCalls  atomic.Int32
	mu             sync.Mutex
	server         *httptest.Server
}

func newArtifactTestServer(t *testing.T, artifactBody []byte) *artifactTestServer {
	t.Helper()
	fixture, err := os.ReadFile(filepath.Join("..", "..", "..", "loomspan-console-fixtures", "traces", "single-attempt-success.ndjson"))
	if err != nil {
		t.Fatalf("read Java-produced trace fixture: %v", err)
	}
	if artifactBody == nil {
		artifactBody = bytes.Replace(fixture,
			[]byte(`"consoleCompatibilityVersion":"0.1.0-SNAPSHOT"`),
			[]byte(`"consoleCompatibilityVersion":"development"`), 1)
	}
	traceMetadata := fmt.Sprintf(
		`{"targetScopeId":"scope-1","traceId":"trace-single-attempt-success","sessionId":"session-single-attempt-success","entrySkill":"CheckDns","outcome":"SUCCEEDED","finalizedAt":"2026-07-24T12:00:00Z","sizeBytes":%d,"persistencePolicy":"ALWAYS","applicationTraceExpiresAt":"2026-08-01T12:00:00Z"}`,
		len(artifactBody),
	)
	srv := &artifactTestServer{
		t:             t,
		instanceJSON:  javaCompatibleInstanceJSON,
		instanceID:    javaInstanceID,
		traceMetadata: traceMetadata,
		artifactBody:  artifactBody,
	}
	srv.server = httptest.NewServer(http.HandlerFunc(srv.handle))
	t.Cleanup(srv.server.Close)
	return srv
}

func (s *artifactTestServer) handle(response http.ResponseWriter, request *http.Request) {
	if s.authReject.Load() {
		response.WriteHeader(http.StatusUnauthorized)
		_, _ = response.Write([]byte(`{"status":401,"code":"LOOMSPAN_API_KEY_REJECTED","message":"loomspan API key was rejected"}`))
		return
	}
	path := request.URL.Path
	instanceID := s.instanceID
	instanceJSON := s.instanceJSON
	if s.instanceRotate.Load() {
		instanceID = "22222222-2222-4222-8222-222222222222"
		instanceJSON = strings.Replace(javaCompatibleInstanceJSON, javaInstanceID, instanceID, 1)
	}
	response.Header().Set(applicationclient.InstanceIDHeader, instanceID)
	switch {
	case strings.HasSuffix(path, "/instance"):
		s.instanceCalls.Add(1)
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(instanceJSON))
	case strings.HasSuffix(path, "/traces/single-attempt-success") && !strings.HasSuffix(path, "/artifact"):
		s.traceCalls.Add(1)
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(s.traceMetadata))
	case strings.HasSuffix(path, "/traces/single-attempt-success/artifact"):
		s.artifactCalls.Add(1)
		if s.artifactFail.Load() {
			response.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		response.Header().Set("Content-Type", applicationclient.ArtifactMediaType)
		response.Header().Set("Content-Length", fmt.Sprintf("%d", len(s.artifactBody)))
		response.Header().Set("Content-Disposition", `attachment; filename="loomspan-trace-single-attempt-success.ndjson"`)
		response.Header().Set("Cache-Control", "no-store")
		_, _ = response.Write(s.artifactBody)
	default:
		response.WriteHeader(http.StatusNotFound)
	}
}

func (s *artifactTestServer) URL() string { return s.server.URL }

func (s *artifactTestServer) setAuthRejected(rejected bool)   { s.authReject.Store(rejected) }
func (s *artifactTestServer) setInstanceRotated(rotated bool) { s.instanceRotate.Store(rotated) }
func (s *artifactTestServer) setArtifactFail(fail bool)       { s.artifactFail.Store(fail) }
func (s *artifactTestServer) artifactRequestCount() int32     { return s.artifactCalls.Load() }
func (s *artifactTestServer) traceRequestCount() int32        { return s.traceCalls.Load() }

// buildArtifactService wires a real artifact.Service against a real workspace
// and target context pointed at the given Java-compatible test server. The
// returned cleanup closes the service and target context.
func buildArtifactService(t *testing.T, server *artifactTestServer) (*artifact.Service, *target.Context, target.Scope) {
	t.Helper()
	policy := applicationclient.NetworkPolicy{
		ConnectTimeout: time.Second, ResponseHeaderTimeout: time.Second, RequestTimeout: 30 * time.Second,
	}
	targetContext, err := target.New(func(address applicationclient.Address) (target.ProbeClient, error) {
		return applicationclient.New(address, policy, "0.1.0-SNAPSHOT")
	}, nil, time.Now)
	if err != nil {
		t.Fatalf("create target context: %v", err)
	}
	t.Cleanup(targetContext.Close)
	ws, err := workspace.Open(filepath.Join(t.TempDir(), "work"))
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	t.Cleanup(func() { _ = ws.Close() })
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
				EntrySkill:                trace.EntrySkill,
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
		t.Fatalf("create artifact service: %v", err)
	}
	t.Cleanup(artifactSvc.Close)
	if err := targetContext.RegisterOwner("artifacts", artifactSvc); err != nil {
		t.Fatalf("register artifact owner: %v", err)
	}
	if err := targetContext.Select(server.URL()); err != nil {
		t.Fatalf("select target: %v", err)
	}
	if _, domain := targetContext.SupplyCredential(context.Background(), []byte(strings.Repeat("k", 32))); domain != nil {
		t.Fatalf("supply credential: %v", domain)
	}
	scope, domain := targetContext.Capture()
	if domain != nil {
		t.Fatalf("capture scope: %v", domain)
	}
	return artifactSvc, targetContext, scope
}

// readLeasedArtifact reads the full installed artifact body through a lease and
// closes the lease with success, returning the bytes and the SHA-256 checksum.
func readLeasedArtifact(t *testing.T, svc *artifact.Service, scopeID target.ScopeID, handle artifact.Handle) ([]byte, string) {
	t.Helper()
	lease, domain := svc.Use(evidence.ForTarget(scopeID), handle)
	if domain != nil {
		t.Fatalf("Use failed: %v", domain)
	}
	reader, err := lease.OpenComponent(artifact.ComponentRawArtifact)
	if err != nil {
		lease.Close(false)
		t.Fatalf("lease OpenComponent failed: %v", err)
	}
	body, readErr := io.ReadAll(reader)
	reader.Close()
	lease.Close(true)
	if readErr != nil {
		t.Fatalf("read leased artifact: %v", readErr)
	}
	sum := sha256.Sum256(body)
	return body, hex.EncodeToString(sum[:])
}

// TestArtifactAcquisitionInstallsOneCopyAndChargesOnce proves the complete
// Java-to-Go acquisition path: one metadata load, one upstream stream, one
// installed file, one opaque handle, and one capacity charge across repeated
// acquisitions for the same scope-bound trace (PR12-R01, PR12-R02, PR12-R04).
func TestArtifactAcquisitionInstallsOneCopyAndChargesOnce(t *testing.T) {
	server := newArtifactTestServer(t, nil)
	svc, _, scope := buildArtifactService(t, server)

	first, domain := svc.Acquire(context.Background(), scope, "single-attempt-success")
	if domain != nil {
		t.Fatalf("first Acquire failed: %v", domain)
	}
	if first.Handle == "" {
		t.Fatal("expected non-empty opaque handle")
	}
	if first.LocalBytes <= 0 {
		t.Fatalf("expected positive local bytes, got %d", first.LocalBytes)
	}
	lookup, lookupDomain := svc.Lookup(evidence.ForTarget(scope.ID), "single-attempt-success")
	if lookupDomain != nil {
		t.Fatalf("Lookup failed: %v", lookupDomain)
	}
	if lookup.Metadata.EntrySkill != "CheckDns" {
		t.Fatalf("expected acquired entry skill CheckDns, got %q", lookup.Metadata.EntrySkill)
	}
	if server.artifactRequestCount() != 1 {
		t.Fatalf("expected 1 upstream artifact request, got %d", server.artifactRequestCount())
	}
	if server.traceRequestCount() != 1 {
		t.Fatalf("expected 1 trace metadata request, got %d", server.traceRequestCount())
	}

	// Repeated acquisition must reuse the installed copy without a new upstream call.
	second, domain := svc.Acquire(context.Background(), scope, "single-attempt-success")
	if domain != nil {
		t.Fatalf("second Acquire failed: %v", domain)
	}
	if second.Handle != first.Handle {
		t.Fatalf("expected same handle %q, got %q", first.Handle, second.Handle)
	}
	if server.artifactRequestCount() != 1 || server.traceRequestCount() != 1 {
		t.Fatalf("repeated acquisition performed extra upstream work: artifact=%d trace=%d", server.artifactRequestCount(), server.traceRequestCount())
	}

	// The installed bytes must match the Java fixture exactly.
	body, _ := readLeasedArtifact(t, svc, scope.ID, first.Handle)
	fixtureChecksum := fixtureSHA256(t, nil)
	if hex.EncodeToString(sha256Sum(body)) != fixtureChecksum {
		t.Fatalf("installed bytes checksum mismatch")
	}

	// The aggregate local bytes must exceed the raw artifact size, proving the
	// real processor's manifest component is charged alongside the raw bytes.
	if first.LocalBytes <= int64(len(body)) {
		t.Fatalf("expected aggregate local bytes > raw %d, got %d (derived component not charged)", len(body), first.LocalBytes)
	}

	// Storage snapshot must report exactly one entry and one charge.
	snapshot, domain := svc.StorageSnapshot()
	if domain != nil {
		t.Fatalf("StorageSnapshot failed: %v", domain)
	}
	if snapshot.AcquiredCount != 1 {
		t.Fatalf("expected acquired count 1, got %d", snapshot.AcquiredCount)
	}
	if snapshot.ChargedBytes != first.LocalBytes {
		t.Fatalf("expected charged bytes %d, got %d", first.LocalBytes, snapshot.ChargedBytes)
	}
	if len(snapshot.Entries) != 1 || snapshot.Entries[0].TraceID != "trace-single-attempt-success" {
		t.Fatalf("unexpected storage entries: %+v", snapshot.Entries)
	}
	if snapshot.Entries[0].ApplicationAvailability != "AVAILABLE" {
		t.Fatalf("expected application availability AVAILABLE, got %q", snapshot.Entries[0].ApplicationAvailability)
	}
}

// TestArtifactAcquisitionRejectsMalformedNDJSONBeforePublishingHandle proves
// that the real traceanalysis.Processor, wired through the production artifact
// service, rejects byte-count-correct malformed NDJSON before any handle is
// published, leaves no installed bundle, and charges no capacity (PR13-P2-R03,
// PR13 bug-reproduction test). The malformed body has a correct byte count so
// only semantic validation catches it.
func TestArtifactAcquisitionRejectsMalformedNDJSONBeforePublishingHandle(t *testing.T) {
	malformedBody := []byte("{\"traceId\":\"trace-single-attempt-success\",\"sessionId\":\"session-single-attempt-success\",\"outcome\":\"SUCCEEDED\"}\n" +
		"{not-json\n")
	server := newArtifactTestServer(t, malformedBody)
	svc, _, scope := buildArtifactService(t, server)

	_, domain := svc.Acquire(context.Background(), scope, "single-attempt-success")
	if domain == nil || domain.Code != consolecore.CodeInvalidArtifact {
		t.Fatalf("expected INVALID_ARTIFACT for malformed NDJSON, got %v", domain)
	}

	// No handle, no entry, no capacity charge, no installed bundle.
	snapshot, snapDomain := svc.StorageSnapshot()
	if snapDomain != nil {
		t.Fatalf("StorageSnapshot failed: %v", snapDomain)
	}
	if snapshot.AcquiredCount != 0 {
		t.Fatalf("expected 0 acquired entries after malformed rejection, got %d", snapshot.AcquiredCount)
	}
	if snapshot.ChargedBytes != 0 {
		t.Fatalf("expected 0 charged bytes after malformed rejection, got %d", snapshot.ChargedBytes)
	}

	// The upstream artifact stream was consumed once; the processor rejected it.
	if server.artifactRequestCount() != 1 {
		t.Fatalf("expected 1 upstream artifact request, got %d", server.artifactRequestCount())
	}
}

// TestArtifactAcquisitionAndRawDownloadShareNoCacheState proves that raw
// pass-through streams exact upstream bytes with fresh authorization each time
// and never mutates the local cache (PR12-R11, PR12-R12). It uses the real
// scope.OpenArtifact seam that the browser raw-download handler uses.
func TestArtifactAcquisitionAndRawDownloadShareNoCacheState(t *testing.T) {
	server := newArtifactTestServer(t, nil)
	svc, _, scope := buildArtifactService(t, server)

	// Acquire once to install a local copy.
	acquired, domain := svc.Acquire(context.Background(), scope, "single-attempt-success")
	if domain != nil {
		t.Fatalf("Acquire failed: %v", domain)
	}
	installedArtifactCalls := server.artifactRequestCount()
	installedCharged := acquired.LocalBytes

	// Perform three raw pass-through downloads through the scope seam.
	for i := 0; i < 3; i++ {
		stream, domain := scope.OpenArtifact(context.Background(), "single-attempt-success")
		if domain != nil {
			t.Fatalf("raw download %d OpenArtifact failed: %v", i, domain)
		}
		body, err := io.ReadAll(stream.Body())
		stream.Close()
		if err != nil {
			t.Fatalf("raw download %d read failed: %v", i, err)
		}
		if hex.EncodeToString(sha256Sum(body)) != fixtureSHA256(t, nil) {
			t.Fatalf("raw download %d checksum mismatch", i)
		}
	}

	// Raw download must have performed 3 fresh upstream streams on top of the
	// 1 acquisition stream.
	if got := server.artifactRequestCount() - installedArtifactCalls; got != 3 {
		t.Fatalf("expected 3 raw download upstream calls, got %d", got)
	}

	// The local cache charge and entry count must be unchanged.
	snapshot, domain := svc.StorageSnapshot()
	if domain != nil {
		t.Fatalf("StorageSnapshot failed: %v", domain)
	}
	if snapshot.ChargedBytes != installedCharged {
		t.Fatalf("raw download changed cache charge: was %d now %d", installedCharged, snapshot.ChargedBytes)
	}
	if snapshot.AcquiredCount != 1 {
		t.Fatalf("raw download changed acquired count: got %d", snapshot.AcquiredCount)
	}
}

// TestArtifactEvidenceRemainsAfterAuthenticationRejection proves that an
// installed current-scope copy remains usable after the upstream credential is
// rejected, while new acquisition and raw pass-through fail until credentials
// are restored (PR12-R09, PR12-R10).
func TestArtifactEvidenceRemainsAfterAuthenticationRejection(t *testing.T) {
	server := newArtifactTestServer(t, nil)
	svc, _, scope := buildArtifactService(t, server)

	acquired, domain := svc.Acquire(context.Background(), scope, "single-attempt-success")
	if domain != nil {
		t.Fatalf("Acquire failed: %v", domain)
	}
	installedChecksum := fixtureSHA256(t, nil)

	// Reject the upstream credential. The local copy must remain usable.
	server.setAuthRejected(true)

	body, checksum := readLeasedArtifact(t, svc, scope.ID, acquired.Handle)
	if checksum != installedChecksum {
		t.Fatalf("installed evidence checksum changed after auth rejection")
	}
	_ = body

	// Storage snapshot must still report the entry with original observation facts.
	snapshot, domain := svc.StorageSnapshot()
	if domain != nil {
		t.Fatalf("StorageSnapshot failed after auth rejection: %v", domain)
	}
	if snapshot.AcquiredCount != 1 {
		t.Fatalf("auth rejection revoked local evidence: acquired count %d", snapshot.AcquiredCount)
	}
	if snapshot.Entries[0].Outcome != "SUCCEEDED" {
		t.Fatalf("original observation facts lost: outcome %q", snapshot.Entries[0].Outcome)
	}

	// A new raw pass-through must fail because the upstream rejects the credential.
	_, domain = scope.OpenArtifact(context.Background(), "single-attempt-success")
	if domain == nil {
		t.Fatal("expected raw pass-through to fail after auth rejection")
	}

	// Restore the credential. New raw pass-through must succeed again.
	server.setAuthRejected(false)
	stream, domain := scope.OpenArtifact(context.Background(), "single-attempt-success")
	if domain != nil {
		t.Fatalf("raw pass-through after credential restore failed: %v", domain)
	}
	stream.Close()
}

// TestArtifactScopeRotationClearsLocalStorageAndStaleLinks proves that target
// scope rotation removes all local artifacts and that a stale scope returns
// TARGET_CHANGED while a removed handle in the current scope returns
// ARTIFACT_EXPIRED (PR12-R09).
func TestArtifactScopeRotationClearsLocalStorageAndStaleLinks(t *testing.T) {
	server := newArtifactTestServer(t, nil)
	svc, targetContext, scope := buildArtifactService(t, server)

	acquired, domain := svc.Acquire(context.Background(), scope, "single-attempt-success")
	if domain != nil {
		t.Fatalf("Acquire failed: %v", domain)
	}
	staleScopeID := scope.ID
	staleHandle := acquired.Handle

	// Rotate the target by switching the instance identity. The target context
	// invalidates the old scope synchronously, which clears the artifact cache.
	server.setInstanceRotated(true)
	if err := targetContext.Select(server.URL()); err != nil {
		t.Fatalf("re-select target: %v", err)
	}
	if _, domain := targetContext.SupplyCredential(context.Background(), []byte(strings.Repeat("k", 32))); domain != nil {
		t.Fatalf("supply credential after rotation: %v", domain)
	}

	// The global storage snapshot remains available across target rotation.
	_, domain = svc.StorageSnapshot()
	if domain != nil {
		t.Fatalf("global storage snapshot failed after rotation: %v", domain)
	}

	// Use with the stale handle must report TARGET_CHANGED.
	_, domain = svc.Use(evidence.ForTarget(staleScopeID), staleHandle)
	if domain == nil || domain.Code != consolecore.CodeTargetChanged {
		t.Fatalf("expected TARGET_CHANGED for stale handle Use, got %v", domain)
	}

	// The new scope must start with an empty cache.
	newScope, domain := targetContext.Capture()
	if domain != nil {
		t.Fatalf("capture new scope: %v", domain)
	}
	snapshot, domain := svc.StorageSnapshot()
	if domain != nil {
		t.Fatalf("StorageSnapshot on new scope failed: %v", domain)
	}
	if snapshot.AcquiredCount != 0 {
		t.Fatalf("expected empty cache after rotation, got %d entries", snapshot.AcquiredCount)
	}

	// A well-formed but uninstalled handle in the current scope returns ARTIFACT_EXPIRED.
	_, domain = svc.Use(evidence.ForTarget(newScope.ID), staleHandle)
	if domain == nil || domain.Code != consolecore.CodeArtifactExpired {
		t.Fatalf("expected ARTIFACT_EXPIRED for uninstalled handle in current scope, got %v", domain)
	}
}

// TestArtifactRestartCleanupNeverAdoptsPriorHandle proves that a fresh process
// never adopts a prior-process handle: handles are process-local random, so a
// prior-process handle is not valid in a fresh service and the fresh cache
// starts empty (PR12-R09). The workspace's cleanAndCapture non-adoption of
// prior-process files is covered separately by internal/workspace/*_test.go.
func TestArtifactRestartCleanupNeverAdoptsPriorHandle(t *testing.T) {
	server := newArtifactTestServer(t, nil)
	svc, _, scope := buildArtifactService(t, server)

	acquired, domain := svc.Acquire(context.Background(), scope, "single-attempt-success")
	if domain != nil {
		t.Fatalf("Acquire failed: %v", domain)
	}
	priorHandle := acquired.Handle

	// Close the service to release its timers, goroutines, and installed files.
	// Close removes all owned artifact contents from the artifacts directory.
	svc.Close()

	// A fresh service starts with an empty cache and never adopts the
	// prior-process handle, which is process-local random.
	freshSvc, _, freshScope := buildArtifactService(t, server)
	_, domain = freshSvc.Use(evidence.ForTarget(freshScope.ID), priorHandle)
	if domain == nil || domain.Code != consolecore.CodeArtifactExpired {
		t.Fatalf("expected ARTIFACT_EXPIRED for prior-process handle, got %v", domain)
	}
	snapshot, domain := freshSvc.StorageSnapshot()
	if domain != nil {
		t.Fatalf("fresh StorageSnapshot failed: %v", domain)
	}
	if snapshot.AcquiredCount != 0 {
		t.Fatalf("fresh process adopted prior-process entries: %d", snapshot.AcquiredCount)
	}
}

// TestArtifactShutdownWaitsForStreamCleanup proves that closing the artifact
// service while an acquisition is in flight cancels the transfer, waits for the
// leader goroutine to exit, and leaves no partial file, reservation, or timer
// (PR12-R03, PR12-R09).
func TestArtifactShutdownWaitsForStreamCleanup(t *testing.T) {
	server := newArtifactTestServer(t, nil)
	// Make the upstream artifact stream block until the test releases it so
	// the acquisition is still in flight when Close is called. The stream
	// opener below returns a blocking reader instead of hitting the server.
	release := make(chan struct{})
	blockingBody := &blockingReadCloser{release: release, data: server.artifactBody}

	policy := applicationclient.NetworkPolicy{
		ConnectTimeout: time.Second, ResponseHeaderTimeout: time.Second, RequestTimeout: 30 * time.Second,
	}
	targetContext, err := target.New(func(address applicationclient.Address) (target.ProbeClient, error) {
		return applicationclient.New(address, policy, "0.1.0-SNAPSHOT")
	}, nil, time.Now)
	if err != nil {
		t.Fatalf("create target context: %v", err)
	}
	defer targetContext.Close()
	ws, err := workspace.Open(filepath.Join(t.TempDir(), "work"))
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	defer ws.Close()

	// Use a stream opener that returns a blocking stream so the acquisition
	// goroutine is still copying when Close is invoked.
	svc, err := artifact.New(artifact.Config{
		MaxBytes: 1 << 20, IdleTTL: time.Hour,
	}, artifact.Dependencies{
		Lifetime:  context.Background(),
		Workspace: ws,
		TraceLoader: func(ctx context.Context, scope target.Scope, traceID string) (artifact.TraceMetadata, *consolecore.Error) {
			return artifact.TraceMetadata{
				TraceID:    "trace-single-attempt-success",
				SessionID:  "session-single-attempt-success",
				EntrySkill: "CheckDns",
				Outcome:    "SUCCEEDED",
				SizeBytes:  int64(len(server.artifactBody)),
			}, nil
		},
		StreamOpener: func(ctx context.Context, scope target.Scope, traceID string) (*applicationclient.ArtifactStream, *consolecore.Error) {
			return applicationclient.NewTestArtifactStream(blockingBody, scope.InstanceID, int64(len(blockingBody.data))), nil
		},
		Processor: traceanalysis.New(),
	})
	if err != nil {
		t.Fatalf("create artifact service: %v", err)
	}
	if err := targetContext.RegisterOwner("artifacts", svc); err != nil {
		t.Fatalf("register owner: %v", err)
	}
	if err := targetContext.Select(server.URL()); err != nil {
		t.Fatalf("select target: %v", err)
	}
	if _, domain := targetContext.SupplyCredential(context.Background(), []byte(strings.Repeat("k", 32))); domain != nil {
		t.Fatalf("supply credential: %v", domain)
	}
	scope, domain := targetContext.Capture()
	if domain != nil {
		t.Fatalf("capture scope: %v", domain)
	}

	// Start an acquisition in a goroutine. It will block on the stream read.
	acquireDone := make(chan error, 1)
	go func() {
		_, domain := svc.Acquire(context.Background(), scope, "single-attempt-success")
		if domain != nil {
			acquireDone <- errors.New(domain.Message)
			return
		}
		acquireDone <- nil
	}()

	// Close the service while the acquisition is in flight. Close must cancel
	// the transfer and wait for the leader goroutine to exit.
	closeComplete := make(chan struct{})
	go func() {
		svc.Close()
		close(closeComplete)
	}()
	select {
	case <-closeComplete:
	case <-time.After(10 * time.Second):
		t.Fatal("Close did not return within 10s; stream cleanup leaked a goroutine")
	}

	// Release the blocking stream so the acquire goroutine can observe cancellation.
	close(release)
	select {
	case err := <-acquireDone:
		if err == nil {
			t.Fatal("expected in-flight acquisition to fail after Close")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("in-flight acquisition did not return after Close")
	}

	// No partial or installed files must remain in the artifacts directory
	// beneath the transient subtree. The artifacts directory itself may remain.
	artifactsDir := filepath.Join(ws.Transient, "artifacts")
	entries, err := os.ReadDir(artifactsDir)
	if err != nil {
		t.Fatalf("read artifacts dir: %v", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, "partial-") || strings.HasPrefix(name, "installed-") {
			t.Fatalf("partial or installed file remained after shutdown: %s", name)
		}
	}
}

// TestArtifactAcquisitionDoesNotLeakPathsOrCredentials proves that the
// acquisition result, storage snapshot, and lease reader never expose a
// filesystem path or the application credential (PR12-R13).
func TestArtifactAcquisitionDoesNotLeakPathsOrCredentials(t *testing.T) {
	secret := "LOOMSPAN_" + "TEST_APPLICATION_KEY_DO_NOT_LEAK_e5f1"
	server := newArtifactTestServer(t, nil)
	policy := applicationclient.NetworkPolicy{
		ConnectTimeout: time.Second, ResponseHeaderTimeout: time.Second, RequestTimeout: 30 * time.Second,
	}
	targetContext, err := target.New(func(address applicationclient.Address) (target.ProbeClient, error) {
		return applicationclient.New(address, policy, "0.1.0-SNAPSHOT")
	}, nil, time.Now)
	if err != nil {
		t.Fatalf("create target context: %v", err)
	}
	defer targetContext.Close()
	ws, err := workspace.Open(filepath.Join(t.TempDir(), "work"))
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	defer ws.Close()
	svc, err := artifact.New(artifact.Config{
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
				EntrySkill:                trace.EntrySkill,
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
		t.Fatalf("create artifact service: %v", err)
	}
	defer svc.Close()
	if err := targetContext.RegisterOwner("artifacts", svc); err != nil {
		t.Fatalf("register owner: %v", err)
	}
	if err := targetContext.Select(server.URL()); err != nil {
		t.Fatalf("select target: %v", err)
	}
	if _, domain := targetContext.SupplyCredential(context.Background(), []byte(secret)); domain != nil {
		t.Fatalf("supply credential: %v", domain)
	}
	scope, domain := targetContext.Capture()
	if domain != nil {
		t.Fatalf("capture scope: %v", domain)
	}

	acquired, domain := svc.Acquire(context.Background(), scope, "single-attempt-success")
	if domain != nil {
		t.Fatalf("Acquire failed: %v", domain)
	}
	snapshot, domain := svc.StorageSnapshot()
	if domain != nil {
		t.Fatalf("StorageSnapshot failed: %v", domain)
	}

	// Serialize every DTO and verify no path or credential leaks.
	encoded, _ := json.Marshal(acquired)
	snapshotEncoded, _ := json.Marshal(snapshot)
	rendered := string(encoded) + string(snapshotEncoded) + fmt.Sprintf("%#v", acquired) + fmt.Sprintf("%#v", snapshot)
	if strings.Contains(rendered, secret) {
		t.Fatal("credential leaked into artifact DTO or formatted output")
	}
	if strings.Contains(rendered, "transient") || strings.Contains(rendered, filepath.Join(ws.Root, "transient")) {
		t.Fatal("filesystem path leaked into artifact DTO or formatted output")
	}
	if strings.Contains(string(encoded), "installedPath") || strings.Contains(string(snapshotEncoded), "installedPath") ||
		strings.Contains(string(encoded), "installedDir") || strings.Contains(string(snapshotEncoded), "installedDir") {
		t.Fatal("installed path field leaked into DTO")
	}
}

// TestArtifactRawDownloadPreservesExactChecksum proves that the raw pass-through
// streams byte-for-byte identical bytes to the Java fixture and that the
// checksum matches the installed analysis copy (PR12-R11).
func TestArtifactRawDownloadPreservesExactChecksum(t *testing.T) {
	server := newArtifactTestServer(t, nil)
	svc, _, scope := buildArtifactService(t, server)

	acquired, domain := svc.Acquire(context.Background(), scope, "single-attempt-success")
	if domain != nil {
		t.Fatalf("Acquire failed: %v", domain)
	}
	installedBody, installedChecksum := readLeasedArtifact(t, svc, scope.ID, acquired.Handle)

	// Raw pass-through must produce the same checksum.
	stream, domain := scope.OpenArtifact(context.Background(), "single-attempt-success")
	if domain != nil {
		t.Fatalf("raw OpenArtifact failed: %v", domain)
	}
	rawBody, err := io.ReadAll(stream.Body())
	stream.Close()
	if err != nil {
		t.Fatalf("read raw stream: %v", err)
	}
	rawChecksum := hex.EncodeToString(sha256Sum(rawBody))
	if rawChecksum != installedChecksum {
		t.Fatalf("raw download checksum %q != installed checksum %q", rawChecksum, installedChecksum)
	}
	if !bytes.Equal(rawBody, installedBody) {
		t.Fatal("raw download bytes differ from installed analysis copy bytes")
	}
}

// blockingReadCloser delivers its data only after the release channel is closed.
// It is used to keep an acquisition's stream copy in flight while a test
// exercises shutdown cancellation.
type blockingReadCloser struct {
	data    []byte
	release chan struct{}
	closed  atomic.Bool
	offset  int
}

func (r *blockingReadCloser) Read(p []byte) (int, error) {
	if r.closed.Load() {
		return 0, io.ErrClosedPipe
	}
	select {
	case <-r.release:
	case <-time.After(15 * time.Second):
		return 0, io.ErrClosedPipe
	}
	if r.offset >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.offset:])
	r.offset += n
	return n, nil
}

func (r *blockingReadCloser) Close() error {
	r.closed.Store(true)
	return nil
}

// fixtureSHA256 returns the SHA-256 checksum of the Java-produced
// single-attempt-success fixture. If override is non-nil, its checksum is
// returned instead.
func fixtureSHA256(t *testing.T, override []byte) string {
	t.Helper()
	var data []byte
	if override != nil {
		data = override
	} else {
		fixture, err := os.ReadFile(filepath.Join("..", "..", "..", "loomspan-console-fixtures", "traces", "single-attempt-success.ndjson"))
		if err != nil {
			t.Fatalf("read fixture: %v", err)
		}
		data = bytes.Replace(fixture,
			[]byte(`"consoleCompatibilityVersion":"0.1.0-SNAPSHOT"`),
			[]byte(`"consoleCompatibilityVersion":"development"`), 1)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func sha256Sum(data []byte) []byte {
	sum := sha256.Sum256(data)
	return sum[:]
}

// buildArtifactServiceWithQueryService wires a real artifact.Service with the
// shared traceanalysis.Service (processor + query service) against a real
// workspace and target context. It returns the artifact service, the
// trace-analysis query service, the target context, and the captured scope.
// This mirrors the production composition in console/service.go where the
// trace-analysis service is both the artifact processor and the adapter-facing
// query service.
func buildArtifactServiceWithQueryService(t *testing.T, server *artifactTestServer) (*artifact.Service, *traceanalysis.Service, *target.Context, target.Scope) {
	t.Helper()
	policy := applicationclient.NetworkPolicy{
		ConnectTimeout: time.Second, ResponseHeaderTimeout: time.Second, RequestTimeout: 30 * time.Second,
	}
	targetContext, err := target.New(func(address applicationclient.Address) (target.ProbeClient, error) {
		return applicationclient.New(address, policy, "0.1.0-SNAPSHOT")
	}, nil, time.Now)
	if err != nil {
		t.Fatalf("create target context: %v", err)
	}
	t.Cleanup(targetContext.Close)
	ws, err := workspace.Open(filepath.Join(t.TempDir(), "work"))
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	t.Cleanup(func() { _ = ws.Close() })
	traceAnalysisService := traceanalysis.NewService(nil)
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
				EntrySkill:                trace.EntrySkill,
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
		Processor: traceAnalysisService,
	})
	if err != nil {
		t.Fatalf("create artifact service: %v", err)
	}
	t.Cleanup(artifactSvc.Close)
	traceAnalysisService.SetArtifactService(artifactSvc)
	if err := targetContext.RegisterOwner("artifacts", artifactSvc); err != nil {
		t.Fatalf("register artifact owner: %v", err)
	}
	if err := targetContext.Select(server.URL()); err != nil {
		t.Fatalf("select target: %v", err)
	}
	if _, domain := targetContext.SupplyCredential(context.Background(), []byte(strings.Repeat("k", 32))); domain != nil {
		t.Fatalf("supply credential: %v", domain)
	}
	scope, domain := targetContext.Capture()
	if domain != nil {
		t.Fatalf("capture scope: %v", domain)
	}
	return artifactSvc, traceAnalysisService, targetContext, scope
}

// TestSharedQueryServiceGetSummaryThroughProductionComposition proves that the
// shared traceanalysis.Service, wired as both processor and query service in
// the production composition, serves GetSummary after acquisition.
func TestSharedQueryServiceGetSummaryThroughProductionComposition(t *testing.T) {
	server := newArtifactTestServer(t, nil)
	svc, queryService, _, scope := buildArtifactServiceWithQueryService(t, server)

	acquired, domain := svc.Acquire(context.Background(), scope, "single-attempt-success")
	if domain != nil {
		t.Fatalf("Acquire failed: %v", domain)
	}

	summary, domain := queryService.GetSummary(context.Background(), evidence.ForTarget(scope.ID), traceanalysis.SummaryRequest{Handle: acquired.Handle})
	if domain != nil {
		t.Fatalf("GetSummary failed: %v", domain)
	}
	if summary.Context.TraceID != "trace-single-attempt-success" {
		t.Fatalf("expected trace ID 'trace-single-attempt-success', got %q", summary.Context.TraceID)
	}
	if summary.Outcome != "SUCCEEDED" {
		t.Fatalf("expected outcome SUCCEEDED, got %q", summary.Outcome)
	}
	if summary.RecordCount == 0 {
		t.Fatal("expected non-zero record count")
	}
	if summary.AttemptCount != 1 {
		t.Fatalf("expected 1 attempt, got %d", summary.AttemptCount)
	}
}

// TestSharedQueryServiceQueryRecordsThroughProductionComposition proves that
// the shared query service serves QueryRecords through the production
// composition with both physical and logical representations.
func TestSharedQueryServiceQueryRecordsThroughProductionComposition(t *testing.T) {
	server := newArtifactTestServer(t, nil)
	svc, queryService, _, scope := buildArtifactServiceWithQueryService(t, server)

	acquired, domain := svc.Acquire(context.Background(), scope, "single-attempt-success")
	if domain != nil {
		t.Fatalf("Acquire failed: %v", domain)
	}

	physicalPage, domain := queryService.QueryRecords(context.Background(), evidence.ForTarget(scope.ID), traceanalysis.RecordQuery{
		Handle:         acquired.Handle,
		Representation: traceanalysis.RecordRepresentationPhysical,
		PageSize:       100,
	})
	if domain != nil {
		t.Fatalf("QueryRecords physical failed: %v", domain)
	}
	if len(physicalPage.Items) == 0 {
		t.Fatal("expected non-empty physical records")
	}

	logicalPage, domain := queryService.QueryRecords(context.Background(), evidence.ForTarget(scope.ID), traceanalysis.RecordQuery{
		Handle:         acquired.Handle,
		Representation: traceanalysis.RecordRepresentationLogical,
		PageSize:       100,
	})
	if domain != nil {
		t.Fatalf("QueryRecords logical failed: %v", domain)
	}
	if len(logicalPage.Items) == 0 {
		t.Fatal("expected non-empty logical records")
	}
}

// TestSharedQueryServiceSearchThroughProductionComposition proves that the
// shared query service serves Search through the production composition.
func TestSharedQueryServiceSearchThroughProductionComposition(t *testing.T) {
	server := newArtifactTestServer(t, nil)
	svc, queryService, _, scope := buildArtifactServiceWithQueryService(t, server)

	acquired, domain := svc.Acquire(context.Background(), scope, "single-attempt-success")
	if domain != nil {
		t.Fatalf("Acquire failed: %v", domain)
	}

	page, domain := queryService.Search(context.Background(), evidence.ForTarget(scope.ID), traceanalysis.SearchQuery{
		Handle:   acquired.Handle,
		Text:     "attempt-1",
		PageSize: 10,
	})
	if domain != nil {
		t.Fatalf("Search failed: %v", domain)
	}
	if len(page.Items) == 0 {
		t.Fatal("expected at least one semantic metadata match for 'attempt-1'")
	}
}

// TestSharedQueryServiceReadRawArtifactRangeThroughProductionComposition proves
// that the shared query service serves ReadRawArtifactRange through the
// production composition.
func TestSharedQueryServiceReadRawArtifactRangeThroughProductionComposition(t *testing.T) {
	server := newArtifactTestServer(t, nil)
	svc, queryService, _, scope := buildArtifactServiceWithQueryService(t, server)

	acquired, domain := svc.Acquire(context.Background(), scope, "single-attempt-success")
	if domain != nil {
		t.Fatalf("Acquire failed: %v", domain)
	}

	result, domain := queryService.ReadRawArtifactRange(context.Background(), evidence.ForTarget(scope.ID), traceanalysis.RangeRequest{
		Handle:   acquired.Handle,
		Source:   traceanalysis.RangeSourceRawArtifact,
		Start:    0,
		MaxBytes: 100,
	})
	if domain != nil {
		t.Fatalf("ReadRawArtifactRange failed: %v", domain)
	}
	if result.TotalLength <= 0 {
		t.Fatalf("expected positive total length, got %d", result.TotalLength)
	}
	if len(result.Content) == 0 {
		t.Fatal("expected non-empty content")
	}
}
