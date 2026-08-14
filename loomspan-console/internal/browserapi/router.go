package browserapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/browserauth"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/evidence"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/live"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/mcpcredential"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/observability"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/release"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/target"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/traceanalysis"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/traceinventory"
)

const csrfHeader = "X-loomspan-Console-CSRF"

type ArtifactService interface {
	Acquire(ctx context.Context, scope target.Scope, traceID string) (artifact.AcquiredArtifact, *consolecore.Error)
	Import(context.Context, io.Reader, int64) (artifact.AcquiredArtifact, *consolecore.Error)
	ImportLimit() int64
	Lookup(evidence.Reference, string) (artifact.LookupResult, *consolecore.Error)
	StorageSnapshot() (artifact.StorageSnapshot, *consolecore.Error)
	Remove(evidence.Reference, string) *consolecore.Error
	ClearExpired() *consolecore.Error
	ClearAllUnused() *consolecore.Error
}

// TraceAnalysisService is the internal, adapter-facing query surface.  Browser
// callers identify an installed artifact by trace ID; the adapter resolves the
// opaque handle and never exposes it in analysis responses.
type TraceAnalysisService interface {
	GetSummary(context.Context, evidence.Reference, traceanalysis.SummaryRequest) (traceanalysis.TraceSummary, *consolecore.Error)
	QueryFrames(context.Context, evidence.Reference, traceanalysis.FrameQuery) (traceanalysis.Page[traceanalysis.FrameSummary], *consolecore.Error)
	QueryRecords(context.Context, evidence.Reference, traceanalysis.RecordQuery) (traceanalysis.Page[traceanalysis.RecordSummary], *consolecore.Error)
	QueryAttempts(context.Context, evidence.Reference, traceanalysis.AttemptQuery) (traceanalysis.Page[traceanalysis.AttemptSummary], *consolecore.Error)
	QueryRetries(context.Context, evidence.Reference, traceanalysis.RetryQuery) (traceanalysis.Page[traceanalysis.RetrySummary], *consolecore.Error)
	QueryValidationLinks(context.Context, evidence.Reference, traceanalysis.ValidationQuery) (traceanalysis.Page[traceanalysis.ValidationSummary], *consolecore.Error)
	QueryFailures(context.Context, evidence.Reference, traceanalysis.FailureQuery) (traceanalysis.Page[traceanalysis.FailureSummary], *consolecore.Error)
	GetFailureDiagnostic(context.Context, evidence.Reference, traceanalysis.FailureDiagnosticRequest) (traceanalysis.FailureDiagnostic, *consolecore.Error)
	QueryPayloads(context.Context, evidence.Reference, traceanalysis.PayloadQuery) (traceanalysis.Page[traceanalysis.PayloadDescriptor], *consolecore.Error)
	QueryGaps(context.Context, evidence.Reference, traceanalysis.GapQuery) (traceanalysis.Page[traceanalysis.Gap], *consolecore.Error)
	QueryUncertainties(context.Context, evidence.Reference, traceanalysis.UncertaintyQuery) (traceanalysis.Page[traceanalysis.Uncertainty], *consolecore.Error)
	GetUsageBreakdown(context.Context, evidence.Reference, artifact.Handle) (traceanalysis.UsageBreakdown, *consolecore.Error)
	Search(context.Context, evidence.Reference, traceanalysis.SearchQuery) (traceanalysis.Page[traceanalysis.SearchResult], *consolecore.Error)
	ReadPayloadRange(context.Context, evidence.Reference, traceanalysis.RangeRequest) (traceanalysis.ByteRangeResult, *consolecore.Error)
	ReadRawRecordRange(context.Context, evidence.Reference, traceanalysis.RangeRequest) (traceanalysis.ByteRangeResult, *consolecore.Error)
}

type Options struct {
	Policy                Policy
	Pairing               *browserauth.Pairing
	Sessions              *browserauth.Registry
	ProcessID             string
	Workspace             string
	PairingURL            func(string) string
	PrintPairing          func(string) error
	Target                *target.Context
	Observability         *observability.Service
	Live                  *live.Service
	Artifacts             ArtifactService
	TraceAnalysis         TraceAnalysisService
	TraceInventory        *traceinventory.Service
	TargetAddressDefault  string
	ApplicationKeyDefault string
	MCP                   MCPManager
	MCPEndpoint           string
}

