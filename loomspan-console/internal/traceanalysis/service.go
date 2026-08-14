package traceanalysis

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/evidence"
)

// Service is the transport-neutral trace-analysis query service. It is both
// the artifact.Processor (injected into artifact.Service) and the adapter-
// facing query service consumed by PR 14 (browser) and PR 18 (MCP). One
// instance is constructed by the console composition root and retained for
// the process lifetime.
//
// Query methods acquire one artifact lease per call, open only the required
// bundle components, check context cancellation during scans, and close the
// lease successfully only after the complete result is materialized. No
// partial semantic tree/summary is returned from an invalid bundle; bundle
// corruption after publication is a storage/console failure.
type Service struct {
	processor *Processor
	artifacts *artifact.Service
}

// NewService creates the trace-analysis service. The service implements
// artifact.Processor (via Processor) and is also the adapter-facing query
// service. The artifact service dependency is required for query methods to
// acquire leases by handle; it may be nil initially and wired later via
// SetArtifactService when the artifact service itself needs this service as
// its processor (circular dependency at construction time).
func NewService(artifacts *artifact.Service) *Service {
	return &Service{
		processor: New(),
		artifacts: artifacts,
	}
}

// SetArtifactService wires the artifact service after construction. This is
// used by the console composition root, which creates the trace-analysis
// service first (to use as the artifact processor), then creates the artifact
// service, then wires the artifact service back here so query methods can
// acquire leases.
func (service *Service) SetArtifactService(artifacts *artifact.Service) {
	service.artifacts = artifacts
}

// Process implements artifact.Processor by delegating to the embedded
// Processor. This lets the console composition root inject one
// traceanalysis.Service instance as both the processor and the query service.
func (service *Service) Process(req artifact.ProcessRequest) (artifact.ProcessResult, *consolecore.Error) {
	return service.processor.Process(req)
}

// Compile-time assertion that Service satisfies artifact.Processor.
var _ artifact.Processor = (*Service)(nil)

// PreflightImport delegates the bounded import header pass to the same
// processor instance used for complete validation.
func (service *Service) PreflightImport(ctx context.Context, raw io.Reader) (artifact.ImportPreflight, *consolecore.Error) {
	return service.processor.PreflightImport(ctx, raw)
}

var _ artifact.ImportProcessor = (*Service)(nil)

// leaseForHandle acquires a lease for the given handle within the current
// scope. It returns the lease and a domain error if the scope changed, the
// handle expired, or the service is shutting down.
func (service *Service) leaseForHandle(scopeID evidence.Reference, handle artifact.Handle) (*artifact.Lease, *consolecore.Error) {
	if service.artifacts == nil {
		return nil, consolecore.NewError(consolecore.CodeConsoleError,
			"The trace analysis service is not wired to an artifact service.",
			scopeID.ID(), consolecore.Details{}, nil)
	}
	lease, domain := service.artifacts.Use(scopeID, handle)
	if domain != nil {
		return nil, domain
	}
	return lease, nil
}

func (service *Service) leaseForCursor(scopeID evidence.Reference, handle artifact.Handle, token string, op cursorOp) (*artifact.Lease, cursor, int, *consolecore.Error) {
	lease, domain := service.leaseForHandle(scopeID, handle)
	if domain != nil {
		return nil, cursor{}, 0, domain
	}
	if token == "" {
		return lease, cursor{}, 0, nil
	}
	decoded, start, domain := prepareCursor(token, ownerCursorKey(lease.Owner()), scopeID.ID(), op)
	if domain != nil {
		_ = lease.Close(false)
		return nil, cursor{}, 0, domain
	}
	return lease, decoded, start, nil
}

