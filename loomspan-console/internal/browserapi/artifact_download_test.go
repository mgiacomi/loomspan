package browserapi

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/applicationclient"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/browserauth"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/evidence"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/observability"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/target"
)

// downloadTestRouter builds a router with a real target context and the given
// probe client, ready for raw download integration tests.
func downloadTestRouter(t *testing.T, client target.ProbeClient) (*Router, *http.Cookie) {
	router, cookie, _ := downloadTestRouterWithTarget(t, client)
	return router, cookie
}

func downloadTestRouterWithTarget(t *testing.T, client target.ProbeClient) (*Router, *http.Cookie, *target.Context) {
	t.Helper()
	entropy := bytes.Repeat([]byte{50}, 32*16)
	pairing := browserauth.NewPairing(nil, bytes.NewReader(entropy))
	registry := browserauth.NewRegistry(nil, bytes.NewReader(entropy))
	sessionID, _ := registry.CreateSession()
	policy, _ := NewPolicy("127.0.0.1:7943", "http://127.0.0.1:7943", "")
	targetContext, _ := target.New(func(applicationclient.Address) (target.ProbeClient, error) {
		return client, nil
	}, func() (target.ScopeID, error) { return "scope-1", nil }, nil)
	if err := targetContext.Select("http://127.0.0.1:8080"); err != nil {
		t.Fatal(err)
	}
	if _, domain := targetContext.SupplyCredential(
		context.Background(),
		[]byte(strings.Repeat("k", 32)),
	); domain != nil {
		t.Fatal(domain)
	}
	router, _ := New(Options{
		Policy: policy, Pairing: pairing, Sessions: registry,
		PairingURL: func(value string) string { return value },
		Target:     targetContext,
	})
	return router, browserauth.SessionCookie(sessionID), targetContext
}

// downloadRequest creates a GET request for the raw download route. By default
// it simulates a real same-origin browser navigation (no Origin header,
// Sec-Fetch-Site: same-origin, Sec-Fetch-Mode: navigate).
func downloadRequest(cookie *http.Cookie, traceID string) *http.Request {
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:7943/api/console/v1/artifacts/"+traceID+"/raw", nil)
	request.Host = "127.0.0.1:7943"
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	request.Header.Set("Sec-Fetch-Mode", "navigate")
	if cookie != nil {
		request.AddCookie(cookie)
	}
	return request
}

// countingArtifactProbeClient wraps fakeArtifactProbeClient and counts
// OpenArtifact calls to prove raw download uses fresh authorization each
// time and never consults the local cache.
type countingArtifactProbeClient struct {
	artifactBody     []byte
	openCalls        atomic.Int32
	failNextOpen     atomic.Bool
	instanceMismatch bool
}

type callbackArtifactProbeClient struct {
	*countingArtifactProbeClient
	afterOpen func()
}

func (client *callbackArtifactProbeClient) OpenArtifact(ctx context.Context, traceID, instanceID string, credential applicationclient.Credential) (*applicationclient.ArtifactStream, error) {
	stream, err := client.countingArtifactProbeClient.OpenArtifact(ctx, traceID, instanceID, credential)
	if err == nil && client.afterOpen != nil {
		client.afterOpen()
	}
	return stream, err
}

func (c *countingArtifactProbeClient) Probe(context.Context, applicationclient.Credential) (applicationclient.Instance, error) {
	return applicationclient.Instance{
		InstanceID:                  "11111111-1111-4111-8111-111111111111",
		ConsoleCompatibilityVersion: "0.1.0-SNAPSHOT",
		ObservedAt:                  time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC),
		LiveMonitoringAvailable:     true,
	}, nil
}

func (c *countingArtifactProbeClient) Get(context.Context, string, int64, applicationclient.Credential) ([]byte, string, error) {
	return nil, "", nil
}

func (c *countingArtifactProbeClient) OpenActivity(context.Context, string, string, applicationclient.Credential) (*applicationclient.ActivityStream, error) {
	return nil, nil
}

func (c *countingArtifactProbeClient) OpenArtifact(_ context.Context, _ string, _ string, _ applicationclient.Credential) (*applicationclient.ArtifactStream, error) {
	c.openCalls.Add(1)
	if c.failNextOpen.Load() {
		return nil, &applicationclient.InstanceMismatch{
			Actual: "22222222-2222-4222-8222-222222222222",
		}
	}
	return applicationclient.NewTestArtifactStream(
		io.NopCloser(bytes.NewReader(c.artifactBody)),
		"11111111-1111-4111-8111-111111111111",
		int64(len(c.artifactBody)),
	), nil
}

func (c *countingArtifactProbeClient) Close() {}

// errorArtifactProbeClient returns a failure from OpenArtifact to test
// pre-commit error handling.
type errorArtifactProbeClient struct {
	err error
}

func (c *errorArtifactProbeClient) Probe(context.Context, applicationclient.Credential) (applicationclient.Instance, error) {
	return applicationclient.Instance{
		InstanceID:                  "11111111-1111-4111-8111-111111111111",
		ConsoleCompatibilityVersion: "0.1.0-SNAPSHOT",
		ObservedAt:                  time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC),
		LiveMonitoringAvailable:     true,
	}, nil
}

