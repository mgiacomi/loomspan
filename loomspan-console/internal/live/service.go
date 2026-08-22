package live

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math/rand/v2"
	"strconv"
	"sync"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/applicationclient"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/observability"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/target"
)

const (
	ringMaxCount        = 2048
	ringMaxBytes        = 8 * 1024 * 1024
	subscriberMaxFrames = 256
	subscriberMaxBytes  = 1024 * 1024
	recentDefaultLimit  = 100
	recentMaxLimit      = 256
	reconnectBaseDelay  = time.Second
	reconnectMaxDelay   = 30 * time.Second
	baselineInterval    = 30 * time.Second
)

type subscription struct {
	ch           chan Activity
	lifecycleCh  chan LifecycleEvent
	overflow     bool
	terminated   bool
	pendingBytes int
}

type LifecycleEvent struct {
	Kind       string
	Connection *ConnectionFact
	ObservedAt *time.Time
}

const (
	LifecycleBaselineRefreshed  = "baseline_refreshed"
	LifecycleTargetChanged      = "target_changed"
	LifecycleSubscriberOverflow = "subscriber_overflow"
	LifecycleConnectionChanged  = "connection_changed"
)

type interval struct {
	id          string
	instanceID  string
	firstCursor string
	lastCursor  string
	observedAt  time.Time
	reset       *ResetFact
}

type Baseline struct {
	Executions   []observability.ActiveExecution
	ResumeCursor string
	ObservedAt   time.Time
}

type Service struct {
	mu                         sync.Mutex
	scope                      *target.Scope
	stream                     *applicationclient.ActivityStream
	cancel                     context.CancelFunc
	activities                 []Activity
	ringBytes                  int
	lastCursor                 string
	seenCursors                map[string][]byte
	closed                     bool
	liveUnavailable            bool
	globalEvictedThroughCursor string
	sessionCoverage            map[string]sessionCoverage
	subscriptions              map[*subscription]struct{}
	parentCtx                  context.Context
	interval                   *interval
	now                        func() time.Time
	ticker                     func(time.Duration, func()) tickerHandle
	jitter                     func(time.Duration) time.Duration
	baselineLoader             func(context.Context, target.Scope) (Baseline, *consolecore.Error)
	baseline                   Baseline
	connection                 ConnectionFact
	nextIntervalID             uint64
}

type sessionCoverage struct {
	startCursor   string
	evictedCursor string
}

type tickerHandle interface {
	Stop() bool
	C() <-chan time.Time
}

type realTicker struct {
	t *time.Ticker
}

func (r *realTicker) Stop() bool          { r.t.Stop(); return true }
func (r *realTicker) C() <-chan time.Time { return r.t.C }

func NewService(parentCtx context.Context) *Service {
	s := &Service{
		activities:      make([]Activity, 0, ringMaxCount),
		seenCursors:     make(map[string][]byte),
		sessionCoverage: make(map[string]sessionCoverage),
		subscriptions:   make(map[*subscription]struct{}),
		parentCtx:       parentCtx,
		now:             time.Now,
		ticker: func(d time.Duration, _ func()) tickerHandle {
			t := time.NewTicker(d)
			return &realTicker{t: t}
		},
		jitter: func(d time.Duration) time.Duration {
			minimum := d * 8 / 10
			window := d * 4 / 10
			return minimum + time.Duration(rand.Int64N(int64(window)+1))
		},
	}
	return s
}

func NewServiceWithClock(parentCtx context.Context, now func() time.Time) *Service {
	s := NewService(parentCtx)
	s.now = now
	return s
}

func NewServiceWithTesting(parentCtx context.Context, now func() time.Time, ticker func(time.Duration, func()) tickerHandle, jitter func(time.Duration) time.Duration) *Service {
	s := NewService(parentCtx)
	s.now = now
	s.ticker = ticker
	s.jitter = jitter
	return s
}

