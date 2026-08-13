package artifact

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/applicationclient"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/evidence"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/target"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/workspace"
)

const (
	// streamBufferSize is the fixed-size buffer used to copy artifact bytes
	// from the upstream stream to the raw bundle component. It bounds memory
	// for any artifact size.
	streamBufferSize = 32 * 1024
)

type inputStream interface {
	Body() io.Reader
	DeclaredLength() int64
	Close() error
}

// TraceLoader loads authoritative current-scope trace metadata for a trace ID.
// The service uses it rather than browser-supplied size or path metadata.
type TraceLoader func(ctx context.Context, scope target.Scope, traceID string) (TraceMetadata, *consolecore.Error)

// StreamOpener opens a streaming artifact response within the current target
// scope. The returned stream is owned by the service and must be closed.
type StreamOpener func(ctx context.Context, scope target.Scope, traceID string) (*applicationclient.ArtifactStream, *consolecore.Error)

// runAcquisition is the leader goroutine for a joined acquisition. It loads
// metadata, opens one upstream stream, creates one staging bundle directory,
// copies the raw bytes to the raw component, validates the complete transfer,
// invokes the required processor over the installed raw component, syncs and
// atomically renames the bundle before publishing the handle. On any failure it
// removes the entire staged bundle, releases the reservation, classifies the
// failure, and publishes the error to all waiters.
//
// The acquisition context is tied to the target scope and service lifetime,
// not to any individual caller. Individual waiters cancel independently; the
// leader is cancelled only by scope/service cancellation or when no waiter
// remains.
func (service *Service) runAcquisition(entry *entry, scope target.Scope, traceID string) {
	defer close(entry.acquireFinished)

	// 1. Load authoritative trace metadata.
	metadata, domain := service.traceLoader(entry.acquireCtx, scope, traceID)
	if domain != nil {
		service.failAcquisition(entry, domain)
		return
	}
	service.mu.Lock()
	entry.metadata = metadata
	entry.applicationAvailability = ApplicationAvailable
	service.mu.Unlock()

	// 2. Open the upstream artifact stream.
	stream, domain := service.streamOpener(entry.acquireCtx, scope, traceID)
	if domain != nil {
		service.failAcquisition(entry, domain)
		return
	}

	// 3. Install the stream to the staging bundle and process it.
	artifact, domain := service.installStream(entry, stream, metadata, 0)
	if domain != nil {
		service.failAcquisition(entry, domain)
		return
	}

	// 4. Publish the successful result.
	service.publishAcquisitionSuccess(entry, artifact)
}

