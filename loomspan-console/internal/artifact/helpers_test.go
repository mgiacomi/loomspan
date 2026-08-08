package artifact

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/applicationclient"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/target"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/workspace"
)

// manualClock is an injectable clock whose current time can be advanced
// deterministically.
type manualClock struct {
	mu  sync.Mutex
	now time.Time
}

func newManualClock(start time.Time) *manualClock {
	return &manualClock{now: start}
}

func (c *manualClock) nowFunc() func() time.Time {
	return func() time.Time {
		c.mu.Lock()
		defer c.mu.Unlock()
		return c.now
	}
}

func (c *manualClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// manualTimer is a timer whose callback is fired manually rather than by the
// OS timer. It records its scheduled delay so tests can assert timer schedule
// values directly.
type manualTimer struct {
	delay    time.Duration
	callback func()
	stopped  atomic.Bool
	fired    atomic.Bool
}

func (t *manualTimer) Stop() bool {
	stopped := !t.stopped.Swap(true)
	return stopped
}

func (t *manualTimer) fire() {
	if t.stopped.Load() || t.fired.Swap(true) {
		return
	}
	t.callback()
}

// manualTimerFactory records all timers created so tests can fire them in
// order and assert schedule values.
type manualTimerFactory struct {
	mu     sync.Mutex
	timers []*manualTimer
}

func (f *manualTimerFactory) factory() func(time.Duration, func()) timerHandle {
	return func(delay time.Duration, callback func()) timerHandle {
		t := &manualTimer{delay: delay, callback: callback}
		f.mu.Lock()
		f.timers = append(f.timers, t)
		f.mu.Unlock()
		return t
	}
}

func (f *manualTimerFactory) latest() *manualTimer {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.timers) == 0 {
		return nil
	}
	return f.timers[len(f.timers)-1]
}

func (f *manualTimerFactory) fireAll() {
	f.mu.Lock()
	timers := append([]*manualTimer(nil), f.timers...)
	f.mu.Unlock()
	for _, t := range timers {
		t.fire()
	}
}

// deterministicEntropy returns predictable handle bytes so tests can assert
// exact handle values and deterministic eviction tie-breakers.
type deterministicEntropy struct {
	mu    sync.Mutex
	count int
}

func (e *deterministicEntropy) factory() func() ([]byte, error) {
	return func() ([]byte, error) {
		e.mu.Lock()
		defer e.mu.Unlock()
		e.count++
		data := make([]byte, handleByteLength)
		for i := range data {
			data[i] = byte(e.count)
		}
		return data, nil
	}
}

// testWorkspace opens a real verified workspace in a per-test temp directory.
func testWorkspace(t *testing.T) *workspace.Workspace {
	t.Helper()
	ws, err := workspace.Open(filepath.Join(t.TempDir(), "work"))
	if err != nil {
		t.Fatalf("open test workspace: %v", err)
	}
	t.Cleanup(func() { _ = ws.Close() })
	return ws
}

// testTraceMetadata creates valid TraceMetadata for a trace ID and size.
func testTraceMetadata(traceID string, sizeBytes int64) TraceMetadata {
	return TraceMetadata{
		TraceID:                   traceID,
		SessionID:                 "session-" + traceID,
		EntrySkill:                "CheckDns",
		Outcome:                   "COMPLETED",
		FinalizedAt:               time.UnixMilli(1000000),
		SizeBytes:                 sizeBytes,
		PersistencePolicy:         "PERSISTENT",
		ApplicationTraceExpiresAt: time.UnixMilli(2000000),
	}
}

// testScope creates a target scope with a cancellable context for testing.
func testScope(id string) (target.Scope, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	return target.Scope{
		ID:         target.ScopeID(id),
		Context:    ctx,
		InstanceID: "test-instance",
	}, cancel
}

// fakeLoader is a TraceLoader that returns predetermined metadata and records
// calls so tests can verify joined acquisition performs one metadata load.
type fakeLoader struct {
	mu            sync.Mutex
	calls         int
	metadata      TraceMetadata
	err           *consolecore.Error
	barrier       chan struct{}
	release       chan struct{}
	barrierClosed bool
}

func newFakeLoader(metadata TraceMetadata) *fakeLoader {
	return &fakeLoader{metadata: metadata}
}

