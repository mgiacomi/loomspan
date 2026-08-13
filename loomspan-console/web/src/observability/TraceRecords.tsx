import { Fragment, useEffect, useState } from "react";
import type { KeyboardEvent } from "react";
import { getPayloadRange, getRawRecordRange, getTraceRecords } from "../api/client";
import type { TraceRange, TraceSource } from "../api/contracts";
import type { TraceFailure, TraceRecord } from "../api/contracts";
import type { TraceFrameFilter } from "../api/client";
import { comparePlans, toPlanSnapshot } from "./planComparison";
import type { PlanComparison, PlanSnapshot } from "./planComparison";

type Props = { traceId?: string; source?: TraceSource; records: TraceRecord[]; failures: TraceFailure[]; selectedRecordSequence?: number; selectedFailureId?: string; onSelectRecord: (record: TraceRecord) => void; onSelectFailure: (failureId: string) => void; onRelatedFrame?: (filter: TraceFrameFilter) => void; onPayload: (payloadId: string) => void };

type PlanCacheEntry = {
  loading: boolean;
  error?: string;
  json?: string;
  snapshot?: PlanSnapshot;
  comparisonLoading?: boolean;
  comparisonError?: string;
  comparisonReady?: boolean;
  comparison?: PlanComparison;
  previousSequence?: number;
};

type ModelDetail =
  | { kind: "request"; messages: { role: string; text: string }[] }
  | { kind: "response"; content: string };

type ModelCacheEntry = { loading: boolean; error?: string; detail?: ModelDetail };
type RawCacheEntry = { loading: boolean; error?: string; json?: string };
type StepDetail = { stepNumber: number; readyTasks: number; planStatus: string; skillName: string };
type StepCacheEntry = { loading: boolean; error?: string; detail?: StepDetail };
type StepActionKind = "proposed" | "validated" | "rejected";
type RecordSeverity = "normal" | "warning" | "error";

const warningRecordTypes = new Set([
  "MODEL_ATTEMPT_FAILED",
  "PLAN_VALIDATION_FAILED",
  "PLAN_RETRY_REQUESTED",
  "PLAN_QUALITY_WARNING",
  "EVIDENCE_VALIDATION_FAILED",
  "STEP_ACTION_REJECTED",
]);

const errorRecordTypes = new Set(["ERROR_RECORDED", "TOOL_CALL_FAILED"]);

function recordSeverity(record: TraceRecord, linkedFailure?: TraceFailure): RecordSeverity {
  if (linkedFailure || errorRecordTypes.has(record.type)) return "error";
  return warningRecordTypes.has(record.type) ? "warning" : "normal";
}
type StepActionDetail = {
  kind: StepActionKind;
  skillName: string;
  stepNumber: number;
  actionType?: string;
  taskId?: string;
  taskTitle?: string;
  toolName?: string;
  reason?: string;
  earlierRejectedAttempts?: number;
  exhausted?: boolean;
  rawResponse?: string;
  proposedSequence?: number;
};
type StepActionCacheEntry = { loading: boolean; error?: string; detail?: StepActionDetail };
type ToolResultDetail = {
  kind: "tool-result";
  capabilityName: string;
  taskId?: string;
  taskTitle?: string;
  eventId: string;
  note?: string;
  result: string;
};
type ToolInputDetail = {
  kind: "tool-input";
  capabilityName: string;
  taskId?: string;
  taskTitle?: string;
  unplanned: boolean;
  eventId: string;
  note?: string;
  arguments: string;
};
type StructuredOutputIssue = { path: string; message: string; canonicalField?: string };
type StructuredOutputDetail = {
  kind: "structured-output";
  skillName: string;
  status: string;
  attempt: number;
  retryCount: number;
  maxRetries: number;
  failureMode?: string;
  issues: StructuredOutputIssue[];
};
type StepCompletedDetail = {
  kind: "step-completed";
  skillName: string;
  stepNumber: number;
  actionType: string;
  status: "completed" | "failed";
  taskId?: string;
  taskTitle?: string;
  toolName?: string;
  resultPreview?: string;
  error?: string;
  relatedRecord?: TraceRecord;
};
type EvidenceDetail = {
  kind: "evidence";
  skillName: string;
  taskId?: string;
  taskTitle?: string;
  unplanned: boolean;
  availableSources: string[];
  sourceResult?: TraceRecord;
};
type CompletionUsage = {
  skillInvocations: number;
  toolInvocations: number;
  linterRetries: number;
  modelCalls: number;
  providerAttempts: number;
  promptUnits: number;
  completionUnits: number;
  totalUnits: number;
  exactModelResponses: number;
  heuristicModelResponses: number;
  unavailableModelResponses: number;
};
type CompletionDetail = {
  kind: "completion";
  outcome: "SUCCEEDED" | "FAILED" | "ABORTED";
  skillName?: string;
  objective?: string;
  entryPoint?: string;
  remainingFrames: number;
  persistencePolicy: "NEVER" | "ONERROR" | "ALWAYS";
  errored: boolean;
  terminalFailureId?: string;
  usage: CompletionUsage;
};
type RecordDetail = ToolInputDetail | ToolResultDetail | StructuredOutputDetail | StepCompletedDetail | EvidenceDetail | CompletionDetail;
type RecordDetailCacheEntry = { loading: boolean; error?: string; detail?: RecordDetail };

function decodeBytes(range: TraceRange): Uint8Array {
  if (range.encoding === "BASE64") {
    try {
      const binary = atob(range.content);
      return Uint8Array.from(binary, (character) => character.charCodeAt(0));
    } catch {
      throw new Error("Content contained invalid base64 data.");
    }
  }
  return new TextEncoder().encode(range.content);
}

function joinBytes(parts: Uint8Array[]): Uint8Array {
  const result = new Uint8Array(parts.reduce((length, part) => length + part.length, 0));
  let offset = 0;
  for (const part of parts) {
    result.set(part, offset);
    offset += part.length;
  }
  return result;
}

async function readCompleteRecord(traceId: string, sequence: number, source: TraceSource): Promise<string> {
  const parts: Uint8Array[] = [];
  let cursor: string | undefined;
  do {
    const range = await getRawRecordRange(traceId, sequence, cursor, source);
    parts.push(decodeBytes(range));
    if (!range.hasMore) break;
    if (!range.nextCursor || range.nextCursor === cursor) throw new Error("Content continuation was invalid.");
    cursor = range.nextCursor;
  } while (true);
  return new TextDecoder("utf-8", { fatal: true }).decode(joinBytes(parts)).trim();
}

async function readCompletePayload(traceId: string, payloadId: string, source: TraceSource): Promise<string> {
  const parts: Uint8Array[] = [];
  let cursor: string | undefined;
  do {
    const range = await getPayloadRange(traceId, payloadId, cursor, source);
    parts.push(decodeBytes(range));
    if (!range.hasMore) break;
    if (!range.nextCursor || range.nextCursor === cursor) throw new Error("Content continuation was invalid.");
    cursor = range.nextCursor;
  } while (true);
  return new TextDecoder("utf-8", { fatal: true }).decode(joinBytes(parts)).trim();
}

function parseJsonObject(raw: string, label: string): Record<string, unknown> {
  const value: unknown = JSON.parse(raw);
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new Error(`${label} did not contain a JSON object.`);
  }
  return value as Record<string, unknown>;
}

function recordData(rawRecord: string): unknown {
  const envelope = parseJsonObject(rawRecord, "Model record");
  if (!("data" in envelope)) throw new Error("Model record did not contain data.");
  return envelope.data;
}

function parseModelDetail(kind: "request" | "response", value: unknown): ModelDetail {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new Error(`Model ${kind} data was not a JSON object.`);
  }
  const fields = value as Record<string, unknown>;
  if (kind === "request") {
    if (!Array.isArray(fields.messages)) {
      throw new Error("Model request data did not contain messages.");
    }
    const messages = fields.messages.map((message, index) => {
      if (!message || typeof message !== "object" || Array.isArray(message)) {
        throw new Error(`Model request message ${index + 1} was not a JSON object.`);
      }
      const messageFields = message as Record<string, unknown>;
      if (typeof messageFields.messageType !== "string" || typeof messageFields.text !== "string") {
        throw new Error(`Model request message ${index + 1} did not contain messageType and text.`);
      }
      return { role: messageFields.messageType, text: messageFields.text };
    });
    return { kind, messages };
  }

  if (typeof fields.content !== "string") {
    throw new Error("Model response data did not contain text content.");
  }
  const trimmed = fields.content.trim();
  try {
    return { kind, content: JSON.stringify(JSON.parse(trimmed), null, 2) };
  } catch {
    return { kind, content: fields.content };
  }
}