// installStream creates a staging bundle directory, copies the stream to the
// raw component with a fixed buffer, validates the complete transfer, invokes
// the processor over the raw component, syncs all components, and atomically
// renames the staging directory to the installed location. It returns the
// acquired artifact on success or a domain error on failure.
func (service *Service) installStream(entry *entry, stream inputStream, metadata TraceMetadata, rawLimit int64) (AcquiredArtifact, *consolecore.Error) {
	declaredLength := stream.DeclaredLength()
	knownSize := metadata.SizeBytes
	if declaredLength > 0 && declaredLength > knownSize {
		knownSize = declaredLength
	}

	// Reserve capacity for a known-size raw copy upfront.
	if knownSize > 0 {
		service.mu.Lock()
		domain := service.reserveCapacity(knownSize)
		if domain != nil {
			service.mu.Unlock()
			_ = stream.Close()
			return AcquiredArtifact{}, domain
		}
		service.totalCharged += knownSize
		entry.localBytes = knownSize
		entry.rawBytes = knownSize
		service.mu.Unlock()
	}

	// Create the staging bundle directory.
	stagingDir, err := service.storage.createStagingDir()
	if err != nil {
		_ = stream.Close()
		domain := service.storageError(err, entry)
		return AcquiredArtifact{}, service.classifyArtifactFailure(domain, entry, "")
	}

	// cleanupBundle closes resources, removes the staging bundle, and classifies
	// the cleanup through the workspace safety boundary before the reservation
	// is released.
	cleanupBundle := func(domain *consolecore.Error) (AcquiredArtifact, *consolecore.Error) {
		return AcquiredArtifact{}, service.classifyArtifactFailure(domain, entry, stagingDir)
	}

	// Create and stream the raw component.
	rawFile, _, err := service.storage.createComponent(stagingDir, ComponentRawArtifact)
	if err != nil {
		_ = stream.Close()
		return cleanupBundle(service.storageError(err, entry))
	}

	// copyCleanup closes the raw file and stream, then removes the bundle.
	copyCleanup := func(domain *consolecore.Error) (AcquiredArtifact, *consolecore.Error) {
		if closeErr := rawFile.Close(); closeErr != nil {
			domain = service.storageError(errors.Join(domain, closeErr), entry)
		}
		_ = stream.Close()
		return cleanupBundle(domain)
	}

	// Copy the stream to the raw component with a fixed buffer.
	observed, domain := service.copyStream(entry, stream, rawFile, knownSize, rawLimit)
	if domain != nil {
		return copyCleanup(domain)
	}

	// Sync and close the raw component.
	if err := rawFile.Sync(); err != nil {
		return copyCleanup(service.storageError(err, entry))
	}
	if err := rawFile.Close(); err != nil {
		_ = stream.Close()
		return cleanupBundle(service.storageError(err, entry))
	}

	_ = stream.Close()

	// Validate the observed raw byte count against declared length and metadata.
	if declaredLength >= 0 && observed != declaredLength {
		available := true
		domain := consolecore.NewError(consolecore.CodeInvalidArtifact,
			"The downloaded artifact byte count does not match the declared length.",
			entry.key.owner.ID(), consolecore.Details{RawDownloadAvailable: &available}, nil)
		return cleanupBundle(domain)
	}
	if metadata.SizeBytes > 0 && observed != metadata.SizeBytes {
		available := true
		domain := consolecore.NewError(consolecore.CodeInvalidArtifact,
			"The downloaded artifact byte count does not match the trace metadata.",
			entry.key.owner.ID(), consolecore.Details{RawDownloadAvailable: &available}, nil)
		return cleanupBundle(domain)
	}

	// Adjust the raw charge to the exact observed bytes.
	service.mu.Lock()
	if knownSize > 0 && observed != knownSize {
		diff := observed - knownSize
		service.totalCharged += diff
		entry.localBytes += diff
		entry.rawBytes = observed
	} else if knownSize == 0 {
		entry.localBytes = observed
		entry.rawBytes = observed
	}
	service.mu.Unlock()

	// Invoke the required processor over the installed raw component. The
	// processor receives a cancellable reader, immutable metadata, and a sink
	// for derived components; it never sees the staging path.
	processResult, domain := service.runProcessor(entry, stagingDir, metadata)
	if domain != nil {
		return cleanupBundle(domain)
	}

	// Generate the installed bundle directory and atomic rename.
	installedDir, err := service.storage.installedDirName()
	if err != nil {
		domain := service.storageError(err, entry)
		return cleanupBundle(domain)
	}
	if err := service.storage.renameDir(stagingDir, installedDir); err != nil {
		domain := service.storageError(err, entry)
		return cleanupBundle(domain)
	}

	// Publish aggregate raw plus derived bytes.
	service.mu.Lock()
	// Preserve target-only provenance and availability facts while replacing
	// every canonical trace fact with the processor-derived value. Imports pass
	// only the preflight identity here, so they cannot invent those target facts.
	validatedMetadata := metadata
	validatedMetadata.TraceID = processResult.Metadata.TraceID
	validatedMetadata.SessionID = processResult.Metadata.SessionID
	validatedMetadata.Outcome = processResult.Metadata.Outcome
	validatedMetadata.FinalizedAt = processResult.Metadata.FinalizedAt
	validatedMetadata.PersistencePolicy = processResult.Metadata.PersistencePolicy
	validatedMetadata.SizeBytes = observed
	entry.metadata = validatedMetadata
	entry.installedDir = installedDir
	entry.componentSizes = processResult.ComponentSizes
	aggregate := entry.rawBytes
	for _, size := range processResult.ComponentSizes {
		aggregate += size
	}
	entry.localBytes = aggregate
	service.mu.Unlock()

	now := service.clock()
	expiresAt := time.Time{}
	if !service.ttlNeverExpire {
		expiresAt = now.Add(service.ttlIdleTTL)
	}
	return AcquiredArtifact{
		Owner:         entry.key.owner,
		Handle:        entry.handle,
		Metadata:      validatedMetadata,
		LocalBytes:    aggregate,
		AcquiredAt:    entry.acquisitionTime,
		LastUsedAt:    now,
		ExpiresAt:     expiresAt,
		HasIdleExpiry: !service.ttlNeverExpire,
	}, nil
}

