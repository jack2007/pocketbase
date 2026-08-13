import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import * as api from "../api";
import { NodeDetail } from "./NodeDetail";
import type { CenterNode } from "./Nodes";

vi.mock("../api", async () => {
  const actual = await vi.importActual<typeof import("../api")>("../api");
  return {
    ...actual,
    deleteNodePeer: vi.fn(),
    listAuditLogs: vi.fn().mockResolvedValue([]),
    proxyNode: vi.fn().mockResolvedValue({}),
  };
});

const offlineServer: CenterNode = {
  id: "node-1",
  node_key: "server-sg-1",
  name: "Singapore server",
  role: "server",
  online: false,
};

const onlineServer = { ...offlineServer, online: true };

describe("NodeDetail", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("shows the offline write notice on Peers", () => {
    render(<NodeDetail node={offlineServer} onBack={() => undefined} />);

    fireEvent.click(screen.getByRole("tab", { name: "Peers" }));

    expect(screen.getByText("Writes are disabled while this node is offline.")).toBeInTheDocument();
  });

  it("deletes a client peer from Peers after confirmation", async () => {
    const node: CenterNode = {
      id: "node-c1",
      node_key: "client-1",
      name: "Client one",
      role: "client",
      online: true,
    };
    vi.mocked(api.proxyNode).mockImplementation(async (_key, _method, path) => {
      if (path === "/api/v1/peers") {
        return { peers: [{ peer_id: "peer-a", state: "connected", address: "edge:443" }] };
      }
      return {};
    });
    vi.mocked(api.deleteNodePeer).mockResolvedValue({
      peer_id: "peer-a",
      revision_id: "rev-del",
      admin_status: 200,
    });

    render(<NodeDetail node={node} onBack={() => undefined} />);
    fireEvent.click(screen.getByRole("tab", { name: "Peers" }));
    expect(await screen.findByText("peer-a")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Delete" }));
    fireEvent.click(within(screen.getByRole("alertdialog")).getByRole("button", { name: "Delete" }));

    await waitFor(() => {
      expect(api.deleteNodePeer).toHaveBeenCalledWith("client-1", "peer-a");
    });
  });

  it("does not delete a client peer when confirmation is cancelled", async () => {
    const node: CenterNode = {
      id: "node-c1",
      node_key: "client-1",
      name: "Client one",
      role: "client",
      online: true,
    };
    vi.mocked(api.proxyNode).mockImplementation(async (_key, _method, path) => {
      if (path === "/api/v1/peers") {
        return { peers: [{ peer_id: "peer-a", state: "connected" }] };
      }
      return {};
    });

    render(<NodeDetail node={node} onBack={() => undefined} />);
    fireEvent.click(screen.getByRole("tab", { name: "Peers" }));
    await screen.findByText("peer-a");
    fireEvent.click(screen.getByRole("button", { name: "Delete" }));
    fireEvent.click(within(screen.getByRole("alertdialog")).getByRole("button", { name: "Cancel" }));

    expect(api.deleteNodePeer).not.toHaveBeenCalled();
  });

  it("shows Server ACL editor on ACL tab only", async () => {
    render(<NodeDetail node={onlineServer} onBack={() => undefined} />);

    fireEvent.click(screen.getByRole("tab", { name: "Peers" }));
    expect(screen.queryByRole("button", { name: /save acl/i })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("tab", { name: "ACL" }));
    expect(await screen.findByRole("button", { name: /save acl/i })).toBeInTheDocument();
    expect(screen.getByLabelText(/allow targets/i)).toBeInTheDocument();
  });

  it("loads server connections through the node proxy", async () => {
    const node = { ...offlineServer, online: true };
    render(<NodeDetail node={node} onBack={() => undefined} />);

    fireEvent.click(screen.getByRole("tab", { name: "Connections" }));

    await waitFor(() => {
      expect(api.proxyNode).toHaveBeenCalledWith(node.node_key, "GET", "/api/v1/server/connections");
    });
    expect(api.proxyNode).not.toHaveBeenCalledWith(node.node_key, "GET", "/api/v1/server/config");
  });

  it("shows admin-aligned client connection columns and applies both rate patches", async () => {
    const node: CenterNode = {
      id: "node-c1",
      node_key: "client-1",
      name: "Client one",
      role: "client",
      online: true,
    };
    vi.mocked(api.proxyNode).mockImplementation(async (_key, method, path) => {
      if (method === "GET" && path === "/api/v1/peers") {
        return { peers: [{ peer_id: "peer-a" }] };
      }
      if (method === "GET" && path === "/api/v1/peers/peer-a/connections") {
        return {
          connections: [{
            connection_id: "conn-0",
            slot_index: 0,
            generation: 1,
            connected: true,
            retry_scheduled: false,
            state: "ready",
            encryption: "disabled",
            compression_mode: "disabled",
            compression_level: 1,
            path: "default",
            local: "127.0.0.1:1",
            peer: "10.0.0.2:4433",
            active_tunnels: 0,
            last_error: "",
            min_send_rate_kbps: 0,
            max_send_rate_kbps: 0,
            effective_server_tx_min_kbps: 0,
            effective_server_tx_max_kbps: 0,
          }],
        };
      }
      if (method === "GET" && path === "/api/v1/peers/peer-a/connections/conn-0") {
        return { connection_id: "conn-0", state: "ready" };
      }
      return {};
    });

    render(<NodeDetail node={node} onBack={() => undefined} />);
    fireEvent.click(screen.getByRole("tab", { name: "Connections" }));
    expect(await screen.findByText("conn-0")).toBeInTheDocument();

    for (const header of [
      "slot_index",
      "generation",
      "connected",
      "retry_scheduled",
      "encryption",
      "compression_mode",
      "compression_level",
      "client_min_send_rate_kbps",
      "server_min_send_rate_kbps",
    ]) {
      expect(screen.getByRole("columnheader", { name: header })).toBeInTheDocument();
    }

    fireEvent.change(screen.getByLabelText("client_min_send_rate_kbps"), { target: { value: "100" } });
    fireEvent.change(screen.getByLabelText("client_max_send_rate_kbps"), { target: { value: "200" } });
    fireEvent.click(screen.getByRole("button", { name: "Apply client rate" }));
    await waitFor(() => {
      expect(api.proxyNode).toHaveBeenCalledWith(
        "client-1",
        "PATCH",
        "/api/v1/peers/peer-a/connections/conn-0",
        { min_send_rate_kbps: 100, max_send_rate_kbps: 200 },
      );
    });

    fireEvent.change(screen.getByLabelText("server_min_send_rate_kbps"), { target: { value: "30" } });
    fireEvent.change(screen.getByLabelText("server_max_send_rate_kbps"), { target: { value: "40" } });
    fireEvent.click(screen.getByRole("button", { name: "Apply server rate" }));
    await waitFor(() => {
      expect(api.proxyNode).toHaveBeenCalledWith(
        "client-1",
        "PATCH",
        "/api/v1/peers/peer-a/connections/conn-0/server-send-rate",
        { min_send_rate_kbps: 30, max_send_rate_kbps: 40 },
      );
    });

    fireEvent.click(screen.getByRole("button", { name: "Detail" }));
    await waitFor(() => {
      expect(api.proxyNode).toHaveBeenCalledWith(
        "client-1",
        "GET",
        "/api/v1/peers/peer-a/connections/conn-0",
      );
    });
  });

  it("does not patch client rates when bounds are invalid", async () => {
    const node: CenterNode = {
      id: "node-c1",
      node_key: "client-1",
      name: "Client one",
      role: "client",
      online: true,
    };
    vi.mocked(api.proxyNode).mockImplementation(async (_key, method, path) => {
      if (method === "GET" && path === "/api/v1/peers") {
        return { peers: [{ peer_id: "peer-a" }] };
      }
      if (method === "GET" && path === "/api/v1/peers/peer-a/connections") {
        return {
          connections: [{
            connection_id: "conn-0",
            min_send_rate_kbps: 0,
            max_send_rate_kbps: 0,
          }],
        };
      }
      return {};
    });

    render(<NodeDetail node={node} onBack={() => undefined} />);
    fireEvent.click(screen.getByRole("tab", { name: "Connections" }));
    await screen.findByLabelText("client_min_send_rate_kbps");
    fireEvent.change(screen.getByLabelText("client_min_send_rate_kbps"), { target: { value: "abc" } });
    fireEvent.click(screen.getByRole("button", { name: "Apply client rate" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/safe non-negative integers/i);
    expect(vi.mocked(api.proxyNode).mock.calls.filter((call) => call[1] === "PATCH")).toHaveLength(0);
  });

  it("shows admin-aligned server connection columns and applies send rates", async () => {
    vi.mocked(api.proxyNode).mockResolvedValue({
      connections: [{
        connection_id: "sc1",
        client_name: "",
        remote_address: "192.168.1.9:443",
        state: "ready",
        encryption: "enabled",
        active_streams: 2,
        total_streams: 8,
        active_tunnels: 1,
        last_error: "",
        min_send_rate_kbps: 0,
        max_send_rate_kbps: 0,
      }],
    });
    render(<NodeDetail node={onlineServer} onBack={() => undefined} />);
    fireEvent.click(screen.getByRole("tab", { name: "Connections" }));

    expect(await screen.findByText("sc1")).toBeInTheDocument();
    expect(screen.getByText("peer-192.168.1.9:443")).toBeInTheDocument();
    expect(screen.getByText("8")).toBeInTheDocument();
    for (const header of [
      "remote_address",
      "encryption",
      "active_streams",
      "total_streams_opened",
      "active_tunnels",
      "min_send_rate_kbps",
      "max_send_rate_kbps",
    ]) {
      expect(screen.getByRole("columnheader", { name: header })).toBeInTheDocument();
    }
    expect(screen.queryByRole("button", { name: "Detail" })).not.toBeInTheDocument();

    fireEvent.change(screen.getByLabelText("min_send_rate_kbps"), { target: { value: "50" } });
    fireEvent.change(screen.getByLabelText("max_send_rate_kbps"), { target: { value: "80" } });
    fireEvent.click(screen.getByRole("button", { name: "Apply" }));
    await waitFor(() => {
      expect(api.proxyNode).toHaveBeenCalledWith(
        onlineServer.node_key,
        "PATCH",
        "/api/v1/server/connections/sc1",
        { min_send_rate_kbps: 50, max_send_rate_kbps: 80 },
      );
    });
  });

  it("disables connection writes while the node is offline", async () => {
    render(<NodeDetail node={offlineServer} onBack={() => undefined} />);
    fireEvent.click(screen.getByRole("tab", { name: "Connections" }));
    expect(screen.getByText("Writes are disabled while this node is offline.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Apply" })).not.toBeInTheDocument();
    expect(api.proxyNode).not.toHaveBeenCalled();
  });

  it("shows health status from the node record on Overview", () => {
    render(
      <NodeDetail
        node={{ ...offlineServer, health_status: "healthy" }}
        onBack={() => undefined}
      />,
    );

    expect(screen.getByText("Health")).toBeInTheDocument();
    expect(screen.getByText("healthy")).toBeInTheDocument();
  });

  it("does not show a Config tab for client, server, or unknown nodes", () => {
    const { unmount } = render(<NodeDetail node={onlineServer} onBack={() => undefined} />);
    expect(screen.queryByRole("tab", { name: "Config" })).not.toBeInTheDocument();
    unmount();

    const client: CenterNode = {
      id: "node-c1",
      node_key: "client-1",
      name: "Client one",
      role: "client",
      online: true,
    };
    const clientView = render(<NodeDetail node={client} onBack={() => undefined} />);
    expect(screen.queryByRole("tab", { name: "Config" })).not.toBeInTheDocument();
    clientView.unmount();

    const unknown: CenterNode = {
      id: "node-u1",
      node_key: "unknown-1",
      name: "Unknown node",
      role: "unknown",
      online: true,
    };
    render(<NodeDetail node={unknown} onBack={() => undefined} />);
    expect(screen.queryByRole("tab", { name: "Config" })).not.toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Overview" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Audit" })).toBeInTheDocument();
  });

  it("shows a Tunnels tab", () => {
    render(<NodeDetail node={onlineServer} onBack={() => undefined} />);
    expect(screen.getByRole("tab", { name: "Tunnels" })).toBeInTheDocument();
  });

  it("loads client tunnels through the node proxy with client columns", async () => {
    const node: CenterNode = {
      id: "node-c1",
      node_key: "client-1",
      name: "Client one",
      role: "client",
      online: true,
    };
    vi.mocked(api.proxyNode).mockResolvedValue({
      tunnels: [
        {
          tunnel_id: "t-1",
          peer_id: "p1",
          connection_id: "c1",
          target: "10.0.0.1:443",
          state: "open",
          role: "client",
          ingress: "socks",
          compress: "disabled",
          created_at: "2026-08-11T00:00:00Z",
          duration_ms: 1000,
          tcp_read_bytes: 10,
          tcp_write_bytes: 20,
          pending_bytes: 0,
          relay_backend: "linux",
          worker_index: 1,
          last_error: "",
        },
      ],
    });
    render(<NodeDetail node={node} onBack={() => undefined} />);
    fireEvent.click(screen.getByRole("tab", { name: "Tunnels" }));

    await waitFor(() => {
      expect(api.proxyNode).toHaveBeenCalledWith(node.node_key, "GET", "/api/v1/tunnels");
    });
    expect(api.proxyNode).not.toHaveBeenCalledWith(
      node.node_key,
      "GET",
      "/api/v1/server/tunnels",
    );
    expect(screen.getByText("t-1")).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "tunnel_id" })).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "tcp_read_bytes" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /abort/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /drain/i })).not.toBeInTheDocument();
  });

  it("loads server tunnels through the node proxy with server columns", async () => {
    vi.mocked(api.proxyNode).mockResolvedValue({
      tunnels: [
        {
          tunnel_id: "st-1",
          peer_id: "sp1",
          connection_id: "sc1",
          state: "open",
          target: "192.168.1.1:80",
          role: "server",
          duration_ms: 500,
          active: true,
        },
      ],
    });
    render(<NodeDetail node={onlineServer} onBack={() => undefined} />);
    fireEvent.click(screen.getByRole("tab", { name: "Tunnels" }));

    await waitFor(() => {
      expect(api.proxyNode).toHaveBeenCalledWith(
        onlineServer.node_key,
        "GET",
        "/api/v1/server/tunnels",
      );
    });
    expect(screen.getByText("st-1")).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "active" })).toBeInTheDocument();
    expect(screen.queryByRole("columnheader", { name: "tcp_read_bytes" })).not.toBeInTheDocument();
  });

  it("does not proxy tunnels when the node is offline", async () => {
    render(<NodeDetail node={offlineServer} onBack={() => undefined} />);
    fireEvent.click(screen.getByRole("tab", { name: "Tunnels" }));

    expect(
      await screen.findByText("Tunnels are unavailable while this node is offline."),
    ).toBeInTheDocument();
    expect(api.proxyNode).not.toHaveBeenCalled();
  });

  it("does not proxy tunnels for unknown role", async () => {
    const node: CenterNode = {
      id: "node-u1",
      node_key: "unknown-1",
      name: "Unknown node",
      role: "unknown",
      online: true,
    };
    render(<NodeDetail node={node} onBack={() => undefined} />);
    expect(screen.queryByRole("tab", { name: "Tunnels" })).not.toBeInTheDocument();
  });

  it("shows admin-aligned client peer columns", async () => {
    const node: CenterNode = {
      id: "node-c1",
      node_key: "client-1",
      name: "Client one",
      role: "client",
      online: true,
    };
    vi.mocked(api.proxyNode).mockImplementation(async (_key, _method, path) => {
      if (path === "/api/v1/peers") {
        return {
          peers: [{
            peer_id: "peer-a",
            state: "connected",
            enabled: true,
            quic_peer: "edge:443",
            socks_listen: "127.0.0.1:1080",
            http_listen: "127.0.0.1:8080",
            connection_count: 2,
            connected_connections: 1,
            active_streams: 3,
            total_streams: 9,
            reconnects: 4,
            last_error: "timeout",
          }],
        };
      }
      return {};
    });

    render(<NodeDetail node={node} onBack={() => undefined} />);
    fireEvent.click(screen.getByRole("tab", { name: "Peers" }));
    expect(await screen.findByText("peer-a")).toBeInTheDocument();

    for (const header of [
      "socks_listen",
      "http_listen",
      "connection_count",
      "connected_connections",
      "active_streams",
      "total_streams",
      "reconnects",
      "last_error",
    ]) {
      expect(screen.getByRole("columnheader", { name: header })).toBeInTheDocument();
    }
    expect(screen.getByText("timeout")).toBeInTheDocument();
    expect(screen.getByText("127.0.0.1:1080")).toBeInTheDocument();
  });

  it("fills connection fields when editing a peer and saves connection payload", async () => {
    const node: CenterNode = {
      id: "node-c1",
      node_key: "client-1",
      name: "Client one",
      role: "client",
      online: true,
    };
    vi.mocked(api.proxyNode).mockImplementation(async (_key, method, path, body) => {
      if (method === "GET" && path === "/api/v1/peers") {
        return {
          peers: [{
            peer_id: "peer-a",
            state: "connected",
            enabled: true,
            quic_peer: "edge:443",
            socks_listen: "127.0.0.1:1080",
            http_listen: "127.0.0.1:8080",
            proto_connections: 2,
            paths: [],
            port_forwards: [],
            connection_config: {
              desired: { encryption: "disabled", compression: { mode: "enabled", level: 5 } },
              applied: { encryption: "enabled", compression: { mode: "disabled", level: 1 } },
              restart_required: true,
            },
          }],
        };
      }
      if (method === "PUT" && path === "/api/v1/peers/peer-a") {
        return body ?? {};
      }
      return {};
    });

    render(<NodeDetail node={node} onBack={() => undefined} />);
    fireEvent.click(screen.getByRole("tab", { name: "Peers" }));
    await screen.findByText("peer-a");
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));

    expect(screen.getByLabelText("Desired encryption")).toHaveValue("disabled");
    expect(screen.getByLabelText("Desired compression mode")).toHaveValue("enabled");
    expect(screen.getByLabelText("Desired compression level")).toHaveValue(5);
    expect(screen.getByLabelText("Applied encryption")).toHaveTextContent("enabled");
    expect(screen.getByLabelText("Applied compression mode")).toHaveTextContent("disabled");
    expect(screen.getByLabelText("Applied compression level")).toHaveTextContent("1");
    expect(screen.getByLabelText("Restart required")).toHaveTextContent("true");

    fireEvent.change(screen.getByLabelText("Desired compression level"), { target: { value: "6" } });
    fireEvent.click(screen.getByRole("button", { name: "Create / Save" }));

    await waitFor(() => {
      expect(api.proxyNode).toHaveBeenCalledWith(
        "client-1",
        "PUT",
        "/api/v1/peers/peer-a",
        expect.objectContaining({
          peer_id: "peer-a",
          quic_connections: 2,
          connection: {
            encryption: "disabled",
            compression: { mode: "enabled", level: 6 },
          },
        }),
      );
    });
  });

  it("rejects invalid compression level before calling the peer API", async () => {
    const node: CenterNode = {
      id: "node-c1",
      node_key: "client-1",
      name: "Client one",
      role: "client",
      online: true,
    };
    vi.mocked(api.proxyNode).mockImplementation(async (_key, _method, path) => {
      if (path === "/api/v1/peers") {
        return { peers: [] };
      }
      return {};
    });

    render(<NodeDetail node={node} onBack={() => undefined} />);
    fireEvent.click(screen.getByRole("tab", { name: "Peers" }));
    fireEvent.click(await screen.findByRole("button", { name: "Create Peer" }));
    fireEvent.change(screen.getByLabelText("peer_id"), { target: { value: "peer-new" } });
    fireEvent.change(screen.getByLabelText("Desired compression level"), { target: { value: "99" } });
    fireEvent.click(screen.getByRole("button", { name: "Create / Save" }));

    expect(await screen.findByText(/Compression level must be an integer from 1 to 22/i)).toBeInTheDocument();
    expect(api.proxyNode).not.toHaveBeenCalledWith(
      "client-1",
      "POST",
      "/api/v1/peers",
      expect.anything(),
    );
  });
});

describe("center proxy API", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("surfaces HTTP status and error code", async () => {
    const actual = await vi.importActual<typeof import("../api")>("../api");
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(
      JSON.stringify({ code: "node_offline", message: "node_offline" }),
      { status: 503, headers: { "Content-Type": "application/json" } },
    )));

    await expect(actual.proxyNode("node/one", "GET", "/api/v1/server/connections")).rejects.toMatchObject({
      status: 503,
      code: "node_offline",
      data: { code: "node_offline" },
    });
  });
});
