package artifact

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/evidence"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/target"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/workspace"
)

// Config carries the resolved artifact cache policy consumed from
// profile.Resolved without changing config syntax or defaults.
type Config struct {
	MaxBytes    int64
	Unlimited   bool
	IdleTTL     time.Duration
	NeverExpire bool
}

// Dependencies are the external collaborators the service needs.
type Dependencies struct {
	Lifetime     context.Context
	Workspace    *workspace.Workspace
	TraceLoader  TraceLoader
	StreamOpener StreamOpener
	Processor    Processor
	Fatal        func(error)
	Clock        func() time.Time
	Entropy      func() ([]byte, error)
	TimerFactory func(time.Duration, func()) timerHandle
	FileSystem   fileSystem
}

// Service is the sole owner of analysis artifact state. It acquires each
// selected finalized trace at most once per target scope, installs one validated
// bundle (raw artifact plus derived components) atomically beneath the verified
// workspace, and owns its opaque handle, aggregate capacity charge, idle
// lifetime, active-use pinning, and removal.
//
// Browser calls and future MCP calls use the same service. The service is
// transport-neutral; PR 12 exposes browser adapters in Phase 3.
type Service struct {
	mu             sync.Mutex
	lifetime       context.Context
	workspace      *workspace.Workspace
	capacity       capacityConfig
	ttlIdleTTL     time.Duration
	ttlNeverExpire bool
	totalCharged   int64
	entries        map[entryKey]*entry
	handles        map[Handle]*entry
	traceLoader    TraceLoader
	streamOpener   StreamOpener
	processor      Processor
	fatal          func(error)
	clock          func() time.Time
	entropy        func() ([]byte, error)
	timerFactory   func(time.Duration, func()) timerHandle
	idleTimer      timerHandle
	storage        *storage
	currentScopeID target.ScopeID
	importedOwner  evidence.Owner
	closed         bool
}

// New creates the artifact service. The workspace's transient directory is used
// for artifact installation. The service must be registered as a target scope
// owner before TargetContext.StartServing and closed before workspace cleanup
// at shutdown.
func New(config Config, deps Dependencies) (*Service, error) {
	if deps.Workspace == nil {
		return nil, fmt.Errorf("artifact service requires a workspace")
	}
	if deps.TraceLoader == nil {
		return nil, fmt.Errorf("artifact service requires a trace loader")
	}
	if deps.StreamOpener == nil {
		return nil, fmt.Errorf("artifact service requires a stream opener")
	}
	if deps.Processor == nil {
		return nil, fmt.Errorf("artifact service requires a processor")
	}
	clock := deps.Clock
	if clock == nil {
		clock = time.Now
	}
	entropy := deps.Entropy
	if entropy == nil {
		entropy = cryptoRandBytes
	}
	timerFactory := deps.TimerFactory
	if timerFactory == nil {
		timerFactory = func(delay time.Duration, callback func()) timerHandle {
			return realTimer{timer: time.AfterFunc(delay, callback)}
		}
	}
	lifetime := deps.Lifetime
	if lifetime == nil {
		lifetime = context.Background()
	}
	store, err := newStorage(deps.Workspace.Transient, deps.FileSystem, entropy)
	if err != nil {
		return nil, err
	}
	ownerHandle, err := newHandle(entropy)
	if err != nil {
		return nil, fmt.Errorf("create imported evidence owner: %w", err)
	}
	importedOwner, err := evidence.Imported(string(ownerHandle))
	if err != nil {
		return nil, err
	}
	return &Service{
		lifetime:       lifetime,
		workspace:      deps.Workspace,
		capacity:       capacityConfig{maxBytes: config.MaxBytes, unlimited: config.Unlimited},
		ttlIdleTTL:     config.IdleTTL,
		ttlNeverExpire: config.NeverExpire,
		entries:        make(map[entryKey]*entry),
		handles:        make(map[Handle]*entry),
		traceLoader:    deps.TraceLoader,
		streamOpener:   deps.StreamOpener,
		processor:      deps.Processor,
		fatal:          deps.Fatal,
		clock:          clock,
		entropy:        entropy,
		timerFactory:   timerFactory,
		storage:        store,
		importedOwner:  importedOwner,
	}, nil
}

