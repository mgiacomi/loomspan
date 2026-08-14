package mcpadapter

import (
	"net/http"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/release"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Server struct {
	sdk     *mcp.Server
	handler http.Handler
}

func NewServer(port int, credentials authenticator, tracker *Tracker, provider StatusProvider) *Server {
	sdk := mcp.NewServer(&mcp.Implementation{Name: "loomspan-console", Version: release.ProductVersion()}, nil)
	addRuntimeTool(sdk, provider, credentials)
	streamable := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return sdk }, &mcp.StreamableHTTPOptions{
		Stateless: true, JSONResponse: true, MaxRequestBodyBytes: maxRequestBody, PropagateRequestCancellation: true,
	})
	return &Server{sdk: sdk, handler: SecurityHandler(port, credentials, tracker, streamable)}
}

func (server *Server) Handler() http.Handler { return server.handler }

func (server *Server) CloseSessions() {
	if server == nil {
		return
	}
	for session := range server.sdk.Sessions() {
		_ = session.Close()
	}
}
