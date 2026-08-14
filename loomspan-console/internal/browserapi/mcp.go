package browserapi

import (
	"net/http"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/mcpcredential"
)

const maxMCPJSONBody = 1024

type mcpStatusDTO struct {
	Endpoint   string              `json:"endpoint"`
	State      mcpcredential.State `json:"state"`
	Diagnostic string              `json:"diagnostic,omitempty"`
	Setup      []mcpSetupDTO       `json:"setup"`
}

type mcpSetupDTO struct {
	Client   string `json:"client"`
	Scope    string `json:"scope"`
	Guidance string `json:"guidance"`
}

type mcpCredentialDTO struct {
	mcpStatusDTO
	Credential string `json:"credential"`
}

func (router *Router) mcpStatusValue() mcpStatusDTO {
	snapshot := mcpcredential.Snapshot{State: mcpcredential.Disabled}
	if router.options.MCP != nil {
		snapshot = router.options.MCP.Status()
	}
	return mcpStatusDTO{Endpoint: router.options.MCPEndpoint, State: snapshot.State, Diagnostic: snapshot.Diagnostic, Setup: []mcpSetupDTO{
		{Client: "Codex", Scope: "user", Guidance: "Configure the endpoint globally and provide Authorization as an environment-backed Bearer header."},
		{Client: "Claude Code", Scope: "user", Guidance: "Configure the endpoint for the user and paste the key only into the client's protected bearer-header setting."},
		{Client: "Antigravity", Scope: "user", Guidance: "Use the user-level MCP settings with the endpoint and a Bearer header placeholder."},
		{Client: "Cursor", Scope: "user", Guidance: "Use global MCP settings with the endpoint and a Bearer header placeholder."},
		{Client: "Windsurf/Cascade", Scope: "global", Guidance: "Use global MCP settings with the endpoint and a Bearer header placeholder."},
	}}
}

func (router *Router) mcpStatus(response http.ResponseWriter, request *http.Request, _ string) {
	if !router.decodeMCPEmpty(response, request) {
		return
	}
	writeJSON(response, http.StatusOK, router.mcpStatusValue())
}

func (router *Router) mcpEnable(response http.ResponseWriter, request *http.Request, _ string) {
	if !router.decodeMCPEmpty(response, request) {
		return
	}
	if router.options.MCP == nil {
		router.mcpUnavailable(response)
		return
	}
	credential, err := router.options.MCP.Enable(request.Context())
	if err != nil {
		router.mcpOperationError(response)
		return
	}
	writeJSON(response, http.StatusOK, mcpCredentialDTO{mcpStatusDTO: router.mcpStatusValue(), Credential: credential})
}

func (router *Router) mcpReveal(response http.ResponseWriter, request *http.Request, _ string) {
	if !router.decodeMCPEmpty(response, request) {
		return
	}
	if router.options.MCP == nil {
		router.mcpUnavailable(response)
		return
	}
	credential, err := router.options.MCP.Reveal()
	if err != nil {
		router.mcpOperationError(response)
		return
	}
	writeJSON(response, http.StatusOK, mcpCredentialDTO{mcpStatusDTO: router.mcpStatusValue(), Credential: credential})
}

func (router *Router) mcpRegenerate(response http.ResponseWriter, request *http.Request, _ string) {
	if !router.decodeMCPConfirmation(response, request, "REGENERATE") {
		return
	}
	credential, err := router.options.MCP.Regenerate(request.Context())
	if err != nil {
		router.mcpOperationError(response)
		return
	}
	writeJSON(response, http.StatusOK, mcpCredentialDTO{mcpStatusDTO: router.mcpStatusValue(), Credential: credential})
}

func (router *Router) mcpDisable(response http.ResponseWriter, request *http.Request, _ string) {
	if !router.decodeMCPConfirmation(response, request, "DISABLE") {
		return
	}
	if err := router.options.MCP.Disable(request.Context()); err != nil {
		router.mcpOperationError(response)
		return
	}
	writeJSON(response, http.StatusOK, router.mcpStatusValue())
}

func (router *Router) mcpRemoveInvalid(response http.ResponseWriter, request *http.Request, _ string) {
	if !router.decodeMCPConfirmation(response, request, "REMOVE_INVALID") {
		return
	}
	if err := router.options.MCP.RemoveInvalid(request.Context()); err != nil {
		router.mcpOperationError(response)
		return
	}
	writeJSON(response, http.StatusOK, router.mcpStatusValue())
}

func (router *Router) decodeMCPEmpty(response http.ResponseWriter, request *http.Request) bool {
	if err := decodeJSONLimit(request, &struct{}{}, maxMCPJSONBody); err != nil {
		writeError(response, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request.")
		return false
	}
	return true
}

func (router *Router) decodeMCPConfirmation(response http.ResponseWriter, request *http.Request, expected string) bool {
	if router.options.MCP == nil {
		router.mcpUnavailable(response)
		return false
	}
	var body struct {
		Confirmation string `json:"confirmation"`
	}
	if err := decodeJSONLimit(request, &body, maxMCPJSONBody); err != nil || body.Confirmation != expected {
		writeError(response, http.StatusBadRequest, "CONFIRMATION_REQUIRED", "Exact confirmation is required.")
		return false
	}
	return true
}

func (router *Router) mcpUnavailable(response http.ResponseWriter) {
	writeError(response, http.StatusInternalServerError, "CONSOLE_ERROR", "MCP management is unavailable.")
}
func (router *Router) mcpOperationError(response http.ResponseWriter) {
	writeError(response, http.StatusConflict, "MCP_STATE_CHANGED", "MCP state changed or the operation could not be completed.")
}
