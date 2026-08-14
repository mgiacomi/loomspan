import { spawn, type ChildProcessWithoutNullStreams } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { test as base } from "@playwright/test";

type ConsoleProcess = {
  origin: string;
  pairingUrl: string;
  profileDirectory: string;
};

const currentDirectory = path.dirname(fileURLToPath(import.meta.url));
const executable = process.platform === "win32" ? "loomspan-console.exe" : "loomspan-console";
const binary = path.resolve(currentDirectory, "../../../build", executable);

async function waitForPairing(child: ChildProcessWithoutNullStreams): Promise<string> {
  return new Promise((resolve, reject) => {
    let buffered = "";
    let stderr = "";
    const timeout = setTimeout(() => reject(new Error("Console pairing URL was not available")), 15_000);
    child.stdout.on("data", (chunk: Buffer) => {
      buffered += chunk.toString("utf8");
      for (const line of buffered.split(/\r?\n/)) {
        if (line.startsWith("Pairing URL: ")) {
          clearTimeout(timeout);
          resolve(line.slice("Pairing URL: ".length));
          return;
        }
      }
    });
    child.stderr.on("data", (chunk: Buffer) => {
      stderr = (stderr + chunk.toString("utf8")).slice(-4096);
    });
    child.once("exit", (code) => {
      clearTimeout(timeout);
      const detail = stderr.trim() || "no stderr output";
      reject(new Error(`Console exited before pairing (code ${code ?? "unknown"}): ${detail}`));
    });
  });
}

export const test = base.extend<{ consoleProcess: ConsoleProcess }>({
  consoleProcess: async ({}, use) => {
    const root = fs.mkdtempSync(path.join(os.tmpdir(), "loomspan-console-e2e-"));
    const child = spawn(
      binary,
      [
        "--config",
        path.join(root, "profile", "config.yaml"),
        "--work-dir",
        path.join(root, "work"),
        "--listen",
        "127.0.0.1:0",
        "--no-open-browser",
      ],
      { stdio: ["pipe", "pipe", "pipe"], windowsHide: true },
    );
    try {
      const pairingUrl = await waitForPairing(child);
      const parsed = new URL(pairingUrl);
      await use({ origin: parsed.origin, pairingUrl, profileDirectory: path.join(root, "profile") });
    } finally {
      child.kill();
      await new Promise<void>((resolve) => {
        if (child.exitCode !== null) resolve();
        else child.once("exit", () => resolve());
      });
      fs.rmSync(root, { recursive: true, force: true });
    }
  },
});

export { expect } from "@playwright/test";
