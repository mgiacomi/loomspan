package artifact

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/applicationclient"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/target"
)

// PR12-R02: Simultaneous acquisition joins one metadata load, upstream stream,
// installation, and capacity charge.
func TestAcquireJoinsConcurrentWaitersIntoOneInstalledArtifact(t *testing.T) {
	data := bytes.Repeat([]byte("x"), 4096)
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	loader.barrier = make(chan struct{})
	loader.release = make(chan struct{})
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	svc := newTestService(t, config, loader, opener)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	const waiters = 5
	var wg sync.WaitGroup
	results := make([]AcquiredArtifact, waiters)
	errs := make([]*consolecore.Error, waiters)
	started := make(chan struct{}, 1)
	wg.Add(waiters)
	for i := 0; i < waiters; i++ {
		go func(idx int) {
			defer wg.Done()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			select {
			case started <- struct{}{}:
			default:
			}
			results[idx], errs[idx] = svc.Acquire(ctx, scope, "trace-1")
		}(i)
	}
	<-started
	<-loader.barrier
	close(loader.release)
	wg.Wait()

	if loader.callCount() != 1 {
		t.Fatalf("expected 1 metadata load, got %d", loader.callCount())
	}
	if opener.callCount() != 1 {
		t.Fatalf("expected 1 upstream stream, got %d", opener.callCount())
	}
	for i, err := range errs {
		if err != nil {
			t.Fatalf("waiter %d failed: %v", i, err)
		}
	}
	firstHandle := results[0].Handle
	if firstHandle == "" {
		t.Fatal("expected non-empty handle")
	}
	for i, result := range results[1:] {
		if result.Handle != firstHandle {
			t.Fatalf("waiter %d got different handle %q vs %q", i+1, result.Handle, firstHandle)
		}
	}
	snapshot, domain := svc.StorageSnapshot()
	if domain != nil {
		t.Fatalf("snapshot failed: %v", domain)
	}
	if snapshot.AcquiredCount != 1 {
		t.Fatalf("expected 1 installed entry, got %d", snapshot.AcquiredCount)
	}
	if snapshot.ChargedBytes != int64(len(data))+fakeDerivedSize() {
		t.Fatalf("expected charged bytes %d, got %d", int64(len(data))+fakeDerivedSize(), snapshot.ChargedBytes)
	}
}

// PR12-R01: A successful already-installed lookup returns the same handle
// without an upstream call.
func TestAcquireReturnsSameHandleForAlreadyInstalledTrace(t *testing.T) {
	data := []byte("test artifact bytes")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	svc := newTestService(t, config, loader, opener)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	first := acquireSync(t, svc, context.Background(), scope, "trace-1")
	loaderCalls := loader.callCount()
	openerCalls := opener.callCount()

	second := acquireSync(t, svc, context.Background(), scope, "trace-1")
	if second.Handle != first.Handle {
		t.Fatalf("expected same handle %q, got %q", first.Handle, second.Handle)
	}
	if loader.callCount() != loaderCalls {
		t.Fatalf("already-installed lookup should not load metadata, got %d additional calls", loader.callCount()-loaderCalls)
	}
	if opener.callCount() != openerCalls {
		t.Fatalf("already-installed lookup should not open stream, got %d additional calls", opener.callCount()-openerCalls)
	}
}

