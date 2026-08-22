package mcpadapter

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type dualOutputMeasurement struct {
	FullResponse               int
	TextWireContribution       int
	StructuredWireContribution int
}

func TestActiveInspectionDualOutputBaselines(t *testing.T) {
	executionList := loadResultGolden[executionListResult](t, "executions-list.json")
	executionDetail := loadResultGolden[executionDetailResult](t, "execution-detail.json")
	activity := loadResultGolden[activityResult](t, "activity.json")

	maximumExecutions := executionList
	prototypeExecution := executionList.Items[0]
	maximumExecutions.Items = make([]executionDTO, maxMCPPageSize)
	for index := range maximumExecutions.Items {
		item := prototypeExecution
		item.SessionID = fmt.Sprintf("session-%02d", index)
		item.TraceID = fmt.Sprintf("trace-%02d", index)
		maximumExecutions.Items[index] = item
	}
	maximumExecutions.HasMore = true
	maximumExecutions.Continuation = "continuation-after-64-executions"

	maximumActivity := activityResult{
		ObservedAt: time.Date(2026, 8, 13, 21, 0, 0, 123, time.UTC),
		Items:      make([]activityDTO, maxMCPPageSize),
		ReturnedCursorRange: &cursorRangeDTO{
			FirstCursor: "1",
			LastCursor:  "64",
		},
		HasMore:      true,
		Continuation: "continuation-after-64-activities",
		Continuity: &continuityDTO{
			IntervalID:  "interval-1",
			FirstCursor: "1",
			LastCursor:  "64",
			ObservedAt:  time.Date(2026, 8, 13, 20, 0, 0, 0, time.UTC),
		},
		Coverage: coverageDTO{
			SessionStartCursor: "1",
			SessionRetainedCursorRange: &cursorRangeDTO{
				FirstCursor: "1",
				LastCursor:  "64",
			},
		},
	}
	for index := range maximumActivity.Items {
		sequence := int64(index + 1)
		maximumActivity.Items[index] = activityDTO{
			Cursor:            fmt.Sprintf("%d", index+1),
			SessionID:         "session-1",
			TraceID:           "trace-1",
			CanonicalSequence: &sequence,
			Timestamp:         time.Date(2026, 8, 13, 20, 0, index+1, 0, time.UTC),
			Kind:              "MODEL_ATTEMPT_FAILED",
			ExecutionStatus:   "ACTIVE",
			FrameID:           fmt.Sprintf("frame-%02d", index+1),
			FrameType:         "MODEL",
			Route:             "model",
			Summary:           fmt.Sprintf("attempt-%02d", index+1),
			Details: map[string]any{
				"largeInteger": json.Number("9007199254740993"),
				"payload":      strings.Repeat("x", 11*1024),
			},
		}
	}

	tests := []struct {
		name       string
		structured any
		fallback   string
		want       dualOutputMeasurement
	}{
		{
			"execution list, one item", toolEnvelope[executionListResult]{Result: &executionList}, executionListText(executionList),
			dualOutputMeasurement{FullResponse: 2101, TextWireContribution: 1245, StructuredWireContribution: 808},
		},
		{
			"execution detail", toolEnvelope[executionDetailResult]{Result: &executionDetail}, executionDetailText(executionDetail),
			dualOutputMeasurement{FullResponse: 2094, TextWireContribution: 1242, StructuredWireContribution: 804},
		},
		{
			"activity, one item", toolEnvelope[activityResult]{Result: &activity}, activityText(activity),
			dualOutputMeasurement{FullResponse: 2376, TextWireContribution: 1288, StructuredWireContribution: 1040},
		},
		{
			"execution list, maximum page", toolEnvelope[executionListResult]{Result: &maximumExecutions}, executionListText(maximumExecutions),
			dualOutputMeasurement{FullResponse: 120737, TextWireContribution: 74785, StructuredWireContribution: 45904},
		},
		{
			"activity, maximum page", toolEnvelope[activityResult]{Result: &maximumActivity}, activityText(maximumActivity),
			dualOutputMeasurement{FullResponse: 768234, TextWireContribution: 26908, StructuredWireContribution: 741278},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := measureDualOutputResponse(t, test.fallback, test.structured)
			t.Logf("full=%d text-contribution=%d structured-contribution=%d", got.FullResponse, got.TextWireContribution, got.StructuredWireContribution)
			if got != test.want {
				t.Fatalf("measurement=%+v want=%+v", got, test.want)
			}
		})
	}

	maximumActivityText := activityText(maximumActivity)
	if strings.Contains(maximumActivityText, strings.Repeat("x", 64)) || strings.Contains(maximumActivityText, "largeInteger") {
		t.Fatal("maximum activity fallback exposed structured-only details")
	}
}

func loadResultGolden[T any](t *testing.T, name string) T {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var envelope toolEnvelope[T]
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	if err := decoder.Decode(&envelope); err != nil || envelope.Result == nil {
		t.Fatalf("decode %s: result=%#v err=%v", name, envelope.Result, err)
	}
	return *envelope.Result
}

func measureDualOutputResponse(t *testing.T, fallback string, structured any) dualOutputMeasurement {
	t.Helper()
	encode := func(content []mcp.Content, structuredContent any) int {
		body, err := json.Marshal(struct {
			JSONRPC string             `json:"jsonrpc"`
			ID      int                `json:"id"`
			Result  mcp.CallToolResult `json:"result"`
		}{
			JSONRPC: "2.0",
			ID:      2,
			Result: mcp.CallToolResult{
				Content:           content,
				StructuredContent: structuredContent,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		return len(body)
	}
	text := []mcp.Content{&mcp.TextContent{Text: fallback}}
	full := encode(text, structured)
	withoutText := encode([]mcp.Content{}, structured)
	withoutStructured := encode(text, nil)
	return dualOutputMeasurement{
		FullResponse:               full,
		TextWireContribution:       full - withoutText,
		StructuredWireContribution: full - withoutStructured,
	}
}
