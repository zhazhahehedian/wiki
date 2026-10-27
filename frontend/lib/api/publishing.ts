import { apiFetch } from "./client";
export const publishingScopes = ["capabilities:read", "capabilities:write", "capabilities:submit"] as const;
export type PublishingScope = typeof publishingScopes[number];
export interface PublishingCredential {
  id: string; name: string; prefix: string; scopes: PublishingScope[]; expires_at: string;
  created_at: string; revoked_at?: string; last_used_at?: string;
}
const root = "/api/v1/publishing/credentials";
export function listPublishingCredentials(signal?: AbortSignal) { return apiFetch<{items:PublishingCredential[]}>(root,{signal}); }
export function createPublishingCredential(input:{name:string;scopes:PublishingScope[];expires_at:string}) {
  return apiFetch<PublishingCredential & {token:string}>(root,{method:"POST",body:JSON.stringify(input)});
}
export function revokePublishingCredential(id:string) { return apiFetch<void>(`${root}/${encodeURIComponent(id)}`,{method:"DELETE"}); }
