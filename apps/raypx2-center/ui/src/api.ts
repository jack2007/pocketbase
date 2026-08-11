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

export interface ConfigTemplate {
  id: string;
  name: string;
  target_role: "client" | "server";
  body: Record<string, unknown>;
  version: number;
  notes?: string;
}

export interface ApplyJobTarget {
  id: string;
  node: string;
  status: string;
  error?: string;
  result_revision?: string;
}

export interface ApplyJob {
  id: string;
  template: string;
  template_version: number;
  status: string;
  targets: ApplyJobTarget[];
}

export interface ConfigRevision {
  id: string;
  kind: "actual" | "desired";
  source: string;
  created: string;
}

export interface NodeConfigResponse {
  node_key: string;
  role: string;
  online: boolean;
  live: Record<string, unknown> | null;
  editor_draft: Record<string, unknown>;
  writable_paths: string[];
  recent_revisions: ConfigRevision[];
}

export interface NodeConfigUpdateResult {
  applied: Record<string, unknown>;
  ignored_fields: string[];
  revision_id: string;
  admin_status: number;
}

export class CenterRequestError extends Error {
  status: number;
  code?: string;
  data: Record<string, unknown> | null;

  constructor(status: number, message: string, data: Record<string, unknown> | null) {
    super(`${status}: ${message}`);
    this.name = "CenterRequestError";
    this.status = status;
    this.code = typeof data?.code === "string" ? data.code : undefined;
    this.data = data;
  }
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

export async function deleteNode(nodeKey: string): Promise<void> {
  await centerRequest(`/api/center/nodes/${encodeURIComponent(nodeKey)}`, { method: "DELETE" });
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

export async function listTemplates(): Promise<ConfigTemplate[]> {
  const response = await centerRequest<{ items: ConfigTemplate[] }>("/api/center/templates");
  return response.items;
}

export function createTemplate(input: Omit<ConfigTemplate, "id" | "version">): Promise<ConfigTemplate> {
  return centerRequest("/api/center/templates", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export function updateTemplate(
  id: string,
  input: Omit<ConfigTemplate, "id" | "version">,
): Promise<ConfigTemplate> {
  return centerRequest(`/api/center/templates/${encodeURIComponent(id)}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export async function deleteTemplate(id: string): Promise<void> {
  await centerRequest(`/api/center/templates/${encodeURIComponent(id)}`, { method: "DELETE" });
}

export async function listApplyJobs(): Promise<ApplyJob[]> {
  const response = await centerRequest<{ items: ApplyJob[] }>("/api/center/apply-jobs");
  return response.items;
}

export function createApplyJob(template: string, nodes: string[]): Promise<ApplyJob> {
  return centerRequest("/api/center/apply-jobs", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ template, nodes }),
  });
}

export async function listConfigRevisions(nodeId: string): Promise<ConfigRevision[]> {
  const result = await pb.collection("config_revisions").getList<ConfigRevision>(1, 100, {
    filter: pb.filter("node = {:node}", { node: nodeId }),
    sort: "-created",
  });
  return result.items;
}

export function getNodeConfig(nodeKey: string): Promise<NodeConfigResponse> {
  return centerRequest(`/api/center/nodes/${encodeURIComponent(nodeKey)}/config`);
}

export function putNodeConfig(
  nodeKey: string,
  content: Record<string, unknown>,
): Promise<NodeConfigUpdateResult> {
  return centerRequest(`/api/center/nodes/${encodeURIComponent(nodeKey)}/config`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ content }),
  });
}

export interface DeleteNodePeerResult {
  peer_id: string;
  revision_id: string;
  admin_status: number;
}

export function deleteNodePeer(nodeKey: string, peerId: string): Promise<DeleteNodePeerResult> {
  return centerRequest(
    `/api/center/nodes/${encodeURIComponent(nodeKey)}/peers/${encodeURIComponent(peerId)}`,
    { method: "DELETE" },
  );
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
    const rawBody: unknown = await response.json().catch(() => null);
    const body = typeof rawBody === "object" && rawBody !== null && !Array.isArray(rawBody)
      ? rawBody as Record<string, unknown>
      : null;
    const detail = body?.message || body?.code || body?.error || response.statusText || "Request failed";
    throw new CenterRequestError(response.status, String(detail), body);
  }
  if (response.status === 204) return undefined as T;
  return response.json() as Promise<T>;
}
