import { StrictMode, useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import type { Session } from "@supabase/supabase-js";
import { App } from "./App";
import Login from "./login";
import { supabase } from "./supabase-client";

// 注意: index.htmlは/src/main.jsxを参照しており、このファイルは現状使われていない。
// ただしtscの型チェック対象には含まれるため、main.jsxと同じロジックに揃えておく。
function Root() {
  const [session, setSession] = useState<Session | null>(null);
  const [checkingSession, setCheckingSession] = useState(true);

  useEffect(() => {
    if (!supabase) {
      setCheckingSession(false);
      return;
    }
    supabase.auth.getSession().then(({ data }) => {
      setSession(data.session);
      setCheckingSession(false);
    });
    const { data: listener } = supabase.auth.onAuthStateChange((_event, nextSession) => {
      setSession(nextSession);
    });
    return () => listener.subscription.unsubscribe();
  }, []);

  if (checkingSession) return null;
  return session
    ? <App session={session} onLogout={() => supabase?.auth.signOut()} />
    : <Login />;
}

createRoot(document.getElementById("root") as HTMLElement).render(
  <StrictMode>
    <Root />
  </StrictMode>,
);
