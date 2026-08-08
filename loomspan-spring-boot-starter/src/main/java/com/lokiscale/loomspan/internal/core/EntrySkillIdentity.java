package com.lokiscale.loomspan.internal.core;

import java.util.Objects;

final class EntrySkillIdentity
{
    static final int MAX_CODE_POINTS = 256;

    private EntrySkillIdentity() {}

    static String normalize(String value)
    {
        Objects.requireNonNull(value, "entrySkill must not be null");
        if (value.isBlank())
        {
            throw new IllegalArgumentException("entrySkill must not be blank");
        }
        int codePoints = value.codePointCount(0, value.length());
        return codePoints <= MAX_CODE_POINTS
                ? value
                : value.substring(0, value.offsetByCodePoints(0, MAX_CODE_POINTS));
    }
}
