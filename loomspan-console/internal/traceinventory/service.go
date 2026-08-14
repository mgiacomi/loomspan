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

type ArtifactService interface {
	StorageSnapshot() (artifact.StorageSnapshot, *consolecore.Error)
	Lookup(evidence.Reference, string) (artifact.LookupResult, *consolecore.Error)
}

type CatalogService interface {
	ListTraces(context.Context, target.Scope, observability.ListRequest) (observability.Page[observability.Trace], *consolecore.Error)
}

type TargetProvider interface {
	Capture() (target.Scope, *consolecore.Error)
}

// Service joins the two existing trace owners without retaining an inventory.
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

// EnrichTargetCatalogPage applies installed-copy facts to an existing browser
// catalog page without changing its ordering, cursor, or JSON contract.
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

func (service *Service) List(ctx context.Context, query Query) (Result, *consolecore.Error) {
	filter := query.SourceFilter
	if filter == "" {
		filter = SourceFilterAll
	}
	if filter != SourceFilterAll && filter != SourceFilterTarget && filter != SourceFilterImported {
		return Result{}, invalid("The trace source filter is not supported.")
	}
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

	result := Result{ObservedAt: service.now().UTC(), Items: []Entry{}}
	var scope target.Scope
	var captureError *consolecore.Error
	if filter != SourceFilterImported {
		result.ApplicationCatalog.Requested = true
		if service.target == nil {
			captureError = invalid("Select a target first.")
		} else {
			scope, captureError = service.target.Capture()
		}
		if captureError != nil {
			if filter == SourceFilterTarget {
				return Result{}, captureError
			}
			result.ApplicationCatalog.Error = captureError
		} else {
			result.ApplicationCatalog.TargetScopeID = string(scope.ID)
			result.ApplicationCatalog.InstanceID = scope.InstanceID
		}
	}

	fingerprint := queryFingerprint(filter, pageSize, string(scope.ID))
	segment := segmentInstalled
	var after *installedKey
	applicationCursor := ""
	continuedInstalledFingerprint := ""
	if query.Continuation != "" {
		cur, err := decodeCursor(query.Continuation)
		if err != nil || cur.Fingerprint != fingerprint {
			return Result{}, invalidCursor(err)
		}
		applicationCursor = cur.ApplicationCursor
		continuedInstalledFingerprint = cur.InstalledFingerprint
		segment, after = cur.Segment, cur.Installed
	}

	snapshot, domain := service.artifacts.StorageSnapshot()
	if domain != nil {
		return Result{}, domain
	}
	installed := make([]Entry, 0, len(snapshot.Entries))
	installedTraceIDs := make(map[string]bool)
	for _, stored := range snapshot.Entries {
		if !matchesSource(filter, stored.Source) || (stored.Source == evidence.SourceTarget && (scope.ID == "" || stored.TargetScopeID != string(scope.ID))) {
			continue
		}
		ref := evidence.ForImported()
		if stored.Source == evidence.SourceTarget {
			ref = evidence.ForTarget(target.ScopeID(stored.TargetScopeID))
		}
		lookup, lookupDomain := service.artifacts.Lookup(ref, stored.TraceID)
		if lookupDomain != nil {
			return Result{}, lookupDomain
		}
		if !lookup.LocalAvailable {
			continue
		}
		entry := entryFromLookup(lookup)
		installed = append(installed, entry)
		if entry.Source == evidence.SourceTarget {
			installedTraceIDs[entry.TraceID] = true
		}
	}
	sort.Slice(installed, func(i, j int) bool { return entryLess(installed[i], installed[j]) })
	installedFingerprint := targetInstalledFingerprint(installed)
	if segment == segmentApplication && continuedInstalledFingerprint != installedFingerprint {
		return Result{}, invalidCursor(fmt.Errorf("installed trace set changed during application traversal"))
	}

	if segment == segmentInstalled {
		for _, entry := range installed {
			key := keyFor(entry)
			if after != nil && !keyAfter(key, *after) {
				continue
			}
			if len(result.Items) == pageSize {
				service.probeCatalog(ctx, scope, filter, &result)
				result.HasMore = true
				result.Continuation, _ = encodeCursor(inventoryCursor{Schema: cursorSchemaV1, Operation: cursorOperation, Fingerprint: fingerprint, Segment: segmentInstalled, Installed: ptrKey(keyFor(result.Items[len(result.Items)-1]))})
				return result, nil
			}
			result.Items = append(result.Items, entry)
		}
		segment = segmentApplication
		if len(result.Items) == pageSize && captureError == nil && filter != SourceFilterImported {
			catalogHasEntries := service.probeCatalog(ctx, scope, filter, &result)
			if catalogHasEntries {
				result.HasMore = true
				result.Continuation, _ = encodeCursor(inventoryCursor{Schema: cursorSchemaV1, Operation: cursorOperation, Fingerprint: fingerprint, Segment: segmentApplication, InstalledFingerprint: installedFingerprint})
			}
			return result, nil
		}
	}

	if filter == SourceFilterImported || captureError != nil {
		return result, nil
	}
	if service.catalog == nil {
		result.ApplicationCatalog.Error = consolecore.NewError(consolecore.CodeConsoleError, "Trace catalog is unavailable.", string(scope.ID), consolecore.Details{}, nil)
		return result, nil
	}
	remaining := pageSize - len(result.Items)
	for remaining > 0 {
		page, catalogDomain := service.catalog.ListTraces(ctx, scope, observability.ListRequest{Cursor: applicationCursor, PageSize: remaining})
		if catalogDomain != nil {
			result.ApplicationCatalog.Error = catalogDomain
			return result, nil
		}
		result.ApplicationCatalog.Available = true
		result.ObservedAt = page.ObservedAt.UTC()
		for _, trace := range page.Items {
			if installedTraceIDs[trace.TraceID] {
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
			cur := inventoryCursor{Schema: cursorSchemaV1, Operation: cursorOperation, Fingerprint: fingerprint, Segment: segmentApplication, InstalledFingerprint: installedFingerprint, ApplicationCursor: applicationCursor}
			continuation, encodeErr := encodeCursor(cur)
			if encodeErr != nil {
				return Result{}, consolecore.NewError(consolecore.CodeConsoleError, "Trace inventory continuation could not be encoded.", string(scope.ID), consolecore.Details{}, encodeErr)
			}
			result.Continuation = continuation
			break
		}
	}
	return result, nil
}

// probeCatalog establishes application availability before an installed-only
// page returns. The probe is deliberately not consumed: once installed
// traversal finishes, the application segment starts from its authoritative
// first cursor and performs ordinary duplicate suppression.
func (service *Service) probeCatalog(ctx context.Context, scope target.Scope, filter SourceFilter, result *Result) bool {
	if filter == SourceFilterImported || scope.ID == "" || result.ApplicationCatalog.Available || result.ApplicationCatalog.Error != nil {
		return false
	}
	if service.catalog == nil {
		result.ApplicationCatalog.Error = consolecore.NewError(consolecore.CodeConsoleError, "Trace catalog is unavailable.", string(scope.ID), consolecore.Details{}, nil)
		return false
	}
	page, domain := service.catalog.ListTraces(ctx, scope, observability.ListRequest{PageSize: 1})
	if domain != nil {
		result.ApplicationCatalog.Error = domain
		return false
	}
	result.ApplicationCatalog.Available = true
	result.ObservedAt = page.ObservedAt.UTC()
	return len(page.Items) > 0 || page.HasMore
}

func matchesSource(filter SourceFilter, source evidence.Source) bool {
	return filter == SourceFilterAll || filter == SourceFilterTarget && source == evidence.SourceTarget || filter == SourceFilterImported && source == evidence.SourceImported
}

func entryFromLookup(value artifact.LookupResult) Entry {
	var appExpiry *time.Time
	if !value.Metadata.ApplicationTraceExpiresAt.IsZero() && value.Owner.Source() == evidence.SourceTarget {
		expires := value.Metadata.ApplicationTraceExpiresAt
		appExpiry = &expires
	}
	return Entry{Source: value.Owner.Source(), TargetScopeID: string(value.Owner.TargetScope()), TraceID: value.Metadata.TraceID,
		SessionID: value.Metadata.SessionID, EntrySkill: value.Metadata.EntrySkill, Outcome: value.Metadata.Outcome,
		FinalizedAt: value.Metadata.FinalizedAt, SizeBytes: value.Metadata.SizeBytes, PersistencePolicy: value.Metadata.PersistencePolicy,
		ApplicationTraceExpiresAt: appExpiry, ApplicationAvailability: string(value.ApplicationAvailability), LocalAvailable: true,
		ArtifactHandle: value.Handle, AcquiredAt: value.AcquiredAt, LastUsedAt: value.LastUsedAt, LocalExpiresAt: value.ExpiresAt,
		HasIdleExpiry: value.HasIdleExpiry, LocalBytes: value.LocalBytes}
}

func entryFromCatalog(value observability.Trace) Entry {
	expires := value.ApplicationTraceExpiresAt
	return Entry{Source: evidence.SourceTarget, TargetScopeID: value.TargetScopeID, TraceID: value.TraceID, SessionID: value.SessionID,
		EntrySkill: value.EntrySkill, Outcome: value.Outcome, FinalizedAt: value.FinalizedAt, SizeBytes: value.SizeBytes,
		PersistencePolicy: value.PersistencePolicy, ApplicationTraceExpiresAt: &expires, ApplicationAvailability: string(artifact.ApplicationAvailable)}
}

func keyFor(entry Entry) installedKey {
	return installedKey{FinalizedAt: entry.FinalizedAt.UTC(), Source: string(entry.Source), TraceID: entry.TraceID}
}

func targetInstalledFingerprint(installed []Entry) string {
	values := make([]struct {
		TraceID string `json:"traceId"`
		Handle  string `json:"artifactHandle"`
	}, 0, len(installed))
	for _, entry := range installed {
		if entry.Source == evidence.SourceTarget {
			values = append(values, struct {
				TraceID string `json:"traceId"`
				Handle  string `json:"artifactHandle"`
			}{TraceID: entry.TraceID, Handle: string(entry.ArtifactHandle)})
		}
	}
	body, _ := json.Marshal(values)
	sum := sha256.Sum256(body)
	return fmt.Sprintf("%x", sum[:])
}
func ptrKey(key installedKey) *installedKey { return &key }
func entryLess(a, b Entry) bool {
	if !a.FinalizedAt.Equal(b.FinalizedAt) {
		return a.FinalizedAt.After(b.FinalizedAt)
	}
	if a.Source != b.Source {
		return a.Source == evidence.SourceTarget
	}
	return a.TraceID < b.TraceID
}
func keyAfter(a, b installedKey) bool {
	if !a.FinalizedAt.Equal(b.FinalizedAt) {
		return a.FinalizedAt.Before(b.FinalizedAt)
	}
	if a.Source != b.Source {
		return b.Source == string(evidence.SourceTarget)
	}
	return a.TraceID > b.TraceID
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
