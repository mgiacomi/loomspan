package mcpadapter

import (
	"encoding/json"
	"fmt"
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
	enumProperty(get, "source", "TARGET", "IMPORTED")
	artifactHandleProperty(get, "artifactHandle")
	exactlyOne(get, "traceId", "artifactHandle")
	body, _ := json.Marshal(get)
	if !strings.Contains(string(body), `"oneOf"`) || !strings.Contains(string(body), `"enum":["TARGET","IMPORTED"]`) || !strings.Contains(string(body), `"pattern":"^[0-9a-f]{64}$"`) || !strings.Contains(string(body), `"minLength":64`) || !strings.Contains(string(body), `"maxLength":64`) {
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
