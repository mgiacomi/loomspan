package com.lokiscale.loomspan.internal.core;

import tools.jackson.core.JacksonException;
import tools.jackson.databind.ObjectMapper;
import com.lokiscale.loomspan.api.SkillMethod;
import com.lokiscale.loomspan.internal.runtime.input.SkillInputContract;
import com.lokiscale.loomspan.internal.runtime.input.SkillInputContractResolver;
import com.lokiscale.loomspan.internal.runtime.input.SkillInputSchemaNode;
import org.junit.jupiter.api.Test;
import com.lokiscale.loomspan.api.SkillParam;
import org.springframework.boot.test.system.CapturedOutput;
import org.springframework.boot.test.system.OutputCaptureExtension;
import org.junit.jupiter.api.extension.ExtendWith;
import org.springframework.core.io.ByteArrayResource;
import org.springframework.core.io.Resource;
import org.springframework.beans.factory.support.StaticListableBeanFactory;
import org.springframework.util.StreamUtils;

import java.io.IOException;
import java.io.InputStream;
import java.lang.reflect.Method;
import java.nio.charset.StandardCharsets;
import java.util.List;
import java.util.Map;

import static org.assertj.core.api.Assertions.assertThat;

@ExtendWith(OutputCaptureExtension.class)
class SkillMethodBeanPostProcessorTest {

    private final ObjectMapper objectMapper = new ObjectMapper();
    private final StaticListableBeanFactory beanFactory = new StaticListableBeanFactory();

    @Test
    void registersAnnotatedMethodAsInternalTarget() {
        InMemorySkillImplementationTargetRegistry registry = new InMemorySkillImplementationTargetRegistry();
        SkillMethodBeanPostProcessor processor = processor(registry);

        process(processor, new RegistrationBean(), "registrationBean");

        SkillImplementationTarget metadata = registry.getTarget("registrationBean#mappedOperation");
        assertThat(metadata).isNotNull();
        assertThat(metadata.id()).isEqualTo("registrationBean#mappedOperation");
        assertThat(metadata.description()).isEqualTo("Test desc");
        assertThat(registry.getTarget("registrationBean#nonSkillOperation")).isNull();
        assertThat(registry.getAllTargets()).hasSize(1);
    }

    @Test
    void registersDifferentlyNamedAnnotatedMethodsOnOneBean() {
        InMemorySkillImplementationTargetRegistry registry = new InMemorySkillImplementationTargetRegistry();
        SkillMethodBeanPostProcessor processor = processor(registry);

        process(processor, new MultipleMethodsBean(), "multipleMethodsBean");

        assertThat(registry.getAllTargets())
                .extracting(SkillImplementationTarget::id)
                .containsExactlyInAnyOrder("multipleMethodsBean#first", "multipleMethodsBean#second");
    }

    @Test
    void rejectsAnnotatedOverloadsBeforeRegisteringAnyTargetFromBean() {
        InMemorySkillImplementationTargetRegistry registry = new InMemorySkillImplementationTargetRegistry();
        SkillMethodBeanPostProcessor processor = processor(registry);

        org.assertj.core.api.Assertions.assertThatThrownBy(() ->
                process(processor, new OverloadedMethodsBean(), "overloadedMethodsBean"))
                .isInstanceOf(IllegalStateException.class)
                .hasMessageContaining("overloadedMethodsBean")
                .hasMessageContaining("lookup")
                .hasMessageContaining("overloadedMethodsBean#lookup")
                .hasMessageContaining("must be unique")
                .hasMessageContaining("rename one method");
        assertThat(registry.getAllTargets()).isEmpty();
    }

    @Test
    void invokesCapabilityUsingEnvelopeMap() throws JacksonException {
        InMemorySkillImplementationTargetRegistry registry = new InMemorySkillImplementationTargetRegistry();
        SkillMethodBeanPostProcessor processor = processor(registry);

        process(processor, new InvocationBean(), "invocationBean");

        SkillImplementationTarget metadata = registry.getTarget("invocationBean#combineValues");
        Method method = getDeclaredMethod(InvocationBean.class, "combineValues", String.class, Boolean.class);
        Map<String, Object> arguments = Map.of(
                method.getParameters()[0].getName(), "alpha",
                method.getParameters()[1].getName(), true);

        Object rawResult = metadata.invoker().invoke(arguments);

        assertThat(rawResult).isInstanceOf(String.class);
        assertThat(objectMapper.readValue((String) rawResult, String.class)).isEqualTo("alpha:true");
    }

