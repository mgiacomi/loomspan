package agenteval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/applicationclient"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/live"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/mcpadapter"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/mcpcredential"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/observability"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/profile"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/target"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/traceanalysis"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/traceinventory"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/traceresolution"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/workspace"
)

const SessionFilename = "session.json"

type Session struct {
	SchemaVersion  int       `json:"schemaVersion"`
	CaseID         string    `json:"caseId"`
	Endpoint       string    `json:"endpoint"`
	Key            string    `json:"key,omitempty"`
	StartedAt      time.Time `json:"startedAt"`
	ConsoleVersion string    `json:"consoleVersion"`
	ConsoleCommit  string    `json:"consoleCommit"`
}

type RunningServer struct {
	Session   Session
	http      *http.Server
	listen    net.Listener
	profile   *profile.Profile
	mcp       *mcpadapter.Server
	artifacts *artifact.Service
	workspace *workspace.Workspace
	target    *target.Context
	live      *live.Service
	upstream  *httptest.Server
}

func StartServer(output string, caseValue Case, consoleVersion, consoleCommit string) (*RunningServer, error) {
	abs, err := filepath.Abs(output)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(abs, 0o700); err != nil {
		return nil, err
	}
	if caseValue.MCPAvailable != nil && !*caseValue.MCPAvailable {
		session := Session{SchemaVersion: 1, CaseID: caseValue.ID, Endpoint: "unavailable", StartedAt: time.Now().UTC(),
			ConsoleVersion: consoleVersion, ConsoleCommit: consoleCommit}
		content, err := json.MarshalIndent(session, "", "  ")
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(abs, SessionFilename), append(content, '\n'), 0o600); err != nil {
			return nil, err
		}
		return &RunningServer{Session: session}, nil
	}
	owned, err := profile.Open(filepath.Join(abs, "profile", "config.yaml"))
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*RunningServer, error) {
		_ = owned.Close()
		return nil, err
	}
	store, err := mcpcredential.Open(owned.Directory, nil)
	if err != nil {
		return fail(err)
	}
	prepared, err := store.Prepare()
	if err != nil {
		return fail(err)
	}
	key, err := store.CommitEnable(prepared)
	if err != nil {
		return fail(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fail(err)
	}
	_, portText, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(portText)
	tracker := mcpadapter.NewTracker()
	fixtureWorkspace, err := workspace.Open(filepath.Join(abs, "fixture-workspace"))
	if err != nil {
		listener.Close()
		return fail(err)
	}
	analysis := traceanalysis.NewServiceForCompatibilityVersion(nil, consoleVersion)
	pr34Case := caseValue.ID == "pr34-tools-only-active-execution-review" || caseValue.ID == "pr34-skill-assisted-active-execution-review"
	artifacts, err := artifact.New(artifact.Config{MaxBytes: 128 << 20, IdleTTL: time.Hour}, artifact.Dependencies{
		Workspace: fixtureWorkspace,
		TraceLoader: func(context.Context, target.Scope, string) (artifact.TraceMetadata, *consolecore.Error) {
			if !pr34Case {
				return artifact.TraceMetadata{}, consolecore.NewError(consolecore.CodeTargetUnavailable, "Evaluation cases do not acquire application traces.", "", consolecore.Details{}, nil)
			}
			return artifact.TraceMetadata{}, consolecore.NewError(consolecore.CodeNotFound, "The evaluation trace is unavailable.", "", consolecore.Details{}, nil)
		},
		StreamOpener: func(context.Context, target.Scope, string) (*applicationclient.ArtifactStream, *consolecore.Error) {
			return nil, consolecore.NewError(consolecore.CodeTargetUnavailable, "Evaluation cases do not acquire application traces.", "", consolecore.Details{}, nil)
		},
		Processor: analysis,
	})
	if err != nil {
		fixtureWorkspace.Close()
		listener.Close()
		return fail(err)
	}
	analysis.SetArtifactService(artifacts)
	repository, _ := RepositoryRoot()
	for _, source := range caseValue.FixtureSources {
		if filepath.Ext(source) != ".ndjson" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(repository, filepath.FromSlash(source)))
		if err != nil {
			artifacts.Close()
			fixtureWorkspace.Close()
			listener.Close()
			return fail(err)
		}
		if _, domain := artifacts.Import(context.Background(), bytes.NewReader(raw), int64(len(raw))); domain != nil {
			artifacts.Close()
			fixtureWorkspace.Close()
			listener.Close()
			return fail(fmt.Errorf("import evaluation fixture %s: %v", source, domain))
		}
	}
	inventory := traceinventory.New(artifacts, nil, nil, time.Now)
	var targetContext *target.Context
	var liveService *live.Service
	var observabilityService *observability.Service
	var upstream *httptest.Server
	statusProvider := func() consolecore.StatusSnapshot { return evaluationStatus(caseValue.ID, time.Now().UTC()) }
	if caseValue.ID == "slow-execution" {
		targetContext, liveService, observabilityService, upstream, err = startSlowEvaluationTarget(repository, consoleVersion)
		if err != nil {
			artifacts.Close()
			fixtureWorkspace.Close()
			listener.Close()
			return fail(err)
		}
		statusProvider = func() consolecore.StatusSnapshot { return targetContext.Snapshot().Status }
	} else if pr34Case {
		targetContext, liveService, observabilityService, upstream, err = startPR34EvaluationTarget(repository, caseValue.FixtureSources[0], consoleVersion, artifacts)
		if err != nil {
			artifacts.Close()
			fixtureWorkspace.Close()
			listener.Close()
			return fail(err)
		}
		statusProvider = func() consolecore.StatusSnapshot { return targetContext.Snapshot().Status }
	}
	var resolverCatalog traceresolution.CatalogService
	var resolverTarget traceresolution.TargetProvider
	if observabilityService != nil {
		resolverCatalog = observabilityService
	}
	if targetContext != nil {
		resolverTarget = targetContext
	}
	resolver := traceresolution.New(artifacts, resolverCatalog, resolverTarget)
	mcpServer := mcpadapter.NewServer(mcpadapter.ServerOptions{
		Port: port, Credentials: store, Tracker: tracker,
		Status:                 statusProvider,
		EvaluationCapabilities: &caseValue.Capabilities,
		TraceResolver:          resolver, TraceAnalysis: analysis, TraceInventory: inventory,
		Target: targetContext, Live: liveService, Observability: observabilityService,
	})
	httpServer := &http.Server{Handler: mcpServer.Handler(), ReadHeaderTimeout: 5 * time.Second}
	running := &RunningServer{http: httpServer, listen: listener, profile: owned, mcp: mcpServer, artifacts: artifacts,
		workspace: fixtureWorkspace, target: targetContext, live: liveService, upstream: upstream}
	running.Session = Session{SchemaVersion: 1, CaseID: caseValue.ID, Endpoint: "http://" + listener.Addr().String() + "/mcp",
		Key: key, StartedAt: time.Now().UTC(), ConsoleVersion: consoleVersion, ConsoleCommit: consoleCommit}
	content, err := json.MarshalIndent(running.Session, "", "  ")
	if err != nil {
		listener.Close()
		return fail(err)
	}
	content = append(content, '\n')
	if err := os.WriteFile(filepath.Join(abs, SessionFilename), content, 0o600); err != nil {
		listener.Close()
		return fail(err)
	}
	go func() { _ = httpServer.Serve(listener) }()
	return running, nil
}

