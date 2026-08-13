package com.lokiscale.loomspan.internal.release;

import java.io.IOException;
import java.io.InputStream;
import java.nio.charset.StandardCharsets;
import java.util.Collections;
import java.util.Properties;

public final class LoomspanReleaseVersion
{
    private static final String RESOURCE = "META-INF/loomspan-release.properties";
    private static final String KEY = "consoleCompatibilityVersion";

    private LoomspanReleaseVersion()
    {
    }

    public static String load()
    {
        try
        {
            var resources = Collections.list(
                    LoomspanReleaseVersion.class.getClassLoader().getResources(RESOURCE));
            if (resources.size() != 1)
            {
                throw new IllegalStateException("loomspan release metadata must exist exactly once");
            }
            byte[] bytes;
            try (InputStream input = resources.getFirst().openStream())
            {
                bytes = input.readAllBytes();
            }
            long declarations = new String(bytes, StandardCharsets.ISO_8859_1).lines()
                    .map(String::strip)
                    .filter(line -> line.startsWith(KEY + "=") || line.startsWith(KEY + ":"))
                    .count();
            if (declarations != 1)
            {
                throw new IllegalStateException("loomspan release metadata is invalid");
            }
            Properties properties = new Properties();
            properties.load(new java.io.ByteArrayInputStream(bytes));
            String value = properties.getProperty(KEY);
            if (value == null || value.isBlank() || value.contains("${"))
            {
                throw new IllegalStateException("loomspan release metadata is invalid");
            }
            return value;
        }
        catch (IOException ex)
        {
            throw new IllegalStateException("loomspan release metadata cannot be read", ex);
        }
    }
}
