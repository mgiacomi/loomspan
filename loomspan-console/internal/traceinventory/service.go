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

func New(a ArtifactService, c CatalogService, t TargetProvider, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{a, c, t, now}
}

func (s *Service) EnrichTargetCatalogPage(scopeID target.ScopeID, page observability.Page[observability.Trace]) observability.Page[observability.Trace] {
	if s == nil || s.artifacts == nil {
		return page
	}
	for i := range page.Items {
		lookup, d := s.artifacts.Lookup(evidence.ForTarget(scopeID), page.Items[i].TraceID)
		if d == nil && lookup.LocalAvailable {
			page.Items[i].LocalAvailable = true
			page.Items[i].ArtifactHandle = string(lookup.Handle)
			page.Items[i].ApplicationAvailability = string(lookup.ApplicationAvailability)
		}
	}
	return page
}

type evidenceInstance struct {
	source                                  EvidenceSource
	traceID, sessionID, entrySkill, outcome string
	finalizedAt, sourceTime                 time.Time
	identity                                string
}
type evidenceGroup struct {
	traceID   string
	instances []evidenceInstance
}

func (s *Service) List(ctx context.Context, q Query) (Result, *consolecore.Error) {
	pageSize, d := validateQuery(&q)
	if d != nil {
		return Result{}, d
	}
	if s == nil || s.artifacts == nil {
		return Result{}, consolecore.NewError(consolecore.CodeConsoleError, "Trace inventory is unavailable.", "", consolecore.Details{}, nil)
	}
	result := Result{ObservedAt: s.now().UTC(), Items: []Entry{}, Complete: true, Limitations: []Limitation{}}
	var scope target.Scope
	hasTarget := false
	if s.target != nil {
		captured, td := s.target.Capture()
		if td == nil {
			scope, hasTarget = captured, true
		} else if td.Code != consolecore.CodeInvalidArgument {
			return Result{}, td
		}
	}
	fingerprint := queryFingerprint(q, pageSize, string(scope.ID))
	segment, offset, appCursor, appOffset, continuedSet := segmentInstalled, 0, "", 0, ""
	if q.Continuation != "" {
		cur, err := decodeCursor(q.Continuation)
		if err != nil || cur.Fingerprint != fingerprint {
			return Result{}, invalidCursor(err)
		}
		segment, offset, appCursor, appOffset, continuedSet = cur.Segment, cur.InstalledOffset, cur.ApplicationCursor, cur.ApplicationOffset, cur.InstalledFingerprint
	}

	snapshot, sd := s.artifacts.StorageSnapshot()
	if sd != nil {
		return Result{}, sd
	}
	groups := map[string]*evidenceGroup{}
	for _, stored := range snapshot.Entries {
		if stored.Source == evidence.SourceTarget && (!hasTarget || stored.TargetScopeID != string(scope.ID)) {
			continue
		}
		ref := evidence.ForImported()
		source := SourceImported
		if stored.Source == evidence.SourceTarget {
			ref = evidence.ForTarget(scope.ID)
			source = SourceTarget
		}
		lookup, ld := s.artifacts.Lookup(ref, stored.TraceID)
		if ld != nil {
			return Result{}, ld
		}
		if !lookup.LocalAvailable {
			continue
		}
		inst := instanceFromLookup(source, lookup)
		g := groups[inst.traceID]
		if g == nil {
			g = &evidenceGroup{traceID: inst.traceID}
			groups[inst.traceID] = g
		}
		g.instances = append(g.instances, inst)
	}
	// A target/import collision remains ambiguous even when the target copy is
	// catalog-only. Probe only imported-only identities; failures make discovery incomplete.
	if hasTarget && s.catalog != nil {
		for _, g := range groups {
			if hasSource(g, SourceImported) && !hasSource(g, SourceTarget) {
				tr, pd := s.catalog.GetTrace(ctx, scope, g.traceID)
				if pd == nil {
					g.instances = append(g.instances, instanceFromCatalog(tr))
				} else if pd.Code != consolecore.CodeNotFound {
					markIncomplete(&result)
				}
			}
		}
	}
	installed := make([]Entry, 0, len(groups))
	orderKeys := make(map[string]entrySortKey, len(groups))
	for _, g := range groups {
		matching := matchingInstances(g.instances, q)
		if len(matching) == 0 {
			continue
		}
		e, limits := projectGroup(g)
		installed = append(installed, e)
		orderKeys[e.TraceID] = sortKeyForMatching(matching, q.Order)
		addLimitations(&result, limits...)
	}
	sortEntries(installed, q.Order, orderKeys)
	setFingerprint := installedSetFingerprint(groups)
	if q.Continuation != "" && continuedSet != setFingerprint {
		return Result{}, invalidCursor(fmt.Errorf("installed trace set changed during inventory traversal"))
	}

	if offset > len(installed) {
		return Result{}, invalidCursor(fmt.Errorf("installed position changed"))
	}
	if !hasTarget {
		end := min(len(installed), offset+pageSize)
		result.Items = append(result.Items, installed[offset:end]...)
		if end < len(installed) {
			result.Complete, result.HasMore = false, true
			result.Continuation, _ = encodeCursor(inventoryCursor{Schema: cursorSchemaV1, Operation: cursorOperation, Fingerprint: fingerprint, Segment: segmentInstalled, InstalledOffset: end, InstalledFingerprint: setFingerprint})
		}
		return result, nil
	}
	if s.catalog == nil {
		markIncomplete(&result)
		return s.finish(scope, true, result)
	}
	_ = segment // segment remains in the opaque shape for strict decoding.
	page, pd := s.catalog.ListTraces(ctx, scope, observability.ListRequest{Cursor: appCursor, PageSize: maxPageSize})
	if pd != nil {
		markIncomplete(&result)
		end := min(len(installed), offset+pageSize)
		result.Items = append(result.Items, installed[offset:end]...)
		return s.finish(scope, true, result)
	}
	result.ObservedAt = page.ObservedAt.UTC()
	application := make([]Entry, 0, len(page.Items))
	for _, tr := range page.Items {
		if groups[tr.TraceID] != nil {
			continue
		}
		inst := instanceFromCatalog(tr)
		if !matches(inst, q) {
			continue
		}
		e, _ := projectGroup(&evidenceGroup{traceID: tr.TraceID, instances: []evidenceInstance{inst}})
		application = append(application, e)
	}
	sortEntries(application, q.Order, nil)
	if appOffset > len(application) {
		return Result{}, invalidCursor(fmt.Errorf("application position changed"))
	}
	eligibleInstalledEnd := len(installed)
	if page.HasMore && len(page.Items) > 0 {
		eligibleInstalledEnd = offset
		boundary := page.Items[len(page.Items)-1].FinalizedAt
		for eligibleInstalledEnd < len(installed) && installedSafeBeforeCatalogRemainder(orderKeys[installed[eligibleInstalledEnd].TraceID], q.Order, boundary) {
			eligibleInstalledEnd++
		}
	}
	type candidate struct {
		entry     Entry
		installed bool
	}
	candidates := make([]candidate, 0, eligibleInstalledEnd-offset+len(application)-appOffset)
	for _, entry := range installed[offset:eligibleInstalledEnd] {
		candidates = append(candidates, candidate{entry: entry, installed: true})
	}
	for _, entry := range application[appOffset:] {
		candidates = append(candidates, candidate{entry: entry})
	}
	sort.SliceStable(candidates, func(i, j int) bool { return entryBefore(candidates[i].entry, candidates[j].entry, q.Order, orderKeys) })
	consumedInstalled, consumedApplication := 0, 0
	for _, value := range candidates {
		if len(result.Items) == pageSize {
			break
		}
		result.Items = append(result.Items, value.entry)
		if value.installed {
			consumedInstalled++
		} else {
			consumedApplication++
		}
	}
	offset += consumedInstalled
	appOffset += consumedApplication
	applicationRemaining := appOffset < len(application)
	moreCatalog := page.HasMore
	if !applicationRemaining && page.HasMore {
		if page.NextCursor == nil || *page.NextCursor == "" {
			markIncomplete(&result)
			return s.finish(scope, true, result)
		}
		appCursor, appOffset = *page.NextCursor, 0
	} else if !applicationRemaining {
		moreCatalog = false
	}
	if offset < len(installed) || applicationRemaining || moreCatalog {
		result.Complete, result.HasMore = false, true
		result.Continuation, _ = encodeCursor(inventoryCursor{Schema: cursorSchemaV1, Operation: cursorOperation, Fingerprint: fingerprint, Segment: segmentApplication, InstalledOffset: offset, InstalledFingerprint: setFingerprint, ApplicationCursor: appCursor, ApplicationOffset: appOffset})
	}
	return s.finish(scope, true, result)
}

