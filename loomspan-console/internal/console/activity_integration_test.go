package console

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/applicationclient"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/browserapi"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/browserauth"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/live"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/target"
)

func TestActivitySSEEndToEndRelay(t *testing.T) {
	const instanceID = "11111111-1111-4111-8111-111111111111"
	const sseResponse = "event: handshake\ndata: {\"instanceId\":\"" + instanceID + "\",\"observedAt\":\"2026-07-25T12:00:00Z\",\"afterCursor\":\"0\"}\n\n" +
		"id: 7\nevent: activity\ndata: {\"instanceId\":\"" + instanceID + "\",\"cursor\":\"7\",\"sessionId\":\"session-1\",\"traceId\":\"trace-1\",\"canonicalSequence\":7,\"timestamp\":\"2026-07-25T12:00:00Z\",\"kind\":\"TRACE_COMPLETED\",\"executionStatus\":\"COMPLETED\",\"summary\":\"Execution completed\",\"details\":{\"applicationTraceAvailability\":\"AVAILABLE\"}}\n\n"

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if strings.Contains(request.URL.Path, "/activity") {
			response.Header().Set(applicationclient.InstanceIDHeader, instanceID)
			response.Header().Set("Content-Type", "text/event-stream")
			_, _ = response.Write([]byte(sseResponse))
			return
		}
		response.Header().Set(applicationclient.InstanceIDHeader, instanceID)
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"instanceId":"` + instanceID + `","consoleCompatibilityVersion":"0.1.0-SNAPSHOT","observedAt":"2026-07-25T12:00:00Z","liveMonitoringAvailable":true,"registeredSkillCount":0,"activeExecutionCount":0,"catalogedTraceCount":0,"tracePersistencePolicy":"PERSISTENT","completionGraceTtl":"PT2M","traceCatalogMetadataTtl":"PT168H"}`))
	}))
	defer server.Close()

	policy := applicationclient.NetworkPolicy{
		ConnectTimeout: time.Second, ResponseHeaderTimeout: time.Second, RequestTimeout: time.Second,
	}
	targetContext, err := target.New(func(address applicationclient.Address) (target.ProbeClient, error) {
		return applicationclient.New(address, policy, "0.1.0-SNAPSHOT")
	}, nil, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	defer targetContext.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	liveService := live.NewService(ctx)
	defer liveService.Close()
	if err := targetContext.RegisterOwner("live", liveService); err != nil {
		t.Fatal(err)
	}

	if err := targetContext.Select(server.URL); err != nil {
		t.Fatal(err)
	}
	if _, domain := targetContext.SupplyCredential(context.Background(), []byte(strings.Repeat("k", 32))); domain != nil {
		t.Fatal(domain)
	}

	time.Sleep(300 * time.Millisecond)

	recent, domain := liveService.Recent(live.RecentRequest{Limit: 10})
	if domain != nil {
		t.Fatal(domain)
	}
	items := recent.Items
	if len(items) != 1 {
		t.Fatalf("expected 1 activity in ring buffer, got %d", len(items))
	}
	if items[0].Cursor != "7" || items[0].Kind != live.KindTraceCompleted {
		t.Fatalf("unexpected activity: cursor=%s kind=%s", items[0].Cursor, items[0].Kind)
	}

	pairing := browserauth.NewPairing(nil, nil)
	sessions := browserauth.NewRegistry(nil, nil)
	defer pairing.Close()
	defer sessions.Close()
	sessionID, _ := sessions.CreateSession()

	router, err := browserapi.New(browserapi.Options{
		Policy:     mustNewPolicy(t),
		Pairing:    pairing,
		Sessions:   sessions,
		PairingURL: func(s string) string { return s },
		Target:     targetContext,
		Live:       liveService,
	})
	if err != nil {
		t.Fatal(err)
	}

	recentRequest := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7943/api/console/v1/activity/recent", strings.NewReader(`{}`))
	recentRequest.Host = "127.0.0.1:7943"
	recentRequest.Header.Set("Origin", "http://127.0.0.1:7943")
	recentRequest.AddCookie(browserauth.SessionCookie(sessionID))
	recentResponse := httptest.NewRecorder()
	router.ServeHTTP(recentResponse, recentRequest)
	if recentResponse.Code != http.StatusOK {
		t.Fatalf("recent endpoint returned %d: %s", recentResponse.Code, recentResponse.Body.String())
	}
	if !strings.Contains(recentResponse.Body.String(), `"cursor":"7"`) {
		t.Fatalf("recent response does not contain activity: %s", recentResponse.Body.String())
	}
	if !strings.Contains(recentResponse.Body.String(), `"TRACE_COMPLETED"`) {
		t.Fatalf("recent response does not contain kind: %s", recentResponse.Body.String())
	}
}

func TestActivitySSEStreamEndToEndWithTabHeader(t *testing.T) {
	const instanceID = "11111111-1111-4111-8111-111111111111"
	const sseResponse = "event: handshake\ndata: {\"instanceId\":\"" + instanceID + "\",\"observedAt\":\"2026-07-25T12:00:00Z\",\"afterCursor\":\"0\"}\n\n" +
		"id: 1\nevent: activity\ndata: {\"instanceId\":\"" + instanceID + "\",\"cursor\":\"1\",\"sessionId\":\"session-1\",\"traceId\":\"trace-1\",\"canonicalSequence\":1,\"timestamp\":\"2026-07-25T12:00:00Z\",\"kind\":\"STEP_STARTED\",\"executionStatus\":\"RUNNING\",\"summary\":\"Step started\",\"details\":{}}\n\n"

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if strings.Contains(request.URL.Path, "/activity") {
			response.Header().Set(applicationclient.InstanceIDHeader, instanceID)
			response.Header().Set("Content-Type", "text/event-stream")
			_, _ = response.Write([]byte(sseResponse))
			return
		}
		response.Header().Set(applicationclient.InstanceIDHeader, instanceID)
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"instanceId":"` + instanceID + `","consoleCompatibilityVersion":"0.1.0-SNAPSHOT","observedAt":"2026-07-25T12:00:00Z","liveMonitoringAvailable":true,"registeredSkillCount":0,"activeExecutionCount":0,"catalogedTraceCount":0,"tracePersistencePolicy":"PERSISTENT","completionGraceTtl":"PT2M","traceCatalogMetadataTtl":"PT168H"}`))
	}))
	defer server.Close()

	policy := applicationclient.NetworkPolicy{
		ConnectTimeout: time.Second, ResponseHeaderTimeout: time.Second, RequestTimeout: time.Second,
	}
	targetContext, err := target.New(func(address applicationclient.Address) (target.ProbeClient, error) {
		return applicationclient.New(address, policy, "0.1.0-SNAPSHOT")
	}, nil, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	defer targetContext.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	liveService := live.NewService(ctx)
	defer liveService.Close()
	if err := targetContext.RegisterOwner("live", liveService); err != nil {
		t.Fatal(err)
	}

	if err := targetContext.Select(server.URL); err != nil {
		t.Fatal(err)
	}
	if _, domain := targetContext.SupplyCredential(context.Background(), []byte(strings.Repeat("k", 32))); domain != nil {
		t.Fatal(domain)
	}

	time.Sleep(300 * time.Millisecond)

	pairing := browserauth.NewPairing(nil, nil)
	sessions := browserauth.NewRegistry(nil, nil)
	defer pairing.Close()
	defer sessions.Close()
	sessionID, _ := sessions.CreateSession()

	router, err := browserapi.New(browserapi.Options{
		Policy:     mustNewPolicy(t),
		Pairing:    pairing,
		Sessions:   sessions,
		PairingURL: func(s string) string { return s },
		Target:     targetContext,
		Live:       liveService,
	})
	if err != nil {
		t.Fatal(err)
	}

	bootstrapRequest := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7943/api/console/v1/bootstrap", strings.NewReader(`{}`))
	bootstrapRequest.Host = "127.0.0.1:7943"
	bootstrapRequest.Header.Set("Origin", "http://127.0.0.1:7943")
	bootstrapRequest.AddCookie(browserauth.SessionCookie(sessionID))
	bootstrapResponse := httptest.NewRecorder()
	router.ServeHTTP(bootstrapResponse, bootstrapRequest)
	if bootstrapResponse.Code != http.StatusOK {
		t.Fatalf("bootstrap failed: %d %s", bootstrapResponse.Code, bootstrapResponse.Body.String())
	}
	var bs struct {
		TabID     string `json:"tabId"`
		CSRFToken string `json:"csrfToken"`
	}
	if err := json.Unmarshal(bootstrapResponse.Body.Bytes(), &bs); err != nil {
		t.Fatal(err)
	}

	streamCtx, streamCancel := context.WithCancel(context.Background())
	defer streamCancel()
	streamRequest := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7943/api/console/v1/activity/stream", strings.NewReader(`{}`))
	streamRequest = streamRequest.WithContext(streamCtx)
	streamRequest.Host = "127.0.0.1:7943"
	streamRequest.Header.Set("Origin", "http://127.0.0.1:7943")
	streamRequest.Header.Set("X-loomspan-Console-Tab", bs.TabID)
	streamRequest.Header.Set("X-loomspan-Console-CSRF", bs.CSRFToken)
	streamRequest.AddCookie(browserauth.SessionCookie(sessionID))
	streamResponse := httptest.NewRecorder()
	streamDone := make(chan struct{})
	go func() {
		router.ServeHTTP(streamResponse, streamRequest)
		close(streamDone)
	}()

	time.Sleep(200 * time.Millisecond)

	// Snapshot the response body under the stream lock. The header map is
	// written by the handler goroutine via ApplyHeaders and must not be read
	// concurrently. We cancel the stream and wait for the handler to exit
	// before reading headers to avoid a data race on the ResponseRecorder's
	// header map.
	streamCancel()
	select {
	case <-streamDone:
	case <-time.After(time.Second):
		t.Fatal("stream handler did not exit after context cancellation")
	}

	if contentType := streamResponse.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/event-stream") {
		t.Fatalf("expected text/event-stream, got %q", contentType)
	}

	body := streamResponse.Body.String()
	if !strings.Contains(body, "console.connection") {
		t.Fatalf("expected console.connection event in stream output: %s", body)
	}
	if !strings.Contains(body, `"connected":false`) || !strings.Contains(body, `"reason":"upstream_disconnected"`) {
		t.Fatalf("expected the completed upstream stream to report its disconnected state: %s", body)
	}
	if !strings.Contains(body, "loomspan.activity") {
		t.Fatalf("expected loomspan.activity namespace in stream output: %s", body)
	}
	if !strings.Contains(body, `"STEP_STARTED"`) {
		t.Fatalf("expected STEP_STARTED activity in stream output: %s", body)
	}
	if xcto := streamResponse.Header().Get("X-Content-Type-Options"); xcto != "nosniff" {
		t.Fatalf("expected X-Content-Type-Options nosniff, got %q", xcto)
	}
}