    @Test
    void handlesMissingOptionalParameterWithoutCrashing() throws JacksonException {
        InMemorySkillImplementationTargetRegistry registry = new InMemorySkillImplementationTargetRegistry();
        SkillMethodBeanPostProcessor processor = processor(registry);

        process(processor, new OptionalInvocationBean(), "optionalInvocationBean");

        SkillImplementationTarget metadata = registry.getTarget("optionalInvocationBean#optionalValues");
        Method method = getDeclaredMethod(OptionalInvocationBean.class, "optionalValues", String.class, String.class);
        Map<String, Object> arguments = Map.of(method.getParameters()[0].getName(), "base");

        Object rawResult = metadata.invoker().invoke(arguments);

        assertThat(rawResult).isInstanceOf(String.class);
        assertThat(objectMapper.readValue((String) rawResult, String.class)).isEqualTo("base:default");
    }

    @Test
    void publishesDescriptionsAndBindsUnconstrainedReflectedInputs() throws JacksonException
    {
        InMemorySkillImplementationTargetRegistry registry = new InMemorySkillImplementationTargetRegistry();
        SkillMethodBeanPostProcessor processor = processor(registry);
        process(processor, new UnconstrainedInvocationBean(), "unconstrainedInvocationBean");

        SkillImplementationTarget target = registry.getTarget("unconstrainedInvocationBean#acceptValues");
        Method method = getDeclaredMethod(UnconstrainedInvocationBean.class, "acceptValues", Object.class, List.class);
        String valueName = method.getParameters()[0].getName();
        String optionsName = method.getParameters()[1].getName();
        tools.jackson.databind.JsonNode schema = objectMapper.readTree(target.inputSchema());

        assertThat(schema.path("properties").path(valueName).has("type")).isFalse();
        assertThat(schema.path("properties").path(valueName).path("description").asText())
                .isEqualTo("Any JSON-compatible value");
        assertThat(schema.path("required").size()).isEqualTo(1);
        assertThat(schema.path("required").get(0).asText()).isEqualTo(optionsName);
        assertThat(target.inputContract().schema().properties().get(valueName).isUnconstrained()).isTrue();
        assertThat(target.inputContract().schema().properties().get(optionsName)
                .items().additionalPropertiesSchema().isUnconstrained()).isTrue();

        Map<String, Object> arguments = Map.of(
                valueName, List.of("alpha", 2, true),
                optionsName, List.of(Map.of("operator", "Northeast Regional", "price", 69.0)));
        tools.jackson.databind.JsonNode result = objectMapper.readTree((String) target.invoker().invoke(arguments));
        assertThat(result.path("value").get(1).isIntegralNumber()).isTrue();
        assertThat(result.path("options").get(0).path("operator").asText()).isEqualTo("Northeast Regional");
        assertThat(result.path("options").get(0).path("price").isFloatingPointNumber()).isTrue();
    }

    @Test
    void rejectsOptionalPrimitiveParameterDuringTargetDiscovery()
    {
        InMemorySkillImplementationTargetRegistry registry = new InMemorySkillImplementationTargetRegistry();
        SkillMethodBeanPostProcessor processor = processor(registry);

        org.assertj.core.api.Assertions.assertThatThrownBy(() ->
                process(processor, new OptionalPrimitiveBean(), "optionalPrimitiveBean"))
                .isInstanceOf(IllegalStateException.class)
                .hasMessageContaining("Invalid @SkillParam contract")
                .hasMessageContaining("optionalPrimitiveBean")
                .hasMessageContaining("optionalCount")
                .hasMessageContaining("required=false")
                .hasMessageContaining("primitive int")
                .hasMessageContaining("boxed type");
        assertThat(registry.getAllTargets()).isEmpty();
    }

