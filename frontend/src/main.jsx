import React, { StrictMode, useEffect, useState } from 'react'
import { createRoot } from 'react-dom/client'
import { App } from './App'
import Login from './login.jsx'
import { supabase } from './supabase-client'

function Root() {
  // ログイン状態はローカルstateではなく、Supabaseの実セッションで判定する。
  const [session, setSession] = useState(null)
  const [checkingSession, setCheckingSession] = useState(true)

  useEffect(() => {
    if (!supabase) {
      setCheckingSession(false)
      return
    }
    // リロード時に既存セッションが残っていればログイン状態を維持する。
    supabase.auth.getSession().then(({ data }) => {
      setSession(data.session)
      setCheckingSession(false)
    })
    // Googleの同意画面から戻ってきた直後や、ログアウト操作をここで検知する。
    const { data: listener } = supabase.auth.onAuthStateChange((_event, nextSession) => {
      setSession(nextSession)
    })
    return () => listener.subscription.unsubscribe()
  }, [])

  if (checkingSession) return null
  return session ? <App session={session} /> : <Login />
}

createRoot(document.getElementById('root')).render(
  <StrictMode>
    <Root />
  </StrictMode>,
)
