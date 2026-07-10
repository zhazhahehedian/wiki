const KEY = "it-wiki:last-kb-id";

export function getLastKbId(): string | null {
  if (typeof window === "undefined") return null;
  return localStorage.getItem(KEY);
}

export function setLastKbId(kbId: string): void {
  if (typeof window === "undefined") return;
  localStorage.setItem(KEY, kbId);
}
