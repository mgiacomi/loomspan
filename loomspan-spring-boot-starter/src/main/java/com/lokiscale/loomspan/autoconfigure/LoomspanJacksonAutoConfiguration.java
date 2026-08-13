package com.lokiscale.loomspan.autoconfigure;

import com.lokiscale.loomspan.internal.serialization.LoomspanJacksonCodecs;
import org.springframework.boot.autoconfigure.AutoConfiguration;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Role;
import org.springframework.beans.factory.config.BeanDefinition;
import tools.jackson.databind.ObjectMapper;
import org.springframework.beans.factory.ObjectProvider;

@AutoConfiguration
@Role(BeanDefinition.ROLE_INFRASTRUCTURE)
public class LoomspanJacksonAutoConfiguration
{
    @Bean
    @Role(BeanDefinition.ROLE_INFRASTRUCTURE)
    LoomspanJacksonCodecs loomspanJacksonCodecs(ObjectProvider<ObjectMapper> objectMapperProvider)
    {
        return new LoomspanJacksonCodecs(objectMapperProvider.getIfAvailable(ObjectMapper::new));
    }
}