// runProcessor opens the raw component for reading and invokes the configured
// processor with a sink backed by the staging bundle directory and the service's
// capacity accounting. The processor never sees the staging path.
func (service *Service) runProcessor(entry *entry, stagingDir string, metadata TraceMetadata) (ProcessResult, *consolecore.Error) {
	rawReader, err := service.storage.openComponent(stagingDir, ComponentRawArtifact)
	if err != nil {
		return ProcessResult{}, service.storageError(err, entry)
	}

	sink := &stagingSink{
		service:    service,
		entry:      entry,
		stagingDir: stagingDir,
		components: make(map[ComponentName]int64),
		open:       make(map[ComponentName]bool),
		writers:    make(map[*sinkWriter]struct{}),
	}

	req := ProcessRequest{
		Context:  entry.acquireCtx,
		Metadata: metadata,
		Raw:      &cancelableReader{ctx: entry.acquireCtx, r: rawReader},
		Sink:     sink,
	}
	result, domain := service.processor.Process(req)
	hadOpenWriters := sink.closeOpenWriters()
	_ = rawReader.Close()
	if domain != nil {
		// Release any derived bytes the sink charged before the failure.
		service.mu.Lock()
		sink.releaseLocked()
		service.mu.Unlock()
		// If the acquisition context was cancelled (scope rotation, shutdown,
		// or last waiter departed), reclassify cancellation-class processor
		// errors through the same scope-aware classifier used by copyStream so
		// the caller receives TARGET_CHANGED/CONSOLE_ERROR rather than the
		// processor's generic TARGET_UNAVAILABLE/LOCAL_STORAGE_UNAVAILABLE.
		// Content invalidity (INVALID_ARTIFACT) is preserved because the
		// processor detects it before observing cancellation.
		if entry.acquireCtx.Err() != nil &&
			(domain.Code == consolecore.CodeTargetUnavailable ||
				domain.Code == consolecore.CodeLocalStorageUnavailable) {
			return ProcessResult{}, service.cancellationError(entry)
		}
		return ProcessResult{}, domain
	}
	// The processor result and the sink's authoritative accounting must agree
	// exactly. Accepting omitted, invented, or inaccurate sizes would publish a
	// bundle whose capacity charge and readable component set disagree.
	service.mu.Lock()
	integrityOK := !hadOpenWriters && !sink.invalid && len(result.ComponentSizes) == len(sink.components)
	if integrityOK {
		for name, size := range sink.components {
			if reported, ok := result.ComponentSizes[name]; !ok || reported != size {
				integrityOK = false
				break
			}
		}
	}
	if !integrityOK {
		sink.releaseLocked()
	}
	authoritative := make(map[ComponentName]int64, len(sink.components))
	if integrityOK {
		for name, size := range sink.components {
			authoritative[name] = size
		}
	}
	service.mu.Unlock()
	if !integrityOK {
		return ProcessResult{}, consolecore.NewError(consolecore.CodeConsoleError,
			"The artifact processor produced an inconsistent component bundle.",
			entry.key.owner.ID(), consolecore.Details{}, nil)
	}
	result.ComponentSizes = authoritative
	return result, nil
}