function parseStepStartedDetail(rawRecord: string, route: string): StepDetail {
  const envelope = parseJsonObject(rawRecord, "Step record");
  if (!envelope.metadata || typeof envelope.metadata !== "object" || Array.isArray(envelope.metadata)) {
    throw new Error("Step record did not contain metadata.");
  }
  if (!envelope.data || typeof envelope.data !== "object" || Array.isArray(envelope.data)) {
    throw new Error("Step record did not contain data.");
  }
  const metadata = envelope.metadata as Record<string, unknown>;
  const data = envelope.data as Record<string, unknown>;
  if (!Number.isSafeInteger(metadata.stepNumber) || !Number.isSafeInteger(metadata.readyTasks) ||
      typeof metadata.stepNumber !== "number" || metadata.stepNumber < 1 ||
      typeof metadata.readyTasks !== "number" || metadata.readyTasks < 0 ||
      typeof data.planStatus !== "string" || data.planStatus.length === 0) {
    throw new Error("Step record contained invalid step facts.");
  }
  const separator = route.indexOf("#step-");
  if (separator <= 0) throw new Error("Step record route did not identify its owning skill.");
  return {
    stepNumber: metadata.stepNumber,
    readyTasks: metadata.readyTasks,
    planStatus: data.planStatus,
    skillName: route.slice(0, separator),
  };
}

function parseStepRoute(route: string): { skillName: string; stepNumber: number } {
  const match = /^(.*)#step-(\d+)$/.exec(route);
  if (!match || match[1].length === 0) throw new Error("Step action route did not identify its owning skill and step.");
  const stepNumber = Number(match[2]);
  if (!Number.isSafeInteger(stepNumber) || stepNumber < 1) throw new Error("Step action route contained an invalid step number.");
  return { skillName: match[1], stepNumber };
}

function optionalNonemptyString(value: unknown): string | undefined {
  return typeof value === "string" && value.length > 0 ? value : undefined;
}

function requiredNonnegativeInteger(value: unknown, label: string): number {
  if (typeof value !== "number" || !Number.isSafeInteger(value) || value < 0) throw new Error(`${label} was invalid.`);
  return value;
}

function prettyValue(value: unknown): string {
  if (typeof value === "string") {
    try {
      return JSON.stringify(JSON.parse(value), null, 2);
    } catch {
      return value;
    }
  }
  return JSON.stringify(value, null, 2);
}

function recordParts(rawRecord: string, label: string): { metadata: Record<string, unknown>; data: Record<string, unknown> } {
  const envelope = parseJsonObject(rawRecord, label);
  if (!envelope.metadata || typeof envelope.metadata !== "object" || Array.isArray(envelope.metadata)) {
    throw new Error(`${label} did not contain metadata.`);
  }
  if (!envelope.data || typeof envelope.data !== "object" || Array.isArray(envelope.data)) {
    throw new Error(`${label} did not contain data.`);
  }
  return {
    metadata: envelope.metadata as Record<string, unknown>,
    data: envelope.data as Record<string, unknown>,
  };
}

function parseStepActionDetail(rawRecord: string, route: string, kind: StepActionKind): StepActionDetail {
  const envelope = parseJsonObject(rawRecord, "Step action record");
  if (!envelope.metadata || typeof envelope.metadata !== "object" || Array.isArray(envelope.metadata)) {
    throw new Error("Step action record did not contain metadata.");
  }
  if (!envelope.data || typeof envelope.data !== "object" || Array.isArray(envelope.data)) {
    throw new Error("Step action record did not contain data.");
  }
  const metadata = envelope.metadata as Record<string, unknown>;
  const data = envelope.data as Record<string, unknown>;
  const routeDetail = parseStepRoute(route);

  if (kind === "proposed") {
    if (typeof metadata.stepAction !== "string" || metadata.stepAction.length === 0 ||
        typeof metadata.taskId !== "string" || typeof metadata.toolName !== "string") {
      throw new Error("Proposed action record contained invalid action facts.");
    }
    return {
      kind,
      ...routeDetail,
      actionType: metadata.stepAction,
      taskId: optionalNonemptyString(metadata.taskId),
      toolName: optionalNonemptyString(metadata.toolName),
    };
  }

  if (kind === "validated") {
    if (typeof metadata.stepAction !== "string" || metadata.stepAction.length === 0) {
      throw new Error("Validated action record did not identify the accepted action type.");
    }
    return { kind, ...routeDetail, actionType: metadata.stepAction };
  }

  if (typeof metadata.reason !== "string" || metadata.reason.length === 0) {
    throw new Error("Rejected action record did not contain a rejection reason.");
  }
  if (metadata.retry !== undefined && (!Number.isSafeInteger(metadata.retry) || typeof metadata.retry !== "number" || metadata.retry < 0)) {
    throw new Error("Rejected action record contained an invalid retry count.");
  }
  if (metadata.exhausted !== undefined && typeof metadata.exhausted !== "boolean") {
    throw new Error("Rejected action record contained an invalid exhausted value.");
  }
  if (data.stepAction !== undefined && (typeof data.stepAction !== "string" || data.stepAction.length === 0)) {
    throw new Error("Rejected action record contained an invalid action type.");
  }
  if (data.rawResponse !== undefined && typeof data.rawResponse !== "string") {
    throw new Error("Rejected action record contained an invalid response excerpt.");
  }
  return {
    kind,
    ...routeDetail,
    actionType: optionalNonemptyString(data.stepAction),
    reason: metadata.reason,
    earlierRejectedAttempts: metadata.retry as number | undefined,
    exhausted: metadata.exhausted as boolean | undefined,
    rawResponse: optionalNonemptyString(data.rawResponse),
  };
}

function ModelDetailView({ detail }: { detail: ModelDetail }) {
  if (detail.kind === "response") return <pre>{detail.content}</pre>;
  return <div className="trace-model-messages">
    {detail.messages.map((message, index) => <section className="trace-model-message" key={`${message.role}-${index}`}>
      <h5>{message.role.toLowerCase().replaceAll("_", " ")}</h5>
      <pre>{message.text}</pre>
    </section>)}
  </div>;
}

function parsePlanRecord(rawRecord: string): PlanSnapshot {
  const envelope = parseJsonObject(rawRecord, "Plan record");
  const plan = envelope.data;
  if (plan === undefined || plan === null) {
    throw new Error("Plan record did not contain plan data.");
  }
  return toPlanSnapshot(plan);
}

async function readPlanSnapshot(traceId: string, record: TraceRecord, source: TraceSource): Promise<PlanSnapshot> {
  if (record.payloadId) {
    const raw = await readCompletePayload(traceId, record.payloadId, source);
    return toPlanSnapshot(parseJsonObject(raw, "Plan payload"));
  }
  return parsePlanRecord(await readCompleteRecord(traceId, record.sequence, source));
}

async function findPreviousPlan(traceId: string, sequence: number, planId: string, source: TraceSource): Promise<{ sequence: number; snapshot: PlanSnapshot } | undefined> {
  if (sequence <= 1) return undefined;
  const candidates: TraceRecord[] = [];
  let cursor: string | undefined;
  do {
    const page = await getTraceRecords(traceId, cursor, {
      types: ["PLAN_CREATED", "PLAN_UPDATED"],
      maxSequence: sequence - 1,
    }, source);
    candidates.push(...page.items);
    if (!page.hasMore) break;
    if (!page.nextCursor || page.nextCursor === cursor) throw new Error("Plan history continuation was invalid.");
    cursor = page.nextCursor;
  } while (true);

  candidates.sort((left, right) => right.sequence - left.sequence);
  for (const candidate of candidates) {
    let snapshot: PlanSnapshot;
    try {
      snapshot = await readPlanSnapshot(traceId, candidate, source);
    } catch {
      continue;
    }
    if (snapshot.planId === planId) return { sequence: candidate.sequence, snapshot };
  }
  return undefined;
}

async function findProposedAction(traceId: string, record: TraceRecord, source: TraceSource): Promise<{ sequence: number; detail: StepActionDetail } | undefined> {
  if (!record.frameId || record.sequence <= 1) return undefined;
  const candidates: TraceRecord[] = [];
  let cursor: string | undefined;
  do {
    const page = await getTraceRecords(traceId, cursor, {
      types: ["STEP_ACTION_PROPOSED"],
      frameId: record.frameId,
      maxSequence: record.sequence - 1,
    }, source);
    candidates.push(...page.items);
    if (!page.hasMore) break;
    if (!page.nextCursor || page.nextCursor === cursor) throw new Error("Proposed action continuation was invalid.");
    cursor = page.nextCursor;
  } while (true);
  const candidate = candidates.sort((left, right) => right.sequence - left.sequence)[0];
  if (!candidate) return undefined;
  const detail = parseStepActionDetail(await readCompleteRecord(traceId, candidate.sequence, source), candidate.route, "proposed");
  return { sequence: candidate.sequence, detail };
}

