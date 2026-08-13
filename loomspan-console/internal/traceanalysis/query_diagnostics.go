package traceanalysis

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/evidence"
)

// GetFailureDiagnostic deliberately returns one complete, canonically bounded diagnostic.
func (service *Service) GetFailureDiagnostic(ctx context.Context, scopeID evidence.Reference, req FailureDiagnosticRequest) (FailureDiagnostic, *consolecore.Error) {
	if req.FailureID == "" || req.Ordinal < 0 {
		return FailureDiagnostic{}, invalidityError(CategoryUnsupportedValue, scopeID.ID())
	}
	lease, domain := service.leaseForHandle(scopeID, req.Handle)
	if domain != nil {
		return FailureDiagnostic{}, domain
	}
	success := false
	defer func() { _ = lease.Close(success) }()
	if err := ctx.Err(); err != nil {
		return FailureDiagnostic{}, canceledError(err)
	}
	var fact failureSummary
	found := false
	err := scanFactRows[failureSummary](lease, ComponentFailureIndex, 0, func(row failureSummary, _ int64) bool {
		if row.FailureID == req.FailureID {
			fact, found = row, true
			return true
		}
		return false
	})
	if err != nil {
		return FailureDiagnostic{}, storageError(scopeID.ID(), err)
	}
	if !found || req.Ordinal >= len(fact.Diagnostics) {
		return FailureDiagnostic{}, invalidityError(CategoryUnsupportedValue, scopeID.ID())
	}
	var raw []byte
	if fact.PayloadID != "" {
		desc, err := findPayloadDescriptorInIndex(lease, fact.PayloadID)
		if err != nil {
			return FailureDiagnostic{}, storageError(scopeID.ID(), err)
		}
		if desc == nil {
			return FailureDiagnostic{}, storageError(scopeID.ID(), fmt.Errorf("failure payload descriptor not found"))
		}
		raw, err = readInlinePayload(ctx, lease, *desc)
		if err != nil {
			if ctx.Err() != nil {
				return FailureDiagnostic{}, canceledError(ctx.Err())
			}
			return FailureDiagnostic{}, storageError(scopeID.ID(), err)
		}
	} else {
		raw, err = readFailureRecordData(ctx, lease, fact.Sequence)
		if err != nil {
			if ctx.Err() != nil {
				return FailureDiagnostic{}, canceledError(ctx.Err())
			}
			return FailureDiagnostic{}, storageError(scopeID.ID(), err)
		}
	}
	if err := ctx.Err(); err != nil {
		return FailureDiagnostic{}, canceledError(err)
	}
	data, err := decodeFailureData(ctx, raw)
	if err != nil {
		if ctx.Err() != nil {
			return FailureDiagnostic{}, canceledError(ctx.Err())
		}
		return FailureDiagnostic{}, storageError(scopeID.ID(), fmt.Errorf("failure diagnostic payload mismatch"))
	}
	if req.Ordinal >= len(data.Diagnostics) {
		return FailureDiagnostic{}, storageError(scopeID.ID(), fmt.Errorf("failure diagnostic payload mismatch"))
	}
	selected := data.Diagnostics[req.Ordinal]
	desc := fact.Diagnostics[req.Ordinal]
	if selected.Text == nil || selected.Truncated == nil || desc.Ordinal != req.Ordinal || selected.Kind != desc.Kind || selected.ContentType != desc.ContentType || *selected.Truncated != desc.Truncated || selected.CaptureLimitBytes != desc.CaptureLimitBytes || len([]byte(*selected.Text)) != desc.DecodedBytes || len([]byte(*selected.Text)) > desc.CaptureLimitBytes || len([]byte(*selected.Text)) > 1<<20 {
		return FailureDiagnostic{}, storageError(scopeID.ID(), fmt.Errorf("failure diagnostic descriptor mismatch"))
	}
	traceCtx, err := traceContextForLease(lease, scopeID, req.Handle)
	if err != nil {
		return FailureDiagnostic{}, storageError(scopeID.ID(), err)
	}
	if err := ctx.Err(); err != nil {
		return FailureDiagnostic{}, canceledError(err)
	}
	success = true
	return FailureDiagnostic{Context: traceCtx, FailureID: fact.FailureID, Descriptor: desc, Text: *selected.Text}, nil
}

func decodeFailureData(ctx context.Context, raw []byte) (failureData, error) {
	var data failureData
	decoder := json.NewDecoder(&contextChunkReader{ctx: ctx, reader: bytes.NewReader(raw)})
	if err := decoder.Decode(&data); err != nil {
		return failureData{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return failureData{}, fmt.Errorf("unexpected trailing JSON value")
		}
		return failureData{}, err
	}
	if err := ctx.Err(); err != nil {
		return failureData{}, err
	}
	return data, nil
}

func readFailureRecordData(ctx context.Context, lease *artifact.Lease, sequence int64) ([]byte, error) {
	idx, err := lease.OpenComponent(artifact.ComponentName(ComponentRecordIndex))
	if err != nil {
		return nil, err
	}
	defer idx.Close()
	stopIndexCancellationClose := closeReaderOnCancellation(ctx, idx)
	defer stopIndexCancellationClose()
	rawReader, err := lease.OpenComponent(artifact.ComponentRawArtifact)
	if err != nil {
		return nil, err
	}
	defer rawReader.Close()
	stopRawCancellationClose := closeReaderOnCancellation(ctx, rawReader)
	defer stopRawCancellationClose()
	for pos := int64(0); ; pos++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		row, err := readRecordIndexRowAt(idx, pos)
		if err == io.EOF {
			return nil, fmt.Errorf("failure record not found")
		}
		if err != nil {
			return nil, err
		}
		if row.Sequence != sequence {
			continue
		}
		line, err := readRawRecordBytesFrom(rawReader, row)
		if err != nil {
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		rec, domain := decodeRecord(line, RawAddress{})
		if domain != nil {
			return nil, fmt.Errorf("decode failure record")
		}
		return rec.Data, nil
	}
}
