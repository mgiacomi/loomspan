package com.lokiscale.loomspan.internal.model;

import java.util.Map;

public record ModelInteractionResult(String content, Map<String, Object> context)
{
    public ModelInteractionResult
    {
        content = content == null ? "" : content;
        context = context == null ? Map.of() : Map.copyOf(context);
    }

    public static ModelInteractionResult content(String content)
    {
        return new ModelInteractionResult(content, Map.of());
    }
}