async function findTaskTitle(traceId: string, sequence: number, skillName: string, taskId: string, source: TraceSource): Promise<string | undefined> {
  if (sequence <= 1) return undefined;
  const candidates: TraceRecord[] = [];
  let cursor: string | undefined;
  do {
    const page = await getTraceRecords(traceId, cursor, {
      types: ["PLAN_CREATED", "PLAN_UPDATED"],
      maxSequence: sequence - 1,
    }, source);
    candidates.push(...page.items);
    if (!page.hasMore) break;
    if (!page.nextCursor || page.nextCursor === cursor) throw new Error("Plan history continuation was invalid.");
    cursor = page.nextCursor;
  } while (true);
  candidates.sort((left, right) => right.sequence - left.sequence);
  for (const candidate of candidates) {
    try {
      const snapshot = await readPlanSnapshot(traceId, candidate, source);
      if (snapshot.capabilityName !== skillName) continue;
      const task = snapshot.tasks.find((current) => current.taskId === taskId);
      if (task) return task.title;
    } catch {
      // A different plan record may still contain the authoritative task facts.
    }
  }
  return undefined;
}

async function readStepActionDetail(traceId: string, record: TraceRecord, kind: StepActionKind, source: TraceSource): Promise<StepActionDetail> {
  let detail = parseStepActionDetail(await readCompleteRecord(traceId, record.sequence, source), record.route, kind);
  if (kind !== "proposed") {
    const proposed = await findProposedAction(traceId, record, source);
    if (proposed && proposed.detail.actionType === detail.actionType) {
      detail = {
        ...detail,
        taskId: proposed.detail.taskId,
        toolName: proposed.detail.toolName,
        proposedSequence: proposed.sequence,
      };
    }
  }
  if (detail.taskId) {
    detail = { ...detail, taskTitle: await findTaskTitle(traceId, record.sequence, detail.skillName, detail.taskId, source) };
  }
  return detail;
}

async function readToolResultDetail(traceId: string, record: TraceRecord, source: TraceSource): Promise<ToolResultDetail> {
  const { metadata, data } = recordParts(await readCompleteRecord(traceId, record.sequence, source), "Tool result record");
  const capabilityName = optionalNonemptyString(metadata.capabilityName) ?? optionalNonemptyString(data.capabilityName);
  const taskId = optionalNonemptyString(metadata.linkedTaskId) ?? optionalNonemptyString(data.linkedTaskId);
  const eventId = optionalNonemptyString(data.eventId);
  if (!capabilityName || !eventId) throw new Error("Tool result record contained invalid identifying facts.");
  if (!data.details || typeof data.details !== "object" || Array.isArray(data.details)) {
    throw new Error("Tool result record did not contain result details.");
  }
  const details = data.details as Record<string, unknown>;
  if (!("result" in details)) throw new Error("Tool result details did not contain a result.");
  const result = prettyValue(details.result);
  let owningSkill: string | undefined;
  if (taskId && record.parentFrameId) {
    const page = await getTraceRecords(traceId, undefined, {
      types: ["STEP_STARTED"],
      frameId: record.parentFrameId,
      maxSequence: record.sequence - 1,
    }, source);
    const step = [...page.items].sort((left, right) => right.sequence - left.sequence)[0];
    if (step) owningSkill = parseStepRoute(step.route).skillName;
  }
  return {
    kind: "tool-result",
    capabilityName,
    taskId,
    taskTitle: taskId && owningSkill ? await findTaskTitle(traceId, record.sequence, owningSkill, taskId, source) : undefined,
    eventId,
    note: optionalNonemptyString(data.note),
    result,
  };
}

async function findOwningSkill(traceId: string, record: TraceRecord, source: TraceSource): Promise<string | undefined> {
  if (!record.parentFrameId || record.sequence <= 1) return undefined;
  let cursor: string | undefined;
  do {
    const page = await getTraceRecords(traceId, cursor, {
      types: ["STEP_STARTED"],
      frameId: record.parentFrameId,
      maxSequence: record.sequence - 1,
    }, source);
    const step = [...page.items].sort((left, right) => right.sequence - left.sequence)[0];
    if (step) return parseStepRoute(step.route).skillName;
    if (!page.hasMore) break;
    if (!page.nextCursor || page.nextCursor === cursor) throw new Error("Owning step continuation was invalid.");
    cursor = page.nextCursor;
  } while (true);

  cursor = undefined;
  do {
    const page = await getTraceRecords(traceId, cursor, {
      types: ["FRAME_OPENED"],
      frameId: record.parentFrameId,
      maxSequence: record.sequence - 1,
    }, source);
    const parent = [...page.items].sort((left, right) => right.sequence - left.sequence)[0];
    if (parent) {
      if (parent.frameType === "STEP_EXECUTION") return parseStepRoute(parent.route).skillName;
      if (parent.frameType === "MODEL_CALL") {
        const match = /^(.*)#(?:mission-model|planning-model|step-\d+-model)$/.exec(parent.route);
        if (match?.[1]) return match[1];
      }
      if ((parent.frameType === "ROOT_MISSION" || parent.frameType === "SKILL_EXECUTION") && parent.route.length > 0) {
        return parent.route;
      }
      return undefined;
    }
    if (!page.hasMore) break;
    if (!page.nextCursor || page.nextCursor === cursor) throw new Error("Owning frame continuation was invalid.");
    cursor = page.nextCursor;
  } while (true);
  return undefined;
}

async function readToolInputDetail(traceId: string, record: TraceRecord, source: TraceSource): Promise<ToolInputDetail> {
  const { metadata, data } = recordParts(await readCompleteRecord(traceId, record.sequence, source), "Tool input record");
  const capabilityName = optionalNonemptyString(metadata.capabilityName) ?? optionalNonemptyString(data.capabilityName);
  const taskId = optionalNonemptyString(metadata.linkedTaskId) ?? optionalNonemptyString(data.linkedTaskId);
  const eventId = optionalNonemptyString(data.eventId);
  if (!capabilityName || !eventId) throw new Error("Tool input record contained invalid identifying facts.");
  if (metadata.unplanned !== undefined && metadata.unplanned !== true) {
    throw new Error("Tool input record contained an invalid unplanned marker.");
  }
  const unplanned = metadata.unplanned === true;
  if (unplanned && taskId) throw new Error("Unplanned tool input unexpectedly identified a plan task.");
  if (!unplanned && !taskId) throw new Error("Planned tool input did not identify a plan task.");
  if (!data.details || typeof data.details !== "object" || Array.isArray(data.details)) {
    throw new Error("Tool input record did not contain input details.");
  }
  const details = data.details as Record<string, unknown>;
  if (!("arguments" in details)) throw new Error("Tool input details did not contain arguments.");
  const owningSkill = taskId ? await findOwningSkill(traceId, record, source) : undefined;
  return {
    kind: "tool-input",
    capabilityName,
    taskId,
    taskTitle: taskId && owningSkill ? await findTaskTitle(traceId, record.sequence, owningSkill, taskId, source) : undefined,
    unplanned,
    eventId,
    note: optionalNonemptyString(data.note),
    arguments: prettyValue(details.arguments),
  };
}

function parseStructuredOutputDetail(rawRecord: string): StructuredOutputDetail {
  const { metadata, data } = recordParts(rawRecord, "Structured output record");
  const skillName = optionalNonemptyString(data.skillName) ?? optionalNonemptyString(metadata.skillName);
  const status = optionalNonemptyString(data.status) ?? optionalNonemptyString(metadata.status);
  if (!skillName || !status || !["PASSED", "RETRYING", "EXHAUSTED"].includes(status)) {
    throw new Error("Structured output record contained invalid validation facts.");
  }
  if (!Array.isArray(data.issues)) throw new Error("Structured output record did not contain validation issues.");
  const issues = data.issues.map((candidate, index): StructuredOutputIssue => {
    if (!candidate || typeof candidate !== "object" || Array.isArray(candidate)) {
      throw new Error(`Validation issue ${index + 1} was not a JSON object.`);
    }
    const issue = candidate as Record<string, unknown>;
    if (typeof issue.path !== "string" || typeof issue.message !== "string") {
      throw new Error(`Validation issue ${index + 1} did not contain a path and message.`);
    }
    if (issue.canonicalField !== undefined && issue.canonicalField !== null && typeof issue.canonicalField !== "string") {
      throw new Error(`Validation issue ${index + 1} contained an invalid canonical field.`);
    }
    return { path: issue.path, message: issue.message, canonicalField: optionalNonemptyString(issue.canonicalField) };
  });
  if (status === "PASSED" && issues.length > 0) throw new Error("Passed output validation unexpectedly contained issues.");
  const attempt = requiredNonnegativeInteger(data.attempt, "Validation attempt");
  if (attempt < 1) throw new Error("Validation attempt was invalid.");
  return {
    kind: "structured-output",
    skillName,
    status,
    attempt,
    retryCount: requiredNonnegativeInteger(data.retryCount, "Validation retry count"),
    maxRetries: requiredNonnegativeInteger(data.maxRetries, "Maximum validation retries"),
    failureMode: optionalNonemptyString(data.failureMode),
    issues,
  };
}

