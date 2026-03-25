import { FormEvent, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import * as api from "../api/client";
import { useAuth } from "../auth";

export function SettingsPage() {
  const { state, logout } = useAuth();
  const token = state.access!;
  const [loading, setLoading] = useState(true);
  const [theme, setTheme] = useState<"light" | "dark" | "system">("system");
  const [voiceEnabled, setVoiceEnabled] = useState(false);
  const [autoPlay, setAutoPlay] = useState(false);
  const [speechRate, setSpeechRate] = useState(1);
  const [speechVol, setSpeechVol] = useState(1);
  const [msg, setMsg] = useState<string | null>(null);

  useEffect(() => {
    let on = true;
    api
      .getSettings(token)
      .then((s) => {
        if (!on) return;
        setTheme(s.theme);
        setVoiceEnabled(s.voiceEnabled);
        setAutoPlay(s.autoPlayVoice);
        setSpeechRate(s.speechRate);
        setSpeechVol(s.speechVolume);
      })
      .catch(() => setMsg("Failed to load settings"))
      .finally(() => on && setLoading(false));
    return () => {
      on = false;
    };
  }, [token]);

  async function save(e: FormEvent) {
    e.preventDefault();
    setMsg(null);
    try {
      await api.patchSettings(token, {
        theme,
        voiceEnabled,
        autoPlayVoice: autoPlay,
        speechRate,
        speechVolume: speechVol,
      });
      setMsg("Saved");
    } catch {
      setMsg("Save failed");
    }
  }

  if (loading) return <p style={{ padding: 24 }}>Loading…</p>;

  return (
    <div style={{ maxWidth: 520, margin: "2rem auto", padding: 24 }}>
      <Link to="/">← Chat</Link>
      <h1>Settings</h1>
      <form onSubmit={save}>
        <label>
          Theme
          <select
            value={theme}
            onChange={(e) => setTheme(e.target.value as typeof theme)}
            style={{ display: "block", marginBottom: 12 }}
          >
            <option value="light">Light</option>
            <option value="dark">Dark</option>
            <option value="system">System</option>
          </select>
        </label>
        <label>
          <input type="checkbox" checked={voiceEnabled} onChange={(e) => setVoiceEnabled(e.target.checked)} />{" "}
          Voice enabled
        </label>
        <br />
        <label>
          <input type="checkbox" checked={autoPlay} onChange={(e) => setAutoPlay(e.target.checked)} /> Auto-play
          voice
        </label>
        <label>
          <div>Speech rate {speechRate}</div>
          <input
            type="range"
            min={0.5}
            max={2}
            step={0.1}
            value={speechRate}
            onChange={(e) => setSpeechRate(Number(e.target.value))}
            style={{ width: "100%" }}
          />
        </label>
        <label>
          <div>Volume {speechVol}</div>
          <input
            type="range"
            min={0}
            max={1}
            step={0.1}
            value={speechVol}
            onChange={(e) => setSpeechVol(Number(e.target.value))}
            style={{ width: "100%" }}
          />
        </label>
        {msg && <p>{msg}</p>}
        <button type="submit" style={{ marginTop: 12 }}>
          Save
        </button>
      </form>
      <p>
        <button type="button" onClick={() => logout()}>
          Log out
        </button>
      </p>
    </div>
  );
}
