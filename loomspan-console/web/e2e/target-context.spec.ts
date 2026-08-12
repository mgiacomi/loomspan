import http from "node:http";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { test as consoleTest, expect } from "./fixtures/consoleProcess";

type TargetState = {
  instanceId: string;
  artifactBody: string;
};

const test = consoleTest.extend<{
  targetApplication: {
    origin: string;
    close(): Promise<void>;
    setState(s: Partial<TargetState>): void;
  };
}>({
  targetApplication: async ({}, use) => {
    const traceId = "trace-with-a-long-identifier-1234567890";
    const sessionId = "session-with-a-long-identifier-1234567890";
    const fixtureRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../../../loomspan-console-fixtures/traces");
    const artifactBody = fs.readFileSync(path.join(fixtureRoot, "single-attempt-success.ndjson"), "utf8").replaceAll("trace-single-attempt-success", traceId).replaceAll("session-single-attempt-success", sessionId);
    const traceMetadata = JSON.stringify({ traceId, sessionId, entrySkill: "CheckDns", outcome: "SUCCEEDED", finalizedAt: "2026-07-24T12:00:00Z", sizeBytes: new TextEncoder().encode(artifactBody).byteLength, persistencePolicy: "ALWAYS", applicationTraceExpiresAt: "2026-08-03T00:00:00Z" });
    let state: TargetState = {
      instanceId: "11111111-1111-4111-8111-111111111111",
      artifactBody,
    };
    const server = http.createServer((request, response) => {
      const path = new URL(request.url ?? "/", "http://127.0.0.1").pathname;
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
            activeExecutionCount: 0,
            catalogedTraceCount: 1,
            tracePersistencePolicy: "PERSISTENT",
            completionGraceTtl: "PT2M",
            traceCatalogMetadataTtl: "PT168H",
          }),
        );
        return;
      }
      if (path === "/_loomspan/observability/v1/traces") {
        response.writeHead(200, headers);
        response.end(JSON.stringify({ items: [JSON.parse(traceMetadata)], hasMore: false, nextCursor: null, observedAt: "2026-07-27T00:00:00Z" }));
        return;
      }
      // Trace detail endpoint for the cataloged trace.
      if (path === `/_loomspan/observability/v1/traces/${traceId}`) {
        response.writeHead(200, headers);
        response.end(traceMetadata);
        return;
      }
      // Artifact endpoint for the cataloged trace.
      if (path === `/_loomspan/observability/v1/traces/${traceId}/artifact`) {
        response.writeHead(200, {
          "Content-Type": "application/x-ndjson",
          "X-loomspan-Instance-Id": state.instanceId,
          "Content-Length": String(state.artifactBody.length),
          "Content-Disposition": 'attachment; filename="loomspan-trace-trace-with-a-long-identifier.ndjson"',
          "Cache-Control": "no-store",
        });
        response.end(state.artifactBody);
        return;
      }
      response.writeHead(404).end();
    });
    await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
    const address = server.address();
    if (!address || typeof address === "string") throw new Error("Target test server did not bind");
    const close = () => new Promise<void>((resolve, reject) =>
      server.close((error) => error ? reject(error) : resolve()),
    );
    try {
      await use({
        origin: `http://127.0.0.1:${address.port}`,
        close,
        setState: (s) => { state = { ...state, ...s }; },
      });
    } finally {
      await close();
    }
  },
});

test.use({ trace: "off", screenshot: "off", video: "off" });

// navigateToTraceDetail goes to the trace catalog, clicks the trace link (which
// includes the targetScopeId query parameter), and waits for the Trace Detail
// heading. Direct URL navigation doesn't work because TraceDetailView uses
// useScopeBoundRoute which requires the scope parameter.
async function navigateToTraceDetail(page: import("@playwright/test").Page, consoleProcess: { origin: string }, traceId: string) {
  await page.goto(`${consoleProcess.origin}/traces`);
  await expect(page.getByRole("heading", { name: "Trace Catalog" })).toBeVisible();
  await page.getByRole("link", { name: traceId }).click();
  await expect(page.getByRole("heading", { name: "Trace Detail" })).toBeVisible({ timeout: 10_000 });
}

