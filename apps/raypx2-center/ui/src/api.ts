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
    throw new Error(body?.message || body?.error || `Request failed (${response.status}).`);
  }
  return response.json() as Promise<T>;
}
