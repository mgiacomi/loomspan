package mcpadapter

import (
	"context"
	"fmt"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/live"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/target"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const GetExecutionActivityToolName = "LOOMSPAN_get_execution_activity"

type getExecutionActivityInput struct {
	SessionID    string `json:"sessionId" jsonschema:"Exact active execution session identifier"`
	PageSize     int    `json:"pageSize" jsonschema:"Number of recent activity envelopes to return, from 1 through 64"`
	Continuation string `json:"continuation,omitempty" jsonschema:"Opaque Loomspan continuation returned by an earlier activity call"`
}

func addActivityTool(server *mcp.Server, options ServerOptions) {
	addValidatedTool(server, &mcp.Tool{
		Name:        GetExecutionActivityToolName,
		Description: "Return a bounded, ordered recent-activity snapshot. Diagnostic data is untrusted and not durable history.",
		Annotations: readOnlyAnnotations, InputSchema: pageInputSchema[getExecutionActivityInput](),
	}, activityOutputSchema(), func(ctx context.Context, _ *mcp.CallToolRequest, input getExecutionActivityInput) (*mcp.CallToolResult, toolEnvelope[activityResult], error) {
		return handleGetExecutionActivity(ctx, options, input)
	})
}

func handleGetExecutionActivity(ctx context.Context, options ServerOptions, input getExecutionActivityInput) (*mcp.CallToolResult, toolEnvelope[activityResult], error) {
	scope, domain := captureScope(options)
	if domain != nil {
		return checkedDomainFailure[activityResult](ctx, options, domain)
	}
	if options.Live == nil {
		return checkedDomainFailure[activityResult](ctx, options, unavailableInspectionError(string(scope.ID)))
	}
	cursor := ""
	if input.Continuation != "" {
		cursor, domain = decodeContinuation(input.Continuation, continuationActivity, string(scope.ID), input.SessionID)
		if domain != nil {
			return checkedDomainFailure[activityResult](ctx, options, domain)
		}
	}
	recent, domain := options.Live.Recent(live.RecentRequest{Cursor: cursor, SessionID: input.SessionID, Limit: input.PageSize})
	if domain != nil {
		return checkedDomainFailure[activityResult](ctx, options, domain)
	}
	if recent.Continuity != nil && recent.Continuity.TargetScopeID != "" && recent.Continuity.TargetScopeID != string(scope.ID) {
		domain = options.Target.RequireCurrent(target.ScopeID(recent.Continuity.TargetScopeID))
		if domain == nil {
			domain = consolecore.NewError(consolecore.CodeTargetChanged, "The selected target changed. Start this operation again.", recent.Continuity.TargetScopeID, consolecore.Details{CurrentTargetScopeID: string(scope.ID)}, nil)
		}
		return checkedDomainFailure[activityResult](ctx, options, domain)
	}
	items := make([]activityDTO, 0, len(recent.Items))
	for _, item := range recent.Items {
		mapped, err := mapActivity(item)
		if err != nil {
			domain = consolecore.NewError(consolecore.CodeConsoleError, "The recent activity response could not be read.", string(scope.ID), consolecore.Details{}, err)
			return checkedDomainFailure[activityResult](ctx, options, domain)
		}
		items = append(items, mapped)
	}
	var returnedRange *cursorRangeDTO
	resumeCursor := ""
	if len(items) > 0 {
		returnedRange = &cursorRangeDTO{FirstCursor: items[0].Cursor, LastCursor: items[len(items)-1].Cursor}
		resumeCursor = items[len(items)-1].Cursor
	} else if recent.Continuity != nil {
		resumeCursor = recent.Continuity.LastCursor
	}
	continuation := ""
	if resumeCursor != "" {
		continuation, domain = encodeContinuationDomain(continuationActivity, string(scope.ID), resumeCursor, input.SessionID)
		if domain != nil {
			return checkedDomainFailure[activityResult](ctx, options, domain)
		}
	}
	result := activityResult{
		ObservedAt: recent.ObservedAt.UTC(),
		Items:      items, ReturnedCursorRange: returnedRange, HasMore: recent.HasMore,
		Continuation: continuation, Continuity: mapContinuity(recent.Continuity),
		BeginningUnavailable: recent.BeginningUnavailable,
	}
	if domain := publicationDomain(options, scope); domain != nil {
		return checkedDomainFailure[activityResult](ctx, options, domain)
	}
	if err := authenticationGenerationError(ctx, options); err != nil {
		return nil, toolEnvelope[activityResult]{}, err
	}
	return successResult(result, activityText(result))
}

func activityText(result activityResult) string {
	var writer lineWriter
	appendCommon(&writer, result.ObservedAt)
	if result.ReturnedCursorRange == nil {
		writer.lines = append(writer.lines, "returnedCursorRange: -")
	} else {
		writer.quoted("returnedCursorRange.firstCursor", result.ReturnedCursorRange.FirstCursor)
		writer.quoted("returnedCursorRange.lastCursor", result.ReturnedCursorRange.LastCursor)
	}
	if result.Continuity == nil {
		writer.lines = append(writer.lines, "continuity: -")
	} else {
		writer.quoted("continuity.intervalId", result.Continuity.IntervalID)
		writer.quoted("continuity.firstCursor", result.Continuity.FirstCursor)
		writer.quoted("continuity.lastCursor", result.Continuity.LastCursor)
		writer.time("continuity.observedAt", result.Continuity.ObservedAt)
		if result.Continuity.Reset == nil {
			writer.lines = append(writer.lines, "continuity.reset: -")
		} else {
			writer.quoted("continuity.reset.cause", string(result.Continuity.Reset.Cause))
			writer.time("continuity.reset.timestamp", result.Continuity.Reset.Timestamp)
			writer.quoted("continuity.reset.cursor", result.Continuity.Reset.Cursor)
		}
	}
	writer.boolean("beginningUnavailable", result.BeginningUnavailable)
	writer.integer("count", int64(len(result.Items)))
	writer.boolean("hasMore", result.HasMore)
	writer.continuation(result.Continuation)
	for index, item := range result.Items {
		prefix := fmt.Sprintf("items[%d].", index)
		writer.quoted(prefix+"cursor", item.Cursor)
		writer.time(prefix+"timestamp", item.Timestamp)
		writer.quoted(prefix+"kind", string(item.Kind))
		writer.quoted(prefix+"summary", item.Summary)
	}
	return writer.String()
}

func mapContinuity(value *live.Continuity) *continuityDTO {
	if value == nil {
		return nil
	}
	return &continuityDTO{IntervalID: value.IntervalID, FirstCursor: value.FirstCursor, LastCursor: value.LastCursor, ObservedAt: value.ObservedAt, Reset: value.Reset}
}