// copyStream copies bytes from the upstream stream to the raw component with a
// fixed-size buffer. For unknown-length streams it charges capacity
// incrementally. It returns the observed byte count on success or a domain
// error on failure (cancellation, short write, disk-full, or capacity).
func (service *Service) copyStream(entry *entry, stream inputStream, file writableFile, knownSize, rawLimit int64) (int64, *consolecore.Error) {
	buffer := make([]byte, streamBufferSize)
	var observed int64
	for {
		select {
		case <-entry.acquireCtx.Done():
			return observed, service.cancellationError(entry)
		default:
		}
		n, readErr := stream.Body().Read(buffer)
		if n > 0 {
			if rawLimit > 0 && (observed > rawLimit || int64(n) > rawLimit-observed) {
				return observed, consolecore.NewError(consolecore.CodeLimitExceeded,
					"The trace file exceeds the import limit.", entry.key.owner.ID(),
					consolecore.Details{LimitName: "traceImportBytes", LimitValue: rawLimit}, nil)
			}
			if knownSize > 0 && (observed > knownSize || int64(n) > knownSize-observed) {
				available := true
				return observed, consolecore.NewError(consolecore.CodeInvalidArtifact,
					"The downloaded artifact exceeds its declared size.",
					entry.key.owner.ID(),
					consolecore.Details{RawDownloadAvailable: &available}, nil)
			}
			// Unknown-length bytes must be admitted before they reach disk so
			// every partial byte is covered by aggregate capacity.
			if knownSize == 0 {
				service.mu.Lock()
				domain := service.reserveCapacity(int64(n))
				if domain != nil {
					service.mu.Unlock()
					return observed, domain
				}
				service.totalCharged += int64(n)
				entry.localBytes += int64(n)
				entry.rawBytes += int64(n)
				service.mu.Unlock()
			}
			written, writeErr := file.Write(buffer[:n])
			if written < 0 || written > n {
				return observed, service.storageError(errors.New("invalid write count"), entry)
			}
			observed += int64(written)
			if writeErr != nil {
				return observed, service.storageError(writeErr, entry)
			}
			if written < n {
				return observed, service.storageError(errors.New("short write"), entry)
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return observed, nil
			}
			if errors.Is(readErr, context.Canceled) {
				return observed, service.cancellationError(entry)
			}
			return observed, consolecore.NewError(consolecore.CodeTargetUnavailable,
				"The artifact stream was interrupted.", entry.key.owner.ID(),
				consolecore.Details{}, readErr)
		}
	}
}

// cancellationError maps an acquisition-context cancellation to the right
// domain error. If the service is closed, it returns CONSOLE_ERROR. If the
// current scope no longer matches the entry's scope (scope rotation), it
// returns TARGET_CHANGED. Otherwise (last waiter left), it returns a
// request-scoped cancellation.
//
// The acquisition context is always cancelled by the time this runs—either by
// scope rotation, service close, or the last waiter departing—so checking
// acquireCtx.Err() cannot distinguish the causes. Instead we compare the
// current scope ID against the entry's scope ID.
func (service *Service) cancellationError(entry *entry) *consolecore.Error {
	service.mu.Lock()
	closed := service.closed
	currentScopeID := service.currentScopeID
	service.mu.Unlock()
	if closed {
		return consolecore.NewError(consolecore.CodeConsoleError,
			"The Console is shutting down.", entry.key.owner.ID(),
			consolecore.Details{}, nil)
	}
	if entry.key.owner.Source() == evidence.SourceTarget && currentScopeID != entry.key.owner.TargetScope() {
		return consolecore.NewError(consolecore.CodeTargetChanged,
			"The selected target changed. Start this operation again.",
			entry.key.owner.ID(), consolecore.Details{}, nil)
	}
	return consolecore.NewError(consolecore.CodeTargetUnavailable,
		"The operation was canceled.", entry.key.owner.ID(),
		consolecore.Details{}, nil)
}

