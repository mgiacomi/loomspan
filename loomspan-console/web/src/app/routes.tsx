import { createBrowserRouter, createMemoryRouter } from "react-router";
import { App, NotFound } from "./App";
import { Overview } from "../target/Overview";
import { ObservabilityOverview } from "../observability/Overview";
import { SkillCatalog } from "../observability/SkillCatalog";
import { SkillDetailView } from "../observability/SkillDetail";
import { ActiveExecutions } from "../observability/ActiveExecutions";
import { ActiveExecutionDetailView } from "../observability/ActiveExecutionDetail";
import { Traces } from "../observability/Traces";
import { TraceDetailView } from "../observability/TraceDetail";
import { TraceStorage } from "../observability/TraceStorage";
import { ImportedTraceView } from "../observability/ImportedTrace";

function definitions() {
  return [
    {
      path: "/",
      element: <App />,
      children: [
        { index: true, element: <ObservabilityOverview /> },
        { path: "target", element: <Overview /> },
        { path: "skills", element: <SkillCatalog /> },
        { path: "skills/:registeredName", element: <SkillDetailView /> },
        { path: "active-executions", element: <ActiveExecutions /> },
        { path: "active-executions/:sessionId", element: <ActiveExecutionDetailView /> },
        { path: "traces", element: <Traces /> },
		{ path: "traces/:traceId", element: <TraceDetailView /> },
		{ path: "traces/imported/:traceId", element: <ImportedTraceView /> },
        { path: "trace-storage", element: <TraceStorage /> },
        { path: "*", element: <NotFound /> },
      ],
    },
  ];
}

export function browserRouter() {
  return createBrowserRouter(definitions());
}

export function memoryRouter(path: string) {
  return createMemoryRouter(definitions(), { initialEntries: [path] });
}
