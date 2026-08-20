package traceanalysis

import (
	"sort"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
)

// frameBuild is the working state for one frame during iterative processing.
type frameBuild struct {
	frameID             string
	parentFrameID       string
	hasParent           bool
	frameType           TraceFrameType
	route               string
	openedMillis        int64
	closedMillis        int64
	opened              bool
	closed              bool
	children            []string
	directUsage         Usage
	directUsageComplete bool
	skillNames          map[string]struct{}
	outcome             *string
	attemptIDs          map[string]struct{}
	retrySequenceIDs    map[string]struct{}
	validationStatuses  map[string]struct{}
	failureIDs          map[string]struct{}
}

// frameGraph holds the working frame state and produces the final frame results,
// gaps, and uncertainties. It uses iterative parent traversal with explicit
// visitation states to reject cycles and support arbitrarily deep valid frame
// trees without stack growth.
type frameGraph struct {
	frames map[string]*frameBuild
	order  []string // insertion (first-open) order
}

// newFrameGraph creates an empty frame graph.
func newFrameGraph() *frameGraph {
	return &frameGraph{frames: map[string]*frameBuild{}}
}

// onFrameOpened records a FRAME_OPENED record.
func (g *frameGraph) onFrameOpened(rec *Record) *consolecore.Error {
	if rec.FrameID == "" || !rec.HasFrameType {
		return invalidityError(CategoryInvalidFrameRelationship, rec.TraceID)
	}
	if existing, dup := g.frames[rec.FrameID]; dup && existing.opened {
		return invalidityError(CategoryInvalidFrameRelationship, rec.TraceID)
	}
	if rec.HasParentFrame && rec.ParentFrameID == rec.FrameID {
		return invalidityError(CategoryInvalidFrameRelationship, rec.TraceID)
	}
	if rec.HasParentFrame {
		if _, ok := g.frames[rec.ParentFrameID]; !ok {
			return invalidityError(CategoryInvalidFrameRelationship, rec.TraceID)
		}
	}
	f := &frameBuild{
		frameID:             rec.FrameID,
		parentFrameID:       rec.ParentFrameID,
		hasParent:           rec.HasParentFrame,
		frameType:           rec.FrameType,
		route:               rec.Route,
		openedMillis:        rec.TimestampMillis,
		opened:              true,
		directUsageComplete: true,
		skillNames:          map[string]struct{}{},
		attemptIDs:          map[string]struct{}{},
		retrySequenceIDs:    map[string]struct{}{},
		validationStatuses:  map[string]struct{}{},
		failureIDs:          map[string]struct{}{},
	}
	g.frames[rec.FrameID] = f
	g.order = append(g.order, rec.FrameID)
	if rec.HasParentFrame {
		if parent, ok := g.frames[rec.ParentFrameID]; ok {
			parent.children = append(parent.children, rec.FrameID)
		}
	}
	return nil
}

// onFrameClosed records a FRAME_CLOSED record.
func (g *frameGraph) onFrameClosed(rec *Record) *consolecore.Error {
	if rec.FrameID == "" {
		return invalidityError(CategoryInvalidFrameRelationship, rec.TraceID)
	}
	f, ok := g.frames[rec.FrameID]
	if !ok || !f.opened {
		return invalidityError(CategoryInvalidFrameRelationship, rec.TraceID)
	}
	if f.closed {
		return invalidityError(CategoryInvalidFrameRelationship, rec.TraceID)
	}
	if !rec.HasFrameType || rec.FrameType != f.frameType {
		return invalidityError(CategoryInvalidFrameRelationship, rec.TraceID)
	}
	if rec.HasParentFrame != f.hasParent || rec.ParentFrameID != f.parentFrameID {
		return invalidityError(CategoryInvalidFrameRelationship, rec.TraceID)
	}
	if rec.Route != f.route || rec.TimestampMillis < f.openedMillis {
		return invalidityError(CategoryInvalidFrameRelationship, rec.TraceID)
	}
	f.closedMillis = rec.TimestampMillis
	f.closed = true
	return nil
}

// addDirectUsage adds response usage to an explicitly recorded frame. It
// rejects unknown frame references and arithmetic overflow.
func (g *frameGraph) addDirectUsage(frameID string, u Usage) (bool, bool) {
	return g.addDirectUsageWithCompleteness(frameID, u, true)
}

func (g *frameGraph) addDirectUsageWithCompleteness(frameID string, u Usage, complete bool) (bool, bool) {
	if f, ok := g.frames[frameID]; ok {
		var arithmeticOK bool
		f.directUsage, arithmeticOK = f.directUsage.plus(u)
		f.directUsageComplete = f.directUsageComplete && complete
		return true, arithmeticOK
	}
	return false, false
}

