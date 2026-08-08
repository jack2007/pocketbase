import { useState, type FormEvent } from "react";

interface LoginProps {
  onLogin: (identity: string, password: string) => Promise<unknown>;
}

export function Login({ onLogin }: LoginProps) {
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    setSubmitting(true);
    setError("");
    try {
      await onLogin(String(data.get("identity")), String(data.get("password")));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Login failed.");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <main className="login-page">
      <section className="login-card">
        <div className="brand-mark">R2</div>
        <p className="eyebrow">raypx2 center</p>
        <h1>Welcome back</h1>
        <p className="muted">Sign in with a PocketBase superuser account.</p>
        <form onSubmit={submit}>
          <label>
            Email
            <input name="identity" type="email" autoComplete="username" required autoFocus />
          </label>
          <label>
            Password
            <input name="password" type="password" autoComplete="current-password" required />
          </label>
          {error && <p className="form-error">{error}</p>}
          <button className="button full" disabled={submitting}>
            {submitting ? "Signing in…" : "Sign in"}
          </button>
        </form>
      </section>
    </main>
  );
}
