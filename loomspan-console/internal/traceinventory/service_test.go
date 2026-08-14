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

func (f fakeArtifacts) StorageSnapshot() (artifact.StorageSnapshot, *consolecore.Error) {
	return f.snapshot, nil
}
func (f fakeArtifacts) Lookup(ref evidence.Reference, traceID string) (artifact.LookupResult, *consolecore.Error) {
	return f.lookups[string(ref.Source)+":"+traceID], nil
}

type fakeTarget struct {
	scope target.Scope
	err   *consolecore.Error
	calls int
}

func (f *fakeTarget) Capture() (target.Scope, *consolecore.Error) { f.calls++; return f.scope, f.err }

type fakeCatalog struct {
	pages map[string]observability.Page[observability.Trace]
	calls int
}

func (f *fakeCatalog) ListTraces(_ context.Context, _ target.Scope, request observability.ListRequest) (observability.Page[observability.Trace], *consolecore.Error) {
	f.calls++
	return f.pages[request.Cursor], nil
}

func TestInventoryAllWithoutTargetReturnsImportedEntriesAndCatalogError(t *testing.T) {
	when := time.Date(2026, 8, 14, 8, 0, 0, 0, time.UTC)
	owner, _ := evidence.Imported("internal-owner")
	artifacts := fakeArtifacts{snapshot: artifact.StorageSnapshot{Entries: []artifact.StoredEntry{{Source: evidence.SourceImported, TraceID: "import-1", FinalizedAt: when, LocalAvailable: true}}}, lookups: map[string]artifact.LookupResult{"IMPORTED:import-1": {Owner: owner, Handle: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Metadata: artifact.TraceMetadata{TraceID: "import-1", SessionID: "session-1", FinalizedAt: when}, LocalAvailable: true}}}
	targetProvider := &fakeTarget{err: consolecore.NewError(consolecore.CodeInvalidArgument, "Select a target first.", "", consolecore.Details{}, nil)}
	catalog := &fakeCatalog{}
	result, domain := New(artifacts, catalog, targetProvider, func() time.Time { return when }).List(context.Background(), Query{SourceFilter: SourceFilterAll, PageSize: 64})
	if domain != nil || len(result.Items) != 1 || result.Items[0].Source != evidence.SourceImported || result.Items[0].TargetScopeID != "" || !result.Items[0].LocalAvailable {
		t.Fatalf("result=%#v domain=%v", result, domain)
	}
	if !result.ApplicationCatalog.Requested || result.ApplicationCatalog.Available || result.ApplicationCatalog.Error == nil || catalog.calls != 0 {
		t.Fatalf("catalog=%#v calls=%d", result.ApplicationCatalog, catalog.calls)
	}
}

func TestInventoryImportedNeverCapturesTargetAndPaginatesDeterministically(t *testing.T) {
	base := time.Date(2026, 8, 14, 8, 0, 0, 0, time.UTC)
	owner, _ := evidence.Imported("owner")
	entries := []artifact.StoredEntry{{Source: evidence.SourceImported, TraceID: "older", FinalizedAt: base.Add(-time.Minute)}, {Source: evidence.SourceImported, TraceID: "newer", FinalizedAt: base}}
	lookups := map[string]artifact.LookupResult{}
	for _, entry := range entries {
		lookups["IMPORTED:"+entry.TraceID] = artifact.LookupResult{Owner: owner, Handle: artifact.Handle(entry.TraceID + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), Metadata: artifact.TraceMetadata{TraceID: entry.TraceID, FinalizedAt: entry.FinalizedAt}, LocalAvailable: true}
	}
	targetProvider := &fakeTarget{}
	service := New(fakeArtifacts{snapshot: artifact.StorageSnapshot{Entries: entries}, lookups: lookups}, &fakeCatalog{}, targetProvider, func() time.Time { return base })
	first, domain := service.List(context.Background(), Query{SourceFilter: SourceFilterImported, PageSize: 1})
	if domain != nil || len(first.Items) != 1 || first.Items[0].TraceID != "newer" || !first.HasMore {
		t.Fatalf("first=%#v domain=%v", first, domain)
	}
	second, domain := service.List(context.Background(), Query{SourceFilter: SourceFilterImported, PageSize: 1, Continuation: first.Continuation})
	if domain != nil || len(second.Items) != 1 || second.Items[0].TraceID != "older" || targetProvider.calls != 0 {
		t.Fatalf("second=%#v domain=%v calls=%d", second, domain, targetProvider.calls)
	}
}

