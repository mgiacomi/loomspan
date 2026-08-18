package traceresolution

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/evidence"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/observability"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/target"
)

type fakeArtifacts struct {
	lookups      map[evidence.Source]artifact.LookupResult
	lookupCalls  int
	acquired     artifact.AcquiredArtifact
	acquireError *consolecore.Error
	acquireCalls int
	acquire      func(context.Context, target.Scope, string) (artifact.AcquiredArtifact, *consolecore.Error)
	onLookup     func(evidence.Reference, string) (artifact.LookupResult, *consolecore.Error, bool)
}

func (fake *fakeArtifacts) Lookup(ref evidence.Reference, traceID string) (artifact.LookupResult, *consolecore.Error) {
	fake.lookupCalls++
	if fake.onLookup != nil {
		if result, domain, handled := fake.onLookup(ref, traceID); handled {
			return result, domain
		}
	}
	return fake.lookups[ref.Source], nil
}
func (fake *fakeArtifacts) Acquire(ctx context.Context, scope target.Scope, traceID string) (artifact.AcquiredArtifact, *consolecore.Error) {
	fake.acquireCalls++
	if fake.acquire != nil {
		return fake.acquire(ctx, scope, traceID)
	}
	return fake.acquired, fake.acquireError
}

type fakeCatalog struct {
	trace observability.Trace
	err   *consolecore.Error
	calls int
}

func (fake *fakeCatalog) GetTrace(context.Context, target.Scope, string) (observability.Trace, *consolecore.Error) {
	fake.calls++
	return fake.trace, fake.err
}

type fakeTarget struct {
	scope        target.Scope
	captureError *consolecore.Error
	currentError *consolecore.Error
	captureCalls int
	currentCalls int
}

func (fake *fakeTarget) Capture() (target.Scope, *consolecore.Error) {
	fake.captureCalls++
	return fake.scope, fake.captureError
}
func (fake *fakeTarget) RequireCurrent(target.ScopeID) *consolecore.Error {
	fake.currentCalls++
	return fake.currentError
}

func TestResolveRejectsBlankWhitespaceAndOversizedTraceIDBeforeLookup(t *testing.T) {
	artifacts := &fakeArtifacts{}
	targetProvider := &fakeTarget{}
	service := New(artifacts, &fakeCatalog{}, targetProvider)
	for _, traceID := range []string{"", " \t", strings.Repeat("x", MaxTraceIDLength+1)} {
		if _, domain := service.Resolve(context.Background(), traceID); domain == nil || domain.Code != consolecore.CodeInvalidArgument {
			t.Fatalf("traceID length=%d domain=%v", len(traceID), domain)
		}
	}
	if artifacts.lookupCalls != 0 || targetProvider.captureCalls != 0 {
		t.Fatalf("invalid identity reached collaborators: lookups=%d captures=%d", artifacts.lookupCalls, targetProvider.captureCalls)
	}
}

func TestResolveInstalledEvidenceDecisionTable(t *testing.T) {
	importedHandle := artifact.Handle(strings.Repeat("a", 64))
	targetHandle := artifact.Handle(strings.Repeat("b", 64))
	tests := []struct {
		name       string
		lookups    map[evidence.Source]artifact.LookupResult
		captureErr *consolecore.Error
		wantRef    evidence.Reference
		wantHandle artifact.Handle
		wantCode   consolecore.Code
	}{
		{"target", map[evidence.Source]artifact.LookupResult{evidence.SourceTarget: {LocalAvailable: true, Handle: targetHandle}}, nil, evidence.ForTarget("scope-1"), targetHandle, ""},
		{"target-free import", map[evidence.Source]artifact.LookupResult{evidence.SourceImported: {LocalAvailable: true, Handle: importedHandle}}, noTarget(), evidence.ForImported(), importedHandle, ""},
		{"collision", map[evidence.Source]artifact.LookupResult{evidence.SourceTarget: {LocalAvailable: true, Handle: targetHandle}, evidence.SourceImported: {LocalAvailable: true, Handle: importedHandle}}, nil, evidence.Reference{}, "", consolecore.CodeAmbiguousTrace},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			artifacts := &fakeArtifacts{lookups: test.lookups}
			service := New(artifacts, &fakeCatalog{}, &fakeTarget{scope: target.Scope{ID: "scope-1"}, captureError: test.captureErr})
			resolved, domain := service.Resolve(context.Background(), "trace-1")
			if test.wantCode != "" {
				if domain == nil || domain.Code != test.wantCode || artifacts.acquireCalls != 0 {
					t.Fatalf("resolved=%#v domain=%v acquireCalls=%d", resolved, domain, artifacts.acquireCalls)
				}
				return
			}
			if domain != nil || resolved.Reference != test.wantRef || resolved.Handle != test.wantHandle {
				t.Fatalf("resolved=%#v domain=%v", resolved, domain)
			}
		})
	}
}