func (service *Service) InvalidateTargetScope(previous target.ScopeID, cancelled context.Context) {
	service.mu.Lock()
	for sub := range service.subscriptions {
		service.enqueueLifecycleLocked(sub, LifecycleEvent{Kind: LifecycleTargetChanged}, true)
	}
	service.stopWorkerLocked()
	service.resetLocked(ResetTargetScopeChanged)
	service.mu.Unlock()
}

func (service *Service) resetLocked(cause ResetCause) {
	var resetFact *ResetFact
	if service.interval != nil {
		resetFact = &ResetFact{
			Cause:     cause,
			Timestamp: service.now(),
			Cursor:    service.interval.lastCursor,
		}
	}
	service.activities = make([]Activity, 0, ringMaxCount)
	service.ringBytes = 0
	service.lastCursor = ""
	service.seenCursors = make(map[string][]byte)
	service.globalEvictedThroughCursor = ""
	service.sessionCoverage = make(map[string]sessionCoverage)
	service.nextIntervalID++
	service.interval = &interval{
		id:    strconv.FormatUint(service.nextIntervalID, 10),
		reset: resetFact,
	}
	service.baseline = Baseline{}
	for sub := range service.subscriptions {
		close(sub.ch)
		close(sub.lifecycleCh)
		delete(service.subscriptions, sub)
	}
}

func (service *Service) stopWorkerLocked() {
	if service.cancel != nil {
		service.cancel()
		service.cancel = nil
	}
	if service.stream != nil {
		_ = service.stream.Close()
		service.stream = nil
	}
	service.scope = nil
}

func (service *Service) ActivateActivity(scope target.Scope) {
	service.mu.Lock()
	if service.closed {
		service.mu.Unlock()
		return
	}
	scopeCopy := scope
	service.stopWorkerLocked()
	service.resetLocked(ResetTargetScopeChanged)
	service.scope = &scopeCopy
	service.lastCursor = "0"
	service.liveUnavailable = false
	service.connection = ConnectionFact{Connected: false, Reason: "connecting", At: service.now()}
	ctx, cancel := context.WithCancel(service.parentCtx)
	service.cancel = cancel
	service.mu.Unlock()
	go service.run(ctx, scopeCopy)
	go service.runBaselineRefresh(ctx, scopeCopy)
}

func (service *Service) run(ctx context.Context, scope target.Scope) {
	backoff := reconnectBaseDelay
	if cursor, domain := service.refreshBaseline(ctx, scope); domain == nil {
		service.mu.Lock()
		if !service.scopeCurrentLocked(scope.ID) {
			service.mu.Unlock()
			return
		}
		service.lastCursor = cursor
		service.mu.Unlock()
	} else if !isRetryable(domain.Code) {
		return
	}
	for {
		if err := ctx.Err(); err != nil {
			return
		}
		service.mu.Lock()
		afterCursor := service.lastCursor
		service.mu.Unlock()
		stream, domain := scope.OpenActivity(ctx, afterCursor)
		if domain != nil {
			slog.Error("live activity stream open failed", "scopeId", scope.ID, "error", domain.Code)
			if !service.publishConnectionFor(scope.ID, false, string(domain.Code)) {
				return
			}
			if domain.Code == consolecore.CodeLiveMonitoringUnavailable {
				service.mu.Lock()
				if !service.scopeCurrentLocked(scope.ID) {
					service.mu.Unlock()
					return
				}
				service.liveUnavailable = true
				service.mu.Unlock()
				return
			}
			if domain.Code == consolecore.CodeStaleCursor {
				service.mu.Lock()
				if !service.scopeCurrentLocked(scope.ID) {
					service.mu.Unlock()
					return
				}
				service.resetLocked(ResetUpstreamStaleCursor)
				service.scope = &scope
				service.mu.Unlock()
				cursor, baselineDomain := service.refreshBaseline(ctx, scope)
				if baselineDomain == nil {
					service.mu.Lock()
					if !service.scopeCurrentLocked(scope.ID) {
						service.mu.Unlock()
						return
					}
					service.lastCursor = cursor
					service.mu.Unlock()
				}
				delay := service.jitter(backoff)
				select {
				case <-ctx.Done():
					return
				case <-time.After(delay):
					backoff = min(backoff*2, reconnectMaxDelay)
					continue
				}
			}
			if !isRetryable(domain.Code) {
				return
			}
			delay := service.jitter(backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
				backoff = min(backoff*2, reconnectMaxDelay)
				continue
			}
		}
		backoff = reconnectBaseDelay
		service.mu.Lock()
		if !service.scopeCurrentLocked(scope.ID) {
			service.mu.Unlock()
			_ = stream.Close()
			return
		}
		service.stream = stream
		service.mu.Unlock()
		if !service.publishConnectionFor(scope.ID, true, "") {
			_ = stream.Close()
			return
		}
		resetCause := service.consume(ctx, stream, scope)
		_ = stream.Close()
		service.mu.Lock()
		if service.stream == stream {
			service.stream = nil
		}
		service.mu.Unlock()
		if err := ctx.Err(); err != nil {
			return
		}
		if !service.publishConnectionFor(scope.ID, false, "upstream_disconnected") {
			return
		}
		if resetCause != "" {
			service.mu.Lock()
			if !service.scopeCurrentLocked(scope.ID) {
				service.mu.Unlock()
				return
			}
			service.resetLocked(resetCause)
			service.scope = &scope
			service.mu.Unlock()
			if resetCause == ResetInstanceChanged {
				if domain := scope.RevalidateInstance(ctx); domain != nil {
					return
				}
			}
			cursor, domain := service.refreshBaseline(ctx, scope)
			if domain == nil {
				service.mu.Lock()
				if !service.scopeCurrentLocked(scope.ID) {
					service.mu.Unlock()
					return
				}
				service.lastCursor = cursor
				service.mu.Unlock()
			}
		}
		delay := service.jitter(backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
			backoff = min(backoff*2, reconnectMaxDelay)
		}
	}
}

