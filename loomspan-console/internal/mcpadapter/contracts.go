package mcpadapter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/live"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/observability"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	maxMCPPageSize         = 64
	defaultMCPListPageSize = 16
)

type toolEnvelope[T any] struct {
	Result *T              `json:"result,omitempty"`
	Error  *domainErrorDTO `json:"error,omitempty"`
}

type domainErrorDTO struct {
	Code    consolecore.Code `json:"code"`
	Message string           `json:"message"`
	Details errorDetailsDTO  `json:"details"`
}

type errorDetailsDTO struct {
	ExpectedCompatibilityVersion string `json:"expectedCompatibilityVersion,omitempty"`
	ObservedCompatibilityVersion string `json:"observedCompatibilityVersion,omitempty"`
	LimitName                    string `json:"limitName,omitempty"`
	LimitValue                   int64  `json:"limitValue,omitempty"`
	RawDownloadAvailable         *bool  `json:"rawDownloadAvailable,omitempty"`
}

type skillSummaryDTO struct {
	RegisteredName string `json:"registeredName"`
	SourcePath     string `json:"sourcePath"`
}

type skillDetailDTO struct {
	RegisteredName string `json:"registeredName"`
	SourcePath     string `json:"sourcePath"`
	YAML           string `json:"yaml"`
}

type skillListResult struct {
	ObservedAt   time.Time         `json:"observedAt"`
	Items        []skillSummaryDTO `json:"items"`
	HasMore      bool              `json:"hasMore"`
	Continuation string            `json:"continuation,omitempty"`
}

type skillDetailResult struct {
	ObservedAt time.Time      `json:"observedAt"`
	Skill      skillDetailDTO `json:"skill"`
}

type framePathDTO struct {
	FrameID   string `json:"frameId"`
	FrameType string `json:"frameType"`
	Route     string `json:"route"`
}

type executionDTO struct {
	SessionID             string                         `json:"sessionId"`
	TraceID               string                         `json:"traceId"`
	LastCanonicalSequence int                            `json:"lastCanonicalSequence"`
	StartedAt             time.Time                      `json:"startedAt"`
	UpdatedAt             time.Time                      `json:"updatedAt"`
	ElapsedMillis         int64                          `json:"elapsedMillis"`
	EntrySkill            string                         `json:"entrySkill"`
	Status                string                         `json:"status"`
	Phase                 string                         `json:"phase"`
	Summary               string                         `json:"summary"`
	ActivePath            []framePathDTO                 `json:"activePath"`
	TotalFrameDepth       int                            `json:"totalFrameDepth"`
	ActivePathTruncated   bool                           `json:"activePathTruncated"`
	Usage                 observability.Usage            `json:"usage"`
	ConfiguredLimits      observability.ConfiguredLimits `json:"configuredLimits"`
}

type executionListResult struct {
	ObservedAt   time.Time      `json:"observedAt"`
	Items        []executionDTO `json:"items"`
	HasMore      bool           `json:"hasMore"`
	Continuation string         `json:"continuation,omitempty"`
}

type executionDetailResult struct {
	ObservedAt time.Time    `json:"observedAt"`
	Execution  executionDTO `json:"execution"`
}

type activityDTO struct {
	Cursor            string            `json:"cursor"`
	SessionID         string            `json:"sessionId"`
	TraceID           string            `json:"traceId"`
	CanonicalSequence *int64            `json:"canonicalSequence,omitempty"`
	Timestamp         time.Time         `json:"timestamp"`
	Kind              live.ActivityKind `json:"kind"`
	ExecutionStatus   string            `json:"executionStatus,omitempty"`
	FrameID           string            `json:"frameId,omitempty"`
	ParentFrameID     string            `json:"parentFrameId,omitempty"`
	FrameType         string            `json:"frameType,omitempty"`
	Route             string            `json:"route,omitempty"`
	Summary           string            `json:"summary"`
	Details           any               `json:"details"`
}

type cursorRangeDTO struct {
	FirstCursor string `json:"firstCursor"`
	LastCursor  string `json:"lastCursor"`
}

type continuityDTO struct {
	IntervalID  string          `json:"intervalId"`
	FirstCursor string          `json:"firstCursor,omitempty"`
	LastCursor  string          `json:"lastCursor,omitempty"`
	ObservedAt  time.Time       `json:"observedAt,omitempty"`
	Reset       *live.ResetFact `json:"reset,omitempty"`
}

type coverageDTO struct {
	GlobalEvictedThroughCursor  string          `json:"globalEvictedThroughCursor,omitempty"`
	SessionStartCursor          string          `json:"sessionStartCursor,omitempty"`
	SessionEvictedThroughCursor string          `json:"sessionEvictedThroughCursor,omitempty"`
	SessionRetainedCursorRange  *cursorRangeDTO `json:"sessionRetainedCursorRange,omitempty"`
}

type activityResult struct {
	ObservedAt          time.Time       `json:"observedAt"`
	Items               []activityDTO   `json:"items"`
	ReturnedCursorRange *cursorRangeDTO `json:"returnedCursorRange,omitempty"`
	HasMore             bool            `json:"hasMore"`
	Continuation        string          `json:"continuation,omitempty"`
	Continuity          *continuityDTO  `json:"continuity,omitempty"`
	Coverage            coverageDTO     `json:"coverage"`
}

