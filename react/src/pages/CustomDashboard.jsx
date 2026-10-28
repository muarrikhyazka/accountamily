import { useEffect, useState } from 'react'
import { api, MONTHS } from '../api'
import { useYearMonth } from '../YearContext.jsx'
import { categoryColor, seriesColor } from '../categoryColor'
import BarChart from '../BarChart.jsx'
import LineChart from '../LineChart.jsx'
import PieChart from '../PieChart.jsx'

const STORAGE_KEY = 'kinkit_custom_widgets'

// Data sources: each knows how to fetch + shape itself into {labels, datasets}
// for Bar/Line, or {rows:[{category_name,total}]} for Pie. New sources just
// add an entry here - no other wiring needed.
const SOURCES = {
  year_trend: {
    label: 'This Year - Income vs Expense (by month)',
    charts: ['bar', 'line'],
    fetch: ({ year, valueMax }) => api.trend(year, 12, 12, valueMax),
    shape: (d) => ({
      labels: d.map(t => MONTHS[t.month - 1]),
      datasets: [
        { label: 'Income', data: d.map(t => t.income), color: 'var(--success)' },
        { label: 'Expense', data: d.map(t => t.expense), color: 'var(--warning)' },
      ],
    }),
  },
  category_breakdown: {
    label: 'This Year - Spending by Category',
    charts: ['pie', 'bar'],
    fetch: ({ year, valueMax }) => api.reportYear(year, valueMax),
    shape: (d) => ({ rows: d }),
    shapeBar: (d) => ({
      labels: d.map(c => c.category_name),
      datasets: [{ label: 'Total', data: d.map(c => c.total), color: 'var(--primary)' }],
    }),
  },
  years_comparison: {
    label: 'Year-over-Year Income vs Expense',
    charts: ['bar'],
    fetch: ({ valueMax }) => api.years(valueMax),
    shape: (d) => ({
      labels: d.map(y => String(y.year)),
      datasets: [
        { label: 'Income', data: d.map(y => y.income), color: 'var(--success)' },
        { label: 'Expense', data: d.map(y => y.expense), color: 'var(--warning)' },
      ],
    }),
  },
  weekly_this_month: {
    label: 'This Month - Spending by Week',
    charts: ['bar'],
    fetch: ({ year, month, valueMax }) => api.weeklyReport(year, month, valueMax),
    shape: (d) => ({
      labels: d.map(w => `Week ${w.week}`),
      datasets: [{ label: 'Total', data: d.map(w => w.total), color: 'var(--warning)' }],
    }),
  },
}

function loadWidgets() {
  try {
    const saved = JSON.parse(localStorage.getItem(STORAGE_KEY))
    if (Array.isArray(saved) && saved.length) return saved
  } catch { /* corrupt/missing -> defaults */ }
  return [
    { id: 'w1', source: 'year_trend', chart: 'bar' },
    { id: 'w2', source: 'category_breakdown', chart: 'pie' },
  ]
}

export default function CustomDashboard() {
  const { year, month, valueMax } = useYearMonth()
  const [widgets, setWidgets] = useState(loadWidgets)
  const [dragIndex, setDragIndex] = useState(null)

  useEffect(() => { localStorage.setItem(STORAGE_KEY, JSON.stringify(widgets)) }, [widgets])

  function addWidget() {
    const id = 'w' + Date.now()
    setWidgets(w => [...w, { id, source: 'year_trend', chart: 'bar' }])
  }
  function removeWidget(id) { setWidgets(w => w.filter(x => x.id !== id)) }
  function updateWidget(id, patch) {
    setWidgets(w => w.map(x => x.id === id ? { ...x, ...patch } : x))
  }
  function onDrop(targetIndex) {
    if (dragIndex === null || dragIndex === targetIndex) return
    setWidgets(w => {
      const next = [...w]
      const [moved] = next.splice(dragIndex, 1)
      next.splice(targetIndex, 0, moved)
      return next
    })
    setDragIndex(null)
  }

  return (
    <div>
      <div className="page-header">
        <div>
          <h2>Custom Layout</h2>
          <p className="muted">Drag cards to reorder. Pick what data and chart each one shows.</p>
        </div>
        <button onClick={addWidget}>+ Add widget</button>
      </div>

      {widgets.map((w, i) => (
        <div
          key={w.id}
          className="card custom-widget"
          draggable
          onDragStart={() => setDragIndex(i)}
          onDragOver={(e) => e.preventDefault()}
          onDrop={() => onDrop(i)}
        >
          <div className="custom-widget-controls">
            <span className="drag-handle" title="Drag to reorder">⠿</span>
            <select value={w.source} onChange={e => {
              const src = e.target.value
              updateWidget(w.id, { source: src, chart: SOURCES[src].charts[0] })
            }}>
              {Object.entries(SOURCES).map(([key, s]) => <option key={key} value={key}>{s.label}</option>)}
            </select>
            <select value={w.chart} onChange={e => updateWidget(w.id, { chart: e.target.value })}>
              {SOURCES[w.source].charts.map(c => <option key={c} value={c}>{c[0].toUpperCase() + c.slice(1)} chart</option>)}
            </select>
            <button className="icon-btn danger" onClick={() => removeWidget(w.id)} title="Remove">×</button>
          </div>
          <WidgetBody widget={w} ctx={{ year, month, valueMax }} />
        </div>
      ))}
    </div>
  )
}

function WidgetBody({ widget, ctx }) {
  const src = SOURCES[widget.source]
  const [data, setData] = useState(null)
  const [error, setError] = useState('')

  useEffect(() => {
    setData(null); setError('')
    src.fetch(ctx).then(setData).catch(e => setError(e.message))
  }, [widget.source, ctx.year, ctx.month, ctx.valueMax])

  if (error) return <div className="error-box">{error}</div>
  if (!data) return <div className="muted">Loading...</div>

  if (widget.chart === 'pie') {
    const { rows } = src.shape(data)
    return <PieChart data={rows} />
  }
  if (widget.chart === 'bar') {
    const shaped = src.shapeBar ? src.shapeBar(data) : src.shape(data)
    return <BarChart labels={shaped.labels} datasets={shaped.datasets} />
  }
  if (widget.chart === 'line') {
    const shaped = src.shape(data)
    return <LineChart labels={shaped.labels} datasets={shaped.datasets} />
  }
  return null
}
