package com.lokiscale.loomspan.architecture;

import com.tngtech.archunit.core.domain.JavaClass;
import com.tngtech.archunit.core.domain.JavaModifier;
import com.tngtech.archunit.core.importer.ClassFileImporter;
import com.tngtech.archunit.core.importer.ImportOption;
import org.junit.jupiter.api.Test;

import java.lang.annotation.Annotation;
import java.lang.reflect.AnnotatedElement;
import java.lang.reflect.GenericArrayType;
import java.lang.reflect.Modifier;
import java.lang.reflect.ParameterizedType;
import java.lang.reflect.Type;
import java.lang.reflect.TypeVariable;
import java.lang.reflect.WildcardType;
import java.util.Arrays;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.stream.Collectors;
import java.util.stream.Stream;

import static org.assertj.core.api.Assertions.assertThat;

class LoomspanPublicSurfaceArchitectureTest
{
    private static final Set<String> API_TYPES = Set.of(
            "com.lokiscale.loomspan.api.SkillTemplate",
            "com.lokiscale.loomspan.api.SkillExecutionView",
            "com.lokiscale.loomspan.api.SkillExecutionEvent",
            "com.lokiscale.loomspan.api.SkillMethod",
            "com.lokiscale.loomspan.api.SkillParam",
            "com.lokiscale.loomspan.api.SkillException",
            "com.lokiscale.loomspan.api.SkillInputValidationException",
            "com.lokiscale.loomspan.api.SkillInputValidationIssue");

    private static final Set<String> FRAMEWORK_INTEGRATION_TYPES = Set.of(
            "com.lokiscale.loomspan.autoconfigure.LoomspanAutoConfiguration",
            "com.lokiscale.loomspan.autoconfigure.LoomspanAiAutoConfiguration",
            "com.lokiscale.loomspan.autoconfigure.LoomspanObservabilityWebAutoConfiguration",
            "com.lokiscale.loomspan.autoconfigure.LoomspanJacksonAutoConfiguration",
            "com.lokiscale.loomspan.autoconfigure.LoomspanProperties",
            "com.lokiscale.loomspan.autoconfigure.ExecutionTraceProperties",
            "com.lokiscale.loomspan.autoconfigure.AiDriver");