func (service *Service) runBaselineRefresh(ctx context.Context, scope target.Scope) {
	ticker := service.ticker(baselineInterval, func() {})
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C():
			if ctx.Err() != nil {
				return
			}
			service.mu.Lock()
			unavailable := service.liveUnavailable
			service.mu.Unlock()
			if unavailable {
				return
			}
			_, _ = service.refreshBaseline(ctx, scope)
		}
	}
}

func (service *Service) LiveUnavailable() bool {
	service.mu.Lock()
	defer service.mu.Unlock()
	return service.liveUnavailable
}

func (service *Service) Connection() ConnectionFact {
	service.mu.Lock()
	defer service.mu.Unlock()
	return service.connection
}

func (service *Service) publishConnectionFor(scopeID target.ScopeID, connected bool, reason string) bool {
	service.mu.Lock()
	if !service.scopeCurrentLocked(scopeID) {
		service.mu.Unlock()
		return false
	}
	fact := ConnectionFact{Connected: connected, Reason: reason, At: service.now()}
	service.connection = fact
	for sub := range service.subscriptions {
		copy := fact
		service.enqueueLifecycleLocked(sub, LifecycleEvent{
			Kind:       LifecycleConnectionChanged,
			Connection: &copy,
		}, false)
	}
	service.mu.Unlock()
	return true
}

func (service *Service) Continuity() *Continuity {
	service.mu.Lock()
	defer service.mu.Unlock()
	return service.continuityLocked()
}

func (service *Service) SetBaselineLoader(fn func(context.Context, target.Scope) (Baseline, *consolecore.Error)) {
	service.mu.Lock()
	service.baselineLoader = fn
	service.mu.Unlock()
}

