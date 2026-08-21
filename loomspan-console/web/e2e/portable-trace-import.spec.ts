import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { expect, test } from "./fixtures/consoleProcess";

const currentDirectory = path.dirname(fileURLToPath(import.meta.url));
const portableTrace = path.resolve(
  currentDirectory,
  "../../../loomspan-console-fixtures/traces/single-attempt-success.ndjson",
);

test("opens a same-version trace file without a configured target", async ({ page, consoleProcess }) => {
  await page.goto(consoleProcess.pairingUrl);
  await page.goto(`${consoleProcess.origin}/trace-storage`);
  await expect(page.getByRole("heading", { name: "Trace Storage" })).toBeVisible();

  const fileChooserPromise = page.waitForEvent("filechooser");
  await page.getByRole("button", { name: "Import Trace File" }).click();
  await (await fileChooserPromise).setFiles(portableTrace);

  await expect(page).toHaveURL(/\/traces\/imported\/trace-single-attempt-success$/);
  await expect(page.getByRole("heading", { name: "Imported trace" })).toBeVisible();
  await expect(page.getByText("Imported evidence", { exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Trace explorer" })).toBeVisible();
  await expect(page.getByText(/does not establish authenticity or provenance/i)).toBeVisible();
});

test("rejects a different-version trace without installing it", async ({ page, consoleProcess }) => {
  const mismatched = fs.readFileSync(portableTrace).toString("utf8")
    .replace('"consoleCompatibilityVersion":"0.1.0-SNAPSHOT"', '"consoleCompatibilityVersion":"9.9.9"');
  await page.goto(consoleProcess.pairingUrl);
  await page.goto(`${consoleProcess.origin}/trace-storage`);

  await page.getByLabel("Trace files").setInputFiles({
    name: "mismatched.ndjson",
    mimeType: "application/x-ndjson",
    buffer: Buffer.from(mismatched),
  });
  const results = page.getByRole("region", { name: "Trace import results" });
  await expect(results).toContainText("incompatible Loomspan version");
  await expect(results).toContainText("0.1.0-SNAPSHOT");
  await expect(results).toContainText("9.9.9");
  await expect(page.getByText("No artifacts are currently stored.")).toBeVisible();
});
