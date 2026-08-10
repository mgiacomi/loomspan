package traceanalysis

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/target"
)

// SummaryRequest requests the top-level trace summary for an acquired artifact.
type SummaryRequest struct {
	Handle artifact.Handle
}

// GetSummary returns the complete neutral trace summary for one acquired
// artifact. The manifest stores counts and root references so this operation
// remains constant-memory regardless of trace size. Detailed gaps and
// uncertainties are exposed through their finite paged queries.
func (service *Service) GetSummary(ctx context.Context, scopeID target.ScopeID, req SummaryRequest) (TraceSummary, *consolecore.Error) {
	lease, domain := service.leaseForHandle(scopeID, req.Handle)
	if domain != nil {
		return TraceSummary{}, domain
	}
	success := false
	defer func() { _ = lease.Close(success) }()

	if e := ctx.Err(); e != nil {
		return TraceSummary{}, canceledError(e)
	}

	m, err := readManifest(lease)
	if err != nil {
		return TraceSummary{}, storageError(string(scopeID), err)
	}

	traceCtx := TraceContext{
		TargetScopeID: scopeID,
		Handle:        req.Handle,
		TraceID:       m.TraceID,
		SessionID:     m.SessionID,
	}

	// Read usage facts.
	usageFacts, err := readUsageFacts(lease)
	if err != nil {
		return TraceSummary{}, storageError(string(scopeID), err)
	}

	success = true
	return TraceSummary{
		Context:            traceCtx,
		Outcome:            m.Outcome,
		TerminalFailureID:  m.TerminalFailureID,
		ConfiguredLimits:   m.ConfiguredLimits,
		RecordCount:        m.RecordCount,
		FrameCount:         m.FrameCount,
		AttemptCount:       m.AttemptCount,
		RetryCount:         m.RetryCount,
		ValidationCount:    m.ValidationCount,
		FailureCount:       m.FailureCount,
		PayloadCount:       m.PayloadCount,
		GapCount:           m.GapCount,
		UncertaintyCount:   m.UncertaintyCount,
		RootFrameIDs:       append([]string(nil), m.RootFrameIDs...),
		AttributedUsage:    usageFacts.attributed,
		TerminalUsage:      usageFacts.terminal,
		UnattributedUsage:  usageFacts.unattributed,
		UnframedAttributed: usageFacts.unframedAttributed,
		UsageComplete:      m.UsageComplete,
	}, nil
}

// usageFacts holds the parsed usage index values.
type usageFacts struct {
	attributed         Usage
	unattributed       Usage
	unframedAttributed Usage
	terminal           Usage
}

// readUsageFacts reads and parses the usage index component.
func readUsageFacts(lease *artifact.Lease) (usageFacts, error) {
	reader, err := lease.OpenComponent(artifact.ComponentName(ComponentUsageIndex))
	if err != nil {
		return usageFacts{}, err
	}
	defer reader.Close()
	var out usageFacts
	seen := make(map[string]bool, 4)
	for {
		row, err := readLengthPrefixed(reader)
		if err != nil {
			if err == io.EOF {
				break
			}
			return usageFacts{}, err
		}
		var fact struct {
			Kind            string `json:"kind"`
			PromptUnits     *int64 `json:"promptUnits"`
			CompletionUnits *int64 `json:"completionUnits"`
			TotalUnits      *int64 `json:"totalUnits"`
		}
		dec := json.NewDecoder(bytes.NewReader(row))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&fact); err != nil {
			return usageFacts{}, err
		}
		if fact.Kind == "" || fact.PromptUnits == nil || fact.CompletionUnits == nil || fact.TotalUnits == nil || seen[fact.Kind] {
			return usageFacts{}, fmt.Errorf("usage index contains an incomplete or duplicate fact")
		}
		if *fact.PromptUnits < 0 || *fact.CompletionUnits < 0 || *fact.TotalUnits < 0 {
			return usageFacts{}, fmt.Errorf("usage index contains negative units")
		}
		seen[fact.Kind] = true
		u := Usage{
			PromptUnits:     *fact.PromptUnits,
			CompletionUnits: *fact.CompletionUnits,
			TotalUnits:      *fact.TotalUnits,
		}
		switch fact.Kind {
		case "ATTRIBUTED":
			out.attributed = u
		case "UNATTRIBUTED":
			out.unattributed = u
		case "UNFRAMED_ATTRIBUTED":
			out.unframedAttributed = u
		case "TERMINAL":
			out.terminal = u
		default:
			return usageFacts{}, fmt.Errorf("usage index contains unknown fact kind %q", fact.Kind)
		}
	}
	if len(seen) != 4 {
		return usageFacts{}, fmt.Errorf("usage index contains %d facts, expected 4", len(seen))
	}
	return out, nil
}

