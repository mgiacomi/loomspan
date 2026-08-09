package browserapi

import (
	"net/http"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/target"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/traceanalysis"
)

const maxTraceAnalysisJSONBody = 8 * 1024

type analysisRequest struct {
	TraceID  string `json:"traceId"`
	PageSize int    `json:"pageSize,omitempty"`
	Cursor   string `json:"cursor,omitempty"`
}

// analysis resolves consistently while retaining precise domain errors.  The
// small wrapper also keeps every response scope-published.
func (router *Router) resolveAnalysis(response http.ResponseWriter, request *http.Request, body *analysisRequest) (target.Scope, artifact.Handle, bool) {
	if router.options.TraceAnalysis == nil || router.options.Artifacts == nil || router.options.Target == nil {
		writeError(response, 500, "CONSOLE_ERROR", "Trace analysis is unavailable.")
		return target.Scope{}, "", false
	}
	if body.TraceID == "" {
		writeError(response, 400, "INVALID_REQUEST", "A trace ID is required.")
		return target.Scope{}, "", false
	}
	scope, domain := router.options.Target.Capture()
	if domain != nil {
		writeDomainError(response, domain)
		return target.Scope{}, "", false
	}
	entry, domain := router.options.Artifacts.Lookup(scope.ID, body.TraceID)
	if domain != nil {
		writeDomainError(response, domain)
		return target.Scope{}, "", false
	}
	if !entry.LocalAvailable || entry.Handle == "" {
		writeDomainError(response, consolecore.NewError(consolecore.CodeNotFound, "The trace has not been acquired for analysis.", string(scope.ID), consolecore.Details{}, nil))
		return target.Scope{}, "", false
	}
	return scope, entry.Handle, true
}

