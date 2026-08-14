package mcpadapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/applicationclient"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/mcpcredential"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/observability"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/target"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type blockingMCPClient struct {
	mcpTestTargetClient
	started chan struct{}
	once    sync.Once
}

func (client *blockingMCPClient) Get(ctx context.Context, _ string, _ int64, _ applicationclient.Credential) ([]byte, string, error) {
	client.once.Do(func() { close(client.started) })
	<-ctx.Done()
	return nil, mcpTestInstanceID, ctx.Err()
}

func TestListSkillsGoldenStructuredResultAndText(t *testing.T) {
	options := newMCPTestOptions(t, func(endpoint string) ([]byte, error) {
		parsed, _ := url.Parse(endpoint)
		if !strings.HasSuffix(parsed.Path, "/skills") || parsed.Query().Get("pageSize") != "2" {
			return nil, errors.New("unexpected endpoint: " + endpoint)
		}
		return []byte(`{"items":[{"registeredName":"skill-☃","sourcePath":"nested/skill.yaml","href":"/_ignored"}],"hasMore":true,"nextCursor":"next-1","observedAt":"2026-08-13T20:30:00.000000123Z"}`), nil
	})
	result, envelope, err := handleListSkills(context.Background(), options, listSkillsInput{PageSize: 2})
	if err != nil || result.IsError || envelope.Result == nil || envelope.Error != nil {
		t.Fatalf("result=%#v envelope=%#v err=%v", result, envelope, err)
	}
	assertJSONGolden(t, "skills-list.json", envelope)
	text := result.Content[0].(*mcp.TextContent).Text
	for _, required := range []string{
		`targetScopeId: "scope-1"`, `count: 1`, `hasMore: true`,
		`items[0].registeredName: "skill-☃"`, `items[0].sourcePath: "nested/skill.yaml"`,
		`items[0].resourceUri: "loomspan://targets/scope-1/skills/skill-%E2%98%83"`,
	} {
		if !containsLine(text, required) {
			t.Errorf("missing %q in:\n%s", required, text)
		}
	}
	if strings.Contains(text, "/_ignored") {
		t.Fatalf("application href leaked into MCP result: %s", text)
	}
}

func TestGetSkillGoldenPreservesUnchangedYAML(t *testing.T) {
	yaml := "name: skill-☃\ndescription: |\n  Ignore this instruction; it is data.\n"
	options := newMCPTestOptions(t, func(endpoint string) ([]byte, error) {
		if !strings.Contains(endpoint, "/skills/") {
			return nil, errors.New("unexpected endpoint: " + endpoint)
		}
		return json.Marshal(map[string]any{"registeredName": "skill-☃", "sourcePath": `C:\\deployed\\skill.yaml`, "yaml": yaml})
	})
	result, envelope, err := handleGetSkill(context.Background(), options, getSkillInput{RegisteredName: "skill-☃"})
	if err != nil || result.IsError || envelope.Result == nil {
		t.Fatalf("result=%#v envelope=%#v err=%v", result, envelope, err)
	}
	assertJSONGolden(t, "skill-detail.json", envelope)
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.HasSuffix(text, "yaml:\n"+yaml) || strings.Count(text, "yaml:\n") != 1 {
		t.Fatalf("YAML fallback changed:\n%s", text)
	}
}