// AttemptQuery is a bounded, continuable attempt query.
type AttemptQuery struct {
	Handle   artifact.Handle
	PageSize int
	Cursor   string
}

func collectFactPage[T, U any](
	ctx context.Context,
	lease *artifact.Lease,
	scopeID target.ScopeID,
	name component,
	start int,
	pageSize int,
	project func(T) U,
) ([]U, int64, bool, *consolecore.Error) {
	items := make([]U, 0, pageSize)
	var nextPosition int64
	var canceled *consolecore.Error
	hasMore := false
	err := scanFactRows[T](lease, name, int64(start), func(row T, next int64) bool {
		if e := ctx.Err(); e != nil {
			canceled = canceledError(e)
			return true
		}
		if len(items) == pageSize {
			hasMore = true
			return true
		}
		items = append(items, project(row))
		nextPosition = next
		return false
	})
	if canceled != nil {
		return nil, 0, false, canceled
	}
	if err != nil {
		return nil, 0, false, storageError(string(scopeID), err)
	}
	return items, nextPosition, hasMore, nil
}

// QueryAttempts returns a finite, continuable page of attempt summaries.
func (service *Service) QueryAttempts(ctx context.Context, scopeID target.ScopeID, query AttemptQuery) (Page[AttemptSummary], *consolecore.Error) {
	pageSize, domain := validatePageSize(scopeID, query.PageSize)
	if domain != nil {
		return Page[AttemptSummary]{}, domain
	}
	fingerprint, err := canonicalizeRequest(struct {
		PageSize int `json:"pageSize"`
	}{PageSize: pageSize})
	if err != nil {
		return Page[AttemptSummary]{}, consolecore.NewError(consolecore.CodeConsoleError,
			"The attempt query could not be canonicalized.", string(scopeID), consolecore.Details{}, err)
	}
	lease, decodedCursor, startIdx, domain := service.leaseForCursor(scopeID, query.Handle, query.Cursor, cursorOpAttempts)
	if domain != nil {
		return Page[AttemptSummary]{}, domain
	}
	success := false
	defer func() { _ = lease.Close(success) }()
	if decodedCursor.Schema != "" {
		if d := validateCursorFingerprint(decodedCursor, fingerprint, string(scopeID), query.Handle); d != nil {
			return Page[AttemptSummary]{}, d
		}
	}
	if e := ctx.Err(); e != nil {
		return Page[AttemptSummary]{}, canceledError(e)
	}
	traceCtx, err := traceContextForLease(lease, scopeID, query.Handle)
	if err != nil {
		return Page[AttemptSummary]{}, storageError(string(scopeID), err)
	}
	items, nextPosition, hasMore, domain := collectFactPage[attemptResult, AttemptSummary](ctx, lease, scopeID, ComponentAttemptIndex, startIdx, pageSize, func(a attemptResult) AttemptSummary {
		return AttemptSummary{
			Context:               traceCtx,
			RetrySequenceID:       a.RetrySequenceID,
			AttemptID:             a.AttemptID,
			AttemptNumber:         a.AttemptNumber,
			AttemptReason:         a.AttemptReason,
			ProviderAttemptNumber: a.ProviderAttemptNumber,
			Outcome:               a.Outcome,
			FailureClassification: a.FailureClassification,
			FailureCategory:       a.FailureCategory,
			RetryDecision:         a.RetryDecision,
			RetryDelayMillis:      a.RetryDelayMillis,
			RetryDelaySource:      a.RetryDelaySource,
			HTTPStatus:            a.HTTPStatus,
			ProviderErrorType:     a.ProviderErrorType,
			ProviderErrorCode:     a.ProviderErrorCode,
			PayloadID:             a.PayloadID,
			Usage:                 a.Usage,
			UsageComplete:         a.UsageComplete,
		}
	})
	if domain != nil {
		return Page[AttemptSummary]{}, domain
	}
	var nextCursor string
	if hasMore {
		nextCursor, err = encodePositionCursor(cursorOpAttempts, string(scopeID), query.Handle, fingerprint, nextPosition)
		if err != nil {
			return Page[AttemptSummary]{}, cursorError(string(scopeID), err)
		}
	}
	success = true
	return Page[AttemptSummary]{Items: items, NextCursor: nextCursor, HasMore: hasMore}, nil
}

