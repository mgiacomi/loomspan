package traceanalysis

import (
	"context"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/evidence"
)

// FrameOrder enumerates the supported frame query orderings.
type FrameOrder string

const (
	// FrameOrderCanonical preserves first-open record order.
	FrameOrderCanonical FrameOrder = "CANONICAL"
	// FrameOrderDurationDesc orders frames by inclusive duration descending,
	// with frame ID as the stable tie-breaker.
	FrameOrderDurationDesc FrameOrder = "DURATION_DESC"
	// FrameOrderUsageDesc orders frames by inclusive total usage descending,
	// with frame ID as the stable tie-breaker.
	FrameOrderUsageDesc FrameOrder = "USAGE_DESC"
)

type FrameProjection string

const (
	FrameProjectionCompact  FrameProjection = "COMPACT"
	FrameProjectionDetailed FrameProjection = "DETAILED"
)

// FrameFilter selects which frames a frame query returns. Cross-reference
// fields match facts explicitly recorded against that exact frame; they are
// never inferred from adjacency or propagated from descendants. Empty fields
// match all frames. When multiple fields are set, all must match (AND).
type FrameFilter struct {
	FrameIDs      []string `json:"frameIds,omitempty"`
	ParentFrameID string   `json:"parentFrameId,omitempty"`
	FrameType     string   `json:"frameType,omitempty"`
	Route         string   `json:"route,omitempty"`
	SkillName     string   `json:"skillName,omitempty"`
	// Outcome matches the explicit FRAME_CLOSED status metadata.
	Outcome          string `json:"outcome,omitempty"`
	AttemptID        string `json:"attemptId,omitempty"`
	RetrySequenceID  string `json:"retrySequenceId,omitempty"`
	ValidationStatus string `json:"validationStatus,omitempty"`
	FailureID        string `json:"failureId,omitempty"`
}

// FrameQuery is a bounded, continuable frame query.
type FrameQuery struct {
	Handle     artifact.Handle
	Filter     FrameFilter     `json:"filter"`
	Order      FrameOrder      `json:"order"`
	Projection FrameProjection `json:"projection"`
	PageSize   int             `json:"pageSize"`
	Cursor     string          `json:"cursor,omitempty"`
	// Admit is an internal server-owned complete-item admission policy. It is
	// deliberately excluded from continuation fingerprints.
	Admit func(FrameSummary) bool `json:"-"`
}

// frameQueryCanonical is the canonical projection of a FrameQuery used for
// fingerprinting. It omits the handle and cursor (which are per-call
// identity/progress, not query meaning).
type frameQueryCanonical struct {
	Filter     FrameFilter     `json:"filter"`
	Order      FrameOrder      `json:"order"`
	Projection FrameProjection `json:"projection"`
	PageSize   int             `json:"pageSize"`
}

