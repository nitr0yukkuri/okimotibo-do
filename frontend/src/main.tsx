import { StrictMode, useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import type { Session } from "@supabase/supabase-js";
import { App } from "./App";
import Login from "./login";
import { supabase } from "./supabase-client";
import { clearPairingSession, readPairingSession, type PairingSession } from "./pairing";

const localPwaResetKey = "okimochi-local-pwa-reset";

async function clearStaleLocalPwaState(): Promise<void> {
  if (!import.meta.env.DEV || !("serviceWorker" in navigator)) return;

  try {
    const registrations = await navigator.serviceWorker.getRegistrations();
    const cacheKeys = "caches" in window ? await window.caches.keys() : [];
    if (registrations.length === 0 && cacheKeys.length === 0) return;

    await Promise.all(registrations.map((registration) => registration.unregister()));
    await Promise.all(cacheKeys.map((key) => window.caches.delete(key)));

    if (window.sessionStorage.getItem(localPwaResetKey) !== "1") {
      window.sessionStorage.setItem(localPwaResetKey, "1");
      window.location.reload();
    }
  } catch (error) {
    console.warn("Failed to clear stale local PWA state.", error);
  }
}

void clearStaleLocalPwaState();

// 注意: index.htmlは/src/main.jsxを参照しており、このファイルは現状使われていない。
// ただしtscの型チェック対象には含まれるため、main.jsxと同じロジックに揃えておく。
function Root() {
  const [session, setSession] = useState<Session | null>(null);
  const [checkingSession, setCheckingSession] = useState(true);
  const [pairing] = useState<PairingSession | undefined>(() => readPairingSession());
  const hostname = window.location.hostname;
  const localNetworkHost = hostname === "localhost" || hostname === "127.0.0.1" ||
    /^(10|192\.168|172\.(1[6-9]|2\d|3[0-1]))\./.test(hostname);
  const localAnonymousConfig = localNetworkHost &&
    Boolean(import.meta.env.VITE_ROOM_ID && import.meta.env.VITE_USER_ID);
  const publicAnonymousMode = import.meta.env.VITE_ANONYMOUS_MODE === "true";
  const anonymousAvailable = publicAnonymousMode || (localAnonymousConfig && !supabase);
  const anonymousLoggedOut = new URLSearchParams(window.location.search).get("loggedOut") === "1";
  const anonymousMode = anonymousAvailable &&
    !anonymousLoggedOut &&
    (publicAnonymousMode || !supabase);

  useEffect(() => {
    if (pairing) {
      setCheckingSession(false);
      return;
    }
    if (!supabase) {
      setCheckingSession(false);
      return;
    }
    const sessionRequest = supabase.auth.getSession();
    const sessionTimeout = new Promise<never>((_, reject) => {
      window.setTimeout(() => reject(new Error("Supabase session check timed out")), 5000);
    });
    void Promise.race([sessionRequest, sessionTimeout])
      .then(({ data }) => {
        setSession(data.session);
      })
      .catch(() => {
        setSession(null);
      })
      .finally(() => {
        setCheckingSession(false);
      });
    const { data: listener } = supabase.auth.onAuthStateChange((_event, nextSession) => {
      setSession(nextSession);
    });
    return () => listener.subscription.unsubscribe();
  }, [pairing]);

  if (checkingSession) return null;
  if (pairing) {
    return (
      <App
        pairing={pairing}
        onLogout={() => {
          clearPairingSession();
          window.location.reload();
        }}
      />
    );
  }
  if (anonymousLoggedOut && anonymousAvailable) return <Login localOnly />;
  if (anonymousMode) {
    return (
      <App
        onLogout={() => {
          const url = new URL(window.location.href);
          url.search = "?loggedOut=1";
          url.hash = "";
          window.location.assign(url.toString());
        }}
      />
    );
  }
  return session
    ? <App session={session} onLogout={() => supabase?.auth.signOut()} />
    : <Login />;
}

createRoot(document.getElementById("root") as HTMLElement).render(
  <StrictMode>
    <Root />
  </StrictMode>,
);
