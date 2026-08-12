import path from "node:path";
import { fileURLToPath } from "node:url";
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig, type UserConfig } from "vite";

type ConfigInput = {
  command: "build" | "serve";
  mode: string;
  environment: Record<string, string | undefined>;
};

const generatedDirectory = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "../internal/webassets/generated",
);

export function createViteConfig(input: ConfigInput): UserConfig {
  const production = input.command === "build";
  const config: UserConfig = {
    plugins: [react(), tailwindcss()],
    build: {
      target: "baseline-widely-available",
      outDir: generatedDirectory,
      emptyOutDir: false,
      manifest: true,
      rollupOptions: {
        output: {
          assetFileNames: "assets/[name]-[hash][extname]",
          chunkFileNames: "assets/[name]-[hash].js",
          entryFileNames: "assets/[name]-[hash].js",
        },
      },
    },
  };

  if (!production) {
    const target = validateLoopbackOrigin(
      input.environment.LOOMSPAN_CONSOLE_GO_ORIGIN ?? "http://127.0.0.1:7943",
    );
    config.server = {
      host: "127.0.0.1",
      port: 5173,
      strictPort: true,
      proxy: {
        "/api/console/": {
          target,
          changeOrigin: false,
        },
      },
    };
  }
  return config;
}

export function validateLoopbackOrigin(value: string): string {
  let url: URL;
  try {
    url = new URL(value);
  } catch {
    throw new Error("LOOMSPAN_CONSOLE_GO_ORIGIN must be an absolute HTTP loopback origin");
  }
  const loopback = url.hostname === "127.0.0.1" || url.hostname === "[::1]";
  if (url.protocol !== "http:" || !loopback || url.username || url.password || url.pathname !== "/" || url.search || url.hash) {
    throw new Error("LOOMSPAN_CONSOLE_GO_ORIGIN must be an HTTP origin using an explicit loopback IP");
  }
  return url.origin;
}

export default defineConfig(({ command, mode }) =>
  createViteConfig({ command, mode, environment: process.env }),
);
