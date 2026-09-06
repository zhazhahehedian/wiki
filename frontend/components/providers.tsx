"use client";

import { useState, type ReactNode } from "react";
import { ThemeProvider } from "next-themes";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ReactQueryDevtools } from "@tanstack/react-query-devtools";
import { AuthSessionBoundary } from "@/components/auth/auth-session-boundary";
import { Toaster } from "@/components/ui/sonner";
import { TooltipProvider } from "@/components/ui/tooltip";

export function Providers({ children }: { children: ReactNode }) {
  const [client] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: {
            staleTime: 5_000,
            retry: 1,
            refetchOnWindowFocus: false,
          },
        },
      }),
  );

  return (
    <ThemeProvider attribute="class" defaultTheme="light" forcedTheme="light" enableSystem={false} disableTransitionOnChange>
      <TooltipProvider>
        <QueryClientProvider client={client}>
          <AuthSessionBoundary>
            {children}
            <Toaster richColors position="top-right" />
            <ReactQueryDevtools initialIsOpen={false} />
          </AuthSessionBoundary>
        </QueryClientProvider>
      </TooltipProvider>
    </ThemeProvider>
  );
}
