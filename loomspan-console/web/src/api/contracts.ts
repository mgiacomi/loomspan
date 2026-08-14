export type BrowserErrorCode =
  | "INVALID_REQUEST"
  | "BROWSER_SECURITY_REJECTED"
  | "PAIRING_REJECTED"
  | "SESSION_REQUIRED"
  | "LIMIT_EXCEEDED"
  | "RATE_LIMITED"
  | "PAIRING_UNAVAILABLE"
  | "METHOD_NOT_ALLOWED"
  | "NOT_FOUND"
  | "INVALID_ARGUMENT"
  | "TARGET_AUTHENTICATION_REQUIRED"
  | "TARGET_ACCESS_BLOCKED"
  | "TARGET_UNAVAILABLE"
  | "INCOMPATIBLE_TARGET"
	| "INCOMPATIBLE_ARTIFACT"
  | "TARGET_CHANGED"
  | "INVALID_CURSOR"
  | "STALE_CURSOR"
  | "ARTIFACT_EXPIRED"
  | "ARTIFACT_IN_USE"
	| "ARTIFACT_ALREADY_EXISTS"
  | "INVALID_ARTIFACT"
  | "LIVE_MONITORING_UNAVAILABLE"
  | "LOCAL_STORAGE_UNAVAILABLE"
	| "CONFIRMATION_REQUIRED"
	| "MCP_STATE_CHANGED"
  | "CONSOLE_ERROR";

export type MCPState = "DISABLED" | "ENABLED" | "DISABLED_INVALID";

export type MCPSetup = {
  client: string;
  scope: string;
  guidance: string;
};

export type MCPStatus = {
  endpoint: string;
  state: MCPState;
  diagnostic?: string;
  setup: MCPSetup[];
};

export type MCPCredentialResponse = MCPStatus & { credential: string };

export type TargetStatus = {
  observedAt: string;
  targetScopeId?: string;
  targetSelection: "NONE" | "SELECTED";
  targetConnection: "NOT_APPLICABLE" | "UNKNOWN" | "REACHABLE" | "UNAVAILABLE";
  targetAuthentication:
    | "NOT_APPLICABLE"
    | "UNKNOWN"
    | "REQUIRED"
    | "ESTABLISHED"
    | "BLOCKED";
  javaGoCompatibility:
    | "NOT_APPLICABLE"
    | "NOT_CHECKED"
    | "COMPATIBLE"
    | "INCOMPATIBLE";
  runtimeIdentity: "NOT_APPLICABLE" | "NOT_ESTABLISHED" | "ESTABLISHED";
  instanceId?: string;
  liveMonitoring: "NOT_APPLICABLE" | "UNKNOWN" | "AVAILABLE" | "UNAVAILABLE";
};

export type TargetResponse = {
  address?: string;
  unencrypted: boolean;
  status: TargetStatus;
};

export type BrowserErrorDetails = {
  expectedCompatibilityVersion?: string;
  observedCompatibilityVersion?: string;
  currentTargetScopeId?: string;
  transportCategory?: string;
  limitName?: string;
  limitValue?: number;
  rawDownloadAvailable?: boolean;
};

export type ErrorEnvelope = {
  error: {
    code: BrowserErrorCode;
    message: string;
    targetScopeId?: string;
    details?: BrowserErrorDetails;
  };
};

export type BootstrapResponse = {
  processId: string;
  consoleVersion: string;
  workspacePath: string;
  tabId: string;
  csrfToken: string;
  target: TargetResponse;
  targetFormDefaults: {
    address: string;
    applicationKey: string;
  };
};

export type PairingLinkResponse = {
  pairingUrl: string;
};

export type InstanceStatus = {
  targetScopeId: string;
  instanceId: string;
  consoleCompatibilityVersion: string;
  observedAt: string;
  liveMonitoringAvailable: boolean;
  registeredSkillCount: number;
  activeExecutionCount: number;
  catalogedTraceCount: number;
  tracePersistencePolicy: string;
  completionGraceTtl: string;
  traceCatalogMetadataTtl: string;
};

export type SkillSummary = {
  registeredName: string;
  sourcePath: string;
  href?: string;
};

export type SkillDetail = {
  targetScopeId: string;
  registeredName: string;
  sourcePath: string;
  yaml: string;
};

export type FramePathEntry = {
  frameId: string;
  frameType: string;
  route: string;
};

export type Usage = {
  skillInvocations: number;
  toolInvocations: number;
  linterRetries: number;
  modelCalls: number;
  providerAttempts: number;
  promptUnits: number;
  completionUnits: number;
  usageUnits: number;
  exactModelResponses: number;
  heuristicModelResponses: number;
  unavailableModelResponses: number;
};

export type ConfiguredLimits = {
  maxSkillInvocations: number;
  maxToolInvocations: number;
  maxLinterRetries: number;
  maxModelCalls: number;
  maxProviderAttempts: number;
  maxUsageUnits: number;
};

