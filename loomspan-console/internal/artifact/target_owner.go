package artifact

import (
	"context"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/evidence"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/target"
)

// ActivateActivity is called by the target context after a successful probe.
// The artifact service does not consume activity streams, but it uses this
// hook to learn the current target scope ID so Use, Remove, ClearExpired,
// ClearAllUnused, and StorageSnapshot can validate the caller's scope.
func (service *Service) ActivateActivity(scope target.Scope) {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.currentScopeID = scope.ID
}

// InvalidateTargetScope is called synchronously by the target context during
// scope rotation. It stops new old-scope work, cancels acquisitions and leases,
// waits for their bounded cleanup, invalidates all handles, removes scope
// content, and releases all charges before returning.
//
// Already installed current-scope entries are preserved when an upstream
// credential is rejected without scope rotation. Credential replacement and
// instance change continue to rotate and clear the scope.
func (service *Service) InvalidateTargetScope(previous target.ScopeID, cancelled context.Context) {
	service.mu.Lock()
	if service.currentScopeID == previous {
		service.currentScopeID = ""
	}
	// Cancel all acquisition contexts for the old scope and collect finish
	// channels so we can wait for their bounded cleanup.
	var finishChans []chan struct{}
	for _, entry := range service.entries {
		if entry.key.owner != evidence.Target(previous) {
			continue
		}
		if entry.acquireCancel != nil {
			entry.acquireCancel()
		}
		if entry.scopeStop != nil {
			entry.scopeStop()
		}
		if entry.state == stateAcquiring {
			finishChans = append(finishChans, entry.acquireFinished)
		}
	}
	service.mu.Unlock()

	// Wait for acquisition goroutines to exit. This is bounded by the
	// acquisition context cancellation above.
	for _, ch := range finishChans {
		<-ch
	}

	// Now remove all old-scope entries and release their charges.
	service.mu.Lock()
	for key, entry := range service.entries {
		if key.owner != evidence.Target(previous) {
			continue
		}
		service.invalidateLeasesLocked(entry)
		_ = service.removeEntryLocked(entry)
	}
	service.mu.Unlock()
}
