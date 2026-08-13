package artifact

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/evidence"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/target"
)

// PR12-R01: Use returns ARTIFACT_EXPIRED for a well-formed handle that is not
// installed in the current scope.
func TestUseReturnsArtifactExpiredForUnknownHandle(t *testing.T) {
	data := []byte("test")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	svc := newTestService(t, config, loader, opener)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	// Acquire to get a valid handle.
	artifact := acquireSync(t, svc, context.Background(), scope, "trace-1")

	// Use with a valid handle.
	lease, domain := svc.Use(targetRef(scope.ID), artifact.Handle)
	if domain != nil {
		t.Fatalf("Use failed for valid handle: %v", domain)
	}
	_ = lease.Close(true)

	// Use with a well-formed but unknown handle.
	fakeHandle := Handle(bytes.Repeat([]byte("ab"), handleByteLength))
	_, domain = svc.Use(targetRef(scope.ID), fakeHandle)
	if domain == nil || domain.Code != consolecore.CodeArtifactExpired {
		t.Fatalf("expected ARTIFACT_EXPIRED for unknown handle, got %v", domain)
	}
}

// PR12-R01: Use returns INVALID_ARGUMENT for a malformed handle.
func TestUseReturnsInvalidArgumentForMalformedHandle(t *testing.T) {
	data := []byte("test")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	svc := newTestService(t, config, loader, opener)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	_, domain := svc.Use(targetRef(scope.ID), "not-a-valid-handle")
	if domain == nil || domain.Code != consolecore.CodeInvalidArgument {
		t.Fatalf("expected INVALID_ARGUMENT for malformed handle, got %v", domain)
	}
}

// PR12-R01: Use returns TARGET_CHANGED when the scope has rotated.
func TestUseReturnsTargetChangedAfterScopeRotation(t *testing.T) {
	data := []byte("test")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	svc := newTestService(t, config, loader, opener)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	artifact := acquireSync(t, svc, context.Background(), scope, "trace-1")

	// Rotate scope.
	scope2, cancelScope2 := testScope("scope-2")
	defer cancelScope2()
	svc.ActivateActivity(scope2)

	_, domain := svc.Use(targetRef(scope.ID), artifact.Handle)
	if domain == nil || domain.Code != consolecore.CodeTargetChanged {
		t.Fatalf("expected TARGET_CHANGED after rotation, got %v", domain)
	}
}

// PR12-R05: Remove returns ARTIFACT_IN_USE when the artifact has an active
// lease, without force-cancelling the lease.
func TestRemoveReturnsArtifactInUseWhenLeased(t *testing.T) {
	data := []byte("test")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	svc := newTestService(t, config, loader, opener)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	acquireSync(t, svc, context.Background(), scope, "trace-1")

	lease, domain := svc.Use(targetRef(scope.ID), acquireHandle(t, svc, scope, "trace-1"))
	if domain != nil {
		t.Fatalf("Use failed: %v", domain)
	}
	defer lease.Close(true)

	domain = svc.Remove(targetRef(scope.ID), "trace-1")
	if domain == nil || domain.Code != consolecore.CodeArtifactInUse {
		t.Fatalf("expected ARTIFACT_IN_USE, got %v", domain)
	}

	// After closing the lease, Remove should succeed.
	_ = lease.Close(true)
	domain = svc.Remove(targetRef(scope.ID), "trace-1")
	if domain != nil {
		t.Fatalf("Remove after lease close failed: %v", domain)
	}
}

// PR12-R05: Remove deletes an unused installed artifact.
func TestRemoveDeletesUnusedArtifact(t *testing.T) {
	data := []byte("test")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	svc := newTestService(t, config, loader, opener)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	acquireSync(t, svc, context.Background(), scope, "trace-1")
	domain := svc.Remove(targetRef(scope.ID), "trace-1")
	if domain != nil {
		t.Fatalf("Remove failed: %v", domain)
	}
	snapshot, _ := svc.StorageSnapshot()
	if snapshot.AcquiredCount != 0 {
		t.Fatalf("expected 0 entries after remove, got %d", snapshot.AcquiredCount)
	}
}

// PR12-R05: Remove returns ARTIFACT_EXPIRED for a trace that was never
// acquired or has already been removed.
func TestRemoveReturnsArtifactExpiredForUnknownTrace(t *testing.T) {
	data := []byte("test")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	svc := newTestService(t, config, loader, opener)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	domain := svc.Remove(targetRef(scope.ID), "nonexistent")
	if domain == nil || domain.Code != consolecore.CodeArtifactExpired {
		t.Fatalf("expected ARTIFACT_EXPIRED, got %v", domain)
	}
}

