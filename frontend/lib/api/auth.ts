import { apiFetch, apiURL } from "./client";

export interface AuthUser {
  id: string;
  display_name: string;
  email: string;
  avatar_url: string;
  created_at: string;
  updated_at: string;
}

export const authApi = {
  me(): Promise<AuthUser> {
    return apiFetch<AuthUser>("/api/v1/auth/me");
  },

  async logout(): Promise<void> {
    await apiFetch<void>("/api/v1/auth/logout", { method: "POST" });
  },

  feishuStartURL(): string {
    return apiURL("/api/v1/auth/feishu/start");
  },
};