    @Test
    void returnsTransformedErrorWhenSkillMethodThrowsWrappedBusinessException() {
        InMemorySkillImplementationTargetRegistry registry = new InMemorySkillImplementationTargetRegistry();
        SkillMethodBeanPostProcessor processor = processor(registry);

        process(processor, new ThrowingInvocationBean(), "throwingInvocationBean");

        SkillImplementationTarget metadata = registry.getTarget("throwingInvocationBean#throwingOperation");

        assertThat(metadata.invoker().invoke(Map.of())).isEqualTo("ERROR: IllegalArgumentException. HINT: boom");
    }

    @Test
    void logsStackTraceButOmitsItFromReturnedPayload(CapturedOutput output) {
        InMemorySkillImplementationTargetRegistry registry = new InMemorySkillImplementationTargetRegistry();
        SkillMethodBeanPostProcessor processor = processor(registry);

        process(processor, new ThrowingInvocationBean(), "throwingInvocationBean");

        SkillImplementationTarget metadata = registry.getTarget("throwingInvocationBean#throwingOperation");
        Object rawResult = metadata.invoker().invoke(Map.of());

        assertThat(rawResult).isEqualTo("ERROR: IllegalArgumentException. HINT: boom");
        assertThat(output.getAll()).contains("Skill implementation target 'throwingInvocationBean#throwingOperation' failed during deterministic execution");
        assertThat(output.getAll()).contains("java.lang.IllegalStateException: wrapper");
        assertThat(output.getAll()).contains("Caused by: java.lang.IllegalArgumentException: boom");
        assertThat(rawResult.toString()).doesNotContain("at com.lokiscale");
        assertThat(rawResult.toString()).doesNotContain("IllegalStateException: wrapper");
    }

    @Test
    void readsResourceBackedRefsIntoStringParameters() throws JacksonException {
        InMemorySkillImplementationTargetRegistry registry = new InMemorySkillImplementationTargetRegistry();
        SkillMethodBeanPostProcessor processor = processor(registry);

        process(processor, new RefStringInvocationBean(), "refStringInvocationBean");

        SkillImplementationTarget metadata = registry.getTarget("refStringInvocationBean#readRefAsString");
        Method method = getDeclaredMethod(RefStringInvocationBean.class, "readRefAsString", String.class);
        Object rawResult = metadata.invoker().invoke(Map.of(
                method.getParameters()[0].getName(),
                new ByteArrayResource("hello text".getBytes(StandardCharsets.UTF_8))));

        assertThat(objectMapper.readValue((String) rawResult, String.class)).isEqualTo("hello text");
    }

    @Test
    void readsResourceBackedRefsIntoByteArrayParameters() throws JacksonException {
        InMemorySkillImplementationTargetRegistry registry = new InMemorySkillImplementationTargetRegistry();
        SkillMethodBeanPostProcessor processor = processor(registry);

        process(processor, new RefBytesInvocationBean(), "refBytesInvocationBean");

        SkillImplementationTarget metadata = registry.getTarget("refBytesInvocationBean#readRefAsBytes");
        Method method = getDeclaredMethod(RefBytesInvocationBean.class, "readRefAsBytes", byte[].class);
        Object rawResult = metadata.invoker().invoke(Map.of(
                method.getParameters()[0].getName(),
                new ByteArrayResource(new byte[]{0x01, 0x02, 0x03})));

        assertThat(objectMapper.readValue((String) rawResult, String.class)).isEqualTo("010203");
    }

