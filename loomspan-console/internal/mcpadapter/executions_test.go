package mcpadapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestListExecutionsGoldenPreservesBoundedActiveSummaries(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("..", "..", "..", "loomspan-console-fixtures", "application-rest", "active-executions-page.json"))
	if err != nil {
		t.Fatal(err)
	}
	options := newMCPTestOptions(t, func(endpoint string) ([]byte, error) {
		if !strings.Contains(endpoint, "/active-executions?") {
			return nil, errors.New("unexpected endpoint: " + endpoint)
		}
		return fixture, nil
	})
	result, envelope, err := handleListExecutions(context.Background(), options, listExecutionsInput{PageSize: 64})
	if err != nil || result.IsError || envelope.Result == nil {
		t.Fatalf("result=%#v envelope=%#v err=%v", result, envelope, err)
	}
	assertJSONGolden(t, "executions-list.json", envelope)
	text := result.Content[0].(*mcp.TextContent).Text
	for _, required := range []string{`items[0].sessionId: "session-1"`, `items[0].status: "ACTIVE"`, `items[0].phase: "RUNNING"`} {
		if !containsLine(text, required) {
			t.Errorf("missing %q in:\n%s", required, text)
		}
	}
	if strings.Contains(text, "outcome") || strings.Contains(text, "selfDuration") {
		t.Fatalf("execution list invented finalized facts: %s", text)
	}
}

func TestGetExecutionGoldenPreservesProvisionalFactsWithoutDiagnosis(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("..", "..", "..", "loomspan-console-fixtures", "application-rest", "active-execution-detail.json"))
	if err != nil {
		t.Fatal(err)
	}
	options := newMCPTestOptions(t, func(endpoint string) ([]byte, error) {
		if !strings.Contains(endpoint, "/active-executions/session-1") {
			return nil, errors.New("unexpected endpoint: " + endpoint)
		}
		return fixture, nil
	})
	result, envelope, err := handleGetExecution(context.Background(), options, getExecutionInput{SessionID: "session-1"})
	if err != nil || result.IsError || envelope.Result == nil {
		t.Fatalf("result=%#v envelope=%#v err=%v", result, envelope, err)
	}
	assertJSONGolden(t, "execution-detail.json", envelope)
	text := result.Content[0].(*mcp.TextContent).Text
	for _, required := range []string{
		`execution.sessionId: "session-1"`, `execution.status: "ACTIVE"`,
		`execution.activePath.count: 0`, `execution.usage.usageUnits: 15`,
		`execution.configuredLimits.maxUsageUnits: 200000`,
	} {
		if !containsLine(text, required) {
			t.Errorf("missing %q in:\n%s", required, text)
		}
	}
}

func TestListExecutionsMaximumPageHas64WholeItems(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("..", "..", "..", "loomspan-console-fixtures", "application-rest", "active-executions-page.json"))
	if err != nil {
		t.Fatal(err)
	}
	var page struct {
		Items      []map[string]any `json:"items"`
		HasMore    bool             `json:"hasMore"`
		NextCursor *string          `json:"nextCursor"`
		ObservedAt string           `json:"observedAt"`
	}
	if err := json.Unmarshal(fixture, &page); err != nil || len(page.Items) == 0 {
		t.Fatalf("decode execution fixture: items=%d err=%v", len(page.Items), err)
	}
	prototype := page.Items[0]
	page.Items = make([]map[string]any, maxMCPPageSize)
	for index := range page.Items {
		encoded, marshalErr := json.Marshal(prototype)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		var item map[string]any
		if err := json.Unmarshal(encoded, &item); err != nil {
			t.Fatal(err)
		}
		item["sessionId"] = fmt.Sprintf("session-%02d", index)
		item["traceId"] = fmt.Sprintf("trace-%02d", index)
		page.Items[index] = item
	}
	next := "next-64"
	page.HasMore, page.NextCursor = true, &next
	body, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	options := newMCPTestOptions(t, func(string) ([]byte, error) { return body, nil })
	result, envelope, err := handleListExecutions(context.Background(), options, listExecutionsInput{PageSize: maxMCPPageSize})
	if err != nil || result.IsError || envelope.Result == nil {
		t.Fatalf("result=%#v envelope=%#v err=%v", result, envelope, err)
	}
	if len(envelope.Result.Items) != maxMCPPageSize || envelope.Result.Items[63].SessionID != "session-63" || envelope.Result.Continuation == "" {
		t.Fatalf("maximum execution page was truncated or incomplete")
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if !containsLine(text, `count: 64`) || !containsLine(text, `items[63].sessionId: "session-63"`) {
		t.Fatalf("maximum execution page text was truncated")
	}
}
