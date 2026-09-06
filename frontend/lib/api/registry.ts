import { apiFetch, apiURL } from "@/lib/api/client";

export interface Capability {
  id: string;
  slug: string;
  type: "mcp" | "skill";
  name: string;
  description: string;
  owner_open_id: string;
  department: string;
  status: "draft" | "in_review" | "published" | "offline";
  visibility: "org" | "department" | "allowlist";
  is_live?: boolean;
  review_reason?: string;
  current_version_id?: string;
  draft_version_id?: string;
  revision: number;
  created_at: string;
  updated_at: string;
}
export interface MCPTool {
  name: string;
  description: string;
  inputSchema: Record<string, unknown>;
}
export interface CapabilityVersion {
  id: string;
  version: string;
  changelog: string;
  created_by: string;
  created_at: string;
  mcp_endpoint?: string;
  mcp_transport?: string;
  mcp_auth_scheme?: string;
  tools?: MCPTool[];
  skill_manifest?: { name: string; description: string };
  published_at?: string;
  has_bundle: boolean;
}
export interface CapabilityDetail extends Capability {
  versions: CapabilityVersion[];
  allowlist?: string[];
  is_owner?: boolean;
  is_admin?: boolean;
  published?: PublishedMetadata;
}
export interface CapabilityInput {
  slug: string;
  type: Capability["type"];
  name: string;
  description: string;
  department: string;
  visibility: Capability["visibility"];
  allowlist: string[];
  revision: number;
  version: string;
  changelog: string;
  mcp_endpoint: string;
  mcp_transport: string;
  mcp_auth_scheme: string;
  tools: MCPTool[];
}
export interface RegistryFilter {
  type?: string;
  status?: string;
  department?: string;
  q?: string;
  offset?: number;
}
const root = "/api/v1/capabilities";
export function listCapabilities(filter: RegistryFilter, signal?: AbortSignal) {
  const query = new URLSearchParams({ limit: "24" });
  for (const [key, value] of Object.entries(filter))
    if (value !== undefined && value !== "") query.set(key, String(value));
  return apiFetch<{ items: Capability[]; has_more: boolean }>(
    `${root}?${query}`,
    { signal },
  );
}
export function getCapability(slug: string, signal?: AbortSignal) {
  return apiFetch<CapabilityDetail>(`${root}/${encodeURIComponent(slug)}`, {
    signal,
  });
}
export function saveCapability(
  input: CapabilityInput,
  files: File[],
  slug?: string,
) {
  const body = new FormData();
  body.append("metadata", JSON.stringify(input));
  for (const file of files)
    body.append(
      "files",
      file,
      file.webkitRelativePath
        ? file.webkitRelativePath.split("/").slice(1).join("/")
        : file.name,
    );
  return apiFetch<CapabilityDetail>(
    slug ? `${root}/${encodeURIComponent(slug)}` : root,
    { method: slug ? "PUT" : "POST", body },
  );
}
export function skillBundleURL(slug: string, version: string) {
  return apiURL(
    `${root}/${encodeURIComponent(slug)}/versions/${encodeURIComponent(version)}/bundle`,
  );
}

export interface GovernanceIdentity {
  open_id: string;
  is_admin: boolean;
  department: string;
  revision: number;
  display_name?: string;
}
export interface PublishedMetadata {
  name: string;
  description: string;
  department: string;
  visibility: Capability["visibility"];
  allowlist?: string[];
}
export type GovernanceAction =
  | "submit"
  | "approve"
  | "reject"
  | "offline"
  | "new-draft"
  | "force-publish"
  | "visibility";
export interface ActionInput {
  revision: number;
  version_id: string;
  reason: string;
  visibility?: Capability["visibility"];
  department?: string;
  allowlist?: string[];
}
export interface AuditEntry {
  id: string;
  actor_open_id: string;
  action: string;
  target_type: string;
  target_id: string;
  detail: Record<string, unknown>;
  request_id: string;
  created_at: string;
}
export function getGovernanceIdentity(signal?: AbortSignal) {
  return apiFetch<GovernanceIdentity>("/api/v1/governance/me", { signal });
}
export function listCatalog(filter: RegistryFilter, signal?: AbortSignal) {
  return registryPage("/api/v1/catalog", filter, signal);
}
export function listReviews(filter: RegistryFilter, signal?: AbortSignal) {
  return registryPage("/api/v1/governance/reviews", filter, signal);
}
function registryPage(
  path: string,
  filter: RegistryFilter,
  signal?: AbortSignal,
) {
  const query = new URLSearchParams({ limit: "24" });
  for (const [key, value] of Object.entries(filter))
    if (value !== undefined && value !== "") query.set(key, String(value));
  return apiFetch<{ items: Capability[]; has_more: boolean }>(
    `${path}?${query}`,
    { signal },
  );
}
export function actOnCapability(
  slug: string,
  action: GovernanceAction,
  input: ActionInput,
) {
  return apiFetch<CapabilityDetail>(
    `${root}/${encodeURIComponent(slug)}/${action}`,
    { method: "POST", body: JSON.stringify(input) },
  );
}
export function getSkillFiles(
  slug: string,
  version: string,
  signal?: AbortSignal,
) {
  return apiFetch<{
    files: { name: string; size: number; content?: string }[];
  }>(
    `${root}/${encodeURIComponent(slug)}/versions/${encodeURIComponent(version)}/files`,
    { signal },
  );
}
export function listAudit(
  filter: { action?: string; actor?: string; target?: string; offset?: number },
  signal?: AbortSignal,
) {
  const query = new URLSearchParams({ limit: "24" });
  for (const [key, value] of Object.entries(filter))
    if (value !== undefined && value !== "") query.set(key, String(value));
  return apiFetch<{ items: AuditEntry[]; has_more: boolean }>(
    `/api/v1/governance/audit?${query}`,
    { signal },
  );
}
export function listProfiles(
  filter: { q?: string; offset?: number },
  signal?: AbortSignal,
) {
  const query = new URLSearchParams({ limit: "24" });
  for (const [key, value] of Object.entries(filter))
    if (value !== undefined && value !== "") query.set(key, String(value));
  return apiFetch<{ items: GovernanceIdentity[]; has_more: boolean }>(
    `/api/v1/governance/profiles?${query}`,
    { signal },
  );
}
export function setTrustedDepartment(
  profile: GovernanceIdentity,
  department: string,
  reason: string,
) {
  return apiFetch<GovernanceIdentity>(
    `/api/v1/governance/profiles/${encodeURIComponent(profile.open_id)}/department`,
    {
      method: "PUT",
      body: JSON.stringify({ department, revision: profile.revision, reason }),
    },
  );
}