// navigateToTraceStorage extracts the current target scope ID from the page URL
// and navigates to the trace storage page with the scope parameter.
async function navigateToTraceStorage(page: import("@playwright/test").Page, consoleProcess: { origin: string }) {
  // Try to extract the scope ID from the current URL first.
  let scopeId: string | null = null;
  try {
    const currentURL = new URL(page.url());
    scopeId = currentURL.searchParams.get("targetScopeId");
  } catch { /* ignore */ }

  // If we don't have the scope ID, navigate to the trace catalog and click a
  // trace link to get it into the URL.
  if (!scopeId) {
    await page.goto(`${consoleProcess.origin}/traces`);
    await expect(page.getByRole("heading", { name: "Trace Catalog" })).toBeVisible();
    const traceLink = page.locator("table a").first();
    await traceLink.click();
    await expect(page.getByRole("heading", { name: "Trace Detail" })).toBeVisible({ timeout: 10_000 });
    const currentURL = new URL(page.url());
    scopeId = currentURL.searchParams.get("targetScopeId");
    if (!scopeId) throw new Error("Could not extract targetScopeId from trace detail URL");
  }

  await page.goto(`${consoleProcess.origin}/trace-storage?targetScopeId=${encodeURIComponent(scopeId)}`);
  await expect(page.getByRole("heading", { name: "Trace Storage" })).toBeVisible({ timeout: 10_000 });
}

test("paired developer connects and refreshes independent target status", async ({
  page,
  consoleProcess,
  targetApplication,
}) => {
  const directApplicationRequests: string[] = [];
  page.on("request", (request) => {
    if (request.url().includes("/_loomspan/observability/")) {
      directApplicationRequests.push(request.url());
    }
  });
  await page.goto(consoleProcess.pairingUrl);
  await page.goto(`${consoleProcess.origin}/target`);
  await page.getByLabel("Target address").fill(targetApplication.origin);
  await page.getByLabel("Application key").fill("E2E_APPLICATION_KEY_12345678901234567890");
  await page.getByRole("button", { name: "Connect" }).click();
  await expect(page.getByRole("heading", { name: "Instance Overview" })).toBeFocused();
  const targetContext = page.getByRole("complementary", { name: "Current target and live context" });
  await expect(targetContext).toContainText("ConnectionREACHABLE");
  await expect(targetContext).toContainText("CompatibilityCOMPATIBLE");
  await expect(targetContext).toContainText("11111111-1111-4111-8111-111111111111");
  await expect(page.getByText(/Unencrypted/)).toBeVisible();
  await page.reload();
  await expect(targetContext).toContainText("ConnectionREACHABLE");
  await page.setViewportSize({ width: 320, height: 720 });
  await page.goto(`${consoleProcess.origin}/traces`);
  await expect(page.getByRole("heading", { name: "Trace Catalog" })).toBeVisible();
  await expect(page.getByRole("region", { name: "Trace catalog table" })).toBeVisible();
  expect(await page.evaluate(() =>
    document.documentElement.scrollWidth <= document.documentElement.clientWidth
  )).toBe(true);
  expect(directApplicationRequests).toEqual([]);
  expect(page.url()).not.toContain("E2E_APPLICATION_KEY");
  expect(await page.evaluate(() => JSON.stringify({ ...localStorage, ...sessionStorage }))).not.toContain(
    "E2E_APPLICATION_KEY",
  );
});