func TestResolveMissingInstalledEvidenceDecisionTable(t *testing.T) {
	handle := artifact.Handle(strings.Repeat("c", 64))
	imported := artifact.LookupResult{LocalAvailable: true, Handle: handle}
	tests := []struct {
		name         string
		imported     artifact.LookupResult
		catalogError *consolecore.Error
		acquireError *consolecore.Error
		wantCode     consolecore.Code
		wantCatalog  int
		wantAcquire  int
		wantImported bool
	}{
		{"target acquisition", artifact.LookupResult{}, nil, nil, "", 0, 1, false},
		{"authoritative import fallback", imported, consolecore.NewError(consolecore.CodeNotFound, "missing", "", consolecore.Details{}, nil), nil, "", 1, 0, true},
		{"collision", imported, nil, nil, consolecore.CodeAmbiguousTrace, 1, 0, false},
		{"uncertain import", imported, consolecore.NewError(consolecore.CodeTargetAuthentication, "auth", "", consolecore.Details{}, nil), nil, consolecore.CodeTargetAuthentication, 1, 0, false},
		{"missing target", artifact.LookupResult{}, nil, consolecore.NewError(consolecore.CodeNotFound, "missing", "", consolecore.Details{}, nil), consolecore.CodeTraceUnavailable, 0, 1, false},
		{"expired target", artifact.LookupResult{}, nil, consolecore.NewError(consolecore.CodeArtifactExpired, "expired", "", consolecore.Details{}, nil), consolecore.CodeTraceUnavailable, 0, 1, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			artifacts := &fakeArtifacts{lookups: map[evidence.Source]artifact.LookupResult{evidence.SourceImported: test.imported}, acquired: artifact.AcquiredArtifact{Handle: handle}, acquireError: test.acquireError}
			catalog := &fakeCatalog{err: test.catalogError}
			service := New(artifacts, catalog, &fakeTarget{scope: target.Scope{ID: "scope-1"}})
			resolved, domain := service.Resolve(context.Background(), "trace-1")
			if test.wantCode == "" {
				if domain != nil || (resolved.Reference.Source == evidence.SourceImported) != test.wantImported || resolved.Scope.ID != "scope-1" {
					t.Fatalf("resolved=%#v domain=%v", resolved, domain)
				}
			} else if domain == nil || domain.Code != test.wantCode || strings.Contains(strings.ToLower(domain.Message), "handle") {
				t.Fatalf("resolved=%#v domain=%v", resolved, domain)
			}
			if catalog.calls != test.wantCatalog || artifacts.acquireCalls != test.wantAcquire {
				t.Fatalf("catalog=%d acquire=%d", catalog.calls, artifacts.acquireCalls)
			}
		})
	}
}

func TestResolveNeverReturnsEvidenceAcrossTargetRotation(t *testing.T) {
	artifacts := &fakeArtifacts{lookups: map[evidence.Source]artifact.LookupResult{evidence.SourceTarget: {LocalAvailable: true, Handle: artifact.Handle(strings.Repeat("d", 64))}}}
	targetProvider := &fakeTarget{scope: target.Scope{ID: "scope-1"}, currentError: consolecore.NewError(consolecore.CodeTargetChanged, "changed", "scope-1", consolecore.Details{}, nil)}
	resolved, domain := New(artifacts, &fakeCatalog{}, targetProvider).Resolve(context.Background(), "trace-1")
	if domain == nil || domain.Code != consolecore.CodeTargetChanged || resolved.Handle != "" {
		t.Fatalf("resolved=%#v domain=%v", resolved, domain)
	}
}