func (l *fakeLoader) loader() TraceLoader {
	return func(ctx context.Context, scope target.Scope, traceID string) (TraceMetadata, *consolecore.Error) {
		l.mu.Lock()
		l.calls++
		barrier := l.barrier
		release := l.release
		barrierClosed := l.barrierClosed
		l.barrierClosed = true
		l.mu.Unlock()
		if barrier != nil && !barrierClosed {
			close(barrier)
		}
		if release != nil {
			select {
			case <-release:
			case <-ctx.Done():
				return TraceMetadata{}, consolecore.NewError(consolecore.CodeTargetChanged,
					"The selected target changed.", string(scope.ID), consolecore.Details{}, ctx.Err())
			}
		}
		if l.err != nil {
			return TraceMetadata{}, l.err
		}
		return l.metadata, nil
	}
}

func (l *fakeLoader) callCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.calls
}

// fakeOpener is a StreamOpener that returns predetermined bytes and records
// calls so tests can verify joined acquisition performs one upstream stream.
type fakeOpener struct {
	mu            sync.Mutex
	calls         int
	data          []byte
	declared      int64
	err           *consolecore.Error
	barrier       chan struct{}
	release       chan struct{}
	barrierClosed bool
	closeCount    atomic.Int32
}

func newFakeOpener(data []byte, declared int64) *fakeOpener {
	return &fakeOpener{data: data, declared: declared}
}

func (o *fakeOpener) opener() StreamOpener {
	return func(ctx context.Context, scope target.Scope, traceID string) (*applicationclient.ArtifactStream, *consolecore.Error) {
		o.mu.Lock()
		o.calls++
		barrier := o.barrier
		release := o.release
		barrierClosed := o.barrierClosed
		o.barrierClosed = true
		o.mu.Unlock()
		if barrier != nil && !barrierClosed {
			close(barrier)
		}
		if release != nil {
			select {
			case <-release:
			case <-ctx.Done():
				return nil, consolecore.NewError(consolecore.CodeTargetChanged,
					"The selected target changed.", string(scope.ID), consolecore.Details{}, ctx.Err())
			}
		}
		if o.err != nil {
			return nil, o.err
		}
		body := &countingReadCloser{data: o.data, ctx: ctx}
		return applicationclient.NewTestArtifactStream(body, scope.InstanceID, o.declared), nil
	}
}

func (o *fakeOpener) callCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.calls
}

// countingReadCloser wraps a byte slice and respects context cancellation.
type countingReadCloser struct {
	data   []byte
	offset int
	ctx    context.Context
	closed atomic.Bool
}

func (r *countingReadCloser) Read(p []byte) (int, error) {
	if r.closed.Load() {
		return 0, io.ErrClosedPipe
	}
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if r.offset >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.offset:])
	r.offset += n
	return n, nil
}

func (r *countingReadCloser) Close() error {
	r.closed.Store(true)
	return nil
}

// faultyFile wraps a real writableFile and injects failures.
type faultyFile struct {
	real      writableFile
	fs        *faultyFS
	written   int64
	shortAt   int64
	shortDone bool
}

func (f *faultyFile) Write(p []byte) (int, error) {
	if f.shortAt > 0 && !f.shortDone && f.written+int64(len(p)) > f.shortAt {
		f.shortDone = true
		n := int(f.shortAt - f.written)
		if n < 0 {
			n = 0
		}
		if n > len(p) {
			n = len(p)
		}
		written, err := f.real.Write(p[:n])
		f.written += int64(written)
		return written, err
	}
	n, err := f.real.Write(p)
	f.written += int64(n)
	return n, err
}

func (f *faultyFile) Sync() error {
	if f.fs.syncFail != nil {
		return f.fs.syncFail
	}
	return f.real.Sync()
}

func (f *faultyFile) Close() error {
	// Always close the underlying file to avoid leaking handles, then
	// report the simulated failure if configured.
	err := f.real.Close()
	if f.fs.closeFail != nil {
		return f.fs.closeFail
	}
	return err
}

// faultyFS wraps the real filesystem and injects failures at specific points.
type faultyFS struct {
	real          realFileSystem
	syncFail      error
	closeFail     error
	renameFail    error
	removeFail    error
	removeAllFail error
	createFail    error
	statFail      error
	shortAt       int64
}

type blockingRenameFS struct {
	real    realFileSystem
	entered chan struct{}
	release chan struct{}
}

