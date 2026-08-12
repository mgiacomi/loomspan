import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, test, vi } from "vitest";
import type { TargetResponse } from "../api/contracts";
import { BrowserAPIError } from "../api/client";
const operations = vi.hoisted(() => ({
  connect: vi.fn(),
  credential: vi.fn(),
  recheck: vi.fn(),
}));
const view = vi.hoisted(() => ({
  current: undefined as unknown as {
    target: TargetResponse;
    error?: BrowserAPIError;
    scopeGeneration: number;
    defaults?: { address: string; applicationKey: string };
  },
}));
const noTarget: TargetResponse = {
  unencrypted: false,
  status: {
    observedAt: "2026-07-27T00:00:00Z",
    targetSelection: "NONE",
    targetConnection: "NOT_APPLICABLE",
    targetAuthentication: "NOT_APPLICABLE",
    javaGoCompatibility: "NOT_APPLICABLE",
    runtimeIdentity: "NOT_APPLICABLE",
    liveMonitoring: "NOT_APPLICABLE",
  },
};
const selectedTarget: TargetResponse = {
  address: "https://application.example",
  unencrypted: false,
  status: {
    observedAt: "2026-07-27T00:00:00Z",
    targetScopeId: "scope-1",
    targetSelection: "SELECTED",
    targetConnection: "REACHABLE",
    targetAuthentication: "ESTABLISHED",
    javaGoCompatibility: "COMPATIBLE",
    runtimeIdentity: "ESTABLISHED",
    instanceId: "11111111-1111-4111-8111-111111111111",
    liveMonitoring: "AVAILABLE",
  },
};
vi.mock("./TargetProvider", () => ({
  useTarget: () => ({
    defaults: { address: "", applicationKey: "" },
    ...view.current,
    connect: operations.connect,
    credential: operations.credential,
    recheck: operations.recheck,
  }),
}));
import { Overview } from "./Overview";
beforeEach(() => {
  operations.connect.mockReset();
  operations.credential.mockReset();
  operations.recheck.mockReset();
  view.current = { target: noTarget, scopeGeneration: 0 };
});
test("overview presents established HTTP facts and safe incompatibility details", async () => {
  view.current = {
    target: {
      address: "http://application.example",
      unencrypted: true,
      status: {
        observedAt: "2026-07-27T00:00:00Z",
        targetScopeId: "scope-1",
        targetSelection: "SELECTED",
        targetConnection: "REACHABLE",
        targetAuthentication: "ESTABLISHED",
        javaGoCompatibility: "INCOMPATIBLE",
        runtimeIdentity: "NOT_ESTABLISHED",
        liveMonitoring: "UNKNOWN",
      },
    },
    scopeGeneration: 0,
    error: new BrowserAPIError(
      "INCOMPATIBLE_TARGET",
      "The selected target uses a different Loomspan release.",
      409,
      "scope-1",
      {
        expectedCompatibilityVersion: "0.1.0-SNAPSHOT",
        observedCompatibilityVersion: "0.1.0",
      },
    ),
  };
  operations.recheck.mockResolvedValue(undefined);
  render(<Overview />);
  expect(screen.getByText(/Unencrypted/)).toBeVisible();
  expect(
    screen.getByText(/Expected 0.1.0-SNAPSHOT; observed 0.1.0/),
  ).toBeVisible();
  await userEvent.click(screen.getByRole("button", { name: "Recheck target" }));
  expect(operations.recheck).toHaveBeenCalled();
});
test("overview connects and always clears application-key form ownership", async () => {
  const secret = "LOOMSPAN_" + "TEST_APPLICATION_KEY_DO_NOT_LEAK_123456";
  operations.connect.mockRejectedValue(new Error("safe failure"));
  render(<Overview />);
  fireEvent.change(screen.getByLabelText("Target address"), {
    target: { value: "https://application.example" },
  });
  fireEvent.change(screen.getByLabelText("Application key"), {
    target: { value: secret },
  });
  await userEvent.click(screen.getByRole("button", { name: "Connect" }));
  expect(operations.connect).toHaveBeenCalledWith(
    "https://application.example",
    secret,
  );
  expect(screen.getByLabelText("Application key")).toHaveValue("");
  expect(document.body).not.toHaveTextContent(secret);
  expect(sessionStorage.getItem(secret)).toBeNull();
  expect(localStorage.getItem(secret)).toBeNull();
});
test("overview initializes the disconnected form without connecting", () => {
  const applicationKey = "DEFAULT_APPLICATION_KEY_123456789012345";
  view.current = {
    target: noTarget,
    scopeGeneration: 0,
    defaults: {
      address: "http://127.0.0.1:8080/context",
      applicationKey,
    },
  };

  render(<Overview />);

  expect(screen.getByLabelText("Target address")).toHaveValue(
    "http://127.0.0.1:8080/context",
  );
  expect(screen.getByLabelText("Application key")).toHaveValue(applicationKey);
  expect(operations.connect).not.toHaveBeenCalled();
});
test("overview explicitly confirms and submits target replacement", async () => {
  const replacementKey = "LOOMSPAN_" + "REPLACEMENT_KEY_123456789012345";
  view.current = { target: selectedTarget, scopeGeneration: 0 };
  operations.connect.mockResolvedValue(undefined);
  render(<Overview />);
  await userEvent.click(screen.getByRole("button", { name: "Change target" }));
  expect(
    screen.getByText(
      /clears all data associated with the current target scope/,
    ),
  ).toBeVisible();
  fireEvent.change(screen.getByLabelText("Target address"), {
    target: { value: "https://replacement.example" },
  });
  fireEvent.change(screen.getByLabelText("Application key"), {
    target: { value: replacementKey },
  });
  await userEvent.click(screen.getByRole("button", { name: "Replace target" }));
  expect(operations.connect).toHaveBeenCalledWith(
    "https://replacement.example",
    replacementKey,
  );
  expect(screen.getByLabelText("Application key")).toHaveValue("");
});
test("overview presents actionable transport guidance", () => {
  view.current = {
    target: selectedTarget,
    scopeGeneration: 0,
    error: new BrowserAPIError(
      "TARGET_UNAVAILABLE",
      "The selected target is unavailable.",
      503,
      "scope-1",
      { transportCategory: "namespace_not_found" },
    ),
  };
  render(<Overview />);
  expect(screen.getByText(/Check the target context path/)).toBeVisible();
  expect(document.body).not.toHaveTextContent("namespace_not_found");
});
test("overview receives focus after a target-scope reset", () => {
  view.current = { target: selectedTarget, scopeGeneration: 1 };
  render(<Overview />);
  expect(screen.getByRole("heading", { name: "Overview" })).toHaveFocus();
});