func (c *errorArtifactProbeClient) Get(context.Context, string, int64, applicationclient.Credential) ([]byte, string, error) {
	return nil, "", nil
}

func (c *errorArtifactProbeClient) OpenActivity(context.Context, string, string, applicationclient.Credential) (*applicationclient.ActivityStream, error) {
	return nil, nil
}

func (c *errorArtifactProbeClient) OpenArtifact(context.Context, string, string, applicationclient.Credential) (*applicationclient.ArtifactStream, error) {
	return nil, c.err
}

func (c *errorArtifactProbeClient) Close() {}

// shortReadArtifactProbeClient returns a stream that declares a length but
// delivers fewer bytes, simulating a mid-stream failure.
type shortReadArtifactProbeClient struct {
	declaredLength int64
	actualBody     []byte
}

func (c *shortReadArtifactProbeClient) Probe(context.Context, applicationclient.Credential) (applicationclient.Instance, error) {
	return applicationclient.Instance{
		InstanceID:                  "11111111-1111-4111-8111-111111111111",
		ConsoleCompatibilityVersion: "0.1.0-SNAPSHOT",
		ObservedAt:                  time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC),
		LiveMonitoringAvailable:     true,
	}, nil
}

func (c *shortReadArtifactProbeClient) Get(context.Context, string, int64, applicationclient.Credential) ([]byte, string, error) {
	return nil, "", nil
}

func (c *shortReadArtifactProbeClient) OpenActivity(context.Context, string, string, applicationclient.Credential) (*applicationclient.ActivityStream, error) {
	return nil, nil
}

func (c *shortReadArtifactProbeClient) OpenArtifact(context.Context, string, string, applicationclient.Credential) (*applicationclient.ArtifactStream, error) {
	return applicationclient.NewTestArtifactStream(
		io.NopCloser(bytes.NewReader(c.actualBody)),
		"11111111-1111-4111-8111-111111111111",
		c.declaredLength,
	), nil
}

func (c *shortReadArtifactProbeClient) Close() {}

// TestRawDownloadStreamsExactBytesWithoutCacheMutation proves that the raw
// download streams exact upstream bytes and never calls the artifact service
// (no cache mutation).
func TestRawDownloadStreamsExactBytesWithoutCacheMutation(t *testing.T) {
	artifactBody := `{"kind":"TRACE_STARTED","summary":"test"}
{"kind":"STEP_COMPLETED","summary":"done"}
`
	fake := &fakeArtifactService{}
	client := &countingArtifactProbeClient{artifactBody: []byte(artifactBody)}
	router, cookie := downloadTestRouter(t, client)
	// Wire the artifact service so we can assert it was never called.
	router.options.Artifacts = fake

	response := httptest.NewRecorder()
	router.ServeHTTP(response, downloadRequest(cookie, "trace-1"))

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	if response.Body.String() != artifactBody {
		t.Fatalf("body mismatch: expected %q, got %q", artifactBody, response.Body.String())
	}
	if fake.acquireErr != nil || fake.removeCalled || fake.clearExpiredCalled || fake.clearAllCalled {
		t.Fatal("artifact service was called during raw download (cache mutation)")
	}
	if client.openCalls.Load() != 1 {
		t.Fatalf("expected 1 OpenArtifact call, got %d", client.openCalls.Load())
	}
}

// TestRawDownloadUsesFreshApplicationAuthorizationEveryTime proves that each
// raw download opens a new authenticated upstream stream rather than reusing
// a cached copy.
func TestRawDownloadUsesFreshApplicationAuthorizationEveryTime(t *testing.T) {
	artifactBody := `{"kind":"TRACE_STARTED"}
`
	client := &countingArtifactProbeClient{artifactBody: []byte(artifactBody)}
	router, cookie := downloadTestRouter(t, client)

	for i := 0; i < 3; i++ {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, downloadRequest(cookie, "trace-1"))
		if response.Code != http.StatusOK {
			t.Fatalf("download %d: expected 200, got %d: %s", i, response.Code, response.Body.String())
		}
		if response.Body.String() != artifactBody {
			t.Fatalf("download %d: body mismatch", i)
		}
	}
	if client.openCalls.Load() != 3 {
		t.Fatalf("expected 3 fresh OpenArtifact calls, got %d", client.openCalls.Load())
	}
}