func newBlockingRenameFS() *blockingRenameFS {
	return &blockingRenameFS{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (fs *blockingRenameFS) mkdirAll(path string, perm os.FileMode) error {
	return fs.real.mkdirAll(path, perm)
}

func (fs *blockingRenameFS) create(path string) (writableFile, error) {
	return fs.real.create(path)
}

func (fs *blockingRenameFS) rename(oldpath, newpath string) error {
	close(fs.entered)
	<-fs.release
	return fs.real.rename(oldpath, newpath)
}

func (fs *blockingRenameFS) remove(path string) error {
	return fs.real.remove(path)
}

func (fs *blockingRenameFS) removeAll(path string) error {
	return fs.real.removeAll(path)
}

func (fs *blockingRenameFS) open(path string) (io.ReadCloser, error) {
	return fs.real.open(path)
}

func (fs *blockingRenameFS) openSeekable(path string) (io.ReadSeekCloser, error) {
	return fs.real.openSeekable(path)
}

func (fs *blockingRenameFS) readDir(dir string) ([]os.DirEntry, error) {
	return fs.real.readDir(dir)
}

func (fs *blockingRenameFS) stat(path string) (os.FileInfo, error) {
	return fs.real.stat(path)
}

func newFaultyFS() *faultyFS {
	return &faultyFS{real: realFileSystem{}}
}

func (fs *faultyFS) mkdirAll(path string, perm os.FileMode) error {
	return fs.real.mkdirAll(path, perm)
}

func (fs *faultyFS) create(path string) (writableFile, error) {
	if fs.createFail != nil {
		return nil, fs.createFail
	}
	file, err := fs.real.create(path)
	if err != nil {
		return nil, err
	}
	return &faultyFile{real: file, fs: fs, shortAt: fs.shortAt}, nil
}

func (fs *faultyFS) rename(oldpath, newpath string) error {
	if fs.renameFail != nil {
		return fs.renameFail
	}
	return fs.real.rename(oldpath, newpath)
}

func (fs *faultyFS) remove(path string) error {
	if fs.removeFail != nil {
		return fs.removeFail
	}
	return fs.real.remove(path)
}

func (fs *faultyFS) removeAll(path string) error {
	if fs.removeAllFail != nil {
		return fs.removeAllFail
	}
	return fs.real.removeAll(path)
}

func (fs *faultyFS) open(path string) (io.ReadCloser, error) {
	return fs.real.open(path)
}

func (fs *faultyFS) openSeekable(path string) (io.ReadSeekCloser, error) {
	return fs.real.openSeekable(path)
}

func (fs *faultyFS) readDir(dir string) ([]os.DirEntry, error) {
	return fs.real.readDir(dir)
}

func (fs *faultyFS) stat(path string) (os.FileInfo, error) {
	if fs.statFail != nil {
		return nil, fs.statFail
	}
	return fs.real.stat(path)
}

// newTestService creates a fully wired artifact service with injectable
// dependencies for deterministic testing. It uses a fakeProcessor that accepts
// any raw artifact and writes one small derived component, so artifact-only
// lifecycle tests do not need valid NDJSON. Production-composition tests should
// use newTestServiceWithProcessor with the real traceanalysis.Processor and
// valid Java fixture bytes.
func newTestService(t *testing.T, config Config, loader *fakeLoader, opener *fakeOpener) *Service {
	t.Helper()
	return newTestServiceWithDeps(t, config, loader, opener, &manualTimerFactory{}, newManualClock(time.UnixMilli(1000000)), nil)
}

func newTestServiceWithDeps(t *testing.T, config Config, loader *fakeLoader, opener *fakeOpener, timers *manualTimerFactory, clock *manualClock, fs fileSystem) *Service {
	t.Helper()
	return newTestServiceWithProcessor(t, config, loader, opener, timers, clock, fs, newFakeProcessor())
}

// newTestServiceWithProcessor wires a fully configured artifact service with an
// explicit processor. Production-composition tests pass the real
// traceanalysis.Processor; artifact-only lifecycle tests pass a fake.
func newTestServiceWithProcessor(t *testing.T, config Config, loader *fakeLoader, opener *fakeOpener, timers *manualTimerFactory, clock *manualClock, fs fileSystem, processor Processor) *Service {
	t.Helper()
	ws := testWorkspace(t)
	entropy := &deterministicEntropy{}
	deps := Dependencies{
		Lifetime:     context.Background(),
		Workspace:    ws,
		TraceLoader:  loader.loader(),
		StreamOpener: opener.opener(),
		Processor:    processor,
		Clock:        clock.nowFunc(),
		Entropy:      entropy.factory(),
		TimerFactory: timers.factory(),
		FileSystem:   fs,
	}
	svc, err := New(config, deps)
	if err != nil {
		t.Fatalf("create artifact service: %v", err)
	}
	t.Cleanup(func() { svc.Close() })
	return svc
}

// acquireSync calls Acquire and fails the test on error.
func acquireSync(t *testing.T, svc *Service, ctx context.Context, scope target.Scope, traceID string) AcquiredArtifact {
	t.Helper()
	artifact, domain := svc.Acquire(ctx, scope, traceID)
	if domain != nil {
		t.Fatalf("acquire failed: %s: %v", domain.Code, domain.Message)
	}
	return artifact
}

// waitForWaiters polls the service until the entry for the given trace ID has
// at least minWaiters registered, or the timeout expires.
func waitForWaiters(t *testing.T, svc *Service, traceID string, minWaiters int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		for _, entry := range svc.entries {
			if entry.key.traceID == traceID && entry.waiters >= minWaiters {
				svc.mu.Unlock()
				return
			}
		}
		svc.mu.Unlock()
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d waiters on trace %q", minWaiters, traceID)
}

// fakeProcessor is a test Processor that accepts any raw artifact and writes one
// small derived component named by ComponentDerived. It records calls, can
// inject failures and barriers, and lets artifact-only lifecycle tests avoid
// valid NDJSON. It never validates content.
const fakeProcessorComponent ComponentName = "fake-derived.bin"

type fakeProcessor struct {
	mu            sync.Mutex
	calls         int
	err           *consolecore.Error
	derivedBytes  []byte
	barrier       chan struct{}
	release       chan struct{}
	barrierClosed bool
	derivedName   ComponentName
	cancelAfter   int // if > 0, fail the nth Create call with the ctx error
	createCount   int
}

func newFakeProcessor() *fakeProcessor {
	return &fakeProcessor{
		derivedBytes: []byte("derived"),
		derivedName:  fakeProcessorComponent,
	}
}

// fakeDerivedSize returns the byte count the default fake processor charges for
// its derived component. Tests that assert aggregate local/charged bytes add
// this to the raw transfer length.
func fakeDerivedSize() int64 { return int64(len("derived")) }

func (p *fakeProcessor) Process(req ProcessRequest) (ProcessResult, *consolecore.Error) {
	p.mu.Lock()
	p.calls++
	barrier := p.barrier
	release := p.release
	barrierClosed := p.barrierClosed
	p.barrierClosed = true
	p.mu.Unlock()
	if barrier != nil && !barrierClosed {
		close(barrier)
	}
	if release != nil {
		select {
		case <-release:
		case <-req.Context.Done():
			return ProcessResult{}, consolecore.NewError(consolecore.CodeTargetUnavailable,
				"The operation was canceled.", "", consolecore.Details{}, req.Context.Err())
		}
	}
	if p.err != nil {
		return ProcessResult{}, p.err
	}
	name := p.derivedName
	if name == "" {
		name = fakeProcessorComponent
	}
	writer, domain := req.Sink.Create(req.Context, name)
	if domain != nil {
		return ProcessResult{}, domain
	}
	if _, err := writer.Write(p.derivedBytes); err != nil {
		_ = writer.Close()
		return ProcessResult{}, consolecore.NewError(consolecore.CodeLocalStorageUnavailable,
			"Local artifact storage is unavailable.", "", consolecore.Details{}, err)
	}
	if err := writer.Sync(); err != nil {
		_ = writer.Close()
		return ProcessResult{}, consolecore.NewError(consolecore.CodeLocalStorageUnavailable,
			"Local artifact storage is unavailable.", "", consolecore.Details{}, err)
	}
	if err := writer.Close(); err != nil {
		return ProcessResult{}, consolecore.NewError(consolecore.CodeLocalStorageUnavailable,
			"Local artifact storage is unavailable.", "", consolecore.Details{}, err)
	}
	return ProcessResult{
		ComponentSizes: map[ComponentName]int64{
			name: int64(len(p.derivedBytes)),
		},
	}, nil
}

func (p *fakeProcessor) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

// readLeasedComponent reads all bytes from a leased component and closes the
// reader, returning the bytes.
func readLeasedComponent(t *testing.T, lease *Lease, name ComponentName) []byte {
	t.Helper()
	reader, err := lease.OpenComponent(name)
	if err != nil {
		t.Fatalf("OpenComponent(%s) failed: %v", name, err)
	}
	body, readErr := io.ReadAll(reader)
	reader.Close()
	if readErr != nil {
		t.Fatalf("read component %s: %v", name, readErr)
	}
	return body
}

// bundleFileCount counts files (not directories) beneath the artifacts directory
// for assertions that no partial or installed bundle remains.
func bundleFileCount(t *testing.T, svc *Service) int {
	t.Helper()
	count := 0
	err := filepath.WalkDir(svc.storage.dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			count++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk artifacts dir: %v", err)
	}
	return count
}

// bundleDirCount counts installed/staging bundle directories beneath the
// artifacts directory.
func bundleDirCount(t *testing.T, svc *Service) int {
	t.Helper()
	entries, err := os.ReadDir(svc.storage.dir)
	if err != nil {
		t.Fatalf("read artifacts dir: %v", err)
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() {
			count++
		}
	}
	return count
}
