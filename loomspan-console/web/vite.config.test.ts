import { describe, expect, test } from "vitest";
import { createViteConfig, validateLoopbackOrigin } from "./vite.config";

describe("Vite configuration", () => {
  test("production config emits embedded assets without frontend version injection", () => {
    const config = createViteConfig({ command: "build", mode: "production", environment: {} });
    expect(config.build?.target).toBe("baseline-widely-available");
    expect(config.build?.manifest).toBe(true);
    expect(config.server).toBeUndefined();
  });

  test("development config binds loopback and proxies only console paths", () => {
    const config = createViteConfig({
      command: "serve",
      mode: "development",
      environment: { LOOMSPAN_CONSOLE_GO_ORIGIN: "http://127.0.0.1:7943" },
    });
    expect(config.server?.host).toBe("127.0.0.1");
    expect(config.server?.strictPort).toBe(true);
    expect(Object.keys(config.server?.proxy ?? {})).toEqual(["/api/console/"]);
  });

  test("activity streaming routes are proxied via /api/console/ prefix", () => {
    const config = createViteConfig({
      command: "serve",
      mode: "development",
      environment: { LOOMSPAN_CONSOLE_GO_ORIGIN: "http://127.0.0.1:7943" },
    });
    const proxyKeys = Object.keys(config.server?.proxy ?? {});
    expect(proxyKeys).toContain("/api/console/");
    expect(proxyKeys.some((k) => k.startsWith("/_loomspan/"))).toBe(false);
  });

  test("development config rejects non-loopback Go origin", () => {
    for (const value of ["http://0.0.0.0:7943", "http://example.com", "https://127.0.0.1:7943", "http://localhost:7943"]) {
      expect(() => validateLoopbackOrigin(value)).toThrow("loopback");
    }
  });
});
