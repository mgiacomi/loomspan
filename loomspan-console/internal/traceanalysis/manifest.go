package traceanalysis

import (
	"context"
	"encoding/json"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
)

// ComponentManifest is the derived component name for the analysis manifest.
const ComponentManifest artifact.ComponentName = "manifest.json"

// manifestSchemaV1 is the current manifest schema identifier. The manifest is an
// internal same-process format, not versioned persisted state; the schema tag
// lets future phases evolve it without a durable compatibility reader.
const manifestSchemaV1 = "loomspan-trace-analysis-manifest-v1"

// manifest is the Phase 3 manifest body. It records trace identity, counts,
// root references, terminal facts, component sizes, and index offsets. It is
// written after every index is synced so a partial manifest is never published.
type manifest struct {
	Schema             string                           `json:"schema"`
	TraceID            string                           `json:"traceId"`
	SessionID          string                           `json:"sessionId"`
	Outcome            string                           `json:"outcome"`
	TerminalFailureID  *string                          `json:"terminalFailureId"`
	ConfiguredLimits   *ConfiguredLimits                `json:"configuredLimits"`
	RecordCount        int64                            `json:"recordCount"`
	RecordCountsByType map[TraceRecordType]int64        `json:"recordCountsByType"`
	FrameCount         int                              `json:"frameCount"`
	AttemptCount       int                              `json:"attemptCount"`
	RetryCount         int                              `json:"retryCount"`
	ValidationCount    int                              `json:"validationCount"`
	FailureCount       int                              `json:"failureCount"`
	PayloadCount       int                              `json:"payloadCount"`
	GapCount           int                              `json:"gapCount"`
	UncertaintyCount   int                              `json:"uncertaintyCount"`
	RootFrameIDs       []string                         `json:"rootFrameIds,omitempty"`
	UsageComplete      bool                             `json:"usageComplete"`
	ComponentSizes     map[artifact.ComponentName]int64 `json:"componentSizes"`
}

// writeManifest writes the manifest component and records its size.
func writeManifest(ctx context.Context, sink artifact.ComponentSink, scopeID string, m manifest) (map[artifact.ComponentName]int64, *consolecore.Error) {
	body, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, consolecore.NewError(consolecore.CodeConsoleError,
			"The trace analysis manifest could not be encoded.", scopeID, consolecore.Details{}, err)
	}
	body = append(body, '\n')
	if e := ctx.Err(); e != nil {
		return nil, canceledError(e)
	}
	writer, domain := sink.Create(ctx, ComponentManifest)
	if domain != nil {
		return nil, domain
	}
	if _, err := writer.Write(body); err != nil {
		_ = writer.Close()
		return nil, consolecore.NewError(consolecore.CodeLocalStorageUnavailable,
			"Local artifact storage is unavailable.", scopeID, consolecore.Details{}, err)
	}
	if err := writer.Sync(); err != nil {
		_ = writer.Close()
		return nil, consolecore.NewError(consolecore.CodeLocalStorageUnavailable,
			"Local artifact storage is unavailable.", scopeID, consolecore.Details{}, err)
	}
	if err := writer.Close(); err != nil {
		return nil, consolecore.NewError(consolecore.CodeLocalStorageUnavailable,
			"Local artifact storage is unavailable.", scopeID, consolecore.Details{}, err)
	}
	m.ComponentSizes[ComponentManifest] = int64(len(body))
	return m.ComponentSizes, nil
}
