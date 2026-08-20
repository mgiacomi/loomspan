package mcpadapter

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/evidence"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/traceanalysis"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/traceinventory"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/traceresolution"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func addTraceTools(server *mcp.Server, options ServerOptions) {
	add := func(tool *mcp.Tool) { tool.Annotations = readOnlyAnnotations }
	listSchema := traceInputSchema[listTracesInput]()
	prepareListTracesSchema(listSchema)
	list := &mcp.Tool{Name: ListTracesToolName, Description: "List finalized traces. Complete is independent of pagination; limitations constrain negative or uniqueness claims.", InputSchema: listSchema}
	add(list)
	addValidatedTool(server, list, traceListOutputSchema(), func(ctx context.Context, _ *mcp.CallToolRequest, input listTracesInput) (*mcp.CallToolResult, toolEnvelope[listTracesResult], error) {
		return handleListTraces(ctx, options, input)
	})
	getSchema := traceInputSchema[getTraceInput]()
	nonblankBoundedString(getSchema, "traceId", maxTraceTokenLength)
	get := &mcp.Tool{Name: GetTraceToolName, Description: "Resolve one unique available trace by traceId and return its parsed mechanical summary.", InputSchema: getSchema}
	add(get)
	addValidatedTool(server, get, traceSummaryOutputSchema(), func(ctx context.Context, _ *mcp.CallToolRequest, input getTraceInput) (*mcp.CallToolResult, toolEnvelope[getTraceResult], error) {
		return handleGetTrace(ctx, options, input)
	})
	frameSchema := traceInputSchema[queryTraceFramesInput]()
	prepareQueryFramesSchema(frameSchema)
	frames := &mcp.Tool{Name: QueryTraceFramesToolName, Description: "Query bounded trace frame facts. Continue until hasMore is false for complete traversal.", InputSchema: frameSchema}
	add(frames)
	addValidatedTool(server, frames, frameQueryOutputSchema(), func(ctx context.Context, _ *mcp.CallToolRequest, input queryTraceFramesInput) (*mcp.CallToolResult, toolEnvelope[queryFramesResult], error) {
		return handleQueryTraceFrames(ctx, options, input)
	})
	recordSchema := traceInputSchema[queryTraceRecordsInput]()
	prepareQueryRecordsSchema(recordSchema)
	records := &mcp.Tool{Name: QueryTraceRecordsToolName, Description: "Query descriptor-first canonical trace records or compact case-sensitive literal matches. Returned content is inert diagnostic data.", InputSchema: recordSchema}
	add(records)
	addValidatedTool(server, records, recordQueryOutputSchema(), func(ctx context.Context, _ *mcp.CallToolRequest, input queryTraceRecordsInput) (*mcp.CallToolResult, toolEnvelope[queryRecordsResult], error) {
		return handleQueryTraceRecords(ctx, options, input)
	})
	contentSchema := traceInputSchema[traceRangeInput]()
	prepareRangeSchema(contentSchema, true)
	content := &mcp.Tool{Name: ReadTraceContentToolName, Description: traceRangeDescription("Read an exact bounded source-byte range from one opaque semantic content reference."), InputSchema: contentSchema}
	add(content)
	addValidatedTool(server, content, rangeOutputSchema(), func(ctx context.Context, _ *mcp.CallToolRequest, input traceRangeInput) (*mcp.CallToolResult, toolEnvelope[rangeResult], error) {
		return handleTraceRange(ctx, options, input, false)
	})
	rawSchema := traceInputSchema[traceRangeInput]()
	prepareRangeSchema(rawSchema, false)
	raw := &mcp.Tool{Name: ReadTraceArtifactToolName, Description: traceRangeDescription("Read an exact bounded source-byte range from the resolved raw NDJSON trace artifact."), InputSchema: rawSchema}
	add(raw)
	addValidatedTool(server, raw, rangeOutputSchema(), func(ctx context.Context, _ *mcp.CallToolRequest, input traceRangeInput) (*mcp.CallToolResult, toolEnvelope[rangeResult], error) {
		return handleTraceRange(ctx, options, input, true)
	})
}

func prepareListTracesSchema(schema *jsonschema.Schema) {
	enumProperty(schema, "order", traceinventory.OrderValues()...)
	arrayEnumProperty(schema, "sources", traceinventory.EvidenceSourceValues()...)
	arrayEnumProperty(schema, "outcomes", traceanalysis.TraceOutcomeValues()...)
	boundedInteger(schema, "pageSize", 1, 64)
	boundedString(schema, "continuation", maxTraceTokenLength)
}

