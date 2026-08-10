package com.lokiscale.loomspan.internal.provider;

@FunctionalInterface
public interface ProviderFailureTranslator
{
    ProviderFailureDetails translate(Throwable failure);
}
