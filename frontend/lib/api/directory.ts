import { apiFetch } from "./client";

export interface DirectoryDepartment { open_department_id: string; name: string; parent_department_id?: string }
export interface DirectoryUser { open_id: string; name: string }
export interface DirectoryPage<T> { items: T[]; has_more: boolean; page_token: string; limited_root?: boolean }
export function listDirectoryDepartments(parent = "0", page = "", signal?: AbortSignal) {
  const query = new URLSearchParams({ parent_id: parent, page_token: page });
  return apiFetch<DirectoryPage<DirectoryDepartment>>(`/api/v1/directory/departments?${query}`, { signal });
}
export function listDirectoryUsers(department = "0", page = "", signal?: AbortSignal) {
  const query = new URLSearchParams({ department_id: department, page_token: page });
  return apiFetch<DirectoryPage<DirectoryUser>>(`/api/v1/directory/users?${query}`, { signal });
}
