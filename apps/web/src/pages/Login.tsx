import { FormEvent, useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useAuth } from "../auth";

export function LoginPage() {
  const { login, state } = useAuth();
  const nav = useNavigate();
  const [email, setEmail] = useState("admin@local");
  const [password, setPassword] = useState("changeme");
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    if (state.access) nav("/", { replace: true });
  }, [state.access, nav]);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setErr(null);
    try {
      await login(email, password);
      nav("/", { replace: true });
    } catch {
      setErr("Login failed");
    }
  }

  return (
    <div style={{ maxWidth: 360, margin: "4rem auto", padding: 24 }}>
      <h1>EVA</h1>
      <p>LAN voice assistant</p>
      <form onSubmit={onSubmit}>
        <label>
          <div>Email or username</div>
          <input
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            autoComplete="username"
            style={{ width: "100%", padding: 8 }}
          />
        </label>
        <label>
          <div>Password</div>
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="current-password"
            style={{ width: "100%", padding: 8, marginTop: 8 }}
          />
        </label>
        {err && <p style={{ color: "#b91c1c" }}>{err}</p>}
        <button type="submit" style={{ marginTop: 16, padding: "8px 16px" }}>
          Sign in
        </button>
      </form>
    </div>
  );
}