type MCPManager interface {
	Status() mcpcredential.Snapshot
	Enable(context.Context) (string, error)
	Reveal() (string, error)
	Regenerate(context.Context) (string, error)
	Disable(context.Context) error
	RemoveInvalid(context.Context) error
}

type Router struct {
	options Options
}

func New(options Options) (*Router, error) {
	if options.Pairing == nil || options.Sessions == nil || options.PairingURL == nil {
		return nil, fmt.Errorf("browser API dependencies are incomplete")
	}
	return &Router{options: options}, nil
}

func (router *Router) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	ApplyHeaders(response.Header())
	response.Header().Set("Cache-Control", "no-store")
	if !router.options.Policy.ValidateHost(request) {
		writeError(response, http.StatusBadRequest, "INVALID_REQUEST", "Browser request rejected.")
		return
	}
	if strings.HasPrefix(request.URL.Path, "/api/console/v1/artifacts/") && strings.HasSuffix(request.URL.Path, "/raw") {
		if !router.options.Policy.ValidateDownloadRequest(request) {
			writeError(response, http.StatusForbidden, "BROWSER_SECURITY_REJECTED", "Browser request rejected.")
			return
		}
		router.withSessionDownload(response, request, router.artifactRawDownload)
		return
	}
	if !router.options.Policy.ValidateOrigin(request) {
		writeError(response, http.StatusForbidden, "BROWSER_SECURITY_REJECTED", "Browser request rejected.")
		return
	}
	if request.Method != http.MethodPost {
		response.Header().Set("Allow", "POST")
		writeError(response, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Use POST for this operation.")
		return
	}
	switch request.URL.Path {
	case "/api/console/v1/pairing/exchange":
		router.exchange(response, request)
	case "/api/console/v1/pairing/challenge":
		router.manualChallenge(response, request)
	case "/api/console/v1/bootstrap":
		router.withSession(response, request, false, router.bootstrap)
	case "/api/console/v1/pairing/link":
		router.withSession(response, request, true, router.pairingLink)
	case "/api/console/v1/tabs/release":
		router.withSession(response, request, true, router.releaseTab)
	case "/api/console/v1/tabs/heartbeat":
		router.withSession(response, request, true, router.heartbeat)
	case "/api/console/v1/target/status":
		router.withSession(response, request, false, router.targetStatus)
	case "/api/console/v1/target/connect":
		router.withSession(response, request, true, router.targetConnect)
	case "/api/console/v1/target/credential":
		router.withSession(response, request, true, router.targetCredential)
	case "/api/console/v1/target/recheck":
		router.withSession(response, request, true, router.targetRecheck)
	case "/api/console/v1/mcp/status":
		router.withSession(response, request, false, router.mcpStatus)
	case "/api/console/v1/mcp/enable":
		router.withSession(response, request, true, router.mcpEnable)
	case "/api/console/v1/mcp/reveal":
		router.withSession(response, request, true, router.mcpReveal)
	case "/api/console/v1/mcp/regenerate":
		router.withSession(response, request, true, router.mcpRegenerate)
	case "/api/console/v1/mcp/disable":
		router.withSession(response, request, true, router.mcpDisable)
	case "/api/console/v1/mcp/remove-invalid":
		router.withSession(response, request, true, router.mcpRemoveInvalid)
	case "/api/console/v1/observability/instance":
		router.withSession(response, request, false, router.observabilityInstance)
	case "/api/console/v1/skills/list":
		router.withSession(response, request, false, router.skillsList)
	case "/api/console/v1/skills/detail":
		router.withSession(response, request, false, router.skillDetail)
	case "/api/console/v1/active-executions/list":
		router.withSession(response, request, false, router.activeExecutionsList)
	case "/api/console/v1/active-executions/detail":
		router.withSession(response, request, false, router.activeExecutionDetail)
	case "/api/console/v1/traces/list":
		router.withSession(response, request, false, router.tracesList)
	case "/api/console/v1/traces/detail":
		router.withSession(response, request, false, router.traceDetail)
	case "/api/console/v1/traces/analysis/summary":
		router.withSession(response, request, false, router.traceAnalysisSummary)
	case "/api/console/v1/traces/analysis/frames":
		router.withSession(response, request, false, router.traceAnalysisFrames)
	case "/api/console/v1/traces/analysis/records":
		router.withSession(response, request, false, router.traceAnalysisRecords)
	case "/api/console/v1/traces/analysis/usage":
		router.withSession(response, request, false, router.traceAnalysisUsage)
	case "/api/console/v1/traces/analysis/attempts":
		router.withSession(response, request, false, router.traceAnalysisAttempts)
	case "/api/console/v1/traces/analysis/retries":
		router.withSession(response, request, false, router.traceAnalysisRetries)
	case "/api/console/v1/traces/analysis/validation-links":
		router.withSession(response, request, false, router.traceAnalysisValidationLinks)
	case "/api/console/v1/traces/analysis/failures":
		router.withSession(response, request, false, router.traceAnalysisFailures)
	case "/api/console/v1/traces/analysis/failure-diagnostic":
		router.withSession(response, request, false, router.traceAnalysisFailureDiagnostic)
	case "/api/console/v1/traces/analysis/payloads":
		router.withSession(response, request, false, router.traceAnalysisPayloads)
	case "/api/console/v1/traces/analysis/gaps":
		router.withSession(response, request, false, router.traceAnalysisGaps)
	case "/api/console/v1/traces/analysis/uncertainties":
		router.withSession(response, request, false, router.traceAnalysisUncertainties)
	case "/api/console/v1/traces/analysis/search":
		router.withSession(response, request, false, router.traceAnalysisSearch)
	case "/api/console/v1/traces/analysis/payload-range":
		router.withSession(response, request, false, router.traceAnalysisPayloadRange)
	case "/api/console/v1/traces/analysis/raw-record-range":
		router.withSession(response, request, false, router.traceAnalysisRawRecordRange)
	case "/api/console/v1/artifacts/acquire":
		router.withSession(response, request, true, router.artifactAcquire)
	case "/api/console/v1/artifacts/import":
		router.withSession(response, request, true, router.artifactImport)
	case "/api/console/v1/artifacts/storage":
		router.withSession(response, request, false, router.artifactStorage)
	case "/api/console/v1/artifacts/remove":
		router.withSession(response, request, true, router.artifactRemove)
	case "/api/console/v1/artifacts/clear-expired":
		router.withSession(response, request, true, router.artifactClearExpired)
	case "/api/console/v1/artifacts/clear-all-unused":
		router.withSession(response, request, true, router.artifactClearAllUnused)
	case "/api/console/v1/activity/stream":
		router.withSessionSSE(response, request, router.activityStream)
	case "/api/console/v1/activity/recent":
		router.withSession(response, request, false, router.activityRecent)
	default:
		writeError(response, http.StatusNotFound, "NOT_FOUND", "Console operation not found.")
	}
}

