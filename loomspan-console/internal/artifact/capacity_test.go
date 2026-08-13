package artifact

import (
	"bytes"
	"context"
	"math"
	"testing"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
)

func TestCapacityArithmeticCannotOverflow(t *testing.T) {
	service := &Service{
		capacity:       capacityConfig{maxBytes: math.MaxInt64},
		totalCharged:   1,
		entries:        make(map[entryKey]*entry),
		ttlNeverExpire: true,
	}

	domain := service.reserveCapacity(math.MaxInt64)
	if domain == nil || domain.Code != consolecore.CodeInvalidArtifact {
		t.Fatalf("expected INVALID_ARTIFACT instead of overflow admission, got %v", domain)
	}
}

// PR12-R10: Acquire fails with LIMIT_EXCEEDED when the artifact exceeds the
// configured capacity.
func TestCapacityRejectsArtifactExceedingMaxBytes(t *testing.T) {
	data := bytes.Repeat([]byte("x"), 100)
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 50, IdleTTL: time.Hour} // smaller than the artifact
	svc := newTestService(t, config, loader, opener)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	_, domain := svc.Acquire(context.Background(), scope, "trace-1")
	if domain == nil || domain.Code != consolecore.CodeLimitExceeded {
		t.Fatalf("expected LIMIT_EXCEEDED, got %v", domain)
	}
	if domain.Details.LimitName != limitNameMaxBytes {
		t.Fatalf("expected limit name %q, got %q", limitNameMaxBytes, domain.Details.LimitName)
	}
	if domain.Details.LimitValue != 50 {
		t.Fatalf("expected limit value 50, got %d", domain.Details.LimitValue)
	}
}

// PR12-R10: Acquire evicts LRU unpinned installed entries to make room.
func TestCapacityEvictsLRUUnpinnedToMakeRoom(t *testing.T) {
	data := bytes.Repeat([]byte("x"), 100)
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 150, IdleTTL: time.Hour} // fits one, not two
	timers := &manualTimerFactory{}
	clock := newManualClock(time.UnixMilli(1000000))
	svc := newTestServiceWithDeps(t, config, loader, opener, timers, clock, nil)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	// Acquire trace-1.
	acquireSync(t, svc, context.Background(), scope, "trace-1")
	clock.advance(time.Minute)

	// Acquire trace-2, which should evict trace-1.
	loader2 := newFakeLoader(testTraceMetadata("trace-2", int64(len(data))))
	opener2 := newFakeOpener(data, int64(len(data)))
	svc.traceLoader = loader2.loader()
	svc.streamOpener = opener2.opener()
	acquireSync(t, svc, context.Background(), scope, "trace-2")

	snapshot, _ := svc.StorageSnapshot()
	if snapshot.AcquiredCount != 1 {
		t.Fatalf("expected 1 entry after eviction, got %d", snapshot.AcquiredCount)
	}
	if snapshot.Entries[0].TraceID != "trace-2" {
		t.Fatalf("expected trace-2 to remain, got %q", snapshot.Entries[0].TraceID)
	}
}

// PR12-R10: Acquire does not evict pinned entries to make room.
func TestCapacityDoesNotEvictPinnedEntries(t *testing.T) {
	data := bytes.Repeat([]byte("x"), 100)
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 150, IdleTTL: time.Hour}
	timers := &manualTimerFactory{}
	clock := newManualClock(time.UnixMilli(1000000))
	svc := newTestServiceWithDeps(t, config, loader, opener, timers, clock, nil)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	// Acquire and pin trace-1.
	acquireSync(t, svc, context.Background(), scope, "trace-1")
	lease, _ := svc.Use(targetRef(scope.ID), acquireHandle(t, svc, scope, "trace-1"))
	defer lease.Close(true)

	// Acquire trace-2, which should fail because trace-1 is pinned.
	loader2 := newFakeLoader(testTraceMetadata("trace-2", int64(len(data))))
	opener2 := newFakeOpener(data, int64(len(data)))
	svc.traceLoader = loader2.loader()
	svc.streamOpener = opener2.opener()
	_, domain := svc.Acquire(context.Background(), scope, "trace-2")
	if domain == nil || domain.Code != consolecore.CodeLimitExceeded {
		t.Fatalf("expected LIMIT_EXCEEDED when pinned entry cannot be evicted, got %v", domain)
	}
}

// PR12-R10: Unlimited mode skips capacity checks; disk-full is handled
// separately.
func TestCapacityUnlimitedSkipsChecks(t *testing.T) {
	data := bytes.Repeat([]byte("x"), 1<<20)
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 100, Unlimited: true, IdleTTL: time.Hour}
	svc := newTestService(t, config, loader, opener)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	artifact := acquireSync(t, svc, context.Background(), scope, "trace-1")
	if artifact.LocalBytes != int64(len(data))+fakeDerivedSize() {
		t.Fatalf("expected local bytes %d, got %d", int64(len(data))+fakeDerivedSize(), artifact.LocalBytes)
	}
}

