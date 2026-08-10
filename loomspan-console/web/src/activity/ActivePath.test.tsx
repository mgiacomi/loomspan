import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ActivePath } from "./ActivePath";
import type { ActiveExecution } from "../api/contracts";

function execution(paths: number, overrides: Partial<ActiveExecution> = {}): ActiveExecution {
  return {
    targetScopeId: "scope-1",
    sessionId: "session-1",
    traceId: "trace-1",
    lastCanonicalSequence: 10,
    startedAt: "2026-07-25T12:00:00Z",
    updatedAt: "2026-07-25T12:01:00Z",
    elapsedMillis: 60_000,
    entrySkill: "entry",
    status: "ACTIVE",
    phase: "EXECUTING",
    summary: "active",
    activePath: Array.from({ length: paths }, (_, index) => ({
      frameId: `frame-${index + 1}`,
      frameType: "SKILL",
      route: `/frame/${index + 1}`,
    })),
    totalFrameDepth: paths,
    activePathTruncated: false,
    usage: {
      skillInvocations: 0, toolInvocations: 0, linterRetries: 0, modelCalls: 0, providerAttempts: 0,
      promptUnits: 0, completionUnits: 0, usageUnits: 0,
      exactModelResponses: 0, heuristicModelResponses: 0, unavailableModelResponses: 0,
    },
    configuredLimits: {
      maxSkillInvocations: 10, maxToolInvocations: 10, maxLinterRetries: 3,
      maxModelCalls: 10, maxProviderAttempts: 30, maxUsageUnits: 100,
    },
    ...overrides,
  };
}

describe("ActivePath", () => {
  it("renders nothing without an authoritative path", () => {
    const { container } = render(<ActivePath execution={execution(0)} />);
    expect(container.firstChild).toBeNull();
  });

  it("renders the authoritative current path in order", () => {
    render(<ActivePath execution={execution(2)} />);
    const items = screen.getAllByRole("listitem");
    expect(items[0]).toHaveTextContent("/frame/1");
    expect(items[1]).toHaveTextContent("/frame/2");
    expect(screen.getByRole("navigation")).toHaveAttribute(
      "aria-label",
      "Current bounded active skill path",
    );
  });

  it("reports truncation from the authoritative snapshot", () => {
    render(
      <ActivePath
        execution={execution(2, { activePathTruncated: true, totalFrameDepth: 5 })}
      />,
    );
    expect(screen.getByLabelText("Earlier frames truncated")).toBeInTheDocument();
  });

  it("applies the local display bound without claiming a complete tree", () => {
    render(<ActivePath execution={execution(10)} maxFrames={4} />);
    expect(screen.getAllByRole("listitem")).toHaveLength(4);
    expect(screen.getByText("/frame/7")).toBeInTheDocument();
    expect(screen.getByText(/Current bounded path/)).toBeInTheDocument();
  });
});