// WF-TC-ART-01: Target rotation clears the local artifact cache. After
// rotating to a new instance identity, Trace Storage is empty and the trace
// detail page shows the trace as "Not installed" again (the stale local handle
// from the prior scope is gone).
test("WF-TC-ART-01 target rotation clears local storage and stale handle is not reused", async ({
  page,
  consoleProcess,
  targetApplication,
}) => {
  await page.goto(consoleProcess.pairingUrl);
  await page.goto(`${consoleProcess.origin}/target`);
  await page.getByLabel("Target address").fill(targetApplication.origin);
  await page.getByLabel("Application key").fill("E2E_APPLICATION_KEY_12345678901234567890");
  await page.getByRole("button", { name: "Connect" }).click();
  await expect(page.getByRole("heading", { name: "Instance Overview" })).toBeFocused();

  // Acquire the artifact in the original scope.
  await navigateToTraceDetail(page, consoleProcess, "trace-with-a-long-identifier-1234567890");
  await expect(page.getByText("Not installed", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Acquire for analysis" }).click();
  await expect(page.getByText("Artifact acquired successfully.")).toBeVisible({ timeout: 15_000 });

  // Trace Storage must show the acquired artifact.
  await navigateToTraceStorage(page, consoleProcess);
  await expect(page.locator("table.storage-table")).toContainText("trace-with-a-long-identifier-1234567890");

  // Rotate the target by changing the instance identity. The target context
  // will detect the scope change automatically (same address/key, new instance
  // ID) and redirect to /. The old scope's artifacts are cleared.
  targetApplication.setState({ instanceId: "22222222-2222-4222-8222-222222222222" });
  await page.goto(`${consoleProcess.origin}/`);
  await expect(page.getByRole("complementary", { name: "Current target and live context" })).toContainText("22222222-2222-4222-8222-222222222222", { timeout: 15_000 });

  // Trace Storage must be empty in the new scope (TARGET_CHANGED cleared it).
  await navigateToTraceStorage(page, consoleProcess);
  await expect(page.getByText("No artifacts are currently stored.")).toBeVisible({ timeout: 15_000 });

  // The trace detail page must show "Not installed" again (the stale local
  // handle from the prior scope is gone and cannot be reused).
  await navigateToTraceDetail(page, consoleProcess, "trace-with-a-long-identifier-1234567890");
  await expect(page.getByText("Not installed", { exact: true })).toBeVisible();
});

// WF-TC-ART-02: After target rotation, acquiring the artifact in the new scope
// succeeds and the storage snapshot reflects only the new scope's entry. This
// proves the stale scope's handles are not adopted (ARTIFACT_EXPIRED for
// prior-scope handles) and the new scope starts fresh.
test("WF-TC-ART-02 acquisition after target rotation installs a fresh copy in the new scope", async ({
  page,
  consoleProcess,
  targetApplication,
}) => {
  await page.goto(consoleProcess.pairingUrl);
  await page.goto(`${consoleProcess.origin}/target`);
  await page.getByLabel("Target address").fill(targetApplication.origin);
  await page.getByLabel("Application key").fill("E2E_APPLICATION_KEY_12345678901234567890");
  await page.getByRole("button", { name: "Connect" }).click();
  await expect(page.getByRole("heading", { name: "Instance Overview" })).toBeFocused();

  // Acquire in the original scope.
  await navigateToTraceDetail(page, consoleProcess, "trace-with-a-long-identifier-1234567890");
  await page.getByRole("button", { name: "Acquire for analysis" }).click();
  await expect(page.getByText("Artifact acquired successfully.")).toBeVisible({ timeout: 15_000 });

  // Rotate the target. The target context will detect the scope change
  // automatically and redirect to /.
  targetApplication.setState({ instanceId: "22222222-2222-4222-8222-222222222222" });
  await page.goto(`${consoleProcess.origin}/`);
  await expect(page.getByRole("complementary", { name: "Current target and live context" })).toContainText("22222222-2222-4222-8222-222222222222", { timeout: 15_000 });

  // Trace Storage must be empty after rotation.
  await navigateToTraceStorage(page, consoleProcess);
  await expect(page.getByText("No artifacts are currently stored.")).toBeVisible({ timeout: 15_000 });

  // Acquire in the new scope.
  await navigateToTraceDetail(page, consoleProcess, "trace-with-a-long-identifier-1234567890");
  await expect(page.getByText("Not installed", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Acquire for analysis" }).click();
  await expect(page.getByText("Artifact acquired successfully.")).toBeVisible({ timeout: 15_000 });

  // Trace Storage must show exactly one entry (the new scope's copy).
  await navigateToTraceStorage(page, consoleProcess);
  await expect(page.locator("table.storage-table")).toContainText("trace-with-a-long-identifier-1234567890");
  const rows = page.locator("table.storage-table tbody tr");
  await expect(rows).toHaveCount(1);
});
