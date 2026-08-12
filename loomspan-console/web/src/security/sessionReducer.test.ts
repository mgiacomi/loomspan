import { expect, test } from "vitest";
import { sessionReducer } from "./sessionReducer";

const bootstrap = {
  processId: "process",
  consoleVersion: "0.1.0-SNAPSHOT",
  workspacePath: "workspace",
  tabId: "tab",
  csrfToken: "csrf",
  target: {
    unencrypted: false,
    status: {
      observedAt: "2026-07-27T00:00:00Z",
      targetSelection: "NONE" as const,
      targetConnection: "NOT_APPLICABLE" as const,
      targetAuthentication: "NOT_APPLICABLE" as const,
      javaGoCompatibility: "NOT_APPLICABLE" as const,
      runtimeIdentity: "NOT_APPLICABLE" as const,
      liveMonitoring: "NOT_APPLICABLE" as const,
    },
  },
};

test("session reducer represents loading paired and unpaired states", () => {
  expect(sessionReducer({ status: "loading" }, { type: "paired", bootstrap })).toEqual({
    status: "paired",
    bootstrap,
  });
  expect(sessionReducer({ status: "paired", bootstrap }, { type: "unpaired", message: "again" })).toEqual({
    status: "unpaired",
    message: "again",
  });
  expect(sessionReducer({ status: "unpaired" }, { type: "loading" })).toEqual({
    status: "loading",
  });
});
