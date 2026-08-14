package live

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func intPtr(v int64) *int64 { return &v }

func makeActivity(cursor, sessionID, kind string) Activity {
	return Activity{
		InstanceID:        testInstanceID,
		Cursor:            cursor,
		SessionID:         sessionID,
		TraceID:           "trace-1",
		CanonicalSequence: intPtr(7),
		Timestamp:         time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
		Kind:              ActivityKind(kind),
		ExecutionStatus:   "ACTIVE",
		Summary:           "test",
		Details:           json.RawMessage(`{}`),
	}
}

func requireRecent(t *testing.T, service *Service, request RecentRequest) RecentResponse {
	t.Helper()
	response, domain := service.Recent(request)
	if domain != nil {
		t.Fatalf("Recent returned %s: %s", domain.Code, domain.Message)
	}
	return response
}

func TestRecentActivityIgnoresExactDuplicate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service := NewService(ctx)
	defer service.Close()

	activity := makeActivity("1", "session-1", "TRACE_STARTED")
	service.mu.Lock()
	service.appendActivity(activity)
	service.mu.Unlock()

	dup := makeActivity("1", "session-1", "TRACE_STARTED")
	service.mu.Lock()
	appended := service.appendActivity(dup)
	service.mu.Unlock()

	if appended {
		t.Fatal("exact duplicate cursor should be ignored")
	}

	items := requireRecent(t, service, RecentRequest{Limit: 10}).Items
	if len(items) != 1 {
		t.Fatalf("expected 1 item after duplicate, got %d", len(items))
	}
}

func TestRecentActivityQueryDoesNotReturnRetainedStateWhenLiveUnavailable(t *testing.T) {
	service := NewService(context.Background())
	defer service.Close()

	service.mu.Lock()
	service.interval = &interval{instanceID: testInstanceID}
	service.appendActivity(makeActivity("1", "session-1", "TRACE_STARTED"))
	service.liveUnavailable = true
	service.mu.Unlock()

	response, domain := service.Recent(RecentRequest{SessionID: "session-1", Limit: 10})
	if domain == nil || domain.Code != "LIVE_MONITORING_UNAVAILABLE" {
		t.Fatalf("live-unavailable error = %#v", domain)
	}
	if len(response.Items) != 0 {
		t.Fatalf("live-unavailable query returned %d retained activities", len(response.Items))
	}
}

func TestRecentActivityQueryCapturesObservedAtWithWindowSnapshot(t *testing.T) {
	queryTime := time.Date(2026, 8, 13, 20, 0, 0, 123, time.FixedZone("test", -7*60*60))
	continuityTime := time.Date(2026, 8, 13, 18, 0, 0, 0, time.UTC)
	clock := &fakeClock{t: queryTime}
	service := NewServiceWithClock(context.Background(), clock.now)
	defer service.Close()
	service.mu.Lock()
	service.interval = &interval{instanceID: testInstanceID, observedAt: continuityTime}
	service.mu.Unlock()

	response := requireRecent(t, service, RecentRequest{Limit: 10})
	if !response.ObservedAt.Equal(queryTime.UTC()) {
		t.Fatalf("query observedAt = %s, want %s", response.ObservedAt, queryTime.UTC())
	}
	if response.Continuity == nil || !response.Continuity.ObservedAt.Equal(continuityTime) {
		t.Fatalf("continuity observedAt = %#v, want %s", response.Continuity, continuityTime)
	}
	if response.Items == nil {
		t.Fatal("empty recent items must be an array, not null")
	}
}

func TestRecentActivityRejectsConflictingDuplicateAndRegression(t *testing.T) {
	service := NewService(context.Background())
	defer service.Close()
	service.mu.Lock()
	if appended, err := service.acceptActivity(makeActivity("2", "session-1", "TRACE_STARTED")); err != nil || !appended {
		service.mu.Unlock()
		t.Fatalf("initial activity rejected: appended=%v err=%v", appended, err)
	}
	conflict := makeActivity("2", "session-1", "STEP_STARTED")
	if _, err := service.acceptActivity(conflict); err == nil {
		service.mu.Unlock()
		t.Fatal("conflicting duplicate cursor was accepted")
	}
	if _, err := service.acceptActivity(makeActivity("1", "session-1", "TRACE_STARTED")); err == nil {
		service.mu.Unlock()
		t.Fatal("cursor regression was accepted")
	}
	service.mu.Unlock()
}