func installedSafeBeforeCatalogRemainder(key entrySortKey, order Order, boundary time.Time) bool {
	if (order == OrderAcquiredDesc || order == OrderImportedDesc) && key.primary != nil {
		return true
	}
	return key.finalized != nil && !key.finalized.Before(boundary)
}

func validateQuery(q *Query) (int, *consolecore.Error) {
	page := q.PageSize
	if page == 0 {
		page = defaultPageSize
	}
	if page < 1 || page > maxPageSize {
		return 0, invalid("The trace inventory page size must be from 1 through 64.")
	}
	if q.Order == "" {
		q.Order = OrderFinalizedDesc
	}
	if q.Order != OrderFinalizedDesc && q.Order != OrderAcquiredDesc && q.Order != OrderImportedDesc {
		return 0, invalid("The trace inventory order is unsupported.")
	}
	seen := map[EvidenceSource]bool{}
	for _, v := range q.Sources {
		if (v != SourceTarget && v != SourceImported) || seen[v] {
			return 0, invalid("The trace evidence source filter is invalid.")
		}
		seen[v] = true
	}
	seenOutcome := map[string]bool{}
	for _, v := range q.Outcomes {
		if v != "SUCCEEDED" && v != "FAILED" && v != "ABORTED" {
			return 0, invalid("The trace outcome filter is invalid.")
		}
		if seenOutcome[v] {
			return 0, invalid("The trace outcome filter contains duplicates.")
		}
		seenOutcome[v] = true
	}
	if inverted(q.FinalizedFrom, q.FinalizedTo) || inverted(q.AcquiredFrom, q.AcquiredTo) || inverted(q.ImportedFrom, q.ImportedTo) {
		return 0, invalid("A trace inventory time range is inverted.")
	}
	return page, nil
}
func inverted(a, b *time.Time) bool { return a != nil && b != nil && a.After(*b) }
func instanceFromLookup(source EvidenceSource, v artifact.LookupResult) evidenceInstance {
	return evidenceInstance{source, v.Metadata.TraceID, v.Metadata.SessionID, v.Metadata.EntrySkill, v.Metadata.Outcome, v.Metadata.FinalizedAt.UTC(), v.AcquiredAt.UTC(), string(source) + ":" + string(v.Handle)}
}
func instanceFromCatalog(v observability.Trace) evidenceInstance {
	return evidenceInstance{source: SourceTarget, traceID: v.TraceID, sessionID: v.SessionID, entrySkill: v.EntrySkill, outcome: v.Outcome, finalizedAt: v.FinalizedAt.UTC(), identity: "TARGET:CATALOG:" + v.TraceID}
}
func hasSource(g *evidenceGroup, s EvidenceSource) bool {
	for _, i := range g.instances {
		if i.source == s {
			return true
		}
	}
	return false
}
func matchingInstances(values []evidenceInstance, q Query) []evidenceInstance {
	out := []evidenceInstance{}
	for _, v := range values {
		if matches(v, q) {
			out = append(out, v)
		}
	}
	return out
}
func matches(v evidenceInstance, q Query) bool {
	if len(q.Sources) > 0 && !containsSource(q.Sources, v.source) {
		return false
	}
	if len(q.Outcomes) > 0 && !contains(q.Outcomes, v.outcome) {
		return false
	}
	if q.EntrySkill != "" && v.entrySkill != q.EntrySkill {
		return false
	}
	if q.SessionID != "" && v.sessionID != q.SessionID {
		return false
	}
	if !within(v.finalizedAt, q.FinalizedFrom, q.FinalizedTo) {
		return false
	}
	if q.AcquiredFrom != nil || q.AcquiredTo != nil {
		if v.source != SourceTarget || v.sourceTime.IsZero() || !within(v.sourceTime, q.AcquiredFrom, q.AcquiredTo) {
			return false
		}
	}
	if q.ImportedFrom != nil || q.ImportedTo != nil {
		if v.source != SourceImported || v.sourceTime.IsZero() || !within(v.sourceTime, q.ImportedFrom, q.ImportedTo) {
			return false
		}
	}
	return true
}
func within(v time.Time, a, b *time.Time) bool {
	if v.IsZero() {
		return a == nil && b == nil
	}
	return (a == nil || !v.Before(*a)) && (b == nil || !v.After(*b))
}
func contains(v []string, x string) bool {
	for _, s := range v {
		if s == x {
			return true
		}
	}
	return false
}
func containsSource(v []EvidenceSource, x EvidenceSource) bool {
	for _, s := range v {
		if s == x {
			return true
		}
	}
	return false
}