func prepareQueryFramesSchema(schema *jsonschema.Schema) {
	nonblankBoundedString(schema, "traceId", maxTraceTokenLength)
	enumProperty(schema, "order", traceanalysis.FrameOrderValues()...)
	enumProperty(schema, "projection", traceanalysis.FrameProjectionValues()...)
	nestedEnumProperty(schema, "filter", "frameType", traceanalysis.FrameTypeValues()...)
	nestedEnumProperty(schema, "filter", "outcome", traceanalysis.FrameOutcomeValues()...)
	nestedEnumProperty(schema, "filter", "validationStatus", traceanalysis.ValidationStatusValues()...)
	boundedInteger(schema, "pageSize", 1, 64)
	boundedString(schema, "continuation", maxTraceTokenLength)
}

func prepareQueryRecordsSchema(schema *jsonschema.Schema) {
	nonblankBoundedString(schema, "traceId", maxTraceTokenLength)
	enumProperty(schema, "representation", traceanalysis.RecordRepresentationValues()...)
	nestedArrayEnumProperty(schema, "filter", "types", traceanalysis.RecordTypeValues()...)
	nestedEnumProperty(schema, "filter", "validationStatus", traceanalysis.ValidationStatusValues()...)
	boundedInteger(schema, "pageSize", 1, 64)
	boundedString(schema, "continuation", maxTraceTokenLength)
}

func traceRangeDescription(prefix string) string {
	return fmt.Sprintf("%s The default is %d bytes and the maximum is %d source bytes.", prefix, defaultTraceRangeBytes, maxTraceRangeBytes)
}

func setStringEnum(schema *jsonschema.Schema, values ...string) {
	if schema == nil {
		return
	}
	schema.Enum = make([]any, len(values))
	for i, value := range values {
		schema.Enum[i] = value
	}
}

func arrayEnumProperty(schema *jsonschema.Schema, name string, values ...string) {
	if property := schema.Properties[name]; property != nil {
		setStringEnum(property.Items, values...)
		one, maximum := 1, len(values)
		property.MinItems, property.MaxItems, property.UniqueItems = &one, &maximum, true
	}
}

func nestedArrayEnumProperty(schema *jsonschema.Schema, parent, name string, values ...string) {
	if object := schema.Properties[parent]; object != nil {
		arrayEnumProperty(object, name, values...)
	}
}

func nestedEnumProperty(schema *jsonschema.Schema, parent, name string, values ...string) {
	if object := schema.Properties[parent]; object != nil {
		enumProperty(object, name, values...)
	}
}

func prepareRangeSchema(schema *jsonschema.Schema, payload bool) {
	nonblankBoundedString(schema, "traceId", maxTraceTokenLength)
	boundedString(schema, "continuation", maxTraceTokenLength)
	boundedInteger(schema, "start", 0, float64(^uint64(0)>>1))
	boundedInteger(schema, "maxBytes", 1, maxTraceRangeBytes)
	exactlyOne(schema, "start", "continuation")
	if payload {
		schema.Required = append(schema.Required, "contentRef")
		boundedString(schema, "contentRef", maxTraceTokenLength)
	} else {
		delete(schema.Properties, "contentRef")
	}
}

func handleListTraces(ctx context.Context, options ServerOptions, input listTracesInput) (*mcp.CallToolResult, toolEnvelope[listTracesResult], error) {
	if options.TraceInventory == nil {
		return checkedDomainFailure[listTracesResult](ctx, options, unavailableInspectionError(""))
	}
	result, domain := options.TraceInventory.List(ctx, traceinventoryQuery(input))
	if domain != nil {
		return checkedDomainFailure[listTracesResult](ctx, options, domain)
	}
	mapped := mapInventory(result)
	if err := authenticationGenerationError(ctx, options); err != nil {
		return nil, toolEnvelope[listTracesResult]{}, err
	}
	return successResult(mapped, traceListText(mapped))
}

func traceinventoryQuery(input listTracesInput) traceinventory.Query {
	return traceinventory.Query{PageSize: input.PageSize, Continuation: input.Continuation, Sources: input.Sources, Outcomes: input.Outcomes, EntrySkill: input.EntrySkill, SessionID: input.SessionID, FinalizedFrom: input.FinalizedFrom, FinalizedTo: input.FinalizedTo, AcquiredFrom: input.AcquiredFrom, AcquiredTo: input.AcquiredTo, ImportedFrom: input.ImportedFrom, ImportedTo: input.ImportedTo, Order: input.Order}
}

