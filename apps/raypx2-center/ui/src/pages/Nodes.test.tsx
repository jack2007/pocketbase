import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Nodes, type CenterNode } from "./Nodes";

describe("Nodes", () => {
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

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

  it("uses Peer name placeholder and Confirm label in create dialog", () => {
    render(
      <Nodes
        nodes={[]}
        loading={false}
        onRefresh={() => undefined}
        onCreate={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Create node" }));
    expect(screen.getByPlaceholderText("Peer name")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Confirm" })).toBeInTheDocument();
  });

  it("copies enrollment secret to the clipboard", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(window, "isSecureContext", {
      configurable: true,
      value: true,
    });
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText },
    });

    const onCreate = vi.fn().mockResolvedValue({
      node: { id: "n1", node_key: "k1", name: "Peer A", role: "client" },
      enroll_secret: "secret-value-123",
    });

    render(
      <Nodes
        nodes={[]}
        loading={false}
        onRefresh={() => undefined}
        onCreate={onCreate}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Create node" }));
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Peer A" } });
    fireEvent.click(screen.getByRole("button", { name: "Confirm" }));

    await waitFor(() => {
      expect(screen.getByDisplayValue("secret-value-123")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: "Copy secret" }));
    await waitFor(() => {
      expect(writeText).toHaveBeenCalledWith("secret-value-123");
    });
  });

  it("copies enrollment secret via in-dialog selection fallback", async () => {
    Object.defineProperty(window, "isSecureContext", {
      configurable: true,
      value: false,
    });
    const execCommand = vi.fn().mockReturnValue(true);
    Object.defineProperty(document, "execCommand", {
      configurable: true,
      value: execCommand,
    });

    const onCreate = vi.fn().mockResolvedValue({
      node: { id: "n1", node_key: "k1", name: "Peer A", role: "client" },
      enroll_secret: "secret-fallback-456",
    });

    render(
      <Nodes
        nodes={[]}
        loading={false}
        onRefresh={() => undefined}
        onCreate={onCreate}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Create node" }));
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Peer A" } });
    fireEvent.click(screen.getByRole("button", { name: "Confirm" }));

    const secretInput = await screen.findByDisplayValue("secret-fallback-456");
    expect(secretInput).toHaveAccessibleName("Enrollment secret");

    fireEvent.click(screen.getByRole("button", { name: "Copy secret" }));
    await waitFor(() => {
      expect(execCommand).toHaveBeenCalledWith("copy");
    });
    expect(secretInput).toHaveValue("secret-fallback-456");
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
