package com.lokiscale.loomspan.internal.core;

import tools.jackson.databind.JsonNode;
import tools.jackson.databind.ObjectMapper;
import com.lokiscale.loomspan.api.SkillMethod;
import org.junit.jupiter.api.Test;
import com.lokiscale.loomspan.api.SkillParam;
import org.springframework.aop.framework.ProxyFactory;
import org.springframework.beans.factory.support.StaticListableBeanFactory;

import java.util.Map;
import java.util.concurrent.atomic.AtomicInteger;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class SkillMethodTargetDiscoveryIntegrationTests
{
    @Test
    void discoversInterfaceDeclaredAnnotationAndParameterContractOnJdkProxyExactlyOnce() throws Exception
    {
        InterfaceSkill proxy = jdkProxy(new InterfaceSkillImpl(), null);
        TestHarness harness = process(proxy, "interfaceSkill");

        assertThat(harness.registry().getAllTargets())
                .extracting(SkillImplementationTarget::id)
                .containsExactly("interfaceSkill#execute");
        SkillImplementationTarget target = harness.registry().getTarget("interfaceSkill#execute");
        JsonNode schema = new ObjectMapper().readTree(target.inputSchema());
        assertThat(schema.path("properties").has("internalInput")).isFalse();
        assertThat(schema.path("properties").path("externalInput").path("description").asText())
                .isEqualTo("Interface optional input");
        assertThat(schema.path("required"))
                .noneMatch(node -> "externalInput".equals(node.asText()) || "internalInput".equals(node.asText()));
        assertThat(target.invoker().invoke(Map.of("externalInput", "alpha")))
                .isEqualTo("\"interface:alpha\"");
    }

    @Test
    void canonicalizesGenericBridgeMethodWithoutFalseOverload()
    {
        GenericSkill<String> proxy = jdkProxy(new StringGenericSkill(), null);
        TestHarness harness = process(proxy, "genericSkill");

        assertThat(harness.registry().getAllTargets())
                .extracting(SkillImplementationTarget::id)
                .containsExactly("genericSkill#convert");
        SkillImplementationTarget target = harness.registry().getTarget("genericSkill#convert");
        assertThat(target.inputSchema()).contains("genericInput").doesNotContain("implementationInput");
        assertThat(target.invoker().invoke(Map.of("genericInput", "alpha")))
                .isEqualTo("\"generic:alpha\"");
    }

    @Test
    void discoversCglibProxiedImplementationOnceAndInvokesThroughAdvice()
    {
        AtomicInteger adviceCalls = new AtomicInteger();
        ConcreteSkill proxy = classProxy(new ConcreteSkill(), adviceCalls);
        TestHarness harness = process(proxy, "concreteSkill");

        assertThat(harness.registry().getAllTargets())
                .extracting(SkillImplementationTarget::id)
                .containsExactly("concreteSkill#execute");
        assertThat(harness.registry().getTarget("concreteSkill#execute").invoker().invoke(Map.of("input", "alpha")))
                .isEqualTo("\"class:alpha\"");
        assertThat(adviceCalls).hasValue(1);
    }

    @Test
    void stillRejectsGenuineOverloadsAfterProxyCanonicalization()
    {
        OverloadedConcreteSkill proxy = classProxy(new OverloadedConcreteSkill(), new AtomicInteger());
        InMemorySkillImplementationTargetRegistry registry = new InMemorySkillImplementationTargetRegistry();
        SkillMethodBeanPostProcessor processor = new SkillMethodBeanPostProcessor(registry);
        StaticListableBeanFactory beanFactory = new StaticListableBeanFactory();
        beanFactory.addBean("overloadedSkill", proxy);
        processor.setBeanFactory(beanFactory);

        assertThatThrownBy(() -> processor.postProcessAfterInitialization(proxy, "overloadedSkill"))
                .isInstanceOf(IllegalStateException.class)
                .hasMessageContaining("overloadedSkill#execute")
                .hasMessageContaining("must be unique")
                .hasMessageContaining("rename one method");
        assertThat(registry.getAllTargets()).isEmpty();
    }

    @Test
    void selectsAnnotatedInterfaceMethodWhenJdkProxyExposesBroaderOverload()
    {
        Object proxy = jdkProxy(new CompetingInterfaceSkillImpl(), null);
        TestHarness harness = process(proxy, "competingSkill");

        assertThat(harness.registry().getTarget("competingSkill#execute").invoker()
                .invoke(Map.of("input", "alpha")))
                .isEqualTo("\"annotated:alpha\"");
    }

    @Test
    void rejectsConflictingAnnotatedInterfaceContracts()
    {
        Object proxy = jdkProxy(new ConflictingContractSkillImpl(), null);
        InMemorySkillImplementationTargetRegistry registry = new InMemorySkillImplementationTargetRegistry();
        SkillMethodBeanPostProcessor processor = new SkillMethodBeanPostProcessor(registry);
        StaticListableBeanFactory beanFactory = new StaticListableBeanFactory();
        beanFactory.addBean("conflictingContractSkill", proxy);
        processor.setBeanFactory(beanFactory);

        assertThatThrownBy(() -> processor.postProcessAfterInitialization(proxy, "conflictingContractSkill"))
                .isInstanceOf(IllegalStateException.class)
                .hasMessageContaining("conflictingContractSkill")
                .hasMessageContaining("execute")
                .hasMessageContaining("incompatible method or parameter metadata")
                .hasMessageContaining("one public interface contract");
        assertThat(registry.getAllTargets()).isEmpty();
    }

    @Test
    void acceptsEquivalentAnnotatedInterfaceContracts()
    {
        Object proxy = jdkProxy(new EquivalentContractSkillImpl(), null);
        TestHarness harness = process(proxy, "equivalentContractSkill");

        assertThat(harness.registry().getTarget("equivalentContractSkill#execute").invoker()
                .invoke(Map.of("publicInput", "alpha")))
                .isEqualTo("\"alpha\"");
    }

    private TestHarness process(Object bean, String beanName)
    {
        InMemorySkillImplementationTargetRegistry registry = new InMemorySkillImplementationTargetRegistry();
        SkillMethodBeanPostProcessor processor = new SkillMethodBeanPostProcessor(registry);
        StaticListableBeanFactory beanFactory = new StaticListableBeanFactory();
        beanFactory.addBean(beanName, bean);
        processor.setBeanFactory(beanFactory);
        processor.postProcessAfterInitialization(bean, beanName);
        return new TestHarness(registry);
    }

    @SuppressWarnings("unchecked")
    private <T> T jdkProxy(T target, AtomicInteger adviceCalls)
    {
        ProxyFactory factory = new ProxyFactory(target);
        factory.setProxyTargetClass(false);
        if (adviceCalls != null)
        {
            factory.addAdvice((org.aopalliance.intercept.MethodInterceptor) invocation -> {
                adviceCalls.incrementAndGet();
                return invocation.proceed();
            });
        }
        return (T) factory.getProxy();
    }

    private ConcreteSkill classProxy(ConcreteSkill target, AtomicInteger adviceCalls)
    {
        ProxyFactory factory = new ProxyFactory(target);
        factory.setProxyTargetClass(true);
        factory.addAdvice((org.aopalliance.intercept.MethodInterceptor) invocation -> {
            adviceCalls.incrementAndGet();
            return invocation.proceed();
        });
        return (ConcreteSkill) factory.getProxy();
    }

    private OverloadedConcreteSkill classProxy(OverloadedConcreteSkill target, AtomicInteger adviceCalls)
    {
        ProxyFactory factory = new ProxyFactory(target);
        factory.setProxyTargetClass(true);
        factory.addAdvice((org.aopalliance.intercept.MethodInterceptor) invocation -> {
            adviceCalls.incrementAndGet();
            return invocation.proceed();
        });
        return (OverloadedConcreteSkill) factory.getProxy();
    }

    interface InterfaceSkill
    {
        @SkillMethod(description = "Interface target")
        String execute(@SkillParam(description = "Interface optional input", required = false) String externalInput);
    }

    static class InterfaceSkillImpl implements InterfaceSkill
    {
        @Override
        public String execute(String internalInput)
        {
            return "interface:" + internalInput;
        }
    }

    interface FirstContractSkill
    {
        @SkillMethod(description = "Conflicting target")
        String execute(@SkillParam(description = "First contract", required = false) String publicInput);
    }

    interface SecondContractSkill
    {
        @SkillMethod(description = "Conflicting target")
        String execute(@SkillParam(description = "Second contract") String alternateInput);
    }

    static class ConflictingContractSkillImpl implements FirstContractSkill, SecondContractSkill
    {
        @Override
        public String execute(String internalInput)
        {
            return internalInput;
        }
    }

    interface FirstEquivalentContractSkill
    {
        @SkillMethod(description = "Equivalent target")
        String execute(@SkillParam(description = "Public input") String publicInput);
    }

    interface SecondEquivalentContractSkill
    {
        @SkillMethod(description = "Equivalent target")
        String execute(@SkillParam(description = "Public input") String publicInput);
    }

    static class EquivalentContractSkillImpl implements FirstEquivalentContractSkill, SecondEquivalentContractSkill
    {
        @Override
        public String execute(String internalInput)
        {
            return internalInput;
        }
    }

    interface GenericSkill<T>
    {
        @SkillMethod(description = "Generic target")
        T convert(T genericInput);
    }

    static class StringGenericSkill implements GenericSkill<String>
    {
        @Override
        public String convert(String implementationInput)
        {
            return "generic:" + implementationInput;
        }
    }

    interface AnnotatedStringSkill
    {
        @SkillMethod(description = "Annotated string target")
        String execute(String input);
    }

    interface BroadOverloadSkill
    {
        String execute(CharSequence input);
    }

    static class CompetingInterfaceSkillImpl implements AnnotatedStringSkill, BroadOverloadSkill
    {
        @Override
        public String execute(String input)
        {
            return "annotated:" + input;
        }

        @Override
        public String execute(CharSequence input)
        {
            return "broad:" + input;
        }
    }

    static class ConcreteSkill
    {
        @SkillMethod(description = "Concrete target")
        public String execute(String input)
        {
            return "class:" + input;
        }
    }

    static class OverloadedConcreteSkill
    {
        @SkillMethod(description = "String target")
        public String execute(String input)
        {
            return input;
        }

        @SkillMethod(description = "Integer target")
        public String execute(Integer input)
        {
            return String.valueOf(input);
        }
    }

    private record TestHarness(InMemorySkillImplementationTargetRegistry registry)
    {
    }
}
