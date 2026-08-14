package webhost

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/browserapi"
)

func Routes(policy browserapi.Policy, api, mcp http.Handler, files fs.FS) http.Handler {
	static := StaticHandler(files)
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/mcp" {
			mcp.ServeHTTP(response, request)
			return
		}
		if strings.HasPrefix(request.URL.Path, "/api/console/v1/") {
			api.ServeHTTP(response, request)
			return
		}
		if !policy.ValidateHost(request) {
			ApplyBrowserHeaders(response.Header())
			response.Header().Set("Cache-Control", "no-store")
			http.Error(response, "browser request rejected", http.StatusBadRequest)
			return
		}
		static.ServeHTTP(response, request)
	})
}
