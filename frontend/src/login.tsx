import { useState, type FormEvent } from "react";
import "./login.css";
import { buildPairingUrl, pairingApiBase } from "./pairing";
import { supabase } from "./supabase-client";

interface LoginProps {
  localOnly?: boolean;
}

interface PairingClaimResponse {
  token: string;
  roomId: string;
  userId: string;
}

// 実際のGoogleログインはSupabase Authに委譲する。
// supabase未設定(.env.local未設定)の場合はボタンを無効化し、原因がわかるようにする。
const Login = ({ localOnly = false }: LoginProps) => {
  const [pairingCode, setPairingCode] = useState("");
  const [isPairing, setIsPairing] = useState(false);
  const [message, setMessage] = useState<string>();
  const pairingEnabled = localOnly || import.meta.env.VITE_ANONYMOUS_MODE === "true";

  const handleLogin = async () => {
    if (!supabase) return;

    try {
      const { error } = await supabase.auth.signInWithOAuth({
        provider: "google",
        options: { redirectTo: window.location.origin },
      });
      if (error) console.error("Google login could not be started", error);
    } catch (error) {
      console.error("Google login could not be started", error);
    }
  };

  const handlePairingClaim = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const code = pairingCode.replace(/[\s-]/g, "").toUpperCase();
    if (code.length !== 8 || isPairing) return;

    setIsPairing(true);
    setMessage(undefined);
    try {
      const response = await fetch(`${pairingApiBase()}/api/v1/pairing/claim`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ code }),
      });
      const data = await response.json().catch(() => ({})) as Partial<PairingClaimResponse> & { error?: string };
      if (!response.ok || !data.token || !data.roomId || !data.userId) {
        throw new Error(data.error || "合言葉が無効か、期限切れです。");
      }
      window.location.assign(buildPairingUrl({ token: data.token, roomId: data.roomId, userId: data.userId }));
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "合言葉で接続できませんでした。");
    } finally {
      setIsPairing(false);
    }
  };

  const handleLocalStart = () => {
    const url = new URL(window.location.href);
    url.searchParams.delete("loggedOut");
    url.hash = "";
    window.location.assign(url.toString());
  };

  return (
    <div className="login">
      <div className="login-box">
        <img className="login-logo" src="/okimochi_logo.png" alt="おきもちぼ〜ど" />
        {localOnly ? (
          <button className="google-btn" type="button" onClick={handleLocalStart}>
            ローカルで開始（認証不要）
          </button>
        ) : (
          <>
            <button className="google-btn" type="button" onClick={handleLogin} disabled={!supabase}>
              <img src="https://developers.google.com/identity/images/g-logo.png" alt="Google Logo" />
              Login with Google
            </button>
          </>
        )}
        {pairingEnabled && (
          <>
            <div className="login-divider" aria-hidden="true"><span>アカウントなしで接続</span></div>
            <form className="pairing-code-form" onSubmit={handlePairingClaim}>
              <label htmlFor="pairing-code">PCに表示された合言葉</label>
              <input
                id="pairing-code"
                className="email-input pairing-code-input"
                type="text"
                value={pairingCode}
                onChange={(event) => setPairingCode(event.target.value.toUpperCase())}
                placeholder="例: 7KQ2M8PD"
                inputMode="text"
                autoCapitalize="characters"
                autoComplete="one-time-code"
                maxLength={10}
                required
                disabled={isPairing}
              />
              <button className="email-btn" type="submit" disabled={isPairing || pairingCode.replace(/[\s-]/g, "").length !== 8}>
                {isPairing ? "接続中…" : "合言葉で接続"}
              </button>
            </form>
          </>
        )}
        {message && <p className="login-message" role="status">{message}</p>}
      </div>
    </div>
  );
};

export default Login;