// PR12-R03: Cancelling one waiter does not cancel the leader; the remaining
// waiter succeeds.
func TestAcquireCancelsOneWaiterWithoutCancellingLeader(t *testing.T) {
	data := bytes.Repeat([]byte("y"), 2048)
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	loader.barrier = make(chan struct{})
	loader.release = make(chan struct{})
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	svc := newTestService(t, config, loader, opener)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	// Start two waiters: one will be cancelled, one will remain.
	cancelCtx, cancelWaiter := context.WithCancel(context.Background())
	cancelledDone := make(chan *consolecore.Error, 1)
	remainingDone := make(chan *consolecore.Error, 1)
	go func() {
		_, err := svc.Acquire(cancelCtx, scope, "trace-1")
		cancelledDone <- err
	}()
	go func() {
		_, err := svc.Acquire(context.Background(), scope, "trace-1")
		remainingDone <- err
	}()
	<-loader.barrier
	// Wait until both waiters have registered before cancelling one.
	// Otherwise the first waiter could be the only waiter and cancelling
	// it would cancel the leader.
	waitForWaiters(t, svc, "trace-1", 2, 2*time.Second)
	cancelWaiter()
	select {
	case err := <-cancelledDone:
		if err == nil {
			t.Fatal("cancelled waiter should have received an error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled waiter did not return")
	}

	// The remaining waiter should still succeed.
	close(loader.release)
	select {
	case err := <-remainingDone:
		if err != nil {
			t.Fatalf("remaining waiter failed: code=%q message=%q scope=%q", err.Code, err.Message, err.TargetScopeID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("remaining waiter did not return")
	}
}

// PR12-R03: Cancelling the final waiter abandons and cleans the partial
// transfer.
func TestAcquireCancelsLeaderAndCleansWhenLastWaiterLeaves(t *testing.T) {
	data := bytes.Repeat([]byte("z"), 2048)
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	loader.barrier = make(chan struct{})
	loader.release = make(chan struct{})
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	svc := newTestService(t, config, loader, opener)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	cancelCtx, cancelWaiter := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := svc.Acquire(cancelCtx, scope, "trace-1")
		done <- err
	}()
	<-loader.barrier
	// Capture the acquireFinished channel before cancelling so we can wait
	// deterministically for the leader goroutine to exit.
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
		t.Fatal("could not find acquireFinished channel for trace-1")
	}
	cancelWaiter()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("waiter did not return after cancellation")
	}

	// Wait for the acquisition goroutine to finish deterministically.
	select {
	case <-finishChan:
	case <-time.After(2 * time.Second):
		t.Fatal("acquisition goroutine did not exit after final-waiter cancellation")
	}

	// No entry should exist; no capacity should be charged.
	snapshot, domain := svc.StorageSnapshot()
	if domain != nil {
		t.Fatalf("snapshot failed: %v", domain)
	}
	if snapshot.AcquiredCount != 0 {
		t.Fatalf("expected 0 entries after final-waiter cancellation, got %d", snapshot.AcquiredCount)
	}
	if snapshot.ChargedBytes != 0 {
		t.Fatalf("expected 0 charged bytes, got %d", snapshot.ChargedBytes)
	}

	// A new acquisition should succeed. The loader barrier was already
	// consumed by the first call, so we use a fresh loader/opener.
	loader2 := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener2 := newFakeOpener(data, int64(len(data)))
	svc.traceLoader = loader2.loader()
	svc.streamOpener = opener2.opener()
	artifact, domain := svc.Acquire(context.Background(), scope, "trace-1")
	if domain != nil {
		t.Fatalf("new acquisition failed: %v", domain)
	}
	if artifact.Handle == "" {
		t.Fatal("expected non-empty handle")
	}
}