func (router *Router) exchange(response http.ResponseWriter, request *http.Request) {
	var body struct {
		Secret string `json:"secret"`
	}
	if err := decodeJSON(request, &body); err != nil {
		writeError(response, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request.")
		return
	}
	if !router.options.Pairing.Consume(body.Secret) {
		writeError(response, http.StatusUnauthorized, "PAIRING_REJECTED", "Pairing link is invalid or expired.")
		return
	}
	sessionID, err := router.options.Sessions.CreateSession()
	if err != nil {
		writeError(response, http.StatusTooManyRequests, "LIMIT_EXCEEDED", "Browser session limit reached.")
		return
	}
	http.SetCookie(response, browserauth.SessionCookie(sessionID))
	writeJSON(response, http.StatusOK, map[string]bool{"paired": true})
}

func (router *Router) manualChallenge(response http.ResponseWriter, request *http.Request) {
	if err := decodeJSON(request, &struct{}{}); err != nil {
		writeError(response, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request.")
		return
	}
	secret, err := router.options.Pairing.Create(true)
	if err != nil {
		writeError(response, http.StatusTooManyRequests, "RATE_LIMITED", "A pairing challenge is already available. Try again shortly.")
		return
	}
	if router.options.PrintPairing != nil {
		if err := router.options.PrintPairing(router.options.PairingURL(secret)); err != nil {
			writeError(response, http.StatusInternalServerError, "PAIRING_UNAVAILABLE", "Pairing challenge could not be displayed.")
			return
		}
	}
	writeJSON(response, http.StatusAccepted, map[string]bool{"challengePrinted": true})
}

type authenticatedHandler func(http.ResponseWriter, *http.Request, string)

func (router *Router) withSession(response http.ResponseWriter, request *http.Request, csrf bool, handler authenticatedHandler) {
	cookie, err := request.Cookie(browserauth.SessionCookieName)
	if err != nil || !router.options.Sessions.Authenticate(cookie.Value) {
		http.SetCookie(response, browserauth.ExpiredSessionCookie())
		writeError(response, http.StatusUnauthorized, "SESSION_REQUIRED", "Pairing is required.")
		return
	}
	if csrf {
		tabIDs := request.Header.Values("X-loomspan-Console-Tab")
		tokens := request.Header.Values(csrfHeader)
		if len(tabIDs) != 1 || len(tokens) != 1 || strings.Contains(tokens[0], ",") ||
			!router.options.Sessions.ValidateCSRF(cookie.Value, tabIDs[0], tokens[0]) {
			writeError(response, http.StatusForbidden, "BROWSER_SECURITY_REJECTED", "Browser request rejected.")
			return
		}
	}
	handler(response, request, cookie.Value)
}

func (router *Router) bootstrap(response http.ResponseWriter, request *http.Request, sessionID string) {
	var body struct {
		TabID string `json:"tabId,omitempty"`
	}
	if err := decodeJSON(request, &body); err != nil {
		writeError(response, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request.")
		return
	}
	result, err := router.options.Sessions.Bootstrap(sessionID, body.TabID)
	if err != nil {
		writeError(response, http.StatusTooManyRequests, "LIMIT_EXCEEDED", "Browser tab limit reached.")
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"processId":      router.options.ProcessID,
		"consoleVersion": release.ProductVersion(),
		"workspacePath":  router.options.Workspace,
		"tabId":          result.TabID,
		"csrfToken":      result.CSRF,
		"target":         targetResponse(router.targetSnapshot()),
		"targetFormDefaults": map[string]string{
			"address":        router.options.TargetAddressDefault,
			"applicationKey": router.options.ApplicationKeyDefault,
		},
	})
}

func (router *Router) targetSnapshot() target.Snapshot {
	if router.options.Target == nil {
		return target.Snapshot{}
	}
	return router.options.Target.Snapshot()
}

func (router *Router) pairingLink(response http.ResponseWriter, request *http.Request, _ string) {
	if err := decodeJSON(request, &struct{}{}); err != nil {
		writeError(response, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request.")
		return
	}
	secret, err := router.options.Pairing.Create(false)
	if err != nil {
		writeError(response, http.StatusInternalServerError, "PAIRING_UNAVAILABLE", "Pairing link could not be created.")
		return
	}
	writeJSON(response, http.StatusOK, map[string]string{"pairingUrl": router.options.PairingURL(secret)})
}

func (router *Router) releaseTab(response http.ResponseWriter, request *http.Request, sessionID string) {
	if err := decodeJSON(request, &struct{}{}); err != nil {
		writeError(response, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request.")
		return
	}
	tabIDs := request.Header.Values("X-loomspan-Console-Tab")
	if len(tabIDs) == 1 {
		router.options.Sessions.ReleaseTab(sessionID, tabIDs[0])
	}
	writeJSON(response, http.StatusOK, map[string]bool{"released": true})
}

func (router *Router) heartbeat(response http.ResponseWriter, request *http.Request, _ string) {
	var body struct{}
	if err := decodeJSON(request, &body); err != nil {
		writeError(response, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request.")
		return
	}
	writeJSON(response, http.StatusOK, map[string]bool{"active": true})
}
