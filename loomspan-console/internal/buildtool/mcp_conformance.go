package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/mcpadapter"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/mcpcredential"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/profile"
)

func runMCPConformance(paths projectPaths) error {
	root, err := os.MkdirTemp("", "loomspan-mcp-conformance-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	owned, err := profile.Open(filepath.Join(root, "profile", "config.yaml"))
	if err != nil {
		return err
	}
	defer owned.Close()
	store, err := mcpcredential.Open(owned.Directory, nil)
	if err != nil {
		return err
	}
	prepared, err := store.Prepare()
	if err != nil {
		return err
	}
	key, err := store.CommitEnable(prepared)
	if err != nil {
		return err
	}
	defer func() { key = "" }()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	_, portText, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(portText)
	tracker := mcpadapter.NewTracker()
	mcpServer := mcpadapter.NewServer(port, store, tracker, func() consolecore.StatusSnapshot { return consolecore.NoTargetStatus(time.Now().UTC()) })
	productionHTTP := &http.Server{Handler: mcpServer.Handler()}
	go productionHTTP.Serve(listener)
	defer productionHTTP.Close()

	upstream, _ := url.Parse("http://127.0.0.1:" + portText)
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	var proxyHost string
	originalDirector := proxy.Director
	proxy.Director = func(request *http.Request) {
		incomingHost := request.Host
		incomingOrigin := request.Header.Get("Origin")
		originalDirector(request)
		_, proxyPort, _ := net.SplitHostPort(proxyHost)
		if incomingHost == proxyHost {
			request.Host = upstream.Host
		} else if incomingHost == "localhost:"+proxyPort {
			request.Host = "localhost:" + portText
		} else if incomingHost != "" {
			request.Host = incomingHost
		} else {
			request.Host = upstream.Host
		}
		if incomingOrigin == "http://127.0.0.1:"+proxyPort {
			request.Header.Set("Origin", "http://127.0.0.1:"+portText)
		} else if incomingOrigin == "http://localhost:"+proxyPort {
			request.Header.Set("Origin", "http://localhost:"+portText)
		} else if incomingOrigin != "" {
			request.Header.Set("Origin", incomingOrigin)
		}
		request.Header.Set("Authorization", "Bearer "+key)
	}
	proxyListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	proxyHost = proxyListener.Addr().String()
	proxyHTTP := &http.Server{Handler: proxy}
	go proxyHTTP.Serve(proxyListener)
	defer proxyHTTP.Close()

	conformanceDirectory := filepath.Join(paths.module, "mcp-conformance")
	if err := runCommand(conformanceDirectory, nil, "npm", "ci", "--allow-git=all", "--allow-remote=all"); err != nil {
		return err
	}
	endpoint := "http://" + proxyHost + "/mcp"
	runner := filepath.Join(conformanceDirectory, "node_modules", "@modelcontextprotocol", "conformance", "dist", "index.js")
	for _, invocation := range [][]string{
		{"server", "--url", endpoint, "--scenario", "server-initialize", "--spec-version", "2025-11-25"},
		{"server", "--url", endpoint, "--scenario", "tools-list", "--spec-version", "2025-11-25"},
		{"server", "--url", endpoint, "--scenario", "tools-list", "--spec-version", "2026-07-28"},
		{"server", "--url", endpoint, "--scenario", "caching", "--spec-version", "2026-07-28"},
		{"server", "--url", endpoint, "--scenario", "dns-rebinding-protection", "--spec-version", "2026-07-28"},
	} {
		command := exec.Command("node", append([]string{runner}, invocation...)...)
		command.Dir = conformanceDirectory
		command.Env = os.Environ()
		rawOutput, runErr := command.CombinedOutput()
		output := string(rawOutput)
		fmt.Print(output)
		if strings.Contains(output, "FAILURE") || !strings.Contains(output, "0 failed") {
			return fmt.Errorf("official MCP conformance %v reported a failed or incomplete result", invocation)
		}
		// The pinned runner currently triggers a libuv closing assertion after
		// printing a complete successful result on Node 24/Windows. Treat only
		// that post-result process failure as successful; Linux CI must exit 0.
		if runErr != nil && !(runtime.GOOS == "windows" && strings.Contains(output, "Assertion failed: !(handle->flags & UV_HANDLE_CLOSING)")) {
			return fmt.Errorf("official MCP conformance %v: %w", invocation, runErr)
		}
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := tracker.Freeze(shutdown, true, mcpServer.CloseSessions); err != nil {
		return err
	}
	return nil
}