func TestResolveDetectsImportInstalledDuringTargetAcquisition(t *testing.T) {
	handle := artifact.Handle(strings.Repeat("f", 64))
	importedAvailable := false
	artifacts := &fakeArtifacts{
		lookups:  map[evidence.Source]artifact.LookupResult{},
		acquired: artifact.AcquiredArtifact{Handle: handle},
		onLookup: func(ref evidence.Reference, _ string) (artifact.LookupResult, *consolecore.Error, bool) {
			if ref.Source == evidence.SourceImported && importedAvailable {
				return artifact.LookupResult{LocalAvailable: true, Handle: artifact.Handle(strings.Repeat("a", 64))}, nil, true
			}
			return artifact.LookupResult{}, nil, false
		},
	}
	artifacts.acquire = func(context.Context, target.Scope, string) (artifact.AcquiredArtifact, *consolecore.Error) {
		importedAvailable = true
		return artifacts.acquired, nil
	}
	resolved, domain := New(artifacts, &fakeCatalog{}, &fakeTarget{scope: target.Scope{ID: "scope-1"}}).Resolve(context.Background(), "trace-1")
	if domain == nil || domain.Code != consolecore.CodeAmbiguousTrace || resolved.Handle != "" || artifacts.acquireCalls != 1 {
		t.Fatalf("resolved=%#v domain=%v acquireCalls=%d", resolved, domain, artifacts.acquireCalls)
	}
}

func TestResolveForwardsCancellationAndRejectsRotationDuringAcquisition(t *testing.T) {
	t.Run("cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		artifacts := &fakeArtifacts{lookups: map[evidence.Source]artifact.LookupResult{}, acquire: func(ctx context.Context, _ target.Scope, _ string) (artifact.AcquiredArtifact, *consolecore.Error) {
			if !errors.Is(ctx.Err(), context.Canceled) {
				t.Fatalf("acquisition context error=%v", ctx.Err())
			}
			return artifact.AcquiredArtifact{}, consolecore.NewError(consolecore.CodeConsoleError, "canceled", "", consolecore.Details{}, ctx.Err())
		}}
		_, domain := New(artifacts, &fakeCatalog{}, &fakeTarget{scope: target.Scope{ID: "scope-1"}}).Resolve(ctx, "trace-1")
		if domain == nil || !errors.Is(domain, context.Canceled) || artifacts.acquireCalls != 1 {
			t.Fatalf("domain=%v acquireCalls=%d", domain, artifacts.acquireCalls)
		}
	})

	t.Run("rotation during acquisition", func(t *testing.T) {
		targetProvider := &fakeTarget{scope: target.Scope{ID: "scope-1"}}
		artifacts := &fakeArtifacts{lookups: map[evidence.Source]artifact.LookupResult{}, acquire: func(context.Context, target.Scope, string) (artifact.AcquiredArtifact, *consolecore.Error) {
			targetProvider.currentError = consolecore.NewError(consolecore.CodeTargetChanged, "changed", "scope-1", consolecore.Details{}, nil)
			return artifact.AcquiredArtifact{Handle: artifact.Handle(strings.Repeat("e", 64))}, nil
		}}
		resolved, domain := New(artifacts, &fakeCatalog{}, targetProvider).Resolve(context.Background(), "trace-1")
		if domain == nil || domain.Code != consolecore.CodeTargetChanged || resolved.Handle != "" || targetProvider.currentCalls != 1 {
			t.Fatalf("resolved=%#v domain=%v currentCalls=%d", resolved, domain, targetProvider.currentCalls)
		}
	})
}

func noTarget() *consolecore.Error {
	return consolecore.NewError(consolecore.CodeInvalidArgument, "Select a target first.", "", consolecore.Details{}, nil)
}