func TestAcquireFinalWaiterCancellationBeforePublicationRemovesInstalledFile(t *testing.T) {
	data := []byte("cancel-after-copy-before-publication")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	fs := newBlockingRenameFS()
	svc := newTestServiceWithDeps(t, Config{MaxBytes: 1 << 20, IdleTTL: time.Hour},
		loader, opener, &manualTimerFactory{}, newManualClock(time.UnixMilli(1000000)), fs)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan *consolecore.Error, 1)
	go func() {
		_, domain := svc.Acquire(ctx, scope, "trace-1")
		done <- domain
	}()
	<-fs.entered
	cancel()
	domain := <-done
	if domain == nil {
		t.Fatal("expected canceled waiter to return an error")
	}
	close(fs.release)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		entries, charged := len(svc.entries), svc.totalCharged
		svc.mu.Unlock()
		if entries == 0 && charged == 0 {
			files, err := os.ReadDir(svc.storage.dir)
			if err != nil {
				t.Fatal(err)
			}
			if len(files) != 0 {
				t.Fatalf("canceled publication left files: %v", files)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("canceled publication retained an entry or capacity charge")
}

// PR12-R04: A handle is published only after sync, close, size verification,
// and atomic rename.
func TestAcquirePublishesOnlyAfterSyncCloseSizeAndAtomicRename(t *testing.T) {
	t.Run("sync failure prevents handle", func(t *testing.T) {
		data := []byte("sync-fail-test")
		loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
		opener := newFakeOpener(data, int64(len(data)))
		fs := newFaultyFS()
		fs.syncFail = errors.New("sync failed")
		config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
		timers := &manualTimerFactory{}
		clock := newManualClock(time.UnixMilli(1000000))
		svc := newTestServiceWithDeps(t, config, loader, opener, timers, clock, fs)
		scope, cancelScope := testScope("scope-1")
		defer cancelScope()
		svc.ActivateActivity(scope)

		_, domain := svc.Acquire(context.Background(), scope, "trace-1")
		if domain == nil {
			t.Fatal("expected error on sync failure")
		}
		if domain.Code != consolecore.CodeLocalStorageUnavailable && domain.Code != consolecore.CodeConsoleError {
			t.Fatalf("expected LOCAL_STORAGE_UNAVAILABLE or CONSOLE_ERROR, got %s", domain.Code)
		}
		snapshot, _ := svc.StorageSnapshot()
		if snapshot.AcquiredCount != 0 {
			t.Fatalf("expected 0 entries after sync failure, got %d", snapshot.AcquiredCount)
		}
	})

	t.Run("close failure prevents handle", func(t *testing.T) {
		data := []byte("close-fail-test")
		loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
		opener := newFakeOpener(data, int64(len(data)))
		fs := newFaultyFS()
		fs.closeFail = errors.New("close failed")
		config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
		timers := &manualTimerFactory{}
		clock := newManualClock(time.UnixMilli(1000000))
		svc := newTestServiceWithDeps(t, config, loader, opener, timers, clock, fs)
		scope, cancelScope := testScope("scope-1")
		defer cancelScope()
		svc.ActivateActivity(scope)

		_, domain := svc.Acquire(context.Background(), scope, "trace-1")
		if domain == nil {
			t.Fatal("expected error on close failure")
		}
		snapshot, _ := svc.StorageSnapshot()
		if snapshot.AcquiredCount != 0 {
			t.Fatalf("expected 0 entries after close failure, got %d", snapshot.AcquiredCount)
		}
	})

	t.Run("rename failure prevents handle", func(t *testing.T) {
		data := []byte("rename-fail-test")
		loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
		opener := newFakeOpener(data, int64(len(data)))
		fs := newFaultyFS()
		fs.renameFail = errors.New("rename failed")
		config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
		timers := &manualTimerFactory{}
		clock := newManualClock(time.UnixMilli(1000000))
		svc := newTestServiceWithDeps(t, config, loader, opener, timers, clock, fs)
		scope, cancelScope := testScope("scope-1")
		defer cancelScope()
		svc.ActivateActivity(scope)

		_, domain := svc.Acquire(context.Background(), scope, "trace-1")
		if domain == nil {
			t.Fatal("expected error on rename failure")
		}
		snapshot, _ := svc.StorageSnapshot()
		if snapshot.AcquiredCount != 0 {
			t.Fatalf("expected 0 entries after rename failure, got %d", snapshot.AcquiredCount)
		}
	})

	t.Run("size mismatch returns INVALID_ARTIFACT", func(t *testing.T) {
		data := []byte("short")
		loader := newFakeLoader(testTraceMetadata("trace-1", 100)) // metadata says 100 bytes
		opener := newFakeOpener(data, 100)                         // declared 100 but only 5 bytes
		config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
		svc := newTestService(t, config, loader, opener)
		scope, cancelScope := testScope("scope-1")
		defer cancelScope()
		svc.ActivateActivity(scope)

		_, domain := svc.Acquire(context.Background(), scope, "trace-1")
		if domain == nil {
			t.Fatal("expected error on size mismatch")
		}
		if domain.Code != consolecore.CodeInvalidArtifact {
			t.Fatalf("expected INVALID_ARTIFACT, got %s", domain.Code)
		}
		if domain.Details.RawDownloadAvailable == nil || !*domain.Details.RawDownloadAvailable {
			t.Fatal("expected rawDownloadAvailable to be true")
		}
		snapshot, _ := svc.StorageSnapshot()
		if snapshot.AcquiredCount != 0 {
			t.Fatalf("expected 0 entries after size mismatch, got %d", snapshot.AcquiredCount)
		}
	})
}

// PR12-R04: Rejects short, long, failed, or stale transfers without publishing
// a handle.
func TestAcquireRejectsShortLongFailedOrStaleTransferWithoutHandle(t *testing.T) {
	t.Run("metadata load failure", func(t *testing.T) {
		loader := newFakeLoader(testTraceMetadata("trace-1", 100))
		loader.err = consolecore.NewError(consolecore.CodeNotFound, "trace not found", "scope-1", consolecore.Details{}, nil)
		opener := newFakeOpener(nil, 0)
		config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
		svc := newTestService(t, config, loader, opener)
		scope, cancelScope := testScope("scope-1")
		defer cancelScope()
		svc.ActivateActivity(scope)

		_, domain := svc.Acquire(context.Background(), scope, "trace-1")
		if domain == nil || domain.Code != consolecore.CodeNotFound {
			t.Fatalf("expected NOT_FOUND, got %v", domain)
		}
		snapshot, _ := svc.StorageSnapshot()
		if snapshot.AcquiredCount != 0 {
			t.Fatalf("expected 0 entries, got %d", snapshot.AcquiredCount)
		}
	})

	t.Run("stream open failure", func(t *testing.T) {
		loader := newFakeLoader(testTraceMetadata("trace-1", 100))
		opener := newFakeOpener(nil, 0)
		opener.err = consolecore.NewError(consolecore.CodeTargetAuthentication, "auth failed", "scope-1", consolecore.Details{}, nil)
		config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
		svc := newTestService(t, config, loader, opener)
		scope, cancelScope := testScope("scope-1")
		defer cancelScope()
		svc.ActivateActivity(scope)

		_, domain := svc.Acquire(context.Background(), scope, "trace-1")
		if domain == nil || domain.Code != consolecore.CodeTargetAuthentication {
			t.Fatalf("expected TARGET_AUTHENTICATION_REQUIRED, got %v", domain)
		}
		snapshot, _ := svc.StorageSnapshot()
		if snapshot.AcquiredCount != 0 {
			t.Fatalf("expected 0 entries, got %d", snapshot.AcquiredCount)
		}
	})

	t.Run("scope rotation during transfer", func(t *testing.T) {
		data := bytes.Repeat([]byte("r"), 2048)
		loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
		loader.barrier = make(chan struct{})
		loader.release = make(chan struct{})
		opener := newFakeOpener(data, int64(len(data)))
		config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
		svc := newTestService(t, config, loader, opener)
		scope, cancelScope := testScope("scope-1")
		svc.ActivateActivity(scope)

		done := make(chan error, 1)
		go func() {
			_, err := svc.Acquire(context.Background(), scope, "trace-1")
			done <- err
		}()
		<-loader.barrier
		cancelScope()
		svc.InvalidateTargetScope(scope.ID, scope.Context)
		close(loader.release)
		select {
		case err := <-done:
			if err == nil {
				t.Fatal("expected error after scope rotation")
			}
		case <-time.After(2 * time.Second):
			t.Fatal("acquisition did not return after scope rotation")
		}
		snapshot, _ := svc.StorageSnapshot()
		if snapshot.AcquiredCount != 0 {
			t.Fatalf("expected 0 entries after scope rotation, got %d", snapshot.AcquiredCount)
		}
	})
}

// PR12-R13: No handle, DTO, or log contains the workspace root or installed
// path.
func TestAcquireHandleDoesNotContainPath(t *testing.T) {
	data := []byte("path-leak-test")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	svc := newTestService(t, config, loader, opener)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	artifact := acquireSync(t, svc, context.Background(), scope, "trace-1")
	if strings.Contains(string(artifact.Handle), "transient") ||
		strings.Contains(string(artifact.Handle), "artifacts") ||
		strings.Contains(string(artifact.Handle), "installed") {
		t.Fatalf("handle contains path-like content: %q", artifact.Handle)
	}
	snapshot, _ := svc.StorageSnapshot()
	for _, entry := range snapshot.Entries {
		if strings.Contains(entry.TraceID, "transient") {
			t.Fatalf("snapshot entry contains path: %q", entry.TraceID)
		}
	}
}

// guardedReadCloser serves data in chunks and can block to coordinate tests.
type guardedReadCloser struct {
	data    []byte
	offset  int
	chunk   int
	closed  atomic.Bool
	blockAt chan struct{}
	release chan struct{}
}

func newGuardedReadCloser(data []byte, chunk int) *guardedReadCloser {
	return &guardedReadCloser{
		data:    data,
		chunk:   chunk,
		release: make(chan struct{}),
	}
}

func (r *guardedReadCloser) Read(p []byte) (int, error) {
	if r.closed.Load() {
		return 0, io.ErrClosedPipe
	}
	if r.offset >= len(r.data) {
		return 0, io.EOF
	}
	if r.blockAt != nil && r.offset >= r.chunk {
		select {
		case <-r.blockAt:
		case <-r.release:
			r.closed.Store(true)
			return 0, context.Canceled
		}
	}
	end := r.offset + r.chunk
	if end > len(r.data) {
		end = len(r.data)
	}
	n := copy(p, r.data[r.offset:end])
	r.offset += n
	return n, nil
}

func (r *guardedReadCloser) Close() error {
	r.closed.Store(true)
	return nil
}

// TestAcquireUnknownLengthStreamChargesIncrementally verifies that
// unknown-length streams are charged incrementally and still install
// correctly.
func TestAcquireUnknownLengthStreamChargesIncrementally(t *testing.T) {
	data := bytes.Repeat([]byte("u"), 3<<20)                 // 3MB, exceeds incrementalReserveBytes
	loader := newFakeLoader(testTraceMetadata("trace-1", 0)) // metadata size unknown
	opener := newFakeOpener(data, -1)                        // declared length unknown
	config := Config{MaxBytes: 10 << 20, IdleTTL: time.Hour}
	svc := newTestService(t, config, loader, opener)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	artifact := acquireSync(t, svc, context.Background(), scope, "trace-1")
	if artifact.LocalBytes != int64(len(data))+fakeDerivedSize() {
		t.Fatalf("expected local bytes %d, got %d", int64(len(data))+fakeDerivedSize(), artifact.LocalBytes)
	}
	snapshot, _ := svc.StorageSnapshot()
	if snapshot.ChargedBytes != int64(len(data))+fakeDerivedSize() {
		t.Fatalf("expected charged bytes %d, got %d", int64(len(data))+fakeDerivedSize(), snapshot.ChargedBytes)
	}
}

// A positive metadata size remains the maximum writable reservation even when
// the transport omits Content-Length. Extra bytes are rejected before write.
func TestAcquireRejectsUndeclaredTransferBeyondKnownMetadataSize(t *testing.T) {
	data := bytes.Repeat([]byte("x"), 200)
	loader := newFakeLoader(testTraceMetadata("trace-1", 100))
	opener := newFakeOpener(data, -1)
	config := Config{MaxBytes: 100, IdleTTL: time.Hour}
	svc := newTestService(t, config, loader, opener)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	_, domain := svc.Acquire(context.Background(), scope, "trace-1")
	if domain == nil || domain.Code != consolecore.CodeInvalidArtifact {
		t.Fatalf("expected INVALID_ARTIFACT, got %v", domain)
	}
	snapshot, snapshotDomain := svc.StorageSnapshot()
	if snapshotDomain != nil {
		t.Fatal(snapshotDomain)
	}
	if snapshot.ChargedBytes != 0 || snapshot.AcquiredCount != 0 {
		t.Fatalf("failed overrun retained state: %+v", snapshot)
	}
	entries, err := os.ReadDir(svc.storage.dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("failed overrun left %d workspace files", len(entries))
	}
}

// TestAcquireZeroByteArtifactInstallsWhenTransportAgrees verifies that a
// complete zero-byte body is installed if its transport and metadata agree.
func TestAcquireZeroByteArtifactInstallsWhenTransportAgrees(t *testing.T) {
	loader := newFakeLoader(testTraceMetadata("trace-1", 0))
	opener := newFakeOpener(nil, 0)
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	svc := newTestService(t, config, loader, opener)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	artifact := acquireSync(t, svc, context.Background(), scope, "trace-1")
	if artifact.LocalBytes != fakeDerivedSize() {
		t.Fatalf("expected %d local bytes (derived only), got %d", fakeDerivedSize(), artifact.LocalBytes)
	}
	snapshot, _ := svc.StorageSnapshot()
	if snapshot.AcquiredCount != 1 {
		t.Fatalf("expected 1 entry, got %d", snapshot.AcquiredCount)
	}
}

// TestAcquireShortWriteFails verifies that a short write during streaming
// is handled as a storage error.
func TestAcquireShortWriteFails(t *testing.T) {
	data := bytes.Repeat([]byte("s"), 4096)
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	fs := newFaultyFS()
	fs.shortAt = 100 // short write after 100 bytes
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	timers := &manualTimerFactory{}
	clock := newManualClock(time.UnixMilli(1000000))
	svc := newTestServiceWithDeps(t, config, loader, opener, timers, clock, fs)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	_, domain := svc.Acquire(context.Background(), scope, "trace-1")
	if domain == nil {
		t.Fatal("expected error on short write")
	}
	snapshot, _ := svc.StorageSnapshot()
	if snapshot.AcquiredCount != 0 {
		t.Fatalf("expected 0 entries after short write, got %d", snapshot.AcquiredCount)
	}
}

// TestAcquireENOSPCInUnlimitedModeReturnsLocalStorageUnavailable verifies
// that disk-full in unlimited mode maps to LOCAL_STORAGE_UNAVAILABLE, not
// LIMIT_EXCEEDED.
func TestAcquireENOSPCInUnlimitedModeReturnsLocalStorageUnavailable(t *testing.T) {
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
	if domain.Code == consolecore.CodeLimitExceeded {
		t.Fatal("unlimited mode should not return LIMIT_EXCEEDED for disk-full")
	}
}

type enospcError struct{}

func (enospcError) Error() string { return "no space left on device" }

// T1: A stream that provides more bytes than the declared length is rejected
// with INVALID_ARTIFACT.
func TestAcquireRejectsLongTransfer(t *testing.T) {
	data := bytes.Repeat([]byte("x"), 200)
	loader := newFakeLoader(testTraceMetadata("trace-1", 200))
	opener := newFakeOpener(data, 100) // declared 100 but stream has 200
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	svc := newTestService(t, config, loader, opener)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	_, domain := svc.Acquire(context.Background(), scope, "trace-1")
	if domain == nil || domain.Code != consolecore.CodeInvalidArtifact {
		t.Fatalf("expected INVALID_ARTIFACT for long transfer, got %v", domain)
	}
	snapshot, _ := svc.StorageSnapshot()
	if snapshot.AcquiredCount != 0 {
		t.Fatalf("expected 0 entries after long transfer rejection, got %d", snapshot.AcquiredCount)
	}
}

// T5: Calling Close while an acquisition is blocked at the loader barrier
// cancels the acquisition and returns CONSOLE_ERROR.
func TestAcquireShutdownDuringInFlightAcquisition(t *testing.T) {
	data := []byte("shutdown-during-acquire")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	loader.barrier = make(chan struct{})
	loader.release = make(chan struct{})
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	svc := newTestService(t, config, loader, opener)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	started := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		close(started)
		_, domain := svc.Acquire(context.Background(), scope, "trace-1")
		if domain == nil {
			t.Error("expected error on shutdown during acquisition")
		}
	}()
	<-started
	waitForWaiters(t, svc, "trace-1", 1, 2*time.Second)
	svc.Close()
	wg.Wait()
}

