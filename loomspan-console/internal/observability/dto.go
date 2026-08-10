package observability

import "time"

type InstanceStatus struct {
	TargetScopeID               string    `json:"targetScopeId"`
	InstanceID                  string    `json:"instanceId"`
	ConsoleCompatibilityVersion string    `json:"consoleCompatibilityVersion"`
	ObservedAt                  time.Time `json:"observedAt"`
	LiveMonitoringAvailable     bool      `json:"liveMonitoringAvailable"`
	RegisteredSkillCount        int       `json:"registeredSkillCount"`
	ActiveExecutionCount        int       `json:"activeExecutionCount"`
	CatalogedTraceCount         int       `json:"catalogedTraceCount"`
	TracePersistencePolicy      string    `json:"tracePersistencePolicy"`
	CompletionGraceTtl          string    `json:"completionGraceTtl"`
	TraceCatalogMetadataTtl     string    `json:"traceCatalogMetadataTtl"`
}

type SkillSummary struct {
	RegisteredName string `json:"registeredName"`
	SourcePath     string `json:"sourcePath"`
	Href           string `json:"href,omitempty"`
}

type SkillDetail struct {
	TargetScopeID  string `json:"targetScopeId"`
	RegisteredName string `json:"registeredName"`
	SourcePath     string `json:"sourcePath"`
	Yaml           string `json:"yaml"`
}

type FramePathEntry struct {
	FrameID   string `json:"frameId"`
	FrameType string `json:"frameType"`
	Route     string `json:"route"`
}

type Usage struct {
	SkillInvocations          int `json:"skillInvocations"`
	ToolInvocations           int `json:"toolInvocations"`
	LinterRetries             int `json:"linterRetries"`
	ModelCalls                int `json:"modelCalls"`
	ProviderAttempts          int `json:"providerAttempts"`
	PromptUnits               int `json:"promptUnits"`
	CompletionUnits           int `json:"completionUnits"`
	UsageUnits                int `json:"usageUnits"`
	ExactModelResponses       int `json:"exactModelResponses"`
	HeuristicModelResponses   int `json:"heuristicModelResponses"`
	UnavailableModelResponses int `json:"unavailableModelResponses"`
}

type ConfiguredLimits struct {
	MaxSkillInvocations int `json:"maxSkillInvocations"`
	MaxToolInvocations  int `json:"maxToolInvocations"`
	MaxLinterRetries    int `json:"maxLinterRetries"`
	MaxModelCalls       int `json:"maxModelCalls"`
	MaxProviderAttempts int `json:"maxProviderAttempts"`
	MaxUsageUnits       int `json:"maxUsageUnits"`
}

type ActiveExecution struct {
	TargetScopeID         string           `json:"targetScopeId"`
	SessionID             string           `json:"sessionId"`
	TraceID               string           `json:"traceId"`
	LastCanonicalSequence int              `json:"lastCanonicalSequence"`
	StartedAt             time.Time        `json:"startedAt"`
	UpdatedAt             time.Time        `json:"updatedAt"`
	ElapsedMillis         int64            `json:"elapsedMillis"`
	EntrySkill            string           `json:"entrySkill"`
	Status                string           `json:"status"`
	Phase                 string           `json:"phase"`
	Summary               string           `json:"summary"`
	ActivePath            []FramePathEntry `json:"activePath"`
	TotalFrameDepth       int              `json:"totalFrameDepth"`
	ActivePathTruncated   bool             `json:"activePathTruncated"`
	Usage                 Usage            `json:"usage"`
	ConfiguredLimits      ConfiguredLimits `json:"configuredLimits"`
}

type Trace struct {
	TargetScopeID             string    `json:"targetScopeId"`
	TraceID                   string    `json:"traceId"`
	SessionID                 string    `json:"sessionId"`
	EntrySkill                string    `json:"entrySkill"`
	Outcome                   string    `json:"outcome"`
	FinalizedAt               time.Time `json:"finalizedAt"`
	SizeBytes                 int64     `json:"sizeBytes"`
	PersistencePolicy         string    `json:"persistencePolicy"`
	ApplicationTraceExpiresAt time.Time `json:"applicationTraceExpiresAt"`
	LocalAvailable            bool      `json:"localAvailable"`
	ArtifactHandle            string    `json:"artifactHandle,omitempty"`
	ApplicationAvailability   string    `json:"applicationAvailability,omitempty"`
}

type Page[T any] struct {
	TargetScopeID string    `json:"targetScopeId"`
	Items         []T       `json:"items"`
	HasMore       bool      `json:"hasMore"`
	NextCursor    *string   `json:"nextCursor"`
	ObservedAt    time.Time `json:"observedAt"`
}

type ActivePage struct {
	Page[ActiveExecution]
	ResumeCursor *string `json:"resumeCursor"`
}

type ListRequest struct {
	Cursor   string
	PageSize int
}
