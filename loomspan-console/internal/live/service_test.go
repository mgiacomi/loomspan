package live

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/applicationclient"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/target"
)

const testInstanceID = "11111111-1111-4111-8111-111111111111"

func testPolicy() applicationclient.NetworkPolicy {
	return applicationclient.NetworkPolicy{
		ConnectTimeout: time.Second, ResponseHeaderTimeout: time.Second, RequestTimeout: time.Second,
	}
}

func handshakeFrame() string {
	return "event: handshake\ndata: {\"instanceId\":\"" + testInstanceID + "\",\"observedAt\":\"2026-07-25T12:00:00Z\",\"afterCursor\":\"0\"}\n\n"
}

func activityFrame(cursor, kind string) string {
	return "id: " + cursor + "\nevent: activity\ndata: {\"instanceId\":\"" + testInstanceID + "\",\"cursor\":\"" + cursor + "\",\"sessionId\":\"session-1\",\"traceId\":\"trace-1\",\"canonicalSequence\":" + cursor + ",\"timestamp\":\"2026-07-25T12:00:00Z\",\"kind\":\"" + kind + "\",\"executionStatus\":\"ACTIVE\",\"summary\":\"test\",\"details\":{}}\n\n"
}

func activityFrameWithSession(cursor, sessionID, kind string) string {
	return "id: " + cursor + "\nevent: activity\ndata: {\"instanceId\":\"" + testInstanceID + "\",\"cursor\":\"" + cursor + "\",\"sessionId\":\"" + sessionID + "\",\"traceId\":\"trace-1\",\"canonicalSequence\":" + cursor + ",\"timestamp\":\"2026-07-25T12:00:00Z\",\"kind\":\"" + kind + "\",\"executionStatus\":\"ACTIVE\",\"summary\":\"test\",\"details\":{}}\n\n"
}