async function readStepCompletedDetail(traceId: string, record: TraceRecord, source: TraceSource): Promise<StepCompletedDetail> {
  const { metadata, data } = recordParts(await readCompleteRecord(traceId, record.sequence, source), "Completed step record");
  const route = parseStepRoute(record.route);
  const stepNumber = requiredNonnegativeInteger(metadata.stepNumber, "Completed step number");
  const actionType = optionalNonemptyString(metadata.stepAction);
  if (stepNumber < 1 || stepNumber !== route.stepNumber || !actionType) {
    throw new Error("Completed step record contained invalid step facts.");
  }
  const recordedStatus = metadata.status === undefined ? "completed" : metadata.status;
  if (recordedStatus !== "completed" && recordedStatus !== "failed") throw new Error("Completed step record contained an invalid status.");
  const taskId = optionalNonemptyString(metadata.taskId);
  const toolName = optionalNonemptyString(metadata.toolName);
  const resultPreview = optionalNonemptyString(data.resultPreview);
  const error = optionalNonemptyString(data.error);
  let relatedRecord: TraceRecord | undefined;
  if (actionType === "CALL_TOOL") {
    const candidates: TraceRecord[] = [];
    let cursor: string | undefined;
    do {
      const page = await getTraceRecords(traceId, cursor, {
        types: ["TOOL_CALL_COMPLETED", "TOOL_CALL_FAILED"],
        maxSequence: record.sequence - 1,
      });
      candidates.push(...page.items.filter((candidate) => candidate.parentFrameId === record.frameId));
      if (!page.hasMore) break;
      if (!page.nextCursor || page.nextCursor === cursor) throw new Error("Related tool result continuation was invalid.");
      cursor = page.nextCursor;
    } while (true);
    relatedRecord = candidates.sort((left, right) => right.sequence - left.sequence)[0];
  } else if (actionType === "FINAL_RESPONSE") {
    const page = await getTraceRecords(traceId, undefined, {
      types: ["MODEL_RESPONSE_RECEIVED"],
      route: `${record.route}-model`,
      maxSequence: record.sequence - 1,
    }, source);
    relatedRecord = [...page.items].sort((left, right) => right.sequence - left.sequence)[0];
  }
  return {
    kind: "step-completed",
    skillName: route.skillName,
    stepNumber,
    actionType,
    status: recordedStatus,
    taskId,
    taskTitle: taskId ? await findTaskTitle(traceId, record.sequence, route.skillName, taskId, source) : undefined,
    toolName,
    resultPreview,
    error,
    relatedRecord,
  };
}

async function readEvidenceDetail(traceId: string, record: TraceRecord, source: TraceSource): Promise<EvidenceDetail> {
  const { metadata, data } = recordParts(await readCompleteRecord(traceId, record.sequence, source), "Evidence record");
  const skillName = optionalNonemptyString(data.successfulSkill);
  const capabilityName = optionalNonemptyString(metadata.capabilityName);
  const taskId = optionalNonemptyString(metadata.linkedTaskId);
  if (!skillName || !capabilityName || skillName !== capabilityName || typeof metadata.unplanned !== "boolean") {
    throw new Error("Evidence record contained invalid source facts.");
  }
  if (!Array.isArray(data.successfulDirectSkills)) {
    throw new Error("Evidence record did not contain available evidence sources.");
  }
  const availableSources = data.successfulDirectSkills.map((source, index) => {
    if (typeof source !== "string" || source.length === 0) throw new Error(`Evidence source ${index + 1} was invalid.`);
    return source;
  });
  if (!availableSources.includes(skillName)) {
    throw new Error("Evidence record's successful source was missing from the available sources.");
  }
  if (new Set(availableSources).size !== availableSources.length) {
    throw new Error("Evidence record contained duplicate available sources.");
  }
  if (metadata.unplanned && taskId) throw new Error("Unplanned evidence unexpectedly identified a plan task.");
  if (!metadata.unplanned && !taskId) throw new Error("Planned evidence did not identify a plan task.");

  const candidates: TraceRecord[] = [];
  let cursor: string | undefined;
  do {
    const page = await getTraceRecords(traceId, cursor, {
      types: ["TOOL_CALL_COMPLETED"],
      maxSequence: record.sequence - 1,
    }, source);
    candidates.push(...page.items.filter((candidate) => candidate.parentFrameId === record.frameId && candidate.route === capabilityName));
    if (!page.hasMore) break;
    if (!page.nextCursor || page.nextCursor === cursor) throw new Error("Source result continuation was invalid.");
    cursor = page.nextCursor;
  } while (true);

  return {
    kind: "evidence",
    skillName,
    taskId,
    taskTitle: taskId ? await findTaskTitle(traceId, record.sequence, parseStepRoute(record.route).skillName, taskId, source) : undefined,
    unplanned: metadata.unplanned,
    availableSources,
    sourceResult: candidates.sort((left, right) => right.sequence - left.sequence)[0],
  };
}

function parseCompletionDetail(rawRecord: string): CompletionDetail {
  const envelope = parseJsonObject(rawRecord, "Completion record");
  if (!envelope.metadata || typeof envelope.metadata !== "object" || Array.isArray(envelope.metadata)) {
    throw new Error("Completion record did not contain metadata.");
  }
  if (envelope.data !== null) throw new Error("Completion record unexpectedly contained data.");
  const metadata = envelope.metadata as Record<string, unknown>;
  const outcome = metadata.outcome;
  const persistencePolicy = metadata.persistencePolicy;
  if (outcome !== "SUCCEEDED" && outcome !== "FAILED" && outcome !== "ABORTED") {
    throw new Error("Completion record contained an invalid outcome.");
  }
  if (persistencePolicy !== "NEVER" && persistencePolicy !== "ONERROR" && persistencePolicy !== "ALWAYS") {
    throw new Error("Completion record contained an invalid persistence policy.");
  }
  if (typeof metadata.errored !== "boolean") throw new Error("Completion record contained an invalid trace error flag.");
  const remainingFrames = requiredNonnegativeInteger(metadata.remainingFrames, "Remaining frame count");
  const terminalFailureId = optionalNonemptyString(metadata.terminalFailureId);
  if (metadata.terminalFailureId !== undefined && !terminalFailureId) throw new Error("Completion record contained an invalid terminal failure ID.");
  if (outcome === "SUCCEEDED" && terminalFailureId) throw new Error("Successful completion unexpectedly identified a terminal failure.");
  if (outcome !== "SUCCEEDED" && !terminalFailureId) throw new Error("Failed or aborted completion did not identify its terminal failure.");
  if (!metadata.sessionUsageSnapshot || typeof metadata.sessionUsageSnapshot !== "object" || Array.isArray(metadata.sessionUsageSnapshot)) {
    throw new Error("Completion record did not contain a terminal usage snapshot.");
  }
  const usageFields = metadata.sessionUsageSnapshot as Record<string, unknown>;
  const usage: CompletionUsage = {
    skillInvocations: requiredNonnegativeInteger(usageFields.skillInvocations, "Skill invocation count"),
    toolInvocations: requiredNonnegativeInteger(usageFields.toolInvocations, "Tool invocation count"),
    linterRetries: requiredNonnegativeInteger(usageFields.linterRetries, "Linter retry count"),
    modelCalls: requiredNonnegativeInteger(usageFields.modelCalls, "Model call count"),
    providerAttempts: requiredNonnegativeInteger(usageFields.providerAttempts, "Provider attempt count"),
    promptUnits: requiredNonnegativeInteger(usageFields.promptUnits, "Prompt unit count"),
    completionUnits: requiredNonnegativeInteger(usageFields.completionUnits, "Completion unit count"),
    totalUnits: requiredNonnegativeInteger(usageFields.totalUnits, "Total usage unit count"),
    exactModelResponses: requiredNonnegativeInteger(usageFields.exactModelResponses, "Exact model response count"),
    heuristicModelResponses: requiredNonnegativeInteger(usageFields.heuristicModelResponses, "Heuristic model response count"),
    unavailableModelResponses: requiredNonnegativeInteger(usageFields.unavailableModelResponses, "Unavailable model response count"),
  };
  if (usage.promptUnits + usage.completionUnits !== usage.totalUnits) {
    throw new Error("Completion record's terminal usage totals did not reconcile.");
  }
  if (usage.exactModelResponses + usage.heuristicModelResponses + usage.unavailableModelResponses !== usage.modelCalls) {
    throw new Error("Completion record's model response precision counts did not reconcile.");
  }
  return {
    kind: "completion",
    outcome,
    skillName: optionalNonemptyString(metadata.skillName),
    objective: optionalNonemptyString(metadata.objective),
    entryPoint: optionalNonemptyString(metadata.entryPoint),
    remainingFrames,
    persistencePolicy,
    errored: metadata.errored,
    terminalFailureId,
    usage,
  };
}