    @Test
    void passesResourceBackedRefsThroughResourceAndInputStreamParameters() throws JacksonException {
        InMemorySkillImplementationTargetRegistry registry = new InMemorySkillImplementationTargetRegistry();
        SkillMethodBeanPostProcessor processor = processor(registry);

        process(processor, new RefResourceInvocationBean(), "refResourceInvocationBean");
        process(processor, new RefStreamInvocationBean(), "refStreamInvocationBean");

        SkillImplementationTarget resourceMetadata = registry.getTarget("refResourceInvocationBean#readRefAsResource");
        SkillImplementationTarget streamMetadata = registry.getTarget("refStreamInvocationBean#readRefAsStream");
        Method resourceMethod = getDeclaredMethod(RefResourceInvocationBean.class, "readRefAsResource", Resource.class);
        Method streamMethod = getDeclaredMethod(RefStreamInvocationBean.class, "readRefAsStream", InputStream.class);

        Object resourceResult = resourceMetadata.invoker().invoke(Map.of(
                resourceMethod.getParameters()[0].getName(),
                new ByteArrayResource("hello resource".getBytes(StandardCharsets.UTF_8))));
        Object streamResult = streamMetadata.invoker().invoke(Map.of(
                streamMethod.getParameters()[0].getName(),
                new ByteArrayResource("hello stream".getBytes(StandardCharsets.UTF_8))));

        assertThat(objectMapper.readValue((String) resourceResult, String.class)).isEqualTo("hello resource");
        assertThat(objectMapper.readValue((String) streamResult, String.class)).isEqualTo("hello stream");
    }

    @Test
    void publishesRefFriendlyInputSchemasForRefCapableParameters() throws JacksonException {
        InMemorySkillImplementationTargetRegistry registry = new InMemorySkillImplementationTargetRegistry();
        SkillMethodBeanPostProcessor processor = processor(registry);

        process(processor, new RefBytesInvocationBean(), "refBytesInvocationBean");
        process(processor, new RefResourceInvocationBean(), "refResourceInvocationBean");
        process(processor, new RefStreamInvocationBean(), "refStreamInvocationBean");
        process(processor, new NestedRecordInvocationBean(), "nestedRecordInvocationBean");

        assertThat(readPayloadPropertyType(registry.getTarget("refBytesInvocationBean#readRefAsBytes"))).isEqualTo("string");
        assertThat(readPayloadPropertyType(registry.getTarget("refResourceInvocationBean#readRefAsResource"))).isEqualTo("string");
        assertThat(readPayloadPropertyType(registry.getTarget("refStreamInvocationBean#readRefAsStream"))).isEqualTo("string");
        assertThat(readNestedAttachmentItemType(registry.getTarget("nestedRecordInvocationBean#readNestedRecord"))).isEqualTo("string");
        assertThat(readPayloadPropertyRefMarker(registry.getTarget("refBytesInvocationBean#readRefAsBytes"))).isTrue();
        assertThat(readPayloadPropertyRefMarker(registry.getTarget("refResourceInvocationBean#readRefAsResource"))).isTrue();
        assertThat(readPayloadPropertyRefMarker(registry.getTarget("refStreamInvocationBean#readRefAsStream"))).isTrue();
        assertThat(readNestedAttachmentItemRefMarker(registry.getTarget("nestedRecordInvocationBean#readNestedRecord"))).isTrue();
    }

    @Test
    void materializesNestedResourceLeavesInsideTypedRecordParameters() throws JacksonException {
        InMemorySkillImplementationTargetRegistry registry = new InMemorySkillImplementationTargetRegistry();
        SkillMethodBeanPostProcessor processor = processor(registry);

        process(processor, new NestedRecordInvocationBean(), "nestedRecordInvocationBean");

        SkillImplementationTarget metadata = registry.getTarget("nestedRecordInvocationBean#readNestedRecord");
        Method method = getDeclaredMethod(NestedRecordInvocationBean.class, "readNestedRecord", NestedPayload.class);
        Object rawResult = metadata.invoker().invoke(Map.of(
                method.getParameters()[0].getName(),
                Map.of(
                        "document", Map.of(
                                "title", new ByteArrayResource("hello title".getBytes(StandardCharsets.UTF_8)),
                                "attachments", List.of(
                                        new ByteArrayResource(new byte[]{0x01, 0x02}),
                                        new ByteArrayResource(new byte[]{0x0A, 0x0B}))))));

        assertThat(objectMapper.readValue((String) rawResult, String.class)).isEqualTo("hello title|0102,0a0b");
    }

