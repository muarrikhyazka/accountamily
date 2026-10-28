import { useEffect, useState } from 'react'
import { api } from '../api'
import { formatRupiah } from '../api'
import { IconClose } from '../icons.jsx'

export default function Admin() {
  const [rows, setRows] = useState(null)
  const [error, setError] = useState('')
  const [resetting, setResetting] = useState(false)
  const [resetMsg, setResetMsg] = useState('')

  function load() { api.adminAccounts().then(setRows).catch(e => setError(e.message)) }
  useEffect(load, [])

  async function resetDemoData() {
    if (!confirm('Wipe and re-seed demo1/demo2 sample data with fresh randomized values? This cannot be undone.')) return
    setResetting(true)
    setResetMsg('')
    try {
      const d = await api.adminResetDemoData()
      setResetMsg(`Reset: ${d.accounts_reset.map(a => a.email).join(', ')}`)
      load()
    } catch (e) { setError(e.message) } finally { setResetting(false) }
  }

  async function toggleDisabled(row) {
    await api.adminUpdateAccount(row.id, { is_disabled: !row.is_disabled })
    load()
  }

  async function toggleAdmin(row) {
    await api.adminUpdateAccount(row.id, { is_admin: !row.is_admin })
    load()
  }

  async function remove(row) {
    if (!confirm(`Delete account "${row.email}" and ALL its data? This cannot be undone.`)) return
    try {
      await api.adminDeleteAccount(row.id)
      load()
    } catch (e) { setError(e.message) }
  }

  if (!rows) return <div className="loading">Loading...</div>

  return (
    <div>
      <h1>Super Admin</h1>
      <p className="muted">All accounts on this instance. Visible only to admins.</p>
      {error && <div className="error-box">{error}</div>}

      <div className="card" style={{ marginBottom: '1rem' }}>
        <button onClick={resetDemoData} disabled={resetting}>
          {resetting ? 'Resetting...' : 'Reset Demo Data'}
        </button>
        {resetMsg && <span className="muted" style={{ marginLeft: '0.75rem' }}>{resetMsg}</span>}
      </div>

      <div className="card">
        <table className="simple-table">
          <thead>
            <tr><th>Email</th><th>Name</th><th>Currency</th><th>Transactions</th><th>Integrations</th><th>Admin</th><th>Status</th><th>Created</th><th></th></tr>
          </thead>
          <tbody>
            {rows.map(r => (
              <tr key={r.id}>
                <td>{r.email}</td>
                <td>{r.display_name}</td>
                <td>{r.currency}</td>
                <td>{r.spending_count}</td>
                <td>{r.integration_count}</td>
                <td><input type="checkbox" checked={!!r.is_admin} onChange={() => toggleAdmin(r)} /></td>
                <td>
                  <button className={r.is_disabled ? 'secondary' : ''} onClick={() => toggleDisabled(r)}>
                    {r.is_disabled ? 'Disabled' : 'Active'}
                  </button>
                </td>
                <td>{r.created_at}</td>
                <td><button className="icon-btn danger round" onClick={() => remove(r)}><IconClose width={12} height={12} /></button></td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