function humanizeAction(value: string): string {
  const normalized = value.toLowerCase().replaceAll("_", " ");
  return normalized.length > 0 ? normalized[0].toUpperCase() + normalized.slice(1) : normalized;
}

function StepActionDetailView({ detail }: { detail: StepActionDetail }) {
  const status = detail.kind === "proposed" ? "Proposed" : detail.kind === "validated" ? "Accepted" : "Rejected";
  return <>
    <dl className="trace-step-facts">
      <div><dt>Skill</dt><dd>{detail.skillName}</dd></div>
      <div><dt>Step</dt><dd>{detail.stepNumber}</dd></div>
      <div><dt>Status</dt><dd>{status}</dd></div>
      {detail.actionType && <div><dt>Action</dt><dd>{humanizeAction(detail.actionType)}</dd></div>}
      {detail.taskTitle && <div><dt>Task</dt><dd>{detail.taskTitle}</dd></div>}
      {detail.taskId && <div><dt>Task ID</dt><dd>{detail.taskId}</dd></div>}
      {detail.toolName && <div><dt>Tool</dt><dd>{detail.toolName}</dd></div>}
      {detail.earlierRejectedAttempts !== undefined && <div><dt>Earlier rejected attempts</dt><dd>{detail.earlierRejectedAttempts}</dd></div>}
      {detail.exhausted !== undefined && <div><dt>Retries exhausted</dt><dd>{detail.exhausted ? "Yes" : "No"}</dd></div>}
    </dl>
    {detail.kind === "proposed" && <p className="trace-step-note">The planner proposed this action. The runtime has not accepted or executed it yet.</p>}
    {detail.kind === "validated" && <p className="trace-step-note">The runtime accepted this action for execution. This does not mean the tool ran or succeeded.{detail.proposedSequence !== undefined && <> Proposed action: record {detail.proposedSequence}.</>}</p>}
    {detail.kind === "rejected" && <div className="trace-action-rejection">
      <p><strong>Reason:</strong> {detail.reason}</p>
      <p className="trace-step-note">The runtime rejected this proposal before execution. A later record may contain the planner's corrected action.</p>
      {detail.rawResponse && <><h5>Model response excerpt</h5><pre>{detail.rawResponse}</pre></>}
    </div>}
  </>;
}

function RecordDetailView({ detail, onOpenRelated, onSelectFailure }: { detail: RecordDetail; onOpenRelated: (record: TraceRecord) => void; onSelectFailure: (failureId: string) => void }) {
  if (detail.kind === "tool-input") return <>
    <dl className="trace-step-facts">
      <div><dt>Tool</dt><dd>{detail.capabilityName}</dd></div>
      <div><dt>Execution</dt><dd>{detail.unplanned ? "Unplanned" : "Planned"}</dd></div>
      {detail.taskTitle && <div><dt>Task</dt><dd>{detail.taskTitle}</dd></div>}
      {detail.taskId && <div><dt>Task ID</dt><dd>{detail.taskId}</dd></div>}
      <div><dt>Event ID</dt><dd>{detail.eventId}</dd></div>
    </dl>
    {detail.unplanned && <p className="trace-step-note">No plan task was linked to this invocation.</p>}
    {detail.note && <p><strong>Note:</strong> {detail.note}</p>}
    <h5>Arguments</h5>
    <pre>{detail.arguments}</pre>
  </>;

  if (detail.kind === "tool-result") return <>
    <dl className="trace-step-facts">
      <div><dt>Tool</dt><dd>{detail.capabilityName}</dd></div>
      {detail.taskTitle && <div><dt>Task</dt><dd>{detail.taskTitle}</dd></div>}
      {detail.taskId && <div><dt>Task ID</dt><dd>{detail.taskId}</dd></div>}
      <div><dt>Event ID</dt><dd>{detail.eventId}</dd></div>
    </dl>
    {detail.note && <p><strong>Note:</strong> {detail.note}</p>}
    <h5>Result</h5>
    <pre>{detail.result}</pre>
  </>;

  if (detail.kind === "structured-output") return <>
    <dl className="trace-step-facts">
      <div><dt>Skill</dt><dd>{detail.skillName}</dd></div>
      <div><dt>Status</dt><dd>{humanizeAction(detail.status)}</dd></div>
      <div><dt>Attempt</dt><dd>{detail.attempt}</dd></div>
      <div><dt>Retries used</dt><dd>{detail.retryCount} of {detail.maxRetries}</dd></div>
      {detail.failureMode && <div><dt>Failure mode</dt><dd>{humanizeAction(detail.failureMode)}</dd></div>}
    </dl>
    {detail.status === "PASSED" && <p className="trace-step-note">Output schema validation passed{detail.retryCount === 0 ? " without a retry" : ` after ${detail.retryCount} ${detail.retryCount === 1 ? "retry" : "retries"}`}.</p>}
    {detail.status === "RETRYING" && <p className="trace-step-note">The output did not satisfy its schema. Loomspan requested another model response.</p>}
    {detail.status === "EXHAUSTED" && <p className="trace-step-note">The output did not satisfy its schema and no validation retries remain.</p>}
    {detail.issues.length > 0 && <section className="trace-validation-issues" aria-label="Output schema issues">
      <h5>Issues</h5>
      <ul>{detail.issues.map((issue, index) => <li key={`${issue.path}-${index}`}><code>{issue.path}</code> &mdash; {issue.message}{issue.canonicalField && <> <span>(field: <code>{issue.canonicalField}</code>)</span></>}</li>)}</ul>
    </section>}
  </>;

  if (detail.kind === "evidence") return <>
    <dl className="trace-step-facts">
      <div><dt>Evidence source</dt><dd>{detail.skillName}</dd></div>
      {detail.taskTitle && <div><dt>Task</dt><dd>{detail.taskTitle}</dd></div>}
      {detail.taskId && <div><dt>Task ID</dt><dd>{detail.taskId}</dd></div>}
      <div><dt>Execution</dt><dd>{detail.unplanned ? "Unplanned" : "Planned"}</dd></div>
    </dl>
    <section className="trace-evidence-sources" aria-label="Available evidence sources">
      <h5>Available evidence sources after this record</h5>
      <ul>{detail.availableSources.map((source) => <li key={source}><code>{source}</code></li>)}</ul>
    </section>
    <p className="trace-step-note">This successful skill became available as an evidence source. This record does not determine whether a particular final-response claim is supported.</p>
    {detail.sourceResult && <button type="button" onClick={() => onOpenRelated(detail.sourceResult!)}>View source result</button>}
  </>;

  if (detail.kind === "completion") {
    const usage = detail.usage;
    const abnormal = detail.outcome !== "SUCCEEDED" || detail.remainingFrames > 0 || detail.errored || usage.unavailableModelResponses > 0;
    return <>
      <dl className="trace-step-facts">
        <div><dt>Outcome</dt><dd>{humanizeAction(detail.outcome)}</dd></div>
        {detail.skillName && <div><dt>Entry skill</dt><dd>{detail.skillName}</dd></div>}
        {detail.entryPoint && <div><dt>Entry point</dt><dd>{detail.entryPoint}</dd></div>}
        <div><dt>Remaining open frames</dt><dd>{detail.remainingFrames}</dd></div>
        <div><dt>Persistence</dt><dd>{humanizeAction(detail.persistencePolicy)}</dd></div>
        <div><dt>Trace error recorded</dt><dd>{detail.errored ? "Yes" : "No"}</dd></div>
        {detail.terminalFailureId && <div><dt>Terminal failure ID</dt><dd>{detail.terminalFailureId}</dd></div>}
      </dl>
      {detail.objective && <p><strong>Objective:</strong> {detail.objective}</p>}
      <div className="trace-completion-tables">
        <table aria-label="Final execution counters"><caption>Final execution counters</caption><tbody>
          <tr><th scope="row">Skill invocations</th><td>{usage.skillInvocations.toLocaleString()}</td></tr>
          <tr><th scope="row">Tool invocations</th><td>{usage.toolInvocations.toLocaleString()}</td></tr>
          <tr><th scope="row">Model calls</th><td>{usage.modelCalls.toLocaleString()}</td></tr>
          <tr><th scope="row">Provider attempts</th><td>{usage.providerAttempts.toLocaleString()}</td></tr>
          <tr><th scope="row">Linter retries</th><td>{usage.linterRetries.toLocaleString()}</td></tr>
        </tbody></table>
        <table aria-label="Final usage"><caption>Final usage</caption><tbody>
          <tr><th scope="row">Prompt units</th><td>{usage.promptUnits.toLocaleString()}</td></tr>
          <tr><th scope="row">Completion units</th><td>{usage.completionUnits.toLocaleString()}</td></tr>
          <tr><th scope="row">Total units</th><td>{usage.totalUnits.toLocaleString()}</td></tr>
        </tbody></table>
        <table aria-label="Usage precision"><caption>Usage precision</caption><tbody>
          <tr><th scope="row">Exact responses</th><td>{usage.exactModelResponses.toLocaleString()}</td></tr>
          <tr><th scope="row">Heuristic responses</th><td>{usage.heuristicModelResponses.toLocaleString()}</td></tr>
          <tr><th scope="row">Unavailable responses</th><td>{usage.unavailableModelResponses.toLocaleString()}</td></tr>
        </tbody></table>
      </div>
      <p className="trace-step-note">Remaining open frames reports cleanup state when this terminal record was written; it does not make a broader claim about every cleanup operation.</p>
      {abnormal && <p className="trace-completion-warning">This completion contains one or more failure, cleanup, trace-error, or usage-availability conditions.</p>}
      {detail.terminalFailureId && <button className="trace-error-action" type="button" onClick={() => onSelectFailure(detail.terminalFailureId!)}>View terminal error</button>}
    </>;
  }

  return <>
    <dl className="trace-step-facts">
      <div><dt>Skill</dt><dd>{detail.skillName}</dd></div>
      <div><dt>Step</dt><dd>{detail.stepNumber}</dd></div>
      <div><dt>Status</dt><dd>{humanizeAction(detail.status)}</dd></div>
      <div><dt>Action</dt><dd>{humanizeAction(detail.actionType)}</dd></div>
      {detail.taskTitle && <div><dt>Task</dt><dd>{detail.taskTitle}</dd></div>}
      {detail.taskId && <div><dt>Task ID</dt><dd>{detail.taskId}</dd></div>}
      {detail.toolName && <div><dt>Tool</dt><dd>{detail.toolName}</dd></div>}
    </dl>
    {detail.error && <p className="trace-action-error"><strong>Error:</strong> {detail.error}</p>}
    {detail.resultPreview && <><h5>Result preview</h5><pre>{prettyValue(detail.resultPreview)}</pre></>}
    {detail.actionType === "CALL_TOOL" && <p className="trace-step-note">The preview may be truncated. The related tool record is authoritative for the full result.</p>}
    {detail.actionType === "FINAL_RESPONSE" && <p className="trace-step-note">This record confirms the final-response step completed; it does not contain the response body.</p>}
    {detail.relatedRecord && <button type="button" onClick={() => onOpenRelated(detail.relatedRecord!)}>{detail.actionType === "CALL_TOOL" ? (detail.relatedRecord.type === "TOOL_CALL_FAILED" ? "View tool failure record" : "View full tool result") : "View model response record"}</button>}
  </>;
}

