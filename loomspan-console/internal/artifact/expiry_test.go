package artifact

import (
	"context"
	"testing"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
)

// PR12-R11: An idle artifact is removed after the configured idle TTL.
func TestExpiryRemovesIdleArtifactAfterTTL(t *testing.T) {
	data := []byte("test")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 1 << 20, IdleTTL: 5 * time.Minute}
	timers := &manualTimerFactory{}
	clock := newManualClock(time.UnixMilli(1000000))
	svc := newTestServiceWithDeps(t, config, loader, opener, timers, clock, nil)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	acquireSync(t, svc, context.Background(), scope, "trace-1")
	snapshot, _ := svc.StorageSnapshot()
	if snapshot.AcquiredCount != 1 {
		t.Fatalf("expected 1 entry, got %d", snapshot.AcquiredCount)
	}

	// Advance past the TTL and fire the timer.
	clock.advance(6 * time.Minute)
	timers.fireAll()

	snapshot, _ = svc.StorageSnapshot()
	if snapshot.AcquiredCount != 0 {
		t.Fatalf("expected 0 entries after TTL expiry, got %d", snapshot.AcquiredCount)
	}
}

func TestExpiryIsEnforcedOnUseAtExactDeadlineBeforeTimerRuns(t *testing.T) {
	data := []byte("test")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	timers := &manualTimerFactory{}
	clock := newManualClock(time.UnixMilli(1000000))
	svc := newTestServiceWithDeps(t, Config{MaxBytes: 1 << 20, IdleTTL: 5 * time.Minute}, loader, opener, timers, clock, nil)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	acquired := acquireSync(t, svc, context.Background(), scope, "trace-1")
	clock.advance(5 * time.Minute)

	_, domain := svc.Use(targetRef(scope.ID), acquired.Handle)
	if domain == nil || domain.Code != consolecore.CodeArtifactExpired {
		t.Fatalf("expected ARTIFACT_EXPIRED at exact deadline, got %v", domain)
	}
	snapshot, snapshotDomain := svc.StorageSnapshot()
	if snapshotDomain != nil {
		t.Fatal(snapshotDomain)
	}
	if snapshot.AcquiredCount != 0 || snapshot.ChargedBytes != 0 {
		t.Fatalf("expired access retained state: %+v", snapshot)
	}
}

func TestAcquireAtExactDeadlineStartsFreshInstallation(t *testing.T) {
	data := []byte("test")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	timers := &manualTimerFactory{}
	clock := newManualClock(time.UnixMilli(1000000))
	svc := newTestServiceWithDeps(t, Config{MaxBytes: 1 << 20, IdleTTL: 5 * time.Minute}, loader, opener, timers, clock, nil)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	first := acquireSync(t, svc, context.Background(), scope, "trace-1")
	clock.advance(5 * time.Minute)
	second := acquireSync(t, svc, context.Background(), scope, "trace-1")

	if second.Handle == first.Handle {
		t.Fatal("expired acquisition reused its old handle")
	}
	if loader.callCount() != 2 || opener.callCount() != 2 {
		t.Fatalf("expected a fresh load and stream, got loader=%d opener=%d", loader.callCount(), opener.callCount())
	}
}

// PR12-R11: A leased artifact is not removed while the lease is active; it is
// removed after the last lease closes.
func TestExpiryDefersRemovalWhileLeased(t *testing.T) {
	data := []byte("test")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 1 << 20, IdleTTL: 5 * time.Minute}
	timers := &manualTimerFactory{}
	clock := newManualClock(time.UnixMilli(1000000))
	svc := newTestServiceWithDeps(t, config, loader, opener, timers, clock, nil)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	acquireSync(t, svc, context.Background(), scope, "trace-1")
	lease, _ := svc.Use(targetRef(scope.ID), acquireHandle(t, svc, scope, "trace-1"))

	// Advance past the TTL and fire the timer.
	clock.advance(6 * time.Minute)
	timers.fireAll()

	// The entry should still be present because it is pinned.
	snapshot, _ := svc.StorageSnapshot()
	if snapshot.AcquiredCount != 1 {
		t.Fatalf("expected 1 pinned entry after TTL, got %d", snapshot.AcquiredCount)
	}
	lookup, domain := svc.Lookup(targetRef(scope.ID), "trace-1")
	if domain != nil || lookup.LocalAvailable {
		t.Fatalf("expired pinned entry remained inventory-visible: lookup=%#v domain=%v", lookup, domain)
	}

	// Close the lease; the deferred removal should trigger.
	_ = lease.Close(true)
	snapshot, _ = svc.StorageSnapshot()
	if snapshot.AcquiredCount != 0 {
		t.Fatalf("expected 0 entries after lease close, got %d", snapshot.AcquiredCount)
	}
}

