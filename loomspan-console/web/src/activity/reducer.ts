import type { BrowserAPIError } from "../api/client";
import type {
  Activity,
  ActivityCoverage,
  ActivityKind,
  ConnectionFact,
  Continuity,
} from "../api/contracts";

export type ActivityState = {
  activities: Activity[];
  recentCompletions: Activity[];
  connected: boolean;
  connectionFact: ConnectionFact | null;
  error: BrowserAPIError | null;
  loading: boolean;
  lastCursor: string | null;
  continuity: Continuity | null;
  baselineObservedAt: string | null;
  coverage: ActivityCoverage;
  reconnectAttempt: number;
};

export type ActivityAction =
  | { type: "reset" }
  | { type: "recent-loading" }
  | {
      type: "recent-success";
      items: Activity[];
      hasMore: boolean;
      nextCursor: string;
      append: boolean;
      continuity?: Continuity;
      coverage: ActivityCoverage;
    }
  | { type: "recent-error"; error: BrowserAPIError }
  | { type: "stream-connected" }
  | { type: "stream-connection"; fact: ConnectionFact }
  | { type: "stream-activity"; activity: Activity }
  | { type: "stream-continuity"; continuity: Continuity }
  | { type: "stream-replay-gap"; reason: string }
  | { type: "baseline-refreshed"; observedAt: string }
  | { type: "stream-closed" }
  | { type: "stream-error"; error: BrowserAPIError };

export const initialActivityState: ActivityState = {
  activities: [],
  recentCompletions: [],
  connected: false,
  connectionFact: null,
  error: null,
  loading: false,
  lastCursor: null,
  continuity: null,
  baselineObservedAt: null,
  coverage: {},
  reconnectAttempt: 0,
};

const maxActivities = 256;

function isTerminalKind(kind: ActivityKind): boolean {
  return kind === "TRACE_COMPLETED" || kind === "EXECUTION_OBSERVATION_ENDED";
}

function mergeUniqueByCursor(existing: Activity[], additions: Activity[]): Activity[] {
  const cursors = new Set(existing.map((activity) => activity.cursor));
  const merged = [...existing];
  for (const activity of additions) {
    if (!cursors.has(activity.cursor)) {
      cursors.add(activity.cursor);
      merged.push(activity);
    }
  }
  return merged.slice(-maxActivities);
}

function mergeRecentCompletions(
  existing: Activity[],
  additions: Activity[],
): Activity[] {
  const merged = [...existing];
  for (const activity of additions) {
    const index = merged.findIndex(
      (current) =>
        current.instanceId === activity.instanceId &&
        current.sessionId === activity.sessionId,
    );
    if (index >= 0) {
      merged[index] = activity;
    } else {
      merged.push(activity);
    }
  }
  return merged.slice(-maxActivities);
}

function continuityChanged(
  current: Continuity | null,
  next: Continuity | undefined,
): boolean {
  if (!current || !next) return false;
  return current.intervalId !== next.intervalId ||
    current.targetScopeId !== next.targetScopeId ||
    current.instanceId !== next.instanceId;
}

export function activityReducer(
  state: ActivityState,
  action: ActivityAction,
): ActivityState {
  switch (action.type) {
    case "reset":
      return initialActivityState;
    case "recent-loading":
      return { ...state, loading: true, error: null };
    case "recent-success": {
      const crossedBoundary = continuityChanged(
        state.continuity,
        action.continuity,
      );
      const currentActivities = crossedBoundary ? [] : state.activities;
      const currentCompletions = crossedBoundary ? [] : state.recentCompletions;
      let all: Activity[];
      if (action.append) {
        const seen = new Set(currentActivities.map((a) => a.cursor));
        const deduped = action.items.filter((a) => !seen.has(a.cursor));
        all = [...currentActivities, ...deduped];
      } else {
        all = action.items;
      }
      const completions: Activity[] = [];
      for (const activity of all) {
        if (isTerminalKind(activity.kind)) {
          completions.push(activity);
        }
      }
      const mergedCompletions = mergeRecentCompletions(
        currentCompletions,
        completions,
      );
      return {
        ...state,
        activities: all.slice(-maxActivities),
        recentCompletions: mergedCompletions,
        loading: false,
        error: null,
        lastCursor: all.at(-1)?.cursor ??
          (crossedBoundary ? null : state.lastCursor),
        continuity: action.continuity ?? state.continuity,
        coverage: action.coverage,
      };
    }
    case "recent-error":
      return { ...state, loading: false, error: action.error };
    case "stream-connected":
      return { ...state, connected: true, error: null, reconnectAttempt: 0 };
    case "stream-connection":
      return {
        ...state,
        connected: action.fact.connected,
        connectionFact: action.fact,
      };
    case "stream-activity": {
      if (
        state.activities.some((existing) => existing.cursor === action.activity.cursor) ||
        (state.continuity?.instanceId &&
          state.continuity.instanceId !== action.activity.instanceId)
      ) {
        return state;
      }
      const all = [...state.activities, action.activity];
      if (isTerminalKind(action.activity.kind)) {
        const completions = mergeRecentCompletions(
          state.recentCompletions,
          [action.activity],
        );
        return {
          ...state,
          activities: all.slice(-maxActivities),
          recentCompletions: completions,
          lastCursor: action.activity.cursor,
        };
      }
      return {
        ...state,
        activities: all.slice(-maxActivities),
        lastCursor: action.activity.cursor,
      };
    }
    case "stream-continuity":
      if (
        (action.continuity.reset != null && state.continuity == null) ||
        (state.continuity?.intervalId &&
          state.continuity.intervalId !== action.continuity.intervalId) ||
        (state.continuity?.targetScopeId &&
          state.continuity.targetScopeId !== action.continuity.targetScopeId) ||
        (state.continuity?.instanceId &&
          state.continuity.instanceId !== action.continuity.instanceId)
      ) {
        return {
          ...initialActivityState,
          continuity: action.continuity,
          connected: state.connected,
          connectionFact: state.connectionFact,
        };
      }
      return {
        ...state,
        continuity: action.continuity,
      };
    case "stream-replay-gap":
      return {
        ...state,
        connectionFact: { connected: false, reason: action.reason },
        connected: false,
      };
    case "baseline-refreshed":
      return {
        ...state,
        baselineObservedAt: action.observedAt,
      };
    case "stream-closed":
      return {
        ...state,
        connected: false,
        connectionFact: { connected: false, reason: "stream_closed" },
        reconnectAttempt: state.reconnectAttempt + 1,
      };
    case "stream-error":
      return {
        ...state,
        connected: false,
        error: action.error,
        connectionFact: { connected: false, reason: "error" },
        reconnectAttempt: state.reconnectAttempt + 1,
      };
  }
}