func TestRecentActivityResetClearsBeforePostBoundaryAdmission(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	clock := &fakeClock{t: time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)}
	service := NewServiceWithClock(ctx, clock.now)
	defer service.Close()

	activity := makeActivity("1", "session-1", "TRACE_STARTED")
	service.mu.Lock()
	service.appendActivity(activity)
	service.interval = &interval{instanceID: testInstanceID, firstCursor: "1", lastCursor: "1"}
	service.mu.Unlock()

	service.InvalidateTargetScope("scope-1", context.Background())

	recent := requireRecent(t, service, RecentRequest{Limit: 10})
	items, continuity := recent.Items, recent.Continuity
	if len(items) != 0 {
		t.Fatal("expected 0 items after reset")
	}
	if continuity == nil || continuity.Reset == nil {
		t.Fatal("expected reset fact in continuity")
	}
	if continuity.Reset.Cause != ResetTargetScopeChanged {
		t.Fatalf("expected reset cause target_scope_changed, got %s", continuity.Reset.Cause)
	}
}

func TestRecentActivityNeverReturnsMultipleIntervals(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	clock := &fakeClock{t: time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)}
	service := NewServiceWithClock(ctx, clock.now)
	defer service.Close()

	service.mu.Lock()
	service.interval = &interval{instanceID: testInstanceID}
	for i := 1; i <= 5; i++ {
		service.appendActivity(makeActivity(fmt.Sprintf("%d", i), "session-1", "TRACE_STARTED"))
	}
	service.mu.Unlock()

	service.InvalidateTargetScope("scope-1", context.Background())

	service.mu.Lock()
	service.interval.instanceID = testInstanceID
	for i := 6; i <= 8; i++ {
		service.appendActivity(makeActivity(fmt.Sprintf("%d", i), "session-1", "TRACE_STARTED"))
	}
	service.mu.Unlock()

	recent := requireRecent(t, service, RecentRequest{Limit: 100})
	items, continuity := recent.Items, recent.Continuity
	if len(items) != 3 {
		t.Fatalf("expected 3 items from new interval only, got %d", len(items))
	}
	if continuity == nil || continuity.Reset == nil {
		t.Fatal("expected reset fact")
	}
}

func TestRecentActivityQueryFiltersBySessionID(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service := NewService(ctx)
	defer service.Close()

	service.mu.Lock()
	service.appendActivity(makeActivity("1", "session-a", "TRACE_STARTED"))
	service.appendActivity(makeActivity("2", "session-b", "TRACE_STARTED"))
	service.appendActivity(makeActivity("3", "session-a", "STEP_COMPLETED"))
	service.mu.Unlock()

	items := requireRecent(t, service, RecentRequest{SessionID: "session-a", Limit: 100}).Items
	if len(items) != 2 {
		t.Fatalf("expected 2 items for session-a, got %d", len(items))
	}
	for _, item := range items {
		if item.SessionID != "session-a" {
			t.Fatalf("expected session-a, got %s", item.SessionID)
		}
	}
}

func TestRecentActivityQueryClampsPageSize(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service := NewService(ctx)
	defer service.Close()

	for i := 1; i <= 5; i++ {
		service.mu.Lock()
		service.appendActivity(makeActivity(fmt.Sprintf("%d", i), "session-1", "TRACE_STARTED"))
		service.mu.Unlock()
	}

	recent := requireRecent(t, service, RecentRequest{Limit: 2})
	items, hasMore, next := recent.Items, recent.HasMore, recent.NextCursor
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if hasMore {
		t.Fatal("suffix query must not advertise newer items")
	}
	if next != "" || items[0].Cursor != "4" || items[1].Cursor != "5" {
		t.Fatalf("expected newest suffix 4,5 with no continuation, got %#v next=%q", items, next)
	}
}

func TestRecentActivityQueryClampsMaxPageSize(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service := NewService(ctx)
	defer service.Close()

	for i := 1; i <= 5; i++ {
		service.mu.Lock()
		service.appendActivity(makeActivity(fmt.Sprintf("%d", i), "session-1", "TRACE_STARTED"))
		service.mu.Unlock()
	}

	items := requireRecent(t, service, RecentRequest{Limit: 1000}).Items
	if len(items) != 5 {
		t.Fatalf("expected 5 items (clamped to available), got %d", len(items))
	}
}