// storageError maps a filesystem error to a domain error. In unlimited
// mode, disk-full (ENOSPC) maps to LOCAL_STORAGE_UNAVAILABLE. In finite mode,
// disk-full was already caught by capacity reservation. Other I/O errors map
// to LOCAL_STORAGE_UNAVAILABLE.
func (service *Service) storageError(err error, entry *entry) *consolecore.Error {
	return consolecore.NewError(consolecore.CodeLocalStorageUnavailable,
		"Local artifact storage is unavailable.", entry.key.owner.ID(),
		consolecore.Details{}, err)
}

// classifyArtifactFailure removes the staged bundle and verifies that the
// workspace remains safe. Persistent cleanup or workspace-probe failure is
// process-fatal.
func (service *Service) classifyArtifactFailure(domain *consolecore.Error, entry *entry, stagingDir string) *consolecore.Error {
	classified := service.workspace.ClassifyArtifactFailure(domain, func() error {
		if stagingDir != "" {
			return service.storage.removeBundle(stagingDir)
		}
		return nil
	})
	if workspace.IsFatal(classified) {
		slog.Error("artifact storage failure is fatal", "ownerId", entry.key.owner.ID())
		if service.fatal != nil {
			service.fatal(classified)
		}
		return consolecore.NewError(consolecore.CodeConsoleError,
			"The Console workspace is no longer safe.", entry.key.owner.ID(),
			consolecore.Details{}, classified)
	}
	if domainErr, ok := classified.(*consolecore.Error); ok {
		return domainErr
	}
	return domain
}

// failAcquisition releases the already-cleaned reservation and publishes the
// terminal error to all waiters.
func (service *Service) failAcquisition(entry *entry, domain *consolecore.Error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if entry.state == stateRemoved {
		// Already removed by someone else; still publish the error to waiters.
		entry.acquireResult = acquireResult{err: domain}
		select {
		case <-entry.acquireDone:
		default:
			close(entry.acquireDone)
		}
		return
	}
	if entry.acquireCancel != nil {
		entry.acquireCancel()
	}
	if entry.scopeStop != nil {
		entry.scopeStop()
	}
	service.releaseReservationLocked(entry)
	entry.state = stateRemoved
	entry.acquireResult = acquireResult{err: domain}
	close(entry.acquireDone)
	delete(service.entries, entry.key)
	delete(service.handles, entry.handle)
	service.rescheduleIdleTimerLocked()
}

// publishAcquisitionSuccess transitions the entry to installed and publishes
// the handle to all waiters. If the entry was invalidated (scope rotation)
// during the transfer, the installed bundle is removed and TARGET_CHANGED is
// published instead.
func (service *Service) publishAcquisitionSuccess(entry *entry, artifact AcquiredArtifact) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if entry.state == stateRemoved {
		// The entry was invalidated during the transfer. Remove the bundle.
		domain := consolecore.NewError(
			consolecore.CodeTargetChanged,
			"The selected target changed. Start this operation again.",
			entry.key.owner.ID(), consolecore.Details{}, nil)
		if cleanupDomain := service.removeInstalledBundleLocked(entry); cleanupDomain != nil {
			domain = cleanupDomain
		}
		service.releaseReservationLocked(entry)
		entry.acquireResult = acquireResult{err: domain}
		close(entry.acquireDone)
		return
	}
	if entry.acquireCtx.Err() != nil || entry.waiters <= 0 {
		domain := consolecore.NewError(consolecore.CodeTargetUnavailable,
			"The operation was canceled.", entry.key.owner.ID(),
			consolecore.Details{}, entry.acquireCtx.Err())
		if service.closed {
			domain = consolecore.NewError(consolecore.CodeConsoleError,
				"The Console is shutting down.", entry.key.owner.ID(),
				consolecore.Details{}, entry.acquireCtx.Err())
		} else if entry.key.owner.Source() == evidence.SourceTarget && service.currentScopeID != entry.key.owner.TargetScope() {
			domain = consolecore.NewError(consolecore.CodeTargetChanged,
				"The selected target changed. Start this operation again.",
				entry.key.owner.ID(), consolecore.Details{}, entry.acquireCtx.Err())
		}
		if cleanupDomain := service.removeEntryLocked(entry); cleanupDomain != nil {
			domain = cleanupDomain
		}
		entry.acquireResult = acquireResult{err: domain}
		close(entry.acquireDone)
		return
	}
	entry.state = stateInstalled
	entry.lastUsedAt = artifact.LastUsedAt
	if entry.scopeStop != nil {
		entry.scopeStop()
		entry.scopeStop = nil
	}
	entry.acquireResult = acquireResult{artifact: artifact}
	close(entry.acquireDone)
	service.rescheduleIdleTimerLocked()
}

