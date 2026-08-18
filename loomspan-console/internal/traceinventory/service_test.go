package traceinventory

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/evidence"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/observability"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/target"
)

type fakeArtifacts struct {
	snapshot artifact.StorageSnapshot
	lookups  map[string]artifact.LookupResult
}

func (fake *fakeArtifacts) StorageSnapshot() (artifact.StorageSnapshot, *consolecore.Error) {
	return fake.snapshot, nil
}
func (fake *fakeArtifacts) Lookup(ref evidence.Reference, traceID string) (artifact.LookupResult, *consolecore.Error) {
	return fake.lookups[string(ref.Source)+":"+traceID], nil
}

type fakeTarget struct {
	scope      target.Scope
	err        *consolecore.Error
	currentErr *consolecore.Error
	calls      int
}

func (fake *fakeTarget) Capture() (target.Scope, *consolecore.Error) {
	fake.calls++
	return fake.scope, fake.err
}
func (fake *fakeTarget) RequireCurrent(target.ScopeID) *consolecore.Error { return fake.currentErr }

type fakeCatalog struct {
	pages       map[string]observability.Page[observability.Trace]
	listError   *consolecore.Error
	probeErrors map[string]*consolecore.Error
	listCalls   int
	probeCalls  int
}

func (fake *fakeCatalog) ListTraces(_ context.Context, _ target.Scope, request observability.ListRequest) (observability.Page[observability.Trace], *consolecore.Error) {
	fake.listCalls++
	return fake.pages[request.Cursor], fake.listError
}
func (fake *fakeCatalog) GetTrace(_ context.Context, _ target.Scope, traceID string) (observability.Trace, *consolecore.Error) {
	fake.probeCalls++
	if domain, ok := fake.probeErrors[traceID]; ok {
		return observability.Trace{}, domain
	}
	return observability.Trace{TraceID: traceID}, nil
}

func TestInventoryCompletenessDecisionTable(t *testing.T) {
	when := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	imported := installed(evidence.SourceImported, "imported", when)
	tests := []struct {
		name       string
		target     *fakeTarget
		catalog    *fakeCatalog
		entries    []artifact.StoredEntry
		lookups    map[string]artifact.LookupResult
		wantItems  int
		complete   bool
		limitation bool
	}{
		{"no target imports complete", &fakeTarget{err: noTarget()}, &fakeCatalog{}, []artifact.StoredEntry{imported.stored}, imported.lookups, 1, true, false},
		{"selected target complete", &fakeTarget{scope: target.Scope{ID: "scope-1"}}, &fakeCatalog{pages: map[string]observability.Page[observability.Trace]{"": {ObservedAt: when, Items: []observability.Trace{}}}}, nil, map[string]artifact.LookupResult{}, 0, true, false},
		{"catalog failure incomplete", &fakeTarget{scope: target.Scope{ID: "scope-1"}}, &fakeCatalog{listError: consolecore.NewError(consolecore.CodeTargetUnavailable, "unavailable", "", consolecore.Details{}, nil)}, nil, map[string]artifact.LookupResult{}, 0, false, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, domain := New(&fakeArtifacts{snapshot: artifact.StorageSnapshot{Entries: test.entries}, lookups: test.lookups}, test.catalog, test.target, func() time.Time { return when }).List(context.Background(), Query{PageSize: 64})
			if domain != nil || len(result.Items) != test.wantItems || result.Complete != test.complete || (len(result.Limitations) > 0) != test.limitation {
				t.Fatalf("result=%#v domain=%v", result, domain)
			}
			if test.limitation && (len(result.Limitations) != 1 || result.Limitations[0].Code != LimitationTraceDiscoveryIncomplete || result.Limitations[0].Message != incompleteMessage) {
				t.Fatalf("limitations=%#v", result.Limitations)
			}
		})
	}
}

func TestInventoryConsolidatesTraceIdentityBeforePagination(t *testing.T) {
	when := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	targetInstalled := installed(evidence.SourceTarget, "same", when)
	importedInstalled := installed(evidence.SourceImported, "same", when.Add(-time.Minute))
	artifacts := &fakeArtifacts{snapshot: artifact.StorageSnapshot{Entries: []artifact.StoredEntry{targetInstalled.stored, importedInstalled.stored}}, lookups: merge(targetInstalled.lookups, importedInstalled.lookups)}
	catalog := &fakeCatalog{pages: map[string]observability.Page[observability.Trace]{"": {ObservedAt: when, Items: []observability.Trace{{TraceID: "same", FinalizedAt: when}, {TraceID: "catalog", FinalizedAt: when.Add(-2 * time.Minute)}}}}}
	service := New(artifacts, catalog, &fakeTarget{scope: target.Scope{ID: "scope-1"}}, func() time.Time { return when })
	result, domain := service.List(context.Background(), Query{PageSize: 2})
	if domain != nil || len(result.Items) != 2 || result.Items[0].TraceID != "same" || !result.Items[0].Ambiguous || result.Items[1].TraceID != "catalog" {
		t.Fatalf("result=%#v domain=%v", result, domain)
	}
	seen := map[string]bool{}
	for _, item := range result.Items {
		if seen[item.TraceID] {
			t.Fatalf("duplicate identity: %#v", result.Items)
		}
		seen[item.TraceID] = true
	}
}

