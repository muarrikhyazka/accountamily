import { useEffect, useState } from 'react'
import { api } from '../api'
import { IconDashboard, IconTelegram, IconDiscord } from '../icons.jsx'

export default function AdminStats() {
  const [stats, setStats] = useState(null)
  const [error, setError] = useState('')

  useEffect(() => {
    api.adminStats().then(setStats).catch(e => setError(e.message))
  }, [])

  if (error) return <div className="error-box">{error}</div>
  if (!stats) return <div className="loading">Loading...</div>

  const platformIcon = { telegram: IconTelegram, discord: IconDiscord }

  return (
    <div>
      <h1>Platform Stats</h1>

      <div className="cards-row">
        <div className="card stat">
          <div className="muted">Total Accounts</div>
          <div className="stat-value">{stats.accounts.total}</div>
        </div>
        <div className="card stat stat-income">
          <div className="muted">Active</div>
          <div className="stat-value">{stats.accounts.active}</div>
        </div>
        <div className="card stat stat-expense">
          <div className="muted">Disabled</div>
          <div className="stat-value">{stats.accounts.disabled}</div>
        </div>
        <div className="card stat stat-savings">
          <div className="muted">Total Spending Entries</div>
          <div className="stat-value">{stats.spending.count}</div>
        </div>
      </div>

      <div className="card">
        <h2>Connected Bots</h2>
        {stats.integrations_by_platform.length === 0 ? (
          <p className="muted">No accounts have connected a bot yet.</p>
        ) : (
          <table className="simple-table">
            <thead><tr><th>Platform</th><th>Connected accounts</th></tr></thead>
            <tbody>
              {stats.integrations_by_platform.map(row => {
                const Icon = platformIcon[row.platform] || IconDashboard
                return (
                  <tr key={row.platform}>
                    <td><Icon width={16} height={16} /> {row.platform}</td>
                    <td>{row.n}</td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        )}
      </div>

      <div className="card">
        <h2>Signups (last 30 days with activity)</h2>
        {stats.signups_by_day.length === 0 ? (
          <p className="muted">No signups yet.</p>
        ) : (
          <table className="simple-table">
            <thead><tr><th>Day</th><th>New accounts</th></tr></thead>
            <tbody>
              {stats.signups_by_day.map(row => (
                <tr key={row.day}><td>{row.day}</td><td>{row.n}</td></tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}