// releaseReservationLocked releases the entry's charged bytes from the total.
// The caller must hold the service mutex.
func (service *Service) releaseReservationLocked(entry *entry) {
	if entry.localBytes > 0 {
		service.totalCharged -= entry.localBytes
		if service.totalCharged < 0 {
			service.totalCharged = 0
		}
		entry.localBytes = 0
	}
	entry.rawBytes = 0
	entry.componentSizes = nil
}

// computeExpiry returns the idle expiry deadline for an entry.
func (service *Service) computeExpiry(entry *entry) time.Time {
	deadline, _ := service.idleDeadline(entry)
	return deadline
}

// stagingSink is the ComponentSink backed by the staging bundle directory and
// the service's capacity accounting. Each derived Write reserves capacity
// before forwarding bytes to disk, so a capacity rejection never leaves a
// partial uncharged component. The sink never exposes the staging path.
type stagingSink struct {
	service    *Service
	entry      *entry
	stagingDir string
	components map[ComponentName]int64
	open       map[ComponentName]bool
	writers    map[*sinkWriter]struct{}
	invalid    bool
	// charged tracks derived bytes charged through this sink (excludes raw).
	charged int64
}

// Create opens a new derived component for streaming writes inside the staging
// bundle. The name must be a logical component identifier, never a path.
func (sink *stagingSink) Create(ctx context.Context, name ComponentName) (ComponentWriter, *consolecore.Error) {
	if domain := validateComponentName(name); domain != nil {
		sink.service.mu.Lock()
		sink.invalid = true
		sink.service.mu.Unlock()
		return nil, domain
	}
	if name == ComponentRawArtifact {
		sink.service.mu.Lock()
		sink.invalid = true
		sink.service.mu.Unlock()
		return nil, consolecore.NewError(consolecore.CodeConsoleError,
			"The raw artifact component is owned by the acquisition leader.",
			sink.entry.key.owner.ID(), consolecore.Details{}, nil)
	}
	sink.service.mu.Lock()
	if sink.open[name] {
		sink.invalid = true
		sink.service.mu.Unlock()
		return nil, consolecore.NewError(consolecore.CodeConsoleError,
			"The artifact processor opened the same component more than once.",
			sink.entry.key.owner.ID(), consolecore.Details{}, nil)
	}
	if _, exists := sink.components[name]; exists {
		sink.invalid = true
		sink.service.mu.Unlock()
		return nil, consolecore.NewError(consolecore.CodeConsoleError,
			"The artifact processor attempted to replace a completed component.",
			sink.entry.key.owner.ID(), consolecore.Details{}, nil)
	}
	sink.open[name] = true
	sink.service.mu.Unlock()
	file, _, err := sink.service.storage.createComponent(sink.stagingDir, name)
	if err != nil {
		sink.service.mu.Lock()
		delete(sink.open, name)
		sink.service.mu.Unlock()
		return nil, consolecore.NewError(consolecore.CodeLocalStorageUnavailable,
			"Local artifact storage is unavailable.", sink.entry.key.owner.ID(),
			consolecore.Details{}, err)
	}
	writer := &sinkWriter{sink: sink, name: name, file: file, ctx: ctx}
	sink.service.mu.Lock()
	sink.writers[writer] = struct{}{}
	sink.service.mu.Unlock()
	return writer, nil
}

func (sink *stagingSink) closeOpenWriters() bool {
	sink.service.mu.Lock()
	writers := make([]*sinkWriter, 0, len(sink.writers))
	for writer := range sink.writers {
		writers = append(writers, writer)
	}
	sink.service.mu.Unlock()
	for _, writer := range writers {
		_ = writer.Close()
	}
	return len(writers) > 0
}