func setupTargetWithOwner(t *testing.T, sse string, owner target.ScopeOwner) (*target.Context, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if strings.Contains(request.URL.Path, "/activity") {
			response.Header().Set(applicationclient.InstanceIDHeader, testInstanceID)
			response.Header().Set("Content-Type", "text/event-stream")
			_, _ = response.Write([]byte(sse))
			return
		}
		response.Header().Set(applicationclient.InstanceIDHeader, testInstanceID)
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"instanceId":"` + testInstanceID + `","consoleCompatibilityVersion":"0.1.0-SNAPSHOT","observedAt":"2026-07-25T12:00:00Z","liveMonitoringAvailable":true}`))
	}))
	address, _ := applicationclient.NormalizeAddress(server.URL)
	client, _ := applicationclient.New(address, testPolicy(), "0.1.0-SNAPSHOT")
	targetContext, _ := target.New(func(addr applicationclient.Address) (target.ProbeClient, error) {
		return client, nil
	}, nil, time.Now)
	if owner != nil {
		if err := targetContext.RegisterOwner("live", owner); err != nil {
			t.Fatal(err)
		}
	}
	if err := targetContext.Select(server.URL); err != nil {
		t.Fatal(err)
	}
	_, domain := targetContext.SupplyCredential(context.Background(), []byte(strings.Repeat("k", 32)))
	if domain != nil {
		t.Fatal(domain)
	}
	return targetContext, server
}

func TestServiceReceivesActivitiesViaStream(t *testing.T) {
	sse := handshakeFrame() + activityFrame("7", "TRACE_COMPLETED") + activityFrame("8", "EXECUTION_OBSERVATION_ENDED")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service := NewService(ctx)
	targetContext, server := setupTargetWithOwner(t, sse, service)
	defer server.Close()
	defer targetContext.Close()
	defer service.Close()
	time.Sleep(200 * time.Millisecond)
	recent := requireRecent(t, service, RecentRequest{Limit: 10})
	items, hasMore, next := recent.Items, recent.HasMore, recent.NextCursor
	if len(items) != 2 {
		t.Fatalf("expected 2 activities, got %d", len(items))
	}
	if items[0].Cursor != "7" || items[1].Cursor != "8" {
		t.Fatalf("unexpected cursors: %s, %s", items[0].Cursor, items[1].Cursor)
	}
	if hasMore || next != "" {
		t.Fatalf("expected no more, got hasMore=%v next=%s", hasMore, next)
	}
}

func TestServiceRecentWithCursorReturnsOnlyNewer(t *testing.T) {
	sse := handshakeFrame() + activityFrame("7", "TRACE_COMPLETED") + activityFrame("8", "EXECUTION_OBSERVATION_ENDED")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service := NewService(ctx)
	targetContext, server := setupTargetWithOwner(t, sse, service)
	defer server.Close()
	defer targetContext.Close()
	defer service.Close()
	time.Sleep(200 * time.Millisecond)
	recent := requireRecent(t, service, RecentRequest{Cursor: "7", Limit: 10})
	items, hasMore := recent.Items, recent.HasMore
	if len(items) != 1 || items[0].Cursor != "8" {
		t.Fatalf("expected 1 activity after cursor 7, got %d items", len(items))
	}
	if hasMore {
		t.Fatal("expected hasMore=false")
	}
}

func TestServiceRebaselinesAndReconnectsAfterStaleCursor(t *testing.T) {
	var activityRequests atomic.Int32
	var baselineLoads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if strings.Contains(request.URL.Path, "/activity") {
			attempt := activityRequests.Add(1)
			if attempt == 1 {
				response.Header().Set("Content-Type", "application/json")
				response.WriteHeader(http.StatusGone)
				_, _ = response.Write([]byte(`{"status":410,"code":"STALE_CURSOR","message":"stale"}`))
				return
			}
			response.Header().Set(applicationclient.InstanceIDHeader, testInstanceID)
			response.Header().Set("Content-Type", "text/event-stream")
			_, _ = response.Write([]byte(handshakeFrame() + activityFrame("1", "TRACE_STARTED")))
			if flusher, ok := response.(http.Flusher); ok {
				flusher.Flush()
			}
			<-request.Context().Done()
			return
		}
		response.Header().Set(applicationclient.InstanceIDHeader, testInstanceID)
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"instanceId":"` + testInstanceID + `","consoleCompatibilityVersion":"0.1.0-SNAPSHOT","observedAt":"2026-07-25T12:00:00Z","liveMonitoringAvailable":true}`))
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service := NewServiceWithTesting(ctx, time.Now, func(time.Duration, func()) tickerHandle {
		return &realTicker{t: time.NewTicker(time.Hour)}
	}, func(time.Duration) time.Duration { return 0 })
	service.SetBaselineLoader(func(context.Context, target.Scope) (Baseline, *consolecore.Error) {
		baselineLoads.Add(1)
		return Baseline{ResumeCursor: "0"}, nil
	})
	targetContext, _ := target.New(func(address applicationclient.Address) (target.ProbeClient, error) {
		return applicationclient.New(address, testPolicy(), "0.1.0-SNAPSHOT")
	}, nil, time.Now)
	if err := targetContext.RegisterOwner("live", service); err != nil {
		t.Fatal(err)
	}
	defer targetContext.Close()
	defer service.Close()
	if err := targetContext.Select(server.URL); err != nil {
		t.Fatal(err)
	}
	if _, domain := targetContext.SupplyCredential(context.Background(), []byte(strings.Repeat("k", 32))); domain != nil {
		t.Fatal(domain)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		items := requireRecent(t, service, RecentRequest{Limit: 10}).Items
		if len(items) == 1 {
			if activityRequests.Load() < 2 || baselineLoads.Load() < 2 {
				t.Fatalf("recovery counts activity=%d baseline=%d", activityRequests.Load(), baselineLoads.Load())
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("service did not recover activity after STALE_CURSOR")
}

func TestLateBaselineFromRotatedScopeCannotPublish(t *testing.T) {
	service := NewService(context.Background())
	defer service.Close()

	oldScope := target.Scope{ID: "scope-old"}
	newScope := target.Scope{ID: "scope-new"}
	started := make(chan struct{})
	release := make(chan struct{})
	service.SetBaselineLoader(func(context.Context, target.Scope) (Baseline, *consolecore.Error) {
		close(started)
		<-release
		return Baseline{ResumeCursor: "7"}, nil
	})

	service.mu.Lock()
	service.scope = &oldScope
	service.mu.Unlock()

	result := make(chan *consolecore.Error, 1)
	go func() {
		_, domain := service.refreshBaseline(context.Background(), oldScope)
		result <- domain
	}()
	<-started

	service.mu.Lock()
	service.scope = &newScope
	service.baseline = Baseline{ResumeCursor: "9"}
	service.connection = ConnectionFact{Reason: "new-scope"}
	service.mu.Unlock()
	close(release)

	domain := <-result
	if domain == nil || domain.Code != consolecore.CodeTargetChanged {
		t.Fatalf("expected TARGET_CHANGED for late baseline, got %#v", domain)
	}
	if baseline := service.Baseline(); baseline.ResumeCursor != "9" {
		t.Fatalf("late baseline replaced current scope baseline: %#v", baseline)
	}
	if service.publishConnectionFor(oldScope.ID, true, "") {
		t.Fatal("late connection state was accepted for the old scope")
	}
	if connection := service.Connection(); connection.Reason != "new-scope" {
		t.Fatalf("late connection replaced current scope connection: %#v", connection)
	}
}

func TestServiceSubscribeReceivesActivities(t *testing.T) {
	sse := handshakeFrame() + activityFrame("7", "TRACE_COMPLETED")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service := NewService(ctx)
	targetContext, server := setupTargetWithOwner(t, sse, service)
	defer server.Close()
	defer targetContext.Close()
	defer service.Close()
	ch, _, _, unsub := service.Subscribe()
	defer unsub()
	time.Sleep(200 * time.Millisecond)
	select {
	case activity := <-ch:
		if activity.Cursor != "7" {
			t.Fatalf("expected cursor 7, got %s", activity.Cursor)
		}
	case <-time.After(time.Second):
		t.Fatal("did not receive activity via subscription")
	}
}

func TestServiceInvalidateClearsActivities(t *testing.T) {
	sse := handshakeFrame() + activityFrame("7", "TRACE_COMPLETED")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service := NewService(ctx)
	targetContext, server := setupTargetWithOwner(t, sse, service)
	defer server.Close()
	defer targetContext.Close()
	defer service.Close()
	time.Sleep(200 * time.Millisecond)
	items := requireRecent(t, service, RecentRequest{Limit: 10}).Items
	if len(items) != 1 {
		t.Fatalf("expected 1 activity before invalidation, got %d", len(items))
	}
	service.InvalidateTargetScope("scope-1", context.Background())
	time.Sleep(50 * time.Millisecond)
	items = requireRecent(t, service, RecentRequest{Limit: 10}).Items
	if len(items) != 0 {
		t.Fatalf("expected 0 activities after invalidation, got %d", len(items))
	}
}

func TestServiceCloseCleansUp(t *testing.T) {
	sse := handshakeFrame() + activityFrame("7", "TRACE_COMPLETED")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service := NewService(ctx)
	targetContext, server := setupTargetWithOwner(t, sse, service)
	defer server.Close()
	defer targetContext.Close()
	time.Sleep(200 * time.Millisecond)
	service.Close()
	service.Close()
	ch, _, _, unsub := service.Subscribe()
	defer unsub()
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected closed channel from Subscribe after Close")
		}
	case <-time.After(time.Second):
		t.Fatal("expected closed channel from Subscribe after Close")
	}
}

