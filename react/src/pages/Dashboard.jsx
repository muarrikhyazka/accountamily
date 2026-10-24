import { useEffect, useState } from 'react'
import { api, formatRupiah, MONTHS, MONTH_NAMES } from '../api'
import { useYearMonth } from '../YearContext.jsx'
import { categoryColor, seriesColor } from '../categoryColor'
import PieChart from '../PieChart.jsx'
import BarChart from '../BarChart.jsx'
import LineChart from '../LineChart.jsx'

const TABS = ['Across Years', 'Year', 'Month', 'Week']
const WEEKDAY_NAMES = ['Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday', 'Sunday']

export default function Dashboard() {
  const [tab, setTab] = useState('Month')

  return (
    <div>
      <div className="page-header">
        <div>
          <h1>Dashboard</h1>
          <p className="muted">Financial overview</p>
        </div>
      </div>

      <div className="tabs">
        {TABS.map(t => (
          <button key={t} className={`tab ${tab === t ? 'active' : ''}`} onClick={() => setTab(t)}>{t}</button>
        ))}
      </div>

      {tab === 'Across Years' && <YearsTab />}
      {tab === 'Year' && <YearTab />}
      {tab === 'Month' && <MonthTab />}
      {tab === 'Week' && <WeekTab />}
    </div>
  )
}

