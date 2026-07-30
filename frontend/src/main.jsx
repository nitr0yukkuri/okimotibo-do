import { StrictMode, useState } from 'react'
import { createRoot } from 'react-dom/client'
import { App } from './App'
import Login from './login.jsx' 

function Root() {
  const [loggedIn, setLoggedIn] = useState(false)

  return loggedIn ? <App /> : <Login onLogin={() => setLoggedIn(true)} />
}

createRoot(document.getElementById('root')).render(
  <StrictMode>
    <Root />
  </StrictMode>,
)