func (router *Router) traceAnalysisSummary(w http.ResponseWriter, r *http.Request, _ string) {
	var b analysisRequest
	if decodeJSONLimit(r, &b, maxTraceAnalysisJSONBody) != nil {
		writeError(w, 400, "INVALID_REQUEST", "Invalid request.")
		return
	}
	s, h, ok := router.resolveAnalysis(w, r, &b)
	if !ok {
		return
	}
	v, d := router.options.TraceAnalysis.GetSummary(r.Context(), s.ID, traceanalysis.SummaryRequest{Handle: h})
	if d != nil {
		writeDomainError(w, d)
		return
	}
	router.writeScopedJSON(w, s.ID, summaryDTO{TargetScopeID: string(s.ID), TraceID: v.Context.TraceID, SessionID: v.Context.SessionID, Outcome: v.Outcome, TerminalFailureID: v.TerminalFailureID, ConfiguredLimits: configuredLimitsDTOValue(v.ConfiguredLimits), RecordCount: v.RecordCount, FrameCount: v.FrameCount, AttemptCount: v.AttemptCount, RetryCount: v.RetryCount, ValidationCount: v.ValidationCount, FailureCount: v.FailureCount, PayloadCount: v.PayloadCount, GapCount: v.GapCount, UncertaintyCount: v.UncertaintyCount, RootFrameIDs: append([]string{}, v.RootFrameIDs...), AttributedUsage: usageDTOValue(v.AttributedUsage), TerminalUsage: usageDTOValue(v.TerminalUsage), UnattributedUsage: usageDTOValue(v.UnattributedUsage), UnframedAttributedUsage: usageDTOValue(v.UnframedAttributed), UsageComplete: v.UsageComplete})
}
func (router *Router) traceAnalysisFrames(w http.ResponseWriter, r *http.Request, _ string) {
	var b struct {
		analysisRequest
		Filter traceanalysis.FrameFilter `json:"filter"`
		Order  traceanalysis.FrameOrder  `json:"order"`
	}
	if decodeJSONLimit(r, &b, maxTraceAnalysisJSONBody) != nil {
		writeError(w, 400, "INVALID_REQUEST", "Invalid request.")
		return
	}
	s, h, ok := router.resolveAnalysis(w, r, &b.analysisRequest)
	if !ok {
		return
	}
	v, d := router.options.TraceAnalysis.QueryFrames(r.Context(), s.ID, traceanalysis.FrameQuery{Handle: h, Filter: b.Filter, Order: b.Order, PageSize: b.PageSize, Cursor: b.Cursor})
	if d != nil {
		writeDomainError(w, d)
		return
	}
	items := make([]frameDTO, 0, len(v.Items))
	for _, x := range v.Items {
		items = append(items, frameDTO{FrameID: x.FrameID, ParentFrameID: x.ParentFrameID, ChildFrameIDs: append([]string{}, x.ChildFrameIDs...), FrameType: x.FrameType, Route: x.Route, OpenedTimestampMillis: x.OpenedTimestampMillis, ClosedTimestampMillis: x.ClosedTimestampMillis, InclusiveDurationMillis: x.InclusiveDurationMillis, SelfDurationMillis: x.SelfDurationMillis, DirectUsage: usageDTOValue(x.DirectUsage), DirectUsageComplete: x.DirectUsageComplete, DescendantUsage: usageDTOValue(x.DescendantUsage), DescendantUsageComplete: x.DescendantUsageComplete, InclusiveUsage: usageDTOValue(x.InclusiveUsage), InclusiveUsageComplete: x.InclusiveUsageComplete, SkillNames: append([]string{}, x.SkillNames...), Outcomes: append([]string{}, x.Outcomes...), AttemptIDs: append([]string{}, x.AttemptIDs...), RetrySequenceIDs: append([]string{}, x.RetrySequenceIDs...), ValidationStatuses: append([]string{}, x.ValidationStatuses...), FailureIDs: append([]string{}, x.FailureIDs...)})
	}
	router.writeScopedJSON(w, s.ID, pageDTO[frameDTO]{TargetScopeID: string(s.ID), Items: items, HasMore: v.HasMore, NextCursor: nullCursor(v.NextCursor)})
}
func (router *Router) traceAnalysisRecords(w http.ResponseWriter, r *http.Request, _ string) {
	var b struct {
		analysisRequest
		Filter         traceanalysis.RecordFilter         `json:"filter"`
		Representation traceanalysis.RecordRepresentation `json:"representation"`
	}
	if decodeJSONLimit(r, &b, maxTraceAnalysisJSONBody) != nil {
		writeError(w, 400, "INVALID_REQUEST", "Invalid request.")
		return
	}
	s, h, ok := router.resolveAnalysis(w, r, &b.analysisRequest)
	if !ok {
		return
	}
	v, d := router.options.TraceAnalysis.QueryRecords(r.Context(), s.ID, traceanalysis.RecordQuery{Handle: h, Filter: b.Filter, Representation: b.Representation, PageSize: b.PageSize, Cursor: b.Cursor})
	if d != nil {
		writeDomainError(w, d)
		return
	}
	items := make([]recordDTO, 0, len(v.Items))
	for _, x := range v.Items {
		items = append(items, recordDTO{Sequence: x.Sequence, Type: x.Type, FrameID: x.FrameID, ParentFrameID: x.ParentFrameID, FrameType: x.FrameType, Route: x.Route, ThreadName: x.ThreadName, TimestampMillis: x.TimestampMillis, Representation: x.Representation, IsChunk: x.IsChunk, IsEnvelope: x.IsEnvelope, PayloadID: x.PayloadID})
	}
	router.writeScopedJSON(w, s.ID, pageDTO[recordDTO]{TargetScopeID: string(s.ID), Items: items, HasMore: v.HasMore, NextCursor: nullCursor(v.NextCursor)})
}
func (router *Router) traceAnalysisUsage(w http.ResponseWriter, r *http.Request, _ string) {
	var b analysisRequest
	if decodeJSONLimit(r, &b, maxTraceAnalysisJSONBody) != nil {
		writeError(w, 400, "INVALID_REQUEST", "Invalid request.")
		return
	}
	s, h, ok := router.resolveAnalysis(w, r, &b)
	if !ok {
		return
	}
	v, d := router.options.TraceAnalysis.GetUsageBreakdown(r.Context(), s.ID, h)
	if d != nil {
		writeDomainError(w, d)
		return
	}
	router.writeScopedJSON(w, s.ID, usageDTO{TargetScopeID: string(s.ID), Attributed: usageDTOValue(v.Attributed), Unattributed: usageDTOValue(v.Unattributed), UnframedAttributed: usageDTOValue(v.UnframedAttributed), Terminal: usageDTOValue(v.Terminal)})
}
func (router *Router) traceAnalysisAttempts(w http.ResponseWriter, r *http.Request, _ string) {
	var b analysisRequest
	if !router.decodeAnalysis(w, r, &b) {
		return
	}
	s, h, ok := router.resolveAnalysis(w, r, &b)
	if !ok {
		return
	}
	v, d := router.options.TraceAnalysis.QueryAttempts(r.Context(), s.ID, traceanalysis.AttemptQuery{Handle: h, PageSize: b.PageSize, Cursor: b.Cursor})
	if d != nil {
		writeDomainError(w, d)
		return
	}
	items := make([]attemptDTO, 0, len(v.Items))
	for _, x := range v.Items {
		items = append(items, attemptDTO{RetrySequenceID: x.RetrySequenceID, AttemptID: x.AttemptID, AttemptNumber: x.AttemptNumber, Usage: usageDTOValue(x.Usage), UsageComplete: x.UsageComplete})
	}
	router.writeScopedJSON(w, s.ID, pageDTO[attemptDTO]{TargetScopeID: string(s.ID), Items: items, HasMore: v.HasMore, NextCursor: nullCursor(v.NextCursor)})
}
func (router *Router) traceAnalysisRetries(w http.ResponseWriter, r *http.Request, _ string) {
	var b analysisRequest
	if !router.decodeAnalysis(w, r, &b) {
		return
	}
	s, h, ok := router.resolveAnalysis(w, r, &b)
	if !ok {
		return
	}
	v, d := router.options.TraceAnalysis.QueryRetries(r.Context(), s.ID, traceanalysis.RetryQuery{Handle: h, PageSize: b.PageSize, Cursor: b.Cursor})
	if d != nil {
		writeDomainError(w, d)
		return
	}
	items := make([]retryDTO, 0, len(v.Items))
	for _, x := range v.Items {
		items = append(items, retryDTO{RetrySequenceID: x.RetrySequenceID, Usage: usageDTOValue(x.Usage), UsageComplete: x.UsageComplete})
	}
	router.writeScopedJSON(w, s.ID, pageDTO[retryDTO]{TargetScopeID: string(s.ID), Items: items, HasMore: v.HasMore, NextCursor: nullCursor(v.NextCursor)})
}
func (router *Router) traceAnalysisValidationLinks(w http.ResponseWriter, r *http.Request, _ string) {
	var b analysisRequest
	if !router.decodeAnalysis(w, r, &b) {
		return
	}
	s, h, ok := router.resolveAnalysis(w, r, &b)
	if !ok {
		return
	}
	v, d := router.options.TraceAnalysis.QueryValidationLinks(r.Context(), s.ID, traceanalysis.ValidationQuery{Handle: h, PageSize: b.PageSize, Cursor: b.Cursor})
	if d != nil {
		writeDomainError(w, d)
		return
	}
	items := make([]validationDTO, 0, len(v.Items))
	for _, x := range v.Items {
		items = append(items, validationDTO{Status: x.Status, RetrySequenceID: x.RetrySequenceID, AttemptID: x.AttemptID, AttemptNumber: x.AttemptNumber})
	}
	router.writeScopedJSON(w, s.ID, pageDTO[validationDTO]{TargetScopeID: string(s.ID), Items: items, HasMore: v.HasMore, NextCursor: nullCursor(v.NextCursor)})
}
func (router *Router) traceAnalysisFailures(w http.ResponseWriter, r *http.Request, _ string) {
	var b analysisRequest
	if !router.decodeAnalysis(w, r, &b) {
		return
	}
	s, h, ok := router.resolveAnalysis(w, r, &b)
	if !ok {
		return
	}
	v, d := router.options.TraceAnalysis.QueryFailures(r.Context(), s.ID, traceanalysis.FailureQuery{Handle: h, PageSize: b.PageSize, Cursor: b.Cursor})
	if d != nil {
		writeDomainError(w, d)
		return
	}
	items := make([]failureDTO, 0, len(v.Items))
	for _, x := range v.Items {
		diagnostics := make([]diagnosticDescriptorDTO, 0, len(x.Diagnostics))
		for _, d := range x.Diagnostics {
			diagnostics = append(diagnostics, diagnosticDescriptorDTO{Ordinal: d.Ordinal, Kind: d.Kind, ContentType: d.ContentType, Truncated: d.Truncated, CaptureLimitBytes: d.CaptureLimitBytes, DecodedBytes: d.DecodedBytes})
		}
		items = append(items, failureDTO{FailureID: x.FailureID, Terminal: x.Terminal, Sequence: x.Sequence, TimestampMillis: x.TimestampMillis, RecordType: x.RecordType, FrameID: x.FrameID, Route: x.Route, AttemptID: x.AttemptID, RetrySequenceID: x.RetrySequenceID, ValidationStatus: x.ValidationStatus, ExceptionType: x.ExceptionType, ContextSummary: x.ContextSummary, Diagnostics: diagnostics})
	}
	router.writeScopedJSON(w, s.ID, pageDTO[failureDTO]{TargetScopeID: string(s.ID), Items: items, HasMore: v.HasMore, NextCursor: nullCursor(v.NextCursor)})
}
func (router *Router) traceAnalysisFailureDiagnostic(w http.ResponseWriter, r *http.Request, _ string) {
	var b struct {
		analysisRequest
		FailureID string `json:"failureId"`
		Ordinal   *int   `json:"ordinal"`
	}
	if decodeJSONLimit(r, &b, maxTraceAnalysisJSONBody) != nil {
		writeError(w, 400, "INVALID_REQUEST", "Invalid request.")
		return
	}
	if b.Ordinal == nil {
		writeError(w, 400, "INVALID_REQUEST", "Invalid request.")
		return
	}
	s, h, ok := router.resolveAnalysis(w, r, &b.analysisRequest)
	if !ok {
		return
	}
	v, d := router.options.TraceAnalysis.GetFailureDiagnostic(r.Context(), s.ID, traceanalysis.FailureDiagnosticRequest{Handle: h, FailureID: b.FailureID, Ordinal: *b.Ordinal})
	if d != nil {
		writeDomainError(w, d)
		return
	}
	desc := diagnosticDescriptorDTO{Ordinal: v.Descriptor.Ordinal, Kind: v.Descriptor.Kind, ContentType: v.Descriptor.ContentType, Truncated: v.Descriptor.Truncated, CaptureLimitBytes: v.Descriptor.CaptureLimitBytes, DecodedBytes: v.Descriptor.DecodedBytes}
	router.writeScopedJSON(w, s.ID, failureDiagnosticDTO{TargetScopeID: string(s.ID), FailureID: v.FailureID, Descriptor: desc, Text: v.Text})
}
func (router *Router) traceAnalysisPayloads(w http.ResponseWriter, r *http.Request, _ string) {
	var b analysisRequest
	if !router.decodeAnalysis(w, r, &b) {
		return
	}
	s, h, ok := router.resolveAnalysis(w, r, &b)
	if !ok {
		return
	}
	v, d := router.options.TraceAnalysis.QueryPayloads(r.Context(), s.ID, traceanalysis.PayloadQuery{Handle: h, PageSize: b.PageSize, Cursor: b.Cursor})
	if d != nil {
		writeDomainError(w, d)
		return
	}
	items := make([]payloadDTO, 0, len(v.Items))
	for _, x := range v.Items {
		items = append(items, payloadDTO{PayloadID: x.PayloadID, Sequence: x.Sequence, ContentType: x.ContentType, ChunkCount: x.ChunkCount, StoreLength: x.StoreLength})
	}
	router.writeScopedJSON(w, s.ID, pageDTO[payloadDTO]{TargetScopeID: string(s.ID), Items: items, HasMore: v.HasMore, NextCursor: nullCursor(v.NextCursor)})
}
func (router *Router) traceAnalysisGaps(w http.ResponseWriter, r *http.Request, _ string) {
	var b analysisRequest
	if !router.decodeAnalysis(w, r, &b) {
		return
	}
	s, h, ok := router.resolveAnalysis(w, r, &b)
	if !ok {
		return
	}
	v, d := router.options.TraceAnalysis.QueryGaps(r.Context(), s.ID, traceanalysis.GapQuery{Handle: h, PageSize: b.PageSize, Cursor: b.Cursor})
	if d != nil {
		writeDomainError(w, d)
		return
	}
	items := make([]gapDTO, 0, len(v.Items))
	for _, x := range v.Items {
		items = append(items, gapDTO{Kind: x.Kind, FrameID: x.FrameID, AttemptID: x.AttemptID})
	}
	router.writeScopedJSON(w, s.ID, pageDTO[gapDTO]{TargetScopeID: string(s.ID), Items: items, HasMore: v.HasMore, NextCursor: nullCursor(v.NextCursor)})
}
func (router *Router) traceAnalysisUncertainties(w http.ResponseWriter, r *http.Request, _ string) {
	var b analysisRequest
	if !router.decodeAnalysis(w, r, &b) {
		return
	}
	s, h, ok := router.resolveAnalysis(w, r, &b)
	if !ok {
		return
	}
	v, d := router.options.TraceAnalysis.QueryUncertainties(r.Context(), s.ID, traceanalysis.UncertaintyQuery{Handle: h, PageSize: b.PageSize, Cursor: b.Cursor})
	if d != nil {
		writeDomainError(w, d)
		return
	}
	items := make([]uncertaintyDTO, 0, len(v.Items))
	for _, x := range v.Items {
		items = append(items, uncertaintyDTO{Kind: x.Kind, FrameID: x.FrameID})
	}
	router.writeScopedJSON(w, s.ID, pageDTO[uncertaintyDTO]{TargetScopeID: string(s.ID), Items: items, HasMore: v.HasMore, NextCursor: nullCursor(v.NextCursor)})
}
func (router *Router) decodeAnalysis(w http.ResponseWriter, r *http.Request, b *analysisRequest) bool {
	if decodeJSONLimit(r, b, maxTraceAnalysisJSONBody) != nil {
		writeError(w, 400, "INVALID_REQUEST", "Invalid request.")
		return false
	}
	return true
}
func (router *Router) traceAnalysisSearch(w http.ResponseWriter, r *http.Request, _ string) {
	var b struct {
		analysisRequest
		Text string `json:"text"`
	}
	if decodeJSONLimit(r, &b, maxTraceAnalysisJSONBody) != nil {
		writeError(w, 400, "INVALID_REQUEST", "Invalid request.")
		return
	}
	s, h, ok := router.resolveAnalysis(w, r, &b.analysisRequest)
	if !ok {
		return
	}
	v, d := router.options.TraceAnalysis.Search(r.Context(), s.ID, traceanalysis.SearchQuery{Handle: h, Text: b.Text, PageSize: b.PageSize, Cursor: b.Cursor})
	if d != nil {
		writeDomainError(w, d)
		return
	}
	items := make([]searchDTO, 0, len(v.Items))
	for _, x := range v.Items {
		items = append(items, searchDTO{Sequence: x.Sequence, RecordType: x.RecordType, FrameID: x.FrameID, MatchOffset: x.MatchOffset, MatchLength: x.MatchLength, SearchedField: x.SearchedField})
	}
	router.writeScopedJSON(w, s.ID, pageDTO[searchDTO]{TargetScopeID: string(s.ID), Items: items, HasMore: v.HasMore, NextCursor: nullCursor(v.NextCursor)})
}
func (router *Router) traceAnalysisPayloadRange(w http.ResponseWriter, r *http.Request, _ string) {
	router.traceAnalysisRange(w, r, true)
}
func (router *Router) traceAnalysisRawRecordRange(w http.ResponseWriter, r *http.Request, _ string) {
	router.traceAnalysisRange(w, r, false)
}
func (router *Router) traceAnalysisRange(w http.ResponseWriter, r *http.Request, payload bool) {
	var b struct {
		analysisRequest
		PayloadID      string `json:"payloadId"`
		RecordSequence int64  `json:"recordSequence"`
		Start          int64  `json:"start"`
		MaxBytes       int    `json:"maxBytes"`
		ContinueCursor string `json:"continueCursor"`
	}
	if decodeJSONLimit(r, &b, maxTraceAnalysisJSONBody) != nil {
		writeError(w, 400, "INVALID_REQUEST", "Invalid request.")
		return
	}
	s, h, ok := router.resolveAnalysis(w, r, &b.analysisRequest)
	if !ok {
		return
	}
	q := traceanalysis.RangeRequest{Handle: h, PayloadID: b.PayloadID, RecordSequence: b.RecordSequence, Start: b.Start, MaxBytes: b.MaxBytes, ContinueCursor: b.ContinueCursor}
	var v traceanalysis.ByteRangeResult
	var d *consolecore.Error
	if payload {
		if b.PayloadID == "" || b.RecordSequence != 0 {
			writeError(w, 400, "INVALID_REQUEST", "A payload ID is required.")
			return
		}
		v, d = router.options.TraceAnalysis.ReadPayloadRange(r.Context(), s.ID, q)
	} else {
		if b.RecordSequence < 1 || b.PayloadID != "" {
			writeError(w, 400, "INVALID_REQUEST", "A record sequence is required.")
			return
		}
		v, d = router.options.TraceAnalysis.ReadRawRecordRange(r.Context(), s.ID, q)
	}
	if d != nil {
		writeDomainError(w, d)
		return
	}
	content := string(v.Content)
	router.writeScopedJSON(w, s.ID, map[string]any{"targetScopeId": string(s.ID), "actualStart": v.ActualStart, "actualEnd": v.ActualEnd, "totalLength": v.TotalLength, "contentType": v.ContentType, "encoding": v.Encoding, "content": content, "hasMore": v.HasMore, "nextCursor": nullCursor(v.NextCursor)})
}