// realTimer wraps time.AfterFunc to satisfy the timerHandle interface.
type realTimer struct {
	timer *time.Timer
}

func (t realTimer) Stop() bool { return t.timer.Stop() }

// Acquire joins or starts an acquisition for the given trace within the current
// target scope. Concurrent callers for the same (scope, trace) join one
// metadata load, one upstream stream, and one installed file. Each waiter
// cancels independently; the leader is cancelled only by scope/service
// cancellation or when no waiter remains. A successful already-installed
// lookup returns the same handle without an upstream call.
func (service *Service) Acquire(ctx context.Context, scope target.Scope, traceID string) (AcquiredArtifact, *consolecore.Error) {
	if traceID == "" {
		return AcquiredArtifact{}, consolecore.NewError(consolecore.CodeInvalidArgument,
			"A trace ID is required.", string(scope.ID), consolecore.Details{}, nil)
	}
	service.mu.Lock()
	if service.closed {
		service.mu.Unlock()
		return AcquiredArtifact{}, consolecore.NewError(consolecore.CodeConsoleError,
			"The Console is shutting down.", string(scope.ID), consolecore.Details{}, nil)
	}
	if service.currentScopeID != scope.ID || scope.Context == nil || scope.Context.Err() != nil {
		service.mu.Unlock()
		return AcquiredArtifact{}, consolecore.NewError(consolecore.CodeTargetChanged,
			"The selected target changed. Start this operation again.",
			string(scope.ID), consolecore.Details{}, nil)
	}
	key := entryKey{owner: evidence.Target(scope.ID), traceID: traceID}
	ent, exists := service.entries[key]
	if exists && ent.state == stateInstalled {
		expired, domain := service.expireOnAccessLocked(ent)
		if domain != nil {
			service.mu.Unlock()
			return AcquiredArtifact{}, domain
		}
		if expired {
			if ent.state == stateDeferredRemoval {
				service.mu.Unlock()
				return AcquiredArtifact{}, consolecore.NewError(consolecore.CodeArtifactExpired,
					"The artifact is no longer available.", string(scope.ID), consolecore.Details{}, nil)
			}
			exists = false
		} else {
			artifact := service.buildAcquiredArtifactLocked(ent)
			service.mu.Unlock()
			return artifact, nil
		}
	}
	if exists && (ent.state == stateDeferredRemoval || ent.state == stateRemoved) {
		// The entry is being or has been removed. Remove it and start fresh.
		if domain := service.removeEntryLocked(ent); domain != nil {
			service.mu.Unlock()
			return AcquiredArtifact{}, domain
		}
		exists = false
	}
	if exists && ent.state == stateAcquiring && ent.acquireCtx.Err() != nil {
		// The acquisition was cancelled (last waiter left or scope rotated)
		// but the leader goroutine has not yet published the result. Remove
		// the stale entry and start a fresh acquisition so the new caller
		// does not join a doomed one and receive a stale cancellation error.
		if domain := service.removeEntryLocked(ent); domain != nil {
			service.mu.Unlock()
			return AcquiredArtifact{}, domain
		}
		exists = false
	}
	if !exists {
		handle, err := newHandle(service.entropy)
		if err != nil {
			service.mu.Unlock()
			return AcquiredArtifact{}, consolecore.NewError(consolecore.CodeConsoleError,
				"The artifact handle could not be generated.", string(scope.ID), consolecore.Details{}, err)
		}
		acquireCtx, acquireCancel := context.WithCancel(service.lifetime)
		scopeStop := context.AfterFunc(scope.Context, acquireCancel)
		now := service.clock()
		ent = &entry{
			key:             key,
			handle:          handle,
			state:           stateAcquiring,
			acquisitionTime: now,
			acquireCtx:      acquireCtx,
			acquireCancel:   acquireCancel,
			scopeStop:       scopeStop,
			acquireDone:     make(chan struct{}),
			acquireFinished: make(chan struct{}),
		}
		service.entries[key] = ent
		service.handles[handle] = ent
		go service.runAcquisition(ent, scope, traceID)
	}
	ent.waiters++
	done := ent.acquireDone
	service.mu.Unlock()

	select {
	case <-done:
		service.mu.Lock()
		result := ent.acquireResult
		service.mu.Unlock()
		return result.artifact, result.err
	case <-ctx.Done():
		service.mu.Lock()
		ent.waiters--
		if ent.waiters <= 0 && ent.state == stateAcquiring {
			ent.acquireCancel()
		}
		service.mu.Unlock()
		// If the scope context is also done, the cancellation was caused by
		// scope rotation rather than the caller themselves.
		if scope.Context.Err() != nil {
			return AcquiredArtifact{}, consolecore.NewError(consolecore.CodeTargetChanged,
				"The selected target changed. Start this operation again.",
				string(scope.ID), consolecore.Details{}, ctx.Err())
		}
		return AcquiredArtifact{}, consolecore.NewError(consolecore.CodeTargetUnavailable,
			"The operation was canceled.", string(scope.ID), consolecore.Details{}, ctx.Err())
	}
}