func mustNewPolicy(t *testing.T) browserapi.Policy {
	t.Helper()
	policy, err := browserapi.NewPolicy("127.0.0.1:7943", "http://127.0.0.1:7943", "")
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

func TestActivitySSEStreamDoesNotTreatLifetimeThroughputAsBackpressure(t *testing.T) {
	const instanceID = "11111111-1111-4111-8111-111111111111"
	const frameCount = 513

	deliver := make(chan struct{})
	upstreamDelivered := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if strings.Contains(request.URL.Path, "/activity") {
			response.Header().Set(applicationclient.InstanceIDHeader, instanceID)
			response.Header().Set("Content-Type", "text/event-stream")
			response.WriteHeader(http.StatusOK)
			if flusher, ok := response.(http.Flusher); ok {
				flusher.Flush()
			}
			<-deliver
			_, _ = response.Write([]byte("event: handshake\ndata: {\"instanceId\":\"" + instanceID + "\",\"observedAt\":\"2026-07-25T12:00:00Z\",\"afterCursor\":\"0\"}\n\n"))
			if flusher, ok := response.(http.Flusher); ok {
				flusher.Flush()
			}
			for i := 1; i <= frameCount; i++ {
				frame := fmt.Sprintf("id: %d\nevent: activity\ndata: {\"instanceId\":\"%s\",\"cursor\":\"%d\",\"sessionId\":\"session-1\",\"traceId\":\"trace-1\",\"canonicalSequence\":%d,\"timestamp\":\"2026-07-25T12:00:00Z\",\"kind\":\"STEP_STARTED\",\"executionStatus\":\"RUNNING\",\"summary\":\"Step %d\",\"details\":{}}\n\n", i, instanceID, i, i, i)
				_, _ = response.Write([]byte(frame))
				if flusher, ok := response.(http.Flusher); ok {
					flusher.Flush()
				}
				time.Sleep(time.Millisecond)
			}
			close(upstreamDelivered)
			return
		}
		response.Header().Set(applicationclient.InstanceIDHeader, instanceID)
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"instanceId":"` + instanceID + `","consoleCompatibilityVersion":"0.1.0-SNAPSHOT","observedAt":"2026-07-25T12:00:00Z","liveMonitoringAvailable":true,"registeredSkillCount":0,"activeExecutionCount":0,"catalogedTraceCount":0,"tracePersistencePolicy":"PERSISTENT","completionGraceTtl":"PT2M","traceCatalogMetadataTtl":"PT168H"}`))
	}))
	defer server.Close()

	policy := applicationclient.NetworkPolicy{
		ConnectTimeout: time.Second, ResponseHeaderTimeout: time.Second, RequestTimeout: 30 * time.Second,
	}
	targetContext, err := target.New(func(address applicationclient.Address) (target.ProbeClient, error) {
		return applicationclient.New(address, policy, "0.1.0-SNAPSHOT")
	}, nil, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	defer targetContext.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	liveService := live.NewService(ctx)
	defer liveService.Close()
	if err := targetContext.RegisterOwner("live", liveService); err != nil {
		t.Fatal(err)
	}

	if err := targetContext.Select(server.URL); err != nil {
		t.Fatal(err)
	}
	if _, domain := targetContext.SupplyCredential(context.Background(), []byte(strings.Repeat("k", 32))); domain != nil {
		t.Fatal(domain)
	}

	time.Sleep(300 * time.Millisecond)

	pairing := browserauth.NewPairing(nil, nil)
	sessions := browserauth.NewRegistry(nil, nil)
	defer pairing.Close()
	defer sessions.Close()
	sessionID, _ := sessions.CreateSession()

	router, err := browserapi.New(browserapi.Options{
		Policy:     mustNewPolicy(t),
		Pairing:    pairing,
		Sessions:   sessions,
		PairingURL: func(s string) string { return s },
		Target:     targetContext,
		Live:       liveService,
	})
	if err != nil {
		t.Fatal(err)
	}

	bootstrapRequest := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7943/api/console/v1/bootstrap", strings.NewReader(`{}`))
	bootstrapRequest.Host = "127.0.0.1:7943"
	bootstrapRequest.Header.Set("Origin", "http://127.0.0.1:7943")
	bootstrapRequest.AddCookie(browserauth.SessionCookie(sessionID))
	bootstrapResponse := httptest.NewRecorder()
	router.ServeHTTP(bootstrapResponse, bootstrapRequest)
	var bs struct {
		TabID     string `json:"tabId"`
		CSRFToken string `json:"csrfToken"`
	}
	_ = json.Unmarshal(bootstrapResponse.Body.Bytes(), &bs)

	streamCtx, streamCancel := context.WithCancel(context.Background())
	defer streamCancel()
	streamRequest := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7943/api/console/v1/activity/stream", strings.NewReader(`{}`))
	streamRequest = streamRequest.WithContext(streamCtx)
	streamRequest.Host = "127.0.0.1:7943"
	streamRequest.Header.Set("Origin", "http://127.0.0.1:7943")
	streamRequest.Header.Set("X-loomspan-Console-Tab", bs.TabID)
	streamRequest.Header.Set("X-loomspan-Console-CSRF", bs.CSRFToken)
	streamRequest.AddCookie(browserauth.SessionCookie(sessionID))
	streamResponse := httptest.NewRecorder()
	streamDone := make(chan struct{})
	go func() {
		router.ServeHTTP(streamResponse, streamRequest)
		close(streamDone)
	}()

	time.Sleep(200 * time.Millisecond)
	close(deliver)

	select {
	case <-upstreamDelivered:
	case <-time.After(5 * time.Second):
		t.Fatal("upstream did not finish sustained activity delivery")
	}
	time.Sleep(500 * time.Millisecond)
	streamCancel()
	<-streamDone

	body := streamResponse.Body.String()
	if strings.Contains(body, "console.replay_gap") {
		t.Fatalf("healthy relay was incorrectly classified as backpressured: %s", body[:min(len(body), 500)])
	}
	if !strings.Contains(body, `"cursor":"513"`) {
		t.Fatalf("relay did not deliver sustained activity through cursor 513: %s", body[max(0, len(body)-500):])
	}
}

// TestCommittedTerminalActivityUsesCanonicalApplicationTraceAvailability is the
// PR12-R14 boundary-contract test. It loads the committed terminal-activity SSE
// fixture through the existing application activity decoder and asserts that the
// relayed terminal activity exposes the canonical applicationTraceAvailability
// field used by the live Java producer, with no remaining obsolete availability
// key. See ai/thoughts/plans/2026-07-29-PR-12-loomspan-console-artifact-service-testing.md.
func TestCommittedTerminalActivityUsesCanonicalApplicationTraceAvailability(t *testing.T) {
	const instanceID = "11111111-1111-4111-8111-111111111111"
	fixture, err := os.ReadFile(filepath.Join("..", "..", "..", "loomspan-console-fixtures", "application-sse", "activity-trace-completed.sse"))
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set(applicationclient.InstanceIDHeader, instanceID)
		response.Header().Set("Content-Type", "text/event-stream")
		handshake := "event: handshake\ndata: {\"instanceId\":\"" + instanceID + "\",\"observedAt\":\"2026-07-25T12:00:00Z\",\"afterCursor\":\"0\"}\n\n"
		_, _ = response.Write([]byte(handshake))
		_, _ = response.Write(fixture)
	}))
	defer server.Close()

	address, _ := applicationclient.NormalizeAddress(server.URL)
	client, _ := applicationclient.New(address, applicationclient.NetworkPolicy{
		ConnectTimeout: time.Second, ResponseHeaderTimeout: time.Second, RequestTimeout: time.Second,
	}, "0.1.0-SNAPSHOT")
	defer client.Close()

	stream, err := client.OpenActivity(context.Background(), instanceID, "0", testCredentialValue())
	if err != nil {
		t.Fatalf("open activity stream: %v", err)
	}
	defer stream.Close()

	if _, err := stream.Next(); err != nil {
		t.Fatalf("read handshake frame: %v", err)
	}
	terminal, err := stream.Next()
	if err != nil {
		t.Fatalf("read terminal activity frame: %v", err)
	}
	if terminal.Event != "activity" {
		t.Fatalf("expected activity event, got %q", terminal.Event)
	}

	var details map[string]json.RawMessage
	var envelope struct {
		Details json.RawMessage `json:"details"`
	}
	if err := json.Unmarshal(terminal.Data, &envelope); err != nil {
		t.Fatalf("decode activity envelope: %v", err)
	}
	if err := json.Unmarshal(envelope.Details, &details); err != nil {
		t.Fatalf("decode activity details: %v", err)
	}

	availability, ok := details["applicationTraceAvailability"]
	if !ok {
		t.Fatalf("committed terminal activity is missing applicationTraceAvailability: %s", string(envelope.Details))
	}
	var value string
	if err := json.Unmarshal(availability, &value); err != nil || value != "AVAILABLE" {
		t.Fatalf("applicationTraceAvailability = %q, want AVAILABLE", string(availability))
	}
	obsoleteField := "artifact" + "Availability"
	if _, present := details[obsoleteField]; present {
		t.Fatalf("committed terminal activity still carries obsolete availability field: %s", string(envelope.Details))
	}
}

func testCredentialValue() applicationclient.Credential {
	return testCredential(strings.Repeat("k", 32))
}

type testCredential []byte

func (credential testCredential) Apply(request *http.Request) error {
	if err := applicationclient.ValidateCredential(credential); err != nil {
		return err
	}
	request.Header.Set(applicationclient.APIKeyHeader, string(credential))
	return nil
}
