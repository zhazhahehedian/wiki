"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { authApi } from "@/lib/api/auth";

export const authQueryKey = ["auth", "me"] as const;

export function useAuth() {
  return useQuery({
    queryKey: authQueryKey,
    queryFn: () => authApi.me(),
    retry: false,
  });
}

export function useLogout() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => authApi.logout(),
    onSuccess: () => queryClient.removeQueries({ queryKey: ["auth"] }),
  });
}