func handleGetTrace(ctx context.Context, options ServerOptions, input getTraceInput) (*mcp.CallToolResult, toolEnvelope[getTraceResult], error) {
	resolved, domain := resolveTrace(ctx, options, input.TraceID)
	if domain != nil {
		return checkedDomainFailure[getTraceResult](ctx, options, domain)
	}
	if options.TraceAnalysis == nil {
		return checkedDomainFailure[getTraceResult](ctx, options, unavailableInspectionError(""))
	}
	summary, domain := options.TraceAnalysis.GetSummary(ctx, resolved.Reference, traceanalysis.SummaryRequest{Handle: resolved.Handle})
	if domain != nil {
		return checkedDomainFailure[getTraceResult](ctx, options, mapTraceAnalysisError(domain, input.TraceID, false, false))
	}
	result := getTraceResult{Evidence: mapEvidence(summary.Context, options), Summary: mapSummary(summary)}
	if resolved.Scope.ID != "" {
		if domain := publicationDomain(options, resolved.Scope); domain != nil {
			return checkedDomainFailure[getTraceResult](ctx, options, domain)
		}
	}
	if err := authenticationGenerationError(ctx, options); err != nil {
		return nil, toolEnvelope[getTraceResult]{}, err
	}
	return successResult(result, traceSummaryText(result))
}

func handleQueryTraceFrames(ctx context.Context, options ServerOptions, input queryTraceFramesInput) (*mcp.CallToolResult, toolEnvelope[queryFramesResult], error) {
	resolved, domain := resolveTrace(ctx, options, input.TraceID)
	if domain != nil {
		return checkedDomainFailure[queryFramesResult](ctx, options, domain)
	}
	if options.TraceAnalysis == nil {
		return checkedDomainFailure[queryFramesResult](ctx, options, unavailableInspectionError(""))
	}
	pageSize, pageDomain := tracePageSize(input.PageSize)
	if pageDomain != nil {
		return checkedDomainFailure[queryFramesResult](ctx, options, pageDomain)
	}
	page, domain := options.TraceAnalysis.QueryFrames(ctx, resolved.Reference, traceanalysis.FrameQuery{Handle: resolved.Handle, Filter: input.Filter, Order: input.Order, Projection: input.Projection, PageSize: pageSize, Cursor: input.Continuation})
	if domain != nil {
		return checkedDomainFailure[queryFramesResult](ctx, options, mapTraceAnalysisError(domain, input.TraceID, input.Continuation != "", false))
	}
	items := make([]frameDTO, 0, len(page.Items))
	projection := input.Projection
	if projection == "" {
		projection = traceanalysis.FrameProjectionCompact
	}
	for _, item := range page.Items {
		items = append(items, mapFrame(item, projection))
	}
	result := queryFramesResult{Evidence: evidenceFromPageFrames(page, options), Projection: string(projection), Items: items, HasMore: page.HasMore, Continuation: page.NextCursor}
	if resolved.Scope.ID != "" {
		if domain := publicationDomain(options, resolved.Scope); domain != nil {
			return checkedDomainFailure[queryFramesResult](ctx, options, domain)
		}
	}
	if err := authenticationGenerationError(ctx, options); err != nil {
		return nil, toolEnvelope[queryFramesResult]{}, err
	}
	return successResult(result, traceFramesText(result))
}

