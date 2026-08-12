import { expect, test } from "vitest";
import { comparePlans, toPlanSnapshot } from "./planComparison";

test("recognizes added, removed, and structurally changed tasks by task ID", () => {
  const previous = toPlanSnapshot({
    planId: "plan-1",
    tasks: [
      { taskId: "removed", title: "Old task", intent: "Retire the old task.", status: "PENDING" },
      { taskId: "changed", title: "Changed task", intent: "Perform the changed task.", status: "PENDING", dependsOn: [] },
    ],
  });
  const current = toPlanSnapshot({
    planId: "plan-1",
    tasks: [
      { taskId: "changed", title: "Changed task", intent: "Perform the changed task.", status: "PENDING", dependsOn: ["new"] },
      { taskId: "new", title: "New task", intent: "Perform the new task.", status: "PENDING" },
    ],
  });

  const comparison = comparePlans(previous, current);

  expect(comparison.tasks).toEqual([
    { taskId: "removed", title: "Old task", intent: "Retire the old task.", kind: "removed", fields: [] },
    {
      taskId: "changed",
      title: "Changed task",
      intent: "Perform the changed task.",
      kind: "changed",
      fields: [{ label: "Dependencies", before: "None", after: "new" }],
    },
    { taskId: "new", title: "New task", intent: "Perform the new task.", kind: "added", fields: [] },
  ]);
});