// associateRecord captures explicit record-to-frame cross references. It does
// not infer relationships from adjacency or text.
func (g *frameGraph) associateRecord(rec *Record) {
	if rec.FrameID == "" {
		return
	}
	f, ok := g.frames[rec.FrameID]
	if !ok {
		return
	}
	addSetValue(f.skillNames, rec.metadataStringOrEmpty("skillName"))
	addSetValue(f.attemptIDs, rec.metadataStringOrEmpty("attemptId"))
	addSetValue(f.retrySequenceIDs, rec.metadataStringOrEmpty("retrySequenceId"))
	if rec.Type == RecordAdvisorRequestMutation || rec.Type == RecordAdvisorResponseMutation {
		addSetValue(f.validationStatuses, rec.metadataStringOrEmpty("status"))
	}
	if rec.Type == RecordErrorRecorded {
		addSetValue(f.failureIDs, rec.metadataStringOrEmpty("failureId"))
	}
	if rec.Type == RecordFrameClosed {
		if status := rec.metadataStringOrEmpty("status"); status != "" {
			f.outcome = &status
		}
	}
}

func addSetValue(set map[string]struct{}, value string) {
	if value != "" {
		set[value] = struct{}{}
	}
}

func sortedSetValues(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	values := make([]string, 0, len(set))
	for value := range set {
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}

// validate checks the frame graph for missing parents, cycles, close-before-open,
// and complete child intervals outside their complete parent. It uses iterative
// traversal with explicit visitation states so deep valid chains do not grow the
// stack.
func (g *frameGraph) validate() *consolecore.Error {
	for _, id := range g.order {
		f := g.frames[id]
		if f.hasParent {
			if _, ok := g.frames[f.parentFrameID]; !ok {
				return invalidityError(CategoryInvalidFrameRelationship, f.frameID)
			}
		}
	}
	// Cycle detection via color marking: WHITE (unvisited), GRAY (on current
	// path), BLACK (fully validated). This is O(N) total: each frame's parent
	// chain is walked only until a BLACK ancestor is reached, and each frame is
	// marked BLACK exactly once.
	const (
		white = 0
		gray  = 1
		black = 2
	)
	colors := make(map[string]int, len(g.order))
	for _, id := range g.order {
		if colors[id] == black {
			continue
		}
		// Walk the parent chain from id, marking frames GRAY. If we hit a GRAY
		// frame, it's a cycle. If we hit a BLACK frame or a root, the chain is
		// acyclic; mark all GRAY frames BLACK.
		var path []string
		current := id
		for current != "" {
			if colors[current] == black {
				break
			}
			if colors[current] == gray {
				return invalidityError(CategoryInvalidFrameRelationship, id)
			}
			colors[current] = gray
			path = append(path, current)
			f, ok := g.frames[current]
			if !ok {
				break
			}
			if !f.hasParent {
				break
			}
			current = f.parentFrameID
		}
		for _, p := range path {
			colors[p] = black
		}
	}
	// Complete child interval outside its complete parent.
	for _, id := range g.order {
		f := g.frames[id]
		if !f.closed || !f.hasParent {
			continue
		}
		parent, ok := g.frames[f.parentFrameID]
		if !ok || !parent.closed {
			continue
		}
		if f.openedMillis < parent.openedMillis || f.closedMillis > parent.closedMillis {
			return invalidityError(CategoryInvalidFrameRelationship, f.frameID)
		}
	}
	return nil
}

// results computes the final frame results, gaps, and uncertainties in canonical
// (first-open) order. Duration and usage calculations match the Java fixture
// corpus exactly. It reports false if any usage accumulation overflows int64.
func (g *frameGraph) results() ([]frameResult, []gapResult, []uncertaintyResult, bool) {
	// Compute descendant usage bottom-up in one pass: process frames in reverse
	// insertion order so children are settled before parents, then accumulate
	// each frame's (direct + descendant) usage into its parent's descendant total.
	// This is O(frames + parent edges) instead of O(frames^2 * depth).
	descendant, descendantComplete, ok := g.computeDescendantUsage()
	if !ok {
		return nil, nil, nil, false
	}
	frames := make([]frameResult, 0, len(g.order))
	var gaps []gapResult
	var uncertainties []uncertaintyResult
	for _, id := range g.order {
		f := g.frames[id]
		var inclusiveDuration *int64
		var selfDuration *int64
		var closedTimestamp *int64
		if f.closed {
			d := f.closedMillis - f.openedMillis
			inclusiveDuration = &d
			closed := f.closedMillis
			closedTimestamp = &closed
			selfDuration = computeSelfDuration(f, g, &uncertainties)
		}
		desc := descendant[id]
		descComplete := descendantComplete[id]
		direct := f.directUsage
		inclusive, ok := direct.plus(desc)
		if !ok {
			return nil, nil, nil, false
		}
		var parentID *string
		if f.hasParent {
			p := f.parentFrameID
			parentID = &p
		}
		frames = append(frames, frameResult{
			FrameID:                 f.frameID,
			ParentFrameID:           parentID,
			ChildFrameIDs:           append([]string(nil), f.children...),
			FrameType:               string(f.frameType),
			Route:                   f.route,
			OpenedTimestampMillis:   f.openedMillis,
			ClosedTimestampMillis:   closedTimestamp,
			InclusiveDurationMillis: inclusiveDuration,
			SelfDurationMillis:      selfDuration,
			DirectUsage:             direct,
			DirectUsageComplete:     f.directUsageComplete,
			DescendantUsage:         desc,
			DescendantUsageComplete: descComplete,
			InclusiveUsage:          inclusive,
			InclusiveUsageComplete:  f.directUsageComplete && descComplete,
			SkillNames:              sortedSetValues(f.skillNames),
			Outcome:                 copyStringPointer(f.outcome),
			AttemptIDs:              sortedSetValues(f.attemptIDs),
			RetrySequenceIDs:        sortedSetValues(f.retrySequenceIDs),
			ValidationStatuses:      sortedSetValues(f.validationStatuses),
			FailureIDs:              sortedSetValues(f.failureIDs),
		})
		if !f.closed {
			gaps = append(gaps, gapResult{Kind: "OPEN_FRAME_NOT_CLOSED", FrameID: f.frameID})
		}
	}
	return frames, gaps, uncertainties, true
}

func copyStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

// computeDescendantUsage computes descendant usage for every frame in a single
// bottom-up pass. Frames are processed in reverse insertion (first-open) order
// so children are settled before parents. Each frame's inclusive usage (direct
// plus its own descendant usage) is added to its parent's descendant total.
// This is O(frames + parent edges). It reports false if any accumulation
// overflows int64.
func (g *frameGraph) computeDescendantUsage() (map[string]Usage, map[string]bool, bool) {
	descendant := make(map[string]Usage, len(g.order))
	complete := make(map[string]bool, len(g.order))
	for _, id := range g.order {
		complete[id] = true
	}
	for i := len(g.order) - 1; i >= 0; i-- {
		id := g.order[i]
		f := g.frames[id]
		// inclusive = direct + descendant (already computed for this frame)
		inclusive, ok := f.directUsage.plus(descendant[id])
		if !ok {
			return nil, nil, false
		}
		if f.hasParent {
			if _, ok := g.frames[f.parentFrameID]; !ok {
				continue
			}
			pDesc, ok := descendant[f.parentFrameID].plus(inclusive)
			if !ok {
				return nil, nil, false
			}
			descendant[f.parentFrameID] = pDesc
			complete[f.parentFrameID] = complete[f.parentFrameID] && f.directUsageComplete && complete[id]
		}
	}
	return descendant, complete, true
}

// computeSelfDuration computes self duration per the Java corpus algorithm: sum
// of immediate complete non-overlapping child durations subtracted from
// inclusive duration. If any child is incomplete or immediate child intervals
// overlap, self duration is marked unavailable with a precise uncertainty.
func computeSelfDuration(f *frameBuild, g *frameGraph, uncertainties *[]uncertaintyResult) *int64 {
	if len(f.children) == 0 {
		d := f.closedMillis - f.openedMillis
		return &d
	}
	intervals := make([]frameInterval, 0, len(f.children))
	for _, childID := range f.children {
		child := g.frames[childID]
		if !child.closed {
			*uncertainties = append(*uncertainties, uncertaintyResult{
				Kind: "SELF_DURATION_UNAVAILABLE_INCOMPLETE_CHILD", FrameID: f.frameID,
			})
			return nil
		}
		intervals = append(intervals, frameInterval{
			start: child.openedMillis,
			end:   child.closedMillis,
		})
	}
	sort.Slice(intervals, func(i, j int) bool { return intervals[i].start < intervals[j].start })
	latestEnd := int64(-1 << 62)
	overlaps := false
	var childDuration int64
	for _, iv := range intervals {
		if iv.start < latestEnd {
			overlaps = true
		}
		if iv.end > latestEnd {
			latestEnd = iv.end
		}
		childDuration += iv.end - iv.start
	}
	if overlaps {
		*uncertainties = append(*uncertainties, uncertaintyResult{
			Kind: "SELF_DURATION_UNAVAILABLE_OVERLAPPING_CHILDREN", FrameID: f.frameID,
		})
		return nil
	}
	inclusive := f.closedMillis - f.openedMillis
	self := inclusive - childDuration
	return &self
}

// frameInterval is one complete child frame's time interval.
type frameInterval struct {
	start int64
	end   int64
}