// RetryQuery is a bounded, continuable retry query.
type RetryQuery struct {
	Handle   artifact.Handle
	PageSize int
	Cursor   string
}

// QueryRetries returns a finite, continuable page of retry summaries.
func (service *Service) QueryRetries(ctx context.Context, scopeID target.ScopeID, query RetryQuery) (Page[RetrySummary], *consolecore.Error) {
	pageSize, domain := validatePageSize(scopeID, query.PageSize)
	if domain != nil {
		return Page[RetrySummary]{}, domain
	}
	fingerprint, err := canonicalizeRequest(struct {
		PageSize int `json:"pageSize"`
	}{PageSize: pageSize})
	if err != nil {
		return Page[RetrySummary]{}, consolecore.NewError(consolecore.CodeConsoleError,
			"The retry query could not be canonicalized.", string(scopeID), consolecore.Details{}, err)
	}
	lease, decodedCursor, startIdx, domain := service.leaseForCursor(scopeID, query.Handle, query.Cursor, cursorOpRetries)
	if domain != nil {
		return Page[RetrySummary]{}, domain
	}
	success := false
	defer func() { _ = lease.Close(success) }()
	if decodedCursor.Schema != "" {
		if d := validateCursorFingerprint(decodedCursor, fingerprint, string(scopeID), query.Handle); d != nil {
			return Page[RetrySummary]{}, d
		}
	}
	if e := ctx.Err(); e != nil {
		return Page[RetrySummary]{}, canceledError(e)
	}
	traceCtx, err := traceContextForLease(lease, scopeID, query.Handle)
	if err != nil {
		return Page[RetrySummary]{}, storageError(string(scopeID), err)
	}
	items, nextPosition, hasMore, domain := collectFactPage[retryResult, RetrySummary](ctx, lease, scopeID, ComponentRetryIndex, startIdx, pageSize, func(r retryResult) RetrySummary {
		return RetrySummary{
			Context:         traceCtx,
			RetrySequenceID: r.RetrySequenceID,
			Usage:           r.Usage,
			UsageComplete:   r.UsageComplete,
		}
	})
	if domain != nil {
		return Page[RetrySummary]{}, domain
	}
	var nextCursor string
	if hasMore {
		nextCursor, err = encodePositionCursor(cursorOpRetries, string(scopeID), query.Handle, fingerprint, nextPosition)
		if err != nil {
			return Page[RetrySummary]{}, cursorError(string(scopeID), err)
		}
	}
	success = true
	return Page[RetrySummary]{Items: items, NextCursor: nextCursor, HasMore: hasMore}, nil
}

