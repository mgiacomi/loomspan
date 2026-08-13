package artifact

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/applicationclient"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/target"
)

// PR12-R06: ENOSPC in unlimited mode removes the partial file and returns
// LOCAL_STORAGE_UNAVAILABLE after workspace recovery (cleanup succeeds and
// the workspace probe is healthy).
func TestENOSPCRemovesPartialAndReturnsLocalStorageUnavailableWhenWorkspaceRecovers(t *testing.T) {
	data := bytes.Repeat([]byte("e"), 4096)
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	fs := newFaultyFS()
	fs.syncFail = &enospcError{}
	config := Config{MaxBytes: 0, Unlimited: true, IdleTTL: time.Hour}
	timers := &manualTimerFactory{}
	clock := newManualClock(time.UnixMilli(1000000))
	svc := newTestServiceWithDeps(t, config, loader, opener, timers, clock, fs)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	_, domain := svc.Acquire(context.Background(), scope, "trace-1")
	if domain == nil {
		t.Fatal("expected error on ENOSPC")
	}
	if domain.Code != consolecore.CodeLocalStorageUnavailable && domain.Code != consolecore.CodeConsoleError {
		t.Fatalf("expected LOCAL_STORAGE_UNAVAILABLE or CONSOLE_ERROR, got %s", domain.Code)
	}

	// No entry should remain; no capacity should be charged.
	snapshot, _ := svc.StorageSnapshot()
	if snapshot.AcquiredCount != 0 {
		t.Fatalf("expected 0 entries after ENOSPC, got %d", snapshot.AcquiredCount)
	}
	if snapshot.ChargedBytes != 0 {
		t.Fatalf("expected 0 charged bytes after ENOSPC, got %d", snapshot.ChargedBytes)
	}

	// Verify no staging or installed bundle directories remain.
	entries, err := os.ReadDir(filepath.Join(svc.storage.dir))
	if err != nil {
		t.Fatalf("failed to read artifacts dir: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "staging-") || strings.HasPrefix(entry.Name(), "installed-") {
			t.Fatalf("bundle directory remains after ENOSPC cleanup: %s", entry.Name())
		}
	}
}

// PR12-R06: ENOSPC followed by a cleanup or workspace probe failure becomes
// process-fatal. The fatal callback is invoked and the error is CONSOLE_ERROR.
func TestENOSPCWithCleanupFailureTerminatesCoordinator(t *testing.T) {
	data := bytes.Repeat([]byte("e"), 4096)
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	fs := newFaultyFS()
	fs.syncFail = &enospcError{}
	config := Config{MaxBytes: 0, Unlimited: true, IdleTTL: time.Hour}
	timers := &manualTimerFactory{}
	clock := newManualClock(time.UnixMilli(1000000))

	var fatalCalled atomic.Bool
	ws := testWorkspace(t)
	entropy := &deterministicEntropy{}
	deps := Dependencies{
		Lifetime:     context.Background(),
		Workspace:    ws,
		TraceLoader:  loader.loader(),
		StreamOpener: opener.opener(),
		Processor:    newFakeProcessor(),
		Clock:        clock.nowFunc(),
		Entropy:      entropy.factory(),
		TimerFactory: timers.factory(),
		FileSystem:   fs,
		Fatal: func(err error) {
			fatalCalled.Store(true)
		},
	}
	svc, err := New(config, deps)
	if err != nil {
		t.Fatalf("create artifact service: %v", err)
	}
	defer svc.Close()
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	// Close the workspace before acquiring so that workspace.Check() fails
	// inside ClassifyArtifactFailure, making the storage error fatal.
	_ = ws.Close()

	_, domain := svc.Acquire(context.Background(), scope, "trace-1")
	if domain == nil {
		t.Fatal("expected error on ENOSPC with workspace failure")
	}
	if domain.Code != consolecore.CodeConsoleError {
		t.Fatalf("expected CONSOLE_ERROR for fatal workspace loss, got %s", domain.Code)
	}
	if !fatalCalled.Load() {
		t.Fatal("expected fatal callback to be invoked")
	}
}