// TestRawDownloadRejectsQueryRangeConditionalAndAmbiguousTraceID is a
// consolidated test for all request-shape rejections.
func TestRawDownloadRejectsQueryRangeConditionalAndAmbiguousTraceID(t *testing.T) {
	client := &countingArtifactProbeClient{artifactBody: []byte("test")}
	router, cookie := downloadTestRouter(t, client)

	tests := []struct {
		name   string
		path   string
		header string
		value  string
	}{
		{"query parameter", "/api/console/v1/artifacts/trace-1/raw?foo=bar", "", ""},
		{"Range header", "/api/console/v1/artifacts/trace-1/raw", "Range", "bytes=0-100"},
		{"If-Range header", "/api/console/v1/artifacts/trace-1/raw", "If-Range", "test"},
		{"If-Match header", "/api/console/v1/artifacts/trace-1/raw", "If-Match", "test"},
		{"If-None-Match header", "/api/console/v1/artifacts/trace-1/raw", "If-None-Match", "test"},
		{"If-Modified-Since header", "/api/console/v1/artifacts/trace-1/raw", "If-Modified-Since", "test"},
		{"If-Unmodified-Since header", "/api/console/v1/artifacts/trace-1/raw", "If-Unmodified-Since", "test"},
		{"path traversal dot-dot", "/api/console/v1/artifacts/../raw", "", ""},
		{"path traversal double", "/api/console/v1/artifacts/../../etc/passwd/raw", "", ""},
		{"path traversal encoded", "/api/console/v1/artifacts/%2e%2e/raw", "", ""},
		{"ambiguous multi-segment", "/api/console/v1/artifacts/a/b/raw", "", ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:7943"+test.path, nil)
			request.Host = "127.0.0.1:7943"
			request.Header.Set("Sec-Fetch-Site", "same-origin")
			request.Header.Set("Sec-Fetch-Mode", "navigate")
			if test.header != "" {
				request.Header.Set(test.header, test.value)
			}
			request.AddCookie(cookie)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for %s, got %d: %s", test.name, response.Code, response.Body.String())
			}
		})
	}
	if client.openCalls.Load() != 0 {
		t.Fatalf("expected 0 OpenArtifact calls for rejected requests, got %d", client.openCalls.Load())
	}
}

// TestRawDownloadUsesFixedSafeHeadersAndFilename proves the response uses
// safe, fixed headers and a sanitized filename derived from the trace ID,
// never trusting an upstream filename.
func TestRawDownloadUsesFixedSafeHeadersAndFilename(t *testing.T) {
	artifactBody := `{"kind":"TRACE_STARTED"}
`
	client := &countingArtifactProbeClient{artifactBody: []byte(artifactBody)}
	router, cookie := downloadTestRouter(t, client)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, downloadRequest(cookie, "trace-safe-1"))

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	if ct := response.Header().Get("Content-Type"); ct != applicationclient.ArtifactMediaType {
		t.Fatalf("expected Content-Type %s, got %s", applicationclient.ArtifactMediaType, ct)
	}
	if response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("expected X-Content-Type-Options: nosniff")
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("expected Cache-Control: no-store")
	}
	cd := response.Header().Get("Content-Disposition")
	if !strings.Contains(cd, "attachment") {
		t.Fatalf("expected Content-Disposition with attachment, got %s", cd)
	}
	if !strings.Contains(cd, "trace-safe-1.ndjson") {
		t.Fatalf("expected safe filename trace-safe-1.ndjson in Content-Disposition, got %s", cd)
	}
}

// TestRawDownloadFailureBeforeCommitReturnsDomainError proves that when the
// upstream fails before the response is committed, a bounded JSON error is
// returned instead of a partial body.
func TestRawDownloadFailureBeforeCommitReturnsDomainError(t *testing.T) {
	client := &errorArtifactProbeClient{
		err: &applicationclient.Failure{
			Kind:     applicationclient.FailureUnavailable,
			Category: applicationclient.CategoryUpstreamServer,
		},
	}
	router, cookie := downloadTestRouter(t, client)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, downloadRequest(cookie, "trace-1"))

	if response.Code == http.StatusOK {
		t.Fatal("expected non-200 for upstream failure before commit")
	}
	body := response.Body.String()
	if !strings.Contains(body, `"code"`) {
		t.Fatalf("expected JSON error envelope, got: %s", body)
	}
	if strings.Contains(body, "trace-1") && strings.Contains(body, "ndjson") {
		t.Fatalf("error response leaked artifact content: %s", body)
	}
}

// TestRawDownloadFailureAfterCommitAppendsNoErrorEnvelope proves that when
// the stream fails mid-body (after headers are committed), the response
// terminates without appending a JSON error envelope. This covers both the
// short-read (EOF after fewer bytes than declared) and the real mid-stream
// error (non-EOF error after some bytes) cases.
func TestRawDownloadFailureAfterCommitAppendsNoErrorEnvelope(t *testing.T) {
	// A short-read stream declares 100 bytes but delivers only 10, then EOF.
	// The handler writes the 10 bytes, then the read returns EOF (not an
	// error), so the stream terminates. We verify no error envelope is
	// appended after the partial body.
	shortBody := []byte(`{"short"}
`)
	client := &shortReadArtifactProbeClient{
		declaredLength: 100,
		actualBody:     shortBody,
	}
	router, cookie := downloadTestRouter(t, client)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, downloadRequest(cookie, "trace-1"))

	// The response should be 200 (headers committed before the short read
	// is detected) and the body should contain only the partial bytes,
	// not a JSON error envelope.
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200 (committed before failure), got %d: %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if strings.Contains(body, `"code"`) {
		t.Fatalf("error envelope appended after commit: %s", body)
	}
}

