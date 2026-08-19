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
	probes      map[string]observability.Trace
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
	if value, ok := fake.probes[traceID]; ok {
		return value, nil
	}
	return observability.Trace{}, consolecore.NewError(consolecore.CodeNotFound, "not found", "", consolecore.Details{}, nil)
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
	catalog := &fakeCatalog{pages: map[string]observability.Page[observability.Trace]{"": {ObservedAt: when, Items: []observability.Trace{{TraceID: "same", FinalizedAt: when}}}}, probes: map[string]observability.Trace{"same": {TraceID: "same", FinalizedAt: when}}}
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
	if domain != nil || result.HasMore || result.Complete || len(result.Items) != 1 || len(result.Limitations) != 1 || catalog.listCalls != 1 {
		t.Fatalf("result=%#v domain=%v catalogCalls=%d", result, domain, catalog.listCalls)
	}
}

func TestInventoryContinuationRepresentsRemainingCatalogWork(t *testing.T) {
	when := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	value := installed(evidence.SourceTarget, "same", when)
	next := "next"
	artifacts := &fakeArtifacts{snapshot: artifact.StorageSnapshot{Entries: []artifact.StoredEntry{value.stored}}, lookups: value.lookups}
	for _, test := range []struct {
		name      string
		second    observability.Trace
		wantItems int
	}{
		{"duplicates exhausted", observability.Trace{TraceID: "same", FinalizedAt: when}, 0},
		{"unique trace remains", observability.Trace{TraceID: "other", FinalizedAt: when.Add(-time.Minute)}, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			catalog := &fakeCatalog{pages: map[string]observability.Page[observability.Trace]{
				"":     {ObservedAt: when, Items: []observability.Trace{{TraceID: "same", FinalizedAt: when}}, HasMore: true, NextCursor: &next},
				"next": {ObservedAt: when, Items: []observability.Trace{test.second}},
			}}
			service := New(artifacts, catalog, &fakeTarget{scope: target.Scope{ID: "scope-1"}}, func() time.Time { return when })
			first, domain := service.List(context.Background(), Query{PageSize: 1})
			if domain != nil || len(first.Items) != 1 || !first.HasMore || first.Complete || first.Continuation == "" || catalog.listCalls != 1 {
				t.Fatalf("first=%#v domain=%v catalogCalls=%d", first, domain, catalog.listCalls)
			}
			second, domain := service.List(context.Background(), Query{PageSize: 1, Continuation: first.Continuation})
			if domain != nil || len(second.Items) != test.wantItems || second.HasMore || !second.Complete || catalog.listCalls != 2 {
				t.Fatalf("second=%#v domain=%v catalogCalls=%d", second, domain, catalog.listCalls)
			}
			if test.wantItems == 1 && second.Items[0].TraceID != "other" {
				t.Fatalf("second=%#v", second)
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

func TestInventoryFiltersSourceIdentityAndIndependentTimesOnOneInstance(t *testing.T) {
	base := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	targetValue := installed(evidence.SourceTarget, "target-trace", base.Add(-2*time.Hour))
	importValue := installed(evidence.SourceImported, "import-trace", base.Add(-24*time.Hour))
	targetLookup := targetValue.lookups["TARGET:target-trace"]
	targetLookup.AcquiredAt = base.Add(-time.Minute)
	targetLookup.Metadata.EntrySkill = "target.skill"
	targetValue.lookups["TARGET:target-trace"] = targetLookup
	importLookup := importValue.lookups["IMPORTED:import-trace"]
	importLookup.AcquiredAt = base.Add(-2 * time.Minute)
	importLookup.Metadata.EntrySkill = "import.skill"
	importLookup.Metadata.SessionID = "import-session"
	importValue.lookups["IMPORTED:import-trace"] = importLookup
	service := New(
		&fakeArtifacts{snapshot: artifact.StorageSnapshot{Entries: []artifact.StoredEntry{targetValue.stored, importValue.stored}}, lookups: merge(targetValue.lookups, importValue.lookups)},
		&fakeCatalog{pages: map[string]observability.Page[observability.Trace]{"": {ObservedAt: base}}},
		&fakeTarget{scope: target.Scope{ID: "scope-1"}}, func() time.Time { return base },
	)
	queries := []struct {
		name string
		q    Query
		want string
	}{
		{"target source", Query{Sources: []EvidenceSource{SourceTarget}}, "target-trace"},
		{"import source", Query{Sources: []EvidenceSource{SourceImported}}, "import-trace"},
		{"entry skill", Query{EntrySkill: "import.skill"}, "import-trace"},
		{"session", Query{SessionID: "import-session"}, "import-trace"},
		{"acquired window", Query{AcquiredFrom: timePointer(base.Add(-90 * time.Second))}, "target-trace"},
		{"imported window", Query{ImportedFrom: timePointer(base.Add(-3 * time.Minute)), ImportedTo: timePointer(base.Add(-2 * time.Minute))}, "import-trace"},
	}
	for _, test := range queries {
		t.Run(test.name, func(t *testing.T) {
			result, domain := service.List(context.Background(), test.q)
			if domain != nil || len(result.Items) != 1 || result.Items[0].TraceID != test.want {
				t.Fatalf("result=%#v domain=%v", result, domain)
			}
		})
	}
	for _, order := range []Order{OrderFinalizedDesc, OrderAcquiredDesc, OrderImportedDesc} {
		result, domain := service.List(context.Background(), Query{Order: order})
		if domain != nil || len(result.Items) != 2 {
			t.Fatalf("order=%s result=%#v domain=%v", order, result, domain)
		}
	}
	finalizedFrom := base.Add(-25 * time.Hour)
	importedFrom := base.Add(-3 * time.Minute)
	combined, domain := service.List(context.Background(), Query{
		Sources: []EvidenceSource{SourceImported}, Outcomes: []string{"SUCCEEDED"}, EntrySkill: "import.skill", SessionID: "import-session",
		FinalizedFrom: &finalizedFrom, ImportedFrom: &importedFrom, ImportedTo: timePointer(base.Add(-2 * time.Minute)), Order: OrderImportedDesc,
	})
	if domain != nil || len(combined.Items) != 1 || combined.Items[0].TraceID != "import-trace" {
		t.Fatalf("combined result=%+v domain=%v", combined, domain)
	}
	acquiredFrom := base.Add(-90 * time.Second)
	if crossed, domain := service.List(context.Background(), Query{AcquiredFrom: &acquiredFrom, ImportedFrom: &importedFrom}); domain != nil || len(crossed.Items) != 0 {
		t.Fatalf("cross-instance filters matched: result=%+v domain=%v", crossed, domain)
	}
	from, to := base, base.Add(-time.Second)
	if _, domain := service.List(context.Background(), Query{FinalizedFrom: &from, FinalizedTo: &to}); domain == nil || domain.Code != consolecore.CodeInvalidArgument {
		t.Fatalf("inverted range domain=%v", domain)
	}
	if _, domain := service.List(context.Background(), Query{Sources: []EvidenceSource{"UNKNOWN"}}); domain == nil || domain.Code != consolecore.CodeInvalidArgument {
		t.Fatalf("unknown source domain=%v", domain)
	}
}

func TestInventoryEqualTimeOrderingUsesDeterministicTraceIDTieBreak(t *testing.T) {
	base := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	b := installed(evidence.SourceImported, "trace-b", base)
	a := installed(evidence.SourceImported, "trace-a", base)
	for key, value := range merge(a.lookups, b.lookups) {
		value.AcquiredAt = base
		if strings.Contains(key, "trace-a") {
			a.lookups[key] = value
		} else {
			b.lookups[key] = value
		}
	}
	service := New(&fakeArtifacts{snapshot: artifact.StorageSnapshot{Entries: []artifact.StoredEntry{b.stored, a.stored}}, lookups: merge(a.lookups, b.lookups)}, nil, &fakeTarget{err: noTarget()}, func() time.Time { return base })
	for _, order := range []Order{OrderFinalizedDesc, OrderImportedDesc} {
		result, domain := service.List(context.Background(), Query{Order: order})
		if domain != nil || len(result.Items) != 2 || result.Items[0].TraceID != "trace-a" || result.Items[1].TraceID != "trace-b" {
			t.Fatalf("order=%s result=%+v domain=%v", order, result, domain)
		}
	}
}

func timePointer(value time.Time) *time.Time { return &value }

func TestInventoryGloballyMergesInstalledAndCatalogFinalizedOrder(t *testing.T) {
	base := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	old := installed(evidence.SourceTarget, "installed-old", base.Add(-time.Hour))
	catalog := &fakeCatalog{pages: map[string]observability.Page[observability.Trace]{"": {ObservedAt: base, Items: []observability.Trace{{TraceID: "catalog-new", SessionID: "catalog-session", EntrySkill: "catalog.skill", Outcome: "SUCCEEDED", FinalizedAt: base}}}}}
	service := New(&fakeArtifacts{snapshot: artifact.StorageSnapshot{Entries: []artifact.StoredEntry{old.stored}}, lookups: old.lookups}, catalog, &fakeTarget{scope: target.Scope{ID: "scope-1"}}, func() time.Time { return base })
	first, domain := service.List(context.Background(), Query{PageSize: 1, Order: OrderFinalizedDesc})
	if domain != nil || len(first.Items) != 1 || first.Items[0].TraceID != "catalog-new" || !first.HasMore {
		t.Fatalf("first=%+v domain=%v", first, domain)
	}
	second, domain := service.List(context.Background(), Query{PageSize: 1, Order: OrderFinalizedDesc, Continuation: first.Continuation})
	if domain != nil || len(second.Items) != 1 || second.Items[0].TraceID != "installed-old" || second.HasMore || !second.Complete {
		t.Fatalf("second=%+v domain=%v", second, domain)
	}
}

func TestInventoryOrdersCollisionByMatchingInstanceAcrossCatalogPages(t *testing.T) {
	base := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	targetValue := installed(evidence.SourceTarget, "collision", base)
	importValue := installed(evidence.SourceImported, "collision", base.Add(-3*time.Hour))
	targetLookup := targetValue.lookups["TARGET:collision"]
	targetLookup.AcquiredAt = base.Add(time.Hour)
	targetLookup.Metadata.EntrySkill = "other.skill"
	targetValue.lookups["TARGET:collision"] = targetLookup
	importLookup := importValue.lookups["IMPORTED:collision"]
	importLookup.AcquiredAt = base.Add(2 * time.Hour)
	importLookup.Metadata.EntrySkill = "wanted.skill"
	importValue.lookups["IMPORTED:collision"] = importLookup

	next := "next"
	catalog := &fakeCatalog{pages: map[string]observability.Page[observability.Trace]{
		"": {
			ObservedAt: base,
			Items:      []observability.Trace{{TraceID: "catalog-new", EntrySkill: "wanted.skill", FinalizedAt: base.Add(-time.Hour)}},
			HasMore:    true, NextCursor: &next,
		},
		"next": {
			ObservedAt: base,
			Items:      []observability.Trace{{TraceID: "catalog-middle", EntrySkill: "wanted.skill", FinalizedAt: base.Add(-2 * time.Hour)}},
		},
	}}
	service := New(
		&fakeArtifacts{snapshot: artifact.StorageSnapshot{Entries: []artifact.StoredEntry{targetValue.stored, importValue.stored}}, lookups: merge(targetValue.lookups, importValue.lookups)},
		catalog, &fakeTarget{scope: target.Scope{ID: "scope-1"}}, func() time.Time { return base },
	)
	query := Query{PageSize: 2, EntrySkill: "wanted.skill", Order: OrderAcquiredDesc}
	first, domain := service.List(context.Background(), query)
	if domain != nil || len(first.Items) != 1 || first.Items[0].TraceID != "catalog-new" || !first.HasMore {
		t.Fatalf("first=%+v domain=%v", first, domain)
	}
	query.Continuation = first.Continuation
	second, domain := service.List(context.Background(), query)
	if domain != nil || len(second.Items) != 2 || second.Items[0].TraceID != "catalog-middle" || second.Items[1].TraceID != "collision" || second.HasMore || !second.Complete {
		t.Fatalf("second=%+v domain=%v", second, domain)
	}
}

func TestImportWithoutValidatedEntrySkillRemainsDiscoverableWithLimitation(t *testing.T) {
	base := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	value := installed(evidence.SourceImported, "filename-claims-skill", base)
	lookup := value.lookups["IMPORTED:filename-claims-skill"]
	lookup.Metadata.EntrySkill = ""
	lookup.AcquiredAt = base.Add(time.Minute)
	value.lookups["IMPORTED:filename-claims-skill"] = lookup
	service := New(&fakeArtifacts{snapshot: artifact.StorageSnapshot{Entries: []artifact.StoredEntry{value.stored}}, lookups: value.lookups}, &fakeCatalog{pages: map[string]observability.Page[observability.Trace]{"": {ObservedAt: base}}}, &fakeTarget{scope: target.Scope{ID: "scope-1"}}, func() time.Time { return base })
	result, domain := service.List(context.Background(), Query{Sources: []EvidenceSource{SourceImported}})
	if domain != nil || len(result.Items) != 1 || result.Items[0].EntrySkill != nil {
		t.Fatalf("result=%+v domain=%v", result, domain)
	}
	if len(result.Limitations) != 1 || result.Limitations[0].Code != LimitationImportedEntrySkillUnavailable {
		t.Fatalf("limitations=%+v", result.Limitations)
	}
}

func TestOldFinalizedImportIsSelectedByImportedFactsAndOrder(t *testing.T) {
	base := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	recentlyImported := installed(evidence.SourceImported, "old-finalized", base.Add(-30*24*time.Hour))
	lookup := recentlyImported.lookups["IMPORTED:old-finalized"]
	lookup.AcquiredAt = base
	lookup.Metadata.SessionID = "wanted-session"
	recentlyImported.lookups["IMPORTED:old-finalized"] = lookup
	olderImport := installed(evidence.SourceImported, "new-finalized", base)
	lookup = olderImport.lookups["IMPORTED:new-finalized"]
	lookup.AcquiredAt = base.Add(-time.Hour)
	olderImport.lookups["IMPORTED:new-finalized"] = lookup
	service := New(&fakeArtifacts{
		snapshot: artifact.StorageSnapshot{Entries: []artifact.StoredEntry{recentlyImported.stored, olderImport.stored}},
		lookups:  merge(recentlyImported.lookups, olderImport.lookups),
	}, nil, &fakeTarget{err: noTarget()}, func() time.Time { return base })

	from := base.Add(-time.Minute)
	result, domain := service.List(context.Background(), Query{
		Sources: []EvidenceSource{SourceImported}, SessionID: "wanted-session",
		ImportedFrom: &from, Order: OrderImportedDesc,
	})
	if domain != nil || len(result.Items) != 1 || result.Items[0].TraceID != "old-finalized" || result.Items[0].ImportedAt == nil || !result.Items[0].ImportedAt.Equal(base) {
		t.Fatalf("result=%+v domain=%v", result, domain)
	}
}

func TestCatalogTargetGainsAcquiredTimeWithoutChangingFinalization(t *testing.T) {
	base := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	trace := observability.Trace{TraceID: "target-trace", SessionID: "session", EntrySkill: "target.skill", Outcome: "SUCCEEDED", FinalizedAt: base.Add(-time.Hour)}
	catalog := &fakeCatalog{pages: map[string]observability.Page[observability.Trace]{"": {ObservedAt: base, Items: []observability.Trace{trace}}}}
	artifacts := &fakeArtifacts{lookups: map[string]artifact.LookupResult{}}
	service := New(artifacts, catalog, &fakeTarget{scope: target.Scope{ID: "scope-1"}}, func() time.Time { return base })

	before, domain := service.List(context.Background(), Query{})
	if domain != nil || len(before.Items) != 1 || before.Items[0].AcquiredAt != nil || before.Items[0].FinalizedAt == nil || !before.Items[0].FinalizedAt.Equal(trace.FinalizedAt) {
		t.Fatalf("catalog result=%+v domain=%v", before, domain)
	}
	installedTarget := installed(evidence.SourceTarget, trace.TraceID, trace.FinalizedAt)
	lookup := installedTarget.lookups["TARGET:target-trace"]
	lookup.AcquiredAt = base
	lookup.Metadata.SessionID, lookup.Metadata.EntrySkill, lookup.Metadata.Outcome = trace.SessionID, trace.EntrySkill, trace.Outcome
	installedTarget.lookups["TARGET:target-trace"] = lookup
	artifacts.snapshot.Entries = []artifact.StoredEntry{installedTarget.stored}
	artifacts.lookups = installedTarget.lookups
	after, domain := service.List(context.Background(), Query{})
	if domain != nil || len(after.Items) != 1 || after.Items[0].AcquiredAt == nil || !after.Items[0].AcquiredAt.Equal(base) || after.Items[0].FinalizedAt == nil || !after.Items[0].FinalizedAt.Equal(trace.FinalizedAt) {
		t.Fatalf("acquired result=%+v domain=%v", after, domain)
	}
}

func TestCollisionRemainsAmbiguousUnderSourceFilterAndSuppressesConflicts(t *testing.T) {
	base := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	targetValue := installed(evidence.SourceTarget, "same", base)
	importValue := installed(evidence.SourceImported, "same", base.Add(-time.Hour))
	targetLookup := targetValue.lookups["TARGET:same"]
	targetLookup.AcquiredAt = base.Add(-time.Minute)
	targetLookup.Metadata.SessionID = "target-session"
	targetValue.lookups["TARGET:same"] = targetLookup
	importLookup := importValue.lookups["IMPORTED:same"]
	importLookup.AcquiredAt = base
	importLookup.Metadata.SessionID = "import-session"
	importLookup.Metadata.Outcome = "FAILED"
	importValue.lookups["IMPORTED:same"] = importLookup
	service := New(&fakeArtifacts{
		snapshot: artifact.StorageSnapshot{Entries: []artifact.StoredEntry{targetValue.stored, importValue.stored}},
		lookups:  merge(targetValue.lookups, importValue.lookups),
	}, &fakeCatalog{pages: map[string]observability.Page[observability.Trace]{"": {ObservedAt: base}}}, &fakeTarget{scope: target.Scope{ID: "scope-1"}}, func() time.Time { return base })

	for _, source := range []EvidenceSource{SourceTarget, SourceImported} {
		result, domain := service.List(context.Background(), Query{Sources: []EvidenceSource{source}})
		if domain != nil || len(result.Items) != 1 || !result.Items[0].Ambiguous || len(result.Items[0].EvidenceSources) != 2 || result.Items[0].SessionID != nil || result.Items[0].Outcome != nil || result.Items[0].FinalizedAt != nil || result.Items[0].AcquiredAt == nil || result.Items[0].ImportedAt == nil {
			t.Fatalf("source=%s result=%+v domain=%v", source, result, domain)
		}
	}
}

func TestInventoryRejectsEveryUnknownClosedValue(t *testing.T) {
	service := New(&fakeArtifacts{}, nil, &fakeTarget{err: noTarget()}, time.Now)
	for name, query := range map[string]Query{
		"source":  {Sources: []EvidenceSource{"UNKNOWN"}},
		"outcome": {Outcomes: []string{"UNKNOWN"}},
		"order":   {Order: "UNKNOWN"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, domain := service.List(context.Background(), query); domain == nil || domain.Code != consolecore.CodeInvalidArgument {
				t.Fatalf("domain=%v", domain)
			}
		})
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