// readManifest reads and parses the manifest component from a lease.
func readManifest(lease *artifact.Lease) (manifest, error) {
	reader, err := lease.OpenComponent(ComponentManifest)
	if err != nil {
		return manifest{}, err
	}
	defer reader.Close()
	var m manifest
	dec := json.NewDecoder(reader)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return manifest{}, fmt.Errorf("parse manifest: %w", err)
	}
	var trailing json.RawMessage
	if err := dec.Decode(&trailing); err != io.EOF {
		return manifest{}, fmt.Errorf("parse manifest trailing content")
	}
	if m.Schema != manifestSchemaV1 || m.TraceID == "" || m.SessionID == "" ||
		m.RecordCount < 0 || m.FrameCount < 0 || m.AttemptCount < 0 || m.RetryCount < 0 ||
		m.ValidationCount < 0 || m.FailureCount < 0 || m.PayloadCount < 0 ||
		m.GapCount < 0 || m.UncertaintyCount < 0 {
		return manifest{}, fmt.Errorf("manifest invariants are invalid")
	}
	return m, nil
}

// scanFactRows streams a fact index in persisted order. start is the byte
// offset of the next length-prefixed row. Continuations therefore seek
// directly to their next row instead of rereading every earlier page.
func scanFactRows[T any](lease *artifact.Lease, name component, start int64, visit func(row T, next int64) bool) error {
	return scanFactRowsContext(context.Background(), lease, name, start, visit)
}

// scanFactRowsContext is the cancellable form used by page enrichment and
// other request work that may traverse a fact component.
func scanFactRowsContext[T any](ctx context.Context, lease *artifact.Lease, name component, start int64, visit func(row T, next int64) bool) error {
	reader, err := lease.OpenComponent(artifact.ComponentName(name))
	if err != nil {
		return err
	}
	defer reader.Close()
	stopCancellationClose := closeReaderOnCancellation(ctx, reader)
	defer stopCancellationClose()
	if start < 0 {
		return fmt.Errorf("negative fact index offset %d", start)
	}
	if _, err := reader.Seek(start, io.SeekStart); err != nil {
		return err
	}
	position := start
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		row, err := readLengthPrefixed(reader)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read fact row from %s: %w", name, err)
		}
		position += int64(4 + len(row))
		var value T
		if err := json.Unmarshal(row, &value); err != nil {
			return fmt.Errorf("parse fact row from %s: %w", name, err)
		}
		if visit(value, position) {
			return nil
		}
	}
}

func normalizeStringSet(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	normalized := append([]string(nil), values...)
	sort.Strings(normalized)
	out := normalized[:0]
	for _, value := range normalized {
		if len(out) == 0 || value != out[len(out)-1] {
			out = append(out, value)
		}
	}
	return out
}

func readRecordIndexRowAt(reader artifact.ComponentReader, index int64) (recordIndexRow, error) {
	if index < 0 {
		return recordIndexRow{}, fmt.Errorf("negative record index %d", index)
	}
	if _, err := reader.Seek(index*recordIndexRowWidth, io.SeekStart); err != nil {
		return recordIndexRow{}, err
	}
	buf := make([]byte, recordIndexRowWidth)
	if _, err := io.ReadFull(reader, buf); err != nil {
		return recordIndexRow{}, err
	}
	return readRecordIndexRow(buf), nil
}

func lowerBoundRecordSequence(reader artifact.ComponentReader, count, sequence int64) (int64, error) {
	low, high := int64(0), count
	for low < high {
		mid := low + (high-low)/2
		row, err := readRecordIndexRowAt(reader, mid)
		if err != nil {
			return 0, err
		}
		if row.Sequence < sequence {
			low = mid + 1
		} else {
			high = mid
		}
	}
	return low, nil
}

