package traceanalysis

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
)

// TestBundleContainsOnlyClosedInternalComponents proves a processed bundle
// contains only the raw artifact plus closed internal analysis components with
// no trace/handle-derived filesystem name. Every component name is a closed
// logical identifier, never a path or a trace/handle-derived name.
func TestBundleContainsOnlyClosedInternalComponents(t *testing.T) {
	sink := &fakeSink{}
	processor := New()
	_, domain := processor.Process(artifact.ProcessRequest{
		Context:  context.Background(),
		Metadata: artifact.TraceMetadata{TraceID: "trace-t", SessionID: "session-t", Outcome: "SUCCEEDED"},
		Raw:      strings.NewReader(minimalValidTrace),
		Sink:     sink,
	})
	if domain != nil {
		t.Fatalf("Process failed: %v", domain)
	}

	// The set of expected derived component names. These are closed logical
	// identifiers — no trace ID, handle, or filesystem path appears in any name.
	expected := map[artifact.ComponentName]bool{
		artifact.ComponentName(ComponentManifest):      true,
		artifact.ComponentName(ComponentRecordIndex):   true,
		artifact.ComponentName(ComponentFrameIndex):    true,
		artifact.ComponentName(ComponentFrameDuration): true,
		artifact.ComponentName(ComponentFrameUsage):    true,
		artifact.ComponentName(ComponentAttemptIndex):  true,
		artifact.ComponentName(ComponentRetryIndex):    true,
		artifact.ComponentName(ComponentValidationIdx): true,
		artifact.ComponentName(ComponentFailureIndex):  true,
		artifact.ComponentName(ComponentUsageIndex):    true,
		artifact.ComponentName(ComponentGapIndex):      true,
		artifact.ComponentName(ComponentUncertainty):   true,
		artifact.ComponentName(ComponentPayloadStore):  true,
		artifact.ComponentName(ComponentPayloadIndex):  true,
		artifact.ComponentName(ComponentRecordFacts):   true,
		artifact.ComponentName(ComponentRecordFactIdx): true,
	}

	// Every written component must be in the expected set.
	for name := range sink.components {
		if !expected[name] {
			t.Fatalf("unexpected component %q in bundle; derived components must be closed logical identifiers", name)
		}
	}

	// Every expected component must have been written.
	for name := range expected {
		if _, ok := sink.components[name]; !ok {
			t.Fatalf("expected component %q was not written", name)
		}
	}

	// No component name may contain a path separator, the trace ID, or the
	// handle — proving no trace/handle-derived filesystem name leaks.
	for name := range sink.components {
		if strings.ContainsAny(string(name), `/\`) {
			t.Fatalf("component name %q contains a path separator", name)
		}
		if strings.Contains(string(name), "trace-t") {
			t.Fatalf("component name %q contains the trace ID", name)
		}
	}
}

// TestNestedHierarchyCalculationsMatchJavaExpected proves the processor's
// calculated frame duration and usage facts for the nested-frame-usage fixture
// match the committed Java expected file exactly. This is the manual
// verification item: "Compare one nested hierarchy's calculated duration and
// usage facts manually against its Java expected file."
func TestNestedHierarchyCalculationsMatchJavaExpected(t *testing.T) {
	root := fixtureRoot(t)
	tracePath := filepath.Join(root, "traces", "nested-frame-usage.ndjson")
	expectedPath := filepath.Join(root, "expected", "nested-frame-usage.json")

	traceBytes, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatalf("read trace: %v", err)
	}
	expectedBytes, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("read expected: %v", err)
	}

	var expected expectedFile
	if err := json.Unmarshal(expectedBytes, &expected); err != nil {
		t.Fatalf("parse expected: %v", err)
	}

	sink := &fakeSink{}
	processor := newProcessorForVersion(fixtureCompatibilityVersion)
	_, domain := processor.Process(artifact.ProcessRequest{
		Context: context.Background(),
		Metadata: artifact.TraceMetadata{
			TraceID:   expected.TraceID,
			SessionID: expected.SessionID,
			Outcome:   expected.Outcome,
		},
		Raw:  bytes.NewReader(traceBytes),
		Sink: sink,
	})
	if domain != nil {
		t.Fatalf("Process failed: %v", domain)
	}

	// Read the frame index and unmarshal each row.
	frameIdxBytes, ok := sink.components[artifact.ComponentName(ComponentFrameIndex)]
	if !ok {
		t.Fatal("expected frame index component")
	}
	var frames []frameResult
	r := bytes.NewReader(frameIdxBytes)
	for r.Len() > 0 {
		row, err := readLengthPrefixed(r)
		if err != nil {
			t.Fatalf("read frame row: %v", err)
		}
		var fr frameResult
		if err := json.Unmarshal(row, &fr); err != nil {
			t.Fatalf("parse frame row: %v", err)
		}
		frames = append(frames, fr)
	}

	// Parse the expected frames from the committed file.
	var expectedFrames []frameResult
	if err := json.Unmarshal(expected.Frames, &expectedFrames); err != nil {
		t.Fatalf("parse expected frames: %v", err)
	}
	if len(expectedFrames) == 0 {
		t.Fatal("expected file has no frames")
	}
	if len(frames) != len(expectedFrames) {
		t.Fatalf("frame count: got %d want %d", len(frames), len(expectedFrames))
	}

	for i, got := range frames {
		want := expectedFrames[i]
		if got.FrameID != want.FrameID {
			t.Errorf("frame %d frameId: got %s want %s", i, got.FrameID, want.FrameID)
		}
		if got.FrameType != want.FrameType {
			t.Errorf("frame %d frameType: got %s want %s", i, got.FrameType, want.FrameType)
		}
		// Parent frame ID: nil means root.
		if (got.ParentFrameID == nil) != (want.ParentFrameID == nil) {
			t.Errorf("frame %d parentFrameId presence: got %v want %v", i, got.ParentFrameID, want.ParentFrameID)
		}
		if got.ParentFrameID != nil && *got.ParentFrameID != *want.ParentFrameID {
			t.Errorf("frame %d parentFrameId: got %s want %s", i, *got.ParentFrameID, *want.ParentFrameID)
		}
		// Inclusive duration.
		if (got.InclusiveDurationMillis == nil) != (want.InclusiveDurationMillis == nil) {
			t.Errorf("frame %d inclusiveDuration presence: got %v want %v", i, got.InclusiveDurationMillis, want.InclusiveDurationMillis)
		}
		if got.InclusiveDurationMillis != nil && *got.InclusiveDurationMillis != *want.InclusiveDurationMillis {
			t.Errorf("frame %d inclusiveDuration: got %d want %d", i, *got.InclusiveDurationMillis, *want.InclusiveDurationMillis)
		}
		// Self duration.
		if (got.SelfDurationMillis == nil) != (want.SelfDurationMillis == nil) {
			t.Errorf("frame %d selfDuration presence: got %v want %v", i, got.SelfDurationMillis, want.SelfDurationMillis)
		}
		if got.SelfDurationMillis != nil && *got.SelfDurationMillis != *want.SelfDurationMillis {
			t.Errorf("frame %d selfDuration: got %d want %d", i, *got.SelfDurationMillis, *want.SelfDurationMillis)
		}
		// Usage: direct, descendant, inclusive.
		if got.DirectUsage != want.DirectUsage {
			t.Errorf("frame %d directUsage: got %+v want %+v", i, got.DirectUsage, want.DirectUsage)
		}
		if got.DescendantUsage != want.DescendantUsage {
			t.Errorf("frame %d descendantUsage: got %+v want %+v", i, got.DescendantUsage, want.DescendantUsage)
		}
		if got.InclusiveUsage != want.InclusiveUsage {
			t.Errorf("frame %d inclusiveUsage: got %+v want %+v", i, got.InclusiveUsage, want.InclusiveUsage)
		}
	}

	// Also verify unframed attributed usage matches.
	usageIdxBytes, ok := sink.components[artifact.ComponentName(ComponentUsageIndex)]
	if !ok {
		t.Fatal("expected usage index component")
	}
	usageR := bytes.NewReader(usageIdxBytes)
	var unframedAttributed Usage
	for usageR.Len() > 0 {
		row, err := readLengthPrefixed(usageR)
		if err != nil {
			t.Fatalf("read usage row: %v", err)
		}
		var fact map[string]any
		if err := json.Unmarshal(row, &fact); err != nil {
			t.Fatalf("parse usage row: %v", err)
		}
		if fact["kind"] == "UNFRAMED_ATTRIBUTED" {
			unframedAttributed = extractUsageFact(fact, "UNFRAMED_ATTRIBUTED")
		}
	}
	var expectedUnframed Usage
	if len(expected.UnframedAttributedUsage) > 0 {
		if err := json.Unmarshal(expected.UnframedAttributedUsage, &expectedUnframed); err != nil {
			t.Fatalf("parse expected unframed usage: %v", err)
		}
	}
	if unframedAttributed != expectedUnframed {
		t.Errorf("unframedAttributedUsage: got %+v want %+v", unframedAttributed, expectedUnframed)
	}
}