// PR12-R11: A successful lease Close refreshes the last-use time, extending
// the idle deadline.
func TestExpiryLeaseCloseRefreshesLastUsedAt(t *testing.T) {
	data := []byte("test")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 1 << 20, IdleTTL: 5 * time.Minute}
	timers := &manualTimerFactory{}
	clock := newManualClock(time.UnixMilli(1000000))
	svc := newTestServiceWithDeps(t, config, loader, opener, timers, clock, nil)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	acquireSync(t, svc, context.Background(), scope, "trace-1")

	// Advance 3 minutes, use the artifact (refreshing last-used), then advance
	// another 3 minutes. The total elapsed is 6 minutes, but the last-use
	// refresh means the idle deadline is 5 minutes from the 3-minute mark.
	clock.advance(3 * time.Minute)
	lease, _ := svc.Use(targetRef(scope.ID), acquireHandle(t, svc, scope, "trace-1"))
	_ = lease.Close(true)

	clock.advance(3 * time.Minute)
	timers.fireAll()

	// The entry should still be present because the last-use refresh extended
	// the deadline.
	snapshot, _ := svc.StorageSnapshot()
	if snapshot.AcquiredCount != 1 {
		t.Fatalf("expected 1 entry after last-use refresh, got %d", snapshot.AcquiredCount)
	}

	// Advance past the refreshed deadline.
	clock.advance(3 * time.Minute)
	timers.fireAll()
	snapshot, _ = svc.StorageSnapshot()
	if snapshot.AcquiredCount != 0 {
		t.Fatalf("expected 0 entries after refreshed deadline, got %d", snapshot.AcquiredCount)
	}
}

// PR12-R11: Never-expire mode does not schedule idle timers.
func TestExpiryNeverExpireDoesNotScheduleTimers(t *testing.T) {
	data := []byte("test")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 1 << 20, IdleTTL: 5 * time.Minute, NeverExpire: true}
	timers := &manualTimerFactory{}
	clock := newManualClock(time.UnixMilli(1000000))
	svc := newTestServiceWithDeps(t, config, loader, opener, timers, clock, nil)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	acquireSync(t, svc, context.Background(), scope, "trace-1")

	// No timer should have been scheduled.
	if timers.latest() != nil {
		t.Fatal("expected no timer in never-expire mode")
	}

	// Advance far past the TTL; the entry should still be present.
	clock.advance(24 * time.Hour)
	timers.fireAll()
	snapshot, _ := svc.StorageSnapshot()
	if snapshot.AcquiredCount != 1 {
		t.Fatalf("expected 1 entry in never-expire mode, got %d", snapshot.AcquiredCount)
	}
}

// PR12-R11: ClearExpired removes expired unpinned entries.
func TestClearExpiredRemovesExpiredUnpinned(t *testing.T) {
	data := []byte("test")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 1 << 20, IdleTTL: 5 * time.Minute}
	timers := &manualTimerFactory{}
	clock := newManualClock(time.UnixMilli(1000000))
	svc := newTestServiceWithDeps(t, config, loader, opener, timers, clock, nil)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	acquireSync(t, svc, context.Background(), scope, "trace-1")
	clock.advance(6 * time.Minute)

	domain := svc.ClearExpired()
	if domain != nil {
		t.Fatalf("ClearExpired failed: %v", domain)
	}
	snapshot, _ := svc.StorageSnapshot()
	if snapshot.AcquiredCount != 0 {
		t.Fatalf("expected 0 entries after ClearExpired, got %d", snapshot.AcquiredCount)
	}
}

