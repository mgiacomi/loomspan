import http from "node:http";
import { test as consoleTest, expect } from "./fixtures/consoleProcess";

const test = consoleTest.extend<{ targetApplication: { origin: string; close(): Promise<void> } }>({
  targetApplication: async ({}, use) => {
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
        "X-loomspan-Instance-Id": "11111111-1111-4111-8111-111111111111",
      };
      if (path === "/_loomspan/observability/v1/instance") {
        response.writeHead(200, headers);
        response.end(
          '{"instanceId":"11111111-1111-4111-8111-111111111111","consoleCompatibilityVersion":"0.1.0-SNAPSHOT","observedAt":"2026-07-27T00:00:00Z","liveMonitoringAvailable":true,"registeredSkillCount":0,"activeExecutionCount":0,"catalogedTraceCount":0,"tracePersistencePolicy":"PERSISTENT","completionGraceTtl":"PT2M","traceCatalogMetadataTtl":"PT168H"}',
        );
        return;
      }
      if (path === "/_loomspan/observability/v1/active-executions") {
        response.writeHead(200, headers);
        response.end('{"items":[],"hasMore":false,"nextCursor":null,"observedAt":"2026-07-27T00:00:00Z","resumeCursor":"0"}');
        return;
      }
      if (path === "/_loomspan/observability/v1/activity") {
        response.writeHead(200, {
          "Content-Type": "text/event-stream",
          "X-loomspan-Instance-Id": "11111111-1111-4111-8111-111111111111",
          "Cache-Control": "no-store",
        });
        response.write("event: handshake\ndata: {\"instanceId\":\"11111111-1111-4111-8111-111111111111\",\"observedAt\":\"2026-07-27T00:00:00Z\",\"afterCursor\":\"0\"}\n\n");
        response.write("id: 7\nevent: activity\ndata: {\"instanceId\":\"11111111-1111-4111-8111-111111111111\",\"cursor\":\"7\",\"sessionId\":\"session-1\",\"traceId\":\"trace-1\",\"canonicalSequence\":7,\"timestamp\":\"2026-07-27T00:00:00Z\",\"kind\":\"TRACE_COMPLETED\",\"executionStatus\":\"COMPLETED\",\"summary\":\"Execution completed\",\"details\":{\"applicationTraceAvailability\":\"AVAILABLE\"}}\n\n");
        response.end();
        return;
      }
      response.writeHead(404).end();
    });
    await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
    const address = server.address();
    if (!address || typeof address === "string") throw new Error("Target test server did not bind");
    const close = () => new Promise<void>((resolve, reject) => {
      server.close((error) => error ? reject(error) : resolve());
      // Destroy every established socket, including active SSE connections the
      // Go console may reopen over a pooled keep-alive connection during
      // teardown. Without this, server.close()'s callback can wait
      // indefinitely for an immortal active connection and exceed Playwright's
      // test timeout. Safe to call right after server.close.
      server.closeAllConnections();
    });
    try {
      await use({ origin: `http://127.0.0.1:${address.port}`, close });
    } finally {
      await close();
    }
  },
});

test.use({ trace: "off", screenshot: "off", video: "off" });

test("paired developer sees live activity stream after connecting", async ({
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
  await expect(page.getByRole("complementary", { name: "Current target and live context" })).toContainText("ConnectionREACHABLE");
  await page.goto(`${consoleProcess.origin}/`);
  await expect(page.getByRole("heading", { name: "Live Activity" })).toBeVisible();
  await expect(page.getByRole("log", { name: "Activity narrative" }).getByText("Execution completed", { exact: true })).toBeVisible({ timeout: 10_000 });
});
