package com.lokiscale.loomspan.internal.autoconfigure;

import com.lokiscale.loomspan.autoconfigure.LoomspanProperties;
import com.lokiscale.loomspan.internal.provider.ProviderConnectionRuntime;
import com.lokiscale.loomspan.internal.springai.v1_1.SpringAiV11ProviderIntegration;
import org.springframework.beans.factory.DisposableBean;

import java.util.LinkedHashMap;
import java.util.Map;
import java.util.Objects;

public final class NamedAiConnectionRegistry implements DisposableBean
{
    private final Map<String, ProviderConnectionRuntime> connections;

    public NamedAiConnectionRegistry(Map<String, LoomspanProperties.ConnectionProperties> connections,
            SpringAiV11ProviderIntegration integration)
    {
        Objects.requireNonNull(connections, "connections must not be null");
        Objects.requireNonNull(integration, "integration must not be null");
        Map<String, ProviderConnectionRuntime> built = new LinkedHashMap<>();
        for (Map.Entry<String, LoomspanProperties.ConnectionProperties> entry : connections.entrySet())
        {
            String name = entry.getKey();
            LoomspanProperties.ConnectionProperties properties = entry.getValue();
            try
            {
                ProviderConnectionRuntime runtime = integration.create(name, properties);
                if (runtime.attemptOwnership() != com.lokiscale.loomspan.internal.provider.AttemptOwnership.EXACT_ATTEMPT_OWNERSHIP
                        && properties.getProviderRetry().isEnabled())
                {
                    throw new SafeAiConnectionConfigurationException("loomspan.connections." + name
                            + ".provider-retry.enabled must be false because the configured client has opaque retries");
                }
                built.put(name, runtime);
            }
            catch (SafeAiConnectionConfigurationException ex)
            {
                cleanupAfterConstructionFailure(built, ex);
                throw ex;
            }
            catch (RuntimeException ex)
            {
                IllegalStateException failure = new IllegalStateException(
                        "Failed to construct AI connection '" + name + "' for driver " + properties.getDriver());
                cleanupAfterConstructionFailure(built, failure);
                throw failure;
            }
        }
        this.connections = Map.copyOf(built);
    }

    ProviderConnectionRuntime get(String connectionName)
    {
        return connections.get(connectionName);
    }

    public Map<String, ProviderConnectionRuntime> asMap()
    {
        return connections;
    }

    @Override
    public void destroy() throws Exception
    {
        Exception failure = destroyModels(connections);
        if (failure != null) throw failure;
    }

    private static void cleanupAfterConstructionFailure(Map<String, ProviderConnectionRuntime> built, RuntimeException failure)
    {
        Exception cleanupFailure = destroyModels(built);
        if (cleanupFailure != null) failure.addSuppressed(cleanupFailure);
    }

    private static Exception destroyModels(Map<String, ProviderConnectionRuntime> models)
    {
        Exception failure = null;
        for (ProviderConnectionRuntime runtime : models.values())
        {
            Object model = runtime.chatModel();
            try
            {
                if (model instanceof DisposableBean disposableBean) disposableBean.destroy();
                else if (model instanceof AutoCloseable closeable) closeable.close();
            }
            catch (Exception ex)
            {
                if (failure == null) failure = ex; else failure.addSuppressed(ex);
            }
        }
        return failure;
    }
}
