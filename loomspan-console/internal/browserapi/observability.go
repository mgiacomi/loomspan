package browserapi

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/observability"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/target"
)

const maxObservabilityJSONBody = 4 * 1024

func (router *Router) observabilityInstance(response http.ResponseWriter, request *http.Request, _ string) {
	if router.options.Observability == nil || router.options.Target == nil {
		writeError(response, http.StatusInternalServerError, "CONSOLE_ERROR", "Observability service is unavailable.")
		return
	}
	if err := decodeJSONLimit(request, &struct{}{}, maxObservabilityJSONBody); err != nil {
		writeError(response, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request.")
		return
	}
	scope, domain := router.options.Target.Capture()
	if domain != nil {
		writeDomainError(response, domain)
		return
	}
	status, domain := router.options.Observability.GetInstance(request.Context(), scope)
	if domain != nil {
		writeDomainError(response, domain)
		return
	}
	router.writeScopedJSON(response, scope.ID, status)
}

func (router *Router) skillsList(response http.ResponseWriter, request *http.Request, _ string) {
	if router.options.Observability == nil || router.options.Target == nil {
		writeError(response, http.StatusInternalServerError, "CONSOLE_ERROR", "Observability service is unavailable.")
		return
	}
	var body struct {
		Cursor   string `json:"cursor,omitempty"`
		PageSize int    `json:"pageSize,omitempty"`
	}
	if err := decodeJSONLimit(request, &body, maxObservabilityJSONBody); err != nil {
		writeError(response, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request.")
		return
	}
	scope, domain := router.options.Target.Capture()
	if domain != nil {
		writeDomainError(response, domain)
		return
	}
	page, domain := router.options.Observability.ListSkills(request.Context(), scope, observability.ListRequest{
		Cursor:   body.Cursor,
		PageSize: body.PageSize,
	})
	if domain != nil {
		writeDomainError(response, domain)
		return
	}
	router.writeScopedJSON(response, scope.ID, page)
}

func (router *Router) skillDetail(response http.ResponseWriter, request *http.Request, _ string) {
	if router.options.Observability == nil || router.options.Target == nil {
		writeError(response, http.StatusInternalServerError, "CONSOLE_ERROR", "Observability service is unavailable.")
		return
	}
	var body struct {
		RegisteredName string `json:"registeredName"`
	}
	if err := decodeJSONLimit(request, &body, maxObservabilityJSONBody); err != nil {
		writeError(response, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request.")
		return
	}
	scope, domain := router.options.Target.Capture()
	if domain != nil {
		writeDomainError(response, domain)
		return
	}
	detail, domain := router.options.Observability.GetSkill(request.Context(), scope, body.RegisteredName)
	if domain != nil {
		writeDomainError(response, domain)
		return
	}
	router.writeScopedJSON(response, scope.ID, detail)
}

func (router *Router) activeExecutionsList(response http.ResponseWriter, request *http.Request, _ string) {
	if router.options.Observability == nil || router.options.Target == nil {
		writeError(response, http.StatusInternalServerError, "CONSOLE_ERROR", "Observability service is unavailable.")
		return
	}
	var body struct {
		Cursor   string `json:"cursor,omitempty"`
		PageSize int    `json:"pageSize,omitempty"`
	}
	if err := decodeJSONLimit(request, &body, maxObservabilityJSONBody); err != nil {
		writeError(response, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request.")
		return
	}
	scope, domain := router.options.Target.Capture()
	if domain != nil {
		writeDomainError(response, domain)
		return
	}
	page, domain := router.options.Observability.ListActiveExecutions(request.Context(), scope, observability.ListRequest{
		Cursor:   body.Cursor,
		PageSize: body.PageSize,
	})
	if domain != nil {
		writeDomainError(response, domain)
		return
	}
	router.writeScopedJSON(response, scope.ID, page)
}

func (router *Router) activeExecutionDetail(response http.ResponseWriter, request *http.Request, _ string) {
	if router.options.Observability == nil || router.options.Target == nil {
		writeError(response, http.StatusInternalServerError, "CONSOLE_ERROR", "Observability service is unavailable.")
		return
	}
	var body struct {
		SessionID string `json:"sessionId"`
	}
	if err := decodeJSONLimit(request, &body, maxObservabilityJSONBody); err != nil {
		writeError(response, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request.")
		return
	}
	scope, domain := router.options.Target.Capture()
	if domain != nil {
		writeDomainError(response, domain)
		return
	}
	execution, domain := router.options.Observability.GetActiveExecution(request.Context(), scope, body.SessionID)
	if domain != nil {
		writeDomainError(response, domain)
		return
	}
	router.writeScopedJSON(response, scope.ID, execution)
}

func (router *Router) tracesList(response http.ResponseWriter, request *http.Request, _ string) {
	if router.options.Observability == nil || router.options.Target == nil {
		writeError(response, http.StatusInternalServerError, "CONSOLE_ERROR", "Observability service is unavailable.")
		return
	}
	var body struct {
		Cursor   string `json:"cursor,omitempty"`
		PageSize int    `json:"pageSize,omitempty"`
	}
	if err := decodeJSONLimit(request, &body, maxObservabilityJSONBody); err != nil {
		writeError(response, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request.")
		return
	}
	scope, domain := router.options.Target.Capture()
	if domain != nil {
		writeDomainError(response, domain)
		return
	}
	page, domain := router.options.Observability.ListTraces(request.Context(), scope, observability.ListRequest{
		Cursor:   body.Cursor,
		PageSize: body.PageSize,
	})
	if domain != nil {
		if allowsCachedTraceFallback(domain) {
			if cached, ok := router.cachedTracePage(scope.ID); ok {
				router.writeScopedJSON(response, scope.ID, cached)
				return
			}
		}
		writeDomainError(response, domain)
		return
	}
	page = router.enrichTracePage(scope.ID, page)
	router.writeScopedJSON(response, scope.ID, page)
}

func (router *Router) traceDetail(response http.ResponseWriter, request *http.Request, _ string) {
	if router.options.Observability == nil || router.options.Target == nil {
		writeError(response, http.StatusInternalServerError, "CONSOLE_ERROR", "Observability service is unavailable.")
		return
	}
	var body struct {
		TraceID string `json:"traceId"`
	}
	if err := decodeJSONLimit(request, &body, maxObservabilityJSONBody); err != nil {
		writeError(response, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request.")
		return
	}
	scope, domain := router.options.Target.Capture()
	if domain != nil {
		writeDomainError(response, domain)
		return
	}
	trace, domain := router.options.Observability.GetTrace(request.Context(), scope, body.TraceID)
	if domain != nil {
		if allowsCachedTraceFallback(domain) {
			if cached, ok := router.cachedTrace(scope.ID, body.TraceID); ok {
				router.writeScopedJSON(response, scope.ID, cached)
				return
			}
		}
		writeDomainError(response, domain)
		return
	}
	trace = router.enrichTrace(scope.ID, trace)
	router.writeScopedJSON(response, scope.ID, trace)
}

func allowsCachedTraceFallback(domain *consolecore.Error) bool {
	if domain == nil {
		return false
	}
	switch domain.Code {
	case consolecore.CodeTargetAuthentication,
		consolecore.CodeTargetAccessBlocked,
		consolecore.CodeTargetUnavailable,
		consolecore.CodeNotFound,
		consolecore.CodeConsoleError:
		return true
	default:
		return false
	}
}

// cachedTrace returns acquisition-time trace facts for a valid installed
// artifact without claiming that the application is currently reachable or
// authorized.
func (router *Router) cachedTrace(scope target.ScopeID, traceID string) (observability.Trace, bool) {
	if router.options.Artifacts == nil {
		return observability.Trace{}, false
	}
	lookup, domain := router.options.Artifacts.Lookup(scope, traceID)
	if domain != nil || !lookup.LocalAvailable {
		return observability.Trace{}, false
	}
	return observability.Trace{
		TargetScopeID:             string(scope),
		TraceID:                   lookup.Metadata.TraceID,
		SessionID:                 lookup.Metadata.SessionID,
		EntrySkill:                lookup.Metadata.EntrySkill,
		Outcome:                   lookup.Metadata.Outcome,
		FinalizedAt:               lookup.Metadata.FinalizedAt,
		SizeBytes:                 lookup.Metadata.SizeBytes,
		PersistencePolicy:         lookup.Metadata.PersistencePolicy,
		ApplicationTraceExpiresAt: lookup.Metadata.ApplicationTraceExpiresAt,
		LocalAvailable:            true,
		ArtifactHandle:            string(lookup.Handle),
		ApplicationAvailability:   string(lookup.ApplicationAvailability),
	}, true
}

func (router *Router) cachedTracePage(scope target.ScopeID) (observability.Page[observability.Trace], bool) {
	if router.options.Artifacts == nil {
		return observability.Page[observability.Trace]{}, false
	}
	snapshot, domain := router.options.Artifacts.StorageSnapshot(scope)
	if domain != nil || len(snapshot.Entries) == 0 {
		return observability.Page[observability.Trace]{}, false
	}
	items := make([]observability.Trace, 0, len(snapshot.Entries))
	observedAt := snapshot.Entries[0].AcquiredAt
	for _, entry := range snapshot.Entries {
		if entry.AcquiredAt.After(observedAt) {
			observedAt = entry.AcquiredAt
		}
		if trace, ok := router.cachedTrace(scope, entry.TraceID); ok {
			items = append(items, trace)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].TraceID < items[j].TraceID })
	if len(items) == 0 {
		return observability.Page[observability.Trace]{}, false
	}
	return observability.Page[observability.Trace]{
		TargetScopeID: string(scope),
		Items:         items,
		ObservedAt:    observedAt,
	}, true
}

func (router *Router) writeScopedJSON(response http.ResponseWriter, scope target.ScopeID, value any) {
	content, err := json.Marshal(value)
	if err != nil {
		writeError(response, http.StatusInternalServerError, "CONSOLE_ERROR", "The Console response could not be created.")
		return
	}
	content = append(content, '\n')
	if domain := router.options.Target.PublishCurrent(scope, func() {
		writeJSONBytes(response, http.StatusOK, content)
	}); domain != nil {
		writeDomainError(response, domain)
	}
}
