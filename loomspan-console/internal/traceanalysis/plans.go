package traceanalysis

import (
	"strings"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
)

type planRecordBuild struct {
	sequence        int64
	created         bool
	attemptID       string
	retrySequenceID string
}

type planLineageBuild struct {
	planID          string
	planningFrameID string
	records         []planRecordBuild
}

type planGraph struct {
	lineages map[string]*planLineageBuild
}

func newPlanGraph() *planGraph { return &planGraph{lineages: map[string]*planLineageBuild{}} }

func (g *planGraph) onRecord(rec *Record, frames *frameGraph) *consolecore.Error {
	if rec.Type != RecordPlanCreated && rec.Type != RecordPlanUpdated {
		return nil
	}
	planID := rec.metadataStringOrEmpty("planId")
	if strings.TrimSpace(planID) == "" {
		return invalidityError(CategoryInvalidPlanLineage, rec.TraceID)
	}
	lineage := g.lineages[planID]
	if rec.Type == RecordPlanCreated {
		if lineage != nil || rec.FrameID == "" {
			return invalidityError(CategoryInvalidPlanLineage, rec.TraceID)
		}
		frame := frames.frames[rec.FrameID]
		if frame == nil || frame.frameType != FramePlanning {
			return invalidityError(CategoryInvalidPlanLineage, rec.TraceID)
		}
		lineage = &planLineageBuild{planID: planID, planningFrameID: rec.FrameID}
		g.lineages[planID] = lineage
		lineage.records = append(lineage.records, planRecordBuild{
			sequence: rec.Sequence, created: true,
			attemptID: rec.metadataStringOrEmpty("attemptId"), retrySequenceID: rec.metadataStringOrEmpty("retrySequenceId"),
		})
		return nil
	}
	if lineage == nil {
		return invalidityError(CategoryInvalidPlanLineage, rec.TraceID)
	}
	lineage.records = append(lineage.records, planRecordBuild{sequence: rec.Sequence})
	return nil
}

func (g *planGraph) landmarks(frames *frameGraph, scopeID string) (map[int64]PlanLandmark, *consolecore.Error) {
	out := map[int64]PlanLandmark{}
	for _, lineage := range g.lineages {
		current := lineage.planningFrameID
		traceRoot := ""
		mission := ""
		seen := map[string]bool{}
		for current != "" && !seen[current] {
			seen[current] = true
			frame := frames.frames[current]
			if frame == nil {
				return nil, invalidityError(CategoryInvalidPlanLineage, scopeID)
			}
			if mission == "" && frame.frameType == FrameRootMission {
				mission = current
			}
			if !frame.hasParent {
				traceRoot = current
				if frame.frameType != FrameRootMission {
					return nil, invalidityError(CategoryInvalidPlanLineage, scopeID)
				}
				break
			}
			current = frame.parentFrameID
		}
		if traceRoot == "" || mission == "" {
			return nil, invalidityError(CategoryInvalidPlanLineage, scopeID)
		}
		for _, record := range lineage.records {
			landmark := PlanLandmark{
				PlanID: lineage.planID, Sequence: record.sequence,
				TraceRootFrameID: traceRoot, MissionFrameID: mission, PlanningFrameID: lineage.planningFrameID,
			}
			if record.created {
				landmark.AttemptID = record.attemptID
				landmark.RetrySequenceID = record.retrySequenceID
			}
			out[record.sequence] = landmark
		}
	}
	return out, nil
}