    @Test
    void materializesResourceLeavesInsideTypedCollectionParameters() throws JacksonException {
        InMemorySkillImplementationTargetRegistry registry = new InMemorySkillImplementationTargetRegistry();
        SkillMethodBeanPostProcessor processor = processor(registry);

        process(processor, new TypedCollectionInvocationBean(), "typedCollectionInvocationBean");

        SkillImplementationTarget metadata = registry.getTarget("typedCollectionInvocationBean#readTypedCollection");
        Method method = getDeclaredMethod(TypedCollectionInvocationBean.class, "readTypedCollection", List.class);
        Object rawResult = metadata.invoker().invoke(Map.of(
                method.getParameters()[0].getName(),
                List.of(
                        new ByteArrayResource("alpha".getBytes(StandardCharsets.UTF_8)),
                        new ByteArrayResource("beta".getBytes(StandardCharsets.UTF_8)),
                        new ByteArrayResource("gamma".getBytes(StandardCharsets.UTF_8)))));

        assertThat(objectMapper.readValue((String) rawResult, String.class)).isEqualTo("alpha|beta|gamma");
    }

    @Test
    void preservesObjectValuedAdditionalPropertiesWhenResolvingInputContracts() {
        SkillInputContract contract = new SkillInputContractResolver().resolveFromToolSchema("""
                {
                  "type": "object",
                  "properties": {
                    "payload": {
                      "type": "object",
                      "additionalProperties": {
                        "type": "string"
                      }
                    }
                  },
                  "required": ["payload"],
                  "additionalProperties": false
                }
                """);

        assertThat(contract.schema().properties().get("payload").additionalPropertiesSchema()).isNotNull();
        assertThat(contract.schema().properties().get("payload").additionalPropertiesSchema().type()).isEqualTo("string");
        assertThat(contract.isGeneric()).isFalse();
    }

    @Test
    void treatsStrictEmptyObjectSchemaAsConcreteContract() {
        SkillInputContract contract = new SkillInputContractResolver().resolveFromToolSchema("""
                {
                  "type": "object",
                  "additionalProperties": false
                }
                """);

        assertThat(contract.isGeneric()).isFalse();
        assertThat(contract.schema().allowsAdditionalProperties()).isFalse();
    }

    private SkillMethodBeanPostProcessor processor(InMemorySkillImplementationTargetRegistry registry) {
        SkillMethodBeanPostProcessor processor = new SkillMethodBeanPostProcessor(registry);
        processor.setBeanFactory(beanFactory);
        return processor;
    }

    private void process(SkillMethodBeanPostProcessor processor, Object bean, String beanName) {
        beanFactory.addBean(beanName, bean);
        processor.postProcessAfterInitialization(bean, beanName);
    }

    private static Method getDeclaredMethod(Class<?> type, String name, Class<?>... parameterTypes) {
        try {
            return type.getDeclaredMethod(name, parameterTypes);
        }
        catch (NoSuchMethodException ex) {
            throw new IllegalStateException("Test fixture method lookup failed", ex);
        }
    }

    static class RegistrationBean {

        @SkillMethod(description = "Test desc")
        String mappedOperation() {
            return "ok";
        }

        void nonSkillOperation() {
        }
    }

    static class InvocationBean {

        @SkillMethod(description = "Combine values")
        String combineValues(String left, Boolean right) {
            return left + ":" + right;
        }
    }

    static class MultipleMethodsBean {
        @SkillMethod(description = "First")
        String first() { return "first"; }

        @SkillMethod(description = "Second")
        String second() { return "second"; }
    }

    static class OverloadedMethodsBean {
        @SkillMethod(description = "Lookup text")
        String lookup(String value) { return value; }

        @SkillMethod(description = "Lookup number")
        String lookup(long value) { return Long.toString(value); }

        @SkillMethod(description = "Unique")
        String unique() { return "unique"; }
    }

    static class OptionalInvocationBean {

        @SkillMethod(description = "Optional values")
        String optionalValues(String required, @SkillParam(required = false) String optional) {
            return required + ":" + (optional == null ? "default" : optional);
        }
    }

