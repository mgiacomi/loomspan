package mcpadapter

import (
	"context"
	"fmt"
	"sync"
)

type Tracker struct {
	mu        sync.Mutex
	changed   *sync.Cond
	frozen    bool
	permanent bool
	nextID    uint64
	active    map[uint64]context.CancelFunc
}

func NewTracker() *Tracker {
	tracker := &Tracker{active: make(map[uint64]context.CancelFunc)}
	tracker.changed = sync.NewCond(&tracker.mu)
	return tracker
}

func (tracker *Tracker) Admit(parent context.Context, generation uint64) (context.Context, func(), error) {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	if tracker.frozen || tracker.permanent {
		return nil, nil, fmt.Errorf("MCP admission is frozen")
	}
	ctx, cancel := context.WithCancel(parent)
	tracker.nextID++
	id := tracker.nextID
	tracker.active[id] = cancel
	var once sync.Once
	done := func() {
		once.Do(func() {
			cancel()
			tracker.mu.Lock()
			delete(tracker.active, id)
			tracker.changed.Broadcast()
			tracker.mu.Unlock()
		})
	}
	return context.WithValue(ctx, generationKey{}, generation), done, nil
}

type generationKey struct{}

func admittedGeneration(ctx context.Context) (uint64, bool) {
	generation, ok := ctx.Value(generationKey{}).(uint64)
	return generation, ok
}

func (tracker *Tracker) Freeze(ctx context.Context, permanent bool, closeSessions func()) error {
	tracker.beginFreeze(permanent, closeSessions)
	return tracker.waitForDrain(ctx)
}

func (tracker *Tracker) beginFreeze(permanent bool, closeSessions func()) {
	tracker.mu.Lock()
	tracker.frozen = true
	if permanent {
		tracker.permanent = true
	}
	cancellations := make([]context.CancelFunc, 0, len(tracker.active))
	for _, cancel := range tracker.active {
		cancellations = append(cancellations, cancel)
	}
	tracker.mu.Unlock()
	for _, cancel := range cancellations {
		cancel()
	}
	if closeSessions != nil {
		closeSessions()
	}

}

func (tracker *Tracker) waitForDrain(ctx context.Context) error {
	stop := context.AfterFunc(ctx, func() {
		tracker.mu.Lock()
		tracker.changed.Broadcast()
		tracker.mu.Unlock()
	})
	defer stop()
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	for len(tracker.active) != 0 && ctx.Err() == nil {
		tracker.changed.Wait()
	}
	return ctx.Err()
}

func (tracker *Tracker) Reopen() error {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	if tracker.permanent {
		return fmt.Errorf("MCP admission is permanently closed")
	}
	tracker.frozen = false
	return nil
}

func (tracker *Tracker) Frozen() bool {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	return tracker.frozen || tracker.permanent
}