type summaryDTO struct {
	TargetScopeID           string               `json:"targetScopeId"`
	TraceID                 string               `json:"traceId"`
	SessionID               string               `json:"sessionId"`
	Outcome                 string               `json:"outcome"`
	TerminalFailureID       *string              `json:"terminalFailureId"`
	ConfiguredLimits        *configuredLimitsDTO `json:"configuredLimits"`
	RecordCount             int64                `json:"recordCount"`
	FrameCount              int                  `json:"frameCount"`
	AttemptCount            int                  `json:"attemptCount"`
	RetryCount              int                  `json:"retryCount"`
	ValidationCount         int                  `json:"validationCount"`
	FailureCount            int                  `json:"failureCount"`
	PayloadCount            int                  `json:"payloadCount"`
	GapCount                int                  `json:"gapCount"`
	UncertaintyCount        int                  `json:"uncertaintyCount"`
	RootFrameIDs            []string             `json:"rootFrameIds"`
	AttributedUsage         usageValueDTO        `json:"attributedUsage"`
	TerminalUsage           usageValueDTO        `json:"terminalUsage"`
	UnattributedUsage       usageValueDTO        `json:"unattributedUsage"`
	UnframedAttributedUsage usageValueDTO        `json:"unframedAttributedUsage"`
	UsageComplete           bool                 `json:"usageComplete"`
}
type frameDTO struct {
	FrameID                 string        `json:"frameId"`
	ParentFrameID           *string       `json:"parentFrameId"`
	ChildFrameIDs           []string      `json:"childFrameIds"`
	FrameType               string        `json:"frameType"`
	Route                   string        `json:"route"`
	OpenedTimestampMillis   int64         `json:"openedTimestampMillis"`
	ClosedTimestampMillis   *int64        `json:"closedTimestampMillis"`
	InclusiveDurationMillis *int64        `json:"inclusiveDurationMillis"`
	SelfDurationMillis      *int64        `json:"selfDurationMillis"`
	DirectUsage             usageValueDTO `json:"directUsage"`
	DirectUsageComplete     bool          `json:"directUsageComplete"`
	DescendantUsage         usageValueDTO `json:"descendantUsage"`
	DescendantUsageComplete bool          `json:"descendantUsageComplete"`
	InclusiveUsage          usageValueDTO `json:"inclusiveUsage"`
	InclusiveUsageComplete  bool          `json:"inclusiveUsageComplete"`
	SkillNames              []string      `json:"skillNames"`
	Outcomes                []string      `json:"outcomes"`
	AttemptIDs              []string      `json:"attemptIds"`
	RetrySequenceIDs        []string      `json:"retrySequenceIds"`
	ValidationStatuses      []string      `json:"validationStatuses"`
	FailureIDs              []string      `json:"failureIds"`
}
type recordDTO struct {
	Sequence        int64  `json:"sequence"`
	Type            string `json:"type"`
	FrameID         string `json:"frameId"`
	ParentFrameID   string `json:"parentFrameId"`
	FrameType       string `json:"frameType"`
	Route           string `json:"route"`
	ThreadName      string `json:"threadName"`
	TimestampMillis int64  `json:"timestampMillis"`
	Representation  string `json:"representation"`
	IsChunk         bool   `json:"isChunk"`
	IsEnvelope      bool   `json:"isEnvelope"`
	PayloadID       string `json:"payloadId"`
}
type pageDTO[T any] struct {
	TargetScopeID string  `json:"targetScopeId"`
	Items         []T     `json:"items"`
	HasMore       bool    `json:"hasMore"`
	NextCursor    *string `json:"nextCursor"`
}
type usageDTO struct {
	TargetScopeID      string        `json:"targetScopeId"`
	Attributed         usageValueDTO `json:"attributed"`
	Unattributed       usageValueDTO `json:"unattributed"`
	UnframedAttributed usageValueDTO `json:"unframedAttributed"`
	Terminal           usageValueDTO `json:"terminal"`
}
type usageValueDTO struct {
	PromptUnits     int64 `json:"promptUnits"`
	CompletionUnits int64 `json:"completionUnits"`
	TotalUnits      int64 `json:"totalUnits"`
}