// Use issues a lease for an installed artifact, incrementing its pin count.
// The lease provides read access to the installed file without exposing the
// path. This seam is ready for PR 13's streaming/parser work.
func (service *Service) Use(ref evidence.Reference, handle Handle) (*Lease, *consolecore.Error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	owner, domain := service.resolveOwnerLocked(ref)
	if domain != nil {
		return nil, domain
	}
	if service.closed {
		return nil, consolecore.NewError(consolecore.CodeConsoleError,
			"The Console is shutting down.", owner.ID(), consolecore.Details{}, nil)
	}
	if !isValidHandle(handle) {
		return nil, consolecore.NewError(consolecore.CodeInvalidArgument,
			"The artifact handle is malformed.", owner.ID(), consolecore.Details{}, nil)
	}
	entry, exists := service.handles[handle]
	if !exists || entry.state != stateInstalled || entry.key.owner != owner {
		return nil, consolecore.NewError(consolecore.CodeArtifactExpired,
			"The artifact is no longer available.", owner.ID(), consolecore.Details{}, nil)
	}
	expired, domain := service.expireOnAccessLocked(entry)
	if domain != nil {
		return nil, domain
	}
	if expired {
		return nil, consolecore.NewError(consolecore.CodeArtifactExpired,
			"The artifact is no longer available.", owner.ID(), consolecore.Details{}, nil)
	}
	return service.useEntryLocked(entry, owner)
}

// Remove removes an unused installed artifact. If the artifact has an active
// lease, it returns ARTIFACT_IN_USE without force-cancelling the lease.
func (service *Service) Remove(ref evidence.Reference, traceID string) *consolecore.Error {
	service.mu.Lock()
	defer service.mu.Unlock()
	owner, domain := service.resolveOwnerLocked(ref)
	if domain != nil {
		return domain
	}
	if service.closed {
		return consolecore.NewError(consolecore.CodeConsoleError,
			"The Console is shutting down.", owner.ID(), consolecore.Details{}, nil)
	}
	key := entryKey{owner: owner, traceID: traceID}
	entry, exists := service.entries[key]
	if !exists || entry.state != stateInstalled {
		return consolecore.NewError(consolecore.CodeArtifactExpired,
			"The artifact is no longer available.", owner.ID(), consolecore.Details{}, nil)
	}
	if entry.pinCount > 0 {
		return consolecore.NewError(consolecore.CodeArtifactInUse,
			"The artifact is in use and cannot be removed.", owner.ID(), consolecore.Details{}, nil)
	}
	return service.removeEntryLocked(entry)
}

// ClearExpired removes all expired unpinned entries in the current scope.
// Pinned entries are marked for deferred removal.
func (service *Service) ClearExpired() *consolecore.Error {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.closed {
		return consolecore.NewError(consolecore.CodeConsoleError,
			"The Console is shutting down.", "", consolecore.Details{}, nil)
	}
	if domain := service.removeExpiredUnpinnedLocked(); domain != nil {
		return domain
	}
	service.rescheduleIdleTimerLocked()
	return nil
}

// ClearAllUnused removes all unused installed entries in the current scope.
// Pinned entries are preserved.
func (service *Service) ClearAllUnused() *consolecore.Error {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.closed {
		return consolecore.NewError(consolecore.CodeConsoleError,
			"The Console is shutting down.", "", consolecore.Details{}, nil)
	}
	for _, entry := range service.entries {
		if entry.state != stateInstalled || entry.pinCount > 0 {
			continue
		}
		if domain := service.removeEntryLocked(entry); domain != nil {
			return domain
		}
	}
	service.rescheduleIdleTimerLocked()
	return nil
}

