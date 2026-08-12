import { render, screen } from "@testing-library/react";
import { RouterProvider } from "react-router";
import { expect, test } from "vitest";
import { memoryRouter } from "./routes";

test("renders the console shell and runtime compatibility version", () => {
  render(<RouterProvider router={memoryRouter("/")} />);
  expect(screen.getByRole("heading", { name: "loomspan Console" })).toBeVisible();
  expect(screen.getByRole("heading", { name: "Instance Overview" })).toBeVisible();
  expect(screen.getByRole("navigation", { name: "Console" })).toBeVisible();
  expect(screen.getByRole("complementary", { name: "Current target and live context" })).toBeVisible();
  expect(screen.getByRole("link", { name: "Skills" })).toHaveAttribute("href", "/skills");
  expect(screen.getByRole("link", { name: "Active Executions" })).toHaveAttribute("href", "/active-executions");
  expect(screen.getByRole("link", { name: "Traces" })).toHaveAttribute("href", "/traces");
  expect(screen.getByTestId("console-version")).toHaveTextContent("0.1.0-SNAPSHOT");
});

test("does not retain the obsolete foundation deep route", () => {
  render(<RouterProvider router={memoryRouter("/foundation/deep-link")} />);
  expect(screen.getByRole("heading", { name: "This Console route does not exist" })).toBeVisible();
});

test("renders a safe not-found route as text", () => {
  const unsafe = `<img src=x onerror=alert("unsafe")>`;
  render(<RouterProvider router={memoryRouter(`/${encodeURIComponent(unsafe)}`)} />);
  expect(screen.getByRole("heading", { name: "This Console route does not exist" })).toBeVisible();
  expect(document.querySelector("img")).toBeNull();
});