// ValidationQuery is a bounded, continuable validation link query.
type ValidationQuery struct {
	Handle   artifact.Handle
	PageSize int
	Cursor   string
}

// QueryValidationLinks returns a finite, continuable page of validation summaries.
func (service *Service) QueryValidationLinks(ctx context.Context, scopeID target.ScopeID, query ValidationQuery) (Page[ValidationSummary], *consolecore.Error) {
	pageSize, domain := validatePageSize(scopeID, query.PageSize)
	if domain != nil {
		return Page[ValidationSummary]{}, domain
	}
	fingerprint, err := canonicalizeRequest(struct {
		PageSize int `json:"pageSize"`
	}{PageSize: pageSize})
	if err != nil {
		return Page[ValidationSummary]{}, consolecore.NewError(consolecore.CodeConsoleError,
			"The validation query could not be canonicalized.", string(scopeID), consolecore.Details{}, err)
	}
	lease, decodedCursor, startIdx, domain := service.leaseForCursor(scopeID, query.Handle, query.Cursor, cursorOpValidation)
	if domain != nil {
		return Page[ValidationSummary]{}, domain
	}
	success := false
	defer func() { _ = lease.Close(success) }()
	if decodedCursor.Schema != "" {
		if d := validateCursorFingerprint(decodedCursor, fingerprint, string(scopeID), query.Handle); d != nil {
			return Page[ValidationSummary]{}, d
		}
	}
	if e := ctx.Err(); e != nil {
		return Page[ValidationSummary]{}, canceledError(e)
	}
	traceCtx, err := traceContextForLease(lease, scopeID, query.Handle)
	if err != nil {
		return Page[ValidationSummary]{}, storageError(string(scopeID), err)
	}
	items, nextPosition, hasMore, domain := collectFactPage[validationLink, ValidationSummary](ctx, lease, scopeID, ComponentValidationIdx, startIdx, pageSize, func(v validationLink) ValidationSummary {
		return ValidationSummary{
			Context:         traceCtx,
			Status:          v.Status,
			RetrySequenceID: v.RetrySequenceID,
			AttemptID:       v.AttemptID,
			AttemptNumber:   v.AttemptNumber,
		}
	})
	if domain != nil {
		return Page[ValidationSummary]{}, domain
	}
	var nextCursor string
	if hasMore {
		nextCursor, err = encodePositionCursor(cursorOpValidation, string(scopeID), query.Handle, fingerprint, nextPosition)
		if err != nil {
			return Page[ValidationSummary]{}, cursorError(string(scopeID), err)
		}
	}
	success = true
	return Page[ValidationSummary]{Items: items, NextCursor: nextCursor, HasMore: hasMore}, nil
}

// FailureQuery is a bounded, continuable failure query.
type FailureQuery struct {
	Handle   artifact.Handle
	PageSize int
	Cursor   string
}