func projectGroup(g *evidenceGroup) (Entry, []Limitation) {
	e := Entry{TraceID: g.traceID, EvidenceSources: []EvidenceSource{}}
	limits := []Limitation{}
	if hasSource(g, SourceTarget) {
		e.EvidenceSources = append(e.EvidenceSources, SourceTarget)
	}
	if hasSource(g, SourceImported) {
		e.EvidenceSources = append(e.EvidenceSources, SourceImported)
	}
	e.Ambiguous = len(e.EvidenceSources) > 1
	for _, i := range g.instances {
		t := i.sourceTime
		if !t.IsZero() {
			if i.source == SourceTarget {
				e.AcquiredAt = later(e.AcquiredAt, t)
			} else {
				e.ImportedAt = later(e.ImportedAt, t)
			}
		}
	}
	if len(g.instances) > 0 {
		first := g.instances[0]
		if allString(g.instances, func(i evidenceInstance) string { return i.sessionID }) {
			e.SessionID = ptr(first.sessionID)
		}
		if allString(g.instances, func(i evidenceInstance) string { return i.entrySkill }) && first.entrySkill != "" {
			e.EntrySkill = ptr(first.entrySkill)
		}
		if allString(g.instances, func(i evidenceInstance) string { return i.outcome }) {
			e.Outcome = ptr(first.outcome)
		}
		if allTime(g.instances, func(i evidenceInstance) time.Time { return i.finalizedAt }) {
			t := first.finalizedAt
			e.FinalizedAt = &t
		}
	}
	if e.Ambiguous && (e.SessionID == nil || e.Outcome == nil || e.FinalizedAt == nil) {
		limits = append(limits, Limitation{LimitationAmbiguousMetadataUnavailable, "Conflicting source evidence makes shared trace metadata unavailable."})
	}
	if hasSource(g, SourceImported) && e.EntrySkill == nil {
		limits = append(limits, Limitation{LimitationImportedEntrySkillUnavailable, "Imported trace evidence does not contain a validated entry skill."})
	}
	return e, limits
}
func ptr(v string) *string { return &v }
func later(p *time.Time, v time.Time) *time.Time {
	if p == nil || v.After(*p) {
		x := v
		return &x
	}
	return p
}
func allString(v []evidenceInstance, f func(evidenceInstance) string) bool {
	if len(v) == 0 {
		return false
	}
	x := f(v[0])
	for _, i := range v[1:] {
		if f(i) != x {
			return false
		}
	}
	return true
}
func allTime(v []evidenceInstance, f func(evidenceInstance) time.Time) bool {
	if len(v) == 0 {
		return false
	}
	x := f(v[0])
	for _, i := range v[1:] {
		if !f(i).Equal(x) {
			return false
		}
	}
	return true
}