func TestServiceRingBufferEvictsOldEntries(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service := NewService(ctx)
	defer service.Close()
	for i := 0; i < ringMaxCount+10; i++ {
		activity := Activity{
			Cursor:     fmt.Sprintf("%d", i+1),
			Kind:       KindTraceCompleted,
			InstanceID: testInstanceID,
			SessionID:  "session-1",
			TraceID:    "trace-1",
			Timestamp:  time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
			Summary:    "test",
		}
		service.mu.Lock()
		service.appendActivity(activity)
		service.mu.Unlock()
	}
	service.mu.Lock()
	count := len(service.activities)
	service.mu.Unlock()
	if count != ringMaxCount {
		t.Fatalf("expected %d entries, got %d", ringMaxCount, count)
	}
}

func TestActivityJSONRoundTrip(t *testing.T) {
	seq := int64(7)
	original := Activity{
		InstanceID:        testInstanceID,
		Cursor:            "7",
		SessionID:         "session-1",
		TraceID:           "trace-1",
		CanonicalSequence: &seq,
		Timestamp:         time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
		Kind:              KindTraceCompleted,
		ExecutionStatus:   "COMPLETED",
		Summary:           "Execution completed",
		Details:           json.RawMessage(`{"applicationTraceAvailability":"AVAILABLE"}`),
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Activity
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Cursor != original.Cursor || decoded.Kind != original.Kind {
		t.Fatalf("round-trip mismatch: %#v", decoded)
	}
}

func activityFrameWithInstance(cursor, instanceID, kind string) string {
	return "id: " + cursor + "\nevent: activity\ndata: {\"instanceId\":\"" + instanceID + "\",\"cursor\":\"" + cursor + "\",\"sessionId\":\"session-1\",\"traceId\":\"trace-1\",\"canonicalSequence\":" + cursor + ",\"timestamp\":\"2026-07-25T12:00:00Z\",\"kind\":\"" + kind + "\",\"executionStatus\":\"ACTIVE\",\"summary\":\"test\",\"details\":{}}\n\n"
}

func TestInvalidateTargetScopeDeliversLifecycleBeforeClose(t *testing.T) {
	sse := handshakeFrame() + activityFrame("1", "STEP_STARTED")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service := NewService(ctx)
	targetContext, server := setupTargetWithOwner(t, sse, service)
	defer server.Close()
	defer targetContext.Close()
	defer service.Close()
	time.Sleep(200 * time.Millisecond)

	_, lifecycleCh, _, unsub := service.Subscribe()
	defer unsub()

	service.InvalidateTargetScope("scope-1", context.Background())

	select {
	case event := <-lifecycleCh:
		if event.Kind != LifecycleTargetChanged {
			t.Fatalf("expected LifecycleTargetChanged, got %s", event.Kind)
		}
	case <-time.After(time.Second):
		t.Fatal("did not receive LifecycleTargetChanged event before channel close")
	}
}

func TestActivityHandlerRejectsPayloadFromChangedInstance(t *testing.T) {
	otherInstance := "22222222-2222-4222-8222-222222222222"
	sse := handshakeFrame() +
		activityFrame("1", "STEP_STARTED") +
		activityFrameWithInstance("2", otherInstance, "STEP_COMPLETED")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service := NewService(ctx)
	targetContext, server := setupTargetWithOwner(t, sse, service)
	defer server.Close()
	defer targetContext.Close()
	defer service.Close()
	time.Sleep(300 * time.Millisecond)

	service.mu.Lock()
	id := service.interval.instanceID
	reset := service.interval.reset
	service.mu.Unlock()
	if id == otherInstance {
		t.Fatalf("mismatched payload instance %s was adopted", otherInstance)
	}
	if reset == nil || reset.Cause != ResetInstanceChanged {
		t.Fatalf("expected instance_changed reset, got %#v", reset)
	}

	items := requireRecent(t, service, RecentRequest{Limit: 10}).Items
	if len(items) != 0 {
		t.Fatalf("expected mismatched-instance activity to be discarded, got %d", len(items))
	}
}

func TestRecentWithSessionFilterPaginatesCorrectly(t *testing.T) {
	sse := handshakeFrame() +
		activityFrameWithSession("1", "session-a", "STEP_STARTED") +
		activityFrameWithSession("2", "session-b", "STEP_STARTED") +
		activityFrameWithSession("3", "session-a", "STEP_COMPLETED") +
		activityFrameWithSession("4", "session-b", "STEP_STARTED") +
		activityFrameWithSession("5", "session-a", "STEP_COMPLETED") +
		activityFrameWithSession("6", "session-b", "STEP_COMPLETED")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service := NewService(ctx)
	targetContext, server := setupTargetWithOwner(t, sse, service)
	defer server.Close()
	defer targetContext.Close()
	defer service.Close()
	time.Sleep(200 * time.Millisecond)

	recent := requireRecent(t, service, RecentRequest{SessionID: "session-a", Limit: 2})
	items, hasMore, nextCursor := recent.Items, recent.HasMore, recent.NextCursor
	if len(items) != 2 {
		t.Fatalf("expected 2 filtered items, got %d", len(items))
	}
	if items[0].Cursor != "3" || items[1].Cursor != "5" {
		t.Fatalf("expected newest session-a suffix 3 and 5, got %s and %s", items[0].Cursor, items[1].Cursor)
	}
	if hasMore {
		t.Fatal("suffix query must not advertise newer session items")
	}
	if nextCursor != "" {
		t.Fatalf("expected no continuation for suffix query, got %s", nextCursor)
	}
}
