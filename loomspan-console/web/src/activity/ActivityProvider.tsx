import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useReducer,
  useRef,
} from "react";
import {
  BrowserAPIError,
  fetchRecentActivities,
  openActivityStream,
} from "../api/client";
import type { Activity, ConnectionFact, Continuity } from "../api/contracts";
import { useBrowserSession } from "../security/BrowserSessionProvider";
import { useTarget } from "../target/TargetProvider";
import { useOptionalObservability } from "../observability/ObservabilityProvider";
import {
  activityReducer,
  initialActivityState,
  type ActivityState,
} from "./reducer";

type ActivityContextValue = ActivityState & {
  loadRecent: (cursor?: string) => Promise<void>;
};

const authoritativeRefreshDelayMillis = 250;

type AuthoritativeRefresh = {
  instance?: boolean;
  activeExecutions?: boolean;
  traces?: boolean;
};

const ActivityContext = createContext<ActivityContextValue | undefined>(undefined);

export function ActivityProvider({ children }: { children: ReactNode }) {
  const { target, scopeGeneration } = useTarget();
  const observability = useOptionalObservability();
  const loadInstance = observability?.loadInstance;
  const loadActiveExecutions = observability?.loadActiveExecutions;
  const loadTraces = observability?.loadTraces;
  const session = useBrowserSession();
  const [state, dispatch] = useReducer(activityReducer, initialActivityState);
  const generationRef = useRef(0);
  const lastCursorRef = useRef<string | null>(null);
  const intervalRef = useRef<string | null>(null);
  const closeStreamRef = useRef<(() => void) | null>(null);
  const openStreamRef = useRef<(() => void) | null>(null);
  const resyncingRef = useRef(false);
  const authoritativeRefreshTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const pendingAuthoritativeRefreshRef = useRef<AuthoritativeRefresh>({});
  const targetScopeID = target.status.targetScopeId;
  const connected = target.status.targetConnection === "REACHABLE" &&
    target.status.targetAuthentication === "ESTABLISHED";
  const tabId = session.status === "paired" ? session.bootstrap.tabId : null;
  const csrfToken = session.status === "paired" ? session.bootstrap.csrfToken : null;

  useEffect(() => {
    generationRef.current++;
    lastCursorRef.current = null;
    intervalRef.current = null;
    resyncingRef.current = false;
    dispatch({ type: "reset" });
    if (closeStreamRef.current) {
      closeStreamRef.current();
      closeStreamRef.current = null;
    }
    if (authoritativeRefreshTimerRef.current) {
      clearTimeout(authoritativeRefreshTimerRef.current);
      authoritativeRefreshTimerRef.current = null;
    }
    pendingAuthoritativeRefreshRef.current = {};
  }, [scopeGeneration]);

  useEffect(() => () => {
    generationRef.current++;
    resyncingRef.current = false;
    openStreamRef.current = null;
    if (closeStreamRef.current) {
      closeStreamRef.current();
      closeStreamRef.current = null;
    }
    if (authoritativeRefreshTimerRef.current) {
      clearTimeout(authoritativeRefreshTimerRef.current);
      authoritativeRefreshTimerRef.current = null;
    }
    pendingAuthoritativeRefreshRef.current = {};
  }, []);

  const queueAuthoritativeRefresh = useCallback((requested: AuthoritativeRefresh) => {
    pendingAuthoritativeRefreshRef.current = {
      instance: pendingAuthoritativeRefreshRef.current.instance || requested.instance,
      activeExecutions: pendingAuthoritativeRefreshRef.current.activeExecutions ||
        requested.activeExecutions,
      traces: pendingAuthoritativeRefreshRef.current.traces || requested.traces,
    };
    if (authoritativeRefreshTimerRef.current) return;
    authoritativeRefreshTimerRef.current = setTimeout(() => {
      authoritativeRefreshTimerRef.current = null;
      const pending = pendingAuthoritativeRefreshRef.current;
      pendingAuthoritativeRefreshRef.current = {};
      if (pending.instance) void loadInstance?.();
      if (pending.activeExecutions) void loadActiveExecutions?.();
      if (pending.traces) void loadTraces?.();
    }, authoritativeRefreshDelayMillis);
  }, [loadInstance, loadActiveExecutions, loadTraces]);

  const openStream = useCallback(() => {
    if (!connected || !tabId || !csrfToken) return;
    if (closeStreamRef.current) {
      closeStreamRef.current();
      closeStreamRef.current = null;
    }
    const close = openActivityStream(
      {
        onActivity: (activity: Activity) => {
          lastCursorRef.current = activity.cursor;
          dispatch({ type: "stream-activity", activity });
          const executionStarted = activity.kind === "TRACE_STARTED";
          const traceCompleted = activity.kind === "TRACE_COMPLETED";
          const executionEnded = traceCompleted ||
            activity.kind === "EXECUTION_OBSERVATION_ENDED";
          queueAuthoritativeRefresh({
            activeExecutions: true,
            instance: executionStarted || executionEnded,
            traces: traceCompleted,
          });
        },
        onConnection: (fact: ConnectionFact) => {
          dispatch({ type: "stream-connection", fact });
          if (fact.connected) {
            dispatch({ type: "stream-connected" });
            queueAuthoritativeRefresh({
              instance: true,
              activeExecutions: true,
              traces: true,
            });
          }
        },
        onContinuity: (continuity: Continuity) => {
          if (intervalRef.current !== continuity.intervalId) {
            lastCursorRef.current = null;
            intervalRef.current = continuity.intervalId;
          }
          dispatch({ type: "stream-continuity", continuity });
        },
        onReplayGap: (reason: string) => {
          if (resyncingRef.current) return;
          resyncingRef.current = true;
          if (closeStreamRef.current) {
            closeStreamRef.current();
            closeStreamRef.current = null;
          }
          dispatch({ type: "stream-replay-gap", reason });
          const generation = generationRef.current;
          void fetchRecentActivities(undefined, { tabId, csrfToken })
            .then((response) => {
              if (generation !== generationRef.current) return;
              const latest = response.items.at(-1);
              lastCursorRef.current =
                latest?.cursor ?? response.continuity?.lastCursor ?? null;
              intervalRef.current = response.continuity?.intervalId ?? null;
              dispatch({
                type: "recent-success",
                items: response.items,
                hasMore: response.hasMore,
                nextCursor: response.nextCursor,
                append: false,
                continuity: response.continuity,
                coverage: response.coverage,
              });
              queueAuthoritativeRefresh({
                instance: true,
                activeExecutions: true,
                traces: true,
              });
              resyncingRef.current = false;
              openStreamRef.current?.();
            })
            .catch((error) => {
              if (generation !== generationRef.current) return;
              resyncingRef.current = false;
              const recovered = error instanceof BrowserAPIError
                ? error
                : new BrowserAPIError(
                  "CONSOLE_ERROR",
                  "Failed to resynchronize live activity.",
                  0,
                );
              dispatch({ type: "stream-error", error: recovered });
            });
        },
        onBaselineRefreshed: (observedAt?: string) => {
          dispatch({
            type: "baseline-refreshed",
            observedAt: observedAt ?? new Date().toISOString(),
          });
          queueAuthoritativeRefresh({
            instance: true,
            activeExecutions: true,
            traces: true,
          });
        },
        onError: (error: Error) => {
          if (resyncingRef.current) return;
          dispatch({
            type: "stream-error",
            error: new BrowserAPIError("CONSOLE_ERROR", error.message, 0),
          });
        },
        onClose: () => {
          if (resyncingRef.current) return;
          dispatch({ type: "stream-closed" });
        },
      },
      { tabId, csrfToken },
      lastCursorRef.current ?? undefined,
    );
    closeStreamRef.current = close;
  }, [connected, tabId, csrfToken, queueAuthoritativeRefresh]);
  openStreamRef.current = openStream;

  useEffect(() => {
    if (!connected || !tabId || !csrfToken) {
      if (closeStreamRef.current) {
        closeStreamRef.current();
        closeStreamRef.current = null;
      }
      return;
    }
    openStream();
    return () => {
      if (closeStreamRef.current) {
        closeStreamRef.current();
        closeStreamRef.current = null;
      }
    };
  }, [connected, targetScopeID, scopeGeneration, tabId, csrfToken, openStream]);

  useEffect(() => {
    if (state.reconnectAttempt <= 0 || !connected || !tabId || !csrfToken) return;
    const delay = Math.min(1000 * 2 ** (state.reconnectAttempt - 1), 30_000);
    const timer = setTimeout(() => {
      openStream();
    }, delay);
    return () => clearTimeout(timer);
  }, [state.reconnectAttempt, connected, tabId, csrfToken, openStream]);

  const loadRecent = useCallback(async (cursor?: string) => {
    const gen = generationRef.current;
    dispatch({ type: "recent-loading" });
    try {
      const security = tabId && csrfToken ? { tabId, csrfToken } : undefined;
      const response = await fetchRecentActivities(
        { cursor: cursor ?? undefined },
        security,
      );
      if (gen !== generationRef.current) return;
      dispatch({
        type: "recent-success",
        items: response.items,
        hasMore: response.hasMore,
        nextCursor: response.nextCursor,
        append: cursor != null,
        continuity: response.continuity,
        coverage: response.coverage,
      });
    } catch (error) {
      if (gen !== generationRef.current) return;
      const recovered = error instanceof BrowserAPIError
        ? error
        : new BrowserAPIError("CONSOLE_ERROR", "Failed to load recent activities.", 0);
      dispatch({ type: "recent-error", error: recovered });
    }
  }, [tabId, csrfToken]);

  const value = useMemo<ActivityContextValue>(
    () => ({ ...state, loadRecent }),
    [state, loadRecent],
  );

  return (
    <ActivityContext.Provider value={value}>
      {children}
    </ActivityContext.Provider>
  );
}

export function useActivity() {
  const value = useContext(ActivityContext);
  if (!value) throw new Error("ActivityProvider is missing");
  return value;
}

export function useOptionalActivity() {
  return useContext(ActivityContext);
}
