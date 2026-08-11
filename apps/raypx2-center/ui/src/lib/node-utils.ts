export type JsonObject = Record<string, unknown>;

export function isObject(value: unknown): value is JsonObject {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

export function itemsFrom(value: unknown, key: string): JsonObject[] {
  if (Array.isArray(value)) return value.filter(isObject);
  if (!isObject(value)) return [];
  const candidate = value[key] ?? value.items;
  return Array.isArray(candidate) ? candidate.filter(isObject) : [];
}

export function itemId(item: JsonObject): string {
  const value = item.id ?? item.peer_id ?? item.connection_id ?? item.tunnel_id;
  return typeof value === "string" ? value : "";
}

export function display(value: unknown): string {
  if (value === undefined || value === null || value === "") return "—";
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}

export function errorMessage(cause: unknown): string {
  return cause instanceof Error ? cause.message : "Operation failed.";
}

export function formatDate(value?: string) {
  if (!value) return "Never";
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleString();
}

export function readStringArray(value: unknown): string[] {
  return Array.isArray(value) ? value.filter((item): item is string => typeof item === "string") : [];
}

export function lines(value: string): string[] {
  return value.split(/[\n,]+/).map((item) => item.trim()).filter(Boolean);
}

export function sumField(items: JsonObject[], key: string): number {
  return items.reduce((total, item) => {
    const value = item[key];
    return total + (typeof value === "number" ? value : 0);
  }, 0);
}