// QueryFailures returns a finite, continuable page of failure summaries.
func (service *Service) QueryFailures(ctx context.Context, scopeID target.ScopeID, query FailureQuery) (Page[FailureSummary], *consolecore.Error) {
	pageSize, domain := validatePageSize(scopeID, query.PageSize)
	if domain != nil {
		return Page[FailureSummary]{}, domain
	}
	fingerprint, err := canonicalizeRequest(struct {
		PageSize int `json:"pageSize"`
	}{PageSize: pageSize})
	if err != nil {
		return Page[FailureSummary]{}, consolecore.NewError(consolecore.CodeConsoleError,
			"The failure query could not be canonicalized.", string(scopeID), consolecore.Details{}, err)
	}
	lease, decodedCursor, startIdx, domain := service.leaseForCursor(scopeID, query.Handle, query.Cursor, cursorOpFailures)
	if domain != nil {
		return Page[FailureSummary]{}, domain
	}
	success := false
	defer func() { _ = lease.Close(success) }()
	if decodedCursor.Schema != "" {
		if d := validateCursorFingerprint(decodedCursor, fingerprint, string(scopeID), query.Handle); d != nil {
			return Page[FailureSummary]{}, d
		}
	}
	if e := ctx.Err(); e != nil {
		return Page[FailureSummary]{}, canceledError(e)
	}
	traceCtx, err := traceContextForLease(lease, scopeID, query.Handle)
	if err != nil {
		return Page[FailureSummary]{}, storageError(string(scopeID), err)
	}
	items, nextPosition, hasMore, domain := collectFactPage[failureSummary, FailureSummary](ctx, lease, scopeID, ComponentFailureIndex, startIdx, pageSize, func(f failureSummary) FailureSummary {
		return FailureSummary{
			Context: traceCtx, FailureID: f.FailureID, Terminal: f.Terminal,
			Sequence: f.Sequence, TimestampMillis: f.TimestampMillis, RecordType: f.RecordType,
			FrameID: f.FrameID, Route: f.Route, AttemptID: f.AttemptID,
			RetrySequenceID: f.RetrySequenceID, ValidationStatus: f.ValidationStatus,
			ExceptionType: f.ExceptionType, ContextSummary: f.ContextSummary, Diagnostics: f.Diagnostics,
		}
	})
	if domain != nil {
		return Page[FailureSummary]{}, domain
	}
	var nextCursor string
	if hasMore {
		nextCursor, err = encodePositionCursor(cursorOpFailures, string(scopeID), query.Handle, fingerprint, nextPosition)
		if err != nil {
			return Page[FailureSummary]{}, cursorError(string(scopeID), err)
		}
	}
	success = true
	return Page[FailureSummary]{Items: items, NextCursor: nextCursor, HasMore: hasMore}, nil
}

// failureSummary is the internal parsed failure fact.
type failureSummary struct {
	FailureID        string                 `json:"failureId"`
	Terminal         bool                   `json:"terminal"`
	Sequence         int64                  `json:"sequence"`
	TimestampMillis  int64                  `json:"timestampMillis"`
	RecordType       string                 `json:"recordType"`
	FrameID          string                 `json:"frameId,omitempty"`
	Route            string                 `json:"route,omitempty"`
	AttemptID        string                 `json:"attemptId,omitempty"`
	RetrySequenceID  string                 `json:"retrySequenceId,omitempty"`
	ValidationStatus string                 `json:"validationStatus,omitempty"`
	ExceptionType    string                 `json:"exceptionType,omitempty"`
	ContextSummary   string                 `json:"contextSummary,omitempty"`
	Diagnostics      []DiagnosticDescriptor `json:"diagnostics,omitempty"`
	PayloadID        string                 `json:"payloadId,omitempty"`
}

// PayloadQuery is a bounded, continuable payload descriptor query.
type PayloadQuery struct {
	Handle   artifact.Handle
	PageSize int
	Cursor   string
}