func (service *Service) refreshBaseline(ctx context.Context, scope target.Scope) (string, *consolecore.Error) {
	service.mu.Lock()
	if !service.scopeCurrentLocked(scope.ID) {
		service.mu.Unlock()
		return "", consolecore.NewError(consolecore.CodeTargetChanged, "The selected target changed. Start this operation again.", string(scope.ID), consolecore.Details{}, nil)
	}
	loader := service.baselineLoader
	service.mu.Unlock()
	if loader == nil {
		service.mu.Lock()
		observedAt := service.now()
		service.notifyBaselineLocked(observedAt)
		service.mu.Unlock()
		return "0", nil
	}
	baseline, domain := loader(ctx, scope)
	if domain == nil {
		cursor := baseline.ResumeCursor
		if cursor == "" {
			cursor = "0"
		}
		baseline.ResumeCursor = cursor
		service.mu.Lock()
		if !service.scopeCurrentLocked(scope.ID) || ctx.Err() != nil {
			service.mu.Unlock()
			return "", consolecore.NewError(consolecore.CodeTargetChanged, "The selected target changed. Start this operation again.", string(scope.ID), consolecore.Details{}, ctx.Err())
		}
		service.baseline = baseline
		service.notifyBaselineLocked(baseline.ObservedAt)
		service.mu.Unlock()
		return cursor, nil
	}
	return "", domain
}

func (service *Service) notifyBaselineLocked(observedAt time.Time) {
	if observedAt.IsZero() {
		observedAt = service.now()
	}
	for sub := range service.subscriptions {
		value := observedAt
		service.enqueueLifecycleLocked(sub, LifecycleEvent{
			Kind:       LifecycleBaselineRefreshed,
			ObservedAt: &value,
		}, false)
	}
}

func (service *Service) Baseline() Baseline {
	service.mu.Lock()
	defer service.mu.Unlock()
	result := service.baseline
	result.Executions = append([]observability.ActiveExecution(nil), service.baseline.Executions...)
	return result
}

func (service *Service) consume(ctx context.Context, stream *applicationclient.ActivityStream, scope target.Scope) ResetCause {
	for {
		frame, err := stream.Next()
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
				return ""
			}
			var mismatch *applicationclient.InstanceMismatch
			if errors.As(err, &mismatch) {
				return ResetInstanceChanged
			}
			slog.Error("live activity stream read error", "error", err)
			return ""
		}
		if frame.Event == "handshake" {
			var hs Handshake
			if err := json.Unmarshal(frame.Data, &hs); err != nil {
				slog.Error("live activity handshake parse error", "error", err)
				return ""
			}
			service.mu.Lock()
			if !service.scopeCurrentLocked(scope.ID) {
				service.mu.Unlock()
				return ""
			}
			if service.interval == nil {
				service.interval = &interval{}
			}
			if service.interval.instanceID != "" && service.interval.instanceID != hs.InstanceID {
				service.mu.Unlock()
				return ResetInstanceChanged
			}
			service.interval.instanceID = hs.InstanceID
			service.interval.observedAt = hs.ObservedAt
			service.lastCursor = hs.AfterCursor
			service.mu.Unlock()
			continue
		}
		if frame.Event != "activity" {
			continue
		}
		var activity Activity
		if err := json.Unmarshal(frame.Data, &activity); err != nil {
			slog.Error("live activity parse error", "error", err)
			return ""
		}
		if err := activity.Validate(); err != nil {
			slog.Error("live activity validation error", "error", err, "cursor", activity.Cursor)
			return ""
		}
		service.mu.Lock()
		if !service.scopeCurrentLocked(scope.ID) {
			service.mu.Unlock()
			return ""
		}
		if service.interval != nil && service.interval.instanceID != "" && service.interval.instanceID != activity.InstanceID {
			service.mu.Unlock()
			return ResetInstanceChanged
		}
		if service.interval == nil {
			service.interval = &interval{}
		}
		service.interval.instanceID = activity.InstanceID
		appended, err := service.acceptActivity(activity)
		if err != nil {
			service.mu.Unlock()
			slog.Error("live activity cursor protocol error", "error", err, "cursor", activity.Cursor)
			return ""
		}
		if !appended {
			service.mu.Unlock()
			continue
		}
		if activity.Kind == KindTraceCompleted || activity.Kind == KindExecutionObservationEnded {
			for index := range service.baseline.Executions {
				if service.baseline.Executions[index].SessionID == activity.SessionID {
					service.baseline.Executions = append(service.baseline.Executions[:index], service.baseline.Executions[index+1:]...)
					break
				}
			}
		}
		service.publishActivityLocked(activity)
		service.mu.Unlock()
	}
}