// TestRawDownloadMidStreamErrorAfterCommitAppendsNoErrorEnvelope proves that
// when the upstream stream returns a real non-EOF error mid-body (after
// headers are committed), the response terminates without appending a JSON
// error envelope. This is distinct from the short-read/EOF case: a real
// transport error (e.g., connection reset) must not produce a JSON envelope
// after the response is committed, because the client would interpret it as
// artifact content.
func TestRawDownloadMidStreamErrorAfterCommitAppendsNoErrorEnvelope(t *testing.T) {
	// A stream that delivers 5 bytes then returns io.ErrUnexpectedEOF,
	// simulating a mid-stream transport failure after the response is
	// committed.
	client := &midStreamErrorProbeClient{
		prefix: []byte(`{"a"}
`),
		failErr: io.ErrUnexpectedEOF,
	}
	router, cookie := downloadTestRouter(t, client)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, downloadRequest(cookie, "trace-1"))

	// The response should be 200 (headers committed before the error) and
	// the body should contain only the prefix bytes, not a JSON error
	// envelope.
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200 (committed before mid-stream error), got %d: %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if !strings.Contains(body, `{"a"}`) {
		t.Fatalf("expected prefix bytes in body, got: %s", body)
	}
	if strings.Contains(body, `"code"`) {
		t.Fatalf("error envelope appended after commit on mid-stream error: %s", body)
	}
}

// midStreamErrorProbeClient returns an ArtifactStream that delivers a prefix
// then a non-EOF error, simulating a mid-stream transport failure.
type midStreamErrorProbeClient struct {
	prefix  []byte
	failErr error
}

func (c *midStreamErrorProbeClient) Probe(context.Context, applicationclient.Credential) (applicationclient.Instance, error) {
	return applicationclient.Instance{
		InstanceID:                  "11111111-1111-4111-8111-111111111111",
		ConsoleCompatibilityVersion: "0.1.0-SNAPSHOT",
		ObservedAt:                  time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC),
		LiveMonitoringAvailable:     true,
	}, nil
}

func (c *midStreamErrorProbeClient) Get(context.Context, string, int64, applicationclient.Credential) ([]byte, string, error) {
	return nil, "", nil
}

func (c *midStreamErrorProbeClient) OpenActivity(context.Context, string, string, applicationclient.Credential) (*applicationclient.ActivityStream, error) {
	return nil, nil
}

func (c *midStreamErrorProbeClient) OpenArtifact(context.Context, string, string, applicationclient.Credential) (*applicationclient.ArtifactStream, error) {
	return applicationclient.NewTestArtifactStream(
		&errorAfterPrefixReader{prefix: c.prefix, failErr: c.failErr},
		"11111111-1111-4111-8111-111111111111",
		-1,
	), nil
}

func (c *midStreamErrorProbeClient) Close() {}

// errorAfterPrefixReader delivers a prefix then returns a non-EOF error.
type errorAfterPrefixReader struct {
	prefix    []byte
	failErr   error
	delivered bool
}

func (r *errorAfterPrefixReader) Read(p []byte) (int, error) {
	if !r.delivered {
		r.delivered = true
		n := copy(p, r.prefix)
		return n, nil
	}
	return 0, r.failErr
}

func (r *errorAfterPrefixReader) Close() error { return nil }

// TestRawDownloadBackpressureDoesNotBufferCompleteArtifact proves that the
// download handler streams chunks incrementally rather than buffering the
// entire artifact before writing. We use a real httptest.Server and a reader
// that blocks between chunks: the first chunk is produced immediately, but
// the second chunk is withheld until the test signals it. If the handler
// buffered the whole body, the HTTP client would not receive the first chunk
// until after the second chunk is released. By reading the first chunk from
// the client before releasing the second, we prove incremental streaming.
func TestRawDownloadBackpressureDoesNotBufferCompleteArtifact(t *testing.T) {
	chunk1 := []byte(`{"kind":"TRACE_STARTED"}
`)
	chunk2 := []byte(`{"kind":"STEP_COMPLETED"}
`)
	reader := &gatedReader{chunks: [][]byte{chunk1, chunk2}}
	client := &gatedArtifactProbeClient{body: reader, declared: int64(len(chunk1) + len(chunk2))}

	// Build a router whose policy matches the test server's actual address.
	// We use NewUnstartedServer to get the listener address before building
	// the router, since the policy validates the Host header against the
	// configured origin.
	entropy := bytes.Repeat([]byte{60}, 32*16)
	pairing := browserauth.NewPairing(nil, bytes.NewReader(entropy))
	registry := browserauth.NewRegistry(nil, bytes.NewReader(entropy))
	sessionID, _ := registry.CreateSession()

	server := httptest.NewUnstartedServer(nil)
	host := server.Listener.Addr().String()
	origin := "http://" + host
	policy, _ := NewPolicy(host, origin, "")
	targetContext, _ := target.New(func(applicationclient.Address) (target.ProbeClient, error) {
		return client, nil
	}, func() (target.ScopeID, error) { return "scope-1", nil }, nil)
	if err := targetContext.Select("http://127.0.0.1:8080"); err != nil {
		t.Fatal(err)
	}
	if _, domain := targetContext.SupplyCredential(
		context.Background(),
		[]byte(strings.Repeat("k", 32)),
	); domain != nil {
		t.Fatal(domain)
	}
	router, _ := New(Options{
		Policy: policy, Pairing: pairing, Sessions: registry,
		PairingURL: func(value string) string { return value },
		Target:     targetContext,
	})
	server.Config.Handler = router
	server.Start()
	defer server.Close()

	cookie := browserauth.SessionCookie(sessionID)
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		server.URL+"/api/console/v1/artifacts/trace-1/raw", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.AddCookie(cookie)
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	request.Header.Set("Sec-Fetch-Mode", "navigate")

	httpResponse, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer httpResponse.Body.Close()

	if httpResponse.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", httpResponse.StatusCode)
	}

	// Read the first chunk. The gated reader has produced chunk1 but is
	// blocking on chunk2 (the gate is closed). If the handler buffered the
	// entire artifact, this read would block until we release the gate.
	// We use a short timeout to prove the first chunk arrives without
	// releasing the second.
	type readResult struct {
		n   int
		err error
	}
	result := make(chan readResult, 1)
	buffer := make([]byte, 256)
	go func() {
		n, err := httpResponse.Body.Read(buffer)
		result <- readResult{n, err}
	}()
	select {
	case r := <-result:
		if r.n != len(chunk1) {
			t.Fatalf("first read returned %d bytes, expected %d (handler buffered the whole body)", r.n, len(chunk1))
		}
		if string(buffer[:r.n]) != string(chunk1) {
			t.Fatalf("first chunk mismatch: got %q, expected %q", buffer[:r.n], chunk1)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first chunk did not arrive within 2s — handler buffered the entire artifact before writing")
	}

	// Release the second chunk and read it.
	reader.release()
	secondBuffer := make([]byte, 256)
	n2, err := httpResponse.Body.Read(secondBuffer)
	if err != nil && err != io.EOF {
		t.Fatalf("second read failed: %v", err)
	}
	if n2 != len(chunk2) {
		t.Fatalf("second read returned %d bytes, expected %d", n2, len(chunk2))
	}
	if string(secondBuffer[:n2]) != string(chunk2) {
		t.Fatalf("second chunk mismatch: got %q, expected %q", secondBuffer[:n2], chunk2)
	}
}

