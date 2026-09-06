import type { AuthUser } from "@/lib/api/auth";

interface AuthQueryState {
  data: AuthUser | undefined;
  isLoading: boolean;
  isFetching: boolean;
  isError: boolean;
}

export function getAuthenticatedUser(
  auth: AuthQueryState,
  sessionExpired = false,
): AuthUser | undefined {
  // Background refresh keeps the last confirmed session visible; errors and
  // explicit expiry still block protected UI.
  if (auth.isLoading || auth.isError || sessionExpired) {
    return undefined;
  }
  return auth.data;
}
