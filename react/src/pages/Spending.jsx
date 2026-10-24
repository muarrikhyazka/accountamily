import { useEffect, useState } from 'react'
import { api, formatRupiah, MONTHS } from '../api'
import { useYearMonth } from '../YearContext.jsx'
import { useOffline } from '../OfflineContext.jsx'
import { categoryColor } from '../categoryColor'
import MoneyInput from '../MoneyInput.jsx'
import { IconClose, IconEdit, IconSortUp, IconSortDown } from '../icons.jsx'
import CategorySelect from '../CategorySelect.jsx'

const now = new Date()
const todayISO = now.toISOString().slice(0, 10)
const PAGE_SIZES = [10, 20, 50, 100]
const emptyRow = () => ({ tx_date: todayISO, description: '', category: '', amount: '' })

export default function Spending() {
  const { year, month, valueMax } = useYearMonth()
  const { addSpendingResilient } = useOffline()
  const [page, setPage] = useState(1)
  const [perPage, setPerPage] = useState(20)
  const [rows, setRows] = useState([])
  const [total, setTotal] = useState(0)
  const [categories, setCategories] = useState([])
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [formRows, setFormRows] = useState([emptyRow()])
  const [saving, setSaving] = useState(false)
  const [search, setSearch] = useState('')
  const [categoryFilter, setCategoryFilter] = useState('')
  const [sort, setSort] = useState('tx_date')
  const [order, setOrder] = useState('desc')
  const [editingId, setEditingId] = useState(null)
  const [editRow, setEditRow] = useState(null)

  useEffect(() => { api.setup().then(s => setCategories(s.categories)).catch(e => setError(e.message)) }, [])

  function load() {
    setError('')
    api.spending(year, month, page, perPage, { search, category: categoryFilter, sort, order, valueMax })
      .then(d => { setRows(d.rows); setTotal(d.total) })
      .catch(e => setError(e.message))
  }
  useEffect(load, [year, month, page, perPage, search, categoryFilter, sort, order, valueMax])
  useEffect(() => { setPage(1) }, [year, month, perPage, search, categoryFilter, sort, order, valueMax])

  const totalPages = Math.max(1, Math.ceil(total / perPage))

  function toggleSort(col) {
    if (sort === col) setOrder(o => (o === 'asc' ? 'desc' : 'asc'))
    else { setSort(col); setOrder('asc') }
  }
  function sortIndicator(col) {
    if (sort !== col) return null
    return order === 'asc' ? <IconSortUp width={11} height={11} /> : <IconSortDown width={11} height={11} />
  }

  async function handleAdd(e) {
    e.preventDefault()
    const valid = formRows.filter(r => r.description && r.category && r.amount)
    if (valid.length === 0) { setError('Fill in at least one complete row.'); return }
    setSaving(true)
    setError('')
    let queuedCount = 0
    try {
      for (const r of valid) {
        const result = await addSpendingResilient({ ...r, amount: Number(r.amount) })
        if (result.queued) queuedCount++
      }
      setFormRows([emptyRow()])
      if (queuedCount > 0) {
        setNotice(`${queuedCount} of ${valid.length} transaction(s) saved locally (server offline), will sync automatically.`)
      } else {
        setNotice('')
        setPage(1)
        load()
      }
    } catch (err) {
      setError(err.message)
    } finally {
      setSaving(false)
    }
  }

  function updateFormRow(i, field, value) {
    setFormRows(rs => rs.map((r, idx) => (idx === i ? { ...r, [field]: value } : r)))
  }
  function addFormRow() { setFormRows(rs => [...rs, emptyRow()]) }
  function removeFormRow(i) { setFormRows(rs => rs.filter((_, idx) => idx !== i)) }

  async function handleDelete(id) {
    if (!confirm('Delete this transaction? This cannot be undone.')) return
    try {
      await api.deleteSpending(id)
      load()
    } catch (err) { setError(err.message) }
  }

  async function handleCheck(row) {
    try {
      await api.updateSpending(row.id, { checked: row.checked ? 0 : 1 })
      load()
    } catch (err) { setError(err.message) }
  }

  function startEdit(row) {
    setEditingId(row.id)
    setEditRow({ tx_date: row.tx_date, description: row.description, category: row.category_name || '', amount: row.amount })
  }
  function cancelEdit() {
    setEditingId(null)
    setEditRow(null)
  }
  async function saveEdit(id) {
    try {
      await api.updateSpending(id, { ...editRow, amount: Number(editRow.amount) })
      setEditingId(null)
      setEditRow(null)
      load()
    } catch (err) { setError(err.message) }
  }

  return (
    <div>
      <div className="page-header">
        <div>
          <h1>Spending</h1>
          <p className="muted">{total} transactions - {MONTHS[month - 1]} {year}</p>
        </div>
      </div>

      {error && <div className="error-box">{error}</div>}
      {notice && <div className="notice-box">{notice}</div>}

      <form className="card" onSubmit={handleAdd}>
        <div className="quick-add-rows">
          {formRows.map((r, i) => (
            <div className="quick-add-row" key={i}>
              <input type="date" value={r.tx_date} onChange={e => updateFormRow(i, 'tx_date', e.target.value)} required />
              <input placeholder="Description" value={r.description} onChange={e => updateFormRow(i, 'description', e.target.value)} required />
              <CategorySelect value={r.category} onChange={v => updateFormRow(i, 'category', v)} categories={categories} required />
              <MoneyInput placeholder="Amount" value={r.amount} onChange={v => updateFormRow(i, 'amount', v)} required />
              <button type="button" className="icon-btn danger round" onClick={() => removeFormRow(i)} disabled={formRows.length === 1}>
                <IconClose width={12} height={12} />
              </button>
            </div>
          ))}
        </div>
        <div className="quick-add-actions">
          <button type="button" className="secondary" onClick={addFormRow}>+ Add Row</button>
          <button type="submit" disabled={saving}>{saving ? 'Saving...' : 'Save All'}</button>
        </div>
      </form>

      <div className="card inline-form" style={{ marginBottom: '1rem' }}>
        <input placeholder="Search description..." value={search} onChange={e => setSearch(e.target.value)} style={{ minWidth: 200 }} />
        <CategorySelect value={categoryFilter} onChange={setCategoryFilter} categories={categories} placeholder="All categories" />
      </div>

      <div className="card table-scroll">
        <table>
          <thead>
            <tr>
              <th className="sortable" onClick={() => toggleSort('tx_date')}>Date {sortIndicator('tx_date')}</th>
              <th className="sortable" onClick={() => toggleSort('description')}>Description {sortIndicator('description')}</th>
              <th className="sortable" onClick={() => toggleSort('category_name')}>Category {sortIndicator('category_name')}</th>
              <th className="sortable" onClick={() => toggleSort('amount')}>Amount {sortIndicator('amount')}</th>
              <th>Checked</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {rows.map(r => editingId === r.id ? (
              <tr key={r.id}>
                <td><input type="date" value={editRow.tx_date} onChange={e => setEditRow(v => ({ ...v, tx_date: e.target.value }))} /></td>
                <td><input value={editRow.description} onChange={e => setEditRow(v => ({ ...v, description: e.target.value }))} /></td>
                <td>
                  <CategorySelect value={editRow.category} onChange={v => setEditRow(row => ({ ...row, category: v }))} categories={categories} />
                </td>
                <td><MoneyInput value={editRow.amount} onChange={v => setEditRow(row => ({ ...row, amount: v }))} /></td>
                <td><input type="checkbox" checked={!!r.checked} onChange={() => handleCheck(r)} /></td>
                <td style={{ display: 'flex', gap: '.3rem' }}>
                  <button className="secondary" onClick={() => saveEdit(r.id)}>Save</button>
                  <button className="secondary" onClick={cancelEdit}>Cancel</button>
                </td>
              </tr>
            ) : (
              <tr key={r.id}>
                <td>{r.tx_date}</td>
                <td>{r.description}</td>
                <td><span className="tag" style={{ background: categoryColor(r.category_name) + '22', color: categoryColor(r.category_name) }}>{r.category_name || 'Other'}</span></td>
                <td>{formatRupiah(r.amount)}</td>
                <td><input type="checkbox" checked={!!r.checked} onChange={() => handleCheck(r)} /></td>
                <td style={{ display: 'flex', gap: '.3rem' }}>
                  <button className="icon-btn round" onClick={() => startEdit(r)}><IconEdit width={14} height={14} /></button>
                  <button className="icon-btn danger round" onClick={() => handleDelete(r.id)}><IconClose width={14} height={14} /></button>
                </td>
              </tr>
            ))}
            {rows.length === 0 && <tr><td colSpan={6} className="muted">No transactions.</td></tr>}
          </tbody>
        </table>

        <div className="table-footer">
          <label style={{ display: 'flex', alignItems: 'center', gap: '.4rem', fontSize: '.85rem' }}>
            <span className="muted">Rows:</span>
            <select value={perPage} onChange={e => setPerPage(Number(e.target.value))}>
              {PAGE_SIZES.map(n => <option key={n} value={n}>{n}</option>)}
            </select>
          </label>

          {totalPages > 1 && (
            <div className="pagination">
              <button className="secondary" disabled={page <= 1} onClick={() => setPage(p => p - 1)}>‹ Prev</button>
              <span className="muted">Page {page} / {totalPages}</span>
              <button className="secondary" disabled={page >= totalPages} onClick={() => setPage(p => p + 1)}>Next ›</button>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
