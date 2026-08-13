package artifact

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
)

// PR12-R09: InvalidateTargetScope cancels acquisitions and leases for the
// old scope before removing content. Acquisitions in progress are cancelled
// and their partial files cleaned up.
func TestInvalidateTargetScopeCancelsAcquisitionsAndLeases(t *testing.T) {
	data := bytes.Repeat([]byte("x"), 4096)
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	loader.barrier = make(chan struct{})
	loader.release = make(chan struct{})
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	svc := newTestService(t, config, loader, opener)
	scope, cancelScope := testScope("scope-1")
	svc.ActivateActivity(scope)

	// Start an acquisition that blocks at the loader barrier.
	done := make(chan error, 1)
	go func() {
		_, err := svc.Acquire(context.Background(), scope, "trace-1")
		done <- err
	}()
	<-loader.barrier

	// Rotate scope while the acquisition is in progress.
	scope2, cancelScope2 := testScope("scope-2")
	defer cancelScope2()
	svc.ActivateActivity(scope2)
	svc.InvalidateTargetScope(scope.ID, scope.Context)

	// The acquisition should return with an error (TARGET_CHANGED).
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error after scope rotation during acquisition")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("acquisition did not return after scope invalidation")
	}

	// Clean up the scope.
	cancelScope()
	close(loader.release)

	// No old-scope entries should remain.
	snapshot, _ := svc.StorageSnapshot()
	if snapshot.AcquiredCount != 0 {
		t.Fatalf("expected 0 entries for new scope, got %d", snapshot.AcquiredCount)
	}
}

// PR12-R09: InvalidateTargetScope waits for acquisition goroutines to exit
// before returning. This ensures bounded cleanup.
func TestInvalidateTargetScopeWaitsForAcquisitionGoroutines(t *testing.T) {
	data := bytes.Repeat([]byte("x"), 2048)
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	loader.barrier = make(chan struct{})
	loader.release = make(chan struct{})
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	svc := newTestService(t, config, loader, opener)
	scope, cancelScope := testScope("scope-1")
	svc.ActivateActivity(scope)

	// Start an acquisition that blocks at the loader barrier.
	done := make(chan error, 1)
	go func() {
		_, err := svc.Acquire(context.Background(), scope, "trace-1")
		done <- err
	}()
	<-loader.barrier

	// Capture the acquireFinished channel.
	waitForWaiters(t, svc, "trace-1", 1, 2*time.Second)
	var finishChan chan struct{}
	svc.mu.Lock()
	for _, entry := range svc.entries {
		if entry.key.traceID == "trace-1" {
			finishChan = entry.acquireFinished
			break
		}
	}
	svc.mu.Unlock()
	if finishChan == nil {
		t.Fatal("could not find acquireFinished channel")
	}

	// Rotate scope — InvalidateTargetScope should cancel the acquisition and
	// wait for the goroutine to exit.
	scope2, cancelScope2 := testScope("scope-2")
	defer cancelScope2()
	svc.ActivateActivity(scope2)

	invalidated := make(chan struct{})
	go func() {
		svc.InvalidateTargetScope(scope.ID, scope.Context)
		close(invalidated)
	}()

	// InvalidateTargetScope should return in bounded time.
	select {
	case <-invalidated:
	case <-time.After(2 * time.Second):
		t.Fatal("InvalidateTargetScope did not return in bounded time")
	}

	// The acquisition goroutine should have exited.
	select {
	case <-finishChan:
	case <-time.After(2 * time.Second):
		t.Fatal("acquisition goroutine did not exit after invalidation")
	}

	cancelScope()
	close(loader.release)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("acquisition did not return")
	}
}

// PR12-R09: ActivateActivity sets the current scope ID so that Use, Remove,
// and StorageSnapshot can validate the caller's scope.
func TestActivateActivitySetsCurrentScopeID(t *testing.T) {
	data := []byte("activate-test")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	svc := newTestService(t, config, loader, opener)

	// Before activation, Use should return TARGET_CHANGED (currentScopeID is empty).
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	_, domain := svc.Use(targetRef(scope.ID), "anyhandle")
	if domain == nil || domain.Code != "TARGET_CHANGED" {
		t.Fatalf("expected TARGET_CHANGED before activation, got %v", domain)
	}

	// After activation, the scope ID is set.
	svc.ActivateActivity(scope)
	svc.mu.Lock()
	current := svc.currentScopeID
	svc.mu.Unlock()
	if current != scope.ID {
		t.Fatalf("expected currentScopeID %q, got %q", scope.ID, current)
	}
}

// PR12-R09: Restart cleanup never adopts prior-process entries. The artifacts
// directory is created fresh and empty.
func TestRestartCleanupNeverAdoptsPriorEntries(t *testing.T) {
	data := []byte("restart-test")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	svc := newTestService(t, config, loader, opener)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	// Acquire an artifact.
	acquireSync(t, svc, context.Background(), scope, "trace-1")
	snapshot, _ := svc.StorageSnapshot()
	if snapshot.AcquiredCount != 1 {
		t.Fatalf("expected 1 entry before restart, got %d", snapshot.AcquiredCount)
	}

	// Close the service (simulating shutdown).
	svc.Close()

	// Create a new service with the same workspace. The new service should
	// start with no entries, no charges, and no handles.
	svc2, err := New(config, Dependencies{
		Lifetime:     context.Background(),
		Workspace:    svc.workspace,
		TraceLoader:  loader.loader(),
		StreamOpener: opener.opener(),
		Processor:    newFakeProcessor(),
		Clock:        time.Now,
		Entropy:      (&deterministicEntropy{}).factory(),
		TimerFactory: (&manualTimerFactory{}).factory(),
	})
	if err != nil {
		t.Fatalf("create second artifact service: %v", err)
	}
	defer svc2.Close()
	svc2.ActivateActivity(scope)

	snapshot2, _ := svc2.StorageSnapshot()
	if snapshot2.AcquiredCount != 0 {
		t.Fatalf("expected 0 entries after restart, got %d", snapshot2.AcquiredCount)
	}
	if snapshot2.ChargedBytes != 0 {
		t.Fatalf("expected 0 charged bytes after restart, got %d", snapshot2.ChargedBytes)
	}
}

// Ensure that concurrent ActivateActivity and InvalidateTargetScope calls
// don't race or deadlock.
func TestConcurrentActivateAndInvalidateNoDeadlock(t *testing.T) {
	data := []byte("concurrent-owner-test")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	svc := newTestService(t, config, loader, opener)

	const iterations = 50
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			scope, cancel := testScope("scope-A")
			svc.ActivateActivity(scope)
			cancel()
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			svc.InvalidateTargetScope("scope-A", context.Background())
		}
	}()

	wg.Wait()
}

func TestAcquireRejectsPreviouslyCapturedScopeAfterNewScopeActivation(t *testing.T) {
	data := []byte("scope-bound")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	svc := newTestService(t, Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}, loader, opener)
	oldScope, cancelOld := testScope("scope-1")
	defer cancelOld()
	svc.ActivateActivity(oldScope)
	acquireSync(t, svc, context.Background(), oldScope, "trace-1")

	newScope, cancelNew := testScope("scope-2")
	defer cancelNew()
	svc.ActivateActivity(newScope)

	_, domain := svc.Acquire(context.Background(), oldScope, "trace-1")
	if domain == nil || domain.Code != consolecore.CodeTargetChanged {
		t.Fatalf("expected TARGET_CHANGED for stale scope acquisition, got %v", domain)
	}
	if loader.callCount() != 1 || opener.callCount() != 1 {
		t.Fatal("stale scope acquisition reached upstream work")
	}
}