func (service *Service) publishActivityLocked(activity Activity) {
	for sub := range service.subscriptions {
		if sub.terminated {
			continue
		}
		size := activity.EncodedSize()
		if len(sub.ch) >= subscriberMaxFrames || sub.pendingBytes+size > subscriberMaxBytes {
			service.overflowSubscriberLocked(sub)
			continue
		}
		select {
		case sub.ch <- activity:
			sub.pendingBytes += size
		default:
			service.overflowSubscriberLocked(sub)
		}
	}
}

func (service *Service) enqueueLifecycleLocked(sub *subscription, event LifecycleEvent, terminal bool) {
	if sub.terminated && !terminal {
		return
	}
	if terminal {
		for len(sub.lifecycleCh) >= cap(sub.lifecycleCh) {
			<-sub.lifecycleCh
		}
		sub.lifecycleCh <- event
		return
	}
	if len(sub.lifecycleCh) >= cap(sub.lifecycleCh)-1 {
		return
	}
	sub.lifecycleCh <- event
}

func (service *Service) overflowSubscriberLocked(sub *subscription) {
	if sub.terminated {
		return
	}
	sub.overflow = true
	sub.terminated = true
	service.enqueueLifecycleLocked(sub, LifecycleEvent{Kind: LifecycleSubscriberOverflow}, true)
}

func (service *Service) scopeCurrentLocked(scopeID target.ScopeID) bool {
	return !service.closed && service.scope != nil && service.scope.ID == scopeID
}

func (service *Service) appendActivity(activity Activity) bool {
	appended, err := service.acceptActivity(activity)
	return err == nil && appended
}

func (service *Service) acceptActivity(activity Activity) (bool, error) {
	encoded, err := json.Marshal(activity)
	if err != nil {
		return false, err
	}
	if previous, exists := service.seenCursors[activity.Cursor]; exists {
		if string(previous) != string(encoded) {
			return false, errors.New("activity cursor was reused with different content")
		}
		return false, nil
	}
	cursor, err := strconv.ParseUint(activity.Cursor, 10, 64)
	if err != nil || cursor == 0 {
		return false, errors.New("activity cursor must be a positive decimal integer")
	}
	previousCursor := service.lastCursor
	if previousCursor == "" && service.interval != nil {
		previousCursor = service.interval.lastCursor
	}
	if previousCursor != "" {
		previous, parseErr := strconv.ParseUint(previousCursor, 10, 64)
		if parseErr != nil || cursor <= previous {
			return false, errors.New("activity cursor regressed")
		}
	}
	size := activity.EncodedSize()
	if size > maxActivityUTF8Bytes {
		slog.Error("live activity exceeds max size", "cursor", activity.Cursor, "size", size)
		return false, errors.New("activity exceeds maximum encoded size")
	}
	evictedAny := false
	for len(service.activities) >= ringMaxCount || service.ringBytes+size > ringMaxBytes {
		if len(service.activities) == 0 {
			break
		}
		evictedActivity := service.activities[0]
		service.ringBytes -= evictedActivity.EncodedSize()
		delete(service.seenCursors, evictedActivity.Cursor)
		service.activities = service.activities[1:]
		service.globalEvictedThroughCursor = evictedActivity.Cursor
		coverage := service.sessionCoverage[evictedActivity.SessionID]
		coverage.evictedCursor = evictedActivity.Cursor
		service.sessionCoverage[evictedActivity.SessionID] = coverage
		evictedAny = true
	}
	service.activities = append(service.activities, activity)
	if activity.Kind == KindTraceStarted {
		coverage := service.sessionCoverage[activity.SessionID]
		if coverage.startCursor == "" {
			coverage.startCursor = activity.Cursor
		}
		service.sessionCoverage[activity.SessionID] = coverage
	}
	if evictedAny {
		service.pruneSessionCoverageLocked()
	}
	service.ringBytes += size
	service.seenCursors[activity.Cursor] = encoded
	service.lastCursor = activity.Cursor
	if service.interval != nil {
		if service.interval.firstCursor == "" {
			service.interval.firstCursor = activity.Cursor
		}
		service.interval.lastCursor = activity.Cursor
	}
	return true, nil
}