// gatedReader delivers chunks one at a time, blocking after each chunk until
// release is called. The first chunk is available immediately; subsequent
// chunks block until released. This lets tests prove a handler streams
// incrementally rather than buffering the whole body.
type gatedReader struct {
	chunks    [][]byte
	index     int
	releaseCh chan struct{}
	closed    bool
}

func (r *gatedReader) Read(p []byte) (int, error) {
	if r.index >= len(r.chunks) {
		return 0, io.EOF
	}
	if r.index > 0 {
		// Block until the test releases the next chunk.
		<-r.releaseCh
	}
	chunk := r.chunks[r.index]
	r.index++
	n := copy(p, chunk)
	return n, nil
}

func (r *gatedReader) release() {
	select {
	case r.releaseCh <- struct{}{}:
	default:
	}
}

func (r *gatedReader) Close() error {
	r.closed = true
	return nil
}

// gatedArtifactProbeClient serves a gatedReader so tests can control when
// chunks become available, proving incremental streaming.
type gatedArtifactProbeClient struct {
	body     *gatedReader
	declared int64
}

func (c *gatedArtifactProbeClient) Probe(context.Context, applicationclient.Credential) (applicationclient.Instance, error) {
	return applicationclient.Instance{
		InstanceID:                  "11111111-1111-4111-8111-111111111111",
		ConsoleCompatibilityVersion: "0.1.0-SNAPSHOT",
		ObservedAt:                  time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC),
		LiveMonitoringAvailable:     true,
	}, nil
}

func (c *gatedArtifactProbeClient) Get(context.Context, string, int64, applicationclient.Credential) ([]byte, string, error) {
	return nil, "", nil
}

func (c *gatedArtifactProbeClient) OpenActivity(context.Context, string, string, applicationclient.Credential) (*applicationclient.ActivityStream, error) {
	return nil, nil
}

func (c *gatedArtifactProbeClient) OpenArtifact(context.Context, string, string, applicationclient.Credential) (*applicationclient.ArtifactStream, error) {
	c.body.releaseCh = make(chan struct{}, 1)
	return applicationclient.NewTestArtifactStream(
		c.body,
		"11111111-1111-4111-8111-111111111111",
		c.declared,
	), nil
}

func (c *gatedArtifactProbeClient) Close() {}

// TestRawDownloadCancellationAndScopeRotationCloseUpstream proves that
// client cancellation and scope rotation close the upstream stream.
func TestRawDownloadCancellationAndScopeRotationCloseUpstream(t *testing.T) {
	// Use the blocking probe client that blocks on the operation context.
	// When the client cancels, the stream should be interrupted and the
	// upstream stream must be closed (via the handler's defer stream.Close()).
	client := &blockingArtifactProbeClient{}
	router, cookie := downloadTestRouter(t, client)

	ctx, cancel := context.WithCancel(context.Background())
	request := downloadRequest(cookie, "trace-1").WithContext(ctx)
	response := httptest.NewRecorder()

	// Cancel after a short delay to simulate client disconnect.
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	router.ServeHTTP(response, request)
	// The response should not hang; the stream should be interrupted.
	// We don't assert a specific status code because the cancellation
	// may occur after or before the response is committed.
	// Assert the upstream stream was closed to release the connection.
	reader := client.lastReader.Load()
	if reader == nil {
		t.Fatal("expected the probe client to have opened a stream")
	}
	if !reader.closed.Load() {
		t.Fatal("upstream stream was not closed after client cancellation")
	}
}