    private static final Map<String, String> TECHNICALLY_PUBLIC_INTERNAL_TYPES = Map.ofEntries(
            Map.entry("com.lokiscale.loomspan.internal.release.LoomspanReleaseVersion", "Public only for framework-owned release metadata collaboration."),
            Map.entry("com.lokiscale.loomspan.internal.observability.ObservabilityActivationCoordinator", "Public only for framework-owned auto-configuration composition."),
            Map.entry("com.lokiscale.loomspan.internal.observability.ObservabilityRuntime", "Public only for framework-owned adapter composition."),
            Map.entry("com.lokiscale.loomspan.internal.observability.web.BoundedJsonPageWriter", "Public only for framework-owned bounded REST serialization."),
            Map.entry("com.lokiscale.loomspan.internal.observability.web.ObservabilityActivityDelivery", "Public only for framework-owned live-delivery runtime composition."),
            Map.entry("com.lokiscale.loomspan.internal.observability.web.ObservabilityArtifactDelivery", "Public only for framework-owned bounded artifact-delivery runtime composition."),
            Map.entry("com.lokiscale.loomspan.internal.observability.web.ObservabilityAccessService", "Public only for framework-owned operator authorization."),
            Map.entry("com.lokiscale.loomspan.internal.observability.web.ObservabilityApiKeyFilter", "Public only for servlet filter registration by auto-configuration."),
            Map.entry("com.lokiscale.loomspan.internal.observability.web.ObservabilityApiPaths", "Public only to keep internal route ownership coherent."),
            Map.entry("com.lokiscale.loomspan.internal.observability.web.ObservabilityCursorCodec", "Public only for framework-owned REST continuation encoding."),
            Map.entry("com.lokiscale.loomspan.internal.observability.web.ObservabilityDtoMapper", "Public only for framework-owned wire projection."),
            Map.entry("com.lokiscale.loomspan.internal.observability.web.ObservabilityJsonCodec", "Public only for framework-owned stable REST and cursor serialization."),
            Map.entry("com.lokiscale.loomspan.internal.observability.web.ObservabilityException", "Public only for internal web problem propagation."),
            Map.entry("com.lokiscale.loomspan.internal.observability.web.ObservabilityProblem", "Public only for the internal serialized problem boundary."),
            Map.entry("com.lokiscale.loomspan.internal.observability.web.ObservabilityProblemMapper", "Public only for framework-owned problem mapping."),
            Map.entry("com.lokiscale.loomspan.internal.observability.web.ObservabilityRestController", "Public only for programmatic Spring MVC handler registration."),
            Map.entry("com.lokiscale.loomspan.internal.observability.web.ObservabilityRouteCollisionDetector", "Public only for framework-owned namespace inspection."),
            Map.entry("com.lokiscale.loomspan.internal.observability.web.ObservabilityRouteRegistrar", "Public only for framework-owned programmatic route lifecycle."),
            Map.entry("com.lokiscale.loomspan.internal.observability.web.dto.ObservabilityDtos", "Public only as internal hand-authored REST boundary DTOs."),
            Map.entry("com.lokiscale.loomspan.internal.autoconfigure.NamedAiConnectionRegistry", "Public only so LoomspanAutoConfiguration can construct this type across the Spring integration boundary."),
            Map.entry("com.lokiscale.loomspan.internal.autoconfigure.SafeAiConnectionConfigurationException", "Public only so the version-scoped Spring AI integration can report sanitized connection configuration failures."),
            Map.entry("com.lokiscale.loomspan.internal.provider.AttemptOwnership", "Public only for typed collaboration between the neutral provider policy and version-scoped Spring AI integration."),
            Map.entry("com.lokiscale.loomspan.internal.provider.ProviderConnectionRuntime", "Public only to carry an internal connection model and its provider retry policy across subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.provider.ProviderFailureCategory", "Public only for typed internal provider failure diagnostics."),
            Map.entry("com.lokiscale.loomspan.internal.provider.ProviderFailureClassification", "Public only for typed internal provider retry classification."),
            Map.entry("com.lokiscale.loomspan.internal.provider.ProviderFailureDetails", "Public only to carry sanitized provider failure details between internal integration, retry, and trace packages."),
            Map.entry("com.lokiscale.loomspan.internal.provider.ProviderFailureTranslator", "Public only for the neutral retry advisor to invoke a version-scoped provider failure translator."),
            Map.entry("com.lokiscale.loomspan.internal.provider.ProviderRetryDecider", "Public only for internal provider retry policy evaluation by the chat advisor."),
            Map.entry("com.lokiscale.loomspan.internal.provider.ProviderRetryDecision", "Public only to carry a typed internal retry decision into tracing and execution."),
            Map.entry("com.lokiscale.loomspan.internal.provider.ProviderRetryOutcome", "Public only for typed internal provider attempt trace outcomes."),
            Map.entry("com.lokiscale.loomspan.internal.provider.ProviderRetryPolicy", "Public only to carry immutable provider retry policy between internal configuration and execution packages."),
            Map.entry("com.lokiscale.loomspan.internal.provider.RetryDelaySource", "Public only for typed internal retry delay trace metadata."),
            Map.entry("com.lokiscale.loomspan.internal.springai.SpringAiProviderIntegration", "Public only as the official Spring AI integration boundary used by internal auto-configuration."),
            Map.entry("com.lokiscale.loomspan.internal.springai.SpringAiChatOptionsContributor", "Public only for framework-owned Spring AI client assembly."),
            Map.entry("com.lokiscale.loomspan.internal.springai.SpringAiModelInteraction", "Public only to adapt Spring AI behind the neutral model boundary."),
            Map.entry("com.lokiscale.loomspan.internal.springai.SpringAiModelInteractionFactory", "Public only for framework-owned Spring AI composition."),
            Map.entry("com.lokiscale.loomspan.internal.model.ModelInteraction", "Public only as the neutral internal model boundary."),
            Map.entry("com.lokiscale.loomspan.internal.model.ModelInteractionFactory", "Public only as the neutral internal model factory boundary."),
            Map.entry("com.lokiscale.loomspan.internal.model.ModelInteractionMode", "Public only to select neutral internal interaction assembly."),
            Map.entry("com.lokiscale.loomspan.internal.model.ModelInteractionRequest", "Public only to carry neutral internal model requests."),
            Map.entry("com.lokiscale.loomspan.internal.model.ModelInteractionResult", "Public only to carry neutral internal model results."),
            Map.entry("com.lokiscale.loomspan.internal.serialization.LoomspanJacksonCodecs", "Public only for framework-owned purpose-specific codec composition."),
            Map.entry("com.lokiscale.loomspan.internal.serialization.LoomspanMethodInputSchemaGenerator", "Public only for framework-owned schema generation across internal composition packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.tool.BoundCapability", "Public only as the neutral bound-capability boundary."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.tool.CapabilityInvoker", "Public only as the neutral capability invocation boundary."),
            Map.entry("com.lokiscale.loomspan.internal.chat.ProviderAttemptCallAdvisor", "Public only for framework-owned physical-attempt advisor assembly."),
            Map.entry("com.lokiscale.loomspan.internal.chat.DefaultSkillAdvisorResolver", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.chat.DefaultSkillChatModelResolver", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.chat.SkillAdvisorResolver", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.chat.SkillChatModelResolver", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.AdvisorTraceContext", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.AdvisorTraceFact", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.AdvisorTraceRecorder", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.LoomspanExceptionTransformer", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.LoomspanSession", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.LoomspanSessionRunner", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.LoomspanStackOverflowException", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.CapabilityExecutionRouter", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.CapabilityInvoker", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.CapabilityKind", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.CapabilityMetadata", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.CapabilityRegistry", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.CapabilityToolDescriptor", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.DefaultLoomspanExceptionTransformer", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.DefaultExecutionTraceRecorder", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.DefaultPlanTaskLinker", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.ExecutionCoordinator", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.ExecutionFrame", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.ExecutionJournal", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.ExecutionPlan", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.ExecutionTrace", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.ExecutionTraceHandle", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.FinalizedTraceArtifact", "Public only for core-issued finalized artifact collaboration with internal observability."),
            Map.entry("com.lokiscale.loomspan.internal.core.ExecutionTraceReader", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.ExecutionTraceRecorder", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.InMemoryCapabilityRegistry", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.InMemorySkillImplementationTargetRegistry", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.JournalEntry", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.JournalEntryType", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.JournalLevel", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.MissionInputMessageFormatter", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.ModelExecutionIdentity", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.ModelTraceContext", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.TraceOutcome", "Public only for typed Java collaboration between distinct internal trace lifecycle packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.OperationType", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.PlanStatus", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.PlanTask", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.PlanTaskLinker", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.PlanTaskStatus", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.PublicSkillImplementationType", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.SessionContextRunner", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.SkillExecutionDescriptor", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.SkillImplementationTarget", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.SkillImplementationTargetRegistry", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.SkillMethodBeanPostProcessor", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.TaskExecutionEvent", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.ToolTraceContext", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.TraceCompletion", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.TraceFailureMetadata", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.TraceFrameType", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.TracePersistencePolicy", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.TraceRecord", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.core.TraceRecordType", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.linter.LinterCallAdvisor", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.linter.LinterOutcome", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.linter.LinterOutcomeRecorder", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.linter.LinterOutcomeStatus", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.outputschema.OutputSchemaCallAdvisor", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.outputschema.OutputSchemaFailureMode", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.outputschema.OutputSchemaOutcome", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.outputschema.OutputSchemaOutcomeRecorder", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.outputschema.OutputSchemaOutcomeStatus", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.outputschema.OutputSchemaPromptAugmentor", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.outputschema.OutputSchemaValidationIssue", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.outputschema.OutputSchemaValidationResult", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.outputschema.OutputSchemaValidator", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.attachment.LoomspanAttachment", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.attachment.DefaultMissionInputMaterializer", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.attachment.MissionInputMaterializer", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.attachment.RenderedMissionInput", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.LoomspanMissionTimeoutException", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.LoomspanQuotaExceededException", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.DefaultMissionExecutionEngine", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.evidence.EvidenceBackedOutputValidator", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.evidence.EvidenceContract", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.evidence.EvidenceExpression", "Public only for immutable expression collaboration between internal catalog and runtime packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.evidence.EvidenceExpressionParser", "Public only for compile-once expression parsing in the internal catalog."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.evidence.EvidenceRequirement", "Public only for structured current-version evidence diagnostics across internal packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.evidence.EvidenceContractCallAdvisor", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.evidence.EvidenceCoverageIssue", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.evidence.EvidenceCoverageResult", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.evidence.EvidenceCoverageValidator", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.input.SkillInputContract", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.input.SkillInputContractResolver", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.input.SkillInputPromptRenderer", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.input.SkillInputSchemaNode", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.input.SkillInputValidationIssue", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.input.SkillInputValidationResult", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.input.SkillInputValidator", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.MissionExecutionEngine", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.planning.DefaultPlanningService", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.planning.PlanningService", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.prompt.SkillPromptComposer", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.prompt.SkillPromptComposition", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.state.DefaultExecutionStateService", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.state.SuccessfulSkillSnapshot", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.state.ExecutionStateService", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.state.PlanSnapshot", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.step.StepLoopMissionExecutionEngine", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.tool.DefaultCapabilityInvoker", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.tool.DefaultToolSurfaceService", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.tool.CapabilityBindingFactory", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.tool.ToolSurfaceService", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.trace.DefaultExecutionTraceHandle", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.trace.CompletionGraceRetention", "Public only for core-owned trace retention composition across internal packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.trace.ConfiguredLimitsSnapshot", "Public only to carry an immutable run-start quota snapshot from internal core wiring into the trace writer."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.trace.ImmediateCompletionRetention", "Public only for framework-owned disabled trace-retention composition."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.trace.ScheduledCompletionGraceRetention", "Public only for framework-owned trace-retention lifecycle composition."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.trace.ExecutionJournalProjector", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.observation.ActiveExecutionSnapshot", "Public only for Java collaboration between internal observation and future application-adapter packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.observation.ActiveExecutionRegistry", "Public only for Java collaboration between internal observation and future application-adapter packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.observation.ActivityReplayBuffer", "Public only for Java collaboration between internal observation and future application-adapter packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.observation.DefaultExecutionObservationHandleFactory", "Public only for framework-owned composition across internal packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.observation.ExecutionActivity", "Public only for Java collaboration between internal observation and future application-adapter packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.observation.ExecutionActivityKind", "Public only for Java collaboration between internal observation and future application-adapter packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.observation.LiveActivitySignal", "Public only for the internal observation-to-delivery notification boundary."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.observation.ExecutionObservationHandle", "Public only for Java collaboration with the internal canonical trace package."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.observation.ExecutionObservationHandleFactory", "Public only for framework-owned session composition across internal packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.observation.ExecutionObservationLimits", "Public only to keep internal projection and future adapter bounds coherent."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.observation.InMemoryActiveExecutionRegistry", "Public only for framework-owned composition across internal packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.observation.InMemoryActivityReplayBuffer", "Public only for framework-owned composition across internal packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.observation.LiveActivityProjector", "Public only for framework-owned composition across internal packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.observation.LiveMonitoringAvailability", "Public only for Java collaboration between internal observation and future application-adapter packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.observation.NoOpExecutionObservationHandle", "Public only for Java collaboration with the internal canonical trace package."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.observation.NoOpExecutionObservationHandleFactory", "Public only for framework-owned disabled observation composition across internal packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.observation.ObservationCompletionDisposition", "Public only for Java collaboration with the internal session finalization authority."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.observation.ReplayResult", "Public only for Java collaboration between internal observation and future application-adapter packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.observation.catalog.DefaultRegisteredSkillCatalog", "Public only for future framework-owned observability adapter composition."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.observation.catalog.FinalizedTraceCatalog", "Public only for internal finalization and future adapter collaboration."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.observation.catalog.FinalizedTraceCatalogEntry", "Public only for internal observation and future adapter collaboration."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.observation.catalog.InMemoryFinalizedTraceCatalog", "Public only for framework-owned observability composition."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.observation.catalog.RegisteredSkillCatalog", "Public only for future internal application-adapter collaboration."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.observation.catalog.RegisteredSkillFile", "Public only for future internal application-adapter collaboration."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.observation.catalog.SkillSourcePathResolver", "Public only for framework-owned registered-skill catalog construction."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.observation.catalog.TraceCatalogSlice", "Public only for future internal keyset adapter collaboration."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.usage.DefaultSessionUsageService", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.usage.GuardrailType", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.usage.MicrometerUsageMetricsRecorder", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.usage.ModelUsageExtractor", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.usage.ModelUsageRecord", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.usage.NoOpSessionUsageService", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.usage.NoOpUsageMetricsRecorder", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.usage.SessionUsageService", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.usage.SessionUsageSnapshot", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.usage.UsageMetricsRecorder", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.runtime.usage.UsagePrecision", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.security.AccessGuard", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.security.DefaultAccessGuard", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.skill.DefaultSkillVisibilityResolver", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.skill.EffectiveSkillExecutionConfiguration", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.skill.SkillVisibilityResolver", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.skill.YamlSkillCapabilityRegistrar", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.skill.YamlSkillCatalog", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.skill.YamlSkillDefinition", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.skill.YamlSkillSource", "Public only for immutable startup-source collaboration with the internal observability catalog."),
            Map.entry("com.lokiscale.loomspan.internal.skill.YamlSkillManifest", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.skillapi.DefaultSkillTemplate", "Public only so LoomspanAutoConfiguration can construct the application facade implementation."),
            Map.entry("com.lokiscale.loomspan.internal.vfs.DefaultRefResolver", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.vfs.RefResolver", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.vfs.SessionLocalVirtualFileSystem", "Public only for Java collaboration between distinct internal subsystem packages."),
            Map.entry("com.lokiscale.loomspan.internal.vfs.VirtualFileSystem", "Public only for Java collaboration between distinct internal subsystem packages."));

    private final Set<JavaClass> productionClasses = new ClassFileImporter()
            .withImportOption(ImportOption.Predefined.DO_NOT_INCLUDE_TESTS)
            .importPackages("com.lokiscale.loomspan")
            .stream()
            .filter(javaClass -> !javaClass.isNestedClass())
            .collect(Collectors.toSet());

    @Test
    void apiPackageContainsExactlyEightApprovedPublicTypes()
    {
        assertThat(publicTopLevelTypesIn("com.lokiscale.loomspan.api"))
                .containsExactlyInAnyOrderElementsOf(API_TYPES);
    }

    @Test
    void autoconfigurePackageContainsExactlySevenIntegrationTypes()
    {
        assertThat(publicTopLevelTypesIn("com.lokiscale.loomspan.autoconfigure"))
                .containsExactlyInAnyOrderElementsOf(FRAMEWORK_INTEGRATION_TYPES);
    }

    @Test
    void everyExternallyAccessibleTopLevelTypeIsClassified()
    {
        Set<String> exposed = productionClasses.stream()
                .filter(this::isPublic)
                .map(JavaClass::getName)
                .filter(name -> !name.startsWith("com.lokiscale.loomspan.internal."))
                .collect(Collectors.toSet());

        assertThat(exposed)
                .as("Every externally accessible top-level type must be in the closed API or framework-integration allowlist")
                .containsExactlyInAnyOrderElementsOf(Stream.concat(
                                API_TYPES.stream(), FRAMEWORK_INTEGRATION_TYPES.stream())
                        .collect(Collectors.toSet()));
    }

    @Test
    void technicallyPublicInternalTypesHaveNonblankReasons()
    {
        Set<String> actual = productionClasses.stream()
                .filter(this::isPublic)
                .map(JavaClass::getName)
                .filter(name -> name.startsWith("com.lokiscale.loomspan.internal."))
                .collect(Collectors.toSet());

        assertThat(actual)
                .as("Every technically public internal type must be deliberately allowlisted")
                .containsExactlyInAnyOrderElementsOf(TECHNICALLY_PUBLIC_INTERNAL_TYPES.keySet());
        assertThat(TECHNICALLY_PUBLIC_INTERNAL_TYPES)
                .allSatisfy((name, reason) -> assertThat(reason)
                        .as("classification reason for %s", name)
                        .isNotBlank());
    }

    @Test
    void noSupportedSpiPackageOrTypeExists()
    {
        assertThat(productionClasses.stream().map(JavaClass::getPackageName))
                .noneMatch(packageName -> packageName.contains(".spi"));
    }

    @Test
    void apiSignaturesRecursivelyExcludeInternalAndAutoconfigureTypes() throws Exception
    {
        for (String typeName : API_TYPES)
        {
            Class<?> apiType = Class.forName(typeName);
            assertAnnotationsAreApiSafe(apiType, apiType.getName());
            assertApiSafe(apiType.getGenericSuperclass(), apiType.getName() + " superclass", new LinkedHashSet<>());
            for (Type interfaceType : apiType.getGenericInterfaces())
            {
                assertApiSafe(interfaceType, apiType.getName() + " interface", new LinkedHashSet<>());
            }

            for (var field : apiType.getDeclaredFields())
            {
                if (Modifier.isPublic(field.getModifiers()) || Modifier.isProtected(field.getModifiers()))
                {
                    assertApiSafe(field.getGenericType(), field.toString(), new LinkedHashSet<>());
                    assertAnnotationsAreApiSafe(field, field.toString());
                }
            }
            for (var constructor : apiType.getDeclaredConstructors())
            {
                if (Modifier.isPublic(constructor.getModifiers()) || Modifier.isProtected(constructor.getModifiers()))
                {
                    assertExecutableIsApiSafe(constructor, constructor.toString());
                }
            }
            for (var method : apiType.getDeclaredMethods())
            {
                if (Modifier.isPublic(method.getModifiers()) || Modifier.isProtected(method.getModifiers()))
                {
                    assertApiSafe(method.getGenericReturnType(), method.toString(), new LinkedHashSet<>());
                    assertExecutableIsApiSafe(method, method.toString());
                }
            }
            if (apiType.isRecord())
            {
                for (var component : apiType.getRecordComponents())
                {
                    assertApiSafe(component.getGenericType(), component.toString(), new LinkedHashSet<>());
                    assertAnnotationsAreApiSafe(component, component.toString());
                }
            }
        }
    }

    @Test
    void observationDtosExposeOnlyBoundedImmutableDomainTypes() throws Exception
    {
        Set<Class<?>> forbidden = Set.of(
                java.nio.file.Path.class,
                org.springframework.core.io.Resource.class,
                tools.jackson.databind.JsonNode.class,
                com.lokiscale.loomspan.internal.core.TraceRecord.class,
                Throwable.class,
                java.util.stream.Stream.class,
                java.util.concurrent.Flow.Publisher.class);

        for (Class<?> dto : List.of(
                com.lokiscale.loomspan.internal.runtime.observation.ActiveExecutionSnapshot.class,
                com.lokiscale.loomspan.internal.runtime.observation.ExecutionActivity.class))
        {
            for (var component : dto.getRecordComponents())
            {
                Class<?> rawType = component.getType();
                assertThat(forbidden)
                        .as("%s component %s", dto.getSimpleName(), component.getName())
                        .noneMatch(type -> type.isAssignableFrom(rawType) || rawType.isAssignableFrom(type));
            }
        }
    }

    @Test
    void observabilityWireDtosDoNotEmbedRuntimeUsageTypes()
    {
        for (Class<?> dto : com.lokiscale.loomspan.internal.observability.web.dto.ObservabilityDtos.class
                .getDeclaredClasses())
        {
            if (!dto.isRecord())
            {
                continue;
            }
            for (var component : dto.getRecordComponents())
            {
                assertThat(component.getType())
                        .as("%s component %s", dto.getSimpleName(), component.getName())
                        .isNotEqualTo(com.lokiscale.loomspan.internal.runtime.usage.SessionUsageSnapshot.class);
            }
        }
    }

    private Set<String> publicTopLevelTypesIn(String packageName)
    {
        return productionClasses.stream()
                .filter(this::isPublic)
                .filter(javaClass -> javaClass.getPackageName().equals(packageName))
                .map(JavaClass::getName)
                .collect(Collectors.toSet());
    }

    private boolean isPublic(JavaClass javaClass)
    {
        return javaClass.getModifiers().contains(JavaModifier.PUBLIC);
    }

    private void assertExecutableIsApiSafe(java.lang.reflect.Executable executable, String owner)
    {
        for (Type parameter : executable.getGenericParameterTypes())
        {
            assertApiSafe(parameter, owner, new LinkedHashSet<>());
        }
        for (Type exception : executable.getGenericExceptionTypes())
        {
            assertApiSafe(exception, owner, new LinkedHashSet<>());
        }
        assertAnnotationsAreApiSafe(executable, owner);
        Arrays.stream(executable.getParameterAnnotations())
                .flatMap(Arrays::stream)
                .map(Annotation::annotationType)
                .forEach(type -> assertClassIsApiSafe(type, owner));
    }

    private void assertApiSafe(Type type, String owner, Set<Type> visited)
    {
        if (type == null || !visited.add(type))
        {
            return;
        }
        if (type instanceof Class<?> clazz)
        {
            assertClassIsApiSafe(clazz, owner);
            if (clazz.isArray())
            {
                assertApiSafe(clazz.getComponentType(), owner, visited);
            }
        }
        else if (type instanceof ParameterizedType parameterized)
        {
            assertApiSafe(parameterized.getRawType(), owner, visited);
            assertApiSafe(parameterized.getOwnerType(), owner, visited);
            for (Type argument : parameterized.getActualTypeArguments())
            {
                assertApiSafe(argument, owner, visited);
            }
        }
        else if (type instanceof GenericArrayType array)
        {
            assertApiSafe(array.getGenericComponentType(), owner, visited);
        }
        else if (type instanceof WildcardType wildcard)
        {
            Stream.concat(Arrays.stream(wildcard.getUpperBounds()), Arrays.stream(wildcard.getLowerBounds()))
                    .forEach(bound -> assertApiSafe(bound, owner, visited));
        }
        else if (type instanceof TypeVariable<?> variable)
        {
            Arrays.stream(variable.getBounds()).forEach(bound -> assertApiSafe(bound, owner, visited));
        }
    }

    private void assertAnnotationsAreApiSafe(AnnotatedElement element, String owner)
    {
        Arrays.stream(element.getAnnotations())
                .map(Annotation::annotationType)
                .forEach(type -> assertClassIsApiSafe(type, owner));
    }

    private void assertClassIsApiSafe(Class<?> type, String owner)
    {
        Class<?> inspected = type.isArray() ? type.getComponentType() : type;
        if (inspected.getName().startsWith("com.lokiscale.loomspan."))
        {
            assertThat(inspected.getPackageName())
                    .as("Public API signature %s leaks Loomspan type %s", owner, inspected.getName())
                    .isEqualTo("com.lokiscale.loomspan.api");
        }
    }
}
