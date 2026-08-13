package com.lokiscale.loomspan.internal.release;

import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;

class LoomspanReleaseVersionTest
{
    @Test
    void loadsCompleteFilteredMavenReleaseIncludingQualifier()
    {
        assertThat(LoomspanReleaseVersion.load()).isEqualTo("0.1.0-SNAPSHOT");
    }
}
