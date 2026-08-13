package com.lokiscale.loomspan.internal.skillapi;

import tools.jackson.databind.ObjectMapper;
import com.lokiscale.loomspan.api.SkillException;
import com.lokiscale.loomspan.api.SkillExecutionView;
import com.lokiscale.loomspan.api.SkillInputValidationException;
import com.lokiscale.loomspan.internal.core.LoomspanSessionRunner;
import com.lokiscale.loomspan.internal.core.CapabilityExecutionRouter;
import com.lokiscale.loomspan.internal.core.CapabilityInvoker;
import com.lokiscale.loomspan.internal.core.CapabilityKind;
import com.lokiscale.loomspan.internal.core.CapabilityMetadata;
import com.lokiscale.loomspan.internal.core.CapabilityRegistry;
import com.lokiscale.loomspan.internal.core.CapabilityToolDescriptor;
import com.lokiscale.loomspan.internal.core.InMemoryCapabilityRegistry;
import com.lokiscale.loomspan.internal.core.SkillExecutionDescriptor;
import com.lokiscale.loomspan.internal.runtime.input.SkillInputContract;
import com.lokiscale.loomspan.internal.runtime.input.SkillInputSchemaNode;
import com.lokiscale.loomspan.internal.runtime.input.SkillInputValidator;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.AfterEach;
import org.springframework.security.authentication.UsernamePasswordAuthenticationToken;
import org.springframework.security.access.AccessDeniedException;
import org.springframework.security.core.authority.SimpleGrantedAuthority;
import org.springframework.security.core.context.SecurityContextHolder;

import java.time.Clock;
import java.time.Instant;
import java.time.ZoneOffset;
import java.util.List;
import java.util.Map;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicReference;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

class DefaultSkillTemplateTest {

    @AfterEach
    void clearSecurityContext() {
        SecurityContextHolder.clearContext();
    }

    @Test
    void capturesCurrentSecurityContextAuthenticationForRootInvocation() {
        CapabilityRegistry registry = new InMemoryCapabilityRegistry();
        CapabilityExecutionRouter router = mock(CapabilityExecutionRouter.class);
        DefaultSkillTemplate template = new DefaultSkillTemplate(
                registry,
                router,
                new LoomspanSessionRunner(4, com.lokiscale.loomspan.internal.core.TracePersistencePolicy.ALWAYS, fixedClock()),
                new ObjectMapper(),
                new SkillInputValidator());
        CapabilityMetadata yamlSkill = yamlSkillMetadata();
        registry.register("invoiceParser", yamlSkill);
        var authentication = new UsernamePasswordAuthenticationToken(
                "alice",
                "n/a",
                List.of(new SimpleGrantedAuthority("ROLE_ALLOWED")));
        SecurityContextHolder.getContext().setAuthentication(authentication);
        AtomicReference<Object> observedAuthentication = new AtomicReference<>();
        when(router.execute(eq(yamlSkill), eq(Map.of("payload", "hello")), any(), eq(null)))
                .thenAnswer(invocation -> {
                    com.lokiscale.loomspan.internal.core.LoomspanSession session = invocation.getArgument(2);
                    observedAuthentication.set(session.getAuthentication().orElse(null));
                    return "\"ok\"";
                });

        template.invoke("invoiceParser", Map.of("payload", "hello"));

        assertThat(observedAuthentication.get()).isSameAs(authentication);
    }

    @Test
    void preservesAccessDeniedExceptionInstance() {
        CapabilityExecutionRouter router = mock(CapabilityExecutionRouter.class);
        DefaultSkillTemplate template = templateWithRegisteredSkill(router);
        AccessDeniedException failure = new AccessDeniedException("denied");
        when(router.execute(any(), any(), any(), eq(null))).thenThrow(failure);

        assertThatThrownBy(() -> template.invoke("invoiceParser", Map.of("payload", "hello")))
                .isSameAs(failure);
    }

    @Test
    void preservesExistingSkillExceptionInstance() {
        CapabilityExecutionRouter router = mock(CapabilityExecutionRouter.class);
        DefaultSkillTemplate template = templateWithRegisteredSkill(router);
        SkillException failure = new SkillException("safe");
        when(router.execute(any(), any(), any(), eq(null))).thenThrow(failure);

        assertThatThrownBy(() -> template.invoke("invoiceParser", Map.of("payload", "hello")))
                .isSameAs(failure);
    }

    @Test
    void wrapsOtherRuntimeFailureWithSafeSkillExceptionAndCause() {
        CapabilityExecutionRouter router = mock(CapabilityExecutionRouter.class);
        DefaultSkillTemplate template = templateWithRegisteredSkill(router);
        IllegalStateException failure = new IllegalStateException("internal provider details");
        when(router.execute(any(), any(), any(), eq(null))).thenThrow(failure);
        AtomicBoolean observerCalled = new AtomicBoolean(false);

        assertThatThrownBy(() -> template.invoke(
                "invoiceParser", Map.of("payload", "hello"), view -> observerCalled.set(true)))
                .isInstanceOf(SkillException.class)
                .hasMessage("Skill 'invoiceParser' execution failed.")
                .hasCause(failure);
        assertThat(observerCalled).isFalse();
    }

