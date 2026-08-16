package com.lokiscale.loomspan.internal.runtime.planning;

import com.lokiscale.loomspan.internal.core.DefaultPlanTaskLinker;
import com.lokiscale.loomspan.internal.runtime.evidence.EvidenceCoverageValidator;
import com.lokiscale.loomspan.internal.runtime.state.ExecutionStateService;
import com.lokiscale.loomspan.internal.serialization.LoomspanJacksonCodecs;

import java.util.function.Supplier;

/** Test-only access to the internal deterministic plan-ID construction path. */
public final class PlanningServiceTestFactory {

    private PlanningServiceTestFactory() {
    }

    public static DefaultPlanningService withPlanIds(
            ExecutionStateService stateService,
            Supplier<String> planIdSupplier) {
        LoomspanJacksonCodecs codecs = LoomspanJacksonCodecs.defaults();
        return new DefaultPlanningService(
                new DefaultPlanTaskLinker(),
                stateService,
                codecs.planningJson(),
                codecs.planningYaml(),
                new PlanQualityValidator(),
                new EvidenceCoverageValidator(),
                planIdSupplier);
    }
}
