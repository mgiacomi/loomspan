package mcpadapter

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/traceanalysis"
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
	for _, want := range []string{`"maximum":16777216`, `"maxLength":8192`, `"oneOf"`, `"payloadRef"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("range schema missing %s: %s", want, text)
		}
	}
}

func TestTraceToolSchemasExposeOnlyDeveloperIntentIdentity(t *testing.T) {
	rawSchema := traceInputSchema[traceRangeInput]()
	prepareRangeSchema(rawSchema, false)
	tests := []struct {
		name       string
		schema     any
		properties []string
	}{
		{ListTracesToolName, traceInputSchema[listTracesInput](), []string{"continuation", "pageSize"}},
		{GetTraceToolName, traceInputSchema[getTraceInput](), []string{"traceId"}},
		{QueryTraceFramesToolName, traceInputSchema[queryTraceFramesInput](), []string{"continuation", "filter", "order", "pageSize", "traceId"}},
		{QueryTraceRecordsToolName, traceInputSchema[queryTraceRecordsInput](), []string{"continuation", "filter", "inlinePayload", "pageSize", "representation", "traceId"}},
		{ReadTracePayloadToolName, traceInputSchema[traceRangeInput](), []string{"continuation", "maxBytes", "payloadRef", "start", "traceId"}},
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

func TestTraceRangeContentPreservesServiceBase64Exactly(t *testing.T) {
	if got := rangeContent(traceanalysis.ByteRangeResult{Encoding: traceanalysis.RangeEncodingBase64, Content: []byte("AQI=")}); got != "AQI=" {
		t.Fatalf("content=%q", got)
	}
}
