import { useEffect, useMemo, useState } from 'react'
import { api, formatRupiah, MONTHS } from '../api'
import { useYearMonth } from '../YearContext.jsx'
import { categoryColor } from '../categoryColor'
import MoneyInput from '../MoneyInput.jsx'

export default function Budgeting() {
  const { year } = useYearMonth()
  const [setup, setSetup] = useState(null)
  const [data, setData] = useState(null)
  const [error, setError] = useState('')

  useEffect(() => { api.setup().then(setSetup).catch(e => setError(e.message)) }, [])
  useEffect(() => {
    setError('')
    api.budgeting(year).then(setData).catch(e => setError(e.message))
  }, [year])

  const incomeMap = useMemo(() => {
    const m = {}
    data?.income.forEach(r => { m[`${r.source_id}-${r.month}`] = r.amount })
    return m
  }, [data])

  const expenseMap = useMemo(() => {
    const m = {}
    data?.expense.forEach(r => { m[`${r.category_id}-${r.month}`] = r.amount })
    return m
  }, [data])

  function historyStats(historyMap, id) {
    const years = Object.keys(historyMap?.[id] || {}).map(Number).sort((a, b) => a - b)
    if (years.length === 0) return { lastYear: 0, avg: 0 }
    const lastYearNum = years[years.length - 1]
    const lastYear = historyMap[id][lastYearNum]
    const avg = years.reduce((s, y) => s + historyMap[id][y], 0) / years.length
    return { lastYear, avg }
  }

  async function saveIncome(sourceId, month, value) {
    await api.setBudgetIncome({ year, month, source_id: sourceId, amount: Number(value) || 0 })
    setData(d => refreshLocal(d, 'income', 'source_id', sourceId, month, Number(value) || 0))
  }

  async function saveExpense(categoryId, month, value) {
    await api.setBudgetExpense({ year, month, category_id: categoryId, amount: Number(value) || 0 })
    setData(d => refreshLocal(d, 'expense', 'category_id', categoryId, month, Number(value) || 0))
  }

  function refreshLocal(d, key, idField, idVal, month, amount) {
    const list = d[key].filter(r => !(r[idField] === idVal && r.month === month))
    list.push({ [idField]: idVal, month, year, amount })
    return { ...d, [key]: list }
  }

  const categoriesByMain = useMemo(() => {
    const groups = { Needs: [], Wants: [], Savings: [] }
    setup?.categories.forEach(c => { (groups[c.main_category] ||= []).push(c) })
    return groups
  }, [setup])

  const monthTotals = useMemo(() => {
    const totals = Array(12).fill(0)
    data?.income.forEach(r => { totals[r.month - 1] += r.amount })
    return totals
  }, [data])

  if (!setup || !data) return <div className="loading">{error || 'Loading...'}</div>

  return (
    <div>
      <div className="page-header">
        <div>
          <h1>Budgeting</h1>
          <p className="muted">Year {year}</p>
        </div>
      </div>
      {error && <div className="error-box">{error}</div>}

      <div className="card table-scroll">
        <table className="budget-table">
          <thead>
            <tr>
              <th>Income</th>
              {MONTHS.map(m => <th key={m}>{m}</th>)}
              <th>Total</th>
              <th>Last Year</th>
              <th>Avg Past Years</th>
            </tr>
          </thead>
          <tbody>
            {setup.income_sources.map(src => {
              const hist = historyStats(data.income_history, src.id)
              const rowTotal = MONTHS.reduce((s, _, i) => s + (incomeMap[`${src.id}-${i + 1}`] || 0), 0)
              return (
                <tr key={src.id}>
                  <td>{src.name}</td>
                  {MONTHS.map((_, i) => {
                    const month = i + 1
                    return (
                      <td key={month}>
                        <MoneyInput
                          value={incomeMap[`${src.id}-${month}`] ?? ''}
                          onBlur={v => saveIncome(src.id, month, v)}
                        />
                      </td>
                    )
                  })}
                  <td>{formatRupiah(rowTotal)}</td>
                  <td className="muted">{formatRupiah(hist.lastYear)}</td>
                  <td className="muted">{formatRupiah(hist.avg)}</td>
                </tr>
              )
            })}
            <tr className="total-row">
              <td>Total Income</td>
              {monthTotals.map((t, i) => <td key={i}>{formatRupiah(t)}</td>)}
              <td>{formatRupiah(monthTotals.reduce((s, t) => s + t, 0))}</td>
              <td></td>
              <td></td>
            </tr>
          </tbody>
        </table>
      </div>

      {Object.entries(categoriesByMain).map(([main, cats]) => (
        <div className="card table-scroll" key={main}>
          <h2>{main}</h2>
          <table className="budget-table">
            <thead>
              <tr>
                <th>Category</th>
                {MONTHS.map(m => <th key={m}>{m}</th>)}
                <th>Total</th>
                <th>Last Year</th>
                <th>Avg Past Years</th>
              </tr>
            </thead>
            <tbody>
              {cats.map(cat => {
                const hist = historyStats(data.expense_history, cat.id)
                const rowTotal = MONTHS.reduce((s, _, i) => s + (expenseMap[`${cat.id}-${i + 1}`] || 0), 0)
                return (
                  <tr key={cat.id}>
                    <td><span className="cat-dot" style={{ background: categoryColor(cat.name) }} />{cat.name}</td>
                    {MONTHS.map((_, i) => {
                      const month = i + 1
                      return (
                        <td key={month}>
                          <MoneyInput
                            value={expenseMap[`${cat.id}-${month}`] ?? ''}
                            onBlur={v => saveExpense(cat.id, month, v)}
                          />
                        </td>
                      )
                    })}
                    <td>{formatRupiah(rowTotal)}</td>
                    <td className="muted">{formatRupiah(hist.lastYear)}</td>
                    <td className="muted">{formatRupiah(hist.avg)}</td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      ))}
    </div>
  )
}
