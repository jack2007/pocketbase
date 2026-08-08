import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Nodes, type CenterNode } from "./Nodes";

describe("Nodes", () => {
  it("renders a row for each supplied node", () => {
    const nodes: CenterNode[] = [
      {
        id: "node-1",
        node_key: "edge-sg-1",
        name: "Singapore edge",
        role: "server",
        online: true,
        last_seen_at: "2026-08-08 12:00:00.000Z",
      },
    ];

    render(<Nodes nodes={nodes} loading={false} onRefresh={() => undefined} />);

    expect(screen.getByText("Singapore edge")).toBeInTheDocument();
    expect(screen.getByText("edge-sg-1")).toBeInTheDocument();
    expect(screen.getByText("Online")).toBeInTheDocument();
  });
});
