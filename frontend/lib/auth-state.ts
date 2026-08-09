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
  if (auth.isLoading || auth.isFetching || auth.isError || sessionExpired) {
    return undefined;
  }
  return auth.data;
}