// releaseLocked refunds all derived bytes charged through this sink from the
// service total and entry. The caller must hold the service mutex.
func (sink *stagingSink) releaseLocked() {
	if sink.charged > 0 {
		sink.service.totalCharged -= sink.charged
		if sink.service.totalCharged < 0 {
			sink.service.totalCharged = 0
		}
		sink.entry.localBytes -= sink.charged
		if sink.entry.localBytes < 0 {
			sink.entry.localBytes = 0
		}
		sink.charged = 0
	}
	sink.components = nil
	sink.open = nil
	sink.writers = nil
}

// sinkWriter wraps a bundle component file so each Write reserves capacity
// before forwarding bytes to disk.
type sinkWriter struct {
	sink    *stagingSink
	name    ComponentName
	file    writableFile
	ctx     context.Context
	written int64
	closed  bool
	synced  bool
}

func (w *sinkWriter) Write(p []byte) (int, error) {
	if w.closed {
		return 0, errors.New("artifact component writer is closed")
	}
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	if len(p) == 0 {
		return 0, nil
	}
	// Reserve capacity for this write before forwarding to disk.
	w.sink.service.mu.Lock()
	domain := w.sink.service.reserveCapacity(int64(len(p)))
	if domain != nil {
		w.sink.service.mu.Unlock()
		return 0, domain
	}
	w.sink.service.totalCharged += int64(len(p))
	w.sink.entry.localBytes += int64(len(p))
	w.sink.charged += int64(len(p))
	w.sink.service.mu.Unlock()
	n, err := w.file.Write(p)
	if n < 0 || n > len(p) {
		n = 0
		err = errors.New("artifact component writer returned an invalid write count")
	}
	if err != nil {
		w.sink.service.mu.Lock()
		w.sink.invalid = true
		w.sink.service.mu.Unlock()
	}
	w.written += int64(n)
	if n > 0 {
		w.synced = false
	}
	// Refund any short-write overcharge.
	if n < len(p) {
		refund := int64(len(p) - n)
		w.sink.service.mu.Lock()
		w.sink.service.totalCharged -= refund
		if w.sink.service.totalCharged < 0 {
			w.sink.service.totalCharged = 0
		}
		w.sink.entry.localBytes -= refund
		if w.sink.entry.localBytes < 0 {
			w.sink.entry.localBytes = 0
		}
		w.sink.charged -= refund
		if w.sink.charged < 0 {
			w.sink.charged = 0
		}
		w.sink.service.mu.Unlock()
	}
	if err != nil {
		return n, w.sink.service.storageError(err, w.sink.entry)
	}
	return n, nil
}

func (w *sinkWriter) Sync() error {
	if w.closed {
		return nil
	}
	if err := w.file.Sync(); err != nil {
		w.sink.service.mu.Lock()
		w.sink.invalid = true
		w.sink.service.mu.Unlock()
		return err
	}
	w.synced = true
	return nil
}

func (w *sinkWriter) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true
	closeErr := w.file.Close()
	// Record the final synced component size.
	w.sink.service.mu.Lock()
	delete(w.sink.open, w.name)
	delete(w.sink.writers, w)
	if closeErr != nil || !w.synced {
		w.sink.invalid = true
	}
	if closeErr == nil && w.synced && w.sink.components != nil {
		w.sink.components[w.name] = w.written
	}
	w.sink.service.mu.Unlock()
	if closeErr != nil {
		return closeErr
	}
	if !w.synced {
		return errors.New("artifact component was closed without syncing its final contents")
	}
	return nil
}

// cancelableReader wraps a reader so reads respect a context. It is used to
// give the processor a cancellable view of the raw component without exposing
// the storage path.
type cancelableReader struct {
	ctx context.Context
	r   io.Reader
}

func (cr *cancelableReader) Read(p []byte) (int, error) {
	if err := cr.ctx.Err(); err != nil {
		return 0, err
	}
	return cr.r.Read(p)
}
