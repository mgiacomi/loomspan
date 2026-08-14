package console

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/applicationclient"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/browserapi"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/browserauth"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/config"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/lifecycle"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/live"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/mcpadapter"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/mcpcredential"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/observability"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/profile"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/release"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/target"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/traceanalysis"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/traceinventory"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/webhost"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/workspace"
)

type Options struct {
	ConfigPath              string
	WorkDirectory           string
	ListenOverride          string
	DevelopmentOrigin       string
	NoOpenBrowser           bool
	PromptForApplicationKey bool
	TargetAddressDefault    string
	ApplicationKeyDefault   string
}

type Dependencies struct {
	Files                fs.FS
	Output               io.Writer
	OpenBrowser          func(string) error
	PromptApplicationKey func(context.Context) ([]byte, error)
}

func Run(parent context.Context, options Options, dependencies Dependencies) (result error) {
	ownedProfile, err := profile.Open(options.ConfigPath)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := ownedProfile.Close(); result == nil && closeErr != nil {
			result = fmt.Errorf("release profile lock: %w", closeErr)
		}
	}()

	workPath := options.WorkDirectory
	if workPath == "" {
		workPath, err = profile.DefaultWorkspacePath(ownedProfile.Directory)
		if err != nil {
			return err
		}
	}
	ownedWorkspace, err := workspace.Open(workPath)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := ownedWorkspace.Close(); result == nil && closeErr != nil {
			result = fmt.Errorf("release work lock: %w", closeErr)
		}
	}()
	if dependencies.Output != nil {
		fmt.Fprintf(dependencies.Output, "loomspan Console workspace: %s\n", ownedWorkspace.Root)
	}
	mcpStore, err := mcpcredential.Open(ownedProfile.Directory, nil)
	if err != nil {
		return err
	}
	mcpTracker := mcpadapter.NewTracker()
	var mcpServer *mcpadapter.Server
	var mcpLifecycle *mcpadapter.Lifecycle

	networkPolicy := applicationclient.NetworkPolicy{
		ConnectTimeout:        config.DefaultConnectTimeout,
		ResponseHeaderTimeout: config.DefaultResponseHeaderTimeout,
		RequestTimeout:        config.DefaultRequestTimeout,
	}
	if ownedProfile.Resolved.Target != nil {
		networkPolicy.ConnectTimeout = ownedProfile.Resolved.Target.ConnectTimeout
		networkPolicy.ResponseHeaderTimeout = ownedProfile.Resolved.Target.ResponseHeaderTimeout
		networkPolicy.RequestTimeout = ownedProfile.Resolved.Target.RequestTimeout
		networkPolicy.CABundlePEM = ownedProfile.Resolved.Target.CABundlePEM
	}
	targetContext, err := target.New(func(address applicationclient.Address) (target.ProbeClient, error) {
		return applicationclient.New(address, networkPolicy, release.ProductVersion())
	}, nil, nil)
	if err != nil {
		return err
	}
	defer targetContext.Close()
	if ownedProfile.Resolved.Target != nil {
		if domain := targetContext.Select(ownedProfile.Resolved.Target.Address); domain != nil {
			return domain
		}
	}
	if options.PromptForApplicationKey {
		if ownedProfile.Resolved.Target == nil {
			return fmt.Errorf("--prompt-for-application-key requires a configured target")
		}
		if dependencies.PromptApplicationKey == nil {
			return fmt.Errorf("application-key prompt is unavailable")
		}
		key, promptErr := dependencies.PromptApplicationKey(parent)
		if promptErr != nil {
			return promptErr
		}
		_, domain := targetContext.SupplyCredential(parent, key)
		clear(key)
		if domain != nil && domain.Code == consolecore.CodeInvalidArgument {
			return domain
		}
	}

	coordinator := lifecycle.New(parent)
	go ownedProfile.Monitor(coordinator.Context(), 0, coordinator.Fatal)
	go ownedWorkspace.Monitor(coordinator.Context(), 0, coordinator.Fatal)
	pairing := browserauth.NewPairing(nil, nil)
	sessions := browserauth.NewRegistry(nil, nil)
	defer pairing.Close()
	defer sessions.Close()

	observabilityService := observability.New()
	liveService := live.NewService(coordinator.Context())
	liveService.SetBaselineLoader(func(ctx context.Context, scope target.Scope) (live.Baseline, *consolecore.Error) {
		page, domain := observabilityService.ListActiveExecutions(ctx, scope, observability.ListRequest{})
		if domain != nil {
			return live.Baseline{}, domain
		}
		resumeCursor := "0"
		if page.ResumeCursor != nil {
			resumeCursor = *page.ResumeCursor
		}
		executions := append([]observability.ActiveExecution(nil), page.Items...)
		observedAt := page.ObservedAt
		for page.HasMore && page.NextCursor != nil {
			page, domain = observabilityService.ListActiveExecutions(ctx, scope, observability.ListRequest{Cursor: *page.NextCursor})
			if domain != nil {
				return live.Baseline{}, domain
			}
			executions = append(executions, page.Items...)
		}
		return live.Baseline{Executions: executions, ResumeCursor: resumeCursor, ObservedAt: observedAt}, nil
	})
	if err := targetContext.RegisterOwner("live", liveService); err != nil {
		return err
	}
	defer liveService.Close()

	// Create the trace-analysis service first so it can serve as the artifact
	// processor. The artifact service dependency is wired after the artifact
	// service exists, so query methods can acquire leases by handle.
	traceAnalysisService := traceanalysis.NewService(nil)
	artifactService, err := artifact.New(artifact.Config{
		MaxBytes:    ownedProfile.Resolved.MaxBytes,
		Unlimited:   ownedProfile.Resolved.Unlimited,
		IdleTTL:     ownedProfile.Resolved.IdleTTL,
		NeverExpire: ownedProfile.Resolved.NeverExpire,
	}, artifact.Dependencies{
		Lifetime:  coordinator.Context(),
		Workspace: ownedWorkspace,
		TraceLoader: func(ctx context.Context, scope target.Scope, traceID string) (artifact.TraceMetadata, *consolecore.Error) {
			trace, domain := observabilityService.GetTrace(ctx, scope, traceID)
			if domain != nil {
				return artifact.TraceMetadata{}, domain
			}
			return artifact.TraceMetadata{
				TraceID:                   trace.TraceID,
				SessionID:                 trace.SessionID,
				EntrySkill:                trace.EntrySkill,
				Outcome:                   trace.Outcome,
				FinalizedAt:               trace.FinalizedAt,
				SizeBytes:                 trace.SizeBytes,
				PersistencePolicy:         trace.PersistencePolicy,
				ApplicationTraceExpiresAt: trace.ApplicationTraceExpiresAt,
			}, nil
		},
		StreamOpener: func(ctx context.Context, scope target.Scope, traceID string) (*applicationclient.ArtifactStream, *consolecore.Error) {
			return scope.OpenArtifact(ctx, traceID)
		},
		Processor: traceAnalysisService,
		Fatal:     coordinator.Fatal,
	})
	if err != nil {
		return err
	}
	// Wire the artifact service back into the trace-analysis service so
	// adapter-facing query methods (PR 14 browser, PR 18 MCP) can acquire
	// leases by handle.
	traceAnalysisService.SetArtifactService(artifactService)
	traceInventoryService := traceinventory.New(artifactService, observabilityService, targetContext, time.Now)
	if err := targetContext.RegisterOwner("artifacts", artifactService); err != nil {
		return err
	}
	defer artifactService.Close()

	address := ownedProfile.Resolved.ListenerAddress
	if options.ListenOverride != "" {
		address = options.ListenOverride
	}
	processID, err := browserauth.Generate(nil)
	if err != nil {
		return err
	}
	var origin string
	var outputMu sync.Mutex
	printPairing := func(url string) error {
		outputMu.Lock()
		defer outputMu.Unlock()
		if dependencies.Output == nil {
			return nil
		}
		_, err := fmt.Fprintf(dependencies.Output, "Pairing URL: %s\n", url)
		return err
	}
	host := webhost.Host{
		Address: address,
		PreShutdown: func(ctx context.Context) error {
			if mcpLifecycle == nil {
				return nil
			}
			return mcpLifecycle.Shutdown(ctx)
		},
		Prepare: func(authority webhost.Authority) (http.Handler, error) {
			origin = authority.Origin
			_, portText, err := net.SplitHostPort(authority.Host)
			if err != nil {
				return nil, err
			}
			port, err := strconv.Atoi(portText)
			if err != nil {
				return nil, err
			}
			mcpServer = mcpadapter.NewServer(mcpadapter.ServerOptions{
				Port: port, Credentials: mcpStore, Tracker: mcpTracker,
				Status: func() consolecore.StatusSnapshot { return targetContext.Snapshot().Status },
				Target: targetContext, Observability: observabilityService, Live: liveService,
				Artifacts: artifactService, TraceAnalysis: traceAnalysisService, TraceInventory: traceInventoryService,
				Now: time.Now,
			})
			mcpLifecycle = mcpadapter.NewLifecycle(mcpStore, mcpTracker, mcpServer.CloseSessions)
			policy, err := browserapi.NewPolicy(authority.Host, authority.Origin, options.DevelopmentOrigin)
			if err != nil {
				return nil, err
			}
			pairingURL := func(secret string) string {
				return origin + "/#/pair/" + secret
			}
			api, err := browserapi.New(browserapi.Options{
				Policy:                policy,
				Pairing:               pairing,
				Sessions:              sessions,
				ProcessID:             processID,
				Workspace:             ownedWorkspace.Root,
				PairingURL:            pairingURL,
				PrintPairing:          printPairing,
				Target:                targetContext,
				Observability:         observabilityService,
				Live:                  liveService,
				Artifacts:             artifactService,
				TraceAnalysis:         traceAnalysisService,
				TraceInventory:        traceInventoryService,
				TargetAddressDefault:  options.TargetAddressDefault,
				ApplicationKeyDefault: options.ApplicationKeyDefault,
				MCP:                   mcpLifecycle,
				MCPEndpoint:           "http://127.0.0.1:" + portText + "/mcp",
			})
			if err != nil {
				return nil, err
			}
			secret, err := pairing.Create(false)
			if err != nil {
				return nil, err
			}
			url := pairingURL(secret)
			if err := printPairing(url); err != nil {
				return nil, err
			}
			if !options.NoOpenBrowser && dependencies.OpenBrowser != nil {
				if err := dependencies.OpenBrowser(url); err != nil && dependencies.Output != nil {
					fmt.Fprintln(dependencies.Output, "Browser opener unavailable; use the printed pairing URL.")
				}
			}
			return webhost.Routes(policy, api, mcpServer.Handler(), dependencies.Files), nil
		},
	}
	targetContext.StartServing()
	runErr := host.Run(coordinator.Context())
	coordinator.Stop()
	sessions.Close()
	pairing.Close()
	if err := ownedWorkspace.Cleanup(); err != nil && runErr == nil {
		// Cleanup is best-effort during shutdown once an invariant may have
		// failed. A healthy workspace still reports ordinary cleanup failure.
		if ownedWorkspace.Check() == nil {
			runErr = fmt.Errorf("clean transient workspace: %w", err)
		}
	}
	if cause := coordinator.Cause(); cause != nil && cause != context.Canceled {
		return cause
	}
	return runErr
}
