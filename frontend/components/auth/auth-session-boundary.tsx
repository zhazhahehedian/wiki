"use client";

import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { useQueryClient } from "@tanstack/react-query";

import { subscribeToUnauthorized } from "@/lib/api/client";

const SessionExpiredContext = createContext(false);

export function AuthSessionBoundary({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient();
  const [sessionExpired, setSessionExpired] = useState(false);

  useEffect(
    () => subscribeToUnauthorized((path) => {
      if (path === "/api/v1/auth/me") return;
      queryClient.removeQueries({ queryKey: ["auth"] });
      setSessionExpired(true);
    }),
    [queryClient],
  );

  return (
    <SessionExpiredContext.Provider value={sessionExpired}>
      {children}
    </SessionExpiredContext.Provider>
  );
}

export function useSessionExpired(): boolean {
  return useContext(SessionExpiredContext);
}
