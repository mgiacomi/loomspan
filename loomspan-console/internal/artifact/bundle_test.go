package artifact

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
)

type processorFunc func(ProcessRequest) (ProcessResult, *consolecore.Error)

func (fn processorFunc) Process(req ProcessRequest) (ProcessResult, *consolecore.Error) {
	return fn(req)
}

func TestAcquisitionRejectsInconsistentProcessorBundles(t *testing.T) {
	writeComponent := func(req ProcessRequest, sync, closeWriter bool) ComponentWriter {
		writer, domain := req.Sink.Create(req.Context, fakeProcessorComponent)
		if domain != nil {
			t.Fatalf("Create failed: %v", domain)
		}
		if _, err := writer.Write([]byte("derived")); err != nil {
			t.Fatalf("Write failed: %v", err)
		}
		if sync {
			if err := writer.Sync(); err != nil {
				t.Fatalf("Sync failed: %v", err)
			}
		}
		if closeWriter {
			_ = writer.Close()
		}
		return writer
	}
	tests := map[string]processorFunc{
		"misreported size": func(req ProcessRequest) (ProcessResult, *consolecore.Error) {
			writeComponent(req, true, true)
			return ProcessResult{ComponentSizes: map[ComponentName]int64{fakeProcessorComponent: 999}}, nil
		},
		"ignored unsynced close": func(req ProcessRequest) (ProcessResult, *consolecore.Error) {
			writeComponent(req, false, true)
			return ProcessResult{ComponentSizes: map[ComponentName]int64{}}, nil
		},
		"leaked writer": func(req ProcessRequest) (ProcessResult, *consolecore.Error) {
			writeComponent(req, true, false)
			return ProcessResult{ComponentSizes: map[ComponentName]int64{}}, nil
		},
		"ignored duplicate create": func(req ProcessRequest) (ProcessResult, *consolecore.Error) {
			writeComponent(req, true, true)
			_, _ = req.Sink.Create(req.Context, fakeProcessorComponent)
			return ProcessResult{ComponentSizes: map[ComponentName]int64{fakeProcessorComponent: 7}}, nil
		},
	}
	for name, processor := range tests {
		t.Run(name, func(t *testing.T) {
			data := []byte("raw")
			svc := newTestServiceWithProcessor(t, Config{MaxBytes: 1 << 20, IdleTTL: time.Hour},
				newFakeLoader(testTraceMetadata("trace-1", int64(len(data)))), newFakeOpener(data, int64(len(data))),
				&manualTimerFactory{}, newManualClock(time.UnixMilli(1000000)), nil, processor)
			scope, cancel := testScope("scope-1")
			defer cancel()
			svc.ActivateActivity(scope)
			_, domain := svc.Acquire(context.Background(), scope, "trace-1")
			if domain == nil || domain.Code != consolecore.CodeConsoleError {
				t.Fatalf("expected CONSOLE_ERROR, got %v", domain)
			}
			snapshot, _ := svc.StorageSnapshot()
			if snapshot.AcquiredCount != 0 || snapshot.ChargedBytes != 0 || bundleDirCount(t, svc) != 0 {
				t.Fatalf("inconsistent processor bundle leaked state: %+v", snapshot)
			}
		})
	}
}

