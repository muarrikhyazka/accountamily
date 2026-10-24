import { useEffect, useState } from 'react'
import { api } from './api'
import { useOffline } from './OfflineContext.jsx'
import { IconClose } from './icons.jsx'
import MoneyInput from './MoneyInput.jsx'
import CategorySelect from './CategorySelect.jsx'

const todayISO = () => new Date().toISOString().slice(0, 10)
const emptyRow = () => ({ tx_date: todayISO(), description: '', category: '', amount: '' })

export default function QuickAddSpending({ onClose, onSaved }) {
  const { addSpendingResilient } = useOffline()
  const [categories, setCategories] = useState([])
  const [rows, setRows] = useState([emptyRow()])
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [saving, setSaving] = useState(false)

  useEffect(() => { api.setup().then(s => setCategories(s.categories)).catch(e => setError(e.message)) }, [])

  function updateRow(i, field, value) {
    setRows(rs => rs.map((r, idx) => (idx === i ? { ...r, [field]: value } : r)))
  }
  function addRow() { setRows(rs => [...rs, emptyRow()]) }
  function removeRow(i) { setRows(rs => rs.filter((_, idx) => idx !== i)) }

  async function handleSubmit(e) {
    e.preventDefault()
    const valid = rows.filter(r => r.description && r.category && r.amount)
    if (valid.length === 0) { setError('Fill in at least one complete row.'); return }
    setSaving(true)
    setError('')
    let queuedCount = 0
    try {
      for (const r of valid) {
        const result = await addSpendingResilient({ ...r, amount: Number(r.amount) })
        if (result.queued) queuedCount++
      }
      if (queuedCount > 0) {
        setNotice(`${queuedCount} of ${valid.length} transaction(s) saved locally (server offline), will sync automatically.`)
      }
      onSaved && onSaved()
      if (queuedCount === 0) onClose()
    } catch (err) {
      setError(err.message)
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal quick-add" onClick={e => e.stopPropagation()}>
        <div className="modal-header">
          <h2>Add Spending</h2>
          <button className="icon-btn" onClick={onClose}><IconClose width={16} height={16} /></button>
        </div>

        {error && <div className="error-box">{error}</div>}
        {notice && <div className="notice-box">{notice}</div>}

        <form onSubmit={handleSubmit}>
          <div className="quick-add-rows">
            {rows.map((r, i) => (
              <div className="quick-add-row" key={i}>
                <input type="date" value={r.tx_date} onChange={e => updateRow(i, 'tx_date', e.target.value)} required />
                <input placeholder="Description" value={r.description} onChange={e => updateRow(i, 'description', e.target.value)} required />
                <CategorySelect value={r.category} onChange={v => updateRow(i, 'category', v)} categories={categories} required />
                <MoneyInput placeholder="Amount" value={r.amount} onChange={v => updateRow(i, 'amount', v)} required />
                <button type="button" className="icon-btn danger round" onClick={() => removeRow(i)} disabled={rows.length === 1}>
                  <IconClose width={12} height={12} />
                </button>
              </div>
            ))}
          </div>

          <div className="quick-add-actions">
            <button type="button" className="secondary" onClick={addRow}>+ Add Row</button>
            <button type="submit" disabled={saving}>{saving ? 'Saving...' : 'Save All'}</button>
          </div>
        </form>
      </div>
    </div>
  )
}
