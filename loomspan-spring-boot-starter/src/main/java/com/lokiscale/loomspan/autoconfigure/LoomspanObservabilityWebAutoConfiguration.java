package com.lokiscale.loomspan.autoconfigure;

import com.lokiscale.loomspan.internal.observability.ObservabilityActivationCoordinator;
import com.lokiscale.loomspan.internal.observability.web.BoundedJsonPageWriter;
import com.lokiscale.loomspan.internal.observability.web.ObservabilityAccessService;
import com.lokiscale.loomspan.internal.observability.web.ObservabilityApiKeyFilter;
import com.lokiscale.loomspan.internal.observability.web.ObservabilityCursorCodec;
import com.lokiscale.loomspan.internal.observability.web.ObservabilityDtoMapper;
import com.lokiscale.loomspan.internal.observability.web.ObservabilityJsonCodec;
import com.lokiscale.loomspan.internal.observability.web.ObservabilityProblemMapper;
import com.lokiscale.loomspan.internal.observability.web.ObservabilityRestController;
import com.lokiscale.loomspan.internal.observability.web.ObservabilityRouteCollisionDetector;
import com.lokiscale.loomspan.internal.observability.web.ObservabilityRouteRegistrar;
import com.lokiscale.loomspan.internal.skill.YamlSkillCatalog;
import com.lokiscale.loomspan.internal.serialization.LoomspanJacksonCodecs;
import jakarta.servlet.DispatcherType;
import jakarta.servlet.Filter;
import org.springframework.beans.factory.annotation.Qualifier;
import org.springframework.beans.factory.ObjectProvider;
import org.springframework.boot.autoconfigure.AutoConfiguration;
import org.springframework.boot.autoconfigure.AutoConfigureAfter;
import org.springframework.boot.autoconfigure.condition.ConditionalOnClass;
import org.springframework.boot.autoconfigure.condition.ConditionalOnWebApplication;
import org.springframework.boot.webmvc.autoconfigure.WebMvcAutoConfiguration;
import org.springframework.boot.web.servlet.FilterRegistrationBean;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Role;
import org.springframework.beans.factory.config.BeanDefinition;
import org.springframework.web.servlet.DispatcherServlet;
import org.springframework.web.servlet.HandlerMapping;
import org.springframework.web.servlet.mvc.method.annotation.RequestMappingHandlerMapping;

import java.util.EnumSet;
import java.util.List;

@AutoConfiguration
@AutoConfigureAfter({ LoomspanAutoConfiguration.class, WebMvcAutoConfiguration.class })
@ConditionalOnWebApplication(type = ConditionalOnWebApplication.Type.SERVLET)
@ConditionalOnClass({ DispatcherServlet.class, Filter.class })
public class LoomspanObservabilityWebAutoConfiguration
{
    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    ObservabilityJsonCodec observabilityJsonCodec(LoomspanJacksonCodecs codecs)
    {
        return new ObservabilityJsonCodec(codecs.strictObservability());
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    ObservabilityProblemMapper observabilityProblemMapper() { return new ObservabilityProblemMapper(); }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    ObservabilityAccessService observabilityAccessService() { return new ObservabilityAccessService(); }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    ObservabilityDtoMapper observabilityDtoMapper() { return new ObservabilityDtoMapper(); }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    ObservabilityCursorCodec observabilityCursorCodec(ObservabilityJsonCodec json)
    {
        return new ObservabilityCursorCodec(json);
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    BoundedJsonPageWriter boundedJsonPageWriter(ObservabilityJsonCodec json)
    {
        return new BoundedJsonPageWriter(json);
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    ObservabilityRestController observabilityRestController(
            ObservabilityActivationCoordinator activation,
            ObservabilityAccessService access,
            ObservabilityDtoMapper mapper,
            ObservabilityCursorCodec cursors,
            BoundedJsonPageWriter pages,
            ObservabilityJsonCodec json)
    {
        return new ObservabilityRestController(activation, access, mapper, cursors, pages, json);
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    ObservabilityRouteCollisionDetector observabilityRouteCollisionDetector(List<HandlerMapping> handlerMappings)
    {
        return new ObservabilityRouteCollisionDetector(handlerMappings);
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    ObservabilityRouteRegistrar observabilityRouteRegistrar(
            @Qualifier("requestMappingHandlerMapping") ObjectProvider<RequestMappingHandlerMapping> mappings,
            ObservabilityRestController controller,
            ObservabilityRouteCollisionDetector collisions,
            ObservabilityActivationCoordinator activation,
            LoomspanProperties properties,
            ExecutionTraceProperties traceProperties,
            YamlSkillCatalog yamlSkills,
            ObservabilityDtoMapper mapper,
            ObservabilityJsonCodec json)
    {
        return new ObservabilityRouteRegistrar(
                mappings.getIfAvailable(), controller, collisions, activation, properties, traceProperties,
                yamlSkills, mapper, json);
    }

    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    FilterRegistrationBean<ObservabilityApiKeyFilter> observabilityApiKeyFilter(
            ObservabilityActivationCoordinator activation,
            ObservabilityJsonCodec json,
            ObservabilityProblemMapper problems)
    {
        var registration = new FilterRegistrationBean<>(
                new ObservabilityApiKeyFilter(activation, json, problems));
        registration.setName("LoomspanObservabilityApiKeyFilter");
        registration.addUrlPatterns("/_loomspan/observability/v1", "/_loomspan/observability/v1/*");
        registration.setDispatcherTypes(EnumSet.allOf(DispatcherType.class));
        registration.setAsyncSupported(true);
        registration.setOrder(-99);
        return registration;
    }
}
