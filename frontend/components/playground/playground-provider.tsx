"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { useAuth } from "@/lib/hooks/use-auth";
import { useSessionExpired } from "@/components/auth/auth-session-boundary";
import { APIError } from "@/lib/api/client";
import {
  playgroundApi,
  type ModelConnection,
  type ConnectionInput,
  type ModelProtocol,
} from "@/lib/api/playground";
export type { ModelConnection } from "@/lib/api/playground";

const ConnectionContext = createContext<{
  connection: ModelConnection | null;
  loading: boolean;
  error: string;
  reload: () => void;
  saveConnection: (input: ConnectionInput) => Promise<void>;
  clearConnection: () => Promise<void>;
} | null>(null);

export function configurationError(error: unknown): string {
  if (error instanceof APIError && error.status === 409)
    return "配置已发生变化，请重新加载后保存。";
  if (error instanceof APIError && error.status === 400)
    return "请检查接口地址、密钥和模型。修改地址时需重新填写密钥。";
  return "配置服务暂时不可用，请重试。";
}

// Only redacted metadata enters shared React state; the key stays in the form
// until it is sent to the authenticated API, never browser storage or devtools.
export function PlaygroundProvider({
  children,
  enabled = true,
}: {
  children: ReactNode;
  enabled?: boolean;
}) {
  const [connection, setConnection] = useState<ModelConnection | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [revision, setRevision] = useState(0);
  const lifetime = useRef<AbortController | null>(null);
  useEffect(() => {
    const controller = new AbortController();
    lifetime.current = controller;
    setConnection(null);
    setError("");
    setLoading(enabled);
    if (enabled)
      void playgroundApi
        .get(controller.signal)
        .then((value) => {
          if (!controller.signal.aborted) setConnection(value);
        })
        .catch((reason) => {
          if (!controller.signal.aborted) setError(configurationError(reason));
        })
        .finally(() => {
          if (!controller.signal.aborted) setLoading(false);
        });
    return () => controller.abort();
  }, [enabled, revision]);
  const reload = useCallback(() => setRevision((value) => value + 1), []);
  async function saveConnection(input: ConnectionInput) {
    const controller = lifetime.current;
    if (!enabled || !controller || controller.signal.aborted)
      throw new Error("Session unavailable");
    const value = await playgroundApi.save(input, controller.signal);
    if (!controller.signal.aborted) {
      setConnection(value);
      setError("");
    }
  }
  async function clearConnection() {
    const controller = lifetime.current;
    if (!enabled || !controller || controller.signal.aborted)
      throw new Error("Session unavailable");
    await playgroundApi.delete(controller.signal);
    if (!controller.signal.aborted) {
      setConnection(null);
      setError("");
    }
  }
  return (
    <ConnectionContext.Provider
      value={{
        connection,
        loading,
        error,
        reload,
        saveConnection,
        clearConnection,
      }}
    >
      {children}
    </ConnectionContext.Provider>
  );
}

export function AuthenticatedPlaygroundProvider({
  children,
}: {
  children: ReactNode;
}) {
  const auth = useAuth();
  const expired = useSessionExpired();
  const owner = !expired && !auth.isError ? auth.data?.id : undefined;
  return (
    <PlaygroundProvider key={owner ?? "signed-out"} enabled={Boolean(owner)}>
      {children}
    </PlaygroundProvider>
  );
}
export function useModelConnection() {
  const value = useContext(ConnectionContext);
  if (!value) throw new Error("PlaygroundProvider is required");
  return value;
}
export function parseModels(value: string): string[] {
  return [
    ...new Set(
      value
        .split(/[\n,，]/)
        .map((model) => model.trim())
        .filter(Boolean),
    ),
  ];
}
export function normalizeBaseUrl(
  value: string,
  protocol: ModelProtocol = "openai",
): string | null {
  try {
    const url = new URL(value.trim());
    if (
      !["https:", "http:"].includes(url.protocol) ||
      url.username ||
      url.password ||
      url.search ||
      url.hash ||
      url.pathname.replace(/\/+$/, "").endsWith("/chat/completions")
    )
      return null;
    let base = url.toString().replace(/\/+$/, "");
    if (protocol === "anthropic") {
      base = base.replace(/\/v1$/, "");
      if (/\/(messages|models)$/.test(base)) return null;
    }
    return base;
  } catch {
    return null;
  }
}