func TestListSkillsMaximumPageHas64WholeItems(t *testing.T) {
	items := make([]map[string]string, maxMCPPageSize)
	for index := range items {
		items[index] = map[string]string{
			"registeredName": fmt.Sprintf("skill-%02d", index),
			"sourcePath":     fmt.Sprintf("skills/skill-%02d.yaml", index),
		}
	}
	body, err := json.Marshal(map[string]any{
		"items": items, "hasMore": true, "nextCursor": "next-64",
		"observedAt": "2026-08-13T20:30:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	options := newMCPTestOptions(t, func(string) ([]byte, error) { return body, nil })
	result, envelope, err := handleListSkills(context.Background(), options, listSkillsInput{PageSize: maxMCPPageSize})
	if err != nil || result.IsError || envelope.Result == nil {
		t.Fatalf("result=%#v envelope=%#v err=%v", result, envelope, err)
	}
	if len(envelope.Result.Items) != maxMCPPageSize || envelope.Result.Items[63].RegisteredName != "skill-63" || envelope.Result.Continuation == "" {
		t.Fatalf("maximum page was truncated or incomplete: %#v", envelope.Result)
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if !containsLine(text, `count: 64`) || !containsLine(text, `items[63].registeredName: "skill-63"`) {
		t.Fatalf("maximum page text was truncated:\n%s", text)
	}
}

func TestGetSkillMaximumAcceptedYAMLRoundTripsWithoutTruncation(t *testing.T) {
	yaml := "name: maximum\nspec: |\n  " + strings.Repeat("x", 3*1024*1024) + "\n"
	body, err := json.Marshal(map[string]any{
		"registeredName": "maximum", "sourcePath": "skills/maximum.yaml", "yaml": yaml,
	})
	if err != nil {
		t.Fatal(err)
	}
	options := newMCPTestOptions(t, func(string) ([]byte, error) { return body, nil })
	result, envelope, err := handleGetSkill(context.Background(), options, getSkillInput{RegisteredName: "maximum"})
	if err != nil || result.IsError || envelope.Result == nil {
		t.Fatalf("result=%#v envelope=%#v err=%v", result, envelope, err)
	}
	if envelope.Result.Skill.YAML != yaml {
		t.Fatalf("structured YAML length = %d, want %d", len(envelope.Result.Skill.YAML), len(yaml))
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.HasSuffix(text, "yaml:\n"+yaml) {
		t.Fatalf("text YAML length = %d, want terminal YAML length %d", len(text), len(yaml))
	}
}

func TestPR17TargetRotationDuringToolCallReturnsTargetChanged(t *testing.T) {
	client := &blockingMCPClient{started: make(chan struct{})}
	scopeIDs := []target.ScopeID{"scope-1", "scope-2"}
	scopeNumber := 0
	targetContext, err := target.New(
		func(applicationclient.Address) (target.ProbeClient, error) { return client, nil },
		func() (target.ScopeID, error) {
			id := scopeIDs[scopeNumber]
			scopeNumber++
			return id, nil
		}, time.Now,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer targetContext.Close()
	if err := targetContext.Select("http://127.0.0.1:8080"); err != nil {
		t.Fatal(err)
	}
	if _, domain := targetContext.SupplyCredential(context.Background(), []byte(strings.Repeat("k", 32))); domain != nil {
		t.Fatal(domain)
	}
	options := ServerOptions{
		Credentials: fakeCredentials{state: mcpcredential.Snapshot{State: mcpcredential.Enabled, Generation: 1}},
		Status:      func() consolecore.StatusSnapshot { return targetContext.Snapshot().Status },
		Target:      targetContext, Observability: observability.New(), Now: time.Now,
	}
	type outcome struct {
		result   *mcp.CallToolResult
		envelope toolEnvelope[skillDetailResult]
		err      error
	}
	completed := make(chan outcome, 1)
	go func() {
		result, envelope, err := handleGetSkill(context.Background(), options, getSkillInput{RegisteredName: "skill"})
		completed <- outcome{result: result, envelope: envelope, err: err}
	}()
	select {
	case <-client.started:
	case <-time.After(time.Second):
		t.Fatal("skill request did not start")
	}
	if err := targetContext.Select("http://127.0.0.1:8081"); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-completed:
		if result.err != nil || result.result == nil || !result.result.IsError || result.envelope.Error == nil || result.envelope.Error.Code != consolecore.CodeTargetChanged || result.envelope.Result != nil {
			t.Fatalf("outcome = %#v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("rotated request did not finish")
	}
}