type entrySortKey struct {
	primary   *time.Time
	finalized *time.Time
}

func sortKeyForMatching(values []evidenceInstance, order Order) entrySortKey {
	key := entrySortKey{}
	for _, value := range values {
		key.finalized = later(key.finalized, value.finalizedAt)
		var candidate time.Time
		switch order {
		case OrderAcquiredDesc:
			if value.source == SourceTarget {
				candidate = value.sourceTime
			}
		case OrderImportedDesc:
			if value.source == SourceImported {
				candidate = value.sourceTime
			}
		default:
			candidate = value.finalizedAt
		}
		if !candidate.IsZero() {
			key.primary = later(key.primary, candidate)
		}
	}
	return key
}

func sortEntries(v []Entry, o Order, keys map[string]entrySortKey) {
	sort.SliceStable(v, func(i, j int) bool {
		return entryBefore(v[i], v[j], o, keys)
	})
}
func entryBefore(left, right Entry, o Order, keys map[string]entrySortKey) bool {
	a, b := projectedSortKey(left, o), projectedSortKey(right, o)
	if keys != nil {
		if key, ok := keys[left.TraceID]; ok {
			a = key
		}
		if key, ok := keys[right.TraceID]; ok {
			b = key
		}
	}
	if a.primary == nil && b.primary != nil {
		return false
	}
	if a.primary != nil && b.primary == nil {
		return true
	}
	if a.primary != nil && b.primary != nil && !a.primary.Equal(*b.primary) {
		return a.primary.After(*b.primary)
	}
	af, bf := a.finalized, b.finalized
	if af != nil && bf != nil && !af.Equal(*bf) {
		return af.After(*bf)
	}
	if af != nil && bf == nil {
		return true
	}
	if af == nil && bf != nil {
		return false
	}
	return left.TraceID < right.TraceID
}
func projectedSortKey(e Entry, o Order) entrySortKey {
	key := entrySortKey{finalized: e.FinalizedAt}
	if o == OrderAcquiredDesc {
		key.primary = e.AcquiredAt
		return key
	}
	if o == OrderImportedDesc {
		key.primary = e.ImportedAt
		return key
	}
	key.primary = e.FinalizedAt
	return key
}

