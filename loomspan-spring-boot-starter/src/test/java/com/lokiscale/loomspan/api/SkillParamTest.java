package com.lokiscale.loomspan.api;

import org.junit.jupiter.api.Test;

import java.lang.annotation.ElementType;
import java.lang.annotation.Retention;
import java.lang.annotation.RetentionPolicy;
import java.lang.annotation.Target;

import static org.assertj.core.api.Assertions.assertThat;

class SkillParamTest
{
    @Test
    void isRuntimeParameterAnnotation()
    {
        assertThat(SkillParam.class.getAnnotation(Retention.class).value())
                .isEqualTo(RetentionPolicy.RUNTIME);
        assertThat(SkillParam.class.getAnnotation(Target.class).value())
                .containsExactly(ElementType.PARAMETER);
    }

    @Test
    void defaultsToBlankDescriptionAndRequired()
            throws NoSuchMethodException
    {
        SkillParam annotation = Samples.class.getDeclaredMethod("defaults", String.class)
                .getParameters()[0]
                .getAnnotation(SkillParam.class);

        assertThat(annotation.description()).isBlank();
        assertThat(annotation.required()).isTrue();
    }

    @Test
    void exposesExplicitDescriptionAndOptionality()
            throws NoSuchMethodException
    {
        SkillParam annotation = Samples.class.getDeclaredMethod("explicit", String.class)
                .getParameters()[0]
                .getAnnotation(SkillParam.class);

        assertThat(annotation.description()).isEqualTo("Optional external value");
        assertThat(annotation.required()).isFalse();
        assertThat(SkillParam.class.getDeclaredMethods())
                .extracting(java.lang.reflect.Method::getName)
                .containsExactlyInAnyOrder("description", "required");
    }

    static class Samples
    {
        void defaults(@SkillParam String value)
        {
        }

        void explicit(@SkillParam(description = "Optional external value", required = false) String value)
        {
        }
    }
}
