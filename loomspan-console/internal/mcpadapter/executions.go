package mcpadapter

import (
	"context"
	"fmt"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/observability"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	ListExecutionsToolName = "LOOMSPAN_list_executions"
	GetExecutionToolName   = "LOOMSPAN_get_execution"
)

type listExecutionsInput struct {
	PageSize     int    `json:"pageSize,omitempty" jsonschema:"Number of active executions to return, from 1 through 64; omitted defaults to 16"`
	Continuation string `json:"continuation,omitempty" jsonschema:"Opaque Loomspan continuation returned by an earlier execution-list call"`
}

type getExecutionInput struct {
	SessionID string `json:"sessionId" jsonschema:"Exact active execution session identifier"`
}

func addExecutionTools(server *mcp.Server, options ServerOptions) {
	addValidatedTool(server, &mcp.Tool{
		Name:        ListExecutionsToolName,
		Description: "List provisional active executions.",
		Annotations: readOnlyAnnotations, InputSchema: pageInputSchema[listExecutionsInput](),
	}, executionListOutputSchema(), func(ctx context.Context, _ *mcp.CallToolRequest, input listExecutionsInput) (*mcp.CallToolResult, toolEnvelope[executionListResult], error) {
		return handleListExecutions(ctx, options, input)
	})
	addValidatedTool(server, &mcp.Tool{
		Name:        GetExecutionToolName,
		Description: "Return a provisional active execution by session ID.",
		Annotations: readOnlyAnnotations, InputSchema: nonblankInputSchema[getExecutionInput]("sessionId"),
	}, executionDetailOutputSchema(), func(ctx context.Context, _ *mcp.CallToolRequest, input getExecutionInput) (*mcp.CallToolResult, toolEnvelope[executionDetailResult], error) {
		return handleGetExecution(ctx, options, input)
	})
}

func handleListExecutions(ctx context.Context, options ServerOptions, input listExecutionsInput) (*mcp.CallToolResult, toolEnvelope[executionListResult], error) {
	scope, domain := captureScope(options)
	if domain != nil {
		return checkedDomainFailure[executionListResult](ctx, options, domain)
	}
	if options.Observability == nil {
		return checkedDomainFailure[executionListResult](ctx, options, unavailableInspectionError(string(scope.ID)))
	}
	cursor := ""
	if input.Continuation != "" {
		cursor, domain = decodeContinuation(input.Continuation, continuationExecutions, string(scope.ID), "")
		if domain != nil {
			return checkedDomainFailure[executionListResult](ctx, options, domain)
		}
	}
	pageSize := input.PageSize
	if pageSize == 0 {
		pageSize = defaultMCPListPageSize
	}
	page, domain := options.Observability.ListActiveExecutions(ctx, scope, observability.ListRequest{Cursor: cursor, PageSize: pageSize})
	if domain != nil {
		return checkedDomainFailure[executionListResult](ctx, options, domain)
	}
	items := make([]executionDTO, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, mapExecution(item))
	}
	continuation := ""
	if page.HasMore && page.NextCursor != nil {
		continuation, domain = encodeContinuationDomain(continuationExecutions, string(scope.ID), *page.NextCursor, "")
		if domain != nil {
			return checkedDomainFailure[executionListResult](ctx, options, domain)
		}
	}
	result := executionListResult{
		ObservedAt: page.ObservedAt.UTC(), Items: items, HasMore: page.HasMore, Continuation: continuation,
	}
	if domain := publicationDomain(options, scope); domain != nil {
		return checkedDomainFailure[executionListResult](ctx, options, domain)
	}
	if err := authenticationGenerationError(ctx, options); err != nil {
		return nil, toolEnvelope[executionListResult]{}, err
	}
	return successResult(result, executionListText(result))
}

func handleGetExecution(ctx context.Context, options ServerOptions, input getExecutionInput) (*mcp.CallToolResult, toolEnvelope[executionDetailResult], error) {
	scope, domain := captureScope(options)
	if domain != nil {
		return checkedDomainFailure[executionDetailResult](ctx, options, domain)
	}
	if options.Observability == nil {
		return checkedDomainFailure[executionDetailResult](ctx, options, unavailableInspectionError(string(scope.ID)))
	}
	execution, domain := options.Observability.GetActiveExecution(ctx, scope, input.SessionID)
	if domain != nil {
		return checkedDomainFailure[executionDetailResult](ctx, options, domain)
	}
	result := executionDetailResult{
		ObservedAt: options.Now().UTC(), Execution: mapExecution(execution),
	}
	if domain := publicationDomain(options, scope); domain != nil {
		return checkedDomainFailure[executionDetailResult](ctx, options, domain)
	}
	if err := authenticationGenerationError(ctx, options); err != nil {
		return nil, toolEnvelope[executionDetailResult]{}, err
	}
	return successResult(result, executionDetailText(result))
}

