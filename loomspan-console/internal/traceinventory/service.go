package traceinventory

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/evidence"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/observability"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/target"
)

const (
	defaultPageSize = 64
	maxPageSize     = 64
)

const incompleteMessage = "Some trace evidence could not be checked; results may be incomplete."

type ArtifactService interface {
	StorageSnapshot() (artifact.StorageSnapshot, *consolecore.Error)
	Lookup(evidence.Reference, string) (artifact.LookupResult, *consolecore.Error)
}

type CatalogService interface {
	ListTraces(context.Context, target.Scope, observability.ListRequest) (observability.Page[observability.Trace], *consolecore.Error)
	GetTrace(context.Context, target.Scope, string) (observability.Trace, *consolecore.Error)
}

type TargetProvider interface {
	Capture() (target.Scope, *consolecore.Error)
	RequireCurrent(target.ScopeID) *consolecore.Error
}

type Service struct {
	artifacts ArtifactService
	catalog   CatalogService
	target    TargetProvider
	now       func() time.Time
}

func New(artifacts ArtifactService, catalog CatalogService, targetProvider TargetProvider, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{artifacts: artifacts, catalog: catalog, target: targetProvider, now: now}
}

func (service *Service) EnrichTargetCatalogPage(scopeID target.ScopeID, page observability.Page[observability.Trace]) observability.Page[observability.Trace] {
	if service == nil || service.artifacts == nil {
		return page
	}
	for index := range page.Items {
		lookup, domain := service.artifacts.Lookup(evidence.ForTarget(scopeID), page.Items[index].TraceID)
		if domain != nil || !lookup.LocalAvailable {
			continue
		}
		page.Items[index].LocalAvailable = true
		page.Items[index].ArtifactHandle = string(lookup.Handle)
		page.Items[index].ApplicationAvailability = string(lookup.ApplicationAvailability)
	}
	return page
}

type installedGroup struct {
	entry      Entry
	target     bool
	imported   bool
	identities []string
}

