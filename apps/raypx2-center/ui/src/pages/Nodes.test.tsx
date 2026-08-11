import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
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
        health_status: "healthy",
      },
    ];

    render(<Nodes nodes={nodes} loading={false} onRefresh={() => undefined} />);

    expect(screen.getByText("Singapore edge")).toBeInTheDocument();
    expect(screen.getByText("edge-sg-1")).toBeInTheDocument();
    expect(screen.getByText("Online")).toBeInTheDocument();
    expect(screen.getByText("healthy")).toBeInTheDocument();
  });

  it("calls onDelete after confirmation", async () => {
    const onDelete = vi.fn().mockResolvedValue(undefined);
    const nodes: CenterNode[] = [
      {
        id: "node-1",
        node_key: "edge-sg-1",
        name: "Singapore edge",
        role: "server",
        online: false,
      },
    ];

    render(
      <Nodes
        nodes={nodes}
        loading={false}
        onRefresh={() => undefined}
        onDelete={onDelete}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Delete" }));
    fireEvent.click(within(screen.getByRole("alertdialog")).getByRole("button", { name: "Delete" }));
    await waitFor(() => {
      expect(onDelete).toHaveBeenCalledWith(nodes[0]);
    });
  });
});