func (service *Service) Recent(request RecentRequest) (RecentResponse, *consolecore.Error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	response := RecentResponse{ObservedAt: service.now().UTC(), Items: []Activity{}}
	if service.liveUnavailable {
		scopeID := ""
		if service.scope != nil {
			scopeID = string(service.scope.ID)
		}
		return RecentResponse{}, consolecore.NewError(
			consolecore.CodeLiveMonitoringUnavailable,
			"Live monitoring is unavailable.",
			scopeID,
			consolecore.Details{},
			nil,
		)
	}
	cursor, sessionID, limit := request.Cursor, request.SessionID, request.Limit
	if limit <= 0 {
		limit = recentDefaultLimit
	}
	if limit > recentMaxLimit {
		limit = recentMaxLimit
	}
	var start int
	if cursor == "" && len(service.activities) > limit {
		start = len(service.activities) - limit
	}
	if cursor != "" {
		found := false
		for i := len(service.activities) - 1; i >= 0; i-- {
			if service.activities[i].Cursor == cursor {
				start = i + 1
				found = true
				break
			}
		}
		if !found {
			response.Continuity = service.continuityLocked()
			response.Coverage = service.coverageLocked(sessionID)
			return response, nil
		}
	}
	items := make([]Activity, 0)
	var hasMore bool
	var nextCursor string
	if sessionID != "" {
		if cursor == "" {
			matches := 0
			for index := len(service.activities) - 1; index >= 0; index-- {
				if service.activities[index].SessionID == sessionID {
					matches++
					if matches == limit {
						start = index
						break
					}
				}
			}
		}
		for i := start; i < len(service.activities); i++ {
			if service.activities[i].SessionID != sessionID {
				continue
			}
			if len(items) >= limit {
				hasMore = true
				break
			}
			items = append(items, service.activities[i])
		}
		if len(items) > 0 && hasMore {
			nextCursor = items[len(items)-1].Cursor
		}
	} else {
		end := start + limit
		if end > len(service.activities) {
			end = len(service.activities)
		} else {
			hasMore = end < len(service.activities)
		}
		items = make([]Activity, 0, end-start)
		for i := start; i < end; i++ {
			items = append(items, service.activities[i])
		}
		if hasMore && len(items) > 0 {
			nextCursor = items[len(items)-1].Cursor
		}
	}
	response.Items = items
	response.HasMore = hasMore
	response.NextCursor = nextCursor
	response.Continuity = service.continuityLocked()
	response.Coverage = service.coverageLocked(sessionID)
	return response, nil
}

func (service *Service) coverageLocked(sessionID string) Coverage {
	coverage := Coverage{GlobalEvictedThroughCursor: service.globalEvictedThroughCursor}
	if sessionID == "" {
		return coverage
	}
	tracked := service.sessionCoverage[sessionID]
	coverage.SessionStartCursor = tracked.startCursor
	coverage.SessionEvictedThroughCursor = tracked.evictedCursor
	for _, activity := range service.activities {
		if activity.SessionID != sessionID {
			continue
		}
		if coverage.SessionRetainedCursorRange == nil {
			coverage.SessionRetainedCursorRange = &CursorRange{FirstCursor: activity.Cursor}
		}
		coverage.SessionRetainedCursorRange.LastCursor = activity.Cursor
	}
	return coverage
}

func (service *Service) pruneSessionCoverageLocked() {
	retained := make(map[string]struct{})
	for _, activity := range service.activities {
		retained[activity.SessionID] = struct{}{}
	}
	for sessionID := range service.sessionCoverage {
		if _, ok := retained[sessionID]; !ok {
			delete(service.sessionCoverage, sessionID)
		}
	}
}

