import PocketBase from "pocketbase";
import type { CenterNode } from "./pages/Nodes";

export interface CreateNodeInput {
  node_key?: string;
  name: string;
  role: "client" | "server" | "unknown";
}

export interface CreateNodeResult {
  node: CenterNode;
  enroll_secret: string;
}

export interface AuditLog {
  id: string;
  action: string;
  ip?: string;
  created: string;
  request_summary?: Record<string, unknown>;
}

export const pb = new PocketBase(window.location.origin);

export async function login(identity: string, password: string) {
  return pb.collection("_superusers").authWithPassword(identity, password);
}

export function logout() {
  pb.authStore.clear();
}

export async function listNodes(): Promise<CenterNode[]> {
  const response = await centerRequest<{ items: CenterNode[] }>("/api/center/nodes");
  return response.items;
}

export function createNode(input: CreateNodeInput): Promise<CreateNodeResult> {
  return centerRequest("/api/center/nodes", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export function proxyNode<T = unknown>(
  nodeKey: string,
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  return centerRequest(`/api/center/nodes/${encodeURIComponent(nodeKey)}/proxy`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      method,
      path,
      headers: body === undefined ? {} : { "Content-Type": "application/json" },
      ...(body === undefined ? {} : { body }),
    }),
  });
}

export async function listAuditLogs(nodeId: string): Promise<AuditLog[]> {
  const result = await pb.collection("audit_logs").getList<AuditLog>(1, 100, {
    filter: pb.filter("node = {:node}", { node: nodeId }),
    sort: "-created",
  });
  return result.items;
}

async function centerRequest<T>(path: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(path, {
    ...init,
    headers: {
      ...init.headers,
      Authorization: pb.authStore.token,
    },
  });
  if (!response.ok) {
    if (response.status === 401) pb.authStore.clear();
    const body = await response.json().catch(() => null);
    const detail = body?.message || body?.code || body?.error || response.statusText || "Request failed";
    throw new Error(`${response.status}: ${detail}`);
  }
  if (response.status === 204) return undefined as T;
  return response.json() as Promise<T>;
}
