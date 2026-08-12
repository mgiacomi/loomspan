import http from "node:http";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { test as consoleTest, expect } from "./fixtures/consoleProcess";

const fixtureRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../../../loomspan-console-fixtures/traces");
const completedTraceArtifact = fs.readFileSync(path.join(fixtureRoot, "single-attempt-success.ndjson"), "utf8").replaceAll("trace-single-attempt-success", "trace-1").replaceAll("session-single-attempt-success", "session-1");
const completedTraceMetadata = JSON.stringify({ targetScopeId: "scope-1", traceId: "trace-1", sessionId: "session-1", entrySkill: "CheckDns", outcome: "SUCCEEDED", finalizedAt: "2026-07-24T12:00:00Z", sizeBytes: new TextEncoder().encode(completedTraceArtifact).byteLength, persistencePolicy: "ALWAYS", applicationTraceExpiresAt: "2026-08-03T00:00:00Z" });

type TargetState = {
  instanceId: string;
  activityEvents: string[];
  activeExecutions: string;
};

function activePage(sessionId: string, traceId: string): string {
  return JSON.stringify({
    items: [{
      sessionId,
      traceId,
      lastCanonicalSequence: 0,
      startedAt: "2026-07-27T00:00:00Z",
      updatedAt: "2026-07-27T00:00:00Z",
      elapsedMillis: 0,
      entrySkill: "test-skill",
      status: "RUNNING",
      phase: "EXECUTING",
      summary: "Execution is active",
      activePath: [],
      totalFrameDepth: 0,
      activePathTruncated: false,
      usage: {
        skillInvocations: 0, toolInvocations: 0, linterRetries: 0,
        modelCalls: 0, providerAttempts: 0, promptUnits: 0, completionUnits: 0, usageUnits: 0,
        exactModelResponses: 0, heuristicModelResponses: 0, unavailableModelResponses: 0,
      },
      configuredLimits: {
        maxSkillInvocations: 10, maxToolInvocations: 10, maxLinterRetries: 3,
        maxModelCalls: 10, maxProviderAttempts: 30, maxUsageUnits: 1000,
      },
    }],
    hasMore: false,
    nextCursor: null,
    observedAt: "2026-07-27T00:00:00Z",
    resumeCursor: "0",
  });
}