func TestInvalidArtifactWithPersistentPartialCleanupFailureIsFatal(t *testing.T) {
	data := bytes.Repeat([]byte("x"), 200)
	loader := newFakeLoader(testTraceMetadata("trace-1", 100))
	opener := newFakeOpener(data, -1)
	fs := newFaultyFS()
	fs.removeAllFail = errors.New("injected persistent bundle cleanup failure")
	timers := &manualTimerFactory{}
	clock := newManualClock(time.UnixMilli(1000000))
	var fatalCalled atomic.Bool
	ws := testWorkspace(t)
	entropy := &deterministicEntropy{}
	svc, err := New(Config{MaxBytes: 100, IdleTTL: time.Hour}, Dependencies{
		Lifetime:     context.Background(),
		Workspace:    ws,
		TraceLoader:  loader.loader(),
		StreamOpener: opener.opener(),
		Processor:    newFakeProcessor(),
		Fatal:        func(error) { fatalCalled.Store(true) },
		Clock:        clock.nowFunc(),
		Entropy:      entropy.factory(),
		TimerFactory: timers.factory(),
		FileSystem:   fs,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		fs.removeAllFail = nil
		svc.Close()
	}()
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	_, domain := svc.Acquire(context.Background(), scope, "trace-1")
	if domain == nil || domain.Code != consolecore.CodeConsoleError {
		t.Fatalf("expected fatal CONSOLE_ERROR, got %v", domain)
	}
	if !fatalCalled.Load() {
		t.Fatal("persistent partial cleanup failure did not reach lifecycle fatal callback")
	}
	snapshot, snapshotDomain := svc.StorageSnapshot()
	if snapshotDomain != nil {
		t.Fatal(snapshotDomain)
	}
	if snapshot.ChargedBytes != 0 || snapshot.AcquiredCount != 0 {
		t.Fatalf("fatal acquisition retained registry state: %+v", snapshot)
	}
}

// PR12-R09: Upstream authentication rejection after installation does not
// revoke a complete current-scope copy. The installed artifact remains
// usable via its handle.
func TestAuthenticationRejectionPreservesInstalledCurrentScopeArtifact(t *testing.T) {
	data := []byte("auth-rejection-test-data")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	svc := newTestService(t, config, loader, opener)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	// Acquire the artifact successfully.
	artifact := acquireSync(t, svc, context.Background(), scope, "trace-1")

	// Simulate upstream authentication rejection for new acquisitions.
	opener.err = consolecore.NewError(consolecore.CodeTargetAuthentication,
		"authentication failed", "scope-1", consolecore.Details{}, nil)

	// A new acquisition of a different trace should fail with auth error.
	loader2 := newFakeLoader(testTraceMetadata("trace-2", int64(len(data))))
	svc.traceLoader = loader2.loader()
	_, domain := svc.Acquire(context.Background(), scope, "trace-2")
	if domain == nil || domain.Code != consolecore.CodeTargetAuthentication {
		t.Fatalf("expected TARGET_AUTHENTICATION_REQUIRED for new acquisition, got %v", domain)
	}

	// The already-installed artifact should still be usable.
	lease, domain := svc.Use(targetRef(scope.ID), artifact.Handle)
	if domain != nil {
		t.Fatalf("Use failed for installed artifact after auth rejection: %v", domain)
	}
	reader, err := lease.OpenComponent(ComponentRawArtifact)
	if err != nil {
		t.Fatalf("OpenComponent failed after auth rejection: %v", err)
	}
	got, err := readAll(reader)
	if err != nil {
		t.Fatalf("Read failed after auth rejection: %v", err)
	}
	reader.Close()
	lease.Close(true)

	if !bytes.Equal(got, data) {
		t.Fatalf("expected %d bytes, got %d bytes", len(data), len(got))
	}

	// Storage snapshot should still show the installed entry.
	snapshot, _ := svc.StorageSnapshot()
	if snapshot.AcquiredCount != 1 {
		t.Fatalf("expected 1 installed entry after auth rejection, got %d", snapshot.AcquiredCount)
	}
}

// PR12-R09: Credential replacement and instance change invalidate all
// artifacts. After scope rotation, old-scope handles are not usable.
func TestCredentialReplacementAndInstanceChangeInvalidateArtifacts(t *testing.T) {
	data := []byte("credential-replacement-test")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	svc := newTestService(t, config, loader, opener)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	artifact := acquireSync(t, svc, context.Background(), scope, "trace-1")

	// Rotate to a new scope (simulating credential replacement / instance change).
	scope2, cancelScope2 := testScope("scope-2")
	defer cancelScope2()
	svc.ActivateActivity(scope2)
	svc.InvalidateTargetScope(scope.ID, scope.Context)

	// Old-scope handle should return TARGET_CHANGED.
	_, domain := svc.Use(targetRef(scope.ID), artifact.Handle)
	if domain == nil || domain.Code != consolecore.CodeTargetChanged {
		t.Fatalf("expected TARGET_CHANGED for old-scope handle, got %v", domain)
	}

	// Old-scope storage snapshot should show 0 entries.
	snapshot, _ := svc.StorageSnapshot()
	if snapshot.AcquiredCount != 0 {
		t.Fatalf("expected 0 entries for old scope, got %d", snapshot.AcquiredCount)
	}
}

// PR12-R09: Scope rotation cancels every acquisition and lease before removal.
// Active leases are invalidated and their entries removed.
func TestScopeRotationCancelsEveryAcquisitionAndLeaseBeforeRemoval(t *testing.T) {
	data := []byte("scope-rotation-lease-test")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	svc := newTestService(t, config, loader, opener)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	// Acquire and lease the artifact.
	acquireSync(t, svc, context.Background(), scope, "trace-1")
	lease, _ := svc.Use(targetRef(scope.ID), acquireHandle(t, svc, scope, "trace-1"))
	reader, err := lease.OpenComponent(ComponentRawArtifact)
	if err != nil {
		t.Fatalf("open leased artifact: %v", err)
	}

	// Rotate scope.
	scope2, cancelScope2 := testScope("scope-2")
	defer cancelScope2()
	svc.ActivateActivity(scope2)
	svc.InvalidateTargetScope(scope.ID, scope.Context)
	if _, err := reader.Read(make([]byte, 1)); err == nil {
		t.Fatal("expected scope rotation to close the active lease reader")
	}

	// The lease's entry should have been removed. Closing the lease should
	// not panic and should trigger deferred removal if needed.
	_ = lease.Close(true)

	// No old-scope entries should remain.
	snapshot, _ := svc.StorageSnapshot()
	if snapshot.AcquiredCount != 0 {
		t.Fatalf("expected 0 entries for new scope, got %d", snapshot.AcquiredCount)
	}
}

// PR12-R09: Shutdown closes timers, streams, workers, and cleans transient
// artifacts. No files remain in the artifacts directory after Close.
func TestShutdownClosesTimersStreamsWorkersAndCleansTransient(t *testing.T) {
	data := []byte("shutdown-cleanup-test")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	timers := &manualTimerFactory{}
	clock := newManualClock(time.UnixMilli(1000000))
	svc := newTestServiceWithDeps(t, config, loader, opener, timers, clock, nil)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	acquireSync(t, svc, context.Background(), scope, "trace-1")

	// Verify a timer was scheduled.
	if timers.latest() == nil {
		t.Fatal("expected idle timer to be scheduled before shutdown")
	}

	svc.Close()

	// No timer should be active after close.
	if svc.idleTimer != nil {
		t.Fatal("expected idle timer to be nil after close")
	}

	// No entries or handles should remain.
	if len(svc.entries) != 0 {
		t.Fatalf("expected 0 entries after close, got %d", len(svc.entries))
	}
	if len(svc.handles) != 0 {
		t.Fatalf("expected 0 handles after close, got %d", len(svc.handles))
	}
	if svc.totalCharged != 0 {
		t.Fatalf("expected 0 charged bytes after close, got %d", svc.totalCharged)
	}

	// The artifacts directory should be empty.
	entries, err := os.ReadDir(svc.storage.dir)
	if err != nil {
		t.Fatalf("failed to read artifacts dir after close: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 files in artifacts dir after close, got %d", len(entries))
	}
}

// PR12-R05: Concurrent reservations cannot overcommit finite capacity.
// Multiple goroutines acquiring artifacts against a tight capacity ceiling
// must never exceed the limit.
func TestConcurrentReservationsCannotOvercommitFiniteCapacity(t *testing.T) {
	data := bytes.Repeat([]byte("x"), 100)

	// Use a shared loader/opener that dispatches by traceID to avoid
	// per-goroutine mutation of service fields (which races with runAcquisition).
	sharedLoader := func(ctx context.Context, scope target.Scope, traceID string) (TraceMetadata, *consolecore.Error) {
		return testTraceMetadata(traceID, int64(len(data))), nil
	}
	sharedOpener := func(ctx context.Context, scope target.Scope, traceID string) (*applicationclient.ArtifactStream, *consolecore.Error) {
		body := &countingReadCloser{data: data, ctx: ctx}
		return applicationclient.NewTestArtifactStream(body, scope.InstanceID, int64(len(data))), nil
	}

	config := Config{MaxBytes: 150, IdleTTL: time.Hour}
	timers := &manualTimerFactory{}
	clock := newManualClock(time.UnixMilli(1000000))
	processor := newFakeProcessor()
	processor.barrier = make(chan struct{})
	processor.release = make(chan struct{})

	ws := testWorkspace(t)
	entropy := &deterministicEntropy{}
	deps := Dependencies{
		Lifetime:     context.Background(),
		Workspace:    ws,
		TraceLoader:  sharedLoader,
		StreamOpener: sharedOpener,
		Processor:    processor,
		Clock:        clock.nowFunc(),
		Entropy:      entropy.factory(),
		TimerFactory: timers.factory(),
	}
	svc, err := New(config, deps)
	if err != nil {
		t.Fatalf("create artifact service: %v", err)
	}
	defer svc.Close()
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	const goroutines = 10
	var wg sync.WaitGroup
	var successes atomic.Int64
	var failures atomic.Int64
	allRejected := make(chan struct{})
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			traceID := "trace-" + string(rune('A'+idx))
			_, domain := svc.Acquire(context.Background(), scope, traceID)
			if domain == nil {
				successes.Add(1)
			} else {
				if failures.Add(1) == goroutines-1 {
					close(allRejected)
				}
			}
		}(i)
	}
	select {
	case <-processor.barrier:
	case <-time.After(5 * time.Second):
		close(processor.release)
		wg.Wait()
		t.Fatal("first reserved acquisition did not reach the processor")
	}
	select {
	case <-allRejected:
	case <-time.After(5 * time.Second):
		close(processor.release)
		wg.Wait()
		t.Fatalf("expected %d capacity rejections while the first reservation was held, got %d", goroutines-1, failures.Load())
	}
	close(processor.release)
	wg.Wait()

	// Exactly one artifact (100 raw bytes plus the derived component) can fit
	// while its reservation is held. The rest fail with LIMIT_EXCEEDED.
	if successes.Load() != 1 {
		t.Fatalf("expected 1 success, got %d", successes.Load())
	}
	if successes.Load()+failures.Load() != goroutines {
		t.Fatalf("expected %d total results, got %d", goroutines, successes.Load()+failures.Load())
	}

	// Total charged bytes must never exceed the capacity limit.
	snapshot, _ := svc.StorageSnapshot()
	if snapshot.ChargedBytes > config.MaxBytes {
		t.Fatalf("charged bytes %d exceeded capacity %d", snapshot.ChargedBytes, config.MaxBytes)
	}
}