func handleQueryTraceRecords(ctx context.Context, options ServerOptions, input queryTraceRecordsInput) (*mcp.CallToolResult, toolEnvelope[queryRecordsResult], error) {
	resolved, domain := resolveTrace(ctx, options, input.TraceID)
	if domain != nil {
		return checkedDomainFailure[queryRecordsResult](ctx, options, domain)
	}
	if options.TraceAnalysis == nil {
		return checkedDomainFailure[queryRecordsResult](ctx, options, unavailableInspectionError(""))
	}
	pageSize, pageDomain := tracePageSize(input.PageSize)
	if pageDomain != nil {
		return checkedDomainFailure[queryRecordsResult](ctx, options, pageDomain)
	}
	if input.Filter.LiteralText != "" {
		searcher, ok := options.TraceAnalysis.(interface {
			Search(context.Context, evidence.Reference, traceanalysis.SearchQuery) (traceanalysis.SearchPage, *consolecore.Error)
		})
		if !ok {
			return checkedDomainFailure[queryRecordsResult](ctx, options, unavailableInspectionError(""))
		}
		page, searchDomain := searcher.Search(ctx, resolved.Reference, traceanalysis.SearchQuery{Handle: resolved.Handle, Text: input.Filter.LiteralText, Filter: input.Filter, PageSize: pageSize, Cursor: input.Continuation})
		if searchDomain != nil {
			return checkedDomainFailure[queryRecordsResult](ctx, options, mapTraceAnalysisError(searchDomain, input.TraceID, input.Continuation != "", false))
		}
		matches := make([]searchMatchDTO, 0, len(page.Items))
		for _, m := range page.Items {
			matches = append(matches, searchMatchDTO{m.Sequence, m.RecordType, m.FrameID, m.MatchOffset, m.MatchLength, m.SearchedField, m.ContentID})
		}
		descriptors := make([]searchContentDescriptorDTO, 0, len(page.ContentDescriptors))
		for _, descriptor := range page.ContentDescriptors {
			descriptors = append(descriptors, searchContentDescriptorDTO{ContentID: descriptor.ContentID, ContentRef: descriptor.ContentRef})
		}
		limitations := make([]traceLimitationDTO, 0, len(page.SearchLimitations))
		for _, limitation := range page.SearchLimitations {
			limitations = append(limitations, traceLimitationDTO{Code: limitation.Code, Message: limitation.Message})
		}
		result := queryRecordsResult{Evidence: mapEvidence(page.Context, options), Matches: matches, ContentDescriptors: &descriptors, Search: &searchCoverageDTO{Query: input.Filter.LiteralText, CaseSensitive: true, Representation: "LOGICAL", SearchedFields: []string{"metadata", "content"}, SemanticContentCoverage: "AVAILABLE_COMPLETE_TEXT", WorkComplete: !page.HasMore, Limitations: limitations}, HasMore: page.HasMore, Continuation: page.NextCursor}
		return successResult(result, traceRecordsText(result))
	}
	page, domain := options.TraceAnalysis.QueryRecords(ctx, resolved.Reference, traceanalysis.RecordQuery{Handle: resolved.Handle, Filter: input.Filter, Representation: input.Representation, InlineContent: input.InlineContent, PageSize: pageSize, Cursor: input.Continuation})
	if domain != nil {
		return checkedDomainFailure[queryRecordsResult](ctx, options, mapTraceAnalysisError(domain, input.TraceID, input.Continuation != "", false))
	}
	items := make([]recordDTO, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, mapRecord(item))
	}
	result := queryRecordsResult{Evidence: mapEvidence(page.Context, options), Items: items, HasMore: page.HasMore, Continuation: page.NextCursor}
	if resolved.Scope.ID != "" {
		if domain := publicationDomain(options, resolved.Scope); domain != nil {
			return checkedDomainFailure[queryRecordsResult](ctx, options, domain)
		}
	}
	if err := authenticationGenerationError(ctx, options); err != nil {
		return nil, toolEnvelope[queryRecordsResult]{}, err
	}
	return successResult(result, traceRecordsText(result))
}

func handleTraceRange(ctx context.Context, options ServerOptions, input traceRangeInput, raw bool) (*mcp.CallToolResult, toolEnvelope[rangeResult], error) {
	resolved, domain := resolveTrace(ctx, options, input.TraceID)
	if domain != nil {
		return checkedDomainFailure[rangeResult](ctx, options, domain)
	}
	if options.TraceAnalysis == nil {
		return checkedDomainFailure[rangeResult](ctx, options, unavailableInspectionError(""))
	}
	if input.Start != nil && input.Continuation != "" {
		return checkedDomainFailure[rangeResult](ctx, options, invalidTraceArgument("Supply start or continuation, not both."))
	}
	if input.Start == nil && input.Continuation == "" {
		return checkedDomainFailure[rangeResult](ctx, options, invalidTraceArgument("Supply start or continuation."))
	}
	start := int64(0)
	if input.Start != nil {
		start = *input.Start
		if start < 0 {
			return checkedDomainFailure[rangeResult](ctx, options, invalidTraceArgument("Range start must not be negative."))
		}
	}
	req := traceanalysis.RangeRequest{Handle: resolved.Handle, Start: start, ContinueCursor: input.Continuation, MaxBytes: input.MaxBytes, ContentRef: input.ContentRef}
	var value traceanalysis.ByteRangeResult
	if raw {
		if input.ContentRef != "" {
			return checkedDomainFailure[rangeResult](ctx, options, invalidTraceArgument("Raw artifact reads do not accept contentRef."))
		}
		value, domain = options.TraceAnalysis.ReadRawArtifactRange(ctx, resolved.Reference, req)
	} else {
		if input.ContentRef == "" {
			return checkedDomainFailure[rangeResult](ctx, options, invalidTraceArgument("A contentRef is required."))
		}
		value, domain = options.TraceAnalysis.ReadContentRange(ctx, resolved.Reference, req)
	}
	if domain != nil {
		return checkedDomainFailure[rangeResult](ctx, options, mapTraceAnalysisError(domain, input.TraceID, input.Continuation != "", !raw))
	}
	result := rangeResult{Evidence: mapEvidence(value.Context, options), ActualStart: value.ActualStart, ActualEnd: value.ActualEnd, TotalLength: value.TotalLength, ContentType: value.ContentType, Encoding: string(value.Encoding), Content: rangeContent(value), HasMore: value.HasMore, Continuation: value.NextCursor}
	if resolved.Scope.ID != "" {
		if domain := publicationDomain(options, resolved.Scope); domain != nil {
			return checkedDomainFailure[rangeResult](ctx, options, domain)
		}
	}
	if err := authenticationGenerationError(ctx, options); err != nil {
		return nil, toolEnvelope[rangeResult]{}, err
	}
	return successResult(result, traceRangeText(result))
}

