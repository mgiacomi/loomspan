package browserapi

import (
	"errors"
	"net/http"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/evidence"
)

func (router *Router) artifactImport(response http.ResponseWriter, request *http.Request, _ string) {
	if router.options.Artifacts == nil {
		writeError(response, http.StatusInternalServerError, "CONSOLE_ERROR", "Artifact service is unavailable.")
		return
	}
	if request.URL.RawQuery != "" || len(request.Header.Values("Content-Type")) != 1 ||
		request.Header.Get("Content-Type") != "application/x-ndjson" {
		writeError(response, http.StatusBadRequest, "INVALID_REQUEST", "A raw NDJSON trace file is required.")
		return
	}
	limit := router.options.Artifacts.ImportLimit()
	if request.ContentLength > limit {
		writeError(response, http.StatusRequestEntityTooLarge, "LIMIT_EXCEEDED", "The trace file exceeds the import limit.")
		return
	}
	request.Body = http.MaxBytesReader(response, request.Body, limit)
	acquired, domain := router.options.Artifacts.Import(request.Context(), request.Body, request.ContentLength)
	if domain != nil {
		if domain.Code == consolecore.CodeLimitExceeded {
			writeError(response, http.StatusRequestEntityTooLarge, "LIMIT_EXCEEDED", domain.Message)
			return
		}
		var maxBytes *http.MaxBytesError
		if errors.As(domain, &maxBytes) {
			writeError(response, http.StatusRequestEntityTooLarge, "LIMIT_EXCEEDED", "The trace file exceeds the import limit.")
			return
		}
		writeEvidenceDomainError(response, evidence.ForImported(), domain)
		return
	}
	dto := acquiredArtifactDTO{
		Source: evidence.SourceImported, Handle: string(acquired.Handle),
		TraceID: acquired.Metadata.TraceID, SessionID: acquired.Metadata.SessionID,
		Outcome: acquired.Metadata.Outcome, FinalizedAt: acquired.Metadata.FinalizedAt,
		LocalBytes: acquired.LocalBytes, AcquiredAt: acquired.AcquiredAt,
		LastUsedAt: acquired.LastUsedAt, ExpiresAt: acquired.ExpiresAt,
		HasIdleExpiry: acquired.HasIdleExpiry,
	}
	writeJSON(response, http.StatusOK, dto)
}