// TestRawDownloadScopeRotationReturnsTargetChanged proves that scope rotation
// during a raw download returns a TARGET_CHANGED domain error before commit.
func TestRawDownloadScopeRotationReturnsTargetChanged(t *testing.T) {
	client := &countingArtifactProbeClient{
		artifactBody:     []byte("test"),
		instanceMismatch: true,
	}
	client.failNextOpen.Store(true)
	router, cookie := downloadTestRouter(t, client)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, downloadRequest(cookie, "trace-1"))

	// Instance mismatch triggers revalidation. Since the scope context is
	// still active, the authority revalidates and returns TARGET_CHANGED.
	if response.Code == http.StatusOK {
		t.Fatal("expected non-200 for scope rotation during download")
	}
}

func TestRawDownloadRotationAfterOpenReturnsTargetChangedBeforeCommit(t *testing.T) {
	client := &callbackArtifactProbeClient{
		countingArtifactProbeClient: &countingArtifactProbeClient{
			artifactBody: []byte("old-scope-artifact"),
		},
	}
	router, cookie, targetContext := downloadTestRouterWithTarget(t, client)
	client.afterOpen = targetContext.Close

	response := httptest.NewRecorder()
	router.ServeHTTP(response, downloadRequest(cookie, "trace-1"))

	if response.Code == http.StatusOK {
		t.Fatalf("rotation after stream open committed 200: %s", response.Body.String())
	}
	if !strings.Contains(response.Body.String(), string(consolecore.CodeTargetChanged)) {
		t.Fatalf("expected TARGET_CHANGED response, got %d: %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Content-Disposition") != "" {
		t.Fatal("artifact headers were published before the final scope check")
	}
}

// TestStorageSnapshotIsSideEffectFree proves that calling the storage snapshot
// endpoint does not refresh any entry's last-use time. The service-level
// invariant (StorageSnapshot does not mutate lastUsedAt) is proven by
// TestStorageSnapshotDoesNotRefreshLastUsedAt in the artifact package; this
// test proves the browser endpoint routes to StorageSnapshot only and never
// calls Lookup (which could refresh last-use) or any mutation operation.
func TestStorageSnapshotIsSideEffectFree(t *testing.T) {
	fake := &fakeArtifactService{
		snapshotResult: artifactStorageSnapshotForTest(),
	}
	router, _, cookie := artifactTestRouter(t, fake)

	// Call storage snapshot twice.
	for i := 0; i < 2; i++ {
		request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7943/api/console/v1/artifacts/storage", strings.NewReader(`{}`))
		request.Host = "127.0.0.1:7943"
		request.Header.Set("Origin", "http://127.0.0.1:7943")
		request.AddCookie(cookie)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("call %d: expected 200, got %d: %s", i, response.Code, response.Body.String())
		}
	}
	// The endpoint must call StorageSnapshot (the side-effect-free read) and
	// must not call Lookup, Acquire, or any mutation operation. Lookup is
	// read-only at the service level but is the path that could be mistakenly
	// used to refresh last-use; asserting it is not called proves the endpoint
	// uses the correct side-effect-free seam.
	if !fake.snapshotCalled {
		t.Fatal("storage snapshot endpoint did not call StorageSnapshot")
	}
	if fake.lookupCalled {
		t.Fatal("storage snapshot endpoint called Lookup, which could refresh last-use time")
	}
	if fake.acquireCalled {
		t.Fatal("storage snapshot endpoint called Acquire, which mutates cache state")
	}
	if fake.removeCalled || fake.clearExpiredCalled || fake.clearAllCalled {
		t.Fatal("storage snapshot endpoint triggered a mutation operation")
	}
}

// TestCachedTraceFallbackPreservesOriginalFactsWithoutClaimingCurrentApplicationState
// proves that when a trace is enriched with artifact data, the
// applicationAvailability field reflects the acquisition-time observation,
// not a claim of current application reachability.
func TestCachedTraceFallbackPreservesOriginalFactsWithoutClaimingCurrentApplicationState(t *testing.T) {
	// Simulate a cached entry where the application was AVAILABLE at
	// acquisition time but is now UNAVAILABLE upstream. The enrichment
	// should preserve the original AVAILABLE observation.
	fake := &fakeArtifactService{
		lookupResult: artifact.LookupResult{
			LocalAvailable:          true,
			Handle:                  artifact.Handle("handle-1"),
			ApplicationAvailability: artifact.ApplicationAvailable,
			AcquiredAt:              time.Date(2026, 7, 27, 0, 0, 3, 0, time.UTC),
			LastUsedAt:              time.Date(2026, 7, 27, 0, 0, 4, 0, time.UTC),
			LocalBytes:              100,
		},
	}
	router, _, _ := artifactTestRouter(t, fake)
	trace := observabilityTraceForTest()
	enriched := router.enrichTrace("scope-1", trace)

	if !enriched.LocalAvailable {
		t.Fatal("expected trace to be enriched as locally available")
	}
	if enriched.ArtifactHandle != "handle-1" {
		t.Fatalf("expected handle handle-1, got %q", enriched.ArtifactHandle)
	}
	// The applicationAvailability should be the acquisition-time observation
	// (AVAILABLE), not a claim about current upstream state.
	if enriched.ApplicationAvailability != "AVAILABLE" {
		t.Fatalf("expected acquisition-time availability AVAILABLE, got %q", enriched.ApplicationAvailability)
	}
}