// PR12-R03: Cancelling one waiter during the copy phase (after the stream is
// open and bytes are flowing) does not cancel the leader. The remaining waiter
// still receives the installed artifact.
func TestAcquireCancelsOneWaiterDuringCopyWithoutCancellingLeader(t *testing.T) {
	data := bytes.Repeat([]byte("c"), 4096)
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	svc := newTestService(t, config, loader, opener)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	// Replace the opener with one that serves a guarded reader blocking at
	// 1024 bytes so the copy pauses mid-stream.
	guarded := newGuardedReadCloser(data, 1024)
	guarded.blockAt = make(chan struct{})
	customOpener := func(ctx context.Context, scope target.Scope, traceID string) (*applicationclient.ArtifactStream, *consolecore.Error) {
		return applicationclient.NewTestArtifactStream(guarded, scope.InstanceID, int64(len(data))), nil
	}
	svc.streamOpener = customOpener

	cancelCtx, cancelWaiter := context.WithCancel(context.Background())
	cancelledDone := make(chan *consolecore.Error, 1)
	remainingDone := make(chan *consolecore.Error, 1)

	go func() {
		_, err := svc.Acquire(cancelCtx, scope, "trace-1")
		cancelledDone <- err
	}()
	// Wait for the copy to reach the block point (1024 bytes written).
	waitForWaiters(t, svc, "trace-1", 1, 2*time.Second)

	go func() {
		_, err := svc.Acquire(context.Background(), scope, "trace-1")
		remainingDone <- err
	}()
	waitForWaiters(t, svc, "trace-1", 2, 2*time.Second)

	// Cancel the first waiter while the copy is blocked.
	cancelWaiter()
	select {
	case err := <-cancelledDone:
		if err == nil {
			t.Fatal("cancelled waiter should have received an error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled waiter did not return")
	}

	// Unblock the stream so the remaining waiter can finish.
	close(guarded.blockAt)
	select {
	case err := <-remainingDone:
		if err != nil {
			t.Fatalf("remaining waiter failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("remaining waiter did not return")
	}

	// The artifact should be installed.
	snapshot, _ := svc.StorageSnapshot()
	if snapshot.AcquiredCount != 1 {
		t.Fatalf("expected 1 installed entry, got %d", snapshot.AcquiredCount)
	}
}

// PR12-R03: A guarded reader that serves data in small chunks and blocks
// between chunks completes successfully when not cancelled. This proves
// backpressure from a slow upstream does not break the copy loop.
func TestAcquireGuardedReaderBackpressureCompletesSuccessfully(t *testing.T) {
	data := bytes.Repeat([]byte("b"), 8192)
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	svc := newTestService(t, config, loader, newFakeOpener(nil, 0))
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	// Serve data in 512-byte chunks with a guarded reader. The blockAt
	// channel is never closed, so the reader never blocks — it just serves
	// small chunks to simulate a slow upstream.
	guarded := newGuardedReadCloser(data, 512)
	customOpener := func(ctx context.Context, scope target.Scope, traceID string) (*applicationclient.ArtifactStream, *consolecore.Error) {
		return applicationclient.NewTestArtifactStream(guarded, scope.InstanceID, int64(len(data))), nil
	}
	svc.streamOpener = customOpener

	artifact := acquireSync(t, svc, context.Background(), scope, "trace-1")
	if artifact.LocalBytes != int64(len(data))+fakeDerivedSize() {
		t.Fatalf("expected local bytes %d, got %d", int64(len(data))+fakeDerivedSize(), artifact.LocalBytes)
	}
	snapshot, _ := svc.StorageSnapshot()
	if snapshot.AcquiredCount != 1 {
		t.Fatalf("expected 1 entry, got %d", snapshot.AcquiredCount)
	}
	if snapshot.ChargedBytes != int64(len(data))+fakeDerivedSize() {
		t.Fatalf("expected charged bytes %d, got %d", int64(len(data))+fakeDerivedSize(), snapshot.ChargedBytes)
	}
}