func readRawRecordBytesFrom(reader artifact.ComponentReader, row recordIndexRow) ([]byte, error) {
	if _, err := reader.Seek(row.Offset, io.SeekStart); err != nil {
		return nil, err
	}
	buf := make([]byte, row.Length)
	if _, err := io.ReadFull(reader, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func traceContextForLease(lease *artifact.Lease, scopeID evidence.Reference, handle artifact.Handle) (TraceContext, error) {
	m, err := readManifest(lease)
	if err != nil {
		return TraceContext{}, err
	}
	return TraceContext{Evidence: scopeID, Handle: handle, TraceID: m.TraceID, SessionID: m.SessionID}, nil
}

// validatePageSize applies the default and maximum page size bounds. It
// returns the resolved page size or a LIMIT_EXCEEDED domain error.
func validatePageSize(scopeID evidence.Reference, requested int) (int, *consolecore.Error) {
	if requested == 0 {
		return defaultPageSize, nil
	}
	if requested < 0 {
		return 0, consolecore.NewError(consolecore.CodeInvalidArgument,
			"The page size must be positive.",
			scopeID.ID(), consolecore.Details{}, nil)
	}
	if requested > maxPageSize {
		return 0, consolecore.NewError(consolecore.CodeLimitExceeded,
			"The requested page size exceeds the maximum allowed size.",
			scopeID.ID(), consolecore.Details{LimitName: "pageSize", LimitValue: int64(maxPageSize)}, nil)
	}
	return requested, nil
}

// validateRangeSize applies the default and maximum range size bounds.
func validateRangeSize(scopeID evidence.Reference, requested int) (int, *consolecore.Error) {
	if requested == 0 {
		return defaultRangeBytes, nil
	}
	if requested < 0 {
		return 0, consolecore.NewError(consolecore.CodeInvalidArgument,
			"The range size must be positive.",
			scopeID.ID(), consolecore.Details{}, nil)
	}
	if requested > maxRangeBytes {
		return 0, consolecore.NewError(consolecore.CodeLimitExceeded,
			"The requested range size exceeds the maximum allowed size.",
			scopeID.ID(), consolecore.Details{LimitName: "rangeBytes", LimitValue: int64(maxRangeBytes)}, nil)
	}
	return requested, nil
}

// validateLiteralText validates a literal search query against the byte and
// code point limits.
func validateLiteralText(scopeID evidence.Reference, text string) *consolecore.Error {
	if len(text) > maxLiteralTextBytes {
		return consolecore.NewError(consolecore.CodeLimitExceeded,
			"The search literal exceeds the maximum byte size.",
			scopeID.ID(), consolecore.Details{LimitName: "literalTextBytes", LimitValue: int64(maxLiteralTextBytes)}, nil)
	}
	if count := runeCount(text); count > maxLiteralTextRunes {
		return consolecore.NewError(consolecore.CodeLimitExceeded,
			"The search literal exceeds the maximum code point count.",
			scopeID.ID(), consolecore.Details{LimitName: "literalTextRunes", LimitValue: int64(maxLiteralTextRunes)}, nil)
	}
	return nil
}

// runeCount returns the number of Unicode code points in s.
func runeCount(s string) int {
	return len([]rune(s))
}

// sortFrameResults sorts frame results by the requested order with stable
// canonical/ID tie-breakers.
func sortFrameResults(frames []frameResult, order FrameOrder) {
	switch order {
	case FrameOrderDurationDesc:
		sort.SliceStable(frames, func(i, j int) bool {
			ai, bi := int64(0), int64(0)
			if frames[i].InclusiveDurationMillis != nil {
				ai = *frames[i].InclusiveDurationMillis
			}
			if frames[j].InclusiveDurationMillis != nil {
				bi = *frames[j].InclusiveDurationMillis
			}
			if ai != bi {
				return ai > bi
			}
			return frames[i].FrameID < frames[j].FrameID
		})
	case FrameOrderUsageDesc:
		sort.SliceStable(frames, func(i, j int) bool {
			ai := frames[i].InclusiveUsage.TotalUnits
			bi := frames[j].InclusiveUsage.TotalUnits
			if ai != bi {
				return ai > bi
			}
			return frames[i].FrameID < frames[j].FrameID
		})
	default: // FrameOrderCanonical is already persisted in first-open order.
	}
}