// PR12-R10: Capacity charge is the exact installed byte count, not the
// metadata-declared size.
func TestCapacityChargesExactInstalledBytes(t *testing.T) {
	data := bytes.Repeat([]byte("x"), 500)
	loader := newFakeLoader(testTraceMetadata("trace-1", 1000)) // metadata says 1000
	opener := newFakeOpener(data, 1000)                         // declared 1000 but only 500 bytes
	config := Config{MaxBytes: 2000, IdleTTL: time.Hour}
	svc := newTestService(t, config, loader, opener)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	// The size mismatch should cause INVALID_ARTIFACT, so no charge.
	_, domain := svc.Acquire(context.Background(), scope, "trace-1")
	if domain == nil {
		t.Fatal("expected error for size mismatch")
	}

	// Now test with matching sizes.
	loader2 := newFakeLoader(testTraceMetadata("trace-2", 500))
	opener2 := newFakeOpener(data, 500)
	svc.traceLoader = loader2.loader()
	svc.streamOpener = opener2.opener()
	acquireSync(t, svc, context.Background(), scope, "trace-2")

	snapshot, _ := svc.StorageSnapshot()
	if snapshot.ChargedBytes != 500+fakeDerivedSize() {
		t.Fatalf("expected charged bytes %d, got %d", 500+fakeDerivedSize(), snapshot.ChargedBytes)
	}
}

// T2: When two entries have identical lastUsedAt, LRU tie-breaking is
// deterministic: the older acquisition time is evicted first.
func TestCapacityLRUTieBreakIsDeterministic(t *testing.T) {
	data := bytes.Repeat([]byte("x"), 75)
	loader1 := newFakeLoader(testTraceMetadata("trace-a", int64(len(data))))
	opener1 := newFakeOpener(data, int64(len(data)))
	// Each bundle is 75 raw + fakeDerivedSize() derived. Two bundles fit
	// exactly; a third forces eviction.
	perEntry := int64(len(data)) + fakeDerivedSize()
	config := Config{MaxBytes: perEntry * 2, IdleTTL: time.Hour}
	timers := &manualTimerFactory{}
	clock := newManualClock(time.UnixMilli(1000000))
	svc := newTestServiceWithDeps(t, config, loader1, opener1, timers, clock, nil)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	// Acquire trace-a at t=1000000.
	acquireSync(t, svc, context.Background(), scope, "trace-a")

	// Acquire trace-b at t=1000001 (one tick later).
	clock.advance(time.Millisecond)
	loader2 := newFakeLoader(testTraceMetadata("trace-b", int64(len(data))))
	opener2 := newFakeOpener(data, int64(len(data)))
	svc.traceLoader = loader2.loader()
	svc.streamOpener = opener2.opener()
	acquireSync(t, svc, context.Background(), scope, "trace-b")

	// Now advance the clock and refresh both entries' lastUsedAt to the same time.
	clock.advance(time.Minute)
	handleA := acquireHandle(t, svc, scope, "trace-a")
	leaseA, _ := svc.Use(targetRef(scope.ID), handleA)
	_ = leaseA.Close(true)

	handleB := acquireHandle(t, svc, scope, "trace-b")
	leaseB, _ := svc.Use(targetRef(scope.ID), handleB)
	_ = leaseB.Close(true)

	// Both entries now have the same lastUsedAt. Acquire trace-c.
	// 2*perEntry (existing) + perEntry (new) > MaxBytes, so one entry must be
	// evicted. trace-a (older acquisition time) is evicted by the tie-breaker.
	loader3 := newFakeLoader(testTraceMetadata("trace-c", int64(len(data))))
	opener3 := newFakeOpener(data, int64(len(data)))
	svc.traceLoader = loader3.loader()
	svc.streamOpener = opener3.opener()
	acquireSync(t, svc, context.Background(), scope, "trace-c")

	snapshot, _ := svc.StorageSnapshot()
	if snapshot.AcquiredCount != 2 {
		t.Fatalf("expected 2 entries after eviction, got %d", snapshot.AcquiredCount)
	}
	traceIDs := map[string]bool{}
	for _, e := range snapshot.Entries {
		traceIDs[e.TraceID] = true
	}
	if traceIDs["trace-a"] {
		t.Fatal("expected trace-a (older acquisition) to be evicted, but it remains")
	}
	if !traceIDs["trace-b"] || !traceIDs["trace-c"] {
		t.Fatalf("expected trace-b and trace-c to remain, got %v", traceIDs)
	}
}