// ---------- Tab: Across Years ----------
function YearsTab() {
    const { valueMax } = useYearMonth()
    const [years, setYears] = useState([])
  const [byCategory, setByCategory] = useState({ years: [], rows: [] })
  const [monthly, setMonthly] = useState({ years: [], by_year: {} })
  const [dayOfMonthAvg, setDayOfMonthAvg] = useState([])
  const [datemonthAvg, setDatemonthAvg] = useState([])
  const [error, setError] = useState('')

  useEffect(() => {
    Promise.all([
      api.years(valueMax),
      api.yearsByCategory(valueMax),
      api.yearsMonthly(valueMax),
      api.yearsDayOfMonthAvg(valueMax),
      api.yearsDatemonthAvg(valueMax),
    ])
      .then(([y, c, m, d, dm]) => { setYears(y); setByCategory(c); setMonthly(m); setDayOfMonthAvg(d); setDatemonthAvg(dm) })
      .catch(e => setError(e.message))
  }, [valueMax])

  // average expense per month-name, across all years that have data for that month
  const monthNameAvg = MONTHS.map((_, i) => {
    const vals = monthly.years.map(y => monthly.by_year[y]?.[i] || 0).filter(v => v > 0)
    return vals.length ? vals.reduce((s, v) => s + v, 0) / vals.length : 0
  })

  return (
    <div>
      {error && <div className="error-box">{error}</div>}
      <div className="card">
        <h2>Year-over-Year Comparison</h2>
        {years.length === 0 && <div className="muted">No data yet.</div>}
        {years.length > 0 && (
          <BarChart
            labels={years.map(y => String(y.year))}
            datasets={[
              { label: 'Income', data: years.map(y => y.income), color: 'var(--success)' },
              { label: 'Expense', data: years.map(y => y.expense), color: 'var(--warning)' },
            ]}
          />
        )}
      </div>

      <div className="card">
        <table>
          <thead><tr><th>Year</th><th>Income</th><th>Expense</th><th>Net</th></tr></thead>
          <tbody>
            {years.map(y => (
              <tr key={y.year}>
                <td>{y.year}</td>
                <td>{formatRupiah(y.income)}</td>
                <td>{formatRupiah(y.expense)}</td>
                <td className={y.income - y.expense >= 0 ? 'text-positive' : 'text-negative'}>{formatRupiah(y.income - y.expense)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {monthly.years.length > 0 && (
        <div className="card">
          <h2>Monthly Spending - Across Years</h2>
          <LineChart
            labels={MONTHS}
            datasets={monthly.years.map((y, i) => ({
              label: String(y),
              data: (monthly.by_year[y] || []),
              color: seriesColor(i),
            }))}
          />
        </div>
      )}

      {monthNameAvg.some(v => v > 0) && (
        <div className="card">
          <h2>Average Expense per Month (Pattern Across All Years)</h2>
          <LineChart
            labels={MONTHS}
            datasets={[{ label: 'Average', data: monthNameAvg, color: 'var(--primary)' }]}
          />
        </div>
      )}

      {dayOfMonthAvg.length > 0 && (
        <div className="card">
          <h2>Average Expense per Date of Month (Pattern Across All Years)</h2>
          <LineChart
            labels={dayOfMonthAvg.map(d => String(d.day))}
            datasets={[{ label: 'Average', data: dayOfMonthAvg.map(d => d.avg), color: 'var(--primary)' }]}
          />
        </div>
      )}

      {datemonthAvg.length > 0 && (
        <div className="card">
          <h2>Average Expense per Date (Jan 1 - Dec 31, Pattern Across All Years)</h2>
          <div style={{ overflowX: 'auto' }}>
            <div style={{ minWidth: `${datemonthAvg.length * 24}px` }}>
              <LineChart
                labels={datemonthAvg.map(d => d.label)}
                datasets={[{ label: 'Average', data: datemonthAvg.map(d => d.avg), color: 'var(--primary)' }]}
              />
            </div>
          </div>
        </div>
      )}

      <div className="card">
        <h2>Spending by Category - Across Years</h2>
        {byCategory.rows.length === 0 && <div className="muted">No data yet.</div>}
        {byCategory.rows.length > 0 && (
          <div className="table-scroll">
            <table>
              <thead>
                <tr>
                  <th>Category</th>
                  {byCategory.years.map(y => <th key={y}>{y}</th>)}
                </tr>
              </thead>
              <tbody>
                {byCategory.rows.map(row => (
                  <tr key={row.category_name}>
                    <td><span className="cat-dot" style={{ background: categoryColor(row.category_name) }} />{row.category_name}</td>
                    {byCategory.years.map(y => <td key={y}>{formatRupiah(row[String(y)])}</td>)}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  )
}

// ---------- Tab: Year (12-month view of selected year) ----------
function YearTab() {
  const { year, valueMax } = useYearMonth()
  const [trend, setTrend] = useState([])
  const [catBreakdown, setCatBreakdown] = useState([])
  const [patterns, setPatterns] = useState({ by_date: [], by_week: [], by_weekday: [], by_week_in_month: [] })
  const [error, setError] = useState('')

  useEffect(() => {
    setError('')
    Promise.all([api.trend(year, 12, 12, valueMax), api.reportYear(year, valueMax), api.patternsYear(year, valueMax)])
      .then(([t, c, p]) => { setTrend(t); setCatBreakdown(c); setPatterns(p) })
      .catch(e => setError(e.message))
  }, [year, valueMax])

  const totalIncome = trend.reduce((s, t) => s + t.income, 0)
  const totalExpense = trend.reduce((s, t) => s + t.expense, 0)
  const maxCat = Math.max(1, ...catBreakdown.map(c => c.total))

  return (
    <div>
      {error && <div className="error-box">{error}</div>}
      <div className="cards-row">
        <div className="card stat stat-income">
          <div className="muted">Total Income {year}</div>
          <div className="stat-value">{formatRupiah(totalIncome)}</div>
        </div>
        <div className="card stat stat-expense">
          <div className="muted">Total Expense {year}</div>
          <div className="stat-value">{formatRupiah(totalExpense)}</div>
        </div>
        <div className={`card stat ${totalIncome - totalExpense >= 0 ? 'stat-positive' : 'stat-negative'}`}>
          <div className="muted">Net {year}</div>
          <div className="stat-value">{formatRupiah(totalIncome - totalExpense)}</div>
        </div>
      </div>

      <div className="card">
        <h2>12 Months - {year}</h2>
        {trend.length > 0 && (
          <BarChart
            labels={trend.map(t => MONTHS[t.month - 1])}
            datasets={[
              { label: 'Income', data: trend.map(t => t.income), color: 'var(--success)' },
              { label: 'Expense', data: trend.map(t => t.expense), color: 'var(--warning)' },
            ]}
          />
        )}
      </div>

      <div className="grid-2">
        <div className="card">
          <h2>Breakdown by Category - {year}</h2>
          {catBreakdown.length === 0 && <div className="muted">No data yet.</div>}
          {catBreakdown.map(c => {
            const pct = Math.round((c.total / maxCat) * 100)
            const color = categoryColor(c.category_name)
            return (
              <div className="bar-row" key={c.category_name}>
                <div className="bar-label"><span className="cat-dot" style={{ background: color }} />{c.category_name}</div>
                <div className="progress-bar">
                  <div className="progress-fill" style={{ width: `${pct}%`, background: color }} />
                </div>
                <div className="bar-value">{formatRupiah(c.total)}</div>
              </div>
            )
          })}
        </div>

        <div className="card">
          <h2>Category Breakdown - {year}</h2>
          <PieChart data={catBreakdown} />
        </div>
      </div>

      <div className="card">
        <h2>Pattern by Date of Month - {year}</h2>
        <p className="muted" style={{ marginTop: '-.5rem' }}>Average expense for each date (1-31), averaged across the whole year</p>
        {patterns.by_date.length > 0 && (
          <LineChart
            labels={patterns.by_date.map(d => String(d.day))}
            datasets={[{ label: 'Average', data: patterns.by_date.map(d => d.avg), color: 'var(--primary)' }]}
          />
        )}
      </div>

      <div className="card">
        <h2>Pattern by Week of Year - {year}</h2>
        <p className="muted" style={{ marginTop: '-.5rem' }}>Total expense per ISO week across {year}</p>
        {patterns.by_week.length > 0 && (
          <LineChart
            labels={patterns.by_week.map(w => `W${w.week}`)}
            datasets={[{ label: 'Total', data: patterns.by_week.map(w => w.avg), color: 'var(--primary)' }]}
          />
        )}
      </div>

      <div className="card">
        <h2>Pattern by Week in Month - {year}</h2>
        <p className="muted" style={{ marginTop: '-.5rem' }}>Average expense for week 1-5 within a month, averaged across the whole year</p>
        {patterns.by_week_in_month.length > 0 && (
          <LineChart
            labels={patterns.by_week_in_month.map(w => `Week ${w.week}`)}
            datasets={[{ label: 'Average', data: patterns.by_week_in_month.map(w => w.avg), color: 'var(--primary)' }]}
          />
        )}
      </div>

      <div className="card">
        <h2>Pattern by Day of Week - {year}</h2>
        <p className="muted" style={{ marginTop: '-.5rem' }}>Average expense per weekday, averaged across the whole year</p>
        {patterns.by_weekday.length > 0 && (
          <LineChart
            labels={WEEKDAY_NAMES}
            datasets={[{ label: 'Average', data: patterns.by_weekday.map(w => w.avg), color: 'var(--primary)' }]}
          />
        )}
      </div>
    </div>
  )
}

// ---------- Tab: Month (existing dashboard content) ----------
function MonthTab() {
  const { year, month, valueMax } = useYearMonth()
  const [report, setReport] = useState(null)
  const [trend, setTrend] = useState([])
  const [recent, setRecent] = useState([])
  const [compare, setCompare] = useState(null)
  const [error, setError] = useState('')

  useEffect(() => {
    setError('')
    Promise.all([
      api.report(year, month, valueMax),
      api.trend(year, month, 6, valueMax),
      api.spending(year, month, 1, 6, { valueMax }),
      api.reportMonthCompare(year, month, valueMax),
    ]).then(([r, t, s, cmp]) => {
      setReport(r)
      setTrend(t)
      setRecent(s.rows)
      setCompare(cmp)
    }).catch(e => setError(e.message))
  }, [year, month, valueMax])

  const topCategories = report
    ? [...report.expenses_list].filter(r => r.realization > 0).sort((a, b) => b.realization - a.realization).slice(0, 10)
    : []
  const byCategorySorted = report
    ? [...report.expenses_list].sort((a, b) => {
        const pctA = a.allocation > 0 ? a.realization / a.allocation : (a.realization > 0 ? Infinity : 0)
        const pctB = b.allocation > 0 ? b.realization / b.allocation : (b.realization > 0 ? Infinity : 0)
        return pctB - pctA
      })
    : []
  const netFlow = report ? report.total_income - report.budgeted_expenses : 0

  const pctChange = (curr, base) => base > 0 ? ((curr - base) / base) * 100 : (curr > 0 ? null : 0)
  const vsPrevMonth = compare ? pctChange(compare.current, compare.previous_month) : null
  const vsLastYear = compare ? pctChange(compare.current, compare.same_month_last_year) : null

  const renderPctBox = (label, pct) => {
    const display = pct === null ? 'N/A' : `${pct >= 0 ? '+' : ''}${pct.toFixed(1)}%`
    const cls = pct === null ? '' : (pct > 0 ? 'stat-negative' : (pct < 0 ? 'stat-positive' : ''))
    return (
      <div className={`card stat ${cls}`}>
        <div className="muted">{label}</div>
        <div className="stat-value">{display}</div>
      </div>
    )
  }

  return (
    <div>
      <p className="muted" style={{ marginTop: '-0.5rem' }}>{MONTHS[month - 1]} {year}</p>
      {error && <div className="error-box">{error}</div>}

      {report && (
        <>
          <div className="cards-row">
            <div className="card stat stat-income">
              <div className="muted">Total Income</div>
              <div className="stat-value">{formatRupiah(report.total_income)}</div>
            </div>
            <div className="card stat stat-expense">
              <div className="muted">Budgeted Expenses</div>
              <div className="stat-value">{formatRupiah(report.budgeted_expenses)}</div>
            </div>
            <div className="card stat stat-savings">
              <div className="muted">Savings</div>
              <div className="stat-value">{formatRupiah(report.savings)}</div>
            </div>
            <div className={`card stat ${netFlow >= 0 ? 'stat-positive' : 'stat-negative'}`}>
              <div className="muted">Left for Budgeting</div>
              <div className="stat-value">{formatRupiah(report.left_for_budgeting)}</div>
            </div>
          </div>

          <div className="cards-row">
            <div className="card stat stat-expense">
              <div className="muted">Total Spending {MONTHS[month - 1]}</div>
              <div className="stat-value">{formatRupiah(compare ? compare.current : 0)}</div>
            </div>
            {renderPctBox('vs Previous Month', vsPrevMonth)}
            {renderPctBox(`vs ${MONTHS[month - 1]} Last Year`, vsLastYear)}
          </div>

          <div className="grid-2">
            <div className="card">
              <h2>6-Month Trend</h2>
              {trend.length > 0 && (
                <BarChart
                  labels={trend.map(t => MONTHS[t.month - 1])}
                  datasets={[
                    { label: 'Income', data: trend.map(t => t.income), color: 'var(--success)' },
                    { label: 'Expense', data: trend.map(t => t.expense), color: 'var(--warning)' },
                  ]}
                />
              )}
            </div>

            <div className="card">
              <h2>Top 10 Expenses</h2>
              {topCategories.length === 0 && <div className="muted">No expenses this month yet.</div>}
              {topCategories.map((c, i) => {
                const pct = Math.round((c.realization / topCategories[0].realization) * 100)
                return (
                  <div className="bar-row" key={i}>
                    <div className="bar-label"><span className="cat-dot" style={{ background: categoryColor(c.category_name) }} />{c.category_name}</div>
                    <div className="progress-bar">
                      <div className="progress-fill" style={{ width: `${pct}%`, background: categoryColor(c.category_name) }} />
                    </div>
                    <div className="bar-value">{formatRupiah(c.realization)}</div>
                  </div>
                )
              })}
            </div>
          </div>

          <div className="card">
            <h2>Recent Transactions</h2>
            {recent.length === 0 && <div className="muted">No transactions yet.</div>}
            {recent.length > 0 && (
              <div className="table-scroll">
              <table>
                <thead><tr><th>Date</th><th>Description</th><th>Category</th><th>Amount</th></tr></thead>
                <tbody>
                  {recent.map(s => (
                    <tr key={s.id}>
                      <td>{s.tx_date}</td>
                      <td>{s.description}</td>
                      <td><span className="tag" style={{ background: categoryColor(s.category_name) + '22', color: categoryColor(s.category_name) }}>{s.category_name || 'Other'}</span></td>
                      <td>{formatRupiah(s.amount)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
              </div>
            )}
          </div>

          <div className="card table-scroll">
            <h2>By Category</h2>
            <table>
              <thead>
                <tr><th>Category</th><th>Allocation</th><th>Realization</th><th>Progress</th></tr>
              </thead>
              <tbody>
                {byCategorySorted.map((r, i) => {
                  const pctRaw = r.allocation > 0 ? Math.round((r.realization / r.allocation) * 100) : 0
                  const pctDisplay = `${pctRaw}%`
                  const pctWidth = Math.min(100, pctRaw)
                  const over = r.realization > r.allocation
                  const color = categoryColor(r.category_name)
                  return (
                    <tr key={i}>
                      <td><span className="cat-dot" style={{ background: color }} />{r.category_name}</td>
                      <td>{formatRupiah(r.allocation)}</td>
                      <td>{formatRupiah(r.realization)}</td>
                      <td>
                        <div className="progress-bar">
                          <div className={`progress-fill ${over ? 'over' : ''}`} style={{ width: `${pctWidth}%`, background: over ? undefined : color }} />
                        </div>
                        <span className="progress-label">{pctDisplay}</span>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        </>
      )}
    </div>
  )
}

// ---------- Tab: Week (weekly breakdown within selected month) ----------
function WeekTab() {
  const { year, month, valueMax } = useYearMonth()
  const [weeks, setWeeks] = useState([])
  const [error, setError] = useState('')

  useEffect(() => {
    setError('')
    api.weeklyReport(year, month, valueMax).then(setWeeks).catch(e => setError(e.message))
  }, [year, month, valueMax])

  const maxTotal = Math.max(1, ...weeks.map(w => w.total))

  // grouped bar: categories on x-axis, one series per week
  const allCats = [...new Set(weeks.flatMap(w => w.by_category.map(c => c.category_name)))]
    .sort((a, b) => {
      const totalA = weeks.reduce((s, w) => s + (w.by_category.find(c => c.category_name === a)?.total || 0), 0)
      const totalB = weeks.reduce((s, w) => s + (w.by_category.find(c => c.category_name === b)?.total || 0), 0)
      return totalB - totalA
    })

  return (
    <div>
      <p className="muted" style={{ marginTop: '-0.5rem' }}>{MONTHS[month - 1]} {year}</p>
      {error && <div className="error-box">{error}</div>}
      {weeks.length === 0 && <div className="card muted">No transactions this month yet.</div>}

      {weeks.length > 0 && (
        <div className="card">
          <h2>Total Spending per Week</h2>
          <BarChart
            labels={weeks.map(w => `Week ${w.week}`)}
            datasets={[{ label: 'Total', data: weeks.map(w => w.total), color: 'var(--warning)' }]}
          />
        </div>
      )}

      {allCats.length > 0 && (
        <div className="card">
          <h2>By Category - Across Weeks</h2>
          <BarChart
            labels={allCats}
            datasets={weeks.map((w, i) => ({
              label: `Week ${w.week}`,
              data: allCats.map(cat => w.by_category.find(c => c.category_name === cat)?.total || 0),
              color: seriesColor(i),
            }))}
          />
        </div>
      )}

      {weeks.map(w => (
        <div className="card" key={w.week}>
          <h2>Week {w.week} <span className="muted" style={{ fontWeight: 400, fontSize: '.85rem' }}>({formatRupiah(w.total)})</span></h2>
          <div className="progress-bar" style={{ marginBottom: '.8rem' }}>
            <div className="progress-fill" style={{ width: `${(w.total / maxTotal) * 100}%` }} />
          </div>
          {w.by_category.map(c => {
            const pct = Math.round((c.total / w.total) * 100)
            const color = categoryColor(c.category_name)
            return (
              <div className="bar-row" key={c.category_name}>
                <div className="bar-label"><span className="cat-dot" style={{ background: color }} />{c.category_name}</div>
                <div className="progress-bar">
                  <div className="progress-fill" style={{ width: `${pct}%`, background: color }} />
                </div>
                <div className="bar-value">{formatRupiah(c.total)}</div>
              </div>
            )
          })}
        </div>
      ))}
    </div>
  )
}
