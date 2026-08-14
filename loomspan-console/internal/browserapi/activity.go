package browserapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/browserauth"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/live"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/target"
)

const (
	maxActivityRecentBody = 4 * 1024
	maxActivityStreamBody = 4 * 1024
)

type sseHandler func(http.ResponseWriter, *http.Request, string)

func (router *Router) withSessionSSE(response http.ResponseWriter, request *http.Request, handler sseHandler) {
	cookie, err := request.Cookie(browserauth.SessionCookieName)
	if err != nil || !router.options.Sessions.Authenticate(cookie.Value) {
		http.SetCookie(response, browserauth.ExpiredSessionCookie())
		writeError(response, http.StatusUnauthorized, "SESSION_REQUIRED", "Pairing is required.")
		return
	}
	tabIDs := request.Header.Values("X-loomspan-Console-Tab")
	tokens := request.Header.Values(csrfHeader)
	if len(tabIDs) != 1 || len(tokens) != 1 || strings.Contains(tokens[0], ",") ||
		!router.options.Sessions.ValidateCSRF(cookie.Value, tabIDs[0], tokens[0]) {
		writeError(response, http.StatusForbidden, "BROWSER_SECURITY_REJECTED", "Browser request rejected.")
		return
	}
	handler(response, request, cookie.Value)
}

func writeSSEEvent(response http.ResponseWriter, event string, data any) {
	encoded, err := json.Marshal(data)
	if err != nil {
		return
	}
	frame := "event: " + event + "\ndata: " + string(encoded) + "\n\n"
	_, _ = response.Write([]byte(frame))
}

func (router *Router) activityStream(response http.ResponseWriter, request *http.Request, sessionID string) {
	if router.options.Live == nil || router.options.Target == nil {
		writeError(response, http.StatusInternalServerError, "CONSOLE_ERROR", "Live activity monitoring is unavailable.")
		return
	}
	flusher, ok := response.(http.Flusher)
	if !ok {
		writeError(response, http.StatusInternalServerError, "CONSOLE_ERROR", "Streaming is not supported.")
		return
	}
	var body struct {
		AfterCursor string `json:"afterCursor,omitempty"`
	}
	if err := decodeJSONLimit(request, &body, maxActivityStreamBody); err != nil {
		writeError(response, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request.")
		return
	}
	scope, domain := router.options.Target.Capture()
	if domain != nil {
		writeDomainError(response, domain)
		return
	}
	tabID := request.Header.Get("X-loomspan-Console-Tab")
	relayContext, cancelRelay := context.WithCancel(request.Context())
	defer cancelRelay()
	releaseRelay, err := router.options.Sessions.AdmitRelay(sessionID, tabID, cancelRelay)
	if err != nil {
		writeError(response, http.StatusConflict, "RELAY_ADMITTED", "A relay stream is already active for this tab.")
		return
	}
	defer releaseRelay()

	connection, continuity, ch, lifecycleCh, replayOverflow, acknowledge, unsub :=
		router.options.Live.SubscribeSnapshotAfter(body.AfterCursor)
	defer unsub()
	liveUnavailable := router.options.Live.LiveUnavailable()

	if continuity != nil && continuity.TargetScopeID != "" && continuity.TargetScopeID != string(scope.ID) {
		writeDomainError(response, router.options.Target.RequireCurrent(target.ScopeID(continuity.TargetScopeID)))
		return
	}
	if domain := router.options.Target.PublishCurrentAtomic(scope.ID, func() {
		ApplyHeaders(response.Header())
		response.Header().Set("Content-Type", "text/event-stream")
		response.Header().Set("Cache-Control", "no-store")
		response.Header().Set("X-Content-Type-Options", "nosniff")
		response.Header().Set("Connection", "keep-alive")
		response.WriteHeader(http.StatusOK)
		writeSSEEvent(response, "console.connection", connection)
		if continuity != nil {
			writeSSEEvent(response, "console.continuity", continuity)
		}
		if liveUnavailable {
			writeSSEEvent(response, "console.connection", map[string]any{"connected": false, "reason": "live_unavailable"})
		}
		flusher.Flush()
	}); domain != nil {
		writeDomainError(response, domain)
		return
	}

	if replayOverflow {
		writeSSEEvent(response, "console.replay_gap", map[string]any{"reason": "replay_overflow"})
		flusher.Flush()
		return
	}

	ctx := relayContext
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-lifecycleCh:
			if !ok {
				writeSSEEvent(response, "console.connection", map[string]any{"connected": false, "reason": "stream_closed"})
				flusher.Flush()
				return
			}
			switch evt.Kind {
			case live.LifecycleBaselineRefreshed:
				writeSSEEvent(response, "console.baseline_refreshed", map[string]any{"observedAt": evt.ObservedAt})
			case live.LifecycleTargetChanged:
				writeSSEEvent(response, "console.target_changed", map[string]any{})
			case live.LifecycleSubscriberOverflow:
				writeSSEEvent(response, "console.replay_gap", map[string]any{"reason": "subscriber_overflow"})
				writeSSEEvent(response, "console.connection", map[string]any{"connected": false, "reason": "subscriber_overflow"})
				flusher.Flush()
				return
			case live.LifecycleConnectionChanged:
				if evt.Connection != nil {
					writeSSEEvent(response, "console.connection", evt.Connection)
				}
			}
			flusher.Flush()
		case activity, ok := <-ch:
			if !ok {
				writeSSEEvent(response, "console.connection", map[string]any{"connected": false, "reason": "stream_closed"})
				flusher.Flush()
				return
			}
			acknowledge(activity)
			encoded, _ := json.Marshal(activity)
			if _, err := response.Write([]byte("event: loomspan.activity\ndata: ")); err != nil {
				return
			}
			if _, err := response.Write(encoded); err != nil {
				return
			}
			if _, err := response.Write([]byte("\n\n")); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (router *Router) activityRecent(response http.ResponseWriter, request *http.Request, _ string) {
	if router.options.Live == nil || router.options.Target == nil {
		writeError(response, http.StatusInternalServerError, "CONSOLE_ERROR", "Live activity monitoring is unavailable.")
		return
	}
	var body live.RecentRequest
	if err := decodeJSONLimit(request, &body, maxActivityRecentBody); err != nil {
		writeError(response, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request.")
		return
	}
	scope, domain := router.options.Target.Capture()
	if domain != nil {
		writeDomainError(response, domain)
		return
	}
	result, domain := router.options.Live.Recent(body)
	if domain != nil {
		writeDomainError(response, domain)
		return
	}
	if result.Continuity != nil && result.Continuity.TargetScopeID != "" && result.Continuity.TargetScopeID != string(scope.ID) {
		writeDomainError(response, router.options.Target.RequireCurrent(target.ScopeID(result.Continuity.TargetScopeID)))
		return
	}
	content, err := json.Marshal(result)
	if err != nil {
		writeError(response, http.StatusInternalServerError, "CONSOLE_ERROR", "The Console response could not be created.")
		return
	}
	content = append(content, '\n')
	if domain := router.options.Target.PublishCurrentAtomic(scope.ID, func() {
		writeJSONBytes(response, http.StatusOK, content)
	}); domain != nil {
		writeDomainError(response, domain)
	}
}