func usageDTOValue(v traceanalysis.Usage) usageValueDTO {
	return usageValueDTO{PromptUnits: v.PromptUnits, CompletionUnits: v.CompletionUnits, TotalUnits: v.TotalUnits}
}

type attemptDTO struct {
	RetrySequenceID string        `json:"retrySequenceId"`
	AttemptID       string        `json:"attemptId"`
	AttemptNumber   int64         `json:"attemptNumber"`
	Usage           usageValueDTO `json:"usage"`
	UsageComplete   bool          `json:"usageComplete"`
}
type retryDTO struct {
	RetrySequenceID string        `json:"retrySequenceId"`
	Usage           usageValueDTO `json:"usage"`
	UsageComplete   bool          `json:"usageComplete"`
}
type validationDTO struct {
	Status          string `json:"status"`
	RetrySequenceID string `json:"retrySequenceId"`
	AttemptID       string `json:"attemptId"`
	AttemptNumber   int64  `json:"attemptNumber"`
}
type failureDTO struct {
	FailureID        string                    `json:"failureId"`
	Terminal         bool                      `json:"terminal"`
	Sequence         int64                     `json:"sequence"`
	TimestampMillis  int64                     `json:"timestampMillis"`
	RecordType       string                    `json:"recordType"`
	FrameID          string                    `json:"frameId"`
	Route            string                    `json:"route"`
	AttemptID        string                    `json:"attemptId"`
	RetrySequenceID  string                    `json:"retrySequenceId"`
	ValidationStatus string                    `json:"validationStatus"`
	ExceptionType    string                    `json:"exceptionType,omitempty"`
	ContextSummary   string                    `json:"contextSummary,omitempty"`
	Diagnostics      []diagnosticDescriptorDTO `json:"diagnostics,omitempty"`
}
type diagnosticDescriptorDTO struct {
	Ordinal           int    `json:"ordinal"`
	Kind              string `json:"kind"`
	ContentType       string `json:"contentType"`
	Truncated         bool   `json:"truncated"`
	CaptureLimitBytes int    `json:"captureLimitBytes"`
	DecodedBytes      int    `json:"decodedBytes"`
}
type failureDiagnosticDTO struct {
	TargetScopeID string                  `json:"targetScopeId"`
	FailureID     string                  `json:"failureId"`
	Descriptor    diagnosticDescriptorDTO `json:"descriptor"`
	Text          string                  `json:"text"`
}