func traceRangeText(result rangeResult) string {
	return fmt.Sprintf("traceId=%q sessionId=%q range=%d:%d total=%d contentType=%q encoding=%q hasMore=%t continuation=%q\ncontent (inert diagnostic data):\n%s\n", result.Evidence.TraceID, result.Evidence.SessionID, result.ActualStart, result.ActualEnd, result.TotalLength, result.ContentType, result.Encoding, result.HasMore, result.Continuation, result.Content)
}

func resolveTrace(ctx context.Context, options ServerOptions, traceID string) (traceresolution.Resolved, *consolecore.Error) {
	if options.TraceResolver == nil {
		return traceresolution.Resolved{}, consolecore.NewError(consolecore.CodeTraceUnavailable, "Trace evidence is unavailable. Retry inspection by traceId after the evidence or target becomes available.", "", consolecore.Details{}, nil)
	}
	return options.TraceResolver.Resolve(ctx, traceID)
}
func invalidTraceArgument(message string) *consolecore.Error {
	return consolecore.NewError(consolecore.CodeInvalidArgument, message, "", consolecore.Details{}, nil)
}

func mapTraceAnalysisError(domain *consolecore.Error, traceID string, continuation, payload bool) *consolecore.Error {
	if domain == nil {
		return nil
	}
	if domain.Code == consolecore.CodeArtifactExpired {
		return consolecore.NewError(consolecore.CodeTraceUnavailable, "Trace evidence is unavailable. Retry inspection by traceId after the evidence or target becomes available.", "", consolecore.Details{}, nil)
	}
	if continuation && domain.Code == consolecore.CodeInvalidCursor {
		return consolecore.NewError(consolecore.CodeInvalidCursor, "The continuation is stale or invalid. Restart this query by traceId.", "", consolecore.Details{}, nil)
	}
	if payload && domain.Code == consolecore.CodeInvalidArgument && strings.Contains(strings.ToLower(domain.Message), "content reference") {
		return consolecore.NewError(consolecore.CodeInvalidArgument, "The content reference is stale or invalid. Re-query the relevant record descriptor by traceId.", "", consolecore.Details{}, nil)
	}
	_ = traceID
	return domain
}

func tracePageSize(value int) (int, *consolecore.Error) {
	if value == 0 {
		return maxMCPPageSize, nil
	}
	if value < 1 || value > maxMCPPageSize {
		return 0, invalidTraceArgument("Page size must be from 1 through 64.")
	}
	return value, nil
}

