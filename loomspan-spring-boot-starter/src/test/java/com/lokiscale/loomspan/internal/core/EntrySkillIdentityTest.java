package com.lokiscale.loomspan.internal.core;

import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class EntrySkillIdentityTest
{
    @Test
    void rejectsNullAndBlankIdentity()
    {
        assertThatThrownBy(() -> EntrySkillIdentity.normalize(null)).isInstanceOf(NullPointerException.class);
        assertThatThrownBy(() -> EntrySkillIdentity.normalize(""))
                .isInstanceOf(IllegalArgumentException.class);
        assertThatThrownBy(() -> EntrySkillIdentity.normalize(" \t\n"))
                .isInstanceOf(IllegalArgumentException.class);
    }

    @Test
    void preservesExactBoundAndTruncatesSupplementaryUnicodeByCodePoint()
    {
        String exact = "x".repeat(EntrySkillIdentity.MAX_CODE_POINTS);
        String over = "😀".repeat(EntrySkillIdentity.MAX_CODE_POINTS + 1);

        assertThat(EntrySkillIdentity.normalize(exact)).isSameAs(exact);
        assertThat(EntrySkillIdentity.normalize(over))
                .isEqualTo("😀".repeat(EntrySkillIdentity.MAX_CODE_POINTS))
                .hasSize(EntrySkillIdentity.MAX_CODE_POINTS * 2);
    }
}