    @Test
    void doesNotCatchError() {
        CapabilityExecutionRouter router = mock(CapabilityExecutionRouter.class);
        DefaultSkillTemplate template = templateWithRegisteredSkill(router);
        AssertionError failure = new AssertionError("fatal");
        when(router.execute(any(), any(), any(), eq(null))).thenThrow(failure);

        assertThatThrownBy(() -> template.invoke("invoiceParser", Map.of("payload", "hello")))
                .isSameAs(failure);
    }

    @Test
    void rejectsImplementationTargetIdsAsUnknownYamlSkills() {
        CapabilityRegistry registry = new InMemoryCapabilityRegistry();
        CapabilityExecutionRouter router = mock(CapabilityExecutionRouter.class);
        DefaultSkillTemplate template = new DefaultSkillTemplate(
                registry,
                router,
                new LoomspanSessionRunner(4, com.lokiscale.loomspan.internal.core.TracePersistencePolicy.ALWAYS, fixedClock()),
                new ObjectMapper(),
                new SkillInputValidator());

        assertThatThrownBy(() -> template.invoke("missingSkill", Map.of()))
                .isInstanceOf(SkillException.class)
                .hasMessageContaining("Unknown YAML skill");
        assertThatThrownBy(() -> template.invoke("javaSkill", Map.of()))
                .isInstanceOf(SkillException.class)
                .hasMessageContaining("Unknown YAML skill");
        assertThatThrownBy(() -> template.invoke("bean#javaSkill", Map.of()))
                .isInstanceOf(SkillException.class)
                .hasMessageContaining("Unknown YAML skill");
    }

    @Test
    void rejectsCustomRegistryMetadataThatDoesNotMatchRequestedYamlName() {
        CapabilityRegistry registry = mock(CapabilityRegistry.class);
        CapabilityExecutionRouter router = mock(CapabilityExecutionRouter.class);
        DefaultSkillTemplate template = new DefaultSkillTemplate(
                registry,
                router,
                new LoomspanSessionRunner(4, com.lokiscale.loomspan.internal.core.TracePersistencePolicy.ALWAYS, fixedClock()),
                new ObjectMapper(),
                new SkillInputValidator());
        CapabilityMetadata otherSkill = yamlSkillMetadata();
        when(registry.getCapability("requested.skill")).thenReturn(otherSkill);

        assertThatThrownBy(() -> template.invoke("requested.skill", Map.of("payload", "hello")))
                .isInstanceOf(SkillException.class)
                .hasMessageContaining("invoiceParser")
                .hasMessageContaining("requested.skill");
        verify(router, never()).execute(any(), any(), any(), any());
    }

    @Test
    void skillTemplateNullInputAndObserverLifecycle() {
        CapabilityRegistry registry = new InMemoryCapabilityRegistry();
        CapabilityExecutionRouter router = mock(CapabilityExecutionRouter.class);
        DefaultSkillTemplate template = new DefaultSkillTemplate(
                registry,
                router,
                new LoomspanSessionRunner(4, com.lokiscale.loomspan.internal.core.TracePersistencePolicy.ALWAYS, fixedClock()),
                new ObjectMapper(),
                new SkillInputValidator());
        CapabilityMetadata yamlSkill = new CapabilityMetadata(
                "yaml:invoiceParser",
                "invoiceParser",
                "Invoice parser",
                SkillExecutionDescriptor.none(),
                java.util.Set.of(),
                noopInvoker(),
                CapabilityKind.YAML_SKILL,
                CapabilityToolDescriptor.generic("invoiceParser", "Invoice parser"),
                new SkillInputContract(
                        SkillInputContract.SkillInputContractKind.YAML_EXPLICIT,
                        new SkillInputSchemaNode(
                                "object",
                                Map.of("payload", new SkillInputSchemaNode("string", Map.of(), List.of(), null, null, List.of(), null, null, false)),
                                List.of("payload"),
                                Boolean.FALSE,
                                null,
                                List.of(),
                                null,
                                null,
                                false)),
                null);
        registry.register("invoiceParser", yamlSkill);
        when(router.execute(eq(yamlSkill), eq(Map.of("payload", "hello")), any(), eq(null))).thenReturn("\"ok\"");

        assertThatThrownBy(() -> template.invoke("invoiceParser", (Object) null))
                .isInstanceOf(SkillInputValidationException.class);
        assertThatThrownBy(() -> template.invoke("invoiceParser", (Map<String, Object>) null))
                .isInstanceOf(SkillInputValidationException.class);

        AtomicReference<SkillExecutionView> observed = new AtomicReference<>();
        String result = template.invoke("invoiceParser", Map.of("payload", "hello"), observed::set);

        assertThat(result).isEqualTo("\"ok\"");
        assertThat(observed.get()).isNotNull();
        assertThat(observed.get().sessionId()).isNotBlank();
        assertThat(observed.get().events()).isNotNull();

        assertThatThrownBy(() -> template.invoke("invoiceParser", Map.of(), observed::set))
                .isInstanceOf(SkillInputValidationException.class);
        verify(router, never()).execute(eq(yamlSkill), eq(Map.of()), any(), eq(null));
    }

