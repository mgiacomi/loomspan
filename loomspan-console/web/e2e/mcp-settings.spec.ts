import fs from "node:fs";
import path from "node:path";
import { expect, test } from "./fixtures/consoleProcess";

test("paired developer manages MCP without browser persistence", async ({ page, consoleProcess }) => {
  await page.goto(consoleProcess.pairingUrl);
  await page.getByRole("link", { name: "Settings" }).click();
  await expect(page.getByRole("heading", { name: "MCP Integration" })).toBeVisible();
  await expect(page.getByText("Disabled", { exact: true })).toBeVisible();

  await page.getByRole("button", { name: "Enable MCP" }).click();
  const credential = page.locator(".credential-reveal code");
  await expect(credential).toHaveText(/^lsmcp_[A-Za-z0-9_-]{43}$/);
  const firstKey = await credential.textContent();
  expect(firstKey).not.toBeNull();
  expect(page.url()).not.toContain(firstKey!);
  expect(await page.evaluate(() => JSON.stringify({ local: localStorage, session: sessionStorage }))).not.toContain(firstKey!);
  expect(fs.readFileSync(path.join(consoleProcess.profileDirectory, "mcp-access-key"), "utf8").trim()).toBe(firstKey);

  await page.getByRole("button", { name: "Regenerate key" }).click();
  await page.getByRole("button", { name: "Confirm regenerate" }).click();
  await expect(credential).toHaveText(/^lsmcp_[A-Za-z0-9_-]{43}$/);
  expect(await credential.textContent()).not.toBe(firstKey);

  await page.getByRole("button", { name: "Hide access key" }).click();
  await expect(credential).toHaveCount(0);
  await page.reload();
  await expect(page.getByText("Enabled", { exact: true })).toBeVisible();
  expect(page.url()).not.toContain("lsmcp_");
  expect(await page.evaluate(() => JSON.stringify({ local: localStorage, session: sessionStorage }))).not.toContain("lsmcp_");

  await page.getByRole("button", { name: "Disable MCP" }).click();
  await page.getByRole("button", { name: "Confirm disable" }).click();
  await expect(page.getByText("Disabled", { exact: true })).toBeVisible();
  expect(() => fs.readFileSync(path.join(consoleProcess.profileDirectory, "mcp-access-key"))).toThrow();
});
