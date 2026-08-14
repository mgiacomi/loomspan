import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, test, vi } from "vitest";
import { MCPIntegration } from "./MCPIntegration";
import * as client from "../api/client";

vi.mock("../api/client", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../api/client")>();
  return { ...actual, getMCPStatus: vi.fn(), enableMCP: vi.fn(), revealMCP: vi.fn(), regenerateMCP: vi.fn(), disableMCP: vi.fn(), removeInvalidMCP: vi.fn() };
});

const disabled = { endpoint: "http://127.0.0.1:7345/mcp", state: "DISABLED" as const, setup: [{ client: "Codex", scope: "user", guidance: "Use an environment-backed Bearer header." }] };

beforeEach(() => {
  vi.mocked(client.getMCPStatus).mockResolvedValue(disabled);
  vi.mocked(client.enableMCP).mockResolvedValue({ ...disabled, state: "ENABLED", credential: "lsmcp_test_secret" });
});

test("shows disclosure and reveals a key only after explicit enable", async () => {
  const user = userEvent.setup();
  render(<MCPIntegration />);
  expect(screen.getByText(/diagnostic data to their configured model provider/i)).toBeVisible();
  await screen.findByText("Disabled");
  expect(screen.queryByText("lsmcp_test_secret")).toBeNull();
  await user.click(screen.getByRole("button", { name: "Enable MCP" }));
  expect(await screen.findByText("lsmcp_test_secret")).toBeVisible();
  expect(screen.getByText(/Never put this key in a repository, URL, or shell command/i)).toBeVisible();
});

test("invalid state requires a separate removal confirmation", async () => {
  vi.mocked(client.getMCPStatus).mockResolvedValue({ ...disabled, state: "DISABLED_INVALID", diagnostic: "canonical access key format is invalid" });
  const user = userEvent.setup();
  render(<MCPIntegration />);
  await screen.findByText("Disabled — invalid key file");
  await user.click(screen.getByRole("button", { name: "Remove invalid key file" }));
  expect(screen.getByRole("alertdialog", { name: "Confirm removal" })).toBeVisible();
  expect(client.removeInvalidMCP).not.toHaveBeenCalled();
});

test("clears a revealed key when hidden", async () => {
  const user = userEvent.setup();
  render(<MCPIntegration />);
  await screen.findByText("Disabled");
  await user.click(screen.getByRole("button", { name: "Enable MCP" }));
  await screen.findByText("lsmcp_test_secret");
  await user.click(screen.getByRole("button", { name: "Hide access key" }));
  await waitFor(() => expect(screen.queryByText("lsmcp_test_secret")).toBeNull());
});