func TestInventoryCanonicalizesAmbiguousMetadataAcrossContinuationCalls(t *testing.T) {
	base := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	olderTarget := installed(evidence.SourceTarget, "same", base.Add(-2*time.Minute))
	newerImport := installed(evidence.SourceImported, "same", base)
	middle := installed(evidence.SourceImported, "middle", base.Add(-time.Minute))
	artifacts := &fakeArtifacts{
		snapshot: artifact.StorageSnapshot{Entries: []artifact.StoredEntry{olderTarget.stored, newerImport.stored, middle.stored}},
		lookups:  merge(olderTarget.lookups, newerImport.lookups, middle.lookups),
	}
	catalog := &fakeCatalog{pages: map[string]observability.Page[observability.Trace]{"": {ObservedAt: base}}}
	service := New(artifacts, catalog, &fakeTarget{scope: target.Scope{ID: "scope-1"}}, func() time.Time { return base })
	first, domain := service.List(context.Background(), Query{PageSize: 1})
	if domain != nil || len(first.Items) != 1 || first.Items[0].TraceID != "same" || !first.Items[0].Ambiguous || !first.HasMore {
		t.Fatalf("first=%#v domain=%v", first, domain)
	}
	artifacts.snapshot.Entries[0], artifacts.snapshot.Entries[1] = artifacts.snapshot.Entries[1], artifacts.snapshot.Entries[0]
	second, domain := service.List(context.Background(), Query{PageSize: 1, Continuation: first.Continuation})
	if domain != nil || len(second.Items) != 1 || second.Items[0].TraceID != "middle" {
		t.Fatalf("second=%#v domain=%v", second, domain)
	}
}

func TestInventoryMarksImportedCatalogCollisionAndSuppressesCatalogRow(t *testing.T) {
	when := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	value := installed(evidence.SourceImported, "same", when)
	catalog := &fakeCatalog{pages: map[string]observability.Page[observability.Trace]{"": {ObservedAt: when, Items: []observability.Trace{{TraceID: "same", FinalizedAt: when}}}}, probeErrors: map[string]*consolecore.Error{}}
	result, domain := New(&fakeArtifacts{snapshot: artifact.StorageSnapshot{Entries: []artifact.StoredEntry{value.stored}}, lookups: value.lookups}, catalog, &fakeTarget{scope: target.Scope{ID: "scope-1"}}, func() time.Time { return when }).List(context.Background(), Query{PageSize: 64})
	if domain != nil || len(result.Items) != 1 || !result.Items[0].Ambiguous || catalog.probeCalls != 1 {
		t.Fatalf("result=%#v domain=%v probes=%d", result, domain, catalog.probeCalls)
	}
}

func TestInventoryChecksCatalogCompletenessWhenInstalledPageIsFull(t *testing.T) {
	when := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	newer := installed(evidence.SourceTarget, "newer", when)
	older := installed(evidence.SourceTarget, "older", when.Add(-time.Minute))
	catalog := &fakeCatalog{listError: consolecore.NewError(consolecore.CodeTargetUnavailable, "unavailable", "", consolecore.Details{}, nil)}
	result, domain := New(
		&fakeArtifacts{snapshot: artifact.StorageSnapshot{Entries: []artifact.StoredEntry{newer.stored, older.stored}}, lookups: merge(newer.lookups, older.lookups)},
		catalog,
		&fakeTarget{scope: target.Scope{ID: "scope-1"}},
		func() time.Time { return when },
	).List(context.Background(), Query{PageSize: 1})
	if domain != nil || !result.HasMore || result.Complete || len(result.Limitations) != 1 || catalog.listCalls != 1 {
		t.Fatalf("result=%#v domain=%v catalogCalls=%d", result, domain, catalog.listCalls)
	}
}

func TestInventoryHasMoreRequiresAnUnlistedCatalogTrace(t *testing.T) {
	when := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	value := installed(evidence.SourceTarget, "same", when)
	next := "next"
	artifacts := &fakeArtifacts{snapshot: artifact.StorageSnapshot{Entries: []artifact.StoredEntry{value.stored}}, lookups: value.lookups}
	for _, test := range []struct {
		name        string
		second      observability.Trace
		wantHasMore bool
		wantItems   int
	}{
		{"duplicates exhausted", observability.Trace{TraceID: "same", FinalizedAt: when}, false, 0},
		{"unique trace remains", observability.Trace{TraceID: "other", FinalizedAt: when.Add(-time.Minute)}, true, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			catalog := &fakeCatalog{pages: map[string]observability.Page[observability.Trace]{
				"":     {ObservedAt: when, Items: []observability.Trace{{TraceID: "same", FinalizedAt: when}}, HasMore: true, NextCursor: &next},
				"next": {ObservedAt: when, Items: []observability.Trace{test.second}},
			}}
			service := New(artifacts, catalog, &fakeTarget{scope: target.Scope{ID: "scope-1"}}, func() time.Time { return when })
			first, domain := service.List(context.Background(), Query{PageSize: 1})
			if domain != nil || first.HasMore != test.wantHasMore || (first.Continuation != "") != test.wantHasMore || catalog.listCalls != 2 {
				t.Fatalf("first=%#v domain=%v catalogCalls=%d", first, domain, catalog.listCalls)
			}
			if !test.wantHasMore {
				return
			}
			second, domain := service.List(context.Background(), Query{PageSize: 1, Continuation: first.Continuation})
			if domain != nil || len(second.Items) != test.wantItems || second.Items[0].TraceID != "other" {
				t.Fatalf("second=%#v domain=%v", second, domain)
			}
		})
	}
}