// TestAcquisitionInstallsBundleWithRawAndDerivedComponents proves the installed
// bundle contains both the raw artifact component and the processor's derived
// component, and that both are addressable through a lease without exposing the
// filesystem path (PR13-P2-R01).
func TestAcquisitionInstallsBundleWithRawAndDerivedComponents(t *testing.T) {
	data := []byte("bundle-test-raw-data")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	processor := newFakeProcessor()
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	svc := newTestServiceWithProcessor(t, config, loader, opener,
		&manualTimerFactory{}, newManualClock(time.UnixMilli(1000000)), nil, processor)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	acquired := acquireSync(t, svc, context.Background(), scope, "trace-1")
	lease, domain := svc.Use(targetRef(scope.ID), acquired.Handle)
	if domain != nil {
		t.Fatalf("Use failed: %v", domain)
	}
	defer lease.Close(true)

	// The raw component must contain the exact upstream bytes.
	rawBody := readLeasedComponent(t, lease, ComponentRawArtifact)
	if !bytes.Equal(rawBody, data) {
		t.Fatalf("raw component mismatch: expected %d bytes, got %d bytes", len(data), len(rawBody))
	}

	// The derived component must contain the fake processor's bytes.
	derivedBody := readLeasedComponent(t, lease, fakeProcessorComponent)
	if !bytes.Equal(derivedBody, processor.derivedBytes) {
		t.Fatalf("derived component mismatch: expected %q, got %q", processor.derivedBytes, derivedBody)
	}

	// ComponentSize must report the correct sizes for both components.
	rawSize, err := lease.ComponentSize(ComponentRawArtifact)
	if err != nil {
		t.Fatalf("ComponentSize(raw) failed: %v", err)
	}
	if rawSize != int64(len(data)) {
		t.Fatalf("expected raw size %d, got %d", len(data), rawSize)
	}
	derivedSize, err := lease.ComponentSize(fakeProcessorComponent)
	if err != nil {
		t.Fatalf("ComponentSize(derived) failed: %v", err)
	}
	if derivedSize != int64(len(processor.derivedBytes)) {
		t.Fatalf("expected derived size %d, got %d", len(processor.derivedBytes), derivedSize)
	}
}

// TestAcquisitionChargesAggregateRawPlusDerivedBytes proves the capacity charge
// is the sum of the raw and derived component byte counts (PR13-P2-R02).
func TestAcquisitionChargesAggregateRawPlusDerivedBytes(t *testing.T) {
	data := []byte("aggregate-charge-test-data")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	processor := newFakeProcessor()
	processor.derivedBytes = bytes.Repeat([]byte("d"), 100)
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	svc := newTestServiceWithProcessor(t, config, loader, opener,
		&manualTimerFactory{}, newManualClock(time.UnixMilli(1000000)), nil, processor)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	acquired := acquireSync(t, svc, context.Background(), scope, "trace-1")
	expected := int64(len(data)) + int64(len(processor.derivedBytes))
	if acquired.LocalBytes != expected {
		t.Fatalf("expected aggregate local bytes %d, got %d", expected, acquired.LocalBytes)
	}

	snapshot, _ := svc.StorageSnapshot()
	if snapshot.ChargedBytes != expected {
		t.Fatalf("expected aggregate charged bytes %d, got %d", expected, snapshot.ChargedBytes)
	}
}

// TestProcessorFailureRemovesBundleAndPublishesNoHandle proves that when the
// processor rejects the raw artifact, the entire staged bundle is removed, no
// handle is published, and the capacity charge is released (PR13-P2-R03).
func TestProcessorFailureRemovesBundleAndPublishesNoHandle(t *testing.T) {
	data := []byte("processor-failure-test-data")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	processor := newFakeProcessor()
	processor.err = consolecore.NewError(consolecore.CodeInvalidArtifact,
		"The trace artifact is malformed.", "scope-1", consolecore.Details{}, nil)
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	svc := newTestServiceWithProcessor(t, config, loader, opener,
		&manualTimerFactory{}, newManualClock(time.UnixMilli(1000000)), nil, processor)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	_, domain := svc.Acquire(context.Background(), scope, "trace-1")
	if domain == nil || domain.Code != consolecore.CodeInvalidArtifact {
		t.Fatalf("expected INVALID_ARTIFACT from processor, got %v", domain)
	}

	// No entry, no charge, no bundle on disk.
	snapshot, _ := svc.StorageSnapshot()
	if snapshot.AcquiredCount != 0 || snapshot.ChargedBytes != 0 {
		t.Fatalf("processor failure retained state: %+v", snapshot)
	}
	if bundleDirCount(t, svc) != 0 {
		t.Fatalf("expected 0 bundle dirs after processor failure, got %d", bundleDirCount(t, svc))
	}
}

