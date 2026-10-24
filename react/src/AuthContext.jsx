import { createContext, useContext, useEffect, useState } from 'react'
import { api, setCurrency } from './api'

const AuthCtx = createContext(null)

export function AuthProvider({ children }) {
  const [user, setUser] = useState(undefined) // undefined=loading, null=anon, obj=logged in

  useEffect(() => {
    api.me().then(d => {
      if (d.authenticated) setCurrency(d.currency)
      setUser(d.authenticated ? d : null)
    }).catch(() => setUser(null))
  }, [])

  async function login(username, password) {
    const d = await api.login(username, password)
    const me = await api.me()
    setCurrency(me.currency)
    setUser({ authenticated: true, username, display_name: d.display_name, currency: me.currency, is_admin: me.is_admin })
  }

  async function signup(email, password, displayName) {
    const d = await api.signup(email, password, displayName)
    setCurrency('IDR')
    setUser({ authenticated: true, username: email, display_name: d.display_name, currency: 'IDR', is_admin: false })
  }

  async function loginGoogle(credential) {
    const d = await api.loginGoogle(credential)
    const me = await api.me()
    setCurrency(me.currency)
    setUser({ authenticated: true, username: me.username, display_name: d.display_name, currency: me.currency, is_admin: me.is_admin })
  }

  async function logout() {
    await api.logout().catch(() => {})
    setUser(null)
  }

  function setUserCurrency(code) {
    setCurrency(code)
    setUser(u => (u ? { ...u, currency: code } : u))
  }

  return <AuthCtx.Provider value={{ user, login, signup, loginGoogle, logout, setUserCurrency }}>{children}</AuthCtx.Provider>
}

export const useAuth = () => useContext(AuthCtx)