func TestRecentActivityQueryReportsEvictedBeginningAsFact(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	clock := &fakeClock{t: time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)}
	service := NewServiceWithClock(ctx, clock.now)
	defer service.Close()

	service.mu.Lock()
	service.interval = &interval{instanceID: testInstanceID}
	for i := 1; i <= 3; i++ {
		service.appendActivity(makeActivity(fmt.Sprintf("%d", i), "session-1", "TRACE_STARTED"))
	}
	service.mu.Unlock()

	service.InvalidateTargetScope("scope-1", context.Background())

	service.mu.Lock()
	service.interval.instanceID = testInstanceID
	for i := 4; i <= 6; i++ {
		service.appendActivity(makeActivity(fmt.Sprintf("%d", i), "session-1", "TRACE_STARTED"))
	}
	service.mu.Unlock()

	beginningUnavailable := requireRecent(t, service, RecentRequest{Cursor: "5", Limit: 10}).BeginningUnavailable
	if !beginningUnavailable {
		t.Fatal("expected beginningUnavailable=true when querying after cursor from old interval")
	}
}

func TestSubscribeAtomicallyHandsOffReplayToLiveWithoutLossOrDuplicate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service := NewService(ctx)
	defer service.Close()

	for i := 1; i <= 3; i++ {
		service.mu.Lock()
		service.appendActivity(makeActivity(fmt.Sprintf("%d", i), "session-1", "TRACE_STARTED"))
		service.mu.Unlock()
	}

	ch, _, _, unsub := service.Subscribe()
	defer unsub()

	received := make(map[string]bool)
	for i := 0; i < 3; i++ {
		select {
		case activity := <-ch:
			received[activity.Cursor] = true
		case <-time.After(time.Second):
			t.Fatal("did not receive replayed activity")
		}
	}

	service.mu.Lock()
	service.appendActivity(makeActivity("4", "session-1", "TRACE_STARTED"))
	subs := make([]*subscription, 0, len(service.subscriptions))
	for sub := range service.subscriptions {
		subs = append(subs, sub)
	}
	service.mu.Unlock()
	for _, sub := range subs {
		select {
		case sub.ch <- makeActivity("4", "session-1", "TRACE_STARTED"):
		default:
		}
	}

	select {
	case activity := <-ch:
		received[activity.Cursor] = true
	case <-time.After(time.Second):
		t.Fatal("did not receive live activity")
	}

	if len(received) != 4 {
		t.Fatalf("expected 4 unique activities, got %d: %v", len(received), received)
	}
}

func TestSubscribersMaintainIndependentCursors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service := NewService(ctx)
	defer service.Close()

	activity := makeActivity("1", "session-1", "TRACE_STARTED")

	service.mu.Lock()
	service.appendActivity(activity)
	service.mu.Unlock()

	ch1, _, _, unsub1 := service.Subscribe()
	defer unsub1()
	ch2, _, _, unsub2 := service.Subscribe()
	defer unsub2()

	select {
	case activity := <-ch1:
		if activity.Cursor != "1" {
			t.Fatalf("expected cursor 1, got %s", activity.Cursor)
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber 1 did not receive replay activity")
	}
	select {
	case activity := <-ch2:
		if activity.Cursor != "1" {
			t.Fatalf("expected cursor 1, got %s", activity.Cursor)
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber 2 did not receive replay activity")
	}
}

func TestSlowSubscriberDoesNotBlockWindowOrPeer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service := NewService(ctx)
	defer service.Close()

	_, slowLifecycle, _, _, unsubSlow := service.SubscribeAfter("")
	defer unsubSlow()
	fast, _, _, acknowledgeFast, unsubFast := service.SubscribeAfter("")
	defer unsubFast()
	const total = subscriberMaxFrames + 10
	for i := 1; i <= total; i++ {
		service.mu.Lock()
		activity := makeActivity(fmt.Sprintf("%d", i), "session-1", "TRACE_STARTED")
		service.appendActivity(activity)
		service.publishActivityLocked(activity)
		service.mu.Unlock()
		select {
		case received := <-fast:
			acknowledgeFast(received)
		case <-time.After(time.Second):
			t.Fatal("fast peer was delayed by slow subscriber")
		}
	}

	select {
	case event := <-slowLifecycle:
		if event.Kind != LifecycleSubscriberOverflow {
			t.Fatalf("expected subscriber overflow, got %q", event.Kind)
		}
	case <-time.After(time.Second):
		t.Fatal("slow subscriber did not receive a guaranteed overflow event")
	}
	service.mu.Lock()
	count := len(service.activities)
	service.mu.Unlock()
	if count != total {
		t.Fatalf("expected %d activities in ring, got %d", total, count)
	}
}

func TestActivityValidationRejectsUnknownKind(t *testing.T) {
	activity := Activity{
		InstanceID: testInstanceID,
		Cursor:     "1",
		SessionID:  "session-1",
		TraceID:    "trace-1",
		Timestamp:  time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
		Kind:       ActivityKind("UNKNOWN_KIND"),
		Summary:    "test",
	}
	if err := activity.Validate(); err == nil {
		t.Fatal("expected validation error for unknown kind")
	}
}

