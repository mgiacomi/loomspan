package mcpadapter

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/traceanalysis"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/traceinventory"
)

func TestTraceToolSchemasAreClosedBoundedAndUseSettledBranches(t *testing.T) {
	schemas := []any{traceInputSchema[listTracesInput](), traceInputSchema[getTraceInput](), traceInputSchema[queryTraceFramesInput](), traceInputSchema[queryTraceRecordsInput](), traceInputSchema[traceRangeInput]()}
	for _, schema := range schemas {
		body, _ := json.Marshal(schema)
		if !strings.Contains(string(body), `"additionalProperties":false`) {
			t.Fatalf("schema is not closed: %s", body)
		}
	}
	get := traceInputSchema[getTraceInput]()
	nonblankBoundedString(get, "traceId", maxTraceTokenLength)
	body, _ := json.Marshal(get)
	if strings.Contains(string(body), `"oneOf"`) || !strings.Contains(string(body), `"required":["traceId"]`) || !strings.Contains(string(body), `"minLength":1`) || !strings.Contains(string(body), `"maxLength":8192`) || get.Properties["traceId"].Pattern != `.*\S.*` {
		t.Fatalf("get schema=%s", body)
	}
	rangeSchema := traceInputSchema[traceRangeInput]()
	prepareRangeSchema(rangeSchema, true)
	body, _ = json.Marshal(rangeSchema)
	text := string(body)
	for _, want := range []string{`"maximum":16777216`, `"maxLength":8192`, `"not":{"required":["start","continuation"]}`, `"contentRef"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("range schema missing %s: %s", want, text)
		}
	}
}

func TestClosedTraceVocabulariesUseAuthoritativeServiceInventories(t *testing.T) {
	list := traceInputSchema[listTracesInput]()
	prepareListTracesSchema(list)
	frames := traceInputSchema[queryTraceFramesInput]()
	prepareQueryFramesSchema(frames)
	records := traceInputSchema[queryTraceRecordsInput]()
	prepareQueryRecordsSchema(records)
	assertSchemaEnum := func(name string, schema *jsonschema.Schema, want []string) {
		t.Helper()
		got := make([]string, len(schema.Enum))
		for index, value := range schema.Enum {
			got[index] = value.(string)
		}
		if !slices.Equal(got, want) {
			t.Fatalf("%s enum=%v want=%v", name, got, want)
		}
	}
	assertSchemaEnum("inventory order", list.Properties["order"], traceinventory.OrderValues())
	assertSchemaEnum("inventory sources", list.Properties["sources"].Items, traceinventory.EvidenceSourceValues())
	assertSchemaEnum("inventory outcomes", list.Properties["outcomes"].Items, traceanalysis.TraceOutcomeValues())
	assertSchemaEnum("frame order", frames.Properties["order"], traceanalysis.FrameOrderValues())
	assertSchemaEnum("frame projection", frames.Properties["projection"], traceanalysis.FrameProjectionValues())
	assertSchemaEnum("frame type", frames.Properties["filter"].Properties["frameType"], traceanalysis.FrameTypeValues())
	assertSchemaEnum("frame outcome", frames.Properties["filter"].Properties["outcome"], traceanalysis.FrameOutcomeValues())
	assertSchemaEnum("frame validation", frames.Properties["filter"].Properties["validationStatus"], traceanalysis.ValidationStatusValues())
	assertSchemaEnum("record representation", records.Properties["representation"], traceanalysis.RecordRepresentationValues())
	assertSchemaEnum("record type", records.Properties["filter"].Properties["types"].Items, traceanalysis.RecordTypeValues())
	assertSchemaEnum("record validation", records.Properties["filter"].Properties["validationStatus"], traceanalysis.ValidationStatusValues())
}

func TestTraceToolSchemasExposeOnlyDeveloperIntentIdentity(t *testing.T) {
	rawSchema := traceInputSchema[traceRangeInput]()
	prepareRangeSchema(rawSchema, false)
	tests := []struct {
		name       string
		schema     any
		properties []string
	}{
		{ListTracesToolName, traceInputSchema[listTracesInput](), []string{"acquiredFrom", "acquiredTo", "continuation", "entrySkill", "finalizedFrom", "finalizedTo", "importedFrom", "importedTo", "order", "outcomes", "pageSize", "sessionId", "sources"}},
		{GetTraceToolName, traceInputSchema[getTraceInput](), []string{"traceId"}},
		{QueryTraceFramesToolName, traceInputSchema[queryTraceFramesInput](), []string{"continuation", "filter", "order", "pageSize", "projection", "traceId"}},
		{QueryTraceRecordsToolName, traceInputSchema[queryTraceRecordsInput](), []string{"continuation", "filter", "inlineContent", "pageSize", "representation", "traceId"}},
		{ReadTraceContentToolName, traceInputSchema[traceRangeInput](), []string{"contentRef", "continuation", "maxBytes", "start", "traceId"}},
		{ReadTraceArtifactToolName, rawSchema, []string{"continuation", "maxBytes", "start", "traceId"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body, err := json.Marshal(test.schema)
			if err != nil {
				t.Fatal(err)
			}
			var decoded struct {
				Properties map[string]json.RawMessage `json:"properties"`
			}
			if err := json.Unmarshal(body, &decoded); err != nil {
				t.Fatal(err)
			}
			got := make([]string, 0, len(decoded.Properties))
			for name := range decoded.Properties {
				got = append(got, name)
			}
			slices.Sort(got)
			if !slices.Equal(got, test.properties) {
				t.Fatalf("properties mismatch: got %v want %v", got, test.properties)
			}
		})
	}
}

func TestTraceRangeDescriptionsUseSharedContractBounds(t *testing.T) {
	want := fmt.Sprintf("The default is %d bytes and the maximum is %d source bytes.", defaultTraceRangeBytes, maxTraceRangeBytes)
	if got := traceRangeDescription("prefix"); !strings.Contains(got, want) {
		t.Fatalf("description=%q want fragment=%q", got, want)
	}
}

func TestTraceRangeSchemaAllowsOmittedInitialControlsAndRejectsBoth(t *testing.T) {
	schema := traceInputSchema[traceRangeInput]()
	prepareRangeSchema(schema, true)
	resolved, err := schema.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, instance := range map[string]map[string]any{
		"omitted":       {"traceId": "t", "contentRef": "r"},
		"explicit zero": {"traceId": "t", "contentRef": "r", "start": float64(0)},
		"continuation":  {"traceId": "t", "contentRef": "r", "continuation": "c"},
	} {
		if err := resolved.Validate(instance); err != nil {
			t.Fatalf("%s rejected: %v", name, err)
		}
	}
	if err := resolved.Validate(map[string]any{"traceId": "t", "contentRef": "r", "start": float64(0), "continuation": "c"}); err == nil {
		t.Fatal("both start and continuation were accepted")
	}
}

func TestTraceRangeContentPreservesServiceBase64Exactly(t *testing.T) {
	if got := rangeContent(traceanalysis.ByteRangeResult{Encoding: traceanalysis.RangeEncodingBase64, Content: []byte("AQI=")}); got != "AQI=" {
		t.Fatalf("content=%q", got)
	}
}
