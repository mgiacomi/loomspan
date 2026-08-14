package mcpadapter

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const RuntimeToolName = "LOOMSPAN_get_runtime"
const RuntimeStatusCapability = "loomspan.runtime-status.v1"

type StatusProvider func() consolecore.StatusSnapshot

type emptyInput struct{}

type RuntimeOutput struct {
	Capabilities []string                   `json:"capabilities" jsonschema:"Loomspan capability identifiers supported by this server"`
	Status       consolecore.StatusSnapshot `json:"status" jsonschema:"Current side-effect-free Loomspan Console target status"`
}

func addRuntimeTool(server *mcp.Server, provider StatusProvider, credentials authenticator) {
	mcp.AddTool(server, &mcp.Tool{
		Name: RuntimeToolName, Description: "Return current Loomspan Console runtime and target status without contacting the target.",
		Annotations: readOnlyAnnotations,
	},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, RuntimeOutput, error) {
			output, err := buildRuntimeOutput(ctx, provider, credentials)
			if err != nil {
				return nil, RuntimeOutput{}, err
			}
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: runtimeText(output)}}}, output, nil
		})
}

func buildRuntimeOutput(ctx context.Context, provider StatusProvider, credentials authenticator) (RuntimeOutput, error) {
	status := provider()
	if err := status.Validate(); err != nil {
		return RuntimeOutput{}, fmt.Errorf("INTERNAL: runtime status is unavailable")
	}
	if generation, ok := admittedGeneration(ctx); ok && credentials.Snapshot().Generation != generation {
		return RuntimeOutput{}, fmt.Errorf("INTERNAL: MCP authentication generation changed")
	}
	return RuntimeOutput{Capabilities: installedCapabilities(), Status: status}, nil
}

func runtimeText(output RuntimeOutput) string {
	status := output.Status
	value := func(value string) string {
		if value == "" {
			return "-"
		}
		return value
	}
	lines := make([]string, 0, len(output.Capabilities)+9)
	for _, capability := range output.Capabilities {
		lines = append(lines, "capability: "+capability)
	}
	lines = append(lines,
		"targetScopeId: "+value(status.TargetScopeID),
		"targetSelection: "+string(status.TargetSelection),
		"targetConnection: "+string(status.TargetConnection),
		"targetAuthentication: "+string(status.TargetAuthentication),
		"javaGoCompatibility: "+string(status.JavaGoCompatibility),
		"runtimeIdentity: "+string(status.RuntimeIdentity),
		"instanceId: "+value(status.InstanceID),
		"liveMonitoring: "+string(status.LiveMonitoring),
		"observedAt: "+status.ObservedAt.UTC().Format(time.RFC3339Nano),
	)
	return strings.Join(lines, "\n")
}
