import { describe, expect, it } from "vitest";
import {
  RATE_BOUNDS_ERROR,
  RATE_INTEGER_ERROR,
  connectionRowKey,
  parseRateBounds,
  rateField,
  readTotalStreamsOpened,
  serverPeerName,
} from "./connection-form-helpers";

describe("connection-form-helpers", () => {
  it("parses non-negative integer bounds", () => {
    expect(parseRateBounds("0", "0")).toEqual({
      min_send_rate_kbps: 0,
      max_send_rate_kbps: 0,
    });
    expect(parseRateBounds(" 100 ", "200")).toEqual({
      min_send_rate_kbps: 100,
      max_send_rate_kbps: 200,
    });
  });

  it("rejects non-integers", () => {
    expect(() => parseRateBounds("1.5", "2")).toThrow(RATE_INTEGER_ERROR);
    expect(() => parseRateBounds("-1", "2")).toThrow(RATE_INTEGER_ERROR);
    expect(() => parseRateBounds("abc", "1")).toThrow(RATE_INTEGER_ERROR);
    expect(() => parseRateBounds("1", "")).toThrow(RATE_INTEGER_ERROR);
    expect(() => parseRateBounds("9007199254740993", "1")).toThrow(RATE_INTEGER_ERROR);
  });

  it("rejects min greater than max when both are non-zero", () => {
    expect(() => parseRateBounds("20", "10")).toThrow(RATE_BOUNDS_ERROR);
  });

  it("allows min greater than max when either bound is zero", () => {
    expect(parseRateBounds("20", "0")).toEqual({
      min_send_rate_kbps: 20,
      max_send_rate_kbps: 0,
    });
    expect(parseRateBounds("0", "10")).toEqual({
      min_send_rate_kbps: 0,
      max_send_rate_kbps: 10,
    });
  });

  it("stringifies rate fields with a zero default", () => {
    expect(rateField(12)).toBe("12");
    expect(rateField("8")).toBe("8");
    expect(rateField(undefined)).toBe("0");
    expect(rateField("")).toBe("0");
  });

  it("builds a row key from peer_id and connection_id", () => {
    expect(connectionRowKey({ peer_id: "peer-a", connection_id: "conn-0" })).toBe("peer-a:conn-0");
    expect(connectionRowKey({ connection_id: "sc1" })).toBe("sc1");
  });

  it("names a server peer from client_name or remote_address", () => {
    expect(serverPeerName({ client_name: "edge-1", remote_address: "10.0.0.1:443" })).toBe("edge-1");
    expect(serverPeerName({ remote_address: "10.0.0.1:443" })).toBe("peer-10.0.0.1:443");
    expect(serverPeerName({})).toBe("peer-unknown");
    expect(serverPeerName({ client_name: "  ", remote_address: "  " })).toBe("peer-unknown");
  });

  it("reads total_streams_opened preferring the explicit field", () => {
    expect(readTotalStreamsOpened({ total_streams_opened: 9, total_streams: 3 })).toBe(9);
    expect(readTotalStreamsOpened({ total_streams: 3 })).toBe(3);
    expect(readTotalStreamsOpened({})).toBeUndefined();
  });
});