// StorageSnapshot returns a side-effect-free view of the artifact cache.
// Viewing the snapshot does not refresh any entry's last-use time.
func (service *Service) StorageSnapshot() (StorageSnapshot, *consolecore.Error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.closed {
		return StorageSnapshot{}, consolecore.NewError(consolecore.CodeConsoleError,
			"The Console is shutting down.", "", consolecore.Details{}, nil)
	}
	snapshot := StorageSnapshot{
		WorkspaceLabel: filepath.Base(service.workspace.Root),
		MaxBytes:       service.capacity.maxBytes,
		Unlimited:      service.capacity.unlimited,
		IdleTTL:        service.ttlIdleTTL,
		NeverExpire:    service.ttlNeverExpire,
		ChargedBytes:   service.totalCharged,
		AcquiredCount:  0,
		Entries:        []StoredEntry{},
	}
	for _, entry := range service.entries {
		if entry.state == stateAcquiring {
			snapshot.PendingAcquisitionCount++
			snapshot.PendingWaiterCount += entry.waiters
			continue
		}
		if entry.state != stateInstalled && entry.state != stateDeferredRemoval {
			continue
		}
		snapshot.AcquiredCount++
		deadline, hasExpiry := service.idleDeadline(entry)
		snapshot.Entries = append(snapshot.Entries, StoredEntry{
			Source:                    entry.key.owner.Source(),
			TargetScopeID:             string(entry.key.owner.TargetScope()),
			TraceID:                   entry.metadata.TraceID,
			SessionID:                 entry.metadata.SessionID,
			Outcome:                   entry.metadata.Outcome,
			PersistencePolicy:         entry.metadata.PersistencePolicy,
			FinalizedAt:               entry.metadata.FinalizedAt,
			AcquiredAt:                entry.acquisitionTime,
			LastUsedAt:                entry.lastUsedAt,
			ExpiresAt:                 deadline,
			HasIdleExpiry:             hasExpiry,
			LocalBytes:                entry.localBytes,
			ApplicationTraceExpiresAt: applicationTraceExpiresAt(entry),
			ApplicationAvailability:   string(entry.applicationAvailability),
			LocalAvailable:            true,
			ActivePin:                 entry.pinCount > 0,
		})
	}
	return snapshot, nil
}

func applicationTraceExpiresAt(entry *entry) *time.Time {
	if entry.key.owner.Source() != evidence.SourceTarget || entry.metadata.ApplicationTraceExpiresAt.IsZero() {
		return nil
	}
	expiresAt := entry.metadata.ApplicationTraceExpiresAt
	return &expiresAt
}

// Lookup returns the installed artifact entry for the given trace ID within
// the current scope, or a zero result with LocalAvailable=false if no entry
// is installed. This is a read-only side-effect-free lookup; it does not
// refresh the entry's last-use time.
func (service *Service) Lookup(ref evidence.Reference, traceID string) (LookupResult, *consolecore.Error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	owner, domain := service.resolveOwnerLocked(ref)
	if domain != nil {
		return LookupResult{}, domain
	}
	if service.closed {
		return LookupResult{}, consolecore.NewError(consolecore.CodeConsoleError,
			"The Console is shutting down.", owner.ID(), consolecore.Details{}, nil)
	}
	key := entryKey{owner: owner, traceID: traceID}
	entry, exists := service.entries[key]
	if !exists || entry.state != stateInstalled {
		return LookupResult{LocalAvailable: false}, nil
	}
	expired, domain := service.expireOnAccessLocked(entry)
	if domain != nil {
		return LookupResult{}, domain
	}
	if expired {
		return LookupResult{LocalAvailable: false}, nil
	}
	deadline, hasExpiry := service.idleDeadline(entry)
	return LookupResult{
		Owner:                   owner,
		Handle:                  entry.handle,
		Metadata:                entry.metadata,
		LocalAvailable:          true,
		ApplicationAvailability: entry.applicationAvailability,
		AcquiredAt:              entry.acquisitionTime,
		LastUsedAt:              entry.lastUsedAt,
		ExpiresAt:               deadline,
		HasIdleExpiry:           hasExpiry,
		LocalBytes:              entry.localBytes,
	}, nil
}