// TestProcessorFailureReleasesDerivedCapacity proves that derived bytes charged
// by the sink before the processor returns an error are refunded (PR13-P2-R04).
func TestProcessorFailureReleasesDerivedCapacity(t *testing.T) {
	data := []byte("derived-refund-test-data")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	processor := newFakeProcessor()
	processor.err = consolecore.NewError(consolecore.CodeInvalidArtifact,
		"reject after derived write", "scope-1", consolecore.Details{}, nil)
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	svc := newTestServiceWithProcessor(t, config, loader, opener,
		&manualTimerFactory{}, newManualClock(time.UnixMilli(1000000)), nil, processor)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	_, domain := svc.Acquire(context.Background(), scope, "trace-1")
	if domain == nil {
		t.Fatal("expected processor failure")
	}

	// Total charged must be 0: both raw and derived bytes are refunded.
	svc.mu.Lock()
	charged := svc.totalCharged
	svc.mu.Unlock()
	if charged != 0 {
		t.Fatalf("expected 0 charged bytes after processor failure, got %d", charged)
	}
}

// TestLeaseOpenComponentRejectsUnknownName proves OpenComponent returns an error
// for a component name that is not part of the bundle (PR13-P2-R05).
func TestLeaseOpenComponentRejectsUnknownName(t *testing.T) {
	data := []byte("unknown-component-test")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	svc := newTestService(t, config, loader, opener)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	acquireSync(t, svc, context.Background(), scope, "trace-1")
	lease, _ := svc.Use(targetRef(scope.ID), acquireHandle(t, svc, scope, "trace-1"))
	defer lease.Close(true)

	_, err := lease.OpenComponent("nonexistent-component")
	if err == nil {
		t.Fatal("expected error for unknown component name")
	}
}

// TestLeaseOpenComponentRejectsPathSeparator proves a component name with a path
// separator is rejected, preventing path traversal (PR13-P2-R06).
func TestLeaseOpenComponentRejectsPathSeparator(t *testing.T) {
	data := []byte("path-traversal-test")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	svc := newTestService(t, config, loader, opener)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	acquireSync(t, svc, context.Background(), scope, "trace-1")
	lease, _ := svc.Use(targetRef(scope.ID), acquireHandle(t, svc, scope, "trace-1"))
	defer lease.Close(true)

	for _, name := range []ComponentName{"../escape", "sub/dir", "with\\backslash"} {
		_, err := lease.OpenComponent(name)
		if err == nil {
			t.Fatalf("expected error for path-traversal component name %q", name)
		}
	}
}

// TestRemoveClearsEntireBundleDirectory proves Remove eliminates the entire
// bundle directory (raw + derived components), not just the raw file
// (PR13-P2-R07).
func TestRemoveClearsEntireBundleDirectory(t *testing.T) {
	data := []byte("remove-bundle-test-data")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	svc := newTestService(t, config, loader, opener)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	acquireSync(t, svc, context.Background(), scope, "trace-1")
	if bundleDirCount(t, svc) != 1 {
		t.Fatalf("expected 1 bundle dir before remove, got %d", bundleDirCount(t, svc))
	}

	if domain := svc.Remove(targetRef(scope.ID), "trace-1"); domain != nil {
		t.Fatalf("Remove failed: %v", domain)
	}

	if bundleDirCount(t, svc) != 0 {
		t.Fatalf("expected 0 bundle dirs after remove, got %d", bundleDirCount(t, svc))
	}
	if bundleFileCount(t, svc) != 0 {
		t.Fatalf("expected 0 files after remove, got %d", bundleFileCount(t, svc))
	}
}

// TestShutdownClearsAllBundleDirectories proves Close removes every bundle
// directory under the artifacts directory (PR13-P2-R08).
func TestShutdownClearsAllBundleDirectories(t *testing.T) {
	data := []byte("shutdown-bundle-test-data")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	svc := newTestService(t, config, loader, opener)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	acquireSync(t, svc, context.Background(), scope, "trace-1")
	if bundleDirCount(t, svc) != 1 {
		t.Fatalf("expected 1 bundle dir before shutdown, got %d", bundleDirCount(t, svc))
	}

	svc.Close()

	if bundleDirCount(t, svc) != 0 {
		t.Fatalf("expected 0 bundle dirs after shutdown, got %d", bundleDirCount(t, svc))
	}
}

