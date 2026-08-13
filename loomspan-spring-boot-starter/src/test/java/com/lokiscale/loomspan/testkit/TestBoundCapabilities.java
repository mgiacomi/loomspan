package com.lokiscale.loomspan.testkit;

import com.lokiscale.loomspan.internal.core.CapabilityKind;
import com.lokiscale.loomspan.internal.core.CapabilityMetadata;
import com.lokiscale.loomspan.internal.core.CapabilityToolDescriptor;
import com.lokiscale.loomspan.internal.core.SkillExecutionDescriptor;
import com.lokiscale.loomspan.internal.runtime.input.SkillInputContract;
import com.lokiscale.loomspan.internal.runtime.input.SkillInputContractResolver;
import com.lokiscale.loomspan.internal.runtime.tool.BoundCapability;

import java.util.Set;

public final class TestBoundCapabilities {
    private TestBoundCapabilities() {
    }

    public static BoundCapability capability(String name) {
        return capability(name, "{}", SkillInputContract.genericObject());
    }

    public static BoundCapability capability(String name, String inputSchema) {
        return capability(name, inputSchema, new SkillInputContractResolver().resolveFromToolSchema(inputSchema));
    }

    public static BoundCapability describedCapability(String name, String description) {
        return capability(name, description, "{}", SkillInputContract.genericObject());
    }

    public static BoundCapability contractAware(String name, String inputSchema, String contractSchema) {
        return capability(name, inputSchema, new SkillInputContractResolver().resolveFromToolSchema(contractSchema));
    }

    private static BoundCapability capability(String name, String inputSchema, SkillInputContract contract) {
        return capability(name, name, inputSchema, contract);
    }

    private static BoundCapability capability(String name, String description, String inputSchema, SkillInputContract contract) {
        CapabilityMetadata metadata = new CapabilityMetadata(
                "test:" + name,
                name,
                description == null || description.isBlank() ? name : description,
                SkillExecutionDescriptor.none(),
                Set.of(),
                input -> null,
                CapabilityKind.YAML_SKILL,
                new CapabilityToolDescriptor(name, description == null || description.isBlank() ? name : description, inputSchema),
                contract,
                null);
        return new BoundCapability(metadata, (arguments, linkedTaskId) -> null);
    }
}
