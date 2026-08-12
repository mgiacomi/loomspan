export type PlanTaskSnapshot = {
  taskId: string;
  title: string;
  value: Record<string, unknown>;
};

export type PlanSnapshot = {
  planId?: string;
  capabilityName?: string;
  value: Record<string, unknown>;
  tasks: PlanTaskSnapshot[];
};

export type PlanFieldChange = {
  label: string;
  before: string;
  after: string;
};

export type PlanTaskChange = {
  taskId: string;
  title: string;
  intent: string;
  kind: "changed" | "added" | "removed";
  fields: PlanFieldChange[];
};

export type PlanComparison = {
  plan: PlanFieldChange[];
  tasks: PlanTaskChange[];
};

function optionalString(value: unknown): string | undefined {
  return typeof value === "string" && value.length > 0 ? value : undefined;
}

export function toPlanSnapshot(value: unknown): PlanSnapshot {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new Error("Plan data was not a JSON object.");
  }
  const plan = value as Record<string, unknown>;
  const tasks = Array.isArray(plan.tasks)
    ? plan.tasks.flatMap((candidate): PlanTaskSnapshot[] => {
      if (!candidate || typeof candidate !== "object" || Array.isArray(candidate)) return [];
      const task = candidate as Record<string, unknown>;
      const taskId = optionalString(task.taskId);
      if (!taskId) return [];
      return [{ taskId, title: optionalString(task.title) ?? taskId, value: task }];
    })
    : [];
  return {
    planId: optionalString(plan.planId),
    capabilityName: optionalString(plan.capabilityName),
    value: plan,
    tasks,
  };
}

function humanizeEnum(value: string): string {
  const words = value.toLowerCase().replaceAll("_", " ");
  return words.length === 0 ? words : words[0].toUpperCase() + words.slice(1);
}

function sameValue(left: unknown, right: unknown): boolean {
  return JSON.stringify(left) === JSON.stringify(right);
}

function displayValue(value: unknown, kind: "text" | "enum" | "boolean" | "list" = "text"): string {
  if (value === undefined || value === null || value === "") return "None";
  if (kind === "boolean" && typeof value === "boolean") return value ? "Yes" : "No";
  if (kind === "enum" && typeof value === "string") return humanizeEnum(value);
  if (kind === "list" && Array.isArray(value)) return value.length > 0 ? value.map(String).join(", ") : "None";
  return typeof value === "string" ? value : JSON.stringify(value);
}

function taskTitle(taskId: unknown, snapshots: PlanSnapshot[]): string {
  if (typeof taskId !== "string" || taskId.length === 0) return "None";
  for (const snapshot of snapshots) {
    const task = snapshot.tasks.find((candidate) => candidate.taskId === taskId);
    if (task) return task.title;
  }
  return taskId;
}

function fieldChange(label: string, before: unknown, after: unknown, kind: "text" | "enum" | "boolean" | "list" = "text"): PlanFieldChange | undefined {
  if (sameValue(before, after)) return undefined;
  return { label, before: displayValue(before, kind), after: displayValue(after, kind) };
}

export function comparePlans(previous: PlanSnapshot, current: PlanSnapshot): PlanComparison {
  const plan = [
    fieldChange("Plan status", previous.value.status, current.value.status, "enum"),
    sameValue(previous.value.activeTaskId, current.value.activeTaskId) ? undefined : {
      label: "Active task",
      before: taskTitle(previous.value.activeTaskId, [previous, current]),
      after: taskTitle(current.value.activeTaskId, [current, previous]),
    },
  ].filter((change): change is PlanFieldChange => Boolean(change));

  const previousTasks = new Map(previous.tasks.map((task) => [task.taskId, task]));
  const currentTasks = new Map(current.tasks.map((task) => [task.taskId, task]));
  const tasks: PlanTaskChange[] = [];

  for (const oldTask of previous.tasks) {
    if (!currentTasks.has(oldTask.taskId)) {
      tasks.push({ taskId: oldTask.taskId, title: oldTask.title, intent: optionalString(oldTask.value.intent) ?? oldTask.title, kind: "removed", fields: [] });
    }
  }
  for (const newTask of current.tasks) {
    const oldTask = previousTasks.get(newTask.taskId);
    if (!oldTask) {
      tasks.push({ taskId: newTask.taskId, title: newTask.title, intent: optionalString(newTask.value.intent) ?? newTask.title, kind: "added", fields: [] });
      continue;
    }
    const fields = [
      fieldChange("Status", oldTask.value.status, newTask.value.status, "enum"),
      fieldChange("Note", oldTask.value.note, newTask.value.note),
      fieldChange("Title", oldTask.value.title, newTask.value.title),
      fieldChange("Capability", oldTask.value.capabilityName, newTask.value.capabilityName),
      fieldChange("Intent", oldTask.value.intent, newTask.value.intent),
      fieldChange("Dependencies", oldTask.value.dependsOn, newTask.value.dependsOn, "list"),
      fieldChange("Expected outputs", oldTask.value.expectedOutputs, newTask.value.expectedOutputs, "list"),
      fieldChange("Auto-completable", oldTask.value.autoCompletable, newTask.value.autoCompletable, "boolean"),
    ].filter((change): change is PlanFieldChange => Boolean(change));
    if (fields.length > 0) tasks.push({ taskId: newTask.taskId, title: newTask.title, intent: optionalString(newTask.value.intent) ?? newTask.title, kind: "changed", fields });
  }

  return { plan, tasks };
}