func TestUnifiedInventoryContinuationRejectsChangedSelectionAndInstalledSet(t *testing.T) {
	when := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	firstValue := installed(evidence.SourceImported, "newer", when)
	secondValue := installed(evidence.SourceImported, "older", when.Add(-time.Minute))
	artifacts := &fakeArtifacts{snapshot: artifact.StorageSnapshot{Entries: []artifact.StoredEntry{firstValue.stored, secondValue.stored}}, lookups: merge(firstValue.lookups, secondValue.lookups)}
	service := New(artifacts, nil, &fakeTarget{err: noTarget()}, func() time.Time { return when })
	first, domain := service.List(context.Background(), Query{PageSize: 1})
	if domain != nil || !first.HasMore || first.Items[0].TraceID != "newer" {
		t.Fatalf("first=%#v domain=%v", first, domain)
	}
	second, domain := service.List(context.Background(), Query{PageSize: 1, Continuation: first.Continuation})
	if domain != nil || len(second.Items) != 1 || second.Items[0].TraceID != "older" {
		t.Fatalf("second=%#v domain=%v", second, domain)
	}
	if _, domain = service.List(context.Background(), Query{PageSize: 2, Continuation: first.Continuation}); domain == nil || domain.Code != consolecore.CodeInvalidCursor {
		t.Fatalf("changed query domain=%v", domain)
	}
	thirdValue := installed(evidence.SourceImported, "changed", when.Add(-2*time.Minute))
	artifacts.snapshot.Entries = append(artifacts.snapshot.Entries, thirdValue.stored)
	artifacts.lookups = merge(artifacts.lookups, thirdValue.lookups)
	if _, domain = service.List(context.Background(), Query{PageSize: 1, Continuation: first.Continuation}); domain == nil || domain.Code != consolecore.CodeInvalidCursor {
		t.Fatalf("changed installed set domain=%v", domain)
	}
	for _, token := range []string{"%%%", strings.Repeat("a", maxContinuationLength+1)} {
		if _, domain = service.List(context.Background(), Query{PageSize: 1, Continuation: token}); domain == nil || domain.Code != consolecore.CodeInvalidCursor {
			t.Fatalf("token accepted: %v", domain)
		}
	}
}

func TestInventorySuppressesResultAfterTargetRotation(t *testing.T) {
	when := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	targetProvider := &fakeTarget{
		scope:      target.Scope{ID: "scope-1"},
		currentErr: consolecore.NewError(consolecore.CodeTargetChanged, "changed", "scope-1", consolecore.Details{}, nil),
	}
	result, domain := New(
		&fakeArtifacts{lookups: map[string]artifact.LookupResult{}},
		&fakeCatalog{pages: map[string]observability.Page[observability.Trace]{"": {ObservedAt: when}}},
		targetProvider,
		func() time.Time { return when },
	).List(context.Background(), Query{PageSize: 1})
	if domain == nil || domain.Code != consolecore.CodeTargetChanged || len(result.Items) != 0 {
		t.Fatalf("result=%#v domain=%v", result, domain)
	}
}

type installedValue struct {
	stored  artifact.StoredEntry
	lookups map[string]artifact.LookupResult
}

func installed(source evidence.Source, traceID string, finalized time.Time) installedValue {
	owner := evidence.Target("scope-1")
	targetScope := "scope-1"
	if source == evidence.SourceImported {
		owner, _ = evidence.Imported("owner")
		targetScope = ""
	}
	handle := artifact.Handle(strings.Repeat(string(traceID[0]), 64))
	lookup := artifact.LookupResult{Owner: owner, Handle: handle, Metadata: artifact.TraceMetadata{TraceID: traceID, SessionID: "session-" + traceID, EntrySkill: "skill", Outcome: "SUCCEEDED", FinalizedAt: finalized}, LocalAvailable: true}
	return installedValue{artifact.StoredEntry{Source: source, TargetScopeID: targetScope, TraceID: traceID, FinalizedAt: finalized}, map[string]artifact.LookupResult{string(source) + ":" + traceID: lookup}}
}

func merge(maps ...map[string]artifact.LookupResult) map[string]artifact.LookupResult {
	result := map[string]artifact.LookupResult{}
	for _, values := range maps {
		for key, value := range values {
			result[key] = value
		}
	}
	return result
}

func noTarget() *consolecore.Error {
	return consolecore.NewError(consolecore.CodeInvalidArgument, "Select a target first.", "", consolecore.Details{}, nil)
}