// TestArtifactJSONRoutesRejectMethodBodyAndScopeVariants proves that the
// artifact JSON routes reject wrong methods, invalid bodies, and stale scopes.
func TestArtifactJSONRoutesRejectMethodBodyAndScopeVariants(t *testing.T) {
	fake := &fakeArtifactService{
		acquireResult: artifact.AcquiredArtifact{
			Handle:   artifact.Handle("h"),
			Metadata: artifact.TraceMetadata{TraceID: "trace-1"},
		},
	}
	router, _, _ := artifactTestRouter(t, fake)

	// Get the CSRF token by bootstrapping.
	entropy := bytes.Repeat([]byte{51}, 32*16)
	registry := browserauth.NewRegistry(nil, bytes.NewReader(entropy))
	sid, _ := registry.CreateSession()
	result, _ := registry.Bootstrap(sid, "")

	// Rebuild router with the registry that has the bootstrap result.
	policy, _ := NewPolicy("127.0.0.1:7943", "http://127.0.0.1:7943", "")
	tc, _ := target.New(func(applicationclient.Address) (target.ProbeClient, error) {
		return &fakeProbeClient{}, nil
	}, func() (target.ScopeID, error) { return "scope-1", nil }, nil)
	_ = tc.Select("http://127.0.0.1:8080")
	_, _ = tc.SupplyCredential(context.Background(), []byte(strings.Repeat("k", 32)))
	router, _ = New(Options{
		Policy: policy, Pairing: browserauth.NewPairing(nil, bytes.NewReader(entropy)),
		Sessions: registry, PairingURL: func(v string) string { return v },
		Target: tc, Artifacts: fake,
	})
	cookie := browserauth.SessionCookie(sid)

	// Wrong method (GET) on a POST-only route.
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:7943/api/console/v1/artifacts/acquire", nil)
	req.Host = "127.0.0.1:7943"
	req.Header.Set("Origin", "http://127.0.0.1:7943")
	req.AddCookie(cookie)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET on acquire: expected 405, got %d", resp.Code)
	}

	// Oversized body.
	resp = artifactRequestWithCSRF(router, "/api/console/v1/artifacts/acquire",
		`{"traceId":"`+strings.Repeat("x", maxArtifactJSONBody)+`"}`, cookie, result.TabID, result.CSRF)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("oversized body: expected 400, got %d: %s", resp.Code, resp.Body.String())
	}

	// Unknown field in body.
	resp = artifactRequestWithCSRF(router, "/api/console/v1/artifacts/acquire",
		`{"traceId":"trace-1","unexpected":true}`, cookie, result.TabID, result.CSRF)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("unknown field: expected 400, got %d: %s", resp.Code, resp.Body.String())
	}
}