// PR12-R06: ClearAllUnused removes all unused installed entries.
func TestClearAllUnusedRemovesAllUnusedEntries(t *testing.T) {
	data := []byte("test")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 10 << 20, IdleTTL: time.Hour}
	svc := newTestService(t, config, loader, opener)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	acquireSync(t, svc, context.Background(), scope, "trace-1")

	// Acquire a second trace.
	loader2 := newFakeLoader(testTraceMetadata("trace-2", int64(len(data))))
	opener2 := newFakeOpener(data, int64(len(data)))
	svc.traceLoader = loader2.loader()
	svc.streamOpener = opener2.opener()
	acquireSync(t, svc, context.Background(), scope, "trace-2")

	snapshot, _ := svc.StorageSnapshot()
	if snapshot.AcquiredCount != 2 {
		t.Fatalf("expected 2 entries, got %d", snapshot.AcquiredCount)
	}

	domain := svc.ClearAllUnused()
	if domain != nil {
		t.Fatalf("ClearAllUnused failed: %v", domain)
	}
	snapshot, _ = svc.StorageSnapshot()
	if snapshot.AcquiredCount != 0 {
		t.Fatalf("expected 0 entries after ClearAllUnused, got %d", snapshot.AcquiredCount)
	}
}

// PR12-R06: ClearAllUnused preserves pinned entries.
func TestClearAllUnusedPreservesPinnedEntries(t *testing.T) {
	data := []byte("test")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 10 << 20, IdleTTL: time.Hour}
	svc := newTestService(t, config, loader, opener)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	acquireSync(t, svc, context.Background(), scope, "trace-1")
	lease, _ := svc.Use(targetRef(scope.ID), acquireHandle(t, svc, scope, "trace-1"))
	defer lease.Close(true)

	_ = svc.ClearAllUnused()
	snapshot, _ := svc.StorageSnapshot()
	if snapshot.AcquiredCount != 1 {
		t.Fatalf("expected 1 pinned entry preserved, got %d", snapshot.AcquiredCount)
	}
}

// PR12-R07: StorageSnapshot is side-effect-free and does not refresh any
// entry's last-use time.
func TestStorageSnapshotDoesNotRefreshLastUsedAt(t *testing.T) {
	data := []byte("test")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	timers := &manualTimerFactory{}
	clock := newManualClock(time.UnixMilli(1000000))
	svc := newTestServiceWithDeps(t, config, loader, opener, timers, clock, nil)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	artifact := acquireSync(t, svc, context.Background(), scope, "trace-1")
	originalLastUsed := artifact.LastUsedAt

	// Advance the clock.
	clock.advance(10 * time.Minute)

	// Take a snapshot.
	snapshot, _ := svc.StorageSnapshot()
	if len(snapshot.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(snapshot.Entries))
	}
	if !snapshot.Entries[0].LastUsedAt.Equal(originalLastUsed) {
		t.Fatalf("snapshot refreshed lastUsedAt: got %v, expected %v",
			snapshot.Entries[0].LastUsedAt, originalLastUsed)
	}
}

