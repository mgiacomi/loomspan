package mcpadapter

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/evidence"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/live"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/observability"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/release"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/target"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/traceanalysis"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/traceinventory"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Server struct {
	sdk     *mcp.Server
	handler http.Handler
}

type ServerOptions struct {
	Port           int
	Credentials    authenticator
	Tracker        *Tracker
	Status         StatusProvider
	Target         *target.Context
	Observability  *observability.Service
	Live           *live.Service
	Artifacts      TraceArtifactService
	TraceAnalysis  TraceAnalysisService
	TraceInventory TraceInventoryService
	Now            func() time.Time
}

type TraceInventoryService interface {
	List(context.Context, traceinventory.Query) (traceinventory.Result, *consolecore.Error)
}
type TraceArtifactService interface {
	Acquire(context.Context, target.Scope, string) (artifact.AcquiredArtifact, *consolecore.Error)
}
type TraceAnalysisService interface {
	GetSummary(context.Context, evidence.Reference, traceanalysis.SummaryRequest) (traceanalysis.TraceSummary, *consolecore.Error)
	QueryFrames(context.Context, evidence.Reference, traceanalysis.FrameQuery) (traceanalysis.Page[traceanalysis.FrameSummary], *consolecore.Error)
	QueryRecords(context.Context, evidence.Reference, traceanalysis.RecordQuery) (traceanalysis.Page[traceanalysis.RecordSummary], *consolecore.Error)
	ReadPayloadRange(context.Context, evidence.Reference, traceanalysis.RangeRequest) (traceanalysis.ByteRangeResult, *consolecore.Error)
	ReadRawArtifactRange(context.Context, evidence.Reference, traceanalysis.RangeRequest) (traceanalysis.ByteRangeResult, *consolecore.Error)
}

func NewServer(options ServerOptions) *Server {
	if options.Now == nil {
		options.Now = time.Now
	}
	sdk := mcp.NewServer(&mcp.Implementation{Name: "loomspan-console", Version: release.ProductVersion()}, nil)
	addRuntimeTool(sdk, options.Status, options.Credentials)
	addSkillTools(sdk, options)
	addExecutionTools(sdk, options)
	addActivityTool(sdk, options)
	addTraceTools(sdk, options)
	addSkillResource(sdk, options)
	addTraceResources(sdk, options)
	streamable := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return sdk }, &mcp.StreamableHTTPOptions{
		Stateless: true, JSONResponse: true, MaxRequestBodyBytes: maxRequestBody, PropagateRequestCancellation: true,
	})
	return &Server{sdk: sdk, handler: SecurityHandler(options.Port, options.Credentials, options.Tracker, streamable)}
}

func (server *Server) Handler() http.Handler { return server.handler }

func (server *Server) CloseSessions() {
	if server == nil {
		return
	}
	for session := range server.sdk.Sessions() {
		_ = session.Close()
	}
}

func captureScope(options ServerOptions) (target.Scope, *consolecore.Error) {
	if options.Target == nil {
		return target.Scope{}, consolecore.NewError(consolecore.CodeConsoleError, "Runtime inspection is unavailable.", "", consolecore.Details{}, nil)
	}
	return options.Target.Capture()
}

func publicationDomain(options ServerOptions, scope target.Scope) *consolecore.Error {
	if options.Target == nil {
		return consolecore.NewError(consolecore.CodeConsoleError, "Runtime inspection is unavailable.", string(scope.ID), consolecore.Details{}, nil)
	}
	return options.Target.RequireCurrent(scope.ID)
}

func authenticationGenerationError(ctx context.Context, options ServerOptions) error {
	if options.Credentials == nil {
		return fmt.Errorf("INTERNAL: MCP authentication is unavailable")
	}
	if generation, ok := admittedGeneration(ctx); ok && options.Credentials.Snapshot().Generation != generation {
		return fmt.Errorf("INTERNAL: MCP authentication generation changed")
	}
	return nil
}

func checkedDomainFailure[T any](ctx context.Context, options ServerOptions, domain *consolecore.Error) (*mcp.CallToolResult, toolEnvelope[T], error) {
	if err := authenticationGenerationError(ctx, options); err != nil {
		return nil, toolEnvelope[T]{}, err
	}
	return domainFailure[T](domain)
}

func unavailableInspectionError(scopeID string) *consolecore.Error {
	return consolecore.NewError(consolecore.CodeConsoleError, "Runtime inspection is unavailable.", scopeID, consolecore.Details{}, nil)
}
