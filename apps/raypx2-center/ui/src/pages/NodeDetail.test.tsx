import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import * as api from "../api";
import { NodeDetail } from "./NodeDetail";
import type { CenterNode } from "./Nodes";

vi.mock("../api", async () => {
  const actual = await vi.importActual<typeof import("../api")>("../api");
  return {
    ...actual,
    getNodeConfig: vi.fn(),
    listAuditLogs: vi.fn().mockResolvedValue([]),
    putNodeConfig: vi.fn(),
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

const serverConfig = {
  node_key: onlineServer.node_key,
  role: "server",
  online: true,
  live: {
    allow_targets: ["10.0.0.0/8"],
    deny_targets: [],
    connection_config: {
      restart_required: true,
      pending_fields: ["connection.compression.level"],
    },
  },
  editor_draft: {
    allow_targets: ["10.0.0.0/8"],
    deny_targets: [],
    connection: { compression: { level: 3 } },
  },
  writable_paths: ["allow_targets", "deny_targets", "connection.compression.level"],
  recent_revisions: [],
};

describe("NodeDetail", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.getNodeConfig).mockResolvedValue(serverConfig);
  });
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("shows the offline write notice on Ops", () => {
    render(<NodeDetail node={offlineServer} onBack={() => undefined} />);

    fireEvent.click(screen.getByRole("tab", { name: "Ops" }));

    expect(screen.getByText("Writes are disabled while this node is offline.")).toBeInTheDocument();
  });

  it("does not show Server ACL editor on Ops tab", async () => {
    render(<NodeDetail node={onlineServer} onBack={() => undefined} />);

    fireEvent.click(screen.getByRole("tab", { name: "Ops" }));

    expect(screen.queryByRole("button", { name: /save acl/i })).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/allow targets/i)).not.toBeInTheDocument();
  });

  it("loads role-specific operations through the node proxy", async () => {
    const node = { ...offlineServer, online: true };
    render(<NodeDetail node={node} onBack={() => undefined} />);

    fireEvent.click(screen.getByRole("tab", { name: "Ops" }));

    await waitFor(() => {
      expect(api.proxyNode).toHaveBeenCalledWith(node.node_key, "GET", "/api/v1/health");
      expect(api.proxyNode).toHaveBeenCalledWith(node.node_key, "GET", "/api/v1/server/connections");
    });
    expect(api.proxyNode).not.toHaveBeenCalledWith(node.node_key, "GET", "/api/v1/server/config");
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

  it("keeps applied config and shows refresh warning when post-save GET fails", async () => {
    vi.mocked(api.putNodeConfig).mockResolvedValue({
      applied: { allow_targets: ["127.0.0.0/8"], deny_targets: [] },
      ignored_fields: ["listen"],
      revision_id: "rev1",
      admin_status: 200,
    });
    vi.mocked(api.getNodeConfig)
      .mockResolvedValueOnce(serverConfig)
      .mockRejectedValueOnce(new Error("503: Service unavailable"));
    render(<NodeDetail node={onlineServer} onBack={() => undefined} />);

    fireEvent.click(screen.getByRole("tab", { name: "Config" }));
    await screen.findByLabelText("Allow targets");
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    expect(await screen.findByText(/ignored fields/i)).toBeInTheDocument();
    expect(screen.getByText("listen")).toBeInTheDocument();
    expect(screen.getByRole("alert")).toHaveTextContent(/metadata could not be refreshed/i);
    expect(screen.getByRole("alert")).not.toHaveTextContent(/save failed/i);
    expect(screen.getByRole("alert")).not.toHaveTextContent(/operation failed/i);
    expect(screen.getByLabelText("Allow targets")).toHaveValue("127.0.0.0/8");
  });

  it("re-fetches config metadata and revisions after saving", async () => {
    const put = vi.mocked(api.putNodeConfig).mockResolvedValue({
      applied: { allow_targets: ["127.0.0.0/8"], deny_targets: [] },
      ignored_fields: ["listen"],
      revision_id: "rev1",
      admin_status: 200,
    });
    vi.mocked(api.getNodeConfig)
      .mockResolvedValueOnce(serverConfig)
      .mockResolvedValueOnce({
        ...serverConfig,
        live: {
          ...serverConfig.live,
          connection_config: {
            restart_required: false,
            pending_fields: [],
          },
        },
        editor_draft: { allow_targets: ["127.0.0.0/8"], deny_targets: [] },
        recent_revisions: [{
          id: "rev1",
          kind: "desired",
          source: "manual_edit",
          created: "2026-08-10T12:00:00Z",
        }],
      });
    render(<NodeDetail node={onlineServer} onBack={() => undefined} />);

    fireEvent.click(screen.getByRole("tab", { name: "Config" }));

    expect(await screen.findByText(/restart required/i)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(put).toHaveBeenCalledWith(
      onlineServer.node_key,
      serverConfig.editor_draft,
    ));
    await waitFor(() => expect(api.getNodeConfig).toHaveBeenCalledTimes(2));
    expect(await screen.findByText(/ignored fields/i)).toBeInTheDocument();
    expect(screen.getByText("listen")).toBeInTheDocument();
    expect(screen.getByText("Restart required: No")).toBeInTheDocument();
    expect(screen.getByText("manual_edit")).toBeInTheDocument();
    expect(screen.queryByText("View JSON")).not.toBeInTheDocument();
    expect(screen.queryByRole("columnheader", { name: "Summary" })).not.toBeInTheDocument();
  });

  it("blocks switching to Form when JSON is invalid", async () => {
    render(<NodeDetail node={onlineServer} onBack={() => undefined} />);
    fireEvent.click(screen.getByRole("tab", { name: "Config" }));
    await screen.findByLabelText("Allow targets");

    fireEvent.click(screen.getByRole("button", { name: "JSON" }));
    fireEvent.change(screen.getByLabelText("JSON configuration"), { target: { value: "{" } });
    fireEvent.click(screen.getByRole("button", { name: "Form" }));

    expect(screen.getByRole("alert")).toHaveTextContent(/valid JSON object/i);
    expect(screen.getByRole("button", { name: "JSON" })).toHaveAttribute("aria-pressed", "true");
  });

  it("disables config save when offline", async () => {
    vi.mocked(api.getNodeConfig).mockResolvedValue({
      ...serverConfig,
      online: false,
      live: null,
      editor_draft: {},
    });
    render(<NodeDetail node={offlineServer} onBack={() => undefined} />);
    fireEvent.click(screen.getByRole("tab", { name: "Config" }));

    expect(await screen.findByRole("button", { name: "Save" })).toBeDisabled();
    expect(screen.getByText(/configuration is read-only/i)).toBeInTheDocument();
  });

  it("constrains server compression level and blocks invalid saves", async () => {
    render(<NodeDetail node={onlineServer} onBack={() => undefined} />);
    fireEvent.click(screen.getByRole("tab", { name: "Config" }));

    const level = await screen.findByLabelText("Compression level");
    expect(level).toHaveAttribute("min", "1");
    expect(level).toHaveAttribute("max", "22");
    expect(level).toHaveAttribute("step", "1");

    fireEvent.change(level, { target: { value: "23" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    expect(screen.getByRole("alert")).toHaveTextContent(/integer between 1 and 22/i);
    expect(api.putNodeConfig).not.toHaveBeenCalled();
  });

  it("edits client port forwards and QUIC connections while keeping peer ID read-only", async () => {
    vi.mocked(api.getNodeConfig).mockResolvedValue({
      ...serverConfig,
      role: "client",
      editor_draft: {
        peers: [{
          peer_id: "peer-a",
          quic_peer: "edge.example:4433",
          quic_connections: 2,
          port_forwards: [{ listen: ":8080", target: "127.0.0.1:80" }],
          enabled: true,
        }],
      },
      writable_paths: [
        "peers[].peer_id",
        "peers[].port_forwards",
        "peers[].quic_connections",
      ],
    });
    render(<NodeDetail node={{ ...onlineServer, role: "client" }} onBack={() => undefined} />);
    fireEvent.click(screen.getByRole("tab", { name: "Config" }));

    expect(await screen.findByLabelText("Peer ID")).toHaveAttribute("readonly");
    const connections = screen.getByLabelText("QUIC connections");
    expect(connections).toHaveAttribute("type", "number");
    expect(connections).toHaveAttribute("step", "1");
    fireEvent.change(connections, { target: { value: "4" } });
    fireEvent.change(screen.getByLabelText("Port forward 1 listen"), {
      target: { value: ":9090" },
    });
    fireEvent.change(screen.getByLabelText("Port forward 1 target"), {
      target: { value: "127.0.0.1:90" },
    });

    expect(connections).toHaveValue(4);
    expect(screen.getByLabelText("Port forward 1 listen")).toHaveValue(":9090");
    expect(screen.getByLabelText("Port forward 1 target")).toHaveValue("127.0.0.1:90");
    expect(screen.getByRole("button", { name: "Add port forward" })).toBeEnabled();
  });

  it("adds a draft peer with editable peer ID and can remove it before save", async () => {
    vi.mocked(api.getNodeConfig).mockResolvedValue({
      ...serverConfig,
      role: "client",
      editor_draft: {
        peers: [{
          peer_id: "peer-a",
          quic_peer: "edge.example:4433",
          enabled: true,
        }],
      },
      writable_paths: ["peers[].peer_id"],
    });
    vi.mocked(api.putNodeConfig).mockResolvedValue({
      applied: { peers: [{ peer_id: "peer-a" }, { peer_id: "peer-b" }] },
      ignored_fields: [],
      revision_id: "rev-1",
      admin_status: 200,
    });
    render(<NodeDetail node={{ ...onlineServer, role: "client" }} onBack={() => undefined} />);
    fireEvent.click(screen.getByRole("tab", { name: "Config" }));

    expect(await screen.findByLabelText("Peer ID")).toHaveAttribute("readonly");
    fireEvent.click(screen.getByRole("button", { name: "Add peer" }));

    const peerIds = screen.getAllByLabelText("Peer ID");
    expect(peerIds).toHaveLength(2);
    expect(peerIds[0]).toHaveAttribute("readonly");
    expect(peerIds[1]).not.toHaveAttribute("readonly");

    fireEvent.change(peerIds[1], { target: { value: "peer-b" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(api.putNodeConfig).toHaveBeenCalledWith("server-sg-1", {
        peers: [
          expect.objectContaining({ peer_id: "peer-a" }),
          expect.objectContaining({ peer_id: "peer-b" }),
        ],
      });
    });
    const saved = vi.mocked(api.putNodeConfig).mock.calls[0][1] as {
      peers: Array<Record<string, unknown>>;
    };
    expect(saved.peers[1]._draft_new).toBeUndefined();
  });

  it("rejects saving a draft peer without peer ID", async () => {
    vi.mocked(api.getNodeConfig).mockResolvedValue({
      ...serverConfig,
      role: "client",
      editor_draft: { peers: [] },
      writable_paths: ["peers[].peer_id"],
    });
    render(<NodeDetail node={{ ...onlineServer, role: "client" }} onBack={() => undefined} />);
    fireEvent.click(screen.getByRole("tab", { name: "Config" }));
    fireEvent.click(await screen.findByRole("button", { name: "Add peer" }));
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/non-empty Peer ID/i);
    expect(api.putNodeConfig).not.toHaveBeenCalled();
  });

  it("removes only draft peers from the Config form", async () => {
    vi.mocked(api.getNodeConfig).mockResolvedValue({
      ...serverConfig,
      role: "client",
      editor_draft: {
        peers: [{ peer_id: "peer-a", enabled: true }],
      },
      writable_paths: ["peers[].peer_id"],
    });
    render(<NodeDetail node={{ ...onlineServer, role: "client" }} onBack={() => undefined} />);
    fireEvent.click(screen.getByRole("tab", { name: "Config" }));
    await screen.findByLabelText("Peer ID");
    expect(screen.queryByRole("button", { name: "Remove from draft" })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Add peer" }));
    fireEvent.click(screen.getByRole("button", { name: "Remove from draft" }));

    expect(screen.getAllByLabelText("Peer ID")).toHaveLength(1);
    expect(screen.getByLabelText("Peer ID")).toHaveValue("peer-a");
  });

  it("treats a 503 node_offline save response as offline and preserves the draft", async () => {
    vi.mocked(api.putNodeConfig).mockRejectedValue(Object.assign(new Error("node_offline"), {
      status: 503,
      code: "node_offline",
      data: { code: "node_offline" },
    }));
    render(<NodeDetail node={onlineServer} onBack={() => undefined} />);
    fireEvent.click(screen.getByRole("tab", { name: "Config" }));
    const allowTargets = await screen.findByLabelText("Allow targets");
    fireEvent.change(allowTargets, { target: { value: "127.0.0.0/8" } });

    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/offline/i);
    expect(screen.getByLabelText("Allow targets")).toHaveValue("127.0.0.0/8");
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
  });

  it("confirms before leaving Config when the draft is dirty", async () => {
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(false);
    render(<NodeDetail node={onlineServer} onBack={() => undefined} />);
    fireEvent.click(screen.getByRole("tab", { name: "Config" }));
    fireEvent.change(await screen.findByLabelText("Allow targets"), {
      target: { value: "127.0.0.0/8" },
    });

    fireEvent.click(screen.getByRole("tab", { name: "Ops" }));

    expect(confirm).toHaveBeenCalled();
    expect(screen.getByRole("tab", { name: "Config" })).toHaveAttribute("aria-selected", "true");
  });

  it("does not confirm when clicking the active Config tab", async () => {
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(false);
    render(<NodeDetail node={onlineServer} onBack={() => undefined} />);
    fireEvent.click(screen.getByRole("tab", { name: "Config" }));
    fireEvent.change(await screen.findByLabelText("Allow targets"), {
      target: { value: "127.0.0.0/8" },
    });

    fireEvent.click(screen.getByRole("tab", { name: "Config" }));

    expect(confirm).not.toHaveBeenCalled();
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
    fireEvent.click(screen.getByRole("tab", { name: "Tunnels" }));

    expect(
      await screen.findByText('Tunnel inventory is not available for role "unknown".'),
    ).toBeInTheDocument();
    expect(api.proxyNode).not.toHaveBeenCalled();
  });
});

describe("center config API", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("surfaces HTTP status and error code", async () => {
    const actual = await vi.importActual<typeof import("../api")>("../api");
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(
      JSON.stringify({ code: "node_offline", message: "node_offline" }),
      { status: 503, headers: { "Content-Type": "application/json" } },
    )));

    await expect(actual.getNodeConfig("node/one")).rejects.toMatchObject({
      status: 503,
      code: "node_offline",
      data: { code: "node_offline" },
    });
  });
});