var readOnlyAnnotations = func() *mcp.ToolAnnotations {
	falseValue := false
	return &mcp.ToolAnnotations{
		ReadOnlyHint: true, DestructiveHint: &falseValue,
		IdempotentHint: true, OpenWorldHint: &falseValue,
	}
}()

func successResult[T any](value T, fallback string) (*mcp.CallToolResult, toolEnvelope[T], error) {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fallback}}},
		toolEnvelope[T]{Result: &value}, nil
}

func domainFailure[T any](domain *consolecore.Error) (*mcp.CallToolResult, toolEnvelope[T], error) {
	dto := mapDomainError(domain)
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(dto.Code) + ": " + dto.Message}},
		IsError: true,
	}, toolEnvelope[T]{Error: &dto}, nil
}

func mapDomainError(domain *consolecore.Error) domainErrorDTO {
	if domain == nil {
		domain = consolecore.NewError(consolecore.CodeConsoleError, "The Console operation could not be completed.", "", consolecore.Details{}, nil)
	}
	return domainErrorDTO{
		Code: domain.Code, Message: domain.Message, Details: errorDetailsDTO{
			ExpectedCompatibilityVersion: domain.Details.ExpectedCompatibilityVersion,
			ObservedCompatibilityVersion: domain.Details.ObservedCompatibilityVersion,
			LimitName:                    domain.Details.LimitName, LimitValue: domain.Details.LimitValue,
			RawDownloadAvailable: domain.Details.RawDownloadAvailable,
		},
	}
}

type lineWriter struct{ lines []string }

func (writer *lineWriter) quoted(name, value string) {
	encoded, _ := json.Marshal(value) // Encoding a Go string cannot fail.
	writer.lines = append(writer.lines, name+": "+string(encoded))
}

func (writer *lineWriter) integer(name string, value int64) {
	writer.lines = append(writer.lines, fmt.Sprintf("%s: %d", name, value))
}

func (writer *lineWriter) boolean(name string, value bool) {
	writer.lines = append(writer.lines, fmt.Sprintf("%s: %t", name, value))
}

func (writer *lineWriter) time(name string, value time.Time) {
	writer.quoted(name, value.UTC().Format(time.RFC3339Nano))
}

func (writer *lineWriter) continuation(value string) {
	if value == "" {
		writer.lines = append(writer.lines, "continuation: -")
		return
	}
	writer.quoted("continuation", value)
}

func (writer *lineWriter) String() string { return strings.Join(writer.lines, "\n") + "\n" }

func appendCommon(writer *lineWriter, observedAt time.Time) {
	writer.time("observedAt", observedAt)
}

func mapExecution(source observability.ActiveExecution) executionDTO {
	path := make([]framePathDTO, 0, len(source.ActivePath))
	for _, entry := range source.ActivePath {
		path = append(path, framePathDTO{FrameID: entry.FrameID, FrameType: entry.FrameType, Route: entry.Route})
	}
	return executionDTO{
		SessionID: source.SessionID, TraceID: source.TraceID,
		LastCanonicalSequence: source.LastCanonicalSequence,
		StartedAt:             source.StartedAt, UpdatedAt: source.UpdatedAt,
		ElapsedMillis: source.ElapsedMillis, EntrySkill: source.EntrySkill,
		Status: source.Status, Phase: source.Phase, Summary: source.Summary,
		ActivePath: path, TotalFrameDepth: source.TotalFrameDepth,
		ActivePathTruncated: source.ActivePathTruncated,
		Usage:               source.Usage, ConfiguredLimits: source.ConfiguredLimits,
	}
}

func mapActivity(source live.Activity) (activityDTO, error) {
	var details any
	decoder := json.NewDecoder(bytes.NewReader(source.Details))
	decoder.UseNumber()
	if err := decoder.Decode(&details); err != nil {
		return activityDTO{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("activity details contain more than one JSON value")
		}
		return activityDTO{}, err
	}
	return activityDTO{
		Cursor:    source.Cursor,
		SessionID: source.SessionID, TraceID: source.TraceID,
		CanonicalSequence: source.CanonicalSequence, Timestamp: source.Timestamp,
		Kind: source.Kind, ExecutionStatus: source.ExecutionStatus,
		FrameID: source.FrameID, ParentFrameID: source.ParentFrameID,
		FrameType: source.FrameType, Route: source.Route,
		Summary: source.Summary, Details: details,
	}, nil
}

func pageInputSchema[T any]() *jsonschema.Schema {
	schema, err := jsonschema.For[T](nil)
	if err != nil {
		panic(err)
	}
	minimum, maximum := float64(1), float64(maxMCPPageSize)
	schema.Properties["pageSize"].Minimum = &minimum
	schema.Properties["pageSize"].Maximum = &maximum
	if property := schema.Properties["sessionId"]; property != nil {
		minimumLength := 1
		property.MinLength = &minimumLength
		property.Pattern = `.*\S.*`
	}
	return schema
}

func nonblankInputSchema[T any](field string) *jsonschema.Schema {
	schema, err := jsonschema.For[T](nil)
	if err != nil {
		panic(err)
	}
	minimum := 1
	schema.Properties[field].MinLength = &minimum
	schema.Properties[field].Pattern = `.*\S.*`
	return schema
}