function makeTargetServer(initialState: TargetState) {
  let state = initialState;
  let activityClient: any = null;
  let activityRequest: any = null;
  let activityConnectionCount = 0;

  const server = http.createServer((request, response) => {
    const requestUrl = new URL(request.url ?? "/", "http://127.0.0.1");
    const path = requestUrl.pathname;

    if (request.method !== "GET") {
      response.writeHead(404).end();
      return;
    }

    if ((request.headers["x-loomspan-api-key"] ?? "").toString().length < 32) {
      response.writeHead(401, { "Content-Type": "application/json" });
      response.end('{"status":401,"code":"LOOMSPAN_API_KEY_REJECTED","message":"loomspan API key was rejected"}');
      return;
    }

    const headers = {
      "Content-Type": "application/json",
      "X-loomspan-Instance-Id": state.instanceId,
    };

    if (path === "/_loomspan/observability/v1/instance") {
      response.writeHead(200, headers);
      response.end(
        JSON.stringify({
          instanceId: state.instanceId,
          consoleCompatibilityVersion: "0.1.0-SNAPSHOT",
          observedAt: "2026-07-27T00:00:00Z",
          liveMonitoringAvailable: true,
          registeredSkillCount: 0,
          activeExecutionCount: 1,
          catalogedTraceCount: 0,
          tracePersistencePolicy: "PERSISTENT",
          completionGraceTtl: "PT2M",
          traceCatalogMetadataTtl: "PT168H",
        }),
      );
      return;
    }

    if (path === "/_loomspan/observability/v1/active-executions") {
      response.writeHead(200, headers);
      response.end(state.activeExecutions);
      return;
    }
    if (path.startsWith("/_loomspan/observability/v1/active-executions/")) {
      const sessionId = decodeURIComponent(path.slice(path.lastIndexOf("/") + 1));
      const page = JSON.parse(state.activeExecutions);
      const execution = page.items.find((item: { sessionId: string }) => item.sessionId === sessionId);
      if (!execution) {
        response.writeHead(404, { "Content-Type": "application/json" });
        response.end('{"status":404,"code":"NOT_FOUND","message":"not found"}');
        return;
      }
      response.writeHead(200, headers);
      response.end(JSON.stringify(execution));
      return;
    }

    if (path === "/_loomspan/observability/v1/activity") {
      activityConnectionCount += 1;
      const afterCursor = requestUrl.searchParams.get("afterCursor") ?? "0";
      response.writeHead(200, {
        "Content-Type": "text/event-stream",
        "X-loomspan-Instance-Id": state.instanceId,
        "Cache-Control": "no-store",
      });
      response.write(
        `event: handshake\ndata: {"instanceId":"${state.instanceId}","observedAt":"2026-07-27T00:00:00Z","afterCursor":"${afterCursor}"}\n\n`,
      );
      if (afterCursor === "0") {
        for (const evt of state.activityEvents) {
          response.write(evt);
        }
      }
      activityClient = response;
      activityRequest = request;
      request.on("close", () => {
        if (activityClient === response) {
          activityClient = null;
        }
        if (activityRequest === request) {
          activityRequest = null;
        }
      });
      return;
    }

    // Trace detail endpoint: serves metadata for trace-1 so the completed
    // execution can be inspected and acquired after TRACE_COMPLETED.
    if (path === "/_loomspan/observability/v1/traces/trace-1") {
      response.writeHead(200, headers);
      response.end(completedTraceMetadata);
      return;
    }

    // Artifact endpoint: serves a small NDJSON body for trace-1 so the
    // acquisition flow can install a local copy after completion. The body
    // must be exactly 128 bytes to match the sizeBytes in the trace metadata.
    if (path === "/_loomspan/observability/v1/traces/trace-1/artifact") {
      response.writeHead(200, {
        "Content-Type": "application/x-ndjson",
        "X-loomspan-Instance-Id": state.instanceId,
        "Content-Length": String(new TextEncoder().encode(completedTraceArtifact).byteLength),
        "Content-Disposition": 'attachment; filename="loomspan-trace-trace-1.ndjson"',
        "Cache-Control": "no-store",
      });
      response.end(completedTraceArtifact);
      return;
    }

    response.writeHead(404).end();
  });

  return {
    listen: () =>
      new Promise<{ origin: string; close: () => Promise<void>; setState: (s: TargetState) => void; pushEvent: (evt: string) => void; closeActivity: () => void; activityConnectionCount: () => number }>(
        (resolve, reject) => {
          server.listen(0, "127.0.0.1", () => {
            const address = server.address();
            if (!address || typeof address === "string") {
              reject(new Error("Target test server did not bind"));
              return;
            }
            resolve({
              origin: `http://127.0.0.1:${address.port}`,
              close: () =>
                new Promise<void>((res, rej) => {
                  activityClient?.end();
                  activityClient = null;
                  server.close((err) => (err ? rej(err) : res()));
                  // Forcefully destroy every established socket, including
                  // active SSE connections the Go console may reopen over a
                  // pooled keep-alive connection during teardown. Without this,
                  // server.close()'s callback can wait indefinitely for an
                  // immortal active connection and exceed Playwright's test
                  // timeout. closeAllConnections is safe to call right after
                  // server.close and is the recommended pattern.
                  server.closeAllConnections();
                }),
              setState: (s) => { state = s; },
              pushEvent: (evt) => {
                activityClient?.write(evt);
              },
              closeActivity: () => {
                activityClient?.end();
                activityClient = null;
              },
              activityConnectionCount: () => activityConnectionCount,
            });
          });
        },
      ),
  };
}