func installedSetFingerprint(groups map[string]*evidenceGroup) string {
	type x struct {
		TraceID   string             `json:"traceId"`
		Instances []evidenceInstance `json:"instances"`
	}
	v := make([]x, 0, len(groups))
	for id, g := range groups {
		sort.Slice(g.instances, func(i, j int) bool { return g.instances[i].identity < g.instances[j].identity })
		v = append(v, x{id, g.instances})
	}
	sort.Slice(v, func(i, j int) bool { return v[i].TraceID < v[j].TraceID })
	b, _ := json.Marshal(v)
	sum := sha256.Sum256(b)
	return fmt.Sprintf("%x", sum[:])
}
func addLimitations(r *Result, values ...Limitation) {
	for _, v := range values {
		found := false
		for _, x := range r.Limitations {
			if x.Code == v.Code {
				found = true
				break
			}
		}
		if !found {
			r.Limitations = append(r.Limitations, v)
		}
	}
}
func (s *Service) finish(scope target.Scope, has bool, r Result) (Result, *consolecore.Error) {
	if has {
		if d := s.target.RequireCurrent(scope.ID); d != nil {
			return Result{}, d
		}
	}
	return r, nil
}
func markIncomplete(r *Result) {
	r.Complete = false
	addLimitations(r, Limitation{LimitationTraceDiscoveryIncomplete, incompleteMessage})
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
