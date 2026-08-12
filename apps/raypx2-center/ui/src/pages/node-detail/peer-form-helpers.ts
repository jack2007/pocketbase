import type { JsonObject } from "@/lib/node-utils";

export type PeerConnection = {
  encryption: string;
  compression: { mode: string; level: number };
};

export const DEFAULT_CONNECTION: PeerConnection = {
  encryption: "enabled",
  compression: { mode: "disabled", level: 1 },
};

export type PeerFormState = {
  peer_id: string;
  quic_peer: string;
  connections: string;
  socks_listen: string;
  http_listen: string;
  enabled: boolean;
  encryption: string;
  compression_mode: string;
  compression_level: string;
  applied_encryption: string;
  applied_compression_mode: string;
  applied_compression_level: string;
  restart_required: boolean;
  paths: string;
  port_forwards: string;
};

function asObject(value: unknown): JsonObject | undefined {
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as JsonObject)
    : undefined;
}

function normalizeConnection(raw: unknown): PeerConnection {
  const obj = asObject(raw) ?? {};
  const compression = asObject(obj.compression) ?? {};
  const level = Number(compression.level);
  return {
    encryption: obj.encryption === "disabled" ? "disabled" : "enabled",
    compression: {
      mode: compression.mode === "enabled" ? "enabled" : "disabled",
      level: Number.isFinite(level) && level > 0 ? level : 1,
    },
  };
}

export function readPeerConnections(peer: JsonObject): number {
  const value = peer.proto_connections ?? peer.quic_connections ?? peer.connection_count ?? 1;
  const n = Number(value);
  return Number.isFinite(n) && n > 0 ? n : 1;
}

export function readDesiredConnection(peer: JsonObject): PeerConnection {
  const config = asObject(peer.connection_config);
  const desired = config ? config.desired : undefined;
  if (desired !== undefined) return normalizeConnection(desired);
  if (peer.connection !== undefined) return normalizeConnection(peer.connection);
  return { ...DEFAULT_CONNECTION, compression: { ...DEFAULT_CONNECTION.compression } };
}

export function readAppliedConnection(peer: JsonObject): PeerConnection {
  const config = asObject(peer.connection_config);
  if (config?.applied !== undefined) return normalizeConnection(config.applied);
  return { ...DEFAULT_CONNECTION, compression: { ...DEFAULT_CONNECTION.compression } };
}

export function readRestartRequired(peer: JsonObject): boolean {
  const config = asObject(peer.connection_config);
  return config?.restart_required === true;
}

export function validateCompressionLevel(value: string | number): string | null {
  const n = typeof value === "number" ? value : Number(value);
  if (!Number.isInteger(n) || n < 1 || n > 22) {
    return "Compression level must be an integer from 1 to 22.";
  }
  return null;
}

export function emptyPeerForm(): PeerFormState {
  return {
    peer_id: "",
    quic_peer: "",
    connections: "1",
    socks_listen: "127.0.0.1:1080",
    http_listen: "127.0.0.1:8080",
    enabled: true,
    encryption: DEFAULT_CONNECTION.encryption,
    compression_mode: DEFAULT_CONNECTION.compression.mode,
    compression_level: String(DEFAULT_CONNECTION.compression.level),
    applied_encryption: DEFAULT_CONNECTION.encryption,
    applied_compression_mode: DEFAULT_CONNECTION.compression.mode,
    applied_compression_level: String(DEFAULT_CONNECTION.compression.level),
    restart_required: false,
    paths: "[]",
    port_forwards: "[]",
  };
}

export function peerToForm(peer: JsonObject): PeerFormState {
  const desired = readDesiredConnection(peer);
  const applied = readAppliedConnection(peer);
  return {
    peer_id: String(peer.peer_id ?? ""),
    quic_peer: String(peer.quic_peer ?? peer.address ?? ""),
    connections: String(readPeerConnections(peer)),
    socks_listen: String(peer.socks_listen ?? "127.0.0.1:1080"),
    http_listen: String(peer.http_listen ?? "127.0.0.1:8080"),
    enabled: peer.enabled !== false && peer.state !== "disabled",
    encryption: desired.encryption,
    compression_mode: desired.compression.mode,
    compression_level: String(desired.compression.level),
    applied_encryption: applied.encryption,
    applied_compression_mode: applied.compression.mode,
    applied_compression_level: String(applied.compression.level),
    restart_required: readRestartRequired(peer),
    paths: JSON.stringify(peer.paths ?? [], null, 2),
    port_forwards: JSON.stringify(peer.port_forwards ?? [], null, 2),
  };
}

export function buildPeerSavePayload(form: PeerFormState): JsonObject {
  const levelError = validateCompressionLevel(form.compression_level);
  if (levelError) throw new Error(levelError);
  return {
    peer_id: form.peer_id.trim(),
    quic_peer: form.quic_peer.trim(),
    quic_connections: Number(form.connections) || 1,
    socks_listen: form.socks_listen.trim(),
    http_listen: form.http_listen.trim(),
    enabled: form.enabled,
    paths: JSON.parse(form.paths || "[]"),
    port_forwards: JSON.parse(form.port_forwards || "[]"),
    connection: {
      encryption: form.encryption === "disabled" ? "disabled" : "enabled",
      compression: {
        mode: form.compression_mode === "enabled" ? "enabled" : "disabled",
        level: Number(form.compression_level),
      },
    },
  };
}