// QueryPayloads returns a finite, continuable page of payload descriptors.
func (service *Service) QueryPayloads(ctx context.Context, scopeID target.ScopeID, query PayloadQuery) (Page[PayloadDescriptor], *consolecore.Error) {
	pageSize, domain := validatePageSize(scopeID, query.PageSize)
	if domain != nil {
		return Page[PayloadDescriptor]{}, domain
	}
	fingerprint, err := canonicalizeRequest(struct {
		PageSize int `json:"pageSize"`
	}{PageSize: pageSize})
	if err != nil {
		return Page[PayloadDescriptor]{}, consolecore.NewError(consolecore.CodeConsoleError,
			"The payload query could not be canonicalized.", string(scopeID), consolecore.Details{}, err)
	}
	lease, decodedCursor, startIdx, domain := service.leaseForCursor(scopeID, query.Handle, query.Cursor, cursorOpPayloads)
	if domain != nil {
		return Page[PayloadDescriptor]{}, domain
	}
	success := false
	defer func() { _ = lease.Close(success) }()
	if decodedCursor.Schema != "" {
		if d := validateCursorFingerprint(decodedCursor, fingerprint, string(scopeID), query.Handle); d != nil {
			return Page[PayloadDescriptor]{}, d
		}
	}
	if e := ctx.Err(); e != nil {
		return Page[PayloadDescriptor]{}, canceledError(e)
	}
	traceCtx, err := traceContextForLease(lease, scopeID, query.Handle)
	if err != nil {
		return Page[PayloadDescriptor]{}, storageError(string(scopeID), err)
	}
	items, nextPosition, hasMore, domain := collectFactPage[payloadIndexRow, PayloadDescriptor](ctx, lease, scopeID, ComponentPayloadIndex, startIdx, pageSize, func(r payloadIndexRow) PayloadDescriptor {
		return PayloadDescriptor{
			Context:     traceCtx,
			PayloadID:   r.PayloadID,
			Sequence:    r.Sequence,
			ContentType: r.ContentType,
			ChunkCount:  r.ChunkCount,
			StoreOffset: r.StoreOffset,
			StoreLength: r.StoreLength,
		}
	})
	if domain != nil {
		return Page[PayloadDescriptor]{}, domain
	}
	var nextCursor string
	if hasMore {
		nextCursor, err = encodePositionCursor(cursorOpPayloads, string(scopeID), query.Handle, fingerprint, nextPosition)
		if err != nil {
			return Page[PayloadDescriptor]{}, cursorError(string(scopeID), err)
		}
	}
	success = true
	return Page[PayloadDescriptor]{Items: items, NextCursor: nextCursor, HasMore: hasMore}, nil
}

// GapQuery is a bounded, continuable gap query.
type GapQuery struct {
	Handle   artifact.Handle
	PageSize int
	Cursor   string
}

// QueryGaps returns a finite, continuable page of gaps.
func (service *Service) QueryGaps(ctx context.Context, scopeID target.ScopeID, query GapQuery) (Page[Gap], *consolecore.Error) {
	pageSize, domain := validatePageSize(scopeID, query.PageSize)
	if domain != nil {
		return Page[Gap]{}, domain
	}
	fingerprint, err := canonicalizeRequest(struct {
		PageSize int `json:"pageSize"`
	}{PageSize: pageSize})
	if err != nil {
		return Page[Gap]{}, consolecore.NewError(consolecore.CodeConsoleError,
			"The gap query could not be canonicalized.", string(scopeID), consolecore.Details{}, err)
	}
	lease, decodedCursor, startIdx, domain := service.leaseForCursor(scopeID, query.Handle, query.Cursor, cursorOpGaps)
	if domain != nil {
		return Page[Gap]{}, domain
	}
	success := false
	defer func() { _ = lease.Close(success) }()
	if decodedCursor.Schema != "" {
		if d := validateCursorFingerprint(decodedCursor, fingerprint, string(scopeID), query.Handle); d != nil {
			return Page[Gap]{}, d
		}
	}
	if e := ctx.Err(); e != nil {
		return Page[Gap]{}, canceledError(e)
	}
	traceCtx, err := traceContextForLease(lease, scopeID, query.Handle)
	if err != nil {
		return Page[Gap]{}, storageError(string(scopeID), err)
	}
	items, nextPosition, hasMore, domain := collectFactPage[gapResult, Gap](ctx, lease, scopeID, ComponentGapIndex, startIdx, pageSize, func(g gapResult) Gap {
		return Gap{Context: traceCtx, Kind: g.Kind, FrameID: g.FrameID, AttemptID: g.AttemptID}
	})
	if domain != nil {
		return Page[Gap]{}, domain
	}
	var nextCursor string
	if hasMore {
		nextCursor, err = encodePositionCursor(cursorOpGaps, string(scopeID), query.Handle, fingerprint, nextPosition)
		if err != nil {
			return Page[Gap]{}, cursorError(string(scopeID), err)
		}
	}
	success = true
	return Page[Gap]{Items: items, NextCursor: nextCursor, HasMore: hasMore}, nil
}