// PR12-R13: StorageSnapshot does not expose the workspace root or installed
// path in entry fields. The snapshot's top-level WorkspaceLabel is a
// display-safe directory name for the Trace Storage UI; it is not a full
// filesystem path. No per-entry field should contain path-like content.
func TestStorageSnapshotDoesNotExposePath(t *testing.T) {
	data := []byte("test")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	svc := newTestService(t, config, loader, opener)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	acquireSync(t, svc, context.Background(), scope, "trace-1")
	snapshot, _ := svc.StorageSnapshot()

	// The top-level WorkspaceLabel is a display-safe directory name, not a
	// full filesystem path. It must not contain path separators or artifact
	// subpath components.
	if snapshot.WorkspaceLabel == "" {
		t.Fatal("expected non-empty WorkspaceLabel for Trace Storage display")
	}
	if strings.ContainsAny(snapshot.WorkspaceLabel, `/\`) {
		t.Fatalf("WorkspaceLabel should be a base name, not a path: %q", snapshot.WorkspaceLabel)
	}
	if strings.Contains(snapshot.WorkspaceLabel, "transient") ||
		strings.Contains(snapshot.WorkspaceLabel, "artifacts") ||
		strings.Contains(snapshot.WorkspaceLabel, "installed") {
		t.Fatalf("WorkspaceLabel should not contain artifact subpath components: %q", snapshot.WorkspaceLabel)
	}

	// No per-entry field should contain path-like content.
	for _, entry := range snapshot.Entries {
		if strings.Contains(entry.TraceID, "transient") ||
			strings.Contains(entry.TraceID, "artifacts") ||
			strings.Contains(entry.TraceID, "installed") {
			t.Fatalf("snapshot entry contains path-like content: %q", entry.TraceID)
		}
	}
}

// PR12-R08: Close cancels transfers, invalidates handles, closes timers, waits
// for workers, and removes owned state.
func TestCloseCancelsTransfersAndRemovesState(t *testing.T) {
	data := []byte("test")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	svc := newTestService(t, config, loader, opener)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	acquireSync(t, svc, context.Background(), scope, "trace-1")
	svc.Close()

	// After close, Use should fail.
	_, domain := svc.Use(targetRef(scope.ID), "anyhandle")
	if domain == nil {
		t.Fatal("Use should fail after close")
	}
}

// PR12-R09: InvalidateTargetScope stops new old-scope work, cancels
// acquisitions and leases, invalidates handles, removes scope content, and
// releases all charges.
func TestInvalidateTargetScopeRemovesOldScopeEntries(t *testing.T) {
	data := []byte("test")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	svc := newTestService(t, config, loader, opener)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	artifact := acquireSync(t, svc, context.Background(), scope, "trace-1")

	// Rotate to a new scope.
	scope2, cancelScope2 := testScope("scope-2")
	defer cancelScope2()
	svc.ActivateActivity(scope2)
	svc.InvalidateTargetScope(scope.ID, scope.Context)

	// Old-scope handle should not be usable.
	_, domain := svc.Use(targetRef(scope.ID), artifact.Handle)
	if domain == nil || domain.Code != consolecore.CodeTargetChanged {
		t.Fatalf("expected TARGET_CHANGED for old-scope handle, got %v", domain)
	}
}

// PR12-R09: InvalidateTargetScope preserves current-scope entries.
func TestInvalidateTargetScopePreservesCurrentScopeEntries(t *testing.T) {
	data := []byte("test")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	svc := newTestService(t, config, loader, opener)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	acquireSync(t, svc, context.Background(), scope, "trace-1")

	// Invalidate a different (non-current) scope.
	svc.InvalidateTargetScope("scope-other", scope.Context)

	// Current-scope entry should still be present.
	snapshot, _ := svc.StorageSnapshot()
	if snapshot.AcquiredCount != 1 {
		t.Fatalf("expected 1 entry after invalidating different scope, got %d", snapshot.AcquiredCount)
	}
}

func TestRemoveFailureRetainsEntryAndCapacityUntilCleanupSucceeds(t *testing.T) {
	data := []byte("retained-after-remove-failure")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	fs := newFaultyFS()
	svc := newTestServiceWithDeps(t, Config{MaxBytes: 1 << 20, IdleTTL: time.Hour},
		loader, opener, &manualTimerFactory{}, newManualClock(time.UnixMilli(1000000)), fs)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)
	acquireSync(t, svc, context.Background(), scope, "trace-1")

	var fatalCalled bool
	svc.fatal = func(error) { fatalCalled = true }
	fs.removeAllFail = errors.New("remove bundle denied")
	domain := svc.Remove(targetRef(scope.ID), "trace-1")
	if domain == nil || domain.Code != consolecore.CodeConsoleError {
		t.Fatalf("expected CONSOLE_ERROR, got %#v", domain)
	}
	if !fatalCalled {
		t.Fatal("expected persistent cleanup failure to reach the fatal path")
	}
	if svc.totalCharged != int64(len(data))+fakeDerivedSize() {
		t.Fatalf("failed removal released capacity: got %d", svc.totalCharged)
	}
	if _, exists := svc.entries[entryKey{owner: evidence.Target(scope.ID), traceID: "trace-1"}]; !exists {
		t.Fatal("failed removal discarded the owned entry")
	}
	fs.removeFail = nil
}

// acquireHandle acquires a trace and returns its handle, failing the test on
// error.
func acquireHandle(t *testing.T, svc *Service, scope target.Scope, traceID string) Handle {
	t.Helper()
	artifact, domain := svc.Acquire(context.Background(), scope, traceID)
	if domain != nil {
		t.Fatalf("acquire failed: %v", domain)
	}
	return artifact.Handle
}
