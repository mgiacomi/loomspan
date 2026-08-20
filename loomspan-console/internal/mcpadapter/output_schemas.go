package mcpadapter

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/traceanalysis"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const maxToolsListResponseBytes = 23 << 10

func compactFalseSchema() *jsonschema.Schema { return &jsonschema.Schema{Not: &jsonschema.Schema{}} }

func addValidatedTool[In, Out any](server *mcp.Server, tool *mcp.Tool, compact *jsonschema.Schema, handler mcp.ToolHandlerFor[In, Out]) {
	validate := newCompleteOutputValidator[Out](tool.Name)
	tool.OutputSchema = compact
	mcp.AddTool(server, tool, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, Out, error) {
		result, output, callErr := handler(ctx, request, input)
		if callErr != nil {
			return result, output, callErr
		}
		if err := validate(output); err != nil {
			var zero Out
			return nil, zero, err
		}
		return result, output, nil
	})
}

func newCompleteOutputValidator[Out any](toolName string) func(Out) error {
	full, err := jsonschema.For[Out](nil)
	if err != nil {
		panic(fmt.Sprintf("derive complete output schema for %s: %v", toolName, err))
	}
	resolved, err := full.Resolve(nil)
	if err != nil {
		panic(fmt.Sprintf("resolve complete output schema for %s: %v", toolName, err))
	}
	return func(output Out) error {
		encoded, err := json.Marshal(output)
		if err != nil {
			return fmt.Errorf("INTERNAL: %s result could not be encoded: %w", toolName, err)
		}
		var instance any
		if err := json.Unmarshal(encoded, &instance); err != nil {
			return fmt.Errorf("INTERNAL: %s result could not be validated: %w", toolName, err)
		}
		if toolName == GetTraceToolName {
			if err := validateCompleteRecordCounts(instance); err != nil {
				return fmt.Errorf("INTERNAL: %s result violates its complete contract: %w", toolName, err)
			}
		}
		if err := resolved.Validate(instance); err != nil {
			return fmt.Errorf("INTERNAL: %s result violates its complete contract: %w", toolName, err)
		}
		return nil
	}
}

func validateCompleteRecordCounts(instance any) error {
	envelope, ok := instance.(map[string]any)
	if !ok || envelope["result"] == nil {
		return nil
	}
	result, ok := envelope["result"].(map[string]any)
	if !ok {
		return fmt.Errorf("result is not an object")
	}
	summary, ok := result["summary"].(map[string]any)
	if !ok {
		return fmt.Errorf("summary is not an object")
	}
	counts, ok := summary["recordCountsByType"].(map[string]any)
	if !ok {
		return fmt.Errorf("recordCountsByType is not an object")
	}
	known := make(map[string]struct{}, len(traceanalysis.RecordTypeValues()))
	for _, recordType := range traceanalysis.RecordTypeValues() {
		known[recordType] = struct{}{}
	}
	for recordType, raw := range counts {
		if _, ok := known[recordType]; !ok {
			return fmt.Errorf("recordCountsByType contains unknown key %q", recordType)
		}
		count, ok := raw.(float64)
		if !ok || count < 0 || count != float64(int64(count)) {
			return fmt.Errorf("recordCountsByType[%q] is not a nonnegative integer", recordType)
		}
	}
	return nil
}

func compactString() *jsonschema.Schema  { return &jsonschema.Schema{Type: "string"} }
func compactInteger() *jsonschema.Schema { return &jsonschema.Schema{Type: "integer"} }
func compactBoolean() *jsonschema.Schema { return &jsonschema.Schema{Type: "boolean"} }
func compactOpenObject() *jsonschema.Schema {
	return &jsonschema.Schema{Type: "object", AdditionalProperties: &jsonschema.Schema{}}
}
func compactArray(items *jsonschema.Schema) *jsonschema.Schema {
	return &jsonschema.Schema{Type: "array", Items: items}
}
func compactObject(required []string, properties map[string]*jsonschema.Schema, open bool) *jsonschema.Schema {
	additional := compactFalseSchema()
	if open {
		additional = nil
	}
	return &jsonschema.Schema{Type: "object", Required: required, Properties: properties, AdditionalProperties: additional, PropertyOrder: required}
}

func compactErrorSchema() *jsonschema.Schema {
	return compactObject([]string{"code", "message"}, map[string]*jsonschema.Schema{
		"code": compactString(), "message": compactString(),
	}, true)
}

