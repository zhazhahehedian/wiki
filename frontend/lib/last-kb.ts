const KEY = "it-wiki:last-kb-id";

export function getLastKbId(): string | null {
  if (typeof window === "undefined") return null;
  return localStorage.getItem(KEY);
}

export function setLastKbId(kbId: string): void {
  if (typeof window === "undefined") return;
  localStorage.setItem(KEY, kbId);
}

/** 仅当当前存储的就是这个 kbId 时才清除（删除 KB 后避免 Rail 链接指向死路由）。 */
export function clearLastKbId(kbId: string): void {
  if (typeof window === "undefined") return;
  if (localStorage.getItem(KEY) === kbId) localStorage.removeItem(KEY);
}
