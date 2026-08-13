package browserapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/target"
)

type targetDTO struct {
	Address     string                     `json:"address,omitempty"`
	Unencrypted bool                       `json:"unencrypted"`
	Status      consolecore.StatusSnapshot `json:"status"`
}

func targetResponse(snapshot target.Snapshot) targetDTO {
	if snapshot.Status.ObservedAt.IsZero() {
		snapshot.Status = consolecore.NoTargetStatus(timeNow())
	}
	return targetDTO{Address: snapshot.Address, Unencrypted: snapshot.Unencrypted, Status: snapshot.Status}
}

var timeNow = func() time.Time { return time.Now().UTC() }

func (router *Router) targetStatus(response http.ResponseWriter, request *http.Request, _ string) {
	if err := decodeJSONLimit(request, &struct{}{}, maxTargetJSONBody); err != nil {
		writeError(response, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request.")
		return
	}
	writeJSON(response, http.StatusOK, targetResponse(router.targetSnapshot()))
}

func (router *Router) targetConnect(response http.ResponseWriter, request *http.Request, _ string) {
	if router.options.Target == nil {
		writeError(response, http.StatusInternalServerError, "CONSOLE_ERROR", "Target service is unavailable.")
		return
	}
	var body struct {
		TargetAddress  string `json:"targetAddress"`
		ApplicationKey string `json:"applicationKey"`
	}
	if err := decodeJSONLimit(request, &body, maxTargetJSONBody); err != nil {
		writeError(response, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request.")
		return
	}
	key := []byte(body.ApplicationKey)
	body.ApplicationKey = ""
	snapshot, domain := router.options.Target.SelectAndConnect(request.Context(), body.TargetAddress, key)
	clear(key)
	if domain != nil {
		writeDomainError(response, domain)
		return
	}
	writeJSON(response, http.StatusOK, targetResponse(snapshot))
}

func (router *Router) targetCredential(response http.ResponseWriter, request *http.Request, _ string) {
	if router.options.Target == nil {
		writeError(response, http.StatusInternalServerError, "CONSOLE_ERROR", "Target service is unavailable.")
		return
	}
	var body struct {
		ApplicationKey string `json:"applicationKey"`
	}
	if err := decodeJSONLimit(request, &body, maxTargetJSONBody); err != nil {
		writeError(response, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request.")
		return
	}
	key := []byte(body.ApplicationKey)
	body.ApplicationKey = ""
	snapshot, domain := router.options.Target.SupplyCredential(request.Context(), key)
	clear(key)
	if domain != nil {
		writeDomainError(response, domain)
		return
	}
	writeJSON(response, http.StatusOK, targetResponse(snapshot))
}

func (router *Router) targetRecheck(response http.ResponseWriter, request *http.Request, _ string) {
	if router.options.Target == nil {
		writeError(response, http.StatusInternalServerError, "CONSOLE_ERROR", "Target service is unavailable.")
		return
	}
	if err := decodeJSONLimit(request, &struct{}{}, maxTargetJSONBody); err != nil {
		writeError(response, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request.")
		return
	}
	snapshot, domain := router.options.Target.Recheck(request.Context())
	if domain != nil {
		writeDomainError(response, domain)
		return
	}
	writeJSON(response, http.StatusOK, targetResponse(snapshot))
}

func writeDomainError(response http.ResponseWriter, domain *consolecore.Error) {
	var details any
	if domain.Details != (consolecore.Details{}) {
		details = domain.Details
	}
	status := http.StatusInternalServerError
	switch domain.Code {
	case consolecore.CodeInvalidArgument, consolecore.CodeInvalidCursor:
		status = http.StatusBadRequest
	case consolecore.CodeTargetAuthentication:
		status = http.StatusUnauthorized
	case consolecore.CodeTargetAccessBlocked:
		status = http.StatusForbidden
	case consolecore.CodeNotFound:
		status = http.StatusNotFound
	case consolecore.CodeIncompatibleTarget, consolecore.CodeIncompatibleArtifact,
		consolecore.CodeArtifactAlreadyExists, consolecore.CodeTargetChanged, consolecore.CodeStaleCursor,
		consolecore.CodeArtifactExpired, consolecore.CodeArtifactInUse, consolecore.CodeLiveMonitoringUnavailable:
		status = http.StatusConflict
	case consolecore.CodeInvalidArtifact:
		status = http.StatusUnprocessableEntity
	case consolecore.CodeLimitExceeded:
		status = http.StatusTooManyRequests
	case consolecore.CodeTargetUnavailable, consolecore.CodeLocalStorageUnavailable:
		status = http.StatusServiceUnavailable
	}
	ApplyHeaders(response.Header())
	response.Header().Set("Cache-Control", "no-store")
	response.Header().Set("Content-Type", "application/json; charset=utf-8")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(errorEnvelope{Error: browserError{
		Code: string(domain.Code), Message: domain.Message, TargetScopeID: domain.TargetScopeID, Details: details,
	}})
}