func TestActivityValidationRejectsBlankFields(t *testing.T) {
	activity := Activity{
		Kind:      KindTraceStarted,
		Summary:   "test",
		Timestamp: time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
	}
	if err := activity.Validate(); err == nil {
		t.Fatal("expected validation error for blank fields")
	}
}

func TestActivityValidationRejectsZeroTimestamp(t *testing.T) {
	activity := Activity{
		InstanceID: testInstanceID,
		Cursor:     "1",
		SessionID:  "session-1",
		TraceID:    "trace-1",
		Kind:       KindTraceStarted,
		Summary:    "test",
	}
	if err := activity.Validate(); err == nil {
		t.Fatal("expected validation error for zero timestamp")
	}
}

func TestAll18ActivityKindsAreValid(t *testing.T) {
	kinds := []ActivityKind{
		KindTraceStarted, KindFrameOpened, KindFrameClosed,
		KindModelRequestSent, KindModelResponseReceived, KindModelAttemptFailed,
		KindPlanCreated, KindPlanUpdated, KindPlanValidationFailed,
		KindPlanRetryRequested, KindToolCallStarted, KindToolCallCompleted,
		KindToolCallFailed, KindStepStarted, KindStepActionRejected,
		KindStepCompleted, KindErrorRecorded, KindTraceCompleted,
		KindExecutionObservationEnded,
	}
	if len(kinds) != 19 {
		t.Fatalf("expected 19 kinds, got %d", len(kinds))
	}
	for _, kind := range kinds {
		if !IsValidKind(kind) {
			t.Fatalf("kind %s should be valid", kind)
		}
	}
}

func TestKindLabelsCoverAllKinds(t *testing.T) {
	labels := KindLabels()
	if len(labels) != 19 {
		t.Fatalf("expected 19 labels, got %d", len(labels))
	}
	for _, kind := range []ActivityKind{
		KindTraceStarted, KindFrameOpened, KindFrameClosed,
		KindModelRequestSent, KindModelResponseReceived, KindModelAttemptFailed,
		KindPlanCreated, KindPlanUpdated, KindPlanValidationFailed,
		KindPlanRetryRequested, KindToolCallStarted, KindToolCallCompleted,
		KindToolCallFailed, KindStepStarted, KindStepActionRejected,
		KindStepCompleted, KindErrorRecorded, KindTraceCompleted,
		KindExecutionObservationEnded,
	} {
		if labels[kind] == "" {
			t.Fatalf("missing label for kind %s", kind)
		}
	}
	if labels[KindModelAttemptFailed] != "Model attempt failed" {
		t.Fatalf("model attempt failed label = %q", labels[KindModelAttemptFailed])
	}
}

func TestRingByteBoundsEvictOldest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service := NewService(ctx)
	defer service.Close()

	bigDetails := json.RawMessage(`{"data":"` + strings.Repeat("x", 1024) + `"}`)
	for i := 0; i < 100; i++ {
		activity := Activity{
			InstanceID: testInstanceID,
			Cursor:     fmt.Sprintf("cursor-%d", i),
			SessionID:  "session-1",
			TraceID:    "trace-1",
			Timestamp:  time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
			Kind:       KindTraceStarted,
			Summary:    "test with large payload to trigger byte eviction",
			Details:    bigDetails,
		}
		service.mu.Lock()
		service.appendActivity(activity)
		service.mu.Unlock()
	}

	service.mu.Lock()
	count := len(service.activities)
	totalBytes := service.ringBytes
	service.mu.Unlock()

	if totalBytes > ringMaxBytes {
		t.Fatalf("ring bytes %d exceeds limit %d", totalBytes, ringMaxBytes)
	}
	if count > ringMaxCount {
		t.Fatalf("ring count %d exceeds limit %d", count, ringMaxCount)
	}
}

func TestRecentActivityShutdownLeavesNoAdoptableState(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service := NewService(ctx)

	service.mu.Lock()
	service.appendActivity(makeActivity("1", "session-1", "TRACE_STARTED"))
	service.mu.Unlock()

	service.Close()

	items := requireRecent(t, service, RecentRequest{Limit: 100}).Items
	if len(items) != 0 {
		t.Fatalf("expected 0 items after shutdown, got %d", len(items))
	}

	ch, _, _, unsub := service.Subscribe()
	defer unsub()
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected closed channel after shutdown")
		}
	default:
		t.Fatal("expected closed channel after shutdown")
	}
}

