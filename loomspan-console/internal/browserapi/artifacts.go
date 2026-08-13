package browserapi

import (
	"net/http"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/browserauth"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/evidence"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/observability"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/target"
)

const maxArtifactJSONBody = 4 * 1024

type acquiredArtifactDTO struct {
	Source        evidence.Source `json:"source"`
	TargetScopeID string          `json:"targetScopeId,omitempty"`
	Handle        string          `json:"artifactHandle"`
	TraceID       string          `json:"traceId"`
	SessionID     string          `json:"sessionId"`
	Outcome       string          `json:"outcome"`
	FinalizedAt   time.Time       `json:"finalizedAt"`
	LocalBytes    int64           `json:"localBytes"`
	AcquiredAt    time.Time       `json:"acquiredAt"`
	LastUsedAt    time.Time       `json:"lastUsedAt"`
	ExpiresAt     time.Time       `json:"expiresAt"`
	HasIdleExpiry bool            `json:"hasIdleExpiry"`
}

type storageSnapshotDTO struct {
	WorkspaceLabel string                 `json:"workspaceLabel"`
	MaxBytes       int64                  `json:"maxBytes"`
	Unlimited      bool                   `json:"unlimited"`
	IdleTTL        string                 `json:"idleTtl"`
	NeverExpire    bool                   `json:"neverExpire"`
	ChargedBytes   int64                  `json:"chargedBytes"`
	AcquiredCount  int                    `json:"acquiredCount"`
	Entries        []artifact.StoredEntry `json:"entries"`
}

