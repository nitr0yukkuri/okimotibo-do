import React, { StrictMode, useEffect, useState } from 'react'
import { createRoot } from 'react-dom/client'
import { App } from './App'
import Login from './login.tsx'
import { supabase } from './supabase-client'

function Root() {
  // ログイン状態はローカルstateではなく、Supabaseの実セッションで判定する。
  const [session, setSession] = useState(null)
  const [checkingSession, setCheckingSession] = useState(true)
  const anonymousMode = !supabase && import.meta.env.DEV &&
    Boolean(import.meta.env.VITE_ROOM_ID && import.meta.env.VITE_USER_ID)

  useEffect(() => {
    if (!supabase) {
      setCheckingSession(false)
      return
    }
    // リロード時に既存セッションが残っていればログイン状態を維持する。
    supabase.auth.getSession()
      .then(({ data }) => {
        setSession(data.session)
      })
      .catch(() => {
        setSession(null)
      })
      .finally(() => {
        setCheckingSession(false)
      })
    // Googleの同意画面から戻ってきた直後や、ログアウト操作をここで検知する。
    const { data: listener } = supabase.auth.onAuthStateChange((_event, nextSession) => {
      setSession(nextSession)
    })
    return () => listener.subscription.unsubscribe()
  }, [])

  if (checkingSession) return null
  if (anonymousMode) return <App onLogout={() => {}} />
  return session
    ? <App session={session} onLogout={() => supabase?.auth.signOut()} />
    : <Login />
}

createRoot(document.getElementById('root')).render(
  <StrictMode>
    <Root />
  </StrictMode>,
)