// TestLeaseComponentReaderIsSeekable proves the component reader supports Seek,
// which is required for Phase 3 index-based access patterns (PR13-P2-R09).
func TestLeaseComponentReaderIsSeekable(t *testing.T) {
	data := bytes.Repeat([]byte("seekable-raw-data-"), 10)
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	svc := newTestService(t, config, loader, opener)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	acquireSync(t, svc, context.Background(), scope, "trace-1")
	lease, _ := svc.Use(targetRef(scope.ID), acquireHandle(t, svc, scope, "trace-1"))
	defer lease.Close(true)

	reader, err := lease.OpenComponent(ComponentRawArtifact)
	if err != nil {
		t.Fatalf("OpenComponent failed: %v", err)
	}
	defer reader.Close()

	// Seek to the middle of the component.
	mid := int64(len(data) / 2)
	pos, err := reader.Seek(mid, io.SeekStart)
	if err != nil {
		t.Fatalf("Seek failed: %v", err)
	}
	if pos != mid {
		t.Fatalf("expected seek position %d, got %d", mid, pos)
	}

	// Read from the middle.
	buf := make([]byte, 10)
	n, err := io.ReadFull(reader, buf)
	if err != nil || n != 10 {
		t.Fatalf("ReadFull from mid failed: n=%d err=%v", n, err)
	}
	expected := data[mid : mid+10]
	if !bytes.Equal(buf, expected) {
		t.Fatalf("expected %q, got %q", expected, buf)
	}
}

// TestProcessorRunsOncePerAcquisition proves the processor is invoked exactly
// once per joined acquisition, not once per waiter (PR13-P2-R10).
func TestProcessorRunsOncePerAcquisition(t *testing.T) {
	data := []byte("processor-once-test-data")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	processor := newFakeProcessor()
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	svc := newTestServiceWithProcessor(t, config, loader, opener,
		&manualTimerFactory{}, newManualClock(time.UnixMilli(1000000)), nil, processor)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	// Acquire the same trace multiple times (joined acquisition).
	for i := 0; i < 5; i++ {
		acquireSync(t, svc, context.Background(), scope, "trace-1")
	}

	if processor.callCount() != 1 {
		t.Fatalf("expected processor to run once, got %d", processor.callCount())
	}
}

// TestSinkRejectsRawArtifactComponentName proves the sink rejects attempts to
// create a component named ComponentRawArtifact, which is owned by the
// acquisition leader (PR13-P2-R11).
func TestSinkRejectsRawArtifactComponentName(t *testing.T) {
	data := []byte("raw-name-rejection-test")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	processor := &fakeProcessor{
		derivedName:  ComponentRawArtifact, // attempt to overwrite raw
		derivedBytes: []byte("evil"),
	}
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	svc := newTestServiceWithProcessor(t, config, loader, opener,
		&manualTimerFactory{}, newManualClock(time.UnixMilli(1000000)), nil, processor)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	_, domain := svc.Acquire(context.Background(), scope, "trace-1")
	if domain == nil {
		t.Fatal("expected error when processor tries to create raw component name")
	}
}

// TestLeaseOpenComponentFailsAfterClose proves OpenComponent fails after the
// lease is closed (PR13-P2-R12).
func TestLeaseOpenComponentFailsAfterClose(t *testing.T) {
	data := []byte("open-after-close-bundle-test")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	svc := newTestService(t, config, loader, opener)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	acquireSync(t, svc, context.Background(), scope, "trace-1")
	lease, _ := svc.Use(targetRef(scope.ID), acquireHandle(t, svc, scope, "trace-1"))
	_ = lease.Close(true)

	_, err := lease.OpenComponent(ComponentRawArtifact)
	if err == nil {
		t.Fatal("expected error opening component after lease close")
	}
}