// PR25: ClearExpired is global and remains available after target rotation.
func TestClearExpiredRemainsAvailableAfterRotation(t *testing.T) {
	data := []byte("test")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 1 << 20, IdleTTL: 5 * time.Minute}
	svc := newTestService(t, config, loader, opener)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	scope2, cancelScope2 := testScope("scope-2")
	defer cancelScope2()
	svc.ActivateActivity(scope2)

	domain := svc.ClearExpired()
	if domain != nil {
		t.Fatalf("ClearExpired failed after rotation: %v", domain)
	}
}

// T4: A failed lease Close (success=false) does not refresh the last-use time.
func TestExpiryFailedLeaseCloseDoesNotRefreshLastUsedAt(t *testing.T) {
	data := []byte("test")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 1 << 20, IdleTTL: 5 * time.Minute}
	timers := &manualTimerFactory{}
	clock := newManualClock(time.UnixMilli(1000000))
	svc := newTestServiceWithDeps(t, config, loader, opener, timers, clock, nil)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	acquireSync(t, svc, context.Background(), scope, "trace-1")

	// Advance 3 minutes, then open and close a lease with success=false.
	clock.advance(3 * time.Minute)
	lease, _ := svc.Use(targetRef(scope.ID), acquireHandle(t, svc, scope, "trace-1"))
	_ = lease.Close(false)

	// Advance another 3 minutes. Total elapsed is 6 minutes from acquisition.
	// If the failed close had refreshed lastUsedAt, the deadline would be
	// 5 minutes from the 3-minute mark and the entry would survive. Since it
	// did not refresh, the deadline is 5 minutes from acquisition and the
	// entry should be expired.
	clock.advance(3 * time.Minute)
	timers.fireAll()

	snapshot, _ := svc.StorageSnapshot()
	if snapshot.AcquiredCount != 0 {
		t.Fatalf("expected 0 entries after failed close did not refresh, got %d", snapshot.AcquiredCount)
	}
}

// PR12-R11: The idle timer is scheduled at the exact delay matching the
// earliest entry's idle deadline.
func TestExpiryTimerScheduledAtExactDeadline(t *testing.T) {
	data := []byte("test")
	loader := newFakeLoader(testTraceMetadata("trace-1", int64(len(data))))
	opener := newFakeOpener(data, int64(len(data)))
	config := Config{MaxBytes: 1 << 20, IdleTTL: 5 * time.Minute}
	timers := &manualTimerFactory{}
	clock := newManualClock(time.UnixMilli(1000000))
	svc := newTestServiceWithDeps(t, config, loader, opener, timers, clock, nil)
	scope, cancelScope := testScope("scope-1")
	defer cancelScope()
	svc.ActivateActivity(scope)

	acquireSync(t, svc, context.Background(), scope, "trace-1")

	// The entry was acquired at t=1000000. With a 5-minute TTL, the deadline
	// is t=1000000+5min. The timer delay should be exactly 5 minutes.
	latest := timers.latest()
	if latest == nil {
		t.Fatal("expected a timer to be scheduled after acquisition")
	}
	if latest.delay != 5*time.Minute {
		t.Fatalf("expected timer delay of 5m, got %v", latest.delay)
	}

	// Advance 2 minutes and refresh last-used via a lease close.
	// The new deadline is t=1000000+2min+5min = t=1000000+7min.
	// The delay from the current clock (t=1000000+2min) is 5 minutes.
	clock.advance(2 * time.Minute)
	lease, _ := svc.Use(targetRef(scope.ID), acquireHandle(t, svc, scope, "trace-1"))
	_ = lease.Close(true)

	latest = timers.latest()
	if latest == nil {
		t.Fatal("expected a timer to be scheduled after lease close")
	}
	if latest.delay != 5*time.Minute {
		t.Fatalf("expected timer delay of 5m after refresh, got %v", latest.delay)
	}

	// Advance 3 minutes (now at t=1000000+5min). The deadline is at
	// t=1000000+7min, so the remaining delay is 2 minutes.
	clock.advance(3 * time.Minute)
	// Trigger a reschedule by firing the timer — onIdleTimer will not remove
	// the entry (not expired yet) and will reschedule.
	timers.fireAll()

	latest = timers.latest()
	if latest == nil {
		t.Fatal("expected a timer to be rescheduled")
	}
	if latest.delay != 2*time.Minute {
		t.Fatalf("expected timer delay of 2m remaining, got %v", latest.delay)
	}
}