func mapInventory(value traceinventory.Result) listTracesResult {
	out := listTracesResult{ObservedAt: value.ObservedAt, Items: []traceInventoryItemDTO{}, Complete: value.Complete, Limitations: []traceLimitationDTO{}, HasMore: value.HasMore, Continuation: value.Continuation}
	for _, limitation := range value.Limitations {
		out.Limitations = append(out.Limitations, traceLimitationDTO{Code: string(limitation.Code), Message: limitation.Message})
	}
	for _, x := range value.Items {
		out.Items = append(out.Items, traceInventoryItemDTO{TraceID: x.TraceID, EvidenceSources: x.EvidenceSources, SessionID: x.SessionID, EntrySkill: x.EntrySkill, Outcome: x.Outcome, FinalizedAt: x.FinalizedAt, AcquiredAt: x.AcquiredAt, ImportedAt: x.ImportedAt, Ambiguous: x.Ambiguous})
	}
	return out
}
func mapEvidence(value traceanalysis.TraceContext, options ServerOptions) evidenceDTO {
	return evidenceDTO{TraceID: value.TraceID, SessionID: value.SessionID, ObservedAt: options.Now().UTC()}
}
func mapSummary(x traceanalysis.TraceSummary) traceSummaryDTO {
	return traceSummaryDTO{Outcome: x.Outcome, TerminalFailureID: x.TerminalFailureID, ConfiguredLimits: x.ConfiguredLimits, RecordCount: x.RecordCount, FrameCount: x.FrameCount, AttemptCount: x.AttemptCount, RetryCount: x.RetryCount, ValidationCount: x.ValidationCount, FailureCount: x.FailureCount, PayloadCount: x.PayloadCount, GapCount: x.GapCount, UncertaintyCount: x.UncertaintyCount, RootFrameIDs: nonNil(x.RootFrameIDs), AttributedUsage: usageValue(x.AttributedUsage), TerminalUsage: usageValue(x.TerminalUsage), UnattributedUsage: usageValue(x.UnattributedUsage), UnframedAttributedUsage: usageValue(x.UnframedAttributed), UsageComplete: x.UsageComplete}
}
func mapFrame(x traceanalysis.FrameSummary, projection traceanalysis.FrameProjection) frameDTO {
	out := frameDTO{FrameID: x.FrameID, ParentFrameID: x.ParentFrameID, ChildFrameIDs: nonNil(x.ChildFrameIDs), FrameType: x.FrameType, Route: x.Route, OpenedTimestampMillis: x.OpenedTimestampMillis, ClosedTimestampMillis: x.ClosedTimestampMillis, InclusiveDurationMillis: x.InclusiveDurationMillis, SelfDurationMillis: x.SelfDurationMillis, DirectUsageComplete: x.DirectUsageComplete, DescendantUsageComplete: x.DescendantUsageComplete, InclusiveUsageComplete: x.InclusiveUsageComplete, Outcomes: nonNil(x.Outcomes), DirectAttemptCount: x.DirectAttemptCount, DirectRetryCount: x.DirectRetryCount, DirectValidationCount: x.DirectValidationCount, DirectFailureCount: x.DirectFailureCount, GapCount: x.GapCount, UncertaintyCount: x.UncertaintyCount, detailed: projection == traceanalysis.FrameProjectionDetailed}
	if projection == traceanalysis.FrameProjectionDetailed {
		out.SkillNames, out.AttemptIDs, out.RetrySequenceIDs = nonNil(x.SkillNames), nonNil(x.AttemptIDs), nonNil(x.RetrySequenceIDs)
		out.ValidationStatuses, out.FailureIDs = nonNil(x.ValidationStatuses), nonNil(x.FailureIDs)
		out.GapKinds, out.UncertaintyKinds = nonNil(x.GapKinds), nonNil(x.UncertaintyKinds)
	}
	if projection == traceanalysis.FrameProjectionDetailed {
		d, u, i := usageValue(x.DirectUsage), usageValue(x.DescendantUsage), usageValue(x.InclusiveUsage)
		out.DirectUsage = &d
		out.DescendantUsage = &u
		out.InclusiveUsage = &i
	}
	return out
}

