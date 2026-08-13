package browserapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
)

func importRequest(router http.Handler, body, contentType string, cookie *http.Cookie, tab, csrf string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7943/api/console/v1/artifacts/import", strings.NewReader(body))
	request.Host = "127.0.0.1:7943"
	request.Header.Set("Origin", "http://127.0.0.1:7943")
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("X-loomspan-Console-Tab", tab)
	request.Header.Set("X-loomspan-Console-CSRF", csrf)
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestArtifactImportStreamsRawNDJSONAndReturnsImportedSource(t *testing.T) {
	fake := &fakeArtifactService{importResult: artifact.AcquiredArtifact{
		Handle:   "import-handle",
		Metadata: artifact.TraceMetadata{TraceID: "trace-import", SessionID: "session-import", Outcome: "SUCCEEDED"},
	}}
	router, tab, cookie := artifactTestRouter(t, fake)
	security, err := router.options.Sessions.Bootstrap(cookie.Value, tab)
	if err != nil {
		t.Fatal(err)
	}
	body := "{\"recordType\":\"TRACE_STARTED\"}\n"
	response := importRequest(router, body, "application/x-ndjson", cookie, security.TabID, security.CSRF)
	if response.Code != http.StatusOK || string(fake.importBody) != body || fake.importDeclared != int64(len(body)) ||
		!strings.Contains(response.Body.String(), `"source":"IMPORTED"`) || strings.Contains(response.Body.String(), "targetScopeId") {
		t.Fatalf("status=%d body=%s imported=%q declared=%d", response.Code, response.Body.String(), fake.importBody, fake.importDeclared)
	}
}

func TestArtifactImportAuthenticatesBeforeReadingAndRequiresExactMediaType(t *testing.T) {
	fake := &fakeArtifactService{}
	router, tab, cookie := artifactTestRouter(t, fake)
	unauthorized := importRequest(router, "secret", "application/x-ndjson", nil, tab, "bad")
	if unauthorized.Code != http.StatusUnauthorized || fake.importCalled {
		t.Fatalf("unauthorized status=%d importCalled=%v", unauthorized.Code, fake.importCalled)
	}
	security, err := router.options.Sessions.Bootstrap(cookie.Value, tab)
	if err != nil {
		t.Fatal(err)
	}
	wrongType := importRequest(router, "body", "application/x-ndjson; charset=utf-8", cookie, security.TabID, security.CSRF)
	if wrongType.Code != http.StatusBadRequest || fake.importCalled {
		t.Fatalf("wrong media type status=%d importCalled=%v", wrongType.Code, fake.importCalled)
	}
}

func TestArtifactImportMapsKnownAndUnknownLengthLimitFailuresTo413(t *testing.T) {
	fake := &fakeArtifactService{importLimit: 3}
	router, tab, cookie := artifactTestRouter(t, fake)
	security, err := router.options.Sessions.Bootstrap(cookie.Value, tab)
	if err != nil {
		t.Fatal(err)
	}
	known := importRequest(router, "four", "application/x-ndjson", cookie, security.TabID, security.CSRF)
	if known.Code != http.StatusRequestEntityTooLarge || fake.importCalled {
		t.Fatalf("known-length status=%d importCalled=%v", known.Code, fake.importCalled)
	}
	security, err = router.options.Sessions.Bootstrap(cookie.Value, security.TabID)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7943/api/console/v1/artifacts/import", strings.NewReader("four"))
	request.ContentLength = -1
	request.Host = "127.0.0.1:7943"
	request.Header.Set("Origin", "http://127.0.0.1:7943")
	request.Header.Set("Content-Type", "application/x-ndjson")
	request.Header.Set("X-loomspan-Console-Tab", security.TabID)
	request.Header.Set("X-loomspan-Console-CSRF", security.CSRF)
	request.AddCookie(cookie)
	unknown := httptest.NewRecorder()
	router.ServeHTTP(unknown, request)
	if unknown.Code != http.StatusRequestEntityTooLarge || !fake.importCalled {
		t.Fatalf("unknown-length status=%d importCalled=%v body=%s", unknown.Code, fake.importCalled, unknown.Body.String())
	}
}

func TestArtifactImportErrorsNeverExposeImportedOwnerAsTargetScope(t *testing.T) {
	fake := &fakeArtifactService{importErr: consolecore.NewError(
		consolecore.CodeArtifactAlreadyExists,
		"An imported trace with this identity is already installed.",
		"opaque-imported-owner",
		consolecore.Details{},
		nil,
	)}
	router, tab, cookie := artifactTestRouter(t, fake)
	security, err := router.options.Sessions.Bootstrap(cookie.Value, tab)
	if err != nil {
		t.Fatal(err)
	}

	response := importRequest(router, "trace", "application/x-ndjson", cookie, security.TabID, security.CSRF)
	if response.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "targetScopeId") || strings.Contains(response.Body.String(), "opaque-imported-owner") {
		t.Fatalf("imported owner leaked through browser error: %s", response.Body.String())
	}
}
