package traceanalysis

import (
	"encoding/json"
	"testing"
)

func TestPlanGraphDerivesStableNestedMissionLineage(t *testing.T) {
	frames := &frameGraph{frames: map[string]*frameBuild{
		"trace-root":     {frameID: "trace-root", frameType: FrameRootMission},
		"nested-skill":   {frameID: "nested-skill", frameType: FrameSkillExecution, hasParent: true, parentFrameID: "trace-root"},
		"nested-mission": {frameID: "nested-mission", frameType: FrameRootMission, hasParent: true, parentFrameID: "nested-skill"},
		"planning":       {frameID: "planning", frameType: FramePlanning, hasParent: true, parentFrameID: "nested-mission"},
		"step":           {frameID: "step", frameType: FrameStepExecution, hasParent: true, parentFrameID: "nested-mission"},
	}}
	graph := newPlanGraph()
	created := &Record{TraceID: "trace", Sequence: 10, Type: RecordPlanCreated, FrameID: "planning", Metadata: json.RawMessage(`{"planId":"plan-1","attemptId":"attempt-1","retrySequenceId":"retry-1"}`)}
	updated := &Record{TraceID: "trace", Sequence: 20, Type: RecordPlanUpdated, FrameID: "step", Metadata: json.RawMessage(`{"planId":"plan-1"}`)}
	if domain := graph.onRecord(created, frames); domain != nil {
		t.Fatal(domain)
	}
	if domain := graph.onRecord(updated, frames); domain != nil {
		t.Fatal(domain)
	}
	landmarks, domain := graph.landmarks(frames, "trace")
	if domain != nil {
		t.Fatal(domain)
	}
	for _, sequence := range []int64{10, 20} {
		landmark := landmarks[sequence]
		if landmark.TraceRootFrameID != "trace-root" || landmark.MissionFrameID != "nested-mission" || landmark.PlanningFrameID != "planning" {
			t.Fatalf("sequence %d lineage=%+v", sequence, landmark)
		}
	}
	if landmarks[10].AttemptID != "attempt-1" || landmarks[10].RetrySequenceID != "retry-1" || landmarks[20].AttemptID != "" || landmarks[20].RetrySequenceID != "" {
		t.Fatalf("creation-only attempt lineage: created=%+v updated=%+v", landmarks[10], landmarks[20])
	}
}

func TestPlanGraphRejectsInvalidLineage(t *testing.T) {
	frames := &frameGraph{frames: map[string]*frameBuild{
		"root":     {frameID: "root", frameType: FrameRootMission},
		"planning": {frameID: "planning", frameType: FramePlanning, hasParent: true, parentFrameID: "root"},
		"step":     {frameID: "step", frameType: FrameStepExecution, hasParent: true, parentFrameID: "root"},
	}}
	tests := map[string][]*Record{
		"missing plan id":  {{TraceID: "trace", Sequence: 1, Type: RecordPlanCreated, FrameID: "planning", Metadata: json.RawMessage(`{}`)}},
		"blank plan id":    {{TraceID: "trace", Sequence: 1, Type: RecordPlanCreated, FrameID: "planning", Metadata: json.RawMessage(`{"planId":"   "}`)}},
		"outside planning": {{TraceID: "trace", Sequence: 1, Type: RecordPlanCreated, FrameID: "step", Metadata: json.RawMessage(`{"planId":"p"}`)}},
		"update first":     {{TraceID: "trace", Sequence: 1, Type: RecordPlanUpdated, FrameID: "step", Metadata: json.RawMessage(`{"planId":"p"}`)}},
		"duplicate creation": {
			{TraceID: "trace", Sequence: 1, Type: RecordPlanCreated, FrameID: "planning", Metadata: json.RawMessage(`{"planId":"p"}`)},
			{TraceID: "trace", Sequence: 2, Type: RecordPlanCreated, FrameID: "planning", Metadata: json.RawMessage(`{"planId":"p"}`)},
		},
	}
	for name, records := range tests {
		t.Run(name, func(t *testing.T) {
			graph := newPlanGraph()
			var domainErr any
			for _, record := range records {
				if domain := graph.onRecord(record, frames); domain != nil {
					domainErr = domain
					category, ok := categoryOf(domain)
					if !ok || category != CategoryInvalidPlanLineage {
						t.Fatalf("category=%v domain=%v", category, domain)
					}
					break
				}
			}
			if domainErr == nil {
				t.Fatal("expected INVALID_PLAN_LINEAGE")
			}
		})
	}
}
