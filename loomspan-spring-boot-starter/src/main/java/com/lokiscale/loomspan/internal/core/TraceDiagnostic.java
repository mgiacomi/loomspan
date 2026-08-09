package com.lokiscale.loomspan.internal.core;

/** One bounded opaque text diagnostic embedded in an error record. */
record TraceDiagnostic(String kind, String contentType, String text, boolean truncated, int captureLimitBytes)
{
}