// UncertaintyQuery is a bounded, continuable uncertainty query.
type UncertaintyQuery struct {
	Handle   artifact.Handle
	PageSize int
	Cursor   string
}

// QueryUncertainties returns a finite page of explicit calculation
// uncertainties without materializing the complete uncertainty index.
func (service *Service) QueryUncertainties(ctx context.Context, scopeID target.ScopeID, query UncertaintyQuery) (Page[Uncertainty], *consolecore.Error) {
	pageSize, domain := validatePageSize(scopeID, query.PageSize)
	if domain != nil {
		return Page[Uncertainty]{}, domain
	}
	fingerprint, err := canonicalizeRequest(struct {
		PageSize int `json:"pageSize"`
	}{PageSize: pageSize})
	if err != nil {
		return Page[Uncertainty]{}, consolecore.NewError(consolecore.CodeConsoleError,
			"The uncertainty query could not be canonicalized.", string(scopeID), consolecore.Details{}, err)
	}
	lease, decodedCursor, startPosition, domain := service.leaseForCursor(scopeID, query.Handle, query.Cursor, cursorOpUncertainty)
	if domain != nil {
		return Page[Uncertainty]{}, domain
	}
	success := false
	defer func() { _ = lease.Close(success) }()
	if decodedCursor.Schema != "" {
		if d := validateCursorFingerprint(decodedCursor, fingerprint, string(scopeID), query.Handle); d != nil {
			return Page[Uncertainty]{}, d
		}
	}
	traceCtx, err := traceContextForLease(lease, scopeID, query.Handle)
	if err != nil {
		return Page[Uncertainty]{}, storageError(string(scopeID), err)
	}
	items, nextPosition, hasMore, domain := collectFactPage[uncertaintyResult, Uncertainty](ctx, lease, scopeID, ComponentUncertainty, startPosition, pageSize, func(u uncertaintyResult) Uncertainty {
		return Uncertainty{Context: traceCtx, Kind: u.Kind, FrameID: u.FrameID}
	})
	if domain != nil {
		return Page[Uncertainty]{}, domain
	}
	var nextCursor string
	if hasMore {
		nextCursor, err = encodePositionCursor(cursorOpUncertainty, string(scopeID), query.Handle, fingerprint, nextPosition)
		if err != nil {
			return Page[Uncertainty]{}, cursorError(string(scopeID), err)
		}
	}
	success = true
	return Page[Uncertainty]{Items: items, NextCursor: nextCursor, HasMore: hasMore}, nil
}

// GetUsageBreakdown returns the component-wise usage breakdown for a trace.
func (service *Service) GetUsageBreakdown(ctx context.Context, scopeID target.ScopeID, handle artifact.Handle) (UsageBreakdown, *consolecore.Error) {
	lease, domain := service.leaseForHandle(scopeID, handle)
	if domain != nil {
		return UsageBreakdown{}, domain
	}
	success := false
	defer func() { _ = lease.Close(success) }()
	if e := ctx.Err(); e != nil {
		return UsageBreakdown{}, canceledError(e)
	}
	facts, err := readUsageFacts(lease)
	if err != nil {
		return UsageBreakdown{}, storageError(string(scopeID), err)
	}
	traceCtx, err := traceContextForLease(lease, scopeID, handle)
	if err != nil {
		return UsageBreakdown{}, storageError(string(scopeID), err)
	}
	success = true
	return UsageBreakdown{
		Context:            traceCtx,
		Attributed:         facts.attributed,
		Unattributed:       facts.unattributed,
		UnframedAttributed: facts.unframedAttributed,
		Terminal:           facts.terminal,
	}, nil
}