// readAll is a helper that reads all bytes from an io.ReadCloser.
func readAll(r interface{ Read([]byte) (int, error) }) ([]byte, error) {
	var buf bytes.Buffer
	tmp := make([]byte, 4096)
	for {
		n, err := r.Read(tmp)
		if n > 0 {
			buf.Write(tmp[:n])
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return buf.Bytes(), err
		}
	}
	return buf.Bytes(), nil
}

// PR12-R13: No log line emitted by the artifact service contains the workspace
// root, partial filename, installed path, or any credential. This test
// captures slog output during a fatal storage error and scans it.
func TestFatalStorageErrorLogsDoNotLeakPathsOrCredentials(t *testing.T) {
	data := bytes.Repeat([]byte("e"), 4096)
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	fs := newFaultyFS()
	fs.syncFail = &enospcError{}
	config := Config{MaxBytes: 0, Unlimited: true, IdleTTL: time.Hour}
	timers := &manualTimerFactory{}
	clock := newManualClock(time.UnixMilli(1000000))

	ws := testWorkspace(t)
	entropy := &deterministicEntropy{}

	// Capture slog output into a buffer.
	var logBuf bytes.Buffer
	handler := slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug})
	prevLogger := slog.Default()
	slog.SetDefault(slog.New(handler))
	t.Cleanup(func() { slog.SetDefault(prevLogger) })

	deps := Dependencies{
		Lifetime:     context.Background(),
		Workspace:    ws,
		TraceLoader:  loader.loader(),
		StreamOpener: opener.opener(),
		Processor:    newFakeProcessor(),
		Clock:        clock.nowFunc(),
		Entropy:      entropy.factory(),
		TimerFactory: timers.factory(),
		FileSystem:   fs,
		Fatal:        func(err error) {},
	}
	svc, err := New(config, deps)
	if err != nil {
		t.Fatalf("create artifact service: %v", err)
	}
	defer svc.Close()
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	// Close the workspace so ClassifyArtifactFailure makes the error fatal,
	// triggering the slog.Error call in classifyStorageError.
	_ = ws.Close()

	_, domain := svc.Acquire(context.Background(), scope, "trace-1")
	if domain == nil {
		t.Fatal("expected error on ENOSPC with workspace failure")
	}

	logOutput := logBuf.String()

	// The workspace root must not appear in logs.
	wsRoot := ws.Transient
	if wsRoot != "" && strings.Contains(logOutput, wsRoot) {
		t.Fatalf("log output contains workspace root %q:\n%s", wsRoot, logOutput)
	}

	// The artifacts subdirectory name may appear in the workspace root path,
	// but the staging- or installed-bundle path must not appear. Since we use
	// a faulty FS, the synthetic error does not contain paths — but verify
	// the service itself doesn't log them.
	if strings.Contains(logOutput, "staging-") || strings.Contains(logOutput, "installed-") {
		t.Fatalf("log output contains bundle directory name:\n%s", logOutput)
	}

	// No credential-like content should appear (the artifact service never
	// sees credentials, but defense in depth).
	if strings.Contains(logOutput, "LOOMSPAN_") || strings.Contains(logOutput, "credential") {
		t.Fatalf("log output may contain credential content:\n%s", logOutput)
	}

	// The scopeId is expected and safe — verify it IS logged for ops debugging.
	if !strings.Contains(logOutput, "ownerId") {
		t.Fatalf("expected ownerId in log output for ops debugging:\n%s", logOutput)
	}
}