export type ActiveExecution = {
  targetScopeId: string;
  sessionId: string;
  traceId: string;
  lastCanonicalSequence: number;
  startedAt: string;
  updatedAt: string;
  elapsedMillis: number;
  entrySkill: string;
  status: string;
  phase: string;
  summary: string;
  activePath: FramePathEntry[];
  totalFrameDepth: number;
  activePathTruncated: boolean;
  usage: Usage;
  configuredLimits: ConfiguredLimits;
};

export type Trace = {
  targetScopeId: string;
  traceId: string;
  sessionId: string;
  entrySkill: string;
  outcome: string;
  finalizedAt: string;
  sizeBytes: number;
  persistencePolicy: string;
  applicationTraceExpiresAt: string;
  localAvailable: boolean;
  artifactHandle?: string;
  applicationAvailability?: string;
};

export type TraceSource = "TARGET" | "IMPORTED";
export type EvidenceEnvelope = { source: TraceSource; targetScopeId?: string };

export type AcquiredArtifact = {
	 source: TraceSource;
	 targetScopeId?: string;
  artifactHandle: string;
  traceId: string;
  sessionId: string;
  outcome: string;
  finalizedAt: string;
  localBytes: number;
  acquiredAt: string;
  lastUsedAt: string;
  expiresAt: string;
  hasIdleExpiry: boolean;
};

export type StoredEntry = {
	 source: TraceSource;
	 targetScopeId?: string;
  traceId: string;
  sessionId: string;
  outcome: string;
  persistencePolicy: string;
  finalizedAt: string;
  acquiredAt: string;
  lastUsedAt: string;
  expiresAt: string;
  hasIdleExpiry: boolean;
  localBytes: number;
  applicationTraceExpiresAt?: string;
  applicationAvailability?: string;
  localAvailable: boolean;
  activePin: boolean;
};

export type StorageSnapshot = {
  workspaceLabel: string;
  maxBytes: number;
  unlimited: boolean;
  idleTtl: string;
  neverExpire: boolean;
  chargedBytes: number;
  acquiredCount: number;
  entries: StoredEntry[];
};

export type Page<T> = {
  targetScopeId: string;
  items: T[];
  hasMore: boolean;
  nextCursor: string | null;
  observedAt: string;
};

export type TraceAnalysisSummary = EvidenceEnvelope & {
  traceId: string; sessionId: string; outcome: string;
  terminalFailureId: string | null; recordCount: number; frameCount: number;
  attemptCount: number; retryCount: number; validationCount: number;
  failureCount: number; payloadCount: number; gapCount: number;
  uncertaintyCount: number; rootFrameIds: string[];
  attributedUsage: TraceUsageValue; terminalUsage: TraceUsageValue;
  unattributedUsage: TraceUsageValue; unframedAttributedUsage: TraceUsageValue;
  usageComplete: boolean;
  configuredLimits: ConfiguredLimits | null;
};
export type TraceFrame = {
  frameId: string; parentFrameId: string | null; childFrameIds: string[];
  frameType: string; route: string; openedTimestampMillis: number;
  closedTimestampMillis: number | null; inclusiveDurationMillis: number | null;
  selfDurationMillis: number | null; directUsage: TraceUsageValue;
  directUsageComplete: boolean; descendantUsage: TraceUsageValue;
  descendantUsageComplete: boolean; inclusiveUsage: TraceUsageValue;
  inclusiveUsageComplete: boolean;
  skillNames: string[]; outcomes: string[]; attemptIds: string[];
  retrySequenceIds: string[]; validationStatuses: string[]; failureIds: string[];
};
export type TraceRecord = {
  sequence: number; type: string; frameId: string; parentFrameId: string;
  frameType: string; route: string; threadName: string; timestampMillis: number;
  representation: string; isChunk: boolean; isEnvelope: boolean; payloadId: string;
};
export type TraceAnalysisPage<T> = EvidenceEnvelope & { items: T[]; hasMore: boolean; nextCursor: string | null };
export type TraceRange = EvidenceEnvelope & { actualStart: number; actualEnd: number; totalLength: number; contentType: string; encoding: "TEXT" | "BASE64"; content: string; hasMore: boolean; nextCursor: string | null };
export type TraceUsage = EvidenceEnvelope & { attributed: TraceUsageValue; unattributed: TraceUsageValue; unframedAttributed: TraceUsageValue; terminal: TraceUsageValue };
export type TraceUsageValue = { promptUnits: number; completionUnits: number; totalUnits: number };
export type TraceAttempt = { retrySequenceId: string; attemptId: string; attemptNumber: number;
  attemptReason: "INITIAL" | "SEMANTIC_RETRY" | "PROVIDER_RETRY"; providerAttemptNumber: number;
  outcome: "SUCCEEDED" | "FAILED" | "INCOMPLETE"; failureClassification?: string;
  failureCategory?: string; retryDecision?: string; retryDelayMillis: number;
  retryDelaySource?: string; httpStatus?: number; providerErrorType?: string;
  providerErrorCode?: string; payloadId?: string; usage: TraceUsageValue; usageComplete: boolean };