func compactEnvelopeSchema(result *jsonschema.Schema) *jsonschema.Schema {
	oneMinimum := 1
	one := 1
	return &jsonschema.Schema{
		Type: "object", Properties: map[string]*jsonschema.Schema{"result": result, "error": compactErrorSchema()},
		MinProperties: &oneMinimum, MaxProperties: &one, AdditionalProperties: compactFalseSchema(),
		PropertyOrder: []string{"result", "error"},
	}
}

func compactPageResult(item *jsonschema.Schema, extraRequired ...string) *jsonschema.Schema {
	required := append([]string{}, extraRequired...)
	required = append(required, "items", "hasMore")
	properties := map[string]*jsonschema.Schema{
		"items": compactArray(item), "hasMore": compactBoolean(), "continuation": compactString(),
	}
	for _, name := range extraRequired {
		if name == "observedAt" {
			properties[name] = compactString()
		} else if name == "evidence" {
			properties[name] = compactEvidenceSchema()
		} else {
			properties[name] = compactOpenObject()
		}
	}
	return compactObject(required, properties, true)
}

func runtimeOutputSchema() *jsonschema.Schema {
	status := compactObject([]string{"observedAt", "targetSelection", "targetConnection", "targetAuthentication", "javaGoCompatibility", "runtimeIdentity", "liveMonitoring"}, map[string]*jsonschema.Schema{
		"observedAt": compactString(), "targetSelection": compactString(), "targetConnection": compactString(),
		"targetAuthentication": compactString(), "javaGoCompatibility": compactString(), "runtimeIdentity": compactString(), "liveMonitoring": compactString(),
	}, true)
	return compactObject([]string{"capabilities", "status"}, map[string]*jsonschema.Schema{
		"capabilities": compactArray(compactString()), "status": status,
	}, false)
}

