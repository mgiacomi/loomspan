package artifact

import (
	"sort"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/evidence"
)

// timerHandle is the minimal interface for a scheduled idle-expiry timer.
type timerHandle interface {
	Stop() bool
}

// idleDeadline computes the expiry time for an entry from its last-use time and
// the configured idle TTL. Returns false if the entry does not expire.
func (service *Service) idleDeadline(entry *entry) (time.Time, bool) {
	if service.ttlNeverExpire {
		return time.Time{}, false
	}
	if entry.lastUsedAt.IsZero() {
		return entry.acquisitionTime.Add(service.ttlIdleTTL), true
	}
	return entry.lastUsedAt.Add(service.ttlIdleTTL), true
}

// expireOnAccessLocked enforces the idle deadline synchronously so timer
// scheduling delay can never make an expired handle usable. It returns true
// when the entry is expired. Pinned entries are marked for deferred removal;
// unpinned entries are removed immediately.
//
// The caller must hold the service mutex.
func (service *Service) expireOnAccessLocked(entry *entry) (bool, *consolecore.Error) {
	if entry.state != stateInstalled || service.ttlNeverExpire {
		return entry.state == stateDeferredRemoval, nil
	}
	deadline, ok := service.idleDeadline(entry)
	if !ok || deadline.After(service.clock()) {
		return false, nil
	}
	if entry.pinCount > 0 {
		entry.state = stateDeferredRemoval
		service.rescheduleIdleTimerLocked()
		return true, nil
	}
	return true, service.removeEntryLocked(entry)
}

// rescheduleIdleTimerLocked recomputes the earliest idle deadline and
// reschedules the single timer. Called after any change to entries, last-use
// times, or configuration. The caller must hold the service mutex.
func (service *Service) rescheduleIdleTimerLocked() {
	if service.closed || service.ttlNeverExpire {
		if service.idleTimer != nil {
			service.idleTimer.Stop()
			service.idleTimer = nil
		}
		return
	}
	earliest := time.Time{}
	var hasDeadline bool
	for _, entry := range service.entries {
		if entry.state != stateInstalled {
			continue
		}
		deadline, ok := service.idleDeadline(entry)
		if !ok {
			continue
		}
		if !hasDeadline || deadline.Before(earliest) {
			earliest = deadline
			hasDeadline = true
		}
	}
	if service.idleTimer != nil {
		service.idleTimer.Stop()
		service.idleTimer = nil
	}
	if !hasDeadline {
		return
	}
	now := service.clock()
	delay := earliest.Sub(now)
	if delay < 0 {
		delay = 0
	}
	service.idleTimer = service.timerFactory(delay, service.onIdleTimer)
}

// onIdleTimer is the callback for the scheduled idle-expiry timer. It removes
// expired unpinned entries and reschedules.
func (service *Service) onIdleTimer() {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.closed {
		return
	}
	_ = service.removeExpiredUnpinnedLocked()
	service.rescheduleIdleTimerLocked()
}

// removeExpiredUnpinnedLocked removes all installed entries whose idle deadline
// has passed and that have no active leases. Pinned entries are marked for
// deferred removal and deleted when the last lease closes.
//
// The caller must hold the service mutex.
func (service *Service) removeExpiredUnpinnedLocked() *consolecore.Error {
	if service.ttlNeverExpire {
		return nil
	}
	now := service.clock()
	// Collect expired entries in deterministic order for stable test behavior.
	// Only entries for the current scope are eligible; old-scope entries are
	// removed during invalidation but this filter is defense in depth.
	var expired []*entry
	for _, entry := range service.entries {
		if entry.state != stateInstalled {
			continue
		}
		if entry.key.owner.Source() == evidence.SourceTarget && entry.key.owner.TargetScope() != service.currentScopeID {
			continue
		}
		deadline, ok := service.idleDeadline(entry)
		if !ok {
			continue
		}
		if !deadline.After(now) {
			expired = append(expired, entry)
		}
	}
	sort.Slice(expired, func(i, j int) bool {
		a, b := expired[i], expired[j]
		if !a.lastUsedAt.Equal(b.lastUsedAt) {
			return a.lastUsedAt.Before(b.lastUsedAt)
		}
		return a.handle < b.handle
	})
	for _, entry := range expired {
		if entry.pinCount > 0 {
			entry.state = stateDeferredRemoval
			continue
		}
		if domain := service.removeEntryLocked(entry); domain != nil {
			return domain
		}
	}
	return nil
}