// T3: Expired entries are removed before LRU eviction during capacity
// reservation.
func TestCapacityEvictsExpiredBeforeLRU(t *testing.T) {
	data := bytes.Repeat([]byte("x"), 75)
	loader1 := newFakeLoader(testTraceMetadata("trace-a", int64(len(data))))
	opener1 := newFakeOpener(data, int64(len(data)))
	// Each bundle is 75 raw + fakeDerivedSize() derived. Two bundles fit
	// exactly; a third forces eviction.
	perEntry := int64(len(data)) + fakeDerivedSize()
	config := Config{MaxBytes: perEntry * 2, IdleTTL: 5 * time.Minute}
	timers := &manualTimerFactory{}
	clock := newManualClock(time.UnixMilli(1000000))
	svc := newTestServiceWithDeps(t, config, loader1, opener1, timers, clock, nil)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	// Acquire trace-a.
	acquireSync(t, svc, context.Background(), scope, "trace-a")
	clock.advance(time.Minute)

	// Acquire trace-b.
	loader2 := newFakeLoader(testTraceMetadata("trace-b", int64(len(data))))
	opener2 := newFakeOpener(data, int64(len(data)))
	svc.traceLoader = loader2.loader()
	svc.streamOpener = opener2.opener()
	acquireSync(t, svc, context.Background(), scope, "trace-b")

	// Advance past trace-a's TTL (acquired at t=1000000, TTL=5min).
	// trace-b was acquired at t=1000000+1min, so its deadline is
	// t=1000000+6min. Advance to t=1000000+5min30s so trace-a is
	// expired but trace-b is not.
	clock.advance(4*time.Minute + 30*time.Second)

	// Acquire trace-c. 2*perEntry (existing) + perEntry (new) > MaxBytes.
	// trace-a is expired and should be evicted first (before LRU trace-b).
	// After evicting trace-a: perEntry + perEntry = 2*perEntry ≤ MaxBytes,
	// so trace-b survives.
	loader3 := newFakeLoader(testTraceMetadata("trace-c", int64(len(data))))
	opener3 := newFakeOpener(data, int64(len(data)))
	svc.traceLoader = loader3.loader()
	svc.streamOpener = opener3.opener()
	acquireSync(t, svc, context.Background(), scope, "trace-c")

	snapshot, _ := svc.StorageSnapshot()
	if snapshot.AcquiredCount != 2 {
		t.Fatalf("expected 2 entries after expired eviction, got %d", snapshot.AcquiredCount)
	}
	traceIDs := map[string]bool{}
	for _, e := range snapshot.Entries {
		traceIDs[e.TraceID] = true
	}
	if traceIDs["trace-a"] {
		t.Fatal("expected expired trace-a to be evicted, but it remains")
	}
	if !traceIDs["trace-b"] || !traceIDs["trace-c"] {
		t.Fatalf("expected trace-b and trace-c to remain, got %v", traceIDs)
	}
}

// T7: An artifact that exactly fills the remaining capacity succeeds.
func TestCapacityExactFitSucceeds(t *testing.T) {
	data50 := bytes.Repeat([]byte("x"), 50)
	loader1 := newFakeLoader(testTraceMetadata("trace-a", 50))
	opener1 := newFakeOpener(data50, 50)
	// Each bundle is 50 raw + fakeDerivedSize() derived. Two bundles fit
	// exactly.
	perEntry := int64(50) + fakeDerivedSize()
	config := Config{MaxBytes: perEntry * 2, IdleTTL: time.Hour}
	timers := &manualTimerFactory{}
	clock := newManualClock(time.UnixMilli(1000000))
	svc := newTestServiceWithDeps(t, config, loader1, opener1, timers, clock, nil)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	// Acquire trace-a, leaving exactly perEntry bytes of capacity.
	acquireSync(t, svc, context.Background(), scope, "trace-a")

	// Acquire trace-b — exactly fits.
	data50b := bytes.Repeat([]byte("y"), 50)
	loader2 := newFakeLoader(testTraceMetadata("trace-b", 50))
	opener2 := newFakeOpener(data50b, 50)
	svc.traceLoader = loader2.loader()
	svc.streamOpener = opener2.opener()
	artifact := acquireSync(t, svc, context.Background(), scope, "trace-b")
	if artifact.LocalBytes != perEntry {
		t.Fatalf("expected %d local bytes, got %d", perEntry, artifact.LocalBytes)
	}

	snapshot, _ := svc.StorageSnapshot()
	if snapshot.AcquiredCount != 2 {
		t.Fatalf("expected 2 entries (exact fit), got %d", snapshot.AcquiredCount)
	}
	if snapshot.ChargedBytes != perEntry*2 {
		t.Fatalf("expected charged bytes %d, got %d", perEntry*2, snapshot.ChargedBytes)
	}
}