func TestSubscriberOverflowDoesNotResetSharedContinuity(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service := NewService(ctx)
	defer service.Close()

	overflowSub, _, _, unsubOverflow := service.Subscribe()
	defer unsubOverflow()
	_ = overflowSub

	for i := 1; i <= subscriberMaxFrames+50; i++ {
		service.mu.Lock()
		activity := makeActivity(fmt.Sprintf("%d", i), "session-1", "TRACE_STARTED")
		service.appendActivity(activity)
		service.publishActivityLocked(activity)
		service.mu.Unlock()
	}

	service.mu.Lock()
	hasReset := service.interval != nil && service.interval.reset != nil
	count := len(service.activities)
	service.mu.Unlock()

	if hasReset {
		t.Fatal("shared interval should not have reset from subscriber overflow")
	}
	if count == 0 {
		t.Fatal("ring should still have activities")
	}
}

func TestOverflowNotificationSurvivesSaturatedLifecycleQueue(t *testing.T) {
	service := NewService(context.Background())
	defer service.Close()
	_, lifecycle, _, _, unsubscribe := service.SubscribeAfter("")
	defer unsubscribe()

	service.mu.Lock()
	var selected *subscription
	for sub := range service.subscriptions {
		selected = sub
	}
	if selected == nil {
		service.mu.Unlock()
		t.Fatal("subscription was not registered")
	}
	for i := 0; i < cap(selected.lifecycleCh)*2; i++ {
		service.enqueueLifecycleLocked(
			selected,
			LifecycleEvent{Kind: LifecycleConnectionChanged},
			false,
		)
	}
	service.overflowSubscriberLocked(selected)
	service.mu.Unlock()

	foundOverflow := false
	for {
		select {
		case event := <-lifecycle:
			if event.Kind == LifecycleSubscriberOverflow {
				foundOverflow = true
			}
		default:
			if !foundOverflow {
				t.Fatal("saturated lifecycle queue dropped subscriber overflow")
			}
			return
		}
	}
}

func TestContinuityReportsInstanceAndCursors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service := NewService(ctx)
	defer service.Close()

	service.mu.Lock()
	service.interval = &interval{
		instanceID:  testInstanceID,
		firstCursor: "5",
		lastCursor:  "10",
		observedAt:  time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
	}
	service.mu.Unlock()

	continuity := service.continuityLocked()
	if continuity == nil {
		t.Fatal("expected continuity")
	}
	if continuity.InstanceID != testInstanceID {
		t.Fatalf("expected instanceID %s, got %s", testInstanceID, continuity.InstanceID)
	}
	if continuity.FirstCursor != "5" || continuity.LastCursor != "10" {
		t.Fatalf("unexpected cursors: first=%s last=%s", continuity.FirstCursor, continuity.LastCursor)
	}
}

func TestContinuityIntervalIdentityChangesOnlyOnReset(t *testing.T) {
	service := NewService(context.Background())
	defer service.Close()

	service.mu.Lock()
	service.resetLocked(ResetTargetScopeChanged)
	first := service.continuityLocked()
	service.interval.instanceID = testInstanceID
	service.appendActivity(makeActivity("1", "session-1", string(KindTraceStarted)))
	same := service.continuityLocked()
	service.resetLocked(ResetUpstreamStaleCursor)
	second := service.continuityLocked()
	service.mu.Unlock()

	if first.IntervalID == "" || same.IntervalID != first.IntervalID {
		t.Fatalf("activity changed interval identity: first=%q same=%q", first.IntervalID, same.IntervalID)
	}
	if second.IntervalID == first.IntervalID {
		t.Fatalf("reset retained interval identity %q", first.IntervalID)
	}
}

func TestSubscribeSnapshotReturnsContinuityAndReplayFromSameInterval(t *testing.T) {
	service := NewService(context.Background())
	defer service.Close()

	service.mu.Lock()
	service.resetLocked(ResetTargetScopeChanged)
	service.interval.instanceID = testInstanceID
	service.appendActivity(makeActivity("1", "session-1", string(KindTraceStarted)))
	service.mu.Unlock()

	_, continuity, activities, _, overflow, _, unsubscribe :=
		service.SubscribeSnapshotAfter("")
	defer unsubscribe()

	if overflow {
		t.Fatal("unexpected replay overflow")
	}
	if continuity == nil || continuity.IntervalID == "" || continuity.LastCursor != "1" {
		t.Fatalf("unexpected continuity snapshot: %#v", continuity)
	}
	select {
	case activity := <-activities:
		if activity.Cursor != continuity.LastCursor {
			t.Fatalf("replay cursor %q does not match continuity %q", activity.Cursor, continuity.LastCursor)
		}
	case <-time.After(time.Second):
		t.Fatal("missing replay activity")
	}
}

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (fc *fakeClock) now() time.Time {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	return fc.t
}