func startSlowEvaluationTarget(repository, consoleVersion string) (*target.Context, *live.Service, *observability.Service, *httptest.Server, error) {
	read := func(relative string) ([]byte, error) {
		return os.ReadFile(filepath.Join(repository, filepath.FromSlash(relative)))
	}
	instance, err := read("loomspan-console-fixtures/application-rest/instance-status.json")
	if err != nil {
		return nil, nil, nil, nil, err
	}
	activePage, err := read("loomspan-console-fixtures/application-rest/active-executions-page.json")
	if err != nil {
		return nil, nil, nil, nil, err
	}
	activeDetail, err := read("loomspan-console-fixtures/application-rest/active-execution-detail.json")
	if err != nil {
		return nil, nil, nil, nil, err
	}
	replay, err := read("loomspan-console-fixtures/application-sse/replay.sse")
	if err != nil {
		return nil, nil, nil, nil, err
	}
	const instanceID = "11111111-1111-4111-8111-111111111111"
	upstream := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set(applicationclient.InstanceIDHeader, instanceID)
		switch {
		case strings.HasSuffix(request.URL.Path, "/instance"):
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write(instance)
		case strings.HasSuffix(request.URL.Path, "/active-executions"):
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write(activePage)
		case strings.Contains(request.URL.Path, "/active-executions/"):
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write(activeDetail)
		case strings.HasSuffix(request.URL.Path, "/activity"):
			response.Header().Set("Content-Type", "text/event-stream")
			requested := request.URL.Query().Get("afterCursor")
			body := bytes.Replace(replay, []byte(`"afterCursor":"0"`), []byte(`"afterCursor":"`+requested+`"`), 1)
			body = bytes.ReplaceAll(body, []byte("id: 7"), []byte("id: 10"))
			body = bytes.ReplaceAll(body, []byte("id: 8"), []byte("id: 11"))
			body = bytes.ReplaceAll(body, []byte(`"cursor":"7"`), []byte(`"cursor":"10"`))
			body = bytes.ReplaceAll(body, []byte(`"cursor":"8"`), []byte(`"cursor":"11"`))
			_, _ = response.Write(body)
			if flusher, ok := response.(http.Flusher); ok {
				flusher.Flush()
			}
			<-request.Context().Done()
		default:
			http.NotFound(response, request)
		}
	}))
	factory := func(address applicationclient.Address) (target.ProbeClient, error) {
		return applicationclient.New(address, applicationclient.NetworkPolicy{ConnectTimeout: time.Second,
			ResponseHeaderTimeout: time.Second, RequestTimeout: 2 * time.Second}, consoleVersion)
	}
	targetContext, err := target.New(factory, func() (target.ScopeID, error) { return target.ScopeID("eval-scope-live"), nil }, time.Now)
	if err != nil {
		upstream.Close()
		return nil, nil, nil, nil, err
	}
	liveService := live.NewService(context.Background())
	observabilityService := observability.New()
	liveService.SetBaselineLoader(func(ctx context.Context, scope target.Scope) (live.Baseline, *consolecore.Error) {
		page, domain := observabilityService.ListActiveExecutions(ctx, scope, observability.ListRequest{})
		resumeCursor := ""
		if page.ResumeCursor != nil {
			resumeCursor = *page.ResumeCursor
		}
		return live.Baseline{Executions: page.Items, ResumeCursor: resumeCursor, ObservedAt: page.ObservedAt}, domain
	})
	if err := targetContext.RegisterOwner("agent-eval-live", liveService); err != nil {
		liveService.Close()
		targetContext.Close()
		upstream.Close()
		return nil, nil, nil, nil, err
	}
	targetContext.StartServing()
	if _, domain := targetContext.SelectAndConnect(context.Background(), upstream.URL, []byte(strings.Repeat("e", 32))); domain != nil {
		liveService.Close()
		targetContext.Close()
		upstream.Close()
		return nil, nil, nil, nil, fmt.Errorf("connect slow evaluation target: %v", domain)
	}
	for attempt := 0; attempt < 100; attempt++ {
		if recent, domain := liveService.Recent(live.RecentRequest{Limit: 64}); domain == nil && len(recent.Items) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	return targetContext, liveService, observabilityService, upstream, nil
}

