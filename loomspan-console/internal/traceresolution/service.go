package traceresolution

import (
	"context"
	"strings"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/evidence"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/observability"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/target"
)

const MaxTraceIDLength = 8192

type ArtifactService interface {
	Lookup(evidence.Reference, string) (artifact.LookupResult, *consolecore.Error)
	Acquire(context.Context, target.Scope, string) (artifact.AcquiredArtifact, *consolecore.Error)
}

type CatalogService interface {
	GetTrace(context.Context, target.Scope, string) (observability.Trace, *consolecore.Error)
}

type TargetProvider interface {
	Capture() (target.Scope, *consolecore.Error)
	RequireCurrent(target.ScopeID) *consolecore.Error
}

type Resolved struct {
	Reference evidence.Reference
	Handle    artifact.Handle
	Scope     target.Scope
}

type Service struct {
	artifacts ArtifactService
	catalog   CatalogService
	target    TargetProvider
}

func New(artifacts ArtifactService, catalog CatalogService, targetProvider TargetProvider) *Service {
	return &Service{artifacts: artifacts, catalog: catalog, target: targetProvider}
}

func (service *Service) Resolve(ctx context.Context, traceID string) (Resolved, *consolecore.Error) {
	if strings.TrimSpace(traceID) == "" || len(traceID) > MaxTraceIDLength {
		return Resolved{}, consolecore.NewError(consolecore.CodeInvalidArgument, "A nonblank trace ID of at most 8192 characters is required.", "", consolecore.Details{}, nil)
	}
	if service == nil || service.artifacts == nil {
		return Resolved{}, unavailable()
	}

	imported, domain := service.artifacts.Lookup(evidence.ForImported(), traceID)
	if domain != nil {
		return Resolved{}, translateUnavailable(domain)
	}

	var scope target.Scope
	hasTarget := false
	if service.target != nil {
		captured, captureDomain := service.target.Capture()
		if captureDomain == nil {
			scope, hasTarget = captured, true
		} else if captureDomain.Code != consolecore.CodeInvalidArgument {
			return Resolved{}, captureDomain
		}
	}

	var installed artifact.LookupResult
	if hasTarget {
		installed, domain = service.artifacts.Lookup(evidence.ForTarget(scope.ID), traceID)
		if domain != nil {
			return Resolved{}, translateUnavailable(domain)
		}
	}

	if installed.LocalAvailable && imported.LocalAvailable {
		return Resolved{}, ambiguous()
	}
	if installed.LocalAvailable {
		latestImported, lookupDomain := service.artifacts.Lookup(evidence.ForImported(), traceID)
		if lookupDomain != nil {
			return Resolved{}, translateUnavailable(lookupDomain)
		}
		if latestImported.LocalAvailable {
			return Resolved{}, ambiguous()
		}
		if domain := service.target.RequireCurrent(scope.ID); domain != nil {
			return Resolved{}, domain
		}
		return Resolved{Reference: evidence.ForTarget(scope.ID), Handle: installed.Handle, Scope: scope}, nil
	}
	if imported.LocalAvailable {
		if !hasTarget {
			return Resolved{Reference: evidence.ForImported(), Handle: imported.Handle}, nil
		}
		if service.catalog == nil {
			return Resolved{}, unavailable()
		}
		_, probeDomain := service.catalog.GetTrace(ctx, scope, traceID)
		if probeDomain == nil {
			return Resolved{}, ambiguous()
		}
		if probeDomain.Code != consolecore.CodeNotFound {
			return Resolved{}, probeDomain
		}
		if domain := service.target.RequireCurrent(scope.ID); domain != nil {
			return Resolved{}, domain
		}
		return Resolved{Reference: evidence.ForImported(), Handle: imported.Handle, Scope: scope}, nil
	}

	if !hasTarget {
		return Resolved{}, unavailable()
	}
	acquired, domain := service.artifacts.Acquire(ctx, scope, traceID)
	if domain != nil {
		return Resolved{}, translateUnavailable(domain)
	}
	latestImported, lookupDomain := service.artifacts.Lookup(evidence.ForImported(), traceID)
	if lookupDomain != nil {
		return Resolved{}, translateUnavailable(lookupDomain)
	}
	if latestImported.LocalAvailable {
		return Resolved{}, ambiguous()
	}
	if domain := service.target.RequireCurrent(scope.ID); domain != nil {
		return Resolved{}, domain
	}
	return Resolved{Reference: evidence.ForTarget(scope.ID), Handle: acquired.Handle, Scope: scope}, nil
}

func ambiguous() *consolecore.Error {
	return consolecore.NewError(consolecore.CodeAmbiguousTrace, "Multiple trace evidence instances use this trace ID. Resolve the conflict in Console before inspection.", "", consolecore.Details{}, nil)
}

func unavailable() *consolecore.Error {
	return consolecore.NewError(consolecore.CodeTraceUnavailable, "Trace evidence is unavailable. Retry inspection by traceId after the evidence or target becomes available.", "", consolecore.Details{}, nil)
}

func translateUnavailable(domain *consolecore.Error) *consolecore.Error {
	if domain != nil && (domain.Code == consolecore.CodeNotFound || domain.Code == consolecore.CodeArtifactExpired) {
		return unavailable()
	}
	return domain
}
