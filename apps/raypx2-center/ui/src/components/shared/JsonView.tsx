export function JsonView({ value, empty = "No data." }: { value: unknown; empty?: string }) {
  if (value === undefined) {
    return <p className="text-sm text-muted-foreground">{empty}</p>;
  }
  return (
    <pre className="max-h-90 overflow-auto rounded-md bg-zinc-950 p-4 font-mono text-xs leading-relaxed text-zinc-100">
      {JSON.stringify(value, null, 2)}
    </pre>
  );
}