export type TraceRetry = { retrySequenceId: string; usage: TraceUsageValue; usageComplete: boolean };
export type TraceFailure = { failureId: string; terminal: boolean; sequence: number; timestampMillis: number;
  recordType: string; frameId: string; route: string; attemptId: string;
  retrySequenceId: string; validationStatus: string; exceptionType?: string; contextSummary?: string; diagnostics?: TraceDiagnosticDescriptor[] };
export type TraceDiagnosticDescriptor = { ordinal: number; kind: string; contentType: string; truncated: boolean; captureLimitBytes: number; decodedBytes: number };
export type TraceFailureDiagnostic = EvidenceEnvelope & { failureId: string; descriptor: TraceDiagnosticDescriptor; text: string };
export type TraceValidation = { status: string; retrySequenceId: string; attemptId: string; attemptNumber: number };
export type TraceGap = { kind: string; frameId: string; attemptId: string };
export type TraceUncertainty = { kind: string; frameId: string };
export type TracePayload = { payloadId: string; sequence: number; contentType: string; chunkCount: number; storeLength: number };
export type TraceSearchResult = { sequence: number; recordType: string; frameId: string; matchOffset: number; matchLength: number; searchedField: string };

export type ActivePage = Page<ActiveExecution> & {
  resumeCursor: string | null;
};

export type ActivityKind =
  | "TRACE_STARTED"
  | "FRAME_OPENED"
  | "FRAME_CLOSED"
  | "MODEL_REQUEST_SENT"
  | "MODEL_RESPONSE_RECEIVED"
  | "MODEL_ATTEMPT_FAILED"
  | "PLAN_CREATED"
  | "PLAN_UPDATED"
  | "PLAN_VALIDATION_FAILED"
  | "PLAN_RETRY_REQUESTED"
  | "TOOL_CALL_STARTED"
  | "TOOL_CALL_COMPLETED"
  | "TOOL_CALL_FAILED"
  | "STEP_STARTED"
  | "STEP_ACTION_REJECTED"
  | "STEP_COMPLETED"
  | "ERROR_RECORDED"
  | "TRACE_COMPLETED"
  | "EXECUTION_OBSERVATION_ENDED";

export const ACTIVITY_KIND_LABELS: Record<ActivityKind, string> = {
  TRACE_STARTED: "Execution started",
  FRAME_OPENED: "Skill execution started",
  FRAME_CLOSED: "Skill execution completed",
  MODEL_REQUEST_SENT: "Model request sent",
  MODEL_RESPONSE_RECEIVED: "Model response received",
  MODEL_ATTEMPT_FAILED: "Provider attempt failed",
  PLAN_CREATED: "Plan created",
  PLAN_UPDATED: "Plan updated",
  PLAN_VALIDATION_FAILED: "Plan validation failed",
  PLAN_RETRY_REQUESTED: "Plan retry requested",
  TOOL_CALL_STARTED: "Tool call started",
  TOOL_CALL_COMPLETED: "Tool call completed",
  TOOL_CALL_FAILED: "Tool call failed",
  STEP_STARTED: "Step started",
  STEP_ACTION_REJECTED: "Step action rejected",
  STEP_COMPLETED: "Step completed",
  ERROR_RECORDED: "Execution error recorded",
  TRACE_COMPLETED: "Execution completed",
  EXECUTION_OBSERVATION_ENDED: "Execution observation ended",
};

export type ResetCause =
  | "target_scope_changed"
  | "instance_changed"
  | "upstream_stale_cursor"
  | "shutdown";

export type ResetFact = {
  cause: ResetCause;
  timestamp: string;
  cursor?: string;
};

export type Continuity = {
  intervalId: string;
  targetScopeId: string;
  instanceId: string;
  firstCursor?: string;
  lastCursor?: string;
  observedAt?: string;
  reset?: ResetFact;
};

export type ConnectionFact = {
  connected: boolean;
  reason?: string;
  at?: string;
};

export type Activity = {
  instanceId: string;
  cursor: string;
  sessionId: string;
  traceId: string;
  canonicalSequence?: number;
  timestamp: string;
  kind: ActivityKind;
  executionStatus?: string;
  frameId?: string;
  parentFrameId?: string;
  frameType?: string;
  route?: string;
  summary: string;
  details: Record<string, unknown>;
};

export type RecentActivityRequest = {
  cursor?: string;
  sessionId?: string;
  limit?: number;
};

export type RecentActivityResponse = {
  items: Activity[];
  hasMore: boolean;
  nextCursor: string;
  continuity?: Continuity;
  beginningUnavailable: boolean;
};
