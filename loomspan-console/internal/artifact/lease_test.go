package artifact

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/evidence"
)

// PR12-R12: Lease.Open returns a reader over the installed artifact file
// without exposing the path.
func TestLeaseOpenReturnsReaderWithoutPath(t *testing.T) {
	data := []byte("lease-open-test-data")
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

	reader, err := lease.OpenComponent(ComponentRawArtifact)
	if err != nil {
		t.Fatalf("OpenComponent failed: %v", err)
	}
	defer reader.Close()

	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("expected %d bytes, got %d bytes", len(data), len(got))
	}
}

// PR12-R12: Lease.Close is idempotent.
func TestLeaseCloseIsIdempotent(t *testing.T) {
	data := []byte("idempotent-close-test")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	svc := newTestService(t, config, loader, opener)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	acquireSync(t, svc, context.Background(), scope, "trace-1")
	lease, _ := svc.Use(targetRef(scope.ID), acquireHandle(t, svc, scope, "trace-1"))

	if err := lease.Close(true); err != nil {
		t.Fatalf("first Close failed: %v", err)
	}
	if err := lease.Close(true); err != nil {
		t.Fatalf("second Close failed: %v", err)
	}
	if err := lease.Close(false); err != nil {
		t.Fatalf("third Close failed: %v", err)
	}
}

// PR12-R12: Lease.Open fails after Close.
func TestLeaseOpenFailsAfterClose(t *testing.T) {
	data := []byte("open-after-close-test")
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
		t.Fatal("expected error opening after close")
	}
}

// PR12-R12: Multiple leases can be issued for the same artifact.
func TestMultipleLeasesForSameArtifact(t *testing.T) {
	data := []byte("multi-lease-test-data")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 1 << 20, IdleTTL: time.Hour}
	svc := newTestService(t, config, loader, opener)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	acquireSync(t, svc, context.Background(), scope, "trace-1")
	handle := acquireHandle(t, svc, scope, "trace-1")

	lease1, _ := svc.Use(targetRef(scope.ID), handle)
	lease2, _ := svc.Use(targetRef(scope.ID), handle)

	// Both leases should be able to read the data.
	reader1, _ := lease1.OpenComponent(ComponentRawArtifact)
	got1, _ := io.ReadAll(reader1)
	reader1.Close()
	if !bytes.Equal(got1, data) {
		t.Fatalf("lease1 got wrong data")
	}

	reader2, _ := lease2.OpenComponent(ComponentRawArtifact)
	got2, _ := io.ReadAll(reader2)
	reader2.Close()
	if !bytes.Equal(got2, data) {
		t.Fatalf("lease2 got wrong data")
	}

	_ = lease1.Close(true)
	_ = lease2.Close(true)
}

// PR25: Lease.Owner returns the evidence owner the lease was issued for.
func TestLeaseOwner(t *testing.T) {
	data := []byte("scope-id-test")
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

	if lease.Owner() != evidence.Target(scope.ID) {
		t.Fatalf("expected target owner for %q, got %#v", scope.ID, lease.Owner())
	}
}

// T6: A lease remains usable after Remove returns ARTIFACT_IN_USE.
func TestLeaseStillUsableAfterFailedRemove(t *testing.T) {
	data := []byte("lease-after-remove-test")
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

	// Remove should fail with ARTIFACT_IN_USE because the lease is active.
	domain := svc.Remove(targetRef(scope.ID), "trace-1")
	if domain == nil || domain.Code != consolecore.CodeArtifactInUse {
		t.Fatalf("expected ARTIFACT_IN_USE, got %v", domain)
	}

	// The lease should still be able to open and read the artifact.
	reader, err := lease.OpenComponent(ComponentRawArtifact)
	if err != nil {
		t.Fatalf("OpenComponent failed after failed Remove: %v", err)
	}
	defer reader.Close()
	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll failed after failed Remove: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("expected %d bytes, got %d bytes", len(data), len(got))
	}
}
