import { useEffect, useState } from 'react'
import { api, formatRupiah, MONTHS } from '../api'
import BarChart from '../BarChart.jsx'

const REFRESH_MS = 60_000 // auto-refresh for an always-on screen (TV/car head unit)

/**
 * Full-screen, no-sidebar, large-text dashboard meant for a TV or car head
 * unit: glanceable at a distance, no interaction needed, auto-refreshes.
 * Standalone route (/tv) outside the normal <Layout> - deliberately skips
 * auth chrome/nav since this is meant to just sit on a screen.
 */
export default function TVView() {
  const [report, setReport] = useState(null)
  const [trend, setTrend] = useState([])
  const [error, setError] = useState('')
  const now = new Date()
  const year = now.getFullYear()
  const month = now.getMonth() + 1

  useEffect(() => {
    function load() {
      Promise.all([api.report(year, month), api.trend(year, month, 6)])
        .then(([r, t]) => { setReport(r); setTrend(t) })
        .catch(e => setError(e.message))
    }
    load()
    const id = setInterval(load, REFRESH_MS)
    return () => clearInterval(id)
  }, [year, month])

  if (error) return <div className="tv-view tv-error">{error}</div>
  if (!report) return <div className="tv-view tv-loading">Loading...</div>

  const netFlow = report.total_income - report.budgeted_expenses
  const topExpenses = [...report.expenses_list]
    .filter(r => r.realization > 0)
    .sort((a, b) => b.realization - a.realization)
    .slice(0, 5)

  return (
    <div className="tv-view">
      <div className="tv-header">
        <div className="tv-brand">Kinkit</div>
        <div className="tv-date">{MONTHS[month - 1]} {year}</div>
      </div>

      <div className="tv-cards">
        <div className="tv-card">
          <div className="tv-label">Income</div>
          <div className="tv-value tv-positive">{formatRupiah(report.total_income)}</div>
        </div>
        <div className="tv-card">
          <div className="tv-label">Spending</div>
          <div className="tv-value tv-warning">{formatRupiah(report.budgeted_expenses)}</div>
        </div>
        <div className="tv-card">
          <div className="tv-label">Left</div>
          <div className={`tv-value ${netFlow >= 0 ? 'tv-positive' : 'tv-negative'}`}>{formatRupiah(report.left_for_budgeting)}</div>
        </div>
      </div>

      <div className="tv-row">
        <div className="tv-panel">
          <div className="tv-panel-title">6-Month Trend</div>
          {trend.length > 0 && <BarChart labels={trend.map(t => MONTHS[t.month - 1])} datasets={[
            { label: 'Income', data: trend.map(t => t.income), color: 'var(--success)' },
            { label: 'Expense', data: trend.map(t => t.expense), color: 'var(--warning)' },
          ]} height={320} />}
        </div>
        <div className="tv-panel">
          <div className="tv-panel-title">Top Expenses</div>
          {topExpenses.map((c, i) => (
            <div className="tv-expense-row" key={i}>
              <span>{c.category_name}</span>
              <span>{formatRupiah(c.realization)}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}
