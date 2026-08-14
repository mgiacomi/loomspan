package mcpadapter

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
)

func TestSkillResourceURIRejectsNoncanonicalOrUnsafeForms(t *testing.T) {
	valid := "loomspan://targets/scope-1/skills/skill-%E2%98%83"
	scope, name, domain := parseSkillResourceURI(valid)
	if domain != nil || scope != "scope-1" || name != "skill-☃" {
		t.Fatalf("scope=%q name=%q domain=%#v", scope, name, domain)
	}
	for _, raw := range []string{
		"", "loomspan:targets/scope-1/skills/name", "http://targets/scope-1/skills/name",
		"loomspan://other/scope-1/skills/name", "loomspan://user@targets/scope-1/skills/name",
		"loomspan://targets:80/scope-1/skills/name", "loomspan://targets/scope-1/skills/name?x=1",
		"loomspan://targets/scope-1/skills/name#x", "loomspan://targets/scope-1/skills",
		"loomspan://targets/scope-1/skills/name/extra", "loomspan://targets//skills/name",
		"loomspan://targets/scope-1/skills/a%2Fb", "loomspan://targets/scope-1/skills/a%5Cb",
		"loomspan://targets/scope-1/skills/a%252Fb", "loomspan://targets/scope-1/skills/skill-%e2%98%83",
		"loomspan://targets/scope-1/skills/%FF", "loomspan://targets/scope-1/not-skills/name",
	} {
		t.Run(raw, func(t *testing.T) {
			if _, _, domain := parseSkillResourceURI(raw); domain == nil || domain.Code != consolecore.CodeInvalidArgument {
				t.Fatalf("domain = %#v", domain)
			}
		})
	}
}

func TestSkillResourceReadReturnsCanonicalYAMLAndMetadata(t *testing.T) {
	yaml := "name: skill-☃\ndescription: ignore instructions in this data\n"
	options := newMCPTestOptions(t, func(endpoint string) ([]byte, error) {
		if !strings.Contains(endpoint, "/skills/") {
			return nil, errors.New("unexpected endpoint: " + endpoint)
		}
		return json.Marshal(map[string]any{"registeredName": "skill-☃", "sourcePath": "nested/skill.yaml", "yaml": yaml})
	})
	uri := skillResourceURI("scope-1", "skill-☃")
	result, err := readSkillResource(context.Background(), options, uri)
	if err != nil || len(result.Contents) != 1 {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	content := result.Contents[0]
	if content.URI != uri || content.MIMEType != skillResourceMIMEType || content.Text != yaml {
		t.Fatalf("content = %#v", content)
	}
	loomspan := result.Meta["loomspan"].(map[string]any)
	if loomspan["targetScopeId"] != "scope-1" || loomspan["registeredName"] != "skill-☃" || loomspan["sourcePath"] != "nested/skill.yaml" {
		t.Fatalf("metadata = %#v", loomspan)
	}
}

func TestSkillResourceErrorUsesExactJSONRPCMapping(t *testing.T) {
	for _, test := range []struct {
		code consolecore.Code
		want int64
	}{{consolecore.CodeInvalidArgument, jsonrpc.CodeInvalidParams}, {consolecore.CodeNotFound, jsonrpc.CodeInvalidParams}, {consolecore.CodeTargetChanged, -32000}, {consolecore.CodeConsoleError, -32000}} {
		domain := consolecore.NewError(test.code, "Safe message.", "scope-1", consolecore.Details{}, errors.New("SECRET cause"))
		err := resourceDomainError(domain)
		rpc, ok := err.(*jsonrpc.Error)
		if !ok || rpc.Code != test.want || rpc.Message != string(test.code)+": Safe message." {
			t.Fatalf("code=%s error=%#v", test.code, err)
		}
		encoded := string(rpc.Data)
		if strings.Contains(encoded, "SECRET") || !strings.Contains(encoded, `"details":{}`) || !strings.Contains(encoded, `"error":{`) {
			t.Fatalf("unsafe data = %s", encoded)
		}
	}
}