func (service *Service) List(ctx context.Context, query Query) (Result, *consolecore.Error) {
	pageSize := query.PageSize
	if pageSize == 0 {
		pageSize = defaultPageSize
	}
	if pageSize < 1 || pageSize > maxPageSize {
		return Result{}, invalid("The trace inventory page size must be from 1 through 64.")
	}
	if service == nil || service.artifacts == nil {
		return Result{}, consolecore.NewError(consolecore.CodeConsoleError, "Trace inventory is unavailable.", "", consolecore.Details{}, nil)
	}

	result := Result{ObservedAt: service.now().UTC(), Items: []Entry{}, Complete: true, Limitations: []Limitation{}}
	var scope target.Scope
	hasTarget := false
	if service.target != nil {
		captured, domain := service.target.Capture()
		if domain == nil {
			scope, hasTarget = captured, true
		} else if domain.Code != consolecore.CodeInvalidArgument {
			return Result{}, domain
		}
	}

	fingerprint := queryFingerprint(pageSize, string(scope.ID))
	segment := segmentInstalled
	var after *installedKey
	applicationCursor := ""
	continuedInstalledFingerprint := ""
	if query.Continuation != "" {
		cur, err := decodeCursor(query.Continuation)
		if err != nil || cur.Fingerprint != fingerprint {
			return Result{}, invalidCursor(err)
		}
		segment, after = cur.Segment, cur.Installed
		applicationCursor = cur.ApplicationCursor
		continuedInstalledFingerprint = cur.InstalledFingerprint
	}

	snapshot, domain := service.artifacts.StorageSnapshot()
	if domain != nil {
		return Result{}, domain
	}
	groups := map[string]*installedGroup{}
	for _, stored := range snapshot.Entries {
		if stored.Source == evidence.SourceTarget && (!hasTarget || stored.TargetScopeID != string(scope.ID)) {
			continue
		}
		ref := evidence.ForImported()
		if stored.Source == evidence.SourceTarget {
			ref = evidence.ForTarget(scope.ID)
		}
		lookup, lookupDomain := service.artifacts.Lookup(ref, stored.TraceID)
		if lookupDomain != nil {
			return Result{}, lookupDomain
		}
		if !lookup.LocalAvailable {
			continue
		}
		group := groups[stored.TraceID]
		candidate := entryFromLookup(lookup)
		if group == nil {
			group = &installedGroup{entry: candidate}
			groups[stored.TraceID] = group
		} else if canonicalEntryLess(candidate, group.entry) {
			group.entry = candidate
		}
		if stored.Source == evidence.SourceTarget {
			group.target = true
		} else {
			group.imported = true
		}
		group.identities = append(group.identities, string(stored.Source)+":"+string(lookup.Handle))
	}

	for _, group := range groups {
		group.entry.Ambiguous = group.target && group.imported
		if hasTarget && group.imported && !group.target {
			if service.catalog == nil {
				markIncomplete(&result)
				continue
			}
			_, probeDomain := service.catalog.GetTrace(ctx, scope, group.entry.TraceID)
			if probeDomain == nil {
				group.entry.Ambiguous = true
			} else if probeDomain.Code != consolecore.CodeNotFound {
				markIncomplete(&result)
			}
		}
	}

	installed := make([]Entry, 0, len(groups))
	for _, group := range groups {
		installed = append(installed, group.entry)
	}
	sort.Slice(installed, func(i, j int) bool { return entryLess(installed[i], installed[j]) })
	installedFingerprint := installedSetFingerprint(groups)
	if query.Continuation != "" && continuedInstalledFingerprint != installedFingerprint {
		return Result{}, invalidCursor(fmt.Errorf("installed trace set changed during inventory traversal"))
	}

	if segment == segmentInstalled {
		for _, entry := range installed {
			key := keyFor(entry)
			if after != nil && !keyAfter(key, *after) {
				continue
			}
			if len(result.Items) == pageSize {
				result.HasMore = true
				result.Continuation, _ = encodeCursor(inventoryCursor{Schema: cursorSchemaV1, Operation: cursorOperation, Fingerprint: fingerprint, Segment: segmentInstalled, Installed: ptrKey(keyFor(result.Items[len(result.Items)-1])), InstalledFingerprint: installedFingerprint})
				if hasTarget {
					if service.catalog == nil {
						markIncomplete(&result)
					} else if page, catalogDomain := service.catalog.ListTraces(ctx, scope, observability.ListRequest{PageSize: 1}); catalogDomain != nil {
						markIncomplete(&result)
					} else {
						result.ObservedAt = page.ObservedAt.UTC()
					}
				}
				return service.finish(scope, hasTarget, result)
			}
			result.Items = append(result.Items, entry)
		}
		segment = segmentApplication
	}

	if !hasTarget {
		return result, nil
	}
	if service.catalog == nil {
		markIncomplete(&result)
		return service.finish(scope, true, result)
	}

	remaining := pageSize - len(result.Items)
	if remaining == 0 {
		more, observedAt, catalogDomain := service.catalogHasUnlistedTrace(ctx, scope, groups)
		if catalogDomain != nil {
			markIncomplete(&result)
			return service.finish(scope, true, result)
		}
		if !observedAt.IsZero() {
			result.ObservedAt = observedAt.UTC()
		}
		if more {
			result.HasMore = true
			result.Continuation, _ = encodeCursor(inventoryCursor{Schema: cursorSchemaV1, Operation: cursorOperation, Fingerprint: fingerprint, Segment: segmentApplication, InstalledFingerprint: installedFingerprint})
		}
		return service.finish(scope, true, result)
	}
	for remaining > 0 {
		page, catalogDomain := service.catalog.ListTraces(ctx, scope, observability.ListRequest{Cursor: applicationCursor, PageSize: remaining})
		if catalogDomain != nil {
			markIncomplete(&result)
			return service.finish(scope, true, result)
		}
		result.ObservedAt = page.ObservedAt.UTC()
		for _, trace := range page.Items {
			if groups[trace.TraceID] != nil {
				continue
			}
			result.Items = append(result.Items, entryFromCatalog(trace))
			remaining--
			if remaining == 0 {
				break
			}
		}
		if !page.HasMore || page.NextCursor == nil {
			break
		}
		applicationCursor = *page.NextCursor
		if remaining == 0 {
			result.HasMore = true
			result.Continuation, _ = encodeCursor(inventoryCursor{Schema: cursorSchemaV1, Operation: cursorOperation, Fingerprint: fingerprint, Segment: segmentApplication, InstalledFingerprint: installedFingerprint, ApplicationCursor: applicationCursor})
			break
		}
	}
	return service.finish(scope, true, result)
}

