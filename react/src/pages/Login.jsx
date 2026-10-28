import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../AuthContext.jsx'

export default function Login() {
  const { login, signup } = useAuth()
  const navigate = useNavigate()
  const [mode, setMode] = useState('login') // 'login' | 'signup'
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

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
        <h1>Kinkit</h1>
        <p className="muted">Money management for every household, anywhere</p>
        {error && <div className="error-box">{error}</div>}

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
