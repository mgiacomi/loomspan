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
  bootstrap,
  BrowserAPIError,
  exchangePairing,
  heartbeatTab,
  releaseTab,
  requestManualPairing,
  targetStatus,
  connectTarget,
  supplyTargetCredential,
  recheckTarget,
} from "../api/client";
import type { TargetResponse } from "../api/contracts";
import {
  sessionReducer,
  type BrowserSessionState,
} from "./sessionReducer";

const tabHeartbeatIntervalMilliseconds = 60_000;

type SecurityState = {
  tabId: string;
  csrfToken: string;
};

type BrowserSessionContextValue = BrowserSessionState & {
  pair(secret: string): Promise<void>;
  requestManualChallenge(): Promise<void>;
  readTargetStatus(): Promise<TargetResponse>;
  connectTarget(address: string, key: string): Promise<TargetResponse>;
  supplyTargetCredential(key: string): Promise<TargetResponse>;
  recheckTarget(): Promise<TargetResponse>;
  getSecurity(): SecurityState | undefined;
};

const BrowserSessionContext = createContext<BrowserSessionContextValue>({
  status: "paired",
  bootstrap: {
    processId: "test",
    consoleVersion: "0.1.0-SNAPSHOT",
    workspacePath: "test-workspace",
    tabId: "test-tab",
    csrfToken: "test-token",
    targetFormDefaults: { address: "", applicationKey: "" },
    target: {
      unencrypted: false,
      status: {
        observedAt: "2026-07-27T00:00:00Z",
        targetSelection: "NONE",
        targetConnection: "NOT_APPLICABLE",
        targetAuthentication: "NOT_APPLICABLE",
        javaGoCompatibility: "NOT_APPLICABLE",
        runtimeIdentity: "NOT_APPLICABLE",
        liveMonitoring: "NOT_APPLICABLE",
      },
    },
  },
  async pair() {},
  async requestManualChallenge() {},
  async readTargetStatus() { return this.bootstrap.target; },
  async connectTarget() { return this.bootstrap.target; },
  async supplyTargetCredential() { return this.bootstrap.target; },
  async recheckTarget() { return this.bootstrap.target; },
  getSecurity() { return { tabId: this.bootstrap.tabId, csrfToken: this.bootstrap.csrfToken }; },
});

export function BrowserSessionProvider({
  initialPairingSecret,
  children,
}: {
  initialPairingSecret?: string;
  children: ReactNode;
}) {
  const [state, dispatch] = useReducer(sessionReducer, { status: "loading" });
  const started = useRef(false);
  const currentSecurity = useRef<{ tabId: string; csrfToken: string } | undefined>(
    undefined,
  );
  const heartbeatRunning = useRef(false);
  const pageActive = useRef(true);

  const load = useCallback(async (secret?: string) => {
    const pairingAttempt = Boolean(secret);
    let pairingRejected = false;
    dispatch({ type: "loading" });
    try {
      if (secret) {
        try {
          await exchangePairing(secret);
        } catch {
          pairingRejected = true;
        }
      }
      secret = undefined;
      const next = await bootstrap(currentSecurity.current?.tabId);
      currentSecurity.current = { tabId: next.tabId, csrfToken: next.csrfToken };
      dispatch({ type: "paired", bootstrap: next });
    } catch (error) {
      currentSecurity.current = undefined;
      const message =
        error instanceof BrowserAPIError && error.code === "LIMIT_EXCEEDED"
          ? "This Console has reached its browser capacity."
          : pairingAttempt && pairingRejected
            ? "This pairing link is invalid or expired."
            : undefined;
      dispatch({ type: "unpaired", message });
    }
  }, []);

  useEffect(() => {
    if (started.current) return;
    started.current = true;
    let secret = initialPairingSecret;
    void load(secret).finally(() => {
      secret = undefined;
    });
  }, [initialPairingSecret, load]);

  useEffect(() => {
    const keepAlive = async () => {
      const security = currentSecurity.current;
      if (!pageActive.current || !security || heartbeatRunning.current) return;
      heartbeatRunning.current = true;
      try {
        await heartbeatTab(security);
      } catch (error) {
        if (
          error instanceof BrowserAPIError &&
          pageActive.current &&
          (error.code === "BROWSER_SECURITY_REJECTED" ||
            error.code === "SESSION_REQUIRED")
        ) {
          await load();
        }
      } finally {
        heartbeatRunning.current = false;
      }
    };
    pageActive.current = true;
    const interval = window.setInterval(() => void keepAlive(), tabHeartbeatIntervalMilliseconds);
    const resume = () => {
      if (document.visibilityState === "visible") void keepAlive();
    };
    const restore = (event: PageTransitionEvent) => {
      if (event.persisted) void keepAlive();
    };
    document.addEventListener("visibilitychange", resume);
    window.addEventListener("pageshow", restore);
    return () => {
      pageActive.current = false;
      window.clearInterval(interval);
      document.removeEventListener("visibilitychange", resume);
      window.removeEventListener("pageshow", restore);
    };
  }, [load]);

  useEffect(() => {
    const dispose = (event: PageTransitionEvent) => {
      if (event.persisted) return;
      pageActive.current = false;
      const security = currentSecurity.current;
      if (security) void releaseTab(security).catch(() => {});
      currentSecurity.current = undefined;
    };
    window.addEventListener("pagehide", dispose);
    return () => window.removeEventListener("pagehide", dispose);
  }, []);

  const value = useMemo<BrowserSessionContextValue>(
    () => ({
      ...state,
      pair: async (secret: string) => {
        await load(secret);
      },
      requestManualChallenge: async () => {
        await requestManualPairing();
      },
      readTargetStatus: targetStatus,
      connectTarget: async (address: string, key: string) => {
        const security = currentSecurity.current;
        if (!security) throw new BrowserAPIError("SESSION_REQUIRED", "Pairing is required.", 401);
        return connectTarget(address, key, security);
      },
      supplyTargetCredential: async (key: string) => {
        const security = currentSecurity.current;
        if (!security) throw new BrowserAPIError("SESSION_REQUIRED", "Pairing is required.", 401);
        return supplyTargetCredential(key, security);
      },
      recheckTarget: async () => {
        const security = currentSecurity.current;
        if (!security) throw new BrowserAPIError("SESSION_REQUIRED", "Pairing is required.", 401);
        return recheckTarget(security);
      },
      getSecurity: () => currentSecurity.current,
    }),
    [load, state],
  );
  return (
    <BrowserSessionContext.Provider value={value}>
      {children}
    </BrowserSessionContext.Provider>
  );
}

export function useBrowserSession() {
  return useContext(BrowserSessionContext);
}