    @Test
    void objectOverloadDelegatesThroughValidatedMapPath() {
        CapabilityRegistry registry = new InMemoryCapabilityRegistry();
        CapabilityExecutionRouter router = mock(CapabilityExecutionRouter.class);
        DefaultSkillTemplate template = new DefaultSkillTemplate(
                registry,
                router,
                new LoomspanSessionRunner(4, com.lokiscale.loomspan.internal.core.TracePersistencePolicy.ALWAYS, fixedClock()),
                new ObjectMapper(),
                new SkillInputValidator());
        CapabilityMetadata yamlSkill = yamlSkillMetadata();
        registry.register("invoiceParser", yamlSkill);
        when(router.execute(eq(yamlSkill), eq(Map.of("payload", "hello")), any(), eq(null))).thenReturn("\"ok\"");

        String result = template.invoke("invoiceParser", new InvoiceRequest("hello"));

        assertThat(result).isEqualTo("\"ok\"");
        verify(router).execute(eq(yamlSkill), eq(Map.of("payload", "hello")), any(), eq(null));
    }

    @Test
    void observerExceptionPropagatesAfterExecutionCompletes() {
        CapabilityRegistry registry = new InMemoryCapabilityRegistry();
        CapabilityExecutionRouter router = mock(CapabilityExecutionRouter.class);
        DefaultSkillTemplate template = new DefaultSkillTemplate(
                registry,
                router,
                new LoomspanSessionRunner(4, com.lokiscale.loomspan.internal.core.TracePersistencePolicy.ALWAYS, fixedClock()),
                new ObjectMapper(),
                new SkillInputValidator());
        CapabilityMetadata yamlSkill = yamlSkillMetadata();
        registry.register("invoiceParser", yamlSkill);
        when(router.execute(eq(yamlSkill), eq(Map.of("payload", "hello")), any(), eq(null))).thenReturn("\"ok\"");

        assertThatThrownBy(() -> template.invoke("invoiceParser", Map.of("payload", "hello"), view -> {
            throw new IllegalStateException("observer failed");
        }))
                .isInstanceOf(IllegalStateException.class)
                .hasMessageContaining("observer failed");
        verify(router).execute(eq(yamlSkill), eq(Map.of("payload", "hello")), any(), eq(null));
    }

    @Test
    void invalidInputDoesNotInvokeObserver() {
        CapabilityRegistry registry = new InMemoryCapabilityRegistry();
        CapabilityExecutionRouter router = mock(CapabilityExecutionRouter.class);
        DefaultSkillTemplate template = new DefaultSkillTemplate(
                registry,
                router,
                new LoomspanSessionRunner(4, com.lokiscale.loomspan.internal.core.TracePersistencePolicy.ALWAYS, fixedClock()),
                new ObjectMapper(),
                new SkillInputValidator());
        CapabilityMetadata yamlSkill = yamlSkillMetadata();
        registry.register("invoiceParser", yamlSkill);
        AtomicBoolean observerCalled = new AtomicBoolean(false);

        assertThatThrownBy(() -> template.invoke("invoiceParser", Map.of(), view -> observerCalled.set(true)))
                .isInstanceOf(SkillInputValidationException.class);

        assertThat(observerCalled.get()).isFalse();
        verify(router, never()).execute(eq(yamlSkill), eq(Map.of()), any(), eq(null));
    }

    private CapabilityMetadata yamlSkillMetadata() {
        return new CapabilityMetadata(
                "yaml:invoiceParser",
                "invoiceParser",
                "Invoice parser",
                SkillExecutionDescriptor.none(),
                java.util.Set.of(),
                noopInvoker(),
                CapabilityKind.YAML_SKILL,
                CapabilityToolDescriptor.generic("invoiceParser", "Invoice parser"),
                new SkillInputContract(
                        SkillInputContract.SkillInputContractKind.YAML_EXPLICIT,
                        new SkillInputSchemaNode(
                                "object",
                                Map.of("payload", new SkillInputSchemaNode("string", Map.of(), List.of(), null, null, List.of(), null, null, false)),
                                List.of("payload"),
                                Boolean.FALSE,
                                null,
                                List.of(),
                                null,
                                null,
                                false)),
                null);
    }

    private DefaultSkillTemplate templateWithRegisteredSkill(CapabilityExecutionRouter router) {
        CapabilityRegistry registry = new InMemoryCapabilityRegistry();
        registry.register("invoiceParser", yamlSkillMetadata());
        return new DefaultSkillTemplate(
                registry,
                router,
                new LoomspanSessionRunner(4, com.lokiscale.loomspan.internal.core.TracePersistencePolicy.ALWAYS, fixedClock()),
                new ObjectMapper(),
                new SkillInputValidator());
    }

    private record InvoiceRequest(String payload) {
    }

    private CapabilityInvoker noopInvoker() {
        return arguments -> "\"ok\"";
    }

    private Clock fixedClock() {
        return Clock.fixed(Instant.parse("2026-03-30T12:00:00Z"), ZoneOffset.UTC);
    }
}