func (value frameDTO) MarshalJSON() ([]byte, error) {
	type alias frameDTO
	if !value.detailed {
		return json.Marshal(alias(value))
	}
	body, err := json.Marshal(alias(value))
	if err != nil {
		return nil, err
	}
	var fields map[string]any
	if err := json.Unmarshal(body, &fields); err != nil {
		return nil, err
	}
	fields["skillNames"] = nonNil(value.SkillNames)
	fields["attemptIds"] = nonNil(value.AttemptIDs)
	fields["retrySequenceIds"] = nonNil(value.RetrySequenceIDs)
	fields["validationStatuses"] = nonNil(value.ValidationStatuses)
	fields["failureIds"] = nonNil(value.FailureIDs)
	fields["gapKinds"] = nonNil(value.GapKinds)
	fields["uncertaintyKinds"] = nonNil(value.UncertaintyKinds)
	return json.Marshal(fields)
}
func mapRecord(x traceanalysis.RecordSummary) recordDTO {
	facts := recordFactsDTO{Attempts: []attemptDTO{}, Retries: []retryDTO{}, Validations: []validationDTO{}, Failures: []failureDTO{}, SearchMatches: []searchMatchDTO{}}
	if x.Facts.Plan != nil {
		p := x.Facts.Plan
		facts.Plan = &planLandmarkDTO{p.PlanID, p.Sequence, p.TraceRootFrameID, p.MissionFrameID, p.PlanningFrameID, p.AttemptID, p.RetrySequenceID}
	}
	for _, a := range x.Facts.Attempts {
		facts.Attempts = append(facts.Attempts, attemptDTO{a.RetrySequenceID, a.AttemptID, a.AttemptNumber, a.AttemptReason, a.ProviderAttemptNumber, a.Outcome, a.FailureClassification, a.FailureCategory, a.RetryDecision, a.RetryDelayMillis, a.RetryDelaySource, a.HTTPStatus, a.ProviderErrorType, a.ProviderErrorCode, a.ContentRef, usageValue(a.Usage), a.UsageComplete})
	}
	for _, r := range x.Facts.Retries {
		facts.Retries = append(facts.Retries, retryDTO{r.RetrySequenceID, usageValue(r.Usage), r.UsageComplete})
	}
	for _, v := range x.Facts.Validations {
		facts.Validations = append(facts.Validations, validationDTO{v.Status, v.RetrySequenceID, v.AttemptID, v.AttemptNumber})
	}
	for _, f := range x.Facts.Failures {
		facts.Failures = append(facts.Failures, mapFailure(f))
	}
	for _, m := range x.Facts.SearchMatches {
		facts.SearchMatches = append(facts.SearchMatches, searchMatchDTO{m.Sequence, m.RecordType, m.FrameID, m.MatchOffset, m.MatchLength, m.SearchedField, m.ContentID})
	}
	out := recordDTO{Sequence: x.Sequence, Type: x.Type, FailureID: x.FailureID, FrameID: x.FrameID, ParentFrameID: x.ParentFrameID, FrameType: x.FrameType, Route: x.Route, ThreadName: x.ThreadName, TimestampMillis: x.TimestampMillis, Representation: x.Representation, IsChunk: x.IsChunk, IsEnvelope: x.IsEnvelope, Raw: x.Raw, Facts: facts}
	if x.Content != nil {
		c := x.Content
		inline := string(c.InlineContent)
		if c.Encoding == traceanalysis.ContentEncodingBinary && len(c.InlineContent) > 0 {
			inline = base64.StdEncoding.EncodeToString(c.InlineContent)
		}
		out.Content = &contentDescriptorDTO{Role: string(c.Role), ContentType: c.ContentType, Encoding: string(c.Encoding), RetainedBytes: c.RetainedBytes, Available: c.Available, Complete: c.Complete, InlineEligibility: c.InlineEligibility, InlineOmission: string(c.InlineOmission), ContentRef: c.ContentRef, InlineContent: inline}
	}
	return out
}
func mapFailure(f traceanalysis.FailureSummary) failureDTO {
	out := failureDTO{FailureID: f.FailureID, Terminal: f.Terminal, Sequence: f.Sequence, TimestampMillis: f.TimestampMillis, RecordType: f.RecordType, FrameID: f.FrameID, Route: f.Route, AttemptID: f.AttemptID, RetrySequenceID: f.RetrySequenceID, ValidationStatus: f.ValidationStatus, ExceptionType: f.ExceptionType, ContextSummary: f.ContextSummary, Diagnostics: []diagnosticDTO{}}
	for _, d := range f.Diagnostics {
		out.Diagnostics = append(out.Diagnostics, diagnosticDTO{d.Ordinal, d.Kind, d.ContentType, d.Truncated, d.CaptureLimitBytes, d.DecodedBytes, d.ContentRef})
	}
	return out
}
func nonNil[T any](values []T) []T {
	if values == nil {
		return []T{}
	}
	return values
}
func evidenceFromPageFrames(page traceanalysis.Page[traceanalysis.FrameSummary], options ServerOptions) evidenceDTO {
	return mapEvidence(page.Context, options)
}
func traceListText(value listTracesResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "traces count=%d complete=%t hasMore=%t continuation=%q limitations=%d\n", len(value.Items), value.Complete, value.HasMore, fallbackField(value.Continuation), len(value.Limitations))
	for _, limitation := range value.Limitations {
		fmt.Fprintf(&b, "limitation code=%q message=%q\n", limitation.Code, fallbackField(limitation.Message))
	}
	for _, x := range value.Items {
		fmt.Fprintf(&b, "traceId=%q sources=%v outcome=%q finalizedAt=%q acquiredAt=%q importedAt=%q ambiguous=%t\n", fallbackField(x.TraceID), x.EvidenceSources, optionalValue(x.Outcome, fallbackField), optionalValue(x.FinalizedAt, formatFallbackTime), optionalValue(x.AcquiredAt, formatFallbackTime), optionalValue(x.ImportedAt, formatFallbackTime), x.Ambiguous)
	}
	return boundedNavigationText(b.String())
}
func traceSummaryText(value getTraceResult) string {
	s := value.Summary
	return fmt.Sprintf("traceId=%q sessionId=%q outcome=%q records=%d frames=%d attempts=%d retries=%d validations=%d failures=%d gaps=%d uncertainties=%d usageComplete=%t\n", value.Evidence.TraceID, value.Evidence.SessionID, s.Outcome, s.RecordCount, s.FrameCount, s.AttemptCount, s.RetryCount, s.ValidationCount, s.FailureCount, s.GapCount, s.UncertaintyCount, s.UsageComplete)
}
func traceFramesText(value queryFramesResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "traceId=%q projection=%q count=%d hasMore=%t continuation=%q\n", fallbackField(value.Evidence.TraceID), value.Projection, len(value.Items), value.HasMore, fallbackField(value.Continuation))
	for _, x := range value.Items {
		fmt.Fprintf(&b, "frameId=%q parentFrameId=%q type=%q route=%q closedTimestampMillis=%s inclusiveDurationMillis=%s selfDurationMillis=%s outcomes=%v attempts=%d retries=%d validations=%d failures=%d gaps=%d uncertainties=%d\n", fallbackField(x.FrameID), optionalValue(x.ParentFrameID, fallbackField), x.FrameType, fallbackField(x.Route), optionalValue(x.ClosedTimestampMillis, formatFallbackInt64), optionalValue(x.InclusiveDurationMillis, formatFallbackInt64), optionalValue(x.SelfDurationMillis, formatFallbackInt64), x.Outcomes, x.DirectAttemptCount, x.DirectRetryCount, x.DirectValidationCount, x.DirectFailureCount, x.GapCount, x.UncertaintyCount)
	}
	return boundedNavigationText(b.String())
}
func traceRecordsText(value queryRecordsResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "traceId=%q records=%d matches=%d hasMore=%t continuation=%q\n", fallbackField(value.Evidence.TraceID), len(value.Items), len(value.Matches), value.HasMore, fallbackField(value.Continuation))
	if value.Search != nil {
		fmt.Fprintf(&b, "search query=%q caseSensitive=%t workComplete=%t fields=%v coverage=%q limitations=%d\n", value.Search.Query, value.Search.CaseSensitive, value.Search.WorkComplete, value.Search.SearchedFields, value.Search.SemanticContentCoverage, len(value.Search.Limitations))
		for _, limitation := range value.Search.Limitations {
			fmt.Fprintf(&b, "limitation code=%q message=%q\n", limitation.Code, fallbackField(limitation.Message))
		}
	}
	for _, x := range value.Items {
		ref := ""
		bytes := int64(0)
		if x.Content != nil {
			ref = x.Content.ContentRef
			bytes = x.Content.RetainedBytes
		}
		fmt.Fprintf(&b, "sequence=%d type=%q frameId=%q representation=%q contentRef=%q retainedBytes=%d\n", x.Sequence, x.Type, fallbackField(x.FrameID), x.Representation, fallbackField(ref), bytes)
	}
	for _, m := range value.Matches {
		fmt.Fprintf(&b, "sequence=%d type=%q frameId=%q field=%q offset=%d length=%d\n", m.Sequence, m.RecordType, m.FrameID, m.SearchedField, m.MatchOffset, m.MatchLength)
	}
	return boundedNavigationText(b.String())
}

func fallbackField(value string) string {
	const maximum = 512
	if len(value) <= maximum {
		return value
	}
	return value[:maximum] + "…"
}

func optionalValue[T comparable](value *T, format func(T) string) string {
	if value == nil {
		return "-"
	}
	var zero T
	if *value == zero {
		return "unknown"
	}
	return format(*value)
}

func formatFallbackTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func formatFallbackInt64(value int64) string {
	return strconv.FormatInt(value, 10)
}

func boundedNavigationText(value string) string {
	maximum := traceanalysis.MaxCompactResponseBytes
	if len(value) <= maximum {
		return value
	}
	suffix := "\n… additional structured items omitted from text fallback\n"
	return value[:maximum-len(suffix)] + suffix
}
