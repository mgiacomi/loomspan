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
	Capabilities []string         `json:"capabilities" jsonschema:"Loomspan capability identifiers supported by this server"`
	Status       runtimeStatusDTO `json:"status" jsonschema:"Current side-effect-free Loomspan Console target status"`
}

type runtimeStatusDTO struct {
	ObservedAt           time.Time                   `json:"observedAt"`
	TargetSelection      consolecore.Selection       `json:"targetSelection"`
	TargetConnection     consolecore.Connection      `json:"targetConnection"`
	TargetAuthentication consolecore.Authentication  `json:"targetAuthentication"`
	JavaGoCompatibility  consolecore.Compatibility   `json:"javaGoCompatibility"`
	RuntimeIdentity      consolecore.RuntimeIdentity `json:"runtimeIdentity"`
	LiveMonitoring       consolecore.LiveMonitoring  `json:"liveMonitoring"`
}

func addRuntimeTool(server *mcp.Server, provider StatusProvider, credentials authenticator, evaluationCapabilities *[]string) {
	addValidatedTool(server, &mcp.Tool{
		Name: RuntimeToolName, Description: "Return current Loomspan Console runtime and target status without contacting the target.",
		Annotations: readOnlyAnnotations,
	}, runtimeOutputSchema(),
		func(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, RuntimeOutput, error) {
			output, err := buildRuntimeOutputWithCapabilities(ctx, provider, credentials, evaluationCapabilities)
			if err != nil {
				return nil, RuntimeOutput{}, err
			}
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: runtimeText(output)}}}, output, nil
		})
}

func buildRuntimeOutput(ctx context.Context, provider StatusProvider, credentials authenticator) (RuntimeOutput, error) {
	return buildRuntimeOutputWithCapabilities(ctx, provider, credentials, nil)
}

func buildRuntimeOutputWithCapabilities(ctx context.Context, provider StatusProvider, credentials authenticator, evaluationCapabilities *[]string) (RuntimeOutput, error) {
	status := provider()
	if err := status.Validate(); err != nil {
		return RuntimeOutput{}, fmt.Errorf("INTERNAL: runtime status is unavailable")
	}
	if generation, ok := admittedGeneration(ctx); ok && credentials.Snapshot().Generation != generation {
		return RuntimeOutput{}, fmt.Errorf("INTERNAL: MCP authentication generation changed")
	}
	capabilities := installedCapabilities()
	if evaluationCapabilities != nil {
		capabilities = append([]string{}, (*evaluationCapabilities)...)
	}
	mapped := runtimeStatusDTO{ObservedAt: status.ObservedAt, TargetSelection: status.TargetSelection, TargetConnection: status.TargetConnection, TargetAuthentication: status.TargetAuthentication, JavaGoCompatibility: status.JavaGoCompatibility, RuntimeIdentity: status.RuntimeIdentity, LiveMonitoring: status.LiveMonitoring}
	return RuntimeOutput{Capabilities: capabilities, Status: mapped}, nil
}

func runtimeText(output RuntimeOutput) string {
	status := output.Status
	lines := make([]string, 0, len(output.Capabilities)+7)
	for _, capability := range output.Capabilities {
		lines = append(lines, "capability: "+capability)
	}
	lines = append(lines,
		"targetSelection: "+string(status.TargetSelection),
		"targetConnection: "+string(status.TargetConnection),
		"targetAuthentication: "+string(status.TargetAuthentication),
		"javaGoCompatibility: "+string(status.JavaGoCompatibility),
		"runtimeIdentity: "+string(status.RuntimeIdentity),
		"liveMonitoring: "+string(status.LiveMonitoring),
		"observedAt: "+status.ObservedAt.UTC().Format(time.RFC3339Nano),
	)
	return strings.Join(lines, "\n")
}