func (router *Router) artifactAcquire(response http.ResponseWriter, request *http.Request, _ string) {
	if router.options.Artifacts == nil || router.options.Target == nil {
		writeError(response, http.StatusInternalServerError, "CONSOLE_ERROR", "Artifact service is unavailable.")
		return
	}
	var body struct {
		TraceID string `json:"traceId"`
	}
	if err := decodeJSONLimit(request, &body, maxArtifactJSONBody); err != nil {
		writeError(response, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request.")
		return
	}
	if body.TraceID == "" {
		writeError(response, http.StatusBadRequest, "INVALID_REQUEST", "A trace ID is required.")
		return
	}
	scope, domain := router.options.Target.Capture()
	if domain != nil {
		writeDomainError(response, domain)
		return
	}
	acquired, domain := router.options.Artifacts.Acquire(request.Context(), scope, body.TraceID)
	if domain != nil {
		writeDomainError(response, domain)
		return
	}
	dto := acquiredArtifactDTO{
		Source:        evidence.SourceTarget,
		TargetScopeID: string(scope.ID),
		Handle:        string(acquired.Handle),
		TraceID:       acquired.Metadata.TraceID,
		SessionID:     acquired.Metadata.SessionID,
		Outcome:       acquired.Metadata.Outcome,
		FinalizedAt:   acquired.Metadata.FinalizedAt,
		LocalBytes:    acquired.LocalBytes,
		AcquiredAt:    acquired.AcquiredAt,
		LastUsedAt:    acquired.LastUsedAt,
		ExpiresAt:     acquired.ExpiresAt,
		HasIdleExpiry: acquired.HasIdleExpiry,
	}
	router.writeScopedJSON(response, scope.ID, dto)
}

func (router *Router) artifactStorage(response http.ResponseWriter, request *http.Request, _ string) {
	if router.options.Artifacts == nil {
		writeError(response, http.StatusInternalServerError, "CONSOLE_ERROR", "Artifact service is unavailable.")
		return
	}
	if err := decodeJSONLimit(request, &struct{}{}, maxArtifactJSONBody); err != nil {
		writeError(response, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request.")
		return
	}
	snapshot, domain := router.options.Artifacts.StorageSnapshot()
	if domain != nil {
		writeDomainError(response, domain)
		return
	}
	dto := storageSnapshotDTO{
		WorkspaceLabel: snapshot.WorkspaceLabel,
		MaxBytes:       snapshot.MaxBytes,
		Unlimited:      snapshot.Unlimited,
		IdleTTL:        formatDuration(snapshot.IdleTTL, snapshot.NeverExpire),
		NeverExpire:    snapshot.NeverExpire,
		ChargedBytes:   snapshot.ChargedBytes,
		AcquiredCount:  snapshot.AcquiredCount,
		Entries:        snapshot.Entries,
	}
	writeJSON(response, http.StatusOK, dto)
}

func (router *Router) artifactRemove(response http.ResponseWriter, request *http.Request, _ string) {
	if router.options.Artifacts == nil {
		writeError(response, http.StatusInternalServerError, "CONSOLE_ERROR", "Artifact service is unavailable.")
		return
	}
	var body struct {
		TraceID string `json:"traceId"`
		Source  string `json:"source"`
	}
	if err := decodeJSONLimit(request, &body, maxArtifactJSONBody); err != nil {
		writeError(response, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request.")
		return
	}
	if body.TraceID == "" {
		writeError(response, http.StatusBadRequest, "INVALID_REQUEST", "A trace ID is required.")
		return
	}
	ref, domain := router.resolveEvidenceReference(body.Source)
	if domain != nil {
		writeDomainError(response, domain)
		return
	}
	if domain := router.options.Artifacts.Remove(ref, body.TraceID); domain != nil {
		writeEvidenceDomainError(response, ref, domain)
		return
	}
	router.writeEvidenceJSON(response, ref, map[string]bool{"removed": true})
}

func (router *Router) artifactClearExpired(response http.ResponseWriter, request *http.Request, _ string) {
	if router.options.Artifacts == nil {
		writeError(response, http.StatusInternalServerError, "CONSOLE_ERROR", "Artifact service is unavailable.")
		return
	}
	if err := decodeJSONLimit(request, &struct{}{}, maxArtifactJSONBody); err != nil {
		writeError(response, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request.")
		return
	}
	if domain := router.options.Artifacts.ClearExpired(); domain != nil {
		writeUnscopedDomainError(response, domain)
		return
	}
	writeJSON(response, http.StatusOK, map[string]bool{"cleared": true})
}

func (router *Router) artifactClearAllUnused(response http.ResponseWriter, request *http.Request, _ string) {
	if router.options.Artifacts == nil {
		writeError(response, http.StatusInternalServerError, "CONSOLE_ERROR", "Artifact service is unavailable.")
		return
	}
	if err := decodeJSONLimit(request, &struct{}{}, maxArtifactJSONBody); err != nil {
		writeError(response, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request.")
		return
	}
	if domain := router.options.Artifacts.ClearAllUnused(); domain != nil {
		writeUnscopedDomainError(response, domain)
		return
	}
	writeJSON(response, http.StatusOK, map[string]bool{"cleared": true})
}

// enrichTracePage enriches each trace in a page with local artifact
// availability and opaque handle from the artifact service.
func (router *Router) enrichTracePage(scope target.ScopeID, page observability.Page[observability.Trace]) observability.Page[observability.Trace] {
	if router.options.Artifacts == nil {
		return page
	}
	for i := range page.Items {
		lookup, domain := router.options.Artifacts.Lookup(evidence.ForTarget(scope), page.Items[i].TraceID)
		if domain != nil {
			continue
		}
		if lookup.LocalAvailable {
			page.Items[i].LocalAvailable = true
			page.Items[i].ArtifactHandle = string(lookup.Handle)
			page.Items[i].ApplicationAvailability = string(lookup.ApplicationAvailability)
		}
	}
	return page
}

// enrichTrace enriches a single trace with local artifact availability.
func (router *Router) enrichTrace(scope target.ScopeID, trace observability.Trace) observability.Trace {
	if router.options.Artifacts == nil {
		return trace
	}
	lookup, domain := router.options.Artifacts.Lookup(evidence.ForTarget(scope), trace.TraceID)
	if domain != nil {
		return trace
	}
	if lookup.LocalAvailable {
		trace.LocalAvailable = true
		trace.ArtifactHandle = string(lookup.Handle)
		trace.ApplicationAvailability = string(lookup.ApplicationAvailability)
	}
	return trace
}

func (router *Router) resolveEvidenceReference(source string) (evidence.Reference, *consolecore.Error) {
	if source == string(evidence.SourceImported) {
		return evidence.ForImported(), nil
	}
	if source != string(evidence.SourceTarget) || router.options.Target == nil {
		return evidence.Reference{}, consolecore.NewError(consolecore.CodeInvalidArgument,
			"The evidence source is invalid.", "", consolecore.Details{}, nil)
	}
	scope, domain := router.options.Target.Capture()
	if domain != nil {
		return evidence.Reference{}, domain
	}
	return evidence.ForTarget(scope.ID), nil
}

func (router *Router) writeEvidenceJSON(response http.ResponseWriter, ref evidence.Reference, value any) {
	if ref.Source == evidence.SourceTarget {
		router.writeScopedJSON(response, ref.TargetScope, value)
		return
	}
	writeJSON(response, http.StatusOK, value)
}

// writeEvidenceDomainError preserves target-scope continuity for target
// evidence while ensuring the process-local imported owner ID is never exposed
// through the browser's targetScopeId field.
func writeEvidenceDomainError(response http.ResponseWriter, ref evidence.Reference, domain *consolecore.Error) {
	if ref.Source != evidence.SourceImported {
		writeDomainError(response, domain)
		return
	}
	writeUnscopedDomainError(response, domain)
}

// writeUnscopedDomainError prevents aggregate operations from attributing an
// error to an internal artifact owner through the browser's targetScopeId field.
func writeUnscopedDomainError(response http.ResponseWriter, domain *consolecore.Error) {
	sanitized := *domain
	sanitized.TargetScopeID = ""
	writeDomainError(response, &sanitized)
}

// formatDuration formats an idle TTL duration for JSON display. "Never" is
// returned when the service is configured to never expire.
func formatDuration(ttl time.Duration, neverExpire bool) string {
	if neverExpire {
		return "Never"
	}
	return ttl.String()
}

// withSessionDownload authenticates a GET download request using the paired
// session cookie. It does not require CSRF (GET navigation/download) but
// rejects query parameters, ranges, and conditional requests.
func (router *Router) withSessionDownload(response http.ResponseWriter, request *http.Request, handler downloadHandler) {
	if request.Method != http.MethodGet {
		response.Header().Set("Allow", "GET")
		writeError(response, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Use GET for this download.")
		return
	}
	cookie, err := request.Cookie(browserauth.SessionCookieName)
	if err != nil || !router.options.Sessions.Authenticate(cookie.Value) {
		http.SetCookie(response, browserauth.ExpiredSessionCookie())
		writeError(response, http.StatusUnauthorized, "SESSION_REQUIRED", "Pairing is required.")
		return
	}
	handler(response, request, cookie.Value)
}

type downloadHandler func(http.ResponseWriter, *http.Request, string)
