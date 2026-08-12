import { describe, expect, it } from "vitest";
import {
  buildPeerSavePayload,
  emptyPeerForm,
  peerToForm,
  readAppliedConnection,
  readDesiredConnection,
  readPeerConnections,
  readRestartRequired,
  validateCompressionLevel,
} from "./peer-form-helpers";

describe("peer-form-helpers", () => {
  it("reads connections preferring proto_connections", () => {
    expect(readPeerConnections({
      proto_connections: 4,
      quic_connections: 2,
      connection_count: 8,
    })).toBe(4);
    expect(readPeerConnections({ quic_connections: 2 })).toBe(2);
    expect(readPeerConnections({})).toBe(1);
  });

  it("reads desired from connection_config.desired then peer.connection", () => {
    expect(readDesiredConnection({
      connection_config: {
        desired: { encryption: "disabled", compression: { mode: "enabled", level: 5 } },
      },
    }).encryption).toBe("disabled");
    expect(readDesiredConnection({
      connection: { encryption: "disabled", compression: { mode: "enabled", level: 3 } },
    }).compression.level).toBe(3);
    expect(readDesiredConnection({}).encryption).toBe("enabled");
  });

  it("falls back to peer.connection when desired is null", () => {
    expect(readDesiredConnection({
      connection_config: { desired: null },
      connection: {
        encryption: "disabled",
        compression: { mode: "enabled", level: 3 },
      },
    })).toEqual({
      encryption: "disabled",
      compression: { mode: "enabled", level: 3 },
    });
  });

  it("reads applied and restart_required from connection_config", () => {
    const peer = {
      connection_config: {
        applied: { encryption: "disabled", compression: { mode: "enabled", level: 7 } },
        restart_required: true,
      },
    };
    expect(readAppliedConnection(peer).compression.level).toBe(7);
    expect(readRestartRequired(peer)).toBe(true);
    expect(readRestartRequired({})).toBe(false);
  });

  it("validates compression level 1-22 integer", () => {
    expect(validateCompressionLevel(1)).toBeNull();
    expect(validateCompressionLevel("22")).toBeNull();
    expect(validateCompressionLevel(0)).toMatch(/1 to 22/i);
    expect(validateCompressionLevel(1.5)).toMatch(/1 to 22/i);
  });

  it("builds save payload with connection object", () => {
    const form = {
      ...emptyPeerForm(),
      peer_id: "peer-a",
      quic_peer: "edge:443",
      connections: "3",
      encryption: "disabled",
      compression_mode: "enabled",
      compression_level: "5",
      paths: '[{"name":"p1"}]',
      port_forwards: "[]",
    };
    expect(buildPeerSavePayload(form)).toEqual({
      peer_id: "peer-a",
      quic_peer: "edge:443",
      quic_connections: 3,
      socks_listen: "127.0.0.1:1080",
      http_listen: "127.0.0.1:8080",
      enabled: true,
      paths: [{ name: "p1" }],
      port_forwards: [],
      connection: {
        encryption: "disabled",
        compression: { mode: "enabled", level: 5 },
      },
    });
  });

  it("maps peer list row into form state including applied readouts", () => {
    const form = peerToForm({
      peer_id: "peer-a",
      proto_connections: 2,
      socks_listen: "127.0.0.1:1081",
      http_listen: "127.0.0.1:8081",
      enabled: false,
      paths: [{ name: "x" }],
      port_forwards: [{ listen: ":9", target: "1:2" }],
      connection_config: {
        desired: { encryption: "disabled", compression: { mode: "enabled", level: 4 } },
        applied: { encryption: "enabled", compression: { mode: "disabled", level: 1 } },
        restart_required: true,
      },
    });
    expect(form.peer_id).toBe("peer-a");
    expect(form.connections).toBe("2");
    expect(form.encryption).toBe("disabled");
    expect(form.compression_mode).toBe("enabled");
    expect(form.compression_level).toBe("4");
    expect(form.applied_encryption).toBe("enabled");
    expect(form.applied_compression_mode).toBe("disabled");
    expect(form.applied_compression_level).toBe("1");
    expect(form.restart_required).toBe(true);
    expect(form.enabled).toBe(false);
  });

  it("maps address to quic_peer when quic_peer is absent", () => {
    expect(peerToForm({ address: "edge.example:443" }).quic_peer)
      .toBe("edge.example:443");
  });
});