const test = consoleTest.extend<{
  targetApp: {
    origin: string;
    close: () => Promise<void>;
    setState: (s: TargetState) => void;
    pushEvent: (evt: string) => void;
    closeActivity: () => void;
    activityConnectionCount: () => number;
  };
}>({
  targetApp: async ({}, use) => {
    const server = makeTargetServer({
      instanceId: "11111111-1111-4111-8111-111111111111",
      activityEvents: [
        'id: 1\nevent: activity\ndata: {"instanceId":"11111111-1111-4111-8111-111111111111","cursor":"1","sessionId":"session-1","traceId":"trace-1","canonicalSequence":1,"timestamp":"2026-07-27T00:00:00Z","kind":"TRACE_STARTED","executionStatus":"RUNNING","summary":"Execution started","details":{}}\n\n',
        'id: 2\nevent: activity\ndata: {"instanceId":"11111111-1111-4111-8111-111111111111","cursor":"2","sessionId":"session-1","traceId":"trace-1","canonicalSequence":2,"timestamp":"2026-07-27T00:00:01Z","kind":"STEP_COMPLETED","executionStatus":"RUNNING","summary":"Step completed","details":{}}\n\n',
      ],
      activeExecutions: activePage("session-1", "trace-1"),
    });
    const handle = await server.listen();
    try {
      await use(handle);
    } finally {
      await handle.close();
    }
  },
});

test.use({ trace: "off", screenshot: "off", video: "off" });

