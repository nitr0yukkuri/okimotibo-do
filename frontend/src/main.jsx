import React, { StrictMode, useState } from 'react'
import { createRoot } from 'react-dom/client'
import { App } from './App'
import Login from './login.tsx'

function Root() {
  const [loggedIn, setLoggedIn] = useState(false)

  return loggedIn ? <App onLogout={() => setLoggedIn(false)} /> : <Login onLogin={() => setLoggedIn(true)} />
}

createRoot(document.getElementById('root')).render(
  <StrictMode>
    <Root />
  </StrictMode>,
)