// TestLeaseMultipleComponentsConcurrent proves multiple component readers from
// the same lease can read independently without interfering (PR13-P2-R13).
func TestLeaseMultipleComponentsConcurrent(t *testing.T) {
	data := []byte("multi-component-concurrent-test")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	svc := newTestService(t, config, loader, opener)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	acquireSync(t, svc, context.Background(), scope, "trace-1")
	lease, _ := svc.Use(targetRef(scope.ID), acquireHandle(t, svc, scope, "trace-1"))
	defer lease.Close(true)

	rawReader1, _ := lease.OpenComponent(ComponentRawArtifact)
	rawReader2, _ := lease.OpenComponent(ComponentRawArtifact)
	defer rawReader1.Close()
	defer rawReader2.Close()

	body1, _ := io.ReadAll(rawReader1)
	body2, _ := io.ReadAll(rawReader2)
	if !bytes.Equal(body1, data) || !bytes.Equal(body2, data) {
		t.Fatal("concurrent component readers returned mismatched data")
	}
}

// TestProcessorCancellationDuringRunRemovesBundle proves that when the
// acquisition context is cancelled during processor execution, the staged bundle
// is removed and no handle is published (PR13-P2-R14).
func TestProcessorCancellationDuringRunRemovesBundle(t *testing.T) {
	data := []byte("processor-cancel-during-run-test")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	processor := newFakeProcessor()
	processor.barrier = make(chan struct{})
	processor.release = make(chan struct{})
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	svc := newTestServiceWithProcessor(t, config, loader, opener,
		&manualTimerFactory{}, newManualClock(time.UnixMilli(1000000)), nil, processor)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	done := make(chan *consolecore.Error, 1)
	go func() {
		_, domain := svc.Acquire(context.Background(), scope, "trace-1")
		done <- domain
	}()

	// Wait for the processor to enter, then invalidate the scope to cancel
	// the acquisition context.
	<-processor.barrier
	svc.InvalidateTargetScope(scope.ID, scope.Context)
	close(processor.release)

	select {
	case domain := <-done:
		if domain == nil {
			t.Fatal("expected error after cancellation during processor run")
		}
		// Scope rotation must surface TARGET_CHANGED, not the processor's
		// generic TARGET_UNAVAILABLE/LOCAL_STORAGE_UNAVAILABLE.
		if domain.Code != consolecore.CodeTargetChanged {
			t.Fatalf("expected TARGET_CHANGED after scope rotation during processor run, got %s", domain.Code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("acquisition did not return after cancellation")
	}

	// The bundle must be removed.
	if bundleDirCount(t, svc) != 0 {
		t.Fatalf("expected 0 bundle dirs after cancellation, got %d", bundleDirCount(t, svc))
	}
}

// TestValidateComponentNameRejectsInvalidNames proves the component name
// validator rejects empty, traversal, and path-separator names.
func TestValidateComponentNameRejectsInvalidNames(t *testing.T) {
	for _, name := range []ComponentName{"", ".", "..", "a/b", "a\\b", "a:b"} {
		if domain := validateComponentName(name); domain == nil {
			t.Fatalf("expected error for component name %q", name)
		}
	}
	for _, name := range []ComponentName{"valid", "manifest.json", "index-1.bin"} {
		if domain := validateComponentName(name); domain != nil {
			t.Fatalf("unexpected error for valid component name %q: %v", name, domain)
		}
	}
}

// TestAcquisitionBundleDirectoryIsRandom proves the installed bundle directory
// name is not derived from the handle, trace ID, or scope ID.
func TestAcquisitionBundleDirectoryIsRandom(t *testing.T) {
	data := []byte("random-bundle-dir-test")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	svc := newTestService(t, config, loader, opener)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	acquired := acquireSync(t, svc, context.Background(), scope, "trace-1")

	// The bundle directory name must not contain the handle, trace ID, or
	// scope ID.
	entries, err := os.ReadDir(svc.storage.dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.Contains(name, string(acquired.Handle)) {
			t.Fatalf("bundle dir name %q contains the handle", name)
		}
		if strings.Contains(name, "trace-1") {
			t.Fatalf("bundle dir name %q contains the trace ID", name)
		}
		if strings.Contains(name, string(scope.ID)) {
			t.Fatalf("bundle dir name %q contains the scope ID", name)
		}
	}
}