test("WF-SLOW-EXECUTION (WF-SE) preserves selection while live activity advances", async ({
  page,
  consoleProcess,
  targetApp,
}) => {
  await page.goto(consoleProcess.pairingUrl);
  await page.goto(`${consoleProcess.origin}/target`);
  await page.getByLabel("Target address").fill(targetApp.origin);
  await page.getByLabel("Application key").fill("E2E_APPLICATION_KEY_12345678901234567890");
  await page.getByRole("button", { name: "Connect" }).click();
  await expect(page.getByRole("heading", { name: "Instance Overview" })).toBeFocused();

  await page.goto(`${consoleProcess.origin}/active-executions`);
  await page.getByRole("link", { name: "session-1" }).click();
  await expect(page.getByRole("heading", { name: "Active Execution Detail" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Live activity" })).toBeVisible();
  await expect(page.getByRole("log", { name: "Activity narrative" }).getByText("Execution started", { exact: true })).toBeVisible({ timeout: 10_000 });
  await expect(page.getByRole("log", { name: "Activity narrative" }).getByText("Step completed", { exact: true })).toBeVisible();

  targetApp.pushEvent(
    'id: 3\nevent: activity\ndata: {"instanceId":"11111111-1111-4111-8111-111111111111","cursor":"3","sessionId":"session-1","traceId":"trace-1","canonicalSequence":3,"timestamp":"2026-07-27T00:00:02Z","kind":"STEP_STARTED","executionStatus":"RUNNING","summary":"New step started","details":{}}\n\n',
  );
  await expect(page.locator(".activity-narrative-summary", { hasText: "New step started" })).toBeVisible({ timeout: 10_000 });
  await expect(page).toHaveURL(/\/active-executions\/session-1/);
  await expect(page.getByRole("log", { name: "Activity narrative" }).getByText("Execution started", { exact: true })).toBeVisible();
});

test("WF-SE terminal and observation-ended transitions remain in place", async ({
  page,
  consoleProcess,
  targetApp,
}) => {
  await page.goto(consoleProcess.pairingUrl);
  await page.goto(`${consoleProcess.origin}/target`);
  await page.getByLabel("Target address").fill(targetApp.origin);
  await page.getByLabel("Application key").fill("E2E_APPLICATION_KEY_12345678901234567890");
  await page.getByRole("button", { name: "Connect" }).click();
  await expect(page.getByRole("heading", { name: "Instance Overview" })).toBeFocused();

  await page.goto(`${consoleProcess.origin}/active-executions`);
  await page.getByRole("link", { name: "session-1" }).click();
  await expect(page.getByRole("heading", { name: "Live activity" })).toBeVisible();
  await expect(page.getByRole("log", { name: "Activity narrative" }).getByText("Execution started", { exact: true })).toBeVisible({ timeout: 10_000 });

  targetApp.pushEvent(
    'id: 10\nevent: activity\ndata: {"instanceId":"11111111-1111-4111-8111-111111111111","cursor":"10","sessionId":"session-1","traceId":"trace-1","canonicalSequence":10,"timestamp":"2026-07-27T00:00:10Z","kind":"TRACE_COMPLETED","executionStatus":"COMPLETED","summary":"Execution completed","details":{"outcome":"succeeded","applicationTraceAvailability":"AVAILABLE"}}\n\n',
  );
  await expect(page.getByRole("log", { name: "Activity narrative" }).getByText("Execution completed", { exact: true })).toBeVisible({ timeout: 10_000 });
  await expect(page.getByText(/Outcome:/)).toBeVisible();
  await expect(page).toHaveURL(/\/active-executions\/session-1/);
  await expect(page.getByRole("link", { name: "Inspect trace" })).toBeVisible();
});

test("WF-SE same-instance transient disconnect reconnects without discarding context", async ({
  page,
  consoleProcess,
  targetApp,
}) => {
  await page.goto(consoleProcess.pairingUrl);
  await page.goto(`${consoleProcess.origin}/target`);
  await page.getByLabel("Target address").fill(targetApp.origin);
  await page.getByLabel("Application key").fill("E2E_APPLICATION_KEY_12345678901234567890");
  await page.getByRole("button", { name: "Connect" }).click();
  await expect(page.getByRole("heading", { name: "Instance Overview" })).toBeFocused();
  await page.goto(`${consoleProcess.origin}/active-executions`);
  await page.getByRole("link", { name: "session-1" }).click();
  await expect(page.getByRole("log", { name: "Activity narrative" }).getByText("Execution started", { exact: true })).toBeVisible({ timeout: 10_000 });

  const connectionCount = targetApp.activityConnectionCount();
  targetApp.closeActivity();
  await expect.poll(targetApp.activityConnectionCount, { timeout: 15_000 }).toBeGreaterThan(connectionCount);
  targetApp.pushEvent(
    'id: 4\nevent: activity\ndata: {"instanceId":"11111111-1111-4111-8111-111111111111","cursor":"4","sessionId":"session-1","traceId":"trace-1","canonicalSequence":4,"timestamp":"2026-07-27T00:00:04Z","kind":"STEP_COMPLETED","executionStatus":"RUNNING","summary":"Step observed after reconnect","details":{}}\n\n',
  );
  await expect(page.getByText("Step observed after reconnect", { exact: true })).toBeVisible({ timeout: 10_000 });
  await expect(page.getByRole("log", { name: "Activity narrative" }).getByText("Execution started", { exact: true })).toBeVisible();
  await expect(page).toHaveURL(/\/active-executions\/session-1/);
});

test("WF-SE target change discards prior live state", async ({
  page,
  consoleProcess,
  targetApp,
}) => {
  await page.goto(consoleProcess.pairingUrl);
  await page.goto(`${consoleProcess.origin}/target`);
  await page.getByLabel("Target address").fill(targetApp.origin);
  await page.getByLabel("Application key").fill("E2E_APPLICATION_KEY_12345678901234567890");
  await page.getByRole("button", { name: "Connect" }).click();
  await expect(page.getByRole("heading", { name: "Instance Overview" })).toBeFocused();

  await page.goto(`${consoleProcess.origin}/`);
  await expect(page.getByRole("log", { name: "Activity narrative" }).getByText("Execution started", { exact: true })).toBeVisible({ timeout: 10_000 });

  targetApp.setState({
    instanceId: "22222222-2222-4222-8222-222222222222",
    activityEvents: [
      'id: 1\nevent: activity\ndata: {"instanceId":"22222222-2222-4222-8222-222222222222","cursor":"1","sessionId":"session-2","traceId":"trace-2","canonicalSequence":1,"timestamp":"2026-07-27T00:01:00Z","kind":"TRACE_STARTED","executionStatus":"RUNNING","summary":"New execution after restart","details":{}}\n\n',
    ],
    activeExecutions: activePage("session-2", "trace-2"),
  });
  targetApp.closeActivity();

  await page.goto(`${consoleProcess.origin}/target`);
  await expect(page.getByRole("heading", { name: "Instance Overview" })).toBeFocused();
  await expect(page.getByRole("complementary", { name: "Current target and live context" })).toContainText("22222222-2222-4222-8222-222222222222");

  await page.goto(`${consoleProcess.origin}/`);
  await expect(page.getByText("New execution after restart", { exact: true })).toBeVisible({ timeout: 15_000 });
  await expect(page.getByRole("log").getByText("session-1", { exact: true })).toHaveCount(0);
});

// WF-SE-ART: After a completed execution emits TRACE_COMPLETED with
// applicationTraceAvailability "AVAILABLE", the developer follows the
// "Inspect trace" link, deliberately acquires the artifact for analysis, and
// confirms it appears in Trace Storage. This proves the approved
// failed-completed-execution flow: completion does not auto-acquire; the
// developer must explicitly choose to install a local analysis copy.
test("WF-SE-ART completed execution requires deliberate acquisition before appearing in Trace Storage", async ({
  page,
  consoleProcess,
  targetApp,
}) => {
  await page.goto(consoleProcess.pairingUrl);
  await page.goto(`${consoleProcess.origin}/target`);
  await page.getByLabel("Target address").fill(targetApp.origin);
  await page.getByLabel("Application key").fill("E2E_APPLICATION_KEY_12345678901234567890");
  await page.getByRole("button", { name: "Connect" }).click();
  await expect(page.getByRole("heading", { name: "Instance Overview" })).toBeFocused();

  // Watch the live execution until TRACE_COMPLETED arrives.
  await page.goto(`${consoleProcess.origin}/active-executions`);
  await page.getByRole("link", { name: "session-1" }).click();
  await expect(page.getByRole("heading", { name: "Live activity" })).toBeVisible();
  await expect(page.getByRole("log", { name: "Activity narrative" }).getByText("Execution started", { exact: true })).toBeVisible({ timeout: 10_000 });

  targetApp.pushEvent(
    'id: 10\nevent: activity\ndata: {"instanceId":"11111111-1111-4111-8111-111111111111","cursor":"10","sessionId":"session-1","traceId":"trace-1","canonicalSequence":10,"timestamp":"2026-07-27T00:00:10Z","kind":"TRACE_COMPLETED","executionStatus":"COMPLETED","summary":"Execution completed","details":{"outcome":"succeeded","applicationTraceAvailability":"AVAILABLE"}}\n\n',
  );
  await expect(page.getByRole("log", { name: "Activity narrative" }).getByText("Execution completed", { exact: true })).toBeVisible({ timeout: 10_000 });
  await expect(page.getByRole("link", { name: "Inspect trace" })).toBeVisible();

  // Follow the "Inspect trace" link (which includes the targetScopeId
  // parameter) and deliberately acquire the artifact.
  await page.getByRole("link", { name: "Inspect trace" }).click();
  await expect(page.getByRole("heading", { name: "Trace Detail" })).toBeVisible({ timeout: 10_000 });
  await expect(page.getByText("Not installed", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Acquire for analysis" }).click();
  await expect(page.getByText("Artifact acquired successfully.")).toBeVisible({ timeout: 15_000 });

  // Completion must NOT auto-acquire: before the deliberate acquisition above,
  // Trace Storage was empty. Now the artifact must appear in Trace Storage.
  // Extract the scope ID from the current URL to navigate to trace-storage.
  const currentURL = new URL(page.url());
  const scopeId = currentURL.searchParams.get("targetScopeId");
  if (!scopeId) throw new Error("Could not extract targetScopeId from trace detail URL");
  await page.goto(`${consoleProcess.origin}/trace-storage?targetScopeId=${encodeURIComponent(scopeId)}`);
  await expect(page.getByRole("heading", { name: "Trace Storage" })).toBeVisible({ timeout: 10_000 });
  await expect(page.locator("table.storage-table")).toContainText("trace-1");
  await expect(page.locator("table.storage-table")).toContainText("SUCCEEDED");
  await expect(page.locator("table.storage-table")).toContainText("AVAILABLE");
});