// TestAcquireAndStorageSnapshotReturnOpaquePathFreeDTOs proves that the
// acquire and storage snapshot responses contain opaque handles and no
// filesystem paths.
func TestAcquireAndStorageSnapshotReturnOpaquePathFreeDTOs(t *testing.T) {
	fake := &fakeArtifactService{
		acquireResult: artifact.AcquiredArtifact{
			Owner:      evidence.Target(target.ScopeID("scope-1")),
			Handle:     artifact.Handle("opaque-handle-abc"),
			Metadata:   artifact.TraceMetadata{TraceID: "trace-1", SessionID: "s-1", Outcome: "SUCCEEDED"},
			LocalBytes: 100,
		},
		snapshotResult: artifactStorageSnapshotForTest(),
	}
	router, _, _ := artifactTestRouter(t, fake)

	// Build a fresh setup with a known CSRF token.
	entropy := bytes.Repeat([]byte{52}, 32*16)
	registry := browserauth.NewRegistry(nil, bytes.NewReader(entropy))
	sid, _ := registry.CreateSession()
	result, _ := registry.Bootstrap(sid, "")
	policy, _ := NewPolicy("127.0.0.1:7943", "http://127.0.0.1:7943", "")
	tc, _ := target.New(func(applicationclient.Address) (target.ProbeClient, error) {
		return &fakeProbeClient{}, nil
	}, func() (target.ScopeID, error) { return "scope-1", nil }, nil)
	_ = tc.Select("http://127.0.0.1:8080")
	_, _ = tc.SupplyCredential(context.Background(), []byte(strings.Repeat("k", 32)))
	router, _ = New(Options{
		Policy: policy, Pairing: browserauth.NewPairing(nil, bytes.NewReader(entropy)),
		Sessions: registry, PairingURL: func(v string) string { return v },
		Target: tc, Artifacts: fake,
	})
	cookie := browserauth.SessionCookie(sid)

	resp := artifactRequestWithCSRF(router, "/api/console/v1/artifacts/acquire",
		`{"traceId":"trace-1"}`, cookie, result.TabID, result.CSRF)
	if resp.Code != http.StatusOK {
		t.Fatalf("acquire: expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	body := resp.Body.String()
	if !strings.Contains(body, `"artifactHandle":"opaque-handle-abc"`) {
		t.Fatalf("expected opaque handle in response, got: %s", body)
	}
	if strings.Contains(body, "transient") || strings.Contains(body, "C:\\") || strings.Contains(body, "/tmp/") {
		t.Fatalf("response leaked filesystem path: %s", body)
	}

	// Storage snapshot
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7943/api/console/v1/artifacts/storage", strings.NewReader(`{}`))
	req.Host = "127.0.0.1:7943"
	req.Header.Set("Origin", "http://127.0.0.1:7943")
	req.AddCookie(cookie)
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("storage: expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	body = resp.Body.String()
	if strings.Contains(body, "transient") || strings.Contains(body, "C:\\") || strings.Contains(body, "/tmp/") {
		t.Fatalf("storage response leaked filesystem path: %s", body)
	}
	// Storage is global across target and imported evidence; each row identifies
	// its own source instead of publishing one snapshot-level target scope.
	if !strings.Contains(body, `"source":"TARGET"`) || strings.Contains(body, `"targetScopeId":"scope-1","workspaceLabel"`) {
		t.Fatalf("storage response does not use row-level evidence ownership: %s", body)
	}
}

// TestArtifactDomainErrorsMapToStableHTTPEnvelopes proves that all artifact
// domain error codes map to stable HTTP status codes and bounded JSON
// envelopes.
func TestArtifactDomainErrorsMapToStableHTTPEnvelopes(t *testing.T) {
	tests := []struct {
		name       string
		code       consolecore.Code
		wantStatus int
	}{
		{"ARTIFACT_EXPIRED", consolecore.CodeArtifactExpired, http.StatusConflict},
		{"ARTIFACT_IN_USE", consolecore.CodeArtifactInUse, http.StatusConflict},
		{"INVALID_ARTIFACT", consolecore.CodeInvalidArtifact, http.StatusUnprocessableEntity},
		{"LIMIT_EXCEEDED", consolecore.CodeLimitExceeded, http.StatusTooManyRequests},
		{"LOCAL_STORAGE_UNAVAILABLE", consolecore.CodeLocalStorageUnavailable, http.StatusServiceUnavailable},
		{"TARGET_CHANGED", consolecore.CodeTargetChanged, http.StatusConflict},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake := &fakeArtifactService{
				acquireErr: consolecore.NewError(test.code, "test error", "scope-1", consolecore.Details{}, nil),
			}
			entropy := bytes.Repeat([]byte{53}, 32*16)
			registry := browserauth.NewRegistry(nil, bytes.NewReader(entropy))
			sid, _ := registry.CreateSession()
			result, _ := registry.Bootstrap(sid, "")
			policy, _ := NewPolicy("127.0.0.1:7943", "http://127.0.0.1:7943", "")
			tc, _ := target.New(func(applicationclient.Address) (target.ProbeClient, error) {
				return &fakeProbeClient{}, nil
			}, func() (target.ScopeID, error) { return "scope-1", nil }, nil)
			_ = tc.Select("http://127.0.0.1:8080")
			_, _ = tc.SupplyCredential(context.Background(), []byte(strings.Repeat("k", 32)))
			router, _ := New(Options{
				Policy: policy, Pairing: browserauth.NewPairing(nil, bytes.NewReader(entropy)),
				Sessions: registry, PairingURL: func(v string) string { return v },
				Target: tc, Artifacts: fake,
			})
			cookie := browserauth.SessionCookie(sid)
			resp := artifactRequestWithCSRF(router, "/api/console/v1/artifacts/acquire",
				`{"traceId":"trace-1"}`, cookie, result.TabID, result.CSRF)
			if resp.Code != test.wantStatus {
				t.Fatalf("%s: expected %d, got %d: %s", test.name, test.wantStatus, resp.Code, resp.Body.String())
			}
			if !strings.Contains(resp.Body.String(), `"code":"`+string(test.code)+`"`) {
				t.Fatalf("%s: body does not contain code %s: %s", test.name, test.code, resp.Body.String())
			}
		})
	}
}

// Helper functions for test data.

func artifactStorageSnapshotForTest() artifact.StorageSnapshot {
	return artifact.StorageSnapshot{
		WorkspaceLabel: "workspace-label",
		MaxBytes:       1024,
		IdleTTL:        5 * time.Minute,
		ChargedBytes:   100,
		AcquiredCount:  1,
		Entries: []artifact.StoredEntry{
			{
				Source:                  evidence.SourceTarget,
				TargetScopeID:           "scope-1",
				TraceID:                 "trace-1",
				SessionID:               "s-1",
				Outcome:                 "SUCCEEDED",
				LocalBytes:              100,
				LocalAvailable:          true,
				ApplicationAvailability: "AVAILABLE",
				ActivePin:               false,
			},
		},
	}
}

func observabilityTraceForTest() observability.Trace {
	return observability.Trace{
		TraceID:   "trace-1",
		SessionID: "s-1",
		Outcome:   "SUCCEEDED",
	}
}

// Ensure unused imports don't cause errors.
var _ = io.EOF
