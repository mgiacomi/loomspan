package mcpadapter

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/evidence"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/mcpcredential"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/traceanalysis"
)

func TestTraceResourceURIParsingAcceptsExactTargetAndImportedForms(t *testing.T) {
	handle := strings.Repeat("a", 64)
	cases := []struct {
		uri      string
		source   evidence.Source
		selector string
	}{
		{"loomspan://targets/scope-1/artifacts/" + handle + "/summary", evidence.SourceTarget, ""},
		{"loomspan://targets/scope-1/artifacts/" + handle + "/frames/frame-%E2%98%83", evidence.SourceTarget, "frame-☃"},
		{"loomspan://imports/artifacts/" + handle + "/records/42", evidence.SourceImported, "42"},
	}
	for _, tc := range cases {
		parsed, domain := parseTraceResourceURI(tc.uri)
		if domain != nil || parsed.Ref.Source != tc.source || parsed.Selector != tc.selector {
			t.Fatalf("uri=%s parsed=%#v domain=%v", tc.uri, parsed, domain)
		}
	}
}

func TestImportedTraceResourcesMaterializeToolDTOsWithoutTarget(t *testing.T) {
	now := time.Date(2026, 8, 14, 13, 0, 0, 0, time.UTC)
	handle := artifact.Handle("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	traceCtx := traceanalysis.TraceContext{Evidence: evidence.ForImported(), Handle: handle, TraceID: "trace-i", SessionID: "session-i"}
	emptyFacts := traceanalysis.RecordFacts{Attempts: []traceanalysis.AttemptSummary{}, Retries: []traceanalysis.RetrySummary{}, Validations: []traceanalysis.ValidationSummary{}, Failures: []traceanalysis.FailureSummary{}, Payloads: []traceanalysis.PayloadDescriptor{}, SearchMatches: []traceanalysis.SearchResult{}}
	analysis := &fakeTraceAnalysis{
		summary: traceanalysis.TraceSummary{Context: traceCtx, RootFrameIDs: []string{}},
		frames:  traceanalysis.Page[traceanalysis.FrameSummary]{Context: traceCtx, Items: []traceanalysis.FrameSummary{{Context: traceCtx, FrameID: "frame-1", ChildFrameIDs: []string{}, GapKinds: []string{}, UncertaintyKinds: []string{}}}},
		records: traceanalysis.Page[traceanalysis.RecordSummary]{Context: traceCtx, Items: []traceanalysis.RecordSummary{{Context: traceCtx, Sequence: 1, Type: "TRACE_STARTED", Facts: emptyFacts}}},
	}
	options := ServerOptions{Credentials: fakeCredentials{state: mcpcredential.Snapshot{State: mcpcredential.Enabled}}, Now: func() time.Time { return now }, TraceAnalysis: analysis}
	for _, uri := range []string{"loomspan://imports/artifacts/" + string(handle) + "/summary", "loomspan://imports/artifacts/" + string(handle) + "/frames/frame-1", "loomspan://imports/artifacts/" + string(handle) + "/records/1"} {
		result, err := readTraceResource(context.Background(), options, uri)
		if err != nil || len(result.Contents) != 1 || result.Contents[0].MIMEType != traceResourceMIMEType || !strings.Contains(result.Contents[0].Text, "trace-i") && strings.HasSuffix(uri, "/summary") {
			t.Fatalf("uri=%s result=%#v err=%v", uri, result, err)
		}
	}
	for _, ref := range analysis.refs {
		if ref.Source != evidence.SourceImported || ref.TargetScope != "" {
			t.Fatalf("resource invented target: %#v", ref)
		}
	}
}

func TestTraceResourceURIParsingRejectsNoncanonicalAndUnsafeForms(t *testing.T) {
	handle := strings.Repeat("a", 64)
	for _, uri := range []string{"https://imports/artifacts/" + handle + "/summary", "loomspan://imports/artifacts/" + handle + "/summary?x=1", "loomspan://imports/artifacts/" + handle + "/records/0", "loomspan://imports/artifacts/" + handle + "/records/01", "loomspan://targets/scope/artifacts/" + handle + "/frames/%66rame", "loomspan://imports/artifacts/" + handle + "/raw", "loomspan://imports/artifacts/handle-1/summary", "loomspan://imports/artifacts/" + strings.Repeat("A", 64) + "/summary", "loomspan://imports/artifacts/" + strings.Repeat("a", 63) + "/summary"} {
		if _, domain := parseTraceResourceURI(uri); domain == nil {
			t.Fatalf("accepted %s", uri)
		}
	}
}