func (service *Service) resolveOwnerLocked(ref evidence.Reference) (evidence.Owner, *consolecore.Error) {
	if !ref.Valid() {
		return evidence.Owner{}, consolecore.NewError(consolecore.CodeInvalidArgument,
			"The evidence source is invalid.", "", consolecore.Details{}, nil)
	}
	if ref.Source == evidence.SourceImported {
		return service.importedOwner, nil
	}
	if service.currentScopeID != ref.TargetScope {
		return evidence.Owner{}, consolecore.NewError(consolecore.CodeTargetChanged,
			"The selected target changed. Start this operation again.", string(ref.TargetScope),
			consolecore.Details{}, nil)
	}
	return evidence.Target(ref.TargetScope), nil
}

// Close shuts down the service. It cancels all transfers, invalidates handles,
// closes timers, waits for workers, and removes owned state. It must be called
// before workspace cleanup at shutdown.
func (service *Service) Close() {
	service.mu.Lock()
	if service.closed {
		service.mu.Unlock()
		return
	}
	service.closed = true
	if service.idleTimer != nil {
		service.idleTimer.Stop()
		service.idleTimer = nil
	}
	// Cancel all acquisition contexts and collect finish channels.
	var finishChans []chan struct{}
	for _, entry := range service.entries {
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

	// Wait for all acquisition goroutines to exit.
	for _, ch := range finishChans {
		<-ch
	}

	service.mu.Lock()
	defer service.mu.Unlock()
	for _, entry := range service.entries {
		service.invalidateLeasesLocked(entry)
		_ = service.removeEntryLocked(entry)
	}
	if err := service.storage.removeAllContents(); err != nil && service.fatal != nil {
		service.fatal(err)
	}
}

// removeEntryLocked removes an entry: cancels its acquisition context, removes
// its installed bundle directory, releases its charged bytes, and removes it
// from all maps. The caller must hold the service mutex. This is safe for any
// entry state; the leader goroutine will see stateRemoved and publish the error
// via acquireDone.
func (service *Service) removeEntryLocked(entry *entry) *consolecore.Error {
	if entry.state == stateRemoved {
		return nil
	}
	if entry.acquireCancel != nil {
		entry.acquireCancel()
	}
	if entry.scopeStop != nil {
		entry.scopeStop()
	}
	if domain := service.removeInstalledBundleLocked(entry); domain != nil {
		entry.state = stateDeferredRemoval
		return domain
	}
	service.releaseReservationLocked(entry)
	entry.state = stateRemoved
	delete(service.entries, entry.key)
	delete(service.handles, entry.handle)
	service.rescheduleIdleTimerLocked()
	return nil
}

// removeInstalledBundleLocked removes an installed bundle directory with one
// retry through the workspace safety boundary. The caller must hold the service
// mutex.
func (service *Service) removeInstalledBundleLocked(entry *entry) *consolecore.Error {
	if entry.installedDir == "" {
		return nil
	}
	path := entry.installedDir
	if err := service.storage.removeBundle(path); err != nil {
		classified := service.workspace.ClassifyArtifactFailure(err, func() error {
			return service.storage.removeBundle(path)
		})
		if workspace.IsFatal(classified) {
			domain := consolecore.NewError(consolecore.CodeConsoleError,
				"The Console workspace is no longer safe.",
				entry.key.owner.ID(), consolecore.Details{}, classified)
			if service.fatal != nil {
				service.fatal(classified)
			}
			return domain
		}
	}
	entry.installedDir = ""
	return nil
}

// buildAcquiredArtifactLocked constructs the AcquiredArtifact result for an
// already-installed entry. The caller must hold the service mutex.
func (service *Service) buildAcquiredArtifactLocked(entry *entry) AcquiredArtifact {
	deadline, hasExpiry := service.idleDeadline(entry)
	return AcquiredArtifact{
		Owner:         entry.key.owner,
		Handle:        entry.handle,
		Metadata:      entry.metadata,
		LocalBytes:    entry.localBytes,
		AcquiredAt:    entry.acquisitionTime,
		LastUsedAt:    entry.lastUsedAt,
		ExpiresAt:     deadline,
		HasIdleExpiry: hasExpiry,
	}
}