type pr34Execution struct {
	SessionID             string `json:"sessionId"`
	TraceID               string `json:"traceId"`
	LastCanonicalSequence int    `json:"lastCanonicalSequence"`
	ProviderAttempts      int    `json:"providerAttempts"`
	ModelCalls            int    `json:"modelCalls"`
}

type pr34Activity struct {
	Cursor    int    `json:"cursor"`
	SessionID string `json:"sessionId"`
	TraceID   string `json:"traceId"`
	Kind      string `json:"kind"`
}

type pr34Replay struct {
	FillerFromCursor    int            `json:"fillerFromCursor"`
	FillerThroughCursor int            `json:"fillerThroughCursor"`
	FillerSessionID     string         `json:"fillerSessionId"`
	FillerTraceID       string         `json:"fillerTraceId"`
	Activities          []pr34Activity `json:"activities"`
}

type pr34Observation struct {
	ID                  string          `json:"id"`
	Executions          []pr34Execution `json:"executions"`
	Replay              *pr34Replay     `json:"replay,omitempty"`
	Activities          []pr34Activity  `json:"activities,omitempty"`
	CompletedSessionIDs []string        `json:"completedSessionIds,omitempty"`
}

type pr34Fixture struct {
	SchemaVersion int               `json:"schemaVersion"`
	Observations  []pr34Observation `json:"observations"`
}