func (service *Service) catalogHasUnlistedTrace(ctx context.Context, scope target.Scope, groups map[string]*installedGroup) (bool, time.Time, *consolecore.Error) {
	cursor := ""
	seen := map[string]bool{}
	var observedAt time.Time
	for {
		if seen[cursor] {
			return false, observedAt, consolecore.NewError(consolecore.CodeConsoleError, "Trace discovery could not establish catalog pagination.", "", consolecore.Details{}, nil)
		}
		seen[cursor] = true
		page, domain := service.catalog.ListTraces(ctx, scope, observability.ListRequest{Cursor: cursor, PageSize: maxPageSize})
		if domain != nil {
			return false, observedAt, domain
		}
		observedAt = page.ObservedAt
		for _, trace := range page.Items {
			if groups[trace.TraceID] == nil {
				return true, observedAt, nil
			}
		}
		if !page.HasMore {
			return false, observedAt, nil
		}
		if page.NextCursor == nil || *page.NextCursor == "" {
			return false, observedAt, consolecore.NewError(consolecore.CodeConsoleError, "Trace discovery could not establish catalog pagination.", "", consolecore.Details{}, nil)
		}
		cursor = *page.NextCursor
	}
}

func (service *Service) finish(scope target.Scope, hasTarget bool, result Result) (Result, *consolecore.Error) {
	if hasTarget {
		if domain := service.target.RequireCurrent(scope.ID); domain != nil {
			return Result{}, domain
		}
	}
	return result, nil
}

func markIncomplete(result *Result) {
	result.Complete = false
	if len(result.Limitations) == 0 {
		result.Limitations = append(result.Limitations, Limitation{Code: LimitationTraceDiscoveryIncomplete, Message: incompleteMessage})
	}
}

func entryFromLookup(value artifact.LookupResult) Entry {
	return Entry{TraceID: value.Metadata.TraceID, SessionID: value.Metadata.SessionID, EntrySkill: value.Metadata.EntrySkill, Outcome: value.Metadata.Outcome, FinalizedAt: value.Metadata.FinalizedAt}
}
func entryFromCatalog(value observability.Trace) Entry {
	return Entry{TraceID: value.TraceID, SessionID: value.SessionID, EntrySkill: value.EntrySkill, Outcome: value.Outcome, FinalizedAt: value.FinalizedAt}
}

func keyFor(entry Entry) installedKey {
	return installedKey{FinalizedAt: entry.FinalizedAt.UTC(), TraceID: entry.TraceID}
}
func ptrKey(key installedKey) *installedKey { return &key }
func entryLess(a, b Entry) bool {
	if !a.FinalizedAt.Equal(b.FinalizedAt) {
		return a.FinalizedAt.After(b.FinalizedAt)
	}
	return a.TraceID < b.TraceID
}

func canonicalEntryLess(a, b Entry) bool {
	if !a.FinalizedAt.Equal(b.FinalizedAt) {
		return a.FinalizedAt.After(b.FinalizedAt)
	}
	if a.SessionID != b.SessionID {
		return a.SessionID < b.SessionID
	}
	if a.EntrySkill != b.EntrySkill {
		return a.EntrySkill < b.EntrySkill
	}
	return a.Outcome < b.Outcome
}
func keyAfter(a, b installedKey) bool {
	if !a.FinalizedAt.Equal(b.FinalizedAt) {
		return a.FinalizedAt.Before(b.FinalizedAt)
	}
	return a.TraceID > b.TraceID
}

func installedSetFingerprint(groups map[string]*installedGroup) string {
	type identity struct {
		TraceID     string    `json:"traceId"`
		Identities  []string  `json:"identities"`
		SessionID   string    `json:"sessionId"`
		EntrySkill  string    `json:"entrySkill"`
		Outcome     string    `json:"outcome"`
		FinalizedAt time.Time `json:"finalizedAt"`
	}
	values := make([]identity, 0, len(groups))
	for traceID, group := range groups {
		sort.Strings(group.identities)
		values = append(values, identity{
			TraceID: traceID, Identities: group.identities,
			SessionID: group.entry.SessionID, EntrySkill: group.entry.EntrySkill,
			Outcome: group.entry.Outcome, FinalizedAt: group.entry.FinalizedAt.UTC(),
		})
	}
	sort.Slice(values, func(i, j int) bool { return values[i].TraceID < values[j].TraceID })
	body, _ := json.Marshal(values)
	sum := sha256.Sum256(body)
	return fmt.Sprintf("%x", sum[:])
}

func invalid(message string) *consolecore.Error {
	return consolecore.NewError(consolecore.CodeInvalidArgument, message, "", consolecore.Details{}, nil)
}
func invalidCursor(cause error) *consolecore.Error {
	if cause == nil {
		cause = fmt.Errorf("continuation does not match this inventory")
	}
	return consolecore.NewError(consolecore.CodeInvalidCursor, "The continuation does not match this trace inventory.", "", consolecore.Details{}, cause)
}