func (service *Service) continuityLocked() *Continuity {
	if service.interval == nil {
		return nil
	}
	scopeID := ""
	if service.scope != nil {
		scopeID = string(service.scope.ID)
	}
	return &Continuity{
		IntervalID:    service.interval.id,
		TargetScopeID: scopeID,
		InstanceID:    service.interval.instanceID,
		FirstCursor:   service.interval.firstCursor,
		LastCursor:    service.interval.lastCursor,
		ObservedAt:    service.interval.observedAt,
		Reset:         service.interval.reset,
	}
}

func (service *Service) Subscribe() (<-chan Activity, <-chan LifecycleEvent, bool, func()) {
	ch, lifecycleCh, overflow, _, unsubscribe := service.SubscribeAfter("")
	return ch, lifecycleCh, overflow, unsubscribe
}

func (service *Service) SubscribeAfter(afterCursor string) (<-chan Activity, <-chan LifecycleEvent, bool, func(Activity), func()) {
	service.mu.Lock()
	defer service.mu.Unlock()
	return service.subscribeAfterLocked(afterCursor)
}

func (service *Service) SubscribeSnapshotAfter(afterCursor string) (ConnectionFact, *Continuity, <-chan Activity, <-chan LifecycleEvent, bool, func(Activity), func()) {
	service.mu.Lock()
	defer service.mu.Unlock()
	connection := service.connection
	continuity := service.continuityLocked()
	ch, lifecycleCh, overflow, acknowledge, unsubscribe := service.subscribeAfterLocked(afterCursor)
	return connection, continuity, ch, lifecycleCh, overflow, acknowledge, unsubscribe
}

func (service *Service) subscribeAfterLocked(afterCursor string) (<-chan Activity, <-chan LifecycleEvent, bool, func(Activity), func()) {
	sub := &subscription{
		ch:          make(chan Activity, subscriberMaxFrames),
		lifecycleCh: make(chan LifecycleEvent, 16),
	}
	if service.closed {
		close(sub.ch)
		close(sub.lifecycleCh)
		return sub.ch, sub.lifecycleCh, false, func(Activity) {}, func() {}
	}
	start := 0
	if afterCursor != "" {
		start = -1
		for index := len(service.activities) - 1; index >= 0; index-- {
			if service.activities[index].Cursor == afterCursor {
				start = index + 1
				break
			}
		}
		if start < 0 {
			start = len(service.activities)
			sub.overflow = true
		}
	}
	replay := service.activities[start:]
	for len(replay) > 0 {
		totalBytes := 0
		fits := true
		for _, activity := range replay {
			totalBytes += activity.EncodedSize()
			if totalBytes > subscriberMaxBytes {
				fits = false
				break
			}
		}
		if len(replay) <= subscriberMaxFrames && fits {
			break
		}
		replay = replay[1:]
		sub.overflow = true
	}
	for _, activity := range replay {
		select {
		case sub.ch <- activity:
			sub.pendingBytes += activity.EncodedSize()
		default:
			sub.overflow = true
			break
		}
		if sub.overflow {
			break
		}
	}
	service.subscriptions[sub] = struct{}{}
	acknowledge := func(activity Activity) {
		service.mu.Lock()
		sub.pendingBytes -= activity.EncodedSize()
		if sub.pendingBytes < 0 {
			sub.pendingBytes = 0
		}
		service.mu.Unlock()
	}
	return sub.ch, sub.lifecycleCh, sub.overflow, acknowledge, func() {
		service.mu.Lock()
		if _, ok := service.subscriptions[sub]; ok {
			delete(service.subscriptions, sub)
			close(sub.ch)
			close(sub.lifecycleCh)
		}
		service.mu.Unlock()
	}
}

func (service *Service) Close() {
	service.mu.Lock()
	if service.closed {
		service.mu.Unlock()
		return
	}
	service.closed = true
	service.stopWorkerLocked()
	service.resetLocked(ResetShutdown)
	service.mu.Unlock()
}

func isRetryable(code consolecore.Code) bool {
	switch code {
	case consolecore.CodeTargetUnavailable, consolecore.CodeLimitExceeded:
		return true
	default:
		return false
	}
}
