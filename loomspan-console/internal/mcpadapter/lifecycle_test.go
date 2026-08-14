package mcpadapter

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/mcpcredential"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/profile"
)

func TestRegenerateCancelsAndDrainsBeforePublishingNewCredential(t *testing.T) {
	owned, err := profile.Open(filepath.Join(t.TempDir(), "profile", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	defer owned.Close()
	entropy := make([]byte, 96)
	for index := range entropy {
		entropy[index] = byte(index)
	}
	store, err := mcpcredential.Open(owned.Directory, bytes.NewReader(entropy))
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := store.Prepare()
	if err != nil {
		t.Fatal(err)
	}
	oldKey, err := store.CommitEnable(prepared)
	if err != nil {
		t.Fatal(err)
	}
	tracker := NewTracker()
	request, done, err := tracker.Admit(context.Background(), store.Snapshot().Generation)
	if err != nil {
		t.Fatal(err)
	}
	lifecycle := NewLifecycle(store, tracker, nil)
	result := make(chan struct {
		key string
		err error
	}, 1)
	go func() {
		key, regenerateErr := lifecycle.Regenerate(context.Background())
		result <- struct {
			key string
			err error
		}{key, regenerateErr}
	}()
	select {
	case <-request.Done():
	case <-time.After(time.Second):
		t.Fatal("regeneration did not cancel admitted work")
	}
	if _, ok := store.Authenticate(oldKey); !ok {
		t.Fatal("old credential stopped authenticating before admitted work drained")
	}
	select {
	case <-result:
		t.Fatal("regeneration published before admitted work drained")
	default:
	}
	done()
	completed := <-result
	if completed.err != nil || completed.key == "" || completed.key == oldKey {
		t.Fatalf("regeneration = key-present:%t changed:%t err:%v", completed.key != "", completed.key != oldKey, completed.err)
	}
	if _, ok := store.Authenticate(oldKey); ok {
		t.Fatal("old credential authenticated after regeneration commit")
	}
}

func TestShutdownPermanentlyFreezesButPreservesCredential(t *testing.T) {
	owned, err := profile.Open(filepath.Join(t.TempDir(), "profile", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	defer owned.Close()
	store, err := mcpcredential.Open(owned.Directory, bytes.NewReader(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	prepared, _ := store.Prepare()
	key, err := store.CommitEnable(prepared)
	if err != nil {
		t.Fatal(err)
	}
	tracker := NewTracker()
	lifecycle := NewLifecycle(store, tracker, nil)
	if err := lifecycle.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := tracker.Admit(context.Background(), store.Snapshot().Generation); err == nil {
		t.Fatal("shutdown tracker admitted new work")
	}
	reopened, err := mcpcredential.Open(owned.Directory, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reopened.Authenticate(key); !ok {
		t.Fatal("shutdown removed or changed persistent credential")
	}
}

func TestCancelledManagementRequestCannotStrandAdmission(t *testing.T) {
	owned, err := profile.Open(filepath.Join(t.TempDir(), "profile", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	defer owned.Close()
	store, err := mcpcredential.Open(owned.Directory, bytes.NewReader(make([]byte, 64)))
	if err != nil {
		t.Fatal(err)
	}
	prepared, _ := store.Prepare()
	if _, err := store.CommitEnable(prepared); err != nil {
		t.Fatal(err)
	}
	tracker := NewTracker()
	active, done, err := tracker.Admit(context.Background(), store.Snapshot().Generation)
	if err != nil {
		t.Fatal(err)
	}
	lifecycle := NewLifecycle(store, tracker, nil)
	management, cancelManagement := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, regenerateErr := lifecycle.Regenerate(management)
		result <- regenerateErr
	}()
	select {
	case <-active.Done():
	case <-time.After(time.Second):
		t.Fatal("mutation did not begin draining active request")
	}
	cancelManagement()
	done()
	if err := <-result; err != nil {
		t.Fatalf("internally owned mutation failed after browser cancellation: %v", err)
	}
	_, admittedDone, err := tracker.Admit(context.Background(), store.Snapshot().Generation)
	if err != nil {
		t.Fatalf("admission remained frozen after cancelled management request: %v", err)
	}
	admittedDone()
}

func TestCancelledDisableRequestCannotStrandAdmission(t *testing.T) {
	owned, err := profile.Open(filepath.Join(t.TempDir(), "profile", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	defer owned.Close()
	store, err := mcpcredential.Open(owned.Directory, bytes.NewReader(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	prepared, _ := store.Prepare()
	if _, err := store.CommitEnable(prepared); err != nil {
		t.Fatal(err)
	}
	tracker := NewTracker()
	active, done, err := tracker.Admit(context.Background(), store.Snapshot().Generation)
	if err != nil {
		t.Fatal(err)
	}
	lifecycle := NewLifecycle(store, tracker, nil)
	management, cancelManagement := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- lifecycle.Disable(management) }()
	select {
	case <-active.Done():
	case <-time.After(time.Second):
		t.Fatal("disable did not begin draining active request")
	}
	cancelManagement()
	done()
	if err := <-result; err != nil {
		t.Fatalf("internally owned disable failed after browser cancellation: %v", err)
	}
	if snapshot := store.Snapshot(); snapshot.State != mcpcredential.Disabled {
		t.Fatalf("disable state = %+v", snapshot)
	}
	_, admittedDone, err := tracker.Admit(context.Background(), store.Snapshot().Generation)
	if err != nil {
		t.Fatalf("admission remained frozen after cancelled disable: %v", err)
	}
	admittedDone()
}

func TestShutdownPreemptsStuckMutationWithinShutdownBudgetWithoutReopen(t *testing.T) {
	owned, err := profile.Open(filepath.Join(t.TempDir(), "profile", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	defer owned.Close()
	store, err := mcpcredential.Open(owned.Directory, bytes.NewReader(make([]byte, 64)))
	if err != nil {
		t.Fatal(err)
	}
	prepared, _ := store.Prepare()
	oldKey, err := store.CommitEnable(prepared)
	if err != nil {
		t.Fatal(err)
	}
	tracker := NewTracker()
	active, done, err := tracker.Admit(context.Background(), store.Snapshot().Generation)
	if err != nil {
		t.Fatal(err)
	}
	lifecycle := NewLifecycle(store, tracker, nil)
	mutationResult := make(chan error, 1)
	go func() {
		_, regenerateErr := lifecycle.Regenerate(context.Background())
		mutationResult <- regenerateErr
	}()
	select {
	case <-active.Done():
	case <-time.After(time.Second):
		t.Fatal("mutation did not reach its drain barrier")
	}

	shutdown, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	err = lifecycle.Shutdown(shutdown)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shutdown error = %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("shutdown exceeded its own budget: %v", elapsed)
	}
	select {
	case err := <-mutationResult:
		if err == nil {
			t.Fatal("shutdown allowed preempted mutation to commit")
		}
	case <-time.After(time.Second):
		t.Fatal("preempted mutation remained blocked behind its drain timeout")
	}
	if !tracker.Frozen() {
		t.Fatal("preempted temporary freeze reopened permanent shutdown admission")
	}
	if _, _, err := tracker.Admit(context.Background(), store.Snapshot().Generation); err == nil {
		t.Fatal("tracker admitted work after permanent shutdown")
	}
	if _, ok := store.Authenticate(oldKey); !ok {
		t.Fatal("preempted regeneration changed the credential")
	}
	done()
}