// QueryFrames returns a finite, continuable page of frame summaries matching
// the filter and order. One artifact lease is acquired for the call and closed
// after the result is materialized.
func (service *Service) QueryFrames(ctx context.Context, scopeID evidence.Reference, query FrameQuery) (Page[FrameSummary], *consolecore.Error) {
	pageSize, domain := validatePageSize(scopeID, query.PageSize)
	if domain != nil {
		return Page[FrameSummary]{}, domain
	}
	if query.Order == "" {
		query.Order = FrameOrderCanonical
	}
	if query.Projection == "" {
		query.Projection = FrameProjectionCompact
	}
	if query.Projection != FrameProjectionCompact && query.Projection != FrameProjectionDetailed {
		return Page[FrameSummary]{}, consolecore.NewError(consolecore.CodeInvalidArgument, "The frame projection is not supported.", scopeID.ID(), consolecore.Details{}, nil)
	}
	if query.Projection == FrameProjectionCompact && query.Order != FrameOrderCanonical {
		return Page[FrameSummary]{}, consolecore.NewError(consolecore.CodeInvalidArgument, "Compact frames require canonical order.", scopeID.ID(), consolecore.Details{}, nil)
	}
	if _, valid := frameIndexForOrder(query.Order); !valid {
		return Page[FrameSummary]{}, consolecore.NewError(consolecore.CodeInvalidArgument,
			"The frame order is not supported.", scopeID.ID(), consolecore.Details{}, nil)
	}
	if query.Filter.FrameType != "" {
		if _, valid := knownFrameType(query.Filter.FrameType); !valid {
			return Page[FrameSummary]{}, consolecore.NewError(consolecore.CodeInvalidArgument,
				"The frame type filter is not supported.", scopeID.ID(), consolecore.Details{}, nil)
		}
	}
	if query.Filter.Outcome != "" {
		if !containsClosedValue(FrameOutcomeValues(), query.Filter.Outcome) {
			return Page[FrameSummary]{}, consolecore.NewError(consolecore.CodeInvalidArgument, "The frame outcome filter is not supported.", scopeID.ID(), consolecore.Details{}, nil)
		}
	}
	if query.Filter.ValidationStatus != "" && !knownValidationStatus(query.Filter.ValidationStatus) {
		return Page[FrameSummary]{}, consolecore.NewError(consolecore.CodeInvalidArgument, "The validation status filter is not supported.", scopeID.ID(), consolecore.Details{}, nil)
	}
	for _, frameID := range query.Filter.FrameIDs {
		if frameID == "" {
			return Page[FrameSummary]{}, consolecore.NewError(consolecore.CodeInvalidArgument,
				"Frame ID filters must not be blank.", scopeID.ID(), consolecore.Details{}, nil)
		}
	}
	query.Filter.FrameIDs = normalizeStringSet(query.Filter.FrameIDs)
	fingerprint, err := canonicalizeRequest(frameQueryCanonical{
		Filter:     query.Filter,
		Order:      query.Order,
		Projection: query.Projection,
		PageSize:   pageSize,
	})
	if err != nil {
		return Page[FrameSummary]{}, consolecore.NewError(consolecore.CodeConsoleError,
			"The frame query could not be canonicalized.", scopeID.ID(), consolecore.Details{}, err)
	}

	startPosition := 0
	var decodedCursor cursor

	lease, domain := service.leaseForHandle(scopeID, query.Handle)
	if domain != nil {
		return Page[FrameSummary]{}, domain
	}
	success := false
	defer func() { _ = lease.Close(success) }()
	if query.Cursor != "" {
		c, start, cursorDomain := prepareCursor(query.Cursor, ownerCursorKey(lease.Owner()), scopeID.ID(), cursorOpFrames)
		if cursorDomain != nil {
			return Page[FrameSummary]{}, cursorDomain
		}
		decodedCursor = c
		startPosition = start
	}

	if decodedCursor.Schema != "" {
		if d := validateCursorFingerprint(decodedCursor, fingerprint, ownerCursorKey(lease.Owner()), scopeID.ID(), query.Handle); d != nil {
			return Page[FrameSummary]{}, d
		}
	}

	if e := ctx.Err(); e != nil {
		return Page[FrameSummary]{}, canceledError(e)
	}

	componentName, _ := frameIndexForOrder(query.Order)
	items := make([]FrameSummary, 0, pageSize)
	traceCtx, err := traceContextForLease(lease, scopeID, query.Handle)
	if err != nil {
		return Page[FrameSummary]{}, storageError(scopeID.ID(), err)
	}
	idSet := make(map[string]bool, len(query.Filter.FrameIDs))
	for _, id := range query.Filter.FrameIDs {
		idSet[id] = true
	}
	var nextPosition int64
	currentPosition := int64(startPosition)
	hasMore := false
	var canceled *consolecore.Error
	err = scanFactRowsContext[persistedFrameResult](ctx, lease, componentName, int64(startPosition), func(frame persistedFrameResult, next int64) bool {
		if e := ctx.Err(); e != nil {
			canceled = canceledError(e)
			return true
		}
		if !frameMatchesFilter(frame.frameResult, query.Filter, idSet) {
			currentPosition = next
			return false
		}
		if len(items) == pageSize {
			hasMore = true
			return true
		}
		summary := frameResultToSummary(frame.frameResult, traceCtx)
		summary.GapKinds = append([]string{}, frame.GapKinds...)
		summary.UncertaintyKinds = append([]string{}, frame.UncertaintyKinds...)
		populateFrameCounts(&summary)
		if query.Projection == FrameProjectionCompact {
			compactFrameSummary(&summary)
		}
		if query.Admit != nil && !query.Admit(summary) {
			if len(items) == 0 {
				canceled = consolecore.NewError(consolecore.CodeLimitExceeded, "One frame exceeds the safe response budget. Use COMPACT or narrow the filters.", scopeID.ID(), consolecore.Details{}, nil)
				return true
			}
			hasMore = true
			nextPosition = currentPosition
			return true
		}
		items = append(items, summary)
		nextPosition = next
		currentPosition = next
		return false
	})
	if canceled != nil {
		return Page[FrameSummary]{}, canceled
	}
	if err != nil {
		return Page[FrameSummary]{}, storageError(scopeID.ID(), err)
	}
	var nextCursor string
	if hasMore {
		nextCursor, err = encodePositionCursor(cursorOpFrames, ownerCursorKey(lease.Owner()), query.Handle, fingerprint, nextPosition)
		if err != nil {
			return Page[FrameSummary]{}, cursorError(scopeID.ID(), err)
		}
	}
	success = true
	return Page[FrameSummary]{
		Context:    traceCtx,
		Items:      items,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

func populateFrameCounts(summary *FrameSummary) {
	summary.DirectAttemptCount = len(summary.AttemptIDs)
	summary.DirectValidationCount = len(summary.ValidationStatuses)
	summary.DirectFailureCount = len(summary.FailureIDs)
	summary.GapCount = len(summary.GapKinds)
	summary.UncertaintyCount = len(summary.UncertaintyKinds)
}

func compactFrameSummary(summary *FrameSummary) {
	summary.InclusiveDurationMillis = nil
	summary.SelfDurationMillis = nil
	summary.DirectUsage = Usage{}
	summary.DirectUsageComplete = false
	summary.DescendantUsage = Usage{}
	summary.DescendantUsageComplete = false
	summary.InclusiveUsage = Usage{}
	summary.InclusiveUsageComplete = false
	summary.SkillNames = nil
	summary.AttemptIDs = nil
	summary.RetrySequenceIDs = nil
	summary.ValidationStatuses = nil
	summary.FailureIDs = nil
	summary.GapKinds = nil
	summary.UncertaintyKinds = nil
}

// frameMatchesFilter reports whether a frame matches all set filter fields.
func frameMatchesFilter(fr frameResult, f FrameFilter, idSet map[string]bool) bool {
	if len(idSet) > 0 && !idSet[fr.FrameID] {
		return false
	}
	if f.ParentFrameID != "" {
		if fr.ParentFrameID == nil || *fr.ParentFrameID != f.ParentFrameID {
			return false
		}
	}
	if f.FrameType != "" && fr.FrameType != f.FrameType {
		return false
	}
	if f.Route != "" && fr.Route != f.Route {
		return false
	}
	if !containsString(fr.SkillNames, f.SkillName) ||
		!matchesOptionalString(fr.Outcome, f.Outcome) ||
		!containsString(fr.AttemptIDs, f.AttemptID) ||
		!containsString(fr.RetrySequenceIDs, f.RetrySequenceID) ||
		!containsString(fr.ValidationStatuses, f.ValidationStatus) ||
		!containsString(fr.FailureIDs, f.FailureID) {
		return false
	}
	return true
}

func matchesOptionalString(value *string, wanted string) bool {
	return wanted == "" || value != nil && *value == wanted
}

func containsString(values []string, wanted string) bool {
	if wanted == "" {
		return true
	}
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func frameIndexForOrder(order FrameOrder) (component, bool) {
	switch order {
	case FrameOrderCanonical:
		return ComponentFrameIndex, true
	case FrameOrderDurationDesc:
		return ComponentFrameDuration, true
	case FrameOrderUsageDesc:
		return ComponentFrameUsage, true
	default:
		return "", false
	}
}

// frameResultToSummary converts an internal frameResult to a FrameSummary.
func frameResultToSummary(f frameResult, ctx TraceContext) FrameSummary {
	return FrameSummary{
		Context:                 ctx,
		FrameID:                 f.FrameID,
		ParentFrameID:           f.ParentFrameID,
		ChildFrameIDs:           append([]string(nil), f.ChildFrameIDs...),
		FrameType:               f.FrameType,
		Route:                   f.Route,
		OpenedTimestampMillis:   f.OpenedTimestampMillis,
		ClosedTimestampMillis:   f.ClosedTimestampMillis,
		InclusiveDurationMillis: f.InclusiveDurationMillis,
		SelfDurationMillis:      f.SelfDurationMillis,
		DirectUsage:             f.DirectUsage,
		DirectUsageComplete:     f.DirectUsageComplete,
		DescendantUsage:         f.DescendantUsage,
		DescendantUsageComplete: f.DescendantUsageComplete,
		InclusiveUsage:          f.InclusiveUsage,
		InclusiveUsageComplete:  f.InclusiveUsageComplete,
		SkillNames:              append([]string(nil), f.SkillNames...),
		Outcome:                 copyStringPointer(f.Outcome),
		AttemptIDs:              append([]string(nil), f.AttemptIDs...),
		RetrySequenceIDs:        append([]string(nil), f.RetrySequenceIDs...),
		ValidationStatuses:      append([]string(nil), f.ValidationStatuses...),
		FailureIDs:              append([]string(nil), f.FailureIDs...),
		DirectRetryCount:        f.DirectRetryCount,
		GapKinds:                []string{},
		UncertaintyKinds:        []string{},
	}
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

// errCursorOpMismatch is returned when a cursor's operation does not match the
// query method that received it.
var errCursorOpMismatch = errCursorOp("cursor operation does not match query")

type errCursorOp string

func (e errCursorOp) Error() string { return string(e) }
