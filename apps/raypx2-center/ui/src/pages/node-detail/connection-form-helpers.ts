import { itemId, type JsonObject } from "@/lib/node-utils";

export const RATE_INTEGER_ERROR =
  "Minimum and maximum send rates must be safe non-negative integers.";
export const RATE_BOUNDS_ERROR =
  "Minimum send rate must not exceed maximum send rate when both are non-zero.";

export type RateBounds = {
  min_send_rate_kbps: number;
  max_send_rate_kbps: number;
};

export function parseRateBounds(minText: string, maxText: string): RateBounds {
  const minValue = String(minText).trim();
  const maxValue = String(maxText).trim();
  if (!/^\d+$/.test(minValue) || !/^\d+$/.test(maxValue)) {
    throw new Error(RATE_INTEGER_ERROR);
  }
  const minRate = Number(minValue);
  const maxRate = Number(maxValue);
  if (!Number.isSafeInteger(minRate) || minRate < 0
    || !Number.isSafeInteger(maxRate) || maxRate < 0) {
    throw new Error(RATE_INTEGER_ERROR);
  }
  if (minRate !== 0 && maxRate !== 0 && minRate > maxRate) {
    throw new Error(RATE_BOUNDS_ERROR);
  }
  return { min_send_rate_kbps: minRate, max_send_rate_kbps: maxRate };
}

export function rateField(value: unknown): string {
  if (typeof value === "number" && Number.isFinite(value)) return String(value);
  if (typeof value === "string" && value !== "") return value;
  return "0";
}

export function connectionRowKey(connection: JsonObject): string {
  const peerId = typeof connection.peer_id === "string" ? connection.peer_id : "";
  const id = itemId(peerId ? { ...connection, peer_id: undefined } : connection);
  return peerId ? `${peerId}:${id}` : id;
}

export function serverPeerName(connection: JsonObject): string {
  const clientName = typeof connection.client_name === "string" ? connection.client_name.trim() : "";
  if (clientName) return clientName;
  const remote = typeof connection.remote_address === "string" ? connection.remote_address.trim() : "";
  return `peer-${remote || "unknown"}`;
}

export function readTotalStreamsOpened(connection: JsonObject): unknown {
  return connection.total_streams_opened ?? connection.total_streams;
}
