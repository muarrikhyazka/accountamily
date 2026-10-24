import { useState, useEffect, useRef } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../AuthContext.jsx'

const GOOGLE_CLIENT_ID = import.meta.env.VITE_GOOGLE_CLIENT_ID || ''

export default function Login() {
  const { login, signup, loginGoogle } = useAuth()
  const navigate = useNavigate()
  const [mode, setMode] = useState('login') // 'login' | 'signup'
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const googleBtnRef = useRef(null)

  // Render Google's own Sign-In button once GIS script + client id are ready.
  // ponytail: assumes GIS script already loaded (index.html <script> tag);
  // if it races on a slow connection the button just doesn't render - no login
  // is blocked, email/password still works.
  useEffect(() => {
    if (!GOOGLE_CLIENT_ID || !window.google || !googleBtnRef.current) return
    window.google.accounts.id.initialize({
      client_id: GOOGLE_CLIENT_ID,
      callback: async (resp) => {
        setError('')
        setBusy(true)
        try {
          await loginGoogle(resp.credential)
          navigate('/')
        } catch (err) {
          setError(err.message || 'Google login failed')
        } finally {
          setBusy(false)
        }
      },
    })
    window.google.accounts.id.renderButton(googleBtnRef.current, { theme: 'outline', size: 'large', width: 280 })
  }, [])

  async function handleSubmit(e) {
    e.preventDefault()
    setError('')
    setBusy(true)
    try {
      if (mode === 'signup') {
        await signup(username, password, displayName)
      } else {
        await login(username, password)
      }
      navigate('/')
    } catch (err) {
      setError(err.message || (mode === 'signup' ? 'Sign up failed' : 'Login failed'))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="login-page">
      <form className="card login-card" onSubmit={handleSubmit}>
        <h1>Accountamily</h1>
        <p className="muted">Money management for every household, anywhere</p>
        {error && <div className="error-box">{error}</div>}

        {GOOGLE_CLIENT_ID && (
          <>
            <div ref={googleBtnRef} style={{ display: 'flex', justifyContent: 'center', margin: '0.5rem 0' }} />
            <div className="muted" style={{ textAlign: 'center', fontSize: '0.8rem' }}>or</div>
          </>
        )}

        <label>
          Email
          <input type="email" value={username} onChange={e => setUsername(e.target.value)} required autoFocus />
        </label>
        {mode === 'signup' && (
          <label>
            Name
            <input value={displayName} onChange={e => setDisplayName(e.target.value)} placeholder="optional" />
          </label>
        )}
        <label>
          Password
          <input type="password" value={password} onChange={e => setPassword(e.target.value)} required minLength={mode === 'signup' ? 8 : undefined} />
        </label>
        <button type="submit" disabled={busy}>
          {busy ? 'Please wait...' : mode === 'signup' ? 'Create account' : 'Login'}
        </button>
        <button type="button" className="link-btn" onClick={() => { setMode(m => (m === 'login' ? 'signup' : 'login')); setError('') }}>
          {mode === 'login' ? "Don't have an account? Sign up" : 'Already have an account? Log in'}
        </button>
      </form>
    </div>
  )
}
