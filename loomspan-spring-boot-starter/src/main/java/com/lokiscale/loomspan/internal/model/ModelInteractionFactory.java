package com.lokiscale.loomspan.internal.model;

import com.lokiscale.loomspan.internal.skill.YamlSkillDefinition;

public interface ModelInteractionFactory
{
    ModelInteraction create(YamlSkillDefinition definition, ModelInteractionMode mode);
}
