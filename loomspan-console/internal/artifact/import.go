package artifact

import (
	"context"
	"io"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
)

const hardImportLimit int64 = 4 << 30

// Import admits one untrusted canonical NDJSON stream under the process-local
// imported-evidence owner. Identity and metadata come only from processor
// validation; declaredLength is used solely as an admission bound.
func (service *Service) Import(ctx context.Context, reader io.Reader, declaredLength int64) (AcquiredArtifact, *consolecore.Error) {
	if ctx == nil || reader == nil || declaredLength < -1 {
		return AcquiredArtifact{}, consolecore.NewError(consolecore.CodeInvalidArgument,
			"A valid trace stream is required.", service.importedOwner.ID(), consolecore.Details{}, nil)
	}
	if err := ctx.Err(); err != nil {
		return AcquiredArtifact{}, consolecore.NewError(consolecore.CodeConsoleError,
			"The trace import was canceled.", service.importedOwner.ID(), consolecore.Details{}, err)
	}
	limit := service.effectiveImportLimit()
	if declaredLength > limit {
		return AcquiredArtifact{}, consolecore.NewError(consolecore.CodeLimitExceeded,
			"The trace file exceeds the import limit.", service.importedOwner.ID(),
			consolecore.Details{LimitName: "traceImportBytes", LimitValue: limit}, nil)
	}
	processor, ok := service.processor.(ImportProcessor)
	if !ok {
		return AcquiredArtifact{}, consolecore.NewError(consolecore.CodeConsoleError,
			"The trace processor does not support imports.", service.importedOwner.ID(), consolecore.Details{}, nil)
	}
	preflight, domain := processor.PreflightImport(ctx, reader)
	if domain != nil {
		markImportError(domain)
		return AcquiredArtifact{}, domain
	}
	if err := ctx.Err(); err != nil {
		return AcquiredArtifact{}, consolecore.NewError(consolecore.CodeConsoleError,
			"The trace import was canceled.", service.importedOwner.ID(), consolecore.Details{}, err)
	}

	service.mu.Lock()
	if service.closed {
		service.mu.Unlock()
		return AcquiredArtifact{}, consolecore.NewError(consolecore.CodeConsoleError,
			"The Console is shutting down.", service.importedOwner.ID(), consolecore.Details{}, nil)
	}
	key := entryKey{owner: service.importedOwner, traceID: preflight.Header.TraceID}
	if _, exists := service.entries[key]; exists {
		service.mu.Unlock()
		return AcquiredArtifact{}, consolecore.NewError(consolecore.CodeArtifactAlreadyExists,
			"An imported trace with this identity is already installed.", service.importedOwner.ID(),
			consolecore.Details{}, nil)
	}
	handle, err := newHandle(service.entropy)
	if err != nil {
		service.mu.Unlock()
		return AcquiredArtifact{}, consolecore.NewError(consolecore.CodeConsoleError,
			"The artifact handle could not be generated.", service.importedOwner.ID(), consolecore.Details{}, err)
	}
	acquireCtx, cancel := context.WithCancel(service.lifetime)
	requestStop := context.AfterFunc(ctx, cancel)
	now := service.clock()
	entry := &entry{
		key: key, handle: handle, state: stateAcquiring,
		metadata:        TraceMetadata{TraceID: preflight.Header.TraceID, SessionID: preflight.Header.SessionID},
		acquisitionTime: now, acquireCtx: acquireCtx, acquireCancel: cancel, scopeStop: requestStop,
		acquireDone: make(chan struct{}), acquireFinished: make(chan struct{}), waiters: 1,
	}
	service.entries[key] = entry
	service.handles[handle] = entry
	service.mu.Unlock()
	defer close(entry.acquireFinished)

	stream := &readerInput{reader: preflight.Raw, declaredLength: declaredLength}
	installed, domain := service.installStream(entry, stream, entry.metadata, limit)
	if domain != nil {
		markImportError(domain)
		service.failAcquisition(entry, domain)
		return AcquiredArtifact{}, domain
	}
	service.publishAcquisitionSuccess(entry, installed)
	service.mu.Lock()
	result := entry.acquireResult
	service.mu.Unlock()
	return result.artifact, result.err
}

func (service *Service) effectiveImportLimit() int64 {
	if !service.capacity.unlimited && service.capacity.maxBytes > 0 && service.capacity.maxBytes < hardImportLimit {
		return service.capacity.maxBytes
	}
	return hardImportLimit
}

func (service *Service) ImportLimit() int64 { return service.effectiveImportLimit() }

func markImportError(domain *consolecore.Error) {
	if domain != nil && domain.Code == consolecore.CodeInvalidArtifact {
		available := false
		domain.Details.RawDownloadAvailable = &available
	}
}

type readerInput struct {
	reader         io.Reader
	declaredLength int64
}

func (stream *readerInput) Body() io.Reader       { return stream.reader }
func (stream *readerInput) DeclaredLength() int64 { return stream.declaredLength }
func (stream *readerInput) Close() error {
	if closer, ok := stream.reader.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}