type configuredLimitsDTO struct {
	MaxSkillInvocations int64 `json:"maxSkillInvocations"`
	MaxToolInvocations  int64 `json:"maxToolInvocations"`
	MaxLinterRetries    int64 `json:"maxLinterRetries"`
	MaxModelCalls       int64 `json:"maxModelCalls"`
	MaxUsageUnits       int64 `json:"maxUsageUnits"`
}

func configuredLimitsDTOValue(v *traceanalysis.ConfiguredLimits) *configuredLimitsDTO {
	if v == nil {
		return nil
	}
	return &configuredLimitsDTO{MaxSkillInvocations: v.MaxSkillInvocations, MaxToolInvocations: v.MaxToolInvocations,
		MaxLinterRetries: v.MaxLinterRetries, MaxModelCalls: v.MaxModelCalls, MaxUsageUnits: v.MaxUsageUnits}
}

type payloadDTO struct {
	PayloadID   string `json:"payloadId"`
	Sequence    int64  `json:"sequence"`
	ContentType string `json:"contentType"`
	ChunkCount  int    `json:"chunkCount"`
	StoreLength int64  `json:"storeLength"`
}
type gapDTO struct {
	Kind      string `json:"kind"`
	FrameID   string `json:"frameId"`
	AttemptID string `json:"attemptId"`
}
type uncertaintyDTO struct {
	Kind    string `json:"kind"`
	FrameID string `json:"frameId"`
}
type searchDTO struct {
	Sequence      int64  `json:"sequence"`
	RecordType    string `json:"recordType"`
	FrameID       string `json:"frameId"`
	MatchOffset   int64  `json:"matchOffset"`
	MatchLength   int    `json:"matchLength"`
	SearchedField string `json:"searchedField"`
}

func nullCursor(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