function PlanChanges({ comparison, previousSequence }: { comparison: PlanComparison; previousSequence: number }) {
  const hasChanges = comparison.plan.length > 0 || comparison.tasks.length > 0;
  return <section className="trace-plan-changes" aria-label="Plan changes">
    <p className="trace-plan-comparison-source">Changes since record {previousSequence}</p>
    {!hasChanges && <p>No task or plan-state changes were detected. Check Full plan for other fields.</p>}
    {comparison.plan.length > 0 && <div>
      <h5>Plan</h5>
      <ul>{comparison.plan.map((change) => <li key={change.label}><strong>{change.label}:</strong> {change.before} <span aria-label="changed to">{"\u2192"}</span> {change.after}</li>)}</ul>
    </div>}
    {comparison.tasks.map((task) => <div className="trace-plan-task-change" key={task.taskId}>
      <h5>{task.intent}</h5>
      {task.kind === "added" && <p>Task added</p>}
      {task.kind === "removed" && <p>Task removed</p>}
      {task.fields.length > 0 && <ul>{task.fields.map((change) => <li key={change.label}><strong>{change.label}:</strong> {change.before} <span aria-label="changed to">{"\u2192"}</span> {change.after}</li>)}</ul>}
    </div>)}
  </section>;
}

export function TraceRecords({ traceId, source = "TARGET", records, failures, selectedRecordSequence, selectedFailureId, onSelectRecord, onSelectFailure, onRelatedFrame, onPayload }: Props) {
  const related = onRelatedFrame ?? (() => undefined);
  const [expanded, setExpanded] = useState<string | null>(null);
  const [cache, setCache] = useState<Record<string, PlanCacheEntry>>({});
  const [modelCache, setModelCache] = useState<Record<string, ModelCacheEntry>>({});
  const [rawCache, setRawCache] = useState<Record<string, RawCacheEntry>>({});
  const [stepCache, setStepCache] = useState<Record<string, StepCacheEntry>>({});
  const [stepActionCache, setStepActionCache] = useState<Record<string, StepActionCacheEntry>>({});
  const [recordDetailCache, setRecordDetailCache] = useState<Record<string, RecordDetailCacheEntry>>({});
  const [planView, setPlanView] = useState<"changes" | "full">("full");

  useEffect(() => {
    if (!selectedRecordSequence) return;
    document.getElementById(`trace-record-${selectedRecordSequence}`)?.focus();
  }, [records, selectedRecordSequence]);

  const handleTogglePlan = (record: TraceRecord) => {
    const seq = record.sequence;
    const key = `${traceId}:${seq}`;
    const expansionKey = `${key}:plan`;
    if (expanded === expansionKey) {
      setExpanded(null);
      return;
    }
    const isUpdate = record.type === "PLAN_UPDATED";
    setPlanView(isUpdate ? "changes" : "full");
    setExpanded(expansionKey);
    if (!traceId) return;
    const existing = cache[key];
    if (existing?.json || existing?.loading) return;
    setCache((prev) => ({ ...prev, [key]: { loading: true } }));
    void readPlanSnapshot(traceId, record, source)
      .then(async (snapshot) => {
        const json = JSON.stringify(snapshot.value, null, 2);
        setCache((prev) => ({ ...prev, [key]: { loading: false, json, snapshot, comparisonLoading: isUpdate } }));
        if (!isUpdate) return;
        if (!snapshot.planId) {
          setCache((prev) => ({ ...prev, [key]: { ...prev[key], comparisonLoading: false, comparisonError: "The plan update did not contain a plan ID." } }));
          return;
        }
        try {
          const previous = await findPreviousPlan(traceId, seq, snapshot.planId, source);
          const comparison = previous ? comparePlans(previous.snapshot, snapshot) : undefined;
          setCache((prev) => ({ ...prev, [key]: { ...prev[key], comparisonLoading: false, comparisonReady: true, comparison, previousSequence: previous?.sequence } }));
        } catch (err: unknown) {
          const message = err instanceof Error ? err.message : "The previous plan version could not be loaded.";
          setCache((prev) => ({ ...prev, [key]: { ...prev[key], comparisonLoading: false, comparisonError: message } }));
        }
      })
      .catch((err: unknown) => {
        const message = err instanceof Error ? `Plan could not be displayed: ${err.message}` : "Plan could not be displayed.";
        setCache((prev) => ({ ...prev, [key]: { loading: false, error: message } }));
      });
  };

  const handleToggleModel = (record: TraceRecord) => {
    const key = `${traceId}:${record.sequence}`;
    const expansionKey = `${key}:model`;
    if (expanded === expansionKey) {
      setExpanded(null);
      return;
    }
    setExpanded(expansionKey);
    if (!traceId) return;
    const existing = modelCache[key];
    if (existing?.detail || existing?.loading) return;
    const kind = record.type === "MODEL_RESPONSE_RECEIVED" ? "response" : "request";
    setModelCache((previous) => ({ ...previous, [key]: { loading: true } }));
	const detailSource = record.payloadId
      ? readCompletePayload(traceId, record.payloadId, source).then((raw) => parseJsonObject(raw, "Model payload"))
      : readCompleteRecord(traceId, record.sequence, source).then(recordData);
	void detailSource
      .then((value) => {
        const detail = parseModelDetail(kind, value);
        setModelCache((previous) => ({ ...previous, [key]: { loading: false, detail } }));
      })
      .catch((error: unknown) => {
        const message = error instanceof Error ? `${kind === "request" ? "Request" : "Response"} could not be displayed: ${error.message}` : `${kind === "request" ? "Request" : "Response"} could not be displayed.`;
        setModelCache((previous) => ({ ...previous, [key]: { loading: false, error: message } }));
      });
  };

  const handleToggleRaw = (record: TraceRecord) => {
    const key = `${traceId}:${record.sequence}`;
    const expansionKey = `${key}:raw`;
    if (expanded === expansionKey) {
      setExpanded(null);
      return;
    }
    setExpanded(expansionKey);
    if (!traceId) return;
    const existing = rawCache[key];
    if (existing?.json || existing?.loading) return;
    setRawCache((previous) => ({ ...previous, [key]: { loading: true } }));
    void readCompleteRecord(traceId, record.sequence, source)
      .then((raw) => {
        const value: unknown = JSON.parse(raw);
        const json = JSON.stringify(value, null, 2);
        setRawCache((previous) => ({ ...previous, [key]: { loading: false, json } }));
      })
      .catch((error: unknown) => {
        const message = error instanceof Error ? `Raw record could not be displayed: ${error.message}` : "Raw record could not be displayed.";
        setRawCache((previous) => ({ ...previous, [key]: { loading: false, error: message } }));
      });
  };

  const handleToggleStep = (record: TraceRecord) => {
    const key = `${traceId}:${record.sequence}`;
    const expansionKey = `${key}:step`;
    if (expanded === expansionKey) {
      setExpanded(null);
      return;
    }
    setExpanded(expansionKey);
    if (!traceId) return;
    const existing = stepCache[key];
    if (existing?.detail || existing?.loading) return;
    setStepCache((previous) => ({ ...previous, [key]: { loading: true } }));
    void readCompleteRecord(traceId, record.sequence, source)
      .then((raw) => {
        const detail = parseStepStartedDetail(raw, record.route);
        setStepCache((previous) => ({ ...previous, [key]: { loading: false, detail } }));
      })
      .catch((error: unknown) => {
        const message = error instanceof Error ? `Step details could not be displayed: ${error.message}` : "Step details could not be displayed.";
        setStepCache((previous) => ({ ...previous, [key]: { loading: false, error: message } }));
      });
  };

  const handleToggleStepAction = (record: TraceRecord, kind: StepActionKind) => {
    const key = `${traceId}:${record.sequence}`;
    const expansionKey = `${key}:step-action`;
    if (expanded === expansionKey) {
      setExpanded(null);
      return;
    }
    setExpanded(expansionKey);
    if (!traceId) return;
    const existing = stepActionCache[key];
    if (existing?.detail || existing?.loading) return;
    setStepActionCache((previous) => ({ ...previous, [key]: { loading: true } }));
    void readStepActionDetail(traceId, record, kind, source)
      .then((detail) => {
        setStepActionCache((previous) => ({ ...previous, [key]: { loading: false, detail } }));
      })
      .catch((error: unknown) => {
        const message = error instanceof Error ? `Action details could not be displayed: ${error.message}` : "Action details could not be displayed.";
        setStepActionCache((previous) => ({ ...previous, [key]: { loading: false, error: message } }));
      });
  };

  const handleToggleRecordDetail = (record: TraceRecord) => {
    const key = `${traceId}:${record.sequence}`;
    const expansionKey = `${key}:record-detail`;
    if (expanded === expansionKey) {
      setExpanded(null);
      return;
    }
    setExpanded(expansionKey);
    if (!traceId) return;
    const existing = recordDetailCache[key];
    if (existing?.detail || existing?.loading) return;
    setRecordDetailCache((previous) => ({ ...previous, [key]: { loading: true } }));
    const request = record.type === "TOOL_CALL_STARTED"
      ? readToolInputDetail(traceId, record, source)
      : record.type === "TOOL_CALL_COMPLETED"
        ? readToolResultDetail(traceId, record, source)
      : record.type === "STRUCTURED_OUTPUT_RECORDED"
        ? readCompleteRecord(traceId, record.sequence, source).then(parseStructuredOutputDetail)
        : record.type === "EVIDENCE_RECORDED"
          ? readEvidenceDetail(traceId, record, source)
          : record.type === "TRACE_COMPLETED"
            ? readCompleteRecord(traceId, record.sequence, source).then(parseCompletionDetail)
            : readStepCompletedDetail(traceId, record, source);
    void request
      .then((detail) => setRecordDetailCache((previous) => ({ ...previous, [key]: { loading: false, detail } })))
      .catch((error: unknown) => {
        const message = error instanceof Error ? `Details could not be displayed: ${error.message}` : "Details could not be displayed.";
        setRecordDetailCache((previous) => ({ ...previous, [key]: { loading: false, error: message } }));
      });
  };

  const openRelatedRecord = (record: TraceRecord) => {
    onSelectRecord(record);
    if (record.type === "TOOL_CALL_COMPLETED") {
      handleToggleRecordDetail(record);
    } else if (record.type === "MODEL_RESPONSE_RECEIVED") {
      handleToggleModel(record);
    }
  };

  return <div aria-label="Trace records">
    <h4>Records</h4><div className="trace-table-region" role="region" aria-label="Record list" tabIndex={0}><table><thead><tr><th>Sequence</th><th>Type</th><th>Frame</th><th>Timestamp</th><th>Actions</th></tr></thead><tbody>{records.map((record) => {
      const isPlanCreated = record.type === "PLAN_CREATED";
      const isPlanUpdated = record.type === "PLAN_UPDATED";
      const isPlanRecord = isPlanCreated || isPlanUpdated;
      const isPreparedRequest = record.type === "MODEL_REQUEST_PREPARED";
      const isModelRequest = isPreparedRequest || record.type === "MODEL_REQUEST_SENT";
      const isModelResponse = record.type === "MODEL_RESPONSE_RECEIVED";
      const isModelRecord = isModelRequest || isModelResponse;
      const isStepStarted = record.type === "STEP_STARTED";
      const isToolInput = record.type === "TOOL_CALL_STARTED";
      const isToolResult = record.type === "TOOL_CALL_COMPLETED";
      const isStructuredOutput = record.type === "STRUCTURED_OUTPUT_RECORDED";
      const isStepCompleted = record.type === "STEP_COMPLETED";
      const isEvidence = record.type === "EVIDENCE_RECORDED";
      const isCompletion = record.type === "TRACE_COMPLETED";
      const hasRecordDetail = isToolInput || isToolResult || isStructuredOutput || isStepCompleted || isEvidence || isCompletion;
      const recordDetailLabel = isToolInput ? "Tool input" : isToolResult ? "Tool result" : isStructuredOutput ? "Validation details" : isEvidence ? "Evidence details" : isCompletion ? "Completion details" : "Step result";
      const stepActionKind: StepActionKind | undefined = record.type === "STEP_ACTION_PROPOSED" ? "proposed"
        : record.type === "STEP_ACTION_VALIDATED" ? "validated"
          : record.type === "STEP_ACTION_REJECTED" ? "rejected"
            : undefined;
      const modelLabel = isPreparedRequest ? "Prepared request" : isModelRequest ? "Request" : "Response";
      const modelRegionLabel = isPreparedRequest ? "Prepared model request" : isModelRequest ? "Model request" : "Model response";
      const key = `${traceId}:${record.sequence}`;
      const isPlanExpanded = expanded === `${key}:plan`;
      const isModelExpanded = expanded === `${key}:model`;
      const isRawExpanded = expanded === `${key}:raw`;
      const isStepExpanded = expanded === `${key}:step`;
      const isStepActionExpanded = expanded === `${key}:step-action`;
      const isRecordDetailExpanded = expanded === `${key}:record-detail`;
      const entry = cache[key];
      const modelEntry = modelCache[key];
      const rawEntry = rawCache[key];
      const stepEntry = stepCache[key];
      const stepActionEntry = stepActionCache[key];
      const recordDetailEntry = recordDetailCache[key];
      const linkedFailure = failures.find((failure) => failure.sequence === record.sequence);
      const severity = recordSeverity(record, linkedFailure);
      const severityLabel = severity === "error" ? "Failure" : severity === "warning" ? "Retry or warning" : undefined;
      return (
        <Fragment key={record.sequence}>
          <tr className={severity === "normal" ? undefined : `trace-record-${severity}`} aria-label={severityLabel ? `${severityLabel}: record ${record.sequence}, ${record.type}` : undefined} aria-current={selectedRecordSequence === record.sequence ? "true" : undefined}>
            <td><button id={`trace-record-${record.sequence}`} type="button" onClick={() => onSelectRecord(record)}>{record.sequence}: {record.type}</button></td>
            <td>{record.type}</td>
            <td>{record.frameId && <button type="button" onClick={() => related({ frameIds: [record.frameId] })}>{record.frameId}</button>}</td>
            <td>{record.timestampMillis}</td>
            <td>
              <button type="button" aria-expanded={isRawExpanded} aria-controls={`raw-detail-${record.sequence}`} onClick={() => handleToggleRaw(record)}>{isRawExpanded ? "Hide raw record" : "Read raw record"}</button>
              {record.payloadId && !isModelRecord && <button type="button" onClick={() => onPayload(record.payloadId)}>Read payload</button>}
              {linkedFailure && <button className="trace-error-action" type="button" aria-pressed={selectedFailureId === linkedFailure.failureId} onClick={() => onSelectFailure(linkedFailure.failureId)}>View error</button>}
              {isPlanRecord && traceId && (
                <button type="button" aria-expanded={isPlanExpanded} aria-controls={`plan-detail-${record.sequence}`} onClick={() => handleTogglePlan(record)}>
                  {isPlanExpanded ? (isPlanUpdated ? "Hide changes" : "Hide Plan") : (isPlanUpdated ? "View changes" : "Show Plan")}
                </button>
              )}
              {isModelRecord && traceId && (
                <button type="button" aria-expanded={isModelExpanded} aria-controls={`model-detail-${record.sequence}`} onClick={() => handleToggleModel(record)}>
                  {isModelExpanded ? `Hide ${modelLabel.toLowerCase()}` : modelLabel}
                </button>
              )}
              {isStepStarted && traceId && (
                <button type="button" aria-expanded={isStepExpanded} aria-controls={`step-detail-${record.sequence}`} onClick={() => handleToggleStep(record)}>
                  {isStepExpanded ? "Hide step details" : "Step details"}
                </button>
              )}
              {stepActionKind && traceId && (
                <button type="button" aria-expanded={isStepActionExpanded} aria-controls={`step-action-detail-${record.sequence}`} onClick={() => handleToggleStepAction(record, stepActionKind)}>
                  {isStepActionExpanded ? "Hide action details" : "Action details"}
                </button>
              )}
              {hasRecordDetail && traceId && (
                <button type="button" aria-expanded={isRecordDetailExpanded} aria-controls={`record-detail-${record.sequence}`} onClick={() => handleToggleRecordDetail(record)}>
                  {isRecordDetailExpanded ? `Hide ${recordDetailLabel.toLowerCase()}` : recordDetailLabel}
                </button>
              )}
            </td>
          </tr>
          {isRawExpanded && (
            <tr key={`${record.sequence}-raw`}>
              <td colSpan={5}>
                <div id={`raw-detail-${record.sequence}`} className="trace-raw-expanded" role="region" aria-label={`Raw record ${record.sequence}`}>
                  {!traceId && <p role="status">Trace context unavailable.</p>}
                  {traceId && rawEntry?.loading && <p role="status">Loading raw record&hellip;</p>}
                  {rawEntry?.error && <p role="alert">{rawEntry.error}</p>}
                  {rawEntry?.json && <pre>{rawEntry.json}</pre>}
                </div>
              </td>
            </tr>
          )}
          {isPlanRecord && isPlanExpanded && (
            <tr key={`${record.sequence}-plan`}>
              <td colSpan={5}>
                <div id={`plan-detail-${record.sequence}`} className="trace-plan-expanded" role="region" aria-label={`${isPlanUpdated ? "Plan update" : "Plan"} for record ${record.sequence}`}>
                  {!traceId && <p role="status">Trace context unavailable.</p>}
                  {traceId && entry?.loading && <p role="status">Loading plan&hellip;</p>}
                  {entry?.error && <p role="alert">{entry.error}</p>}
                  {entry?.json && isPlanUpdated && <>
                    <div role="tablist" aria-label={`Plan record ${record.sequence} views`}>
                      {(["changes", "full"] as const).map((view) => <button
                        id={`plan-${record.sequence}-tab-${view}`}
                        aria-controls={`plan-${record.sequence}-panel-${view}`}
                        aria-selected={planView === view}
                        key={view}
                        onClick={() => setPlanView(view)}
                        onKeyDown={(event: KeyboardEvent<HTMLButtonElement>) => {
                          if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;
                          event.preventDefault();
                          const next = event.key === "ArrowLeft" || event.key === "Home" ? "changes" : "full";
                          setPlanView(next);
                          document.getElementById(`plan-${record.sequence}-tab-${next}`)?.focus();
                        }}
                        role="tab"
                        tabIndex={planView === view ? 0 : -1}
                        type="button"
                      >{view === "changes" ? "Changes" : "Full plan"}</button>)}
                    </div>
                    {planView === "changes" && <div id={`plan-${record.sequence}-panel-changes`} aria-labelledby={`plan-${record.sequence}-tab-changes`} role="tabpanel">
                      {entry.comparisonLoading && <p role="status">Finding the previous plan version&hellip;</p>}
                      {entry.comparisonError && <p role="alert">Changes could not be determined: {entry.comparisonError}</p>}
                      {entry.comparisonReady && entry.comparison && entry.previousSequence !== undefined && <PlanChanges comparison={entry.comparison} previousSequence={entry.previousSequence} />}
                      {entry.comparisonReady && !entry.comparison && <p>The earlier version of this plan is not available in the trace.</p>}
                    </div>}
                    {planView === "full" && <div id={`plan-${record.sequence}-panel-full`} aria-labelledby={`plan-${record.sequence}-tab-full`} role="tabpanel"><pre>{entry.json}</pre></div>}
                  </>}
                  {entry?.json && isPlanCreated && <pre>{entry.json}</pre>}
                </div>
              </td>
            </tr>
          )}
          {isModelRecord && isModelExpanded && (
            <tr key={`${record.sequence}-model`}>
              <td colSpan={5}>
                <div id={`model-detail-${record.sequence}`} className="trace-model-expanded" role="region" aria-label={`${modelRegionLabel} for record ${record.sequence}`}>
                  {!traceId && <p role="status">Trace context unavailable.</p>}
                  {traceId && modelEntry?.loading && <p role="status">Loading {modelLabel.toLowerCase()}&hellip;</p>}
                  {modelEntry?.error && <p role="alert">{modelEntry.error}</p>}
                  {modelEntry?.detail && <ModelDetailView detail={modelEntry.detail} />}
                </div>
              </td>
            </tr>
          )}
          {isStepStarted && isStepExpanded && (
            <tr key={`${record.sequence}-step`}>
              <td colSpan={5}>
                <div id={`step-detail-${record.sequence}`} className="trace-step-expanded" role="region" aria-label={`Step details for record ${record.sequence}`}>
                  {!traceId && <p role="status">Trace context unavailable.</p>}
                  {traceId && stepEntry?.loading && <p role="status">Loading step details&hellip;</p>}
                  {stepEntry?.error && <p role="alert">{stepEntry.error}</p>}
                  {stepEntry?.detail && <>
                    <dl className="trace-step-facts">
                      <div><dt>Skill</dt><dd>{stepEntry.detail.skillName}</dd></div>
                      <div><dt>Step</dt><dd>{stepEntry.detail.stepNumber}</dd></div>
                      <div><dt>Ready tasks</dt><dd>{stepEntry.detail.readyTasks}</dd></div>
                      <div><dt>Plan status</dt><dd>{stepEntry.detail.planStatus.toLowerCase().replaceAll("_", " ")}</dd></div>
                    </dl>
                    <p className="trace-step-note">No task or action has been selected yet. That decision is recorded by the later STEP_ACTION_PROPOSED record.</p>
                  </>}
                </div>
              </td>
            </tr>
          )}
          {stepActionKind && isStepActionExpanded && (
            <tr key={`${record.sequence}-step-action`}>
              <td colSpan={5}>
                <div id={`step-action-detail-${record.sequence}`} className={`trace-step-expanded${stepActionKind === "rejected" ? " trace-step-rejected" : ""}`} role="region" aria-label={`Action details for record ${record.sequence}`}>
                  {!traceId && <p role="status">Trace context unavailable.</p>}
                  {traceId && stepActionEntry?.loading && <p role="status">Loading action details&hellip;</p>}
                  {stepActionEntry?.error && <p role="alert">{stepActionEntry.error}</p>}
                  {stepActionEntry?.detail && <StepActionDetailView detail={stepActionEntry.detail} />}
                </div>
              </td>
            </tr>
          )}
          {hasRecordDetail && isRecordDetailExpanded && (
            <tr key={`${record.sequence}-record-detail`}>
              <td colSpan={5}>
                <div id={`record-detail-${record.sequence}`} className={`trace-step-expanded${recordDetailEntry?.detail?.kind === "structured-output" && recordDetailEntry.detail.status !== "PASSED" ? " trace-step-rejected" : ""}`} role="region" aria-label={`${recordDetailLabel} for record ${record.sequence}`}>
                  {!traceId && <p role="status">Trace context unavailable.</p>}
                  {traceId && recordDetailEntry?.loading && <p role="status">Loading details&hellip;</p>}
                  {recordDetailEntry?.error && <p role="alert">{recordDetailEntry.error}</p>}
                  {recordDetailEntry?.detail && <RecordDetailView detail={recordDetailEntry.detail} onOpenRelated={openRelatedRecord} onSelectFailure={onSelectFailure} />}
                </div>
              </td>
            </tr>
          )}
        </Fragment>
      );
    })}</tbody></table></div>
  </div>;
}