func TestInventoryCursorRejectsWrongQueryAndUnknownFields(t *testing.T) {
	key := installedKey{FinalizedAt: time.Now().UTC(), Source: "IMPORTED", TraceID: "trace"}
	token, err := encodeCursor(inventoryCursor{Schema: cursorSchemaV1, Operation: cursorOperation, Fingerprint: queryFingerprint(SourceFilterImported, 1, ""), Segment: segmentInstalled, Installed: &key})
	if err != nil {
		t.Fatal(err)
	}
	service := New(fakeArtifacts{}, nil, &fakeTarget{}, time.Now)
	_, domain := service.List(context.Background(), Query{SourceFilter: SourceFilterImported, PageSize: 2, Continuation: token})
	if domain == nil || domain.Code != consolecore.CodeInvalidCursor {
		t.Fatalf("domain=%v", domain)
	}
}

func TestInventoryTargetRequiresSelectionAndAllOrdersThenDeduplicatesCatalog(t *testing.T) {
	when := time.Date(2026, 8, 14, 9, 0, 0, 0, time.UTC)
	noTarget := &fakeTarget{err: consolecore.NewError(consolecore.CodeInvalidArgument, "Select a target first.", "", consolecore.Details{}, nil)}
	if _, domain := New(fakeArtifacts{}, nil, noTarget, func() time.Time { return when }).List(context.Background(), Query{SourceFilter: SourceFilterTarget, PageSize: 1}); domain == nil {
		t.Fatal("TARGET without target was accepted")
	}
	targetOwner := evidence.Target("scope-1")
	importOwner, _ := evidence.Imported("owner")
	handleA := artifact.Handle("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	handleB := artifact.Handle("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	artifacts := fakeArtifacts{snapshot: artifact.StorageSnapshot{Entries: []artifact.StoredEntry{{Source: evidence.SourceImported, TraceID: "imported", FinalizedAt: when}, {Source: evidence.SourceTarget, TargetScopeID: "scope-1", TraceID: "installed", FinalizedAt: when}}}, lookups: map[string]artifact.LookupResult{"TARGET:installed": {Owner: targetOwner, Handle: handleA, Metadata: artifact.TraceMetadata{TraceID: "installed", FinalizedAt: when}, LocalAvailable: true}, "IMPORTED:imported": {Owner: importOwner, Handle: handleB, Metadata: artifact.TraceMetadata{TraceID: "imported", FinalizedAt: when}, LocalAvailable: true}}}
	catalog := &fakeCatalog{pages: map[string]observability.Page[observability.Trace]{"": {Items: []observability.Trace{{TraceID: "installed", FinalizedAt: when}, {TraceID: "catalog-only", FinalizedAt: when.Add(-time.Minute)}}, ObservedAt: when}}}
	provider := &fakeTarget{scope: target.Scope{ID: "scope-1", InstanceID: "instance-1"}}
	result, domain := New(artifacts, catalog, provider, func() time.Time { return when }).List(context.Background(), Query{SourceFilter: SourceFilterAll, PageSize: 4})
	if domain != nil {
		t.Fatal(domain)
	}
	if len(result.Items) != 3 || result.Items[0].TraceID != "installed" || result.Items[1].TraceID != "imported" || result.Items[2].TraceID != "catalog-only" {
		t.Fatalf("items=%#v", result.Items)
	}
	if !result.ApplicationCatalog.Available || result.ApplicationCatalog.TargetScopeID != "scope-1" || result.ApplicationCatalog.InstanceID != "instance-1" {
		t.Fatalf("catalog=%#v", result.ApplicationCatalog)
	}
}

func TestInventoryContinuationUsesKeysetAcrossInstalledSetChange(t *testing.T) {
	base := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	owner, _ := evidence.Imported("owner")
	entries := []artifact.StoredEntry{{Source: evidence.SourceImported, TraceID: "newer", FinalizedAt: base}, {Source: evidence.SourceImported, TraceID: "older", FinalizedAt: base.Add(-time.Minute)}}
	lookups := map[string]artifact.LookupResult{}
	addLookup := func(entry artifact.StoredEntry) {
		lookups["IMPORTED:"+entry.TraceID] = artifact.LookupResult{Owner: owner, Handle: artifact.Handle(strings.Repeat(entry.TraceID[:1], 64)), Metadata: artifact.TraceMetadata{TraceID: entry.TraceID, FinalizedAt: entry.FinalizedAt}, LocalAvailable: true}
	}
	for _, entry := range entries {
		addLookup(entry)
	}
	artifacts := &mutableArtifacts{snapshot: artifact.StorageSnapshot{Entries: entries}, lookups: lookups}
	service := New(artifacts, nil, &fakeTarget{}, func() time.Time { return base })
	first, domain := service.List(context.Background(), Query{SourceFilter: SourceFilterImported, PageSize: 1})
	if domain != nil {
		t.Fatal(domain)
	}
	inserted := artifact.StoredEntry{Source: evidence.SourceImported, TraceID: "inserted", FinalizedAt: base.Add(time.Minute)}
	addLookup(inserted)
	artifacts.snapshot.Entries = append(artifacts.snapshot.Entries, inserted)
	second, domain := service.List(context.Background(), Query{SourceFilter: SourceFilterImported, PageSize: 1, Continuation: first.Continuation})
	if domain != nil || len(second.Items) != 1 || second.Items[0].TraceID != "older" {
		t.Fatalf("keyset continuation after insert: result=%#v domain=%v", second, domain)
	}
}

func TestInventoryCursorCarriesCompositeApplicationState(t *testing.T) {
	const upstream = "application-page-2"
	value := inventoryCursor{Schema: cursorSchemaV1, Operation: cursorOperation, Fingerprint: "fingerprint", Segment: segmentApplication, InstalledFingerprint: "installed-fingerprint", ApplicationCursor: upstream}
	token, err := encodeCursor(value)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeCursor(token)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.ApplicationCursor != upstream {
		t.Fatalf("application cursor=%q", decoded.ApplicationCursor)
	}
}

func TestInventoryApplicationContinuationRejectsInstalledRemovalBeforeCatalogDeduplication(t *testing.T) {
	when := time.Date(2026, 8, 14, 10, 45, 0, 0, time.UTC)
	entry := artifact.StoredEntry{Source: evidence.SourceTarget, TargetScopeID: "scope-1", TraceID: "installed", FinalizedAt: when}
	artifacts := &mutableArtifacts{
		snapshot: artifact.StorageSnapshot{Entries: []artifact.StoredEntry{entry}},
		lookups: map[string]artifact.LookupResult{
			"TARGET:installed": {Owner: evidence.Target("scope-1"), Handle: artifact.Handle(strings.Repeat("a", 64)), Metadata: artifact.TraceMetadata{TraceID: "installed", FinalizedAt: when}, LocalAvailable: true},
		},
	}
	catalog := &fakeCatalog{pages: map[string]observability.Page[observability.Trace]{"": {ObservedAt: when, Items: []observability.Trace{{TraceID: "installed", FinalizedAt: when}}}}}
	service := New(artifacts, catalog, &fakeTarget{scope: target.Scope{ID: "scope-1"}}, func() time.Time { return when })
	first, domain := service.List(context.Background(), Query{SourceFilter: SourceFilterTarget, PageSize: 1})
	if domain != nil || len(first.Items) != 1 || first.Items[0].TraceID != "installed" || !first.HasMore {
		t.Fatalf("first=%#v domain=%v", first, domain)
	}
	artifacts.snapshot.Entries = nil
	delete(artifacts.lookups, "TARGET:installed")
	second, domain := service.List(context.Background(), Query{SourceFilter: SourceFilterTarget, PageSize: 1, Continuation: first.Continuation})
	if domain == nil || domain.Code != consolecore.CodeInvalidCursor || len(second.Items) != 0 {
		t.Fatalf("second=%#v domain=%v", second, domain)
	}
}

func TestInventoryInstalledFullPageStillReportsObservedCatalogAvailability(t *testing.T) {
	when := time.Date(2026, 8, 14, 10, 30, 0, 0, time.UTC)
	owner := evidence.Target("scope-1")
	artifacts := fakeArtifacts{
		snapshot: artifact.StorageSnapshot{Entries: []artifact.StoredEntry{{Source: evidence.SourceTarget, TargetScopeID: "scope-1", TraceID: "installed", FinalizedAt: when}}},
		lookups:  map[string]artifact.LookupResult{"TARGET:installed": {Owner: owner, Handle: artifact.Handle(strings.Repeat("a", 64)), Metadata: artifact.TraceMetadata{TraceID: "installed", FinalizedAt: when}, LocalAvailable: true}},
	}
	catalog := &fakeCatalog{pages: map[string]observability.Page[observability.Trace]{"": {ObservedAt: when, Items: []observability.Trace{}}}}
	result, domain := New(artifacts, catalog, &fakeTarget{scope: target.Scope{ID: "scope-1"}}, func() time.Time { return when }).List(context.Background(), Query{SourceFilter: SourceFilterTarget, PageSize: 1})
	if domain != nil || len(result.Items) != 1 || !result.ApplicationCatalog.Requested || !result.ApplicationCatalog.Available || result.ApplicationCatalog.Error != nil || result.HasMore || catalog.calls != 1 {
		t.Fatalf("result=%#v domain=%v catalogCalls=%d", result, domain, catalog.calls)
	}
}

func TestInventoryContinuationRejectsTargetRotationMalformedAndOversized(t *testing.T) {
	base := time.Date(2026, 8, 14, 11, 0, 0, 0, time.UTC)
	owner := evidence.Target("scope-1")
	entries := []artifact.StoredEntry{{Source: evidence.SourceTarget, TargetScopeID: "scope-1", TraceID: "a", FinalizedAt: base}, {Source: evidence.SourceTarget, TargetScopeID: "scope-1", TraceID: "b", FinalizedAt: base.Add(-time.Minute)}}
	lookups := map[string]artifact.LookupResult{}
	for _, entry := range entries {
		lookups["TARGET:"+entry.TraceID] = artifact.LookupResult{Owner: owner, Handle: artifact.Handle(strings.Repeat(entry.TraceID, 64)), Metadata: artifact.TraceMetadata{TraceID: entry.TraceID, FinalizedAt: entry.FinalizedAt}, LocalAvailable: true}
	}
	provider := &fakeTarget{scope: target.Scope{ID: "scope-1", InstanceID: "one"}}
	service := New(fakeArtifacts{snapshot: artifact.StorageSnapshot{Entries: entries}, lookups: lookups}, &fakeCatalog{}, provider, func() time.Time { return base })
	first, domain := service.List(context.Background(), Query{SourceFilter: SourceFilterTarget, PageSize: 1})
	if domain != nil || !first.HasMore {
		t.Fatalf("first=%#v domain=%v", first, domain)
	}
	provider.scope = target.Scope{ID: "scope-2", InstanceID: "two"}
	if _, domain = service.List(context.Background(), Query{SourceFilter: SourceFilterTarget, PageSize: 1, Continuation: first.Continuation}); domain == nil || domain.Code != consolecore.CodeInvalidCursor {
		t.Fatalf("rotation domain=%v", domain)
	}
	for _, token := range []string{"%%%", strings.Repeat("a", maxContinuationLength+1)} {
		if _, domain = service.List(context.Background(), Query{SourceFilter: SourceFilterTarget, PageSize: 1, Continuation: token}); domain == nil || domain.Code != consolecore.CodeInvalidCursor {
			t.Fatalf("token accepted domain=%v", domain)
		}
	}
}

type mutableArtifacts struct {
	snapshot artifact.StorageSnapshot
	lookups  map[string]artifact.LookupResult
}

func (f *mutableArtifacts) StorageSnapshot() (artifact.StorageSnapshot, *consolecore.Error) {
	return f.snapshot, nil
}
func (f *mutableArtifacts) Lookup(ref evidence.Reference, id string) (artifact.LookupResult, *consolecore.Error) {
	return f.lookups[string(ref.Source)+":"+id], nil
}
