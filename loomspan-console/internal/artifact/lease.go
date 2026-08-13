package artifact

import (
	"errors"
	"io"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/evidence"
)

// Lease pins an installed artifact entry for downstream analysis. It increments
// the entry's pin count for its lifetime, preventing eviction and explicit
// removal. Only a successful Close refreshes the entry's last-use time. The
// lease provides bounded component readers and size lookup without ever
// exposing the filesystem path through any DTO.
type Lease struct {
	service *Service
	entry   *entry
	owner   evidence.Owner
	closed  bool
	readers map[io.ReadCloser]struct{}
}

// OpenComponent opens a named bundle component for seekable reading. The name
// must be a known component of this bundle (ComponentRawArtifact or a derived
// component recorded by the processor). The reader is valid until Close is
// called on the lease or the reader. If the entry has been invalidated (scope
// rotation, removal), OpenComponent returns an error.
//
// Component names are closed logical identifiers, never caller-supplied paths.
func (lease *Lease) OpenComponent(name ComponentName) (ComponentReader, error) {
	lease.service.mu.Lock()
	defer lease.service.mu.Unlock()
	if lease.closed {
		return nil, errors.New("artifact lease is closed")
	}
	if lease.entry.state != stateInstalled {
		return nil, errors.New("artifact is no longer installed")
	}
	if domain := validateComponentName(name); domain != nil {
		return nil, errors.New(domain.Message)
	}
	if !lease.componentExistsLocked(name) {
		return nil, errors.New("artifact component is not available")
	}
	reader, err := lease.service.storage.openComponent(lease.entry.installedDir, name)
	if err != nil {
		return nil, err
	}
	lease.readers[reader] = struct{}{}
	return &leaseReader{lease: lease, reader: reader}, nil
}

// ComponentSize returns the synced byte size of a named bundle component. The
// name must be a known component of this bundle.
func (lease *Lease) ComponentSize(name ComponentName) (int64, error) {
	lease.service.mu.Lock()
	defer lease.service.mu.Unlock()
	if lease.closed {
		return 0, errors.New("artifact lease is closed")
	}
	if lease.entry.state != stateInstalled {
		return 0, errors.New("artifact is no longer installed")
	}
	if domain := validateComponentName(name); domain != nil {
		return 0, errors.New(domain.Message)
	}
	if name == ComponentRawArtifact {
		return lease.entry.rawBytes, nil
	}
	if lease.entry.componentSizes != nil {
		if size, ok := lease.entry.componentSizes[name]; ok {
			return size, nil
		}
	}
	return 0, errors.New("artifact component is not available")
}

// componentExistsLocked reports whether a component name is part of this bundle.
// The caller must hold the service mutex.
func (lease *Lease) componentExistsLocked(name ComponentName) bool {
	if name == ComponentRawArtifact {
		return true
	}
	if lease.entry.componentSizes != nil {
		if _, ok := lease.entry.componentSizes[name]; ok {
			return true
		}
	}
	return false
}

// Close releases the pin. If success is true, the entry's last-use time is
// refreshed, which extends its idle deadline. If success is false (error or
// cancellation), the last-use time is not refreshed. If the entry was marked
// for deferred removal (expiry or scope invalidation while pinned), the final
// Close removes the entry and releases its bytes.
//
// Close is idempotent.
func (lease *Lease) Close(success bool) error {
	lease.service.mu.Lock()
	defer lease.service.mu.Unlock()
	if lease.closed {
		return nil
	}
	lease.closed = true
	entry := lease.entry
	for reader := range lease.readers {
		_ = reader.Close()
		delete(lease.readers, reader)
	}
	delete(entry.leases, lease)
	entry.pinCount--
	if entry.pinCount < 0 {
		entry.pinCount = 0
	}
	if success && entry.state == stateInstalled {
		entry.lastUsedAt = lease.service.clock()
		lease.service.rescheduleIdleTimerLocked()
	}
	// If the entry was marked for deferred removal and the last pin just
	// closed, remove it now.
	if entry.state == stateDeferredRemoval && entry.pinCount == 0 {
		return lease.service.removeEntryLocked(entry)
	}
	return nil
}

func (lease *Lease) Owner() evidence.Owner { return lease.owner }

// useEntry issues a lease for an installed entry, incrementing its pin count.
// The caller must hold the service mutex.
func (service *Service) useEntryLocked(entry *entry, owner evidence.Owner) (*Lease, *consolecore.Error) {
	if entry.state != stateInstalled {
		return nil, consolecore.NewError(consolecore.CodeArtifactExpired,
			"The artifact is no longer available.", owner.ID(), consolecore.Details{}, nil)
	}
	if entry.leases == nil {
		entry.leases = make(map[*Lease]struct{})
	}
	entry.pinCount++
	lease := &Lease{
		service: service,
		entry:   entry,
		owner:   owner,
		readers: make(map[io.ReadCloser]struct{}),
	}
	entry.leases[lease] = struct{}{}
	return lease, nil
}

// invalidateLeasesLocked synchronously closes every reader and invalidates
// every lease for an entry. It is used only for authoritative scope/process
// invalidation; ordinary expiry and removal continue to defer while pinned.
func (service *Service) invalidateLeasesLocked(entry *entry) {
	for lease := range entry.leases {
		lease.closed = true
		for reader := range lease.readers {
			_ = reader.Close()
			delete(lease.readers, reader)
		}
		delete(entry.leases, lease)
	}
	entry.pinCount = 0
}

type leaseReader struct {
	lease  *Lease
	reader io.ReadCloser
	closed bool
}

func (reader *leaseReader) Read(buffer []byte) (int, error) {
	return reader.reader.Read(buffer)
}

func (reader *leaseReader) Seek(offset int64, whence int) (int64, error) {
	seeker, ok := reader.reader.(io.Seeker)
	if !ok {
		return 0, errors.New("artifact component reader is not seekable")
	}
	return seeker.Seek(offset, whence)
}

func (reader *leaseReader) Close() error {
	reader.lease.service.mu.Lock()
	defer reader.lease.service.mu.Unlock()
	if reader.closed {
		return nil
	}
	reader.closed = true
	delete(reader.lease.readers, reader.reader)
	return reader.reader.Close()
}