func startPR34EvaluationTarget(repository, fixtureSource, consoleVersion string, artifacts *artifact.Service) (*target.Context, *live.Service, *observability.Service, *httptest.Server, error) {
	const instanceID = "11111111-1111-4111-8111-111111111111"
	rawFixture, err := os.ReadFile(filepath.Join(repository, filepath.FromSlash(fixtureSource)))
	if err != nil {
		return nil, nil, nil, nil, err
	}
	var fixture pr34Fixture
	if err := json.Unmarshal(rawFixture, &fixture); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("decode PR 34 fixture: %w", err)
	}
	if fixture.SchemaVersion != 1 || len(fixture.Observations) != 2 || fixture.Observations[0].ID != "initial" || fixture.Observations[1].ID != "later" ||
		fixture.Observations[0].Replay == nil || len(fixture.Observations[0].Replay.Activities) == 0 || len(fixture.Observations[1].Activities) == 0 {
		return nil, nil, nil, nil, fmt.Errorf("PR 34 fixture must contain initial and later observations")
	}
	initial, later := fixture.Observations[0], fixture.Observations[1]
	activeJSON := func(execution pr34Execution) ([]byte, error) {
		return json.Marshal(map[string]any{
			"sessionId": execution.SessionID, "traceId": execution.TraceID, "lastCanonicalSequence": execution.LastCanonicalSequence,
			"startedAt": "2026-08-21T12:00:00Z", "updatedAt": "2026-08-21T12:00:05Z", "elapsedMillis": 5000,
			"entrySkill": "review.entry", "status": "ACTIVE", "phase": "RUNNING", "summary": "provisional runtime fact",
			"activePath": []any{}, "totalFrameDepth": 0, "activePathTruncated": false,
			"usage": map[string]any{"skillInvocations": 1, "toolInvocations": 0, "linterRetries": 0, "modelCalls": execution.ModelCalls,
				"providerAttempts": execution.ProviderAttempts, "promptUnits": 0, "completionUnits": 0, "usageUnits": 0,
				"exactModelResponses": execution.ModelCalls, "heuristicModelResponses": 0, "unavailableModelResponses": 0},
			"configuredLimits": map[string]any{"maxSkillInvocations": 64, "maxToolInvocations": 128, "maxLinterRetries": 32,
				"maxModelCalls": 64, "maxProviderAttempts": 0, "maxUsageUnits": 0},
		})
	}
	pageJSON := func(observation pr34Observation) ([]byte, map[string][]byte, error) {
		items := make([]json.RawMessage, 0, len(observation.Executions))
		details := make(map[string][]byte, len(observation.Executions))
		for _, execution := range observation.Executions {
			encoded, encodeErr := activeJSON(execution)
			if encodeErr != nil {
				return nil, nil, encodeErr
			}
			items = append(items, encoded)
			details[execution.SessionID] = encoded
		}
		page, encodeErr := json.Marshal(map[string]any{"items": items, "hasMore": false, "nextCursor": nil,
			"observedAt": "2026-08-21T12:00:06Z", "resumeCursor": "0"})
		return page, details, encodeErr
	}
	initialPage, initialDetails, err := pageJSON(initial)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	laterPage, laterDetails, err := pageJSON(later)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	writeActivity := func(builder *strings.Builder, activity pr34Activity) {
		fmt.Fprintf(builder, "id: %d\nevent: activity\ndata: {\"instanceId\":%q,\"cursor\":%q,\"sessionId\":%q,\"traceId\":%q,\"canonicalSequence\":%d,\"timestamp\":\"2026-08-21T12:00:06Z\",\"kind\":%q,\"executionStatus\":\"ACTIVE\",\"summary\":\"provisional runtime fact\",\"details\":{}}\n\n", activity.Cursor, instanceID, strconv.Itoa(activity.Cursor), activity.SessionID, activity.TraceID, activity.Cursor, activity.Kind)
	}
	var initialReplay strings.Builder
	fmt.Fprintf(&initialReplay, "event: handshake\ndata: {\"instanceId\":%q,\"observedAt\":\"2026-08-21T12:00:06Z\",\"afterCursor\":\"0\"}\n\n", instanceID)
	writeActivity(&initialReplay, initial.Replay.Activities[0])
	for cursor := initial.Replay.FillerFromCursor; cursor <= initial.Replay.FillerThroughCursor; cursor++ {
		writeActivity(&initialReplay, pr34Activity{Cursor: cursor, SessionID: initial.Replay.FillerSessionID, TraceID: initial.Replay.FillerTraceID, Kind: "STEP_STARTED"})
	}
	for _, activity := range initial.Replay.Activities[1:] {
		writeActivity(&initialReplay, activity)
	}
	var laterReplay strings.Builder
	for _, activity := range later.Activities {
		writeActivity(&laterReplay, activity)
	}
	latestInitialCursor := initial.Replay.Activities[len(initial.Replay.Activities)-1].Cursor
	var activeListCalls atomic.Int32
	var laterActive atomic.Bool
	laterReady := make(chan struct{})
	var laterOnce sync.Once

	upstream := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set(applicationclient.InstanceIDHeader, instanceID)
		switch {
		case strings.HasSuffix(request.URL.Path, "/instance"):
			response.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(response, `{"instanceId":%q,"consoleCompatibilityVersion":%q,"observedAt":"2026-08-21T12:00:06Z","liveMonitoringAvailable":true,"registeredSkillCount":1,"activeExecutionCount":5,"catalogedTraceCount":0,"tracePersistencePolicy":"PERSISTENT","completionGraceTtl":"PT2M","traceCatalogMetadataTtl":"PT168H"}`, instanceID, consoleVersion)
		case strings.HasSuffix(request.URL.Path, "/active-executions"):
			response.Header().Set("Content-Type", "application/json")
			if activeListCalls.Add(1) >= 3 {
				laterActive.Store(true)
				laterOnce.Do(func() { close(laterReady) })
				_, _ = response.Write(laterPage)
			} else {
				_, _ = response.Write(initialPage)
			}
		case strings.Contains(request.URL.Path, "/active-executions/"):
			sessionID := request.URL.Path[strings.LastIndex(request.URL.Path, "/")+1:]
			details := initialDetails
			if laterActive.Load() {
				details = laterDetails
			}
			if body := details[sessionID]; body != nil {
				response.Header().Set("Content-Type", "application/json")
				_, _ = response.Write(body)
				return
			}
			response.Header().Set("Content-Type", "application/json")
			response.WriteHeader(http.StatusNotFound)
			_, _ = response.Write([]byte(`{"status":404,"code":"NOT_FOUND","message":"The requested observability resource was not found"}`))
		case strings.HasSuffix(request.URL.Path, "/activity"):
			response.Header().Set("Content-Type", "text/event-stream")
			body := strings.Replace(initialReplay.String(), `"afterCursor":"0"`, `"afterCursor":"`+request.URL.Query().Get("afterCursor")+`"`, 1)
			_, _ = response.Write([]byte(body))
			if flusher, ok := response.(http.Flusher); ok {
				flusher.Flush()
			}
			select {
			case <-laterReady:
				_, _ = response.Write([]byte(laterReplay.String()))
				if flusher, ok := response.(http.Flusher); ok {
					flusher.Flush()
				}
			case <-request.Context().Done():
				return
			}
			<-request.Context().Done()
		default:
			response.Header().Set("Content-Type", "application/json")
			response.WriteHeader(http.StatusNotFound)
			_, _ = response.Write([]byte(`{"status":404,"code":"NOT_FOUND","message":"The requested observability resource was not found"}`))
		}
	}))
	factory := func(address applicationclient.Address) (target.ProbeClient, error) {
		return applicationclient.New(address, applicationclient.NetworkPolicy{ConnectTimeout: time.Second, ResponseHeaderTimeout: time.Second, RequestTimeout: 2 * time.Second}, consoleVersion)
	}
	targetContext, err := target.New(factory, func() (target.ScopeID, error) { return target.ScopeID("eval-scope-pr34"), nil }, time.Now)
	if err != nil {
		upstream.Close()
		return nil, nil, nil, nil, err
	}
	liveService := live.NewService(context.Background())
	observabilityService := observability.New()
	liveService.SetBaselineLoader(func(ctx context.Context, scope target.Scope) (live.Baseline, *consolecore.Error) {
		page, domain := observabilityService.ListActiveExecutions(ctx, scope, observability.ListRequest{})
		resumeCursor := ""
		if page.ResumeCursor != nil {
			resumeCursor = *page.ResumeCursor
		}
		return live.Baseline{Executions: page.Items, ResumeCursor: resumeCursor, ObservedAt: page.ObservedAt}, domain
	})
	if err := targetContext.RegisterOwner("agent-eval-pr34", liveService); err != nil {
		liveService.Close()
		targetContext.Close()
		upstream.Close()
		return nil, nil, nil, nil, err
	}
	if err := targetContext.RegisterOwner("agent-eval-pr34-artifacts", artifacts); err != nil {
		liveService.Close()
		targetContext.Close()
		upstream.Close()
		return nil, nil, nil, nil, err
	}
	targetContext.StartServing()
	if _, domain := targetContext.SelectAndConnect(context.Background(), upstream.URL, []byte(strings.Repeat("e", 32))); domain != nil {
		liveService.Close()
		targetContext.Close()
		upstream.Close()
		return nil, nil, nil, nil, fmt.Errorf("connect PR 34 evaluation target: %v", domain)
	}
	ready := false
	for attempt := 0; attempt < 200; attempt++ {
		if recent, domain := liveService.Recent(live.RecentRequest{Limit: 64}); domain == nil && recent.Continuity != nil && recent.Continuity.LastCursor == strconv.Itoa(latestInitialCursor) {
			ready = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !ready {
		liveService.Close()
		targetContext.Close()
		upstream.Close()
		return nil, nil, nil, nil, fmt.Errorf("PR 34 initial activity did not reach cursor %d", latestInitialCursor)
	}
	return targetContext, liveService, observabilityService, upstream, nil
}

func evaluationStatus(caseID string, observedAt time.Time) consolecore.StatusSnapshot {
	switch caseID {
	case "incompatible-target":
		return consolecore.StatusSnapshot{ObservedAt: observedAt, TargetScopeID: "eval-scope-incompatible",
			TargetSelection: consolecore.SelectionSelected, TargetConnection: consolecore.ConnectionReachable,
			TargetAuthentication: consolecore.AuthenticationEstablished, JavaGoCompatibility: consolecore.CompatibilityIncompatible,
			RuntimeIdentity: consolecore.RuntimeNotEstablished, LiveMonitoring: consolecore.LiveUnknown}
	case "target-authentication-required", "restart-persistent-mcp":
		return consolecore.StatusSnapshot{ObservedAt: observedAt, TargetScopeID: "eval-scope-auth-required",
			TargetSelection: consolecore.SelectionSelected, TargetConnection: consolecore.ConnectionReachable,
			TargetAuthentication: consolecore.AuthenticationRequired, JavaGoCompatibility: consolecore.CompatibilityNotChecked,
			RuntimeIdentity: consolecore.RuntimeNotEstablished, LiveMonitoring: consolecore.LiveUnknown}
	default:
		return consolecore.NoTargetStatus(observedAt)
	}
}

func (server *RunningServer) Wait(ctx context.Context) error {
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return server.Close(shutdown)
}

func (server *RunningServer) Close(ctx context.Context) error {
	if server == nil {
		return nil
	}
	var httpErr error
	if server.mcp != nil {
		server.mcp.CloseSessions()
	}
	if server.http != nil {
		httpErr = server.http.Shutdown(ctx)
	}
	if server.live != nil {
		server.live.Close()
	}
	if server.target != nil {
		server.target.Close()
	}
	if server.upstream != nil {
		server.upstream.Close()
	}
	if server.artifacts != nil {
		server.artifacts.Close()
	}
	var workspaceErr, profileErr error
	if server.workspace != nil {
		workspaceErr = server.workspace.Close()
	}
	if server.profile != nil {
		profileErr = server.profile.Close()
	}
	if httpErr != nil {
		return httpErr
	}
	if workspaceErr != nil {
		return workspaceErr
	}
	return profileErr
}

func LoadSession(directory string) (Session, error) {
	content, err := os.ReadFile(filepath.Join(directory, SessionFilename))
	if err != nil {
		return Session{}, err
	}
	var session Session
	if err := json.Unmarshal(content, &session); err != nil {
		return Session{}, err
	}
	if session.SchemaVersion != 1 || session.CaseID == "" || session.Endpoint == "" || (session.Endpoint != "unavailable" && session.Key == "") {
		return Session{}, fmt.Errorf("evaluation session is incomplete")
	}
	return session, nil
}
