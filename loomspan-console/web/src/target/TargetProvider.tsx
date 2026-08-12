import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useMemo,
  useReducer,
} from "react";
import { useNavigate } from "react-router";
import { BrowserAPIError } from "../api/client";
import type { TargetResponse } from "../api/contracts";
import { useBrowserSession } from "../security/BrowserSessionProvider";
import { targetReducer } from "./targetReducer";

type TargetContextValue = {
  target: TargetResponse;
  error?: BrowserAPIError;
  scopeGeneration: number;
  defaults: { address: string; applicationKey: string };
  connect(address: string, key: string): Promise<void>;
  credential(key: string): Promise<void>;
  recheck(): Promise<void>;
  refresh(): Promise<void>;
};

const TargetContext = createContext<TargetContextValue | undefined>(undefined);

export function TargetProvider({
  initial,
  defaults = { address: "", applicationKey: "" },
  children,
}: {
  initial: TargetResponse;
  defaults?: { address: string; applicationKey: string };
  children: ReactNode;
}) {
  const session = useBrowserSession();
  const navigate = useNavigate();
  const [state, dispatch] = useReducer(targetReducer, {
    target: initial,
    generation: 0,
  });

  const apply = useCallback(
    (next: TargetResponse, preserveError = false) => {
      const changed =
        state.target.status.targetScopeId !== next.status.targetScopeId;
      const replacedExistingScope =
        Boolean(state.target.status.targetScopeId) && changed;
      dispatch({ type: "replace", target: next, preserveError });
      if (changed) {
        navigate("/", {
          replace: true,
          state: replacedExistingScope ? { staleTargetScope: true } : undefined,
        });
      }
    },
    [navigate, state.target.status.targetScopeId],
  );

  const perform = useCallback(
    async (operation: () => Promise<TargetResponse>) => {
      dispatch({ type: "clear-error" });
      try {
        apply(await operation());
      } catch (error) {
        if (error instanceof BrowserAPIError) {
          dispatch({ type: "error", error });
          if (error.code === "TARGET_CHANGED") {
            navigate("/", {
              replace: true,
              state: { staleTargetScope: true },
            });
          }
          try {
            apply(await session.readTargetStatus(), true);
          } catch {
            // Preserve the authoritative operation error.
          }
        } else {
          dispatch({
            type: "error",
            error: new BrowserAPIError(
              "CONSOLE_ERROR",
              "The Console operation could not be completed.",
              500,
            ),
          });
        }
        throw error;
      }
    },
    [apply, navigate, session],
  );

  const value = useMemo<TargetContextValue>(
    () => ({
      target: state.target,
      error: state.error,
      scopeGeneration: state.generation,
      defaults,
      connect: (address, key) =>
        perform(() => session.connectTarget(address, key)),
      credential: (key) => perform(() => session.supplyTargetCredential(key)),
      recheck: () => perform(session.recheckTarget),
      refresh: () => perform(session.readTargetStatus),
    }),
    [defaults, perform, session, state.error, state.target],
  );

  return (
    <TargetContext.Provider value={value}>
      <div key={state.generation}>{children}</div>
    </TargetContext.Provider>
  );
}

export function useTarget() {
  const value = useContext(TargetContext);
  if (!value) throw new Error("TargetProvider is missing");
  return value;
}
