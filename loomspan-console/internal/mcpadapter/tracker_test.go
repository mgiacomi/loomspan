package mcpadapter

import (
	"context"
	"testing"
	"time"
)

func TestFreezeCancelsClosesSessionsAndWaitsForOuterHandlers(t *testing.T) {
	tracker := NewTracker()
	request, done, err := tracker.Admit(context.Background(), 3)
	if err != nil {
		t.Fatal(err)
	}
	closed := make(chan struct{})
	drained := make(chan error, 1)
	go func() { drained <- tracker.Freeze(context.Background(), false, func() { close(closed) }) }()
	select {
	case <-request.Done():
	case <-time.After(time.Second):
		t.Fatal("admitted request was not cancelled")
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("sessions were not closed")
	}
	select {
	case <-drained:
		t.Fatal("freeze completed before outer handler")
	default:
	}
	done()
	if err := <-drained; err != nil {
		t.Fatal(err)
	}
	if _, _, err := tracker.Admit(context.Background(), 3); err == nil {
		t.Fatal("admitted while frozen")
	}
	if err := tracker.Reopen(); err != nil {
		t.Fatal(err)
	}
	_, done, err = tracker.Admit(context.Background(), 3)
	if err != nil {
		t.Fatal(err)
	}
	done()
}

func TestPermanentShutdownCannotReopen(t *testing.T) {
	tracker := NewTracker()
	if err := tracker.Freeze(context.Background(), true, nil); err != nil {
		t.Fatal(err)
	}
	if err := tracker.Reopen(); err == nil {
		t.Fatal("permanent tracker reopened")
	}
}
