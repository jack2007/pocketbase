import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import * as api from "../api";
import { NodeDetail } from "./NodeDetail";
import type { CenterNode } from "./Nodes";

vi.mock("../api", async () => {
  const actual = await vi.importActual<typeof import("../api")>("../api");
  return {
    ...actual,
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

describe("NodeDetail", () => {
  beforeEach(() => vi.clearAllMocks());
  afterEach(cleanup);

  it("disables write controls when the node is offline", async () => {
    render(<NodeDetail node={offlineServer} onBack={() => undefined} />);

    fireEvent.click(screen.getByRole("tab", { name: "Ops" }));

    expect(await screen.findByRole("button", { name: "Save ACL" })).toBeDisabled();
    expect(screen.getByText("Writes are disabled while this node is offline.")).toBeInTheDocument();
  });

  it("loads role-specific operations through the node proxy", async () => {
    const node = { ...offlineServer, online: true };
    render(<NodeDetail node={node} onBack={() => undefined} />);

    fireEvent.click(screen.getByRole("tab", { name: "Ops" }));

    await screen.findByText("Server ACL");
    expect(api.proxyNode).toHaveBeenCalledWith(node.node_key, "GET", "/api/v1/health");
    expect(api.proxyNode).toHaveBeenCalledWith(node.node_key, "GET", "/api/v1/server/config");
    expect(api.proxyNode).toHaveBeenCalledWith(node.node_key, "GET", "/api/v1/server/connections");
  });
});
