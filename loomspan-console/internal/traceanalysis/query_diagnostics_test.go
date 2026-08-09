package traceanalysis

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

func TestContextChunkReaderStopsAfterCancellationBetweenReads(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	source := &cancelAfterFirstRead{cancel: cancel}

	_, err := io.ReadAll(&contextChunkReader{ctx: ctx, reader: source})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
	if source.reads != 1 {
		t.Fatalf("expected cancellation to stop after one source read, got %d", source.reads)
	}
}

func TestContextCancellationClosesAnActiveReader(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	source := newBlockingReadCloser()
	stopCancellationClose := closeReaderOnCancellation(ctx, source)
	defer stopCancellationClose()

	result := make(chan error, 1)
	go func() {
		_, err := io.ReadAll(&contextChunkReader{ctx: ctx, reader: source})
		result <- err
	}()
	<-source.started
	cancel()

	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context cancellation, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled read did not return promptly")
	}
	if !source.wasClosed() {
		t.Fatal("context cancellation did not close the active reader")
	}
}

type cancelAfterFirstRead struct {
	cancel context.CancelFunc
	reads  int
}

func (reader *cancelAfterFirstRead) Read(buf []byte) (int, error) {
	reader.reads++
	buf[0] = 'x'
	reader.cancel()
	return 1, nil
}

type blockingReadCloser struct {
	started chan struct{}
	closed  chan struct{}
	once    sync.Once
}

func newBlockingReadCloser() *blockingReadCloser {
	return &blockingReadCloser{started: make(chan struct{}), closed: make(chan struct{})}
}

func (reader *blockingReadCloser) Read([]byte) (int, error) {
	reader.once.Do(func() { close(reader.started) })
	<-reader.closed
	return 0, errors.New("reader closed")
}

func (reader *blockingReadCloser) Close() error {
	select {
	case <-reader.closed:
	default:
		close(reader.closed)
	}
	return nil
}

func (reader *blockingReadCloser) wasClosed() bool {
	select {
	case <-reader.closed:
		return true
	default:
		return false
	}
}