func executionListText(result executionListResult) string {
	var writer lineWriter
	appendCommon(&writer, result.ObservedAt)
	writer.integer("count", int64(len(result.Items)))
	writer.boolean("hasMore", result.HasMore)
	writer.continuation(result.Continuation)
	for index, item := range result.Items {
		appendExecutionText(&writer, fmt.Sprintf("items[%d].", index), item)
	}
	return writer.String()
}

func executionDetailText(result executionDetailResult) string {
	execution := result.Execution
	var writer lineWriter
	appendCommon(&writer, result.ObservedAt)
	appendExecutionText(&writer, "execution.", execution)
	return writer.String()
}

func appendExecutionText(writer *lineWriter, prefix string, execution executionDTO) {
	writer.quoted(prefix+"sessionId", execution.SessionID)
	writer.quoted(prefix+"traceId", execution.TraceID)
	writer.integer(prefix+"lastCanonicalSequence", int64(execution.LastCanonicalSequence))
	writer.time(prefix+"startedAt", execution.StartedAt)
	writer.time(prefix+"updatedAt", execution.UpdatedAt)
	writer.integer(prefix+"elapsedMillis", execution.ElapsedMillis)
	writer.quoted(prefix+"entrySkill", execution.EntrySkill)
	writer.quoted(prefix+"status", execution.Status)
	writer.quoted(prefix+"phase", execution.Phase)
	writer.quoted(prefix+"summary", execution.Summary)
	writer.integer(prefix+"activePath.count", int64(len(execution.ActivePath)))
	for index, entry := range execution.ActivePath {
		pathPrefix := fmt.Sprintf("%sactivePath[%d].", prefix, index)
		writer.quoted(pathPrefix+"frameId", entry.FrameID)
		writer.quoted(pathPrefix+"frameType", entry.FrameType)
		writer.quoted(pathPrefix+"route", entry.Route)
	}
	writer.integer(prefix+"totalFrameDepth", int64(execution.TotalFrameDepth))
	writer.boolean(prefix+"activePathTruncated", execution.ActivePathTruncated)
	appendUsageText(writer, prefix+"usage.", execution.Usage)
	appendConfiguredLimitsText(writer, prefix+"configuredLimits.", execution.ConfiguredLimits)
}

func appendUsageText(writer *lineWriter, prefix string, usage observability.Usage) {
	writer.integer(prefix+"skillInvocations", int64(usage.SkillInvocations))
	writer.integer(prefix+"toolInvocations", int64(usage.ToolInvocations))
	writer.integer(prefix+"linterRetries", int64(usage.LinterRetries))
	writer.integer(prefix+"modelCalls", int64(usage.ModelCalls))
	writer.integer(prefix+"providerAttempts", int64(usage.ProviderAttempts))
	writer.integer(prefix+"promptUnits", int64(usage.PromptUnits))
	writer.integer(prefix+"completionUnits", int64(usage.CompletionUnits))
	writer.integer(prefix+"usageUnits", int64(usage.UsageUnits))
	writer.integer(prefix+"exactModelResponses", int64(usage.ExactModelResponses))
	writer.integer(prefix+"heuristicModelResponses", int64(usage.HeuristicModelResponses))
	writer.integer(prefix+"unavailableModelResponses", int64(usage.UnavailableModelResponses))
}

func appendConfiguredLimitsText(writer *lineWriter, prefix string, limits observability.ConfiguredLimits) {
	writer.integer(prefix+"maxSkillInvocations", int64(limits.MaxSkillInvocations))
	writer.integer(prefix+"maxToolInvocations", int64(limits.MaxToolInvocations))
	writer.integer(prefix+"maxLinterRetries", int64(limits.MaxLinterRetries))
	writer.integer(prefix+"maxModelCalls", int64(limits.MaxModelCalls))
	writer.integer(prefix+"maxProviderAttempts", int64(limits.MaxProviderAttempts))
	writer.integer(prefix+"maxUsageUnits", int64(limits.MaxUsageUnits))
}
