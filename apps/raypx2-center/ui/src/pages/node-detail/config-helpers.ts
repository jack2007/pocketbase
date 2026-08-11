import type { NodeConfigResponse } from "../../api";
import { isObject, type JsonObject, readStringArray } from "@/lib/node-utils";

export function connectionMetadata(response: NodeConfigResponse): JsonObject {
  if (!response.live || !isObject(response.live.connection_config)) return {};
  return response.live.connection_config;
}

export function parseJsonObject(value: string): JsonObject | null {
  try {
    const parsed: unknown = JSON.parse(value);
    return isObject(parsed) ? parsed : null;
  } catch {
    return null;
  }
}

export function jsonSnapshot(value: string): string {
  const parsed = parseJsonObject(value);
  return parsed ? stableStringify(parsed) : `invalid:${value}`;
}

export function stableStringify(value: unknown): string {
  return JSON.stringify(sortJson(value));
}

export function sortJson(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(sortJson);
  if (!isObject(value)) return value;
  return Object.fromEntries(
    Object.keys(value)
      .sort()
      .filter((key) => value[key] !== undefined)
      .map((key) => [key, sortJson(value[key])]),
  );
}

export function readPath(value: JsonObject, path: string[]): unknown {
  let current: unknown = value;
  for (const key of path) {
    if (!isObject(current)) return undefined;
    current = current[key];
  }
  return current;
}

export function setPath(value: JsonObject, path: string[], next: unknown): JsonObject {
  const [key, ...rest] = path;
  if (!key) return value;
  return {
    ...value,
    [key]: rest.length === 0
      ? next
      : setPath(isObject(value[key]) ? value[key] : {}, rest, next),
  };
}

export function updatePeer(draft: JsonObject, index: number, key: string, value: unknown): JsonObject {
  const peers = Array.isArray(draft.peers) ? [...draft.peers] : [];
  const peer = isObject(peers[index]) ? peers[index] : {};
  peers[index] = { ...peer, [key]: value };
  return { ...draft, peers };
}

export function addDraftPeer(draft: JsonObject): JsonObject {
  const peers = Array.isArray(draft.peers) ? [...draft.peers] : [];
  peers.push({
    _draft_new: true,
    peer_id: "",
    client_name: "",
    quic_peer: "",
    socks_listen: "",
    http_listen: "",
    quic_connections: 1,
    port_forwards: [],
    enabled: true,
  });
  return { ...draft, peers };
}

export function removeDraftPeer(draft: JsonObject, index: number): JsonObject {
  const peers = Array.isArray(draft.peers) ? [...draft.peers] : [];
  const peer = isObject(peers[index]) ? peers[index] : null;
  if (!peer || peer._draft_new !== true) return draft;
  peers.splice(index, 1);
  return { ...draft, peers };
}

export function stripDraftPeerMarkers(content: JsonObject): JsonObject {
  if (!Array.isArray(content.peers)) return content;
  return {
    ...content,
    peers: content.peers.map((raw) => {
      if (!isObject(raw)) return raw;
      const { _draft_new: _ignored, ...peer } = raw;
      return peer;
    }),
  };
}

export function updatePeerPortForward(
  draft: JsonObject,
  peerIndex: number,
  forwardIndex: number,
  key: "listen" | "target",
  value: string,
): JsonObject {
  const peers = Array.isArray(draft.peers) ? [...draft.peers] : [];
  const peer = isObject(peers[peerIndex]) ? peers[peerIndex] : {};
  const forwards = Array.isArray(peer.port_forwards) ? [...peer.port_forwards] : [];
  const forward = isObject(forwards[forwardIndex]) ? forwards[forwardIndex] : {};
  forwards[forwardIndex] = { ...forward, [key]: value };
  peers[peerIndex] = { ...peer, port_forwards: forwards };
  return { ...draft, peers };
}

export function addPeerPortForward(draft: JsonObject, peerIndex: number): JsonObject {
  const peers = Array.isArray(draft.peers) ? [...draft.peers] : [];
  const peer = isObject(peers[peerIndex]) ? peers[peerIndex] : {};
  const forwards = Array.isArray(peer.port_forwards) ? [...peer.port_forwards] : [];
  forwards.push({ listen: "", target: "" });
  peers[peerIndex] = { ...peer, port_forwards: forwards };
  return { ...draft, peers };
}

export function removePeerPortForward(draft: JsonObject, peerIndex: number, forwardIndex: number): JsonObject {
  const peers = Array.isArray(draft.peers) ? [...draft.peers] : [];
  const peer = isObject(peers[peerIndex]) ? peers[peerIndex] : {};
  const forwards = Array.isArray(peer.port_forwards) ? [...peer.port_forwards] : [];
  forwards.splice(forwardIndex, 1);
  peers[peerIndex] = { ...peer, port_forwards: forwards };
  return { ...draft, peers };
}

export function validateConfig(role: string, draft: JsonObject): string {
  if (role === "server") {
    const level = readPath(draft, ["connection", "compression", "level"]);
    if (level === undefined) return "";
    if (typeof level !== "number" || !Number.isInteger(level) || level < 1 || level > 22) {
      return "Compression level must be an integer between 1 and 22.";
    }
    return "";
  }
  if (role !== "client") return "";
  const peers = Array.isArray(draft.peers) ? draft.peers.filter(isObject) : [];
  const seen = new Set<string>();
  for (const peer of peers) {
    const peerId = typeof peer.peer_id === "string" ? peer.peer_id.trim() : "";
    if (!peerId) return "Each peer requires a non-empty Peer ID.";
    if (seen.has(peerId)) return `Duplicate Peer ID “${peerId}”.`;
    seen.add(peerId);
  }
  return "";
}

export function displayInput(value: unknown): string | number {
  return typeof value === "string" || typeof value === "number" ? value : "";
}

export function isNodeOfflineError(cause: unknown): boolean {
  if (!isObject(cause)) return false;
  const status = cause.status;
  const dataCode = isObject(cause.data) ? cause.data.code : undefined;
  const code = cause.code ?? dataCode;
  return code === "node_offline" && (status === 409 || status === 503);
}

export function refreshWarningMessage(cause: unknown): string {
  const detail = cause instanceof Error ? cause.message : "Unknown error";
  return `Configuration saved, but metadata could not be refreshed (${detail}). Use Refresh to reload.`;
}

export { readStringArray };