func skillListOutputSchema() *jsonschema.Schema {
	item := compactObject([]string{"registeredName", "sourcePath"}, map[string]*jsonschema.Schema{"registeredName": compactString(), "sourcePath": compactString()}, true)
	return compactEnvelopeSchema(compactPageResult(item, "observedAt"))
}
func skillDetailOutputSchema() *jsonschema.Schema {
	skill := compactObject([]string{"registeredName", "sourcePath", "yaml"}, map[string]*jsonschema.Schema{"registeredName": compactString(), "sourcePath": compactString(), "yaml": compactString()}, true)
	return compactEnvelopeSchema(compactObject([]string{"observedAt", "skill"}, map[string]*jsonschema.Schema{"observedAt": compactString(), "skill": skill}, true))
}
func executionListOutputSchema() *jsonschema.Schema {
	item := compactObject([]string{"sessionId", "traceId", "entrySkill", "status", "phase"}, map[string]*jsonschema.Schema{
		"sessionId": compactString(), "traceId": compactString(), "entrySkill": compactString(), "status": compactString(), "phase": compactString(),
	}, true)
	return compactEnvelopeSchema(compactPageResult(item, "observedAt"))
}
func executionDetailOutputSchema() *jsonschema.Schema {
	execution := compactObject([]string{"sessionId", "traceId", "status", "phase", "activePath", "usage", "configuredLimits"}, map[string]*jsonschema.Schema{
		"sessionId": compactString(), "traceId": compactString(), "status": compactString(), "phase": compactString(),
		"activePath": compactArray(compactOpenObject()), "usage": compactOpenObject(), "configuredLimits": compactOpenObject(),
	}, true)
	return compactEnvelopeSchema(compactObject([]string{"observedAt", "execution"}, map[string]*jsonschema.Schema{"observedAt": compactString(), "execution": execution}, true))
}
func activityOutputSchema() *jsonschema.Schema {
	item := compactObject([]string{"cursor", "sessionId", "traceId", "timestamp", "kind", "summary", "details"}, map[string]*jsonschema.Schema{
		"cursor": compactString(), "sessionId": compactString(), "traceId": compactString(), "timestamp": compactString(), "kind": compactString(), "summary": compactString(), "details": &jsonschema.Schema{},
	}, true)
	result := compactPageResult(item, "observedAt")
	result.Required = append(result.Required, "beginningUnavailable")
	result.Properties["beginningUnavailable"] = compactBoolean()
	return compactEnvelopeSchema(result)
}
func traceListOutputSchema() *jsonschema.Schema {
	item := compactObject([]string{"traceId", "evidenceSources"}, map[string]*jsonschema.Schema{"traceId": compactString(), "evidenceSources": compactArray(compactString())}, true)
	result := compactPageResult(item, "observedAt")
	result.Required = append(result.Required, "complete")
	result.Properties["complete"] = compactBoolean()
	result.Properties["limitations"] = compactArray(compactOpenObject())
	return compactEnvelopeSchema(result)
}
func traceSummaryOutputSchema() *jsonschema.Schema {
	recordCounts := make(map[string]*jsonschema.Schema, len(traceanalysis.RecordTypeValues()))
	for _, recordType := range traceanalysis.RecordTypeValues() {
		zero := float64(0)
		recordCounts[recordType] = &jsonschema.Schema{Type: "integer", Minimum: &zero}
	}
	summary := compactObject([]string{"outcome", "recordCount", "recordCountsByType", "frameCount", "attemptCount", "retryCount", "failureCount", "rootFrameIds", "usageComplete"}, map[string]*jsonschema.Schema{
		"outcome": compactString(), "recordCount": compactInteger(), "frameCount": compactInteger(), "attemptCount": compactInteger(), "retryCount": compactInteger(),
		"recordCountsByType": compactObject(nil, recordCounts, false), "failureCount": compactInteger(), "rootFrameIds": compactArray(compactString()), "usageComplete": compactBoolean(),
	}, true)
	return compactEnvelopeSchema(compactObject([]string{"evidence", "summary"}, map[string]*jsonschema.Schema{"evidence": compactEvidenceSchema(), "summary": summary}, true))
}
func frameQueryOutputSchema() *jsonschema.Schema {
	frame := compactObject([]string{"frameId", "childFrameIds", "frameType", "openedTimestampMillis"}, map[string]*jsonschema.Schema{
		"frameId": compactString(), "childFrameIds": compactArray(compactString()), "frameType": compactString(), "openedTimestampMillis": compactInteger(), "outcome": compactString(),
	}, true)
	result := compactPageResult(frame, "evidence")
	result.Required = append(result.Required, "projection")
	result.Properties["projection"] = compactString()
	return compactEnvelopeSchema(result)
}
func recordQueryOutputSchema() *jsonschema.Schema {
	content := compactObject([]string{"role", "available", "complete"}, map[string]*jsonschema.Schema{
		"role": compactString(), "available": compactBoolean(), "complete": compactBoolean(),
		"contentRef": compactString(), "inlineContent": compactString(), "inlineOmission": compactString(),
	}, true)
	record := compactObject([]string{"sequence", "type", "threadName", "timestampMillis", "representation", "raw", "facts"}, map[string]*jsonschema.Schema{
		"sequence": compactInteger(), "type": compactString(), "failureId": compactString(), "threadName": compactString(), "timestampMillis": compactInteger(), "representation": compactString(), "raw": compactOpenObject(), "facts": compactOpenObject(), "content": content,
	}, true)
	match := compactObject([]string{"sequence", "recordType", "matchOffset", "matchLength", "searchedField"}, map[string]*jsonschema.Schema{
		"sequence": compactInteger(), "recordType": compactString(), "matchOffset": compactInteger(), "matchLength": compactInteger(), "searchedField": compactString(), "contentId": compactString(),
	}, true)
	searchContent := compactObject([]string{"contentId", "contentRef"}, map[string]*jsonschema.Schema{
		"contentId": compactString(), "contentRef": compactString(),
	}, false)
	result := compactObject([]string{"evidence", "hasMore"}, map[string]*jsonschema.Schema{
		"evidence": compactEvidenceSchema(), "items": compactArray(record), "matches": compactArray(match), "search": compactOpenObject(),
		"contentDescriptors": compactArray(searchContent), "hasMore": compactBoolean(), "continuation": compactString(),
	}, false)
	result.OneOf = []*jsonschema.Schema{
		{Required: []string{"items"}, Not: &jsonschema.Schema{AnyOf: []*jsonschema.Schema{{Required: []string{"matches"}}, {Required: []string{"search"}}, {Required: []string{"contentDescriptors"}}}}},
		{Required: []string{"matches", "search", "contentDescriptors"}, Not: &jsonschema.Schema{Required: []string{"items"}}},
	}
	return compactEnvelopeSchema(result)
}
func rangeOutputSchema() *jsonschema.Schema {
	result := compactObject([]string{"evidence", "actualStart", "actualEnd", "totalLength", "contentType", "encoding", "content", "hasMore"}, map[string]*jsonschema.Schema{
		"evidence": compactEvidenceSchema(), "actualStart": compactInteger(), "actualEnd": compactInteger(), "totalLength": compactInteger(),
		"contentType": compactString(), "encoding": compactString(), "content": compactString(), "hasMore": compactBoolean(), "continuation": compactString(),
	}, true)
	return compactEnvelopeSchema(result)
}
func compactEvidenceSchema() *jsonschema.Schema {
	return compactObject([]string{"traceId", "sessionId", "observedAt"}, map[string]*jsonschema.Schema{"traceId": compactString(), "sessionId": compactString(), "observedAt": compactString()}, true)
}