    static class OptionalPrimitiveBean {

        @SkillMethod(description = "Invalid optional primitive")
        String invalid(@SkillParam(required = false) int optionalCount) {
            return Integer.toString(optionalCount);
        }
    }

    static class ThrowingInvocationBean {

        @SkillMethod(description = "Throwing operation")
        String throwingOperation() {
            throw new IllegalStateException("wrapper", new IllegalArgumentException("boom"));
        }
    }

    static class RefStringInvocationBean {

        @SkillMethod(description = "Read ref as string")
        String readRefAsString(String payload) {
            return payload;
        }
    }

    static class RefBytesInvocationBean {

        @SkillMethod(description = "Read ref as bytes")
        String readRefAsBytes(byte[] payload) {
            StringBuilder builder = new StringBuilder();
            for (byte value : payload) {
                builder.append(String.format("%02x", value));
            }
            return builder.toString();
        }
    }

    static class RefResourceInvocationBean {

        @SkillMethod(description = "Read ref as resource")
        String readRefAsResource(Resource payload) {
            try {
                return StreamUtils.copyToString(payload.getInputStream(), StandardCharsets.UTF_8);
            }
            catch (IOException ex) {
                throw new IllegalStateException(ex);
            }
        }
    }

    static class RefStreamInvocationBean {

        @SkillMethod(description = "Read ref as stream")
        String readRefAsStream(InputStream payload) {
            try {
                return StreamUtils.copyToString(payload, StandardCharsets.UTF_8);
            }
            catch (IOException ex) {
                throw new IllegalStateException(ex);
            }
        }
    }

    static class NestedRecordInvocationBean {

        @SkillMethod(description = "Read nested record")
        String readNestedRecord(NestedPayload payload) {
            return payload.document().title() + "|" + payload.document().attachments().stream()
                    .map(bytes -> {
                        StringBuilder builder = new StringBuilder();
                        for (byte value : bytes) {
                            builder.append(String.format("%02x", value));
                        }
                        return builder.toString();
                    })
                    .reduce((left, right) -> left + "," + right)
                    .orElse("");
        }
    }

    static class TypedCollectionInvocationBean {

        @SkillMethod(description = "Read typed collection")
        String readTypedCollection(List<String> payload) {
            return String.join("|", payload);
        }
    }

    static class UnconstrainedInvocationBean
    {
        @SkillMethod(description = "Accept unconstrained values")
        Map<String, Object> acceptValues(
                @SkillParam(description = "Any JSON-compatible value", required = false) Object value,
                @SkillParam(description = "Scalar-valued option maps") List<Map<String, Object>> options)
        {
            return Map.of("value", value, "options", options);
        }
    }

    record NestedPayload(NestedDocument document) {
    }

    record NestedDocument(String title, List<byte[]> attachments) {
    }

    private String readPayloadPropertyType(SkillImplementationTarget metadata) throws JacksonException {
        return objectMapper.readTree(metadata.inputSchema())
                .path("properties")
                .path("payload")
                .path("type")
                .asText();
    }

    private String readNestedAttachmentItemType(SkillImplementationTarget metadata) throws JacksonException {
        return objectMapper.readTree(metadata.inputSchema())
                .path("properties")
                .path("payload")
                .path("properties")
                .path("document")
                .path("properties")
                .path("attachments")
                .path("items")
                .path("type")
                .asText();
    }

    private boolean readPayloadPropertyRefMarker(SkillImplementationTarget metadata) throws JacksonException {
        return objectMapper.readTree(metadata.inputSchema())
                .path("properties")
                .path("payload")
                .path("x-loomspan-runtime-ref-capable")
                .asBoolean(false);
    }

    private boolean readNestedAttachmentItemRefMarker(SkillImplementationTarget metadata) throws JacksonException {
        return objectMapper.readTree(metadata.inputSchema())
                .path("properties")
                .path("payload")
                .path("properties")
                .path("document")
                .path("properties")
                .path("attachments")
                .path("items")
                .path("x-loomspan-runtime-ref-capable")
                .asBoolean(false);
    }

}
