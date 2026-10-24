import { useEffect, useState } from 'react'
import { api, formatRupiah } from '../api'
import MoneyInput from '../MoneyInput.jsx'
import { IconClose, IconEdit } from '../icons.jsx'

export default function Assets() {
  const [assets, setAssets] = useState([])
  const [target, setTarget] = useState(null)
  const [accounts, setAccounts] = useState([])
  const [error, setError] = useState('')
  const [form, setForm] = useState({ description: '', value: '', account_id: '', kind: 'liquid' })
  const [editingTarget, setEditingTarget] = useState(false)
  const [targetInput, setTargetInput] = useState('')

  function load() {
    setError('')
    api.assets().then(d => { setAssets(d.assets); setTarget(d.target) }).catch(e => setError(e.message))
  }
  useEffect(load, [])
  useEffect(() => { api.setup().then(s => setAccounts(s.accounts.filter(a => a.kind === 'asset'))).catch(() => {}) }, [])

  const liquid = assets.filter(a => a.kind === 'liquid')
  const nonLiquid = assets.filter(a => a.kind === 'non_liquid')
  const liquidTotal = liquid.reduce((s, a) => s + a.value, 0)
  const nonLiquidTotal = nonLiquid.reduce((s, a) => s + a.value, 0)
  const total = liquidTotal + nonLiquidTotal
  const targetPct = target?.target_value > 0 ? Math.min(100, Math.round((total / target.target_value) * 100)) : 0

  async function handleAdd(e) {
    e.preventDefault()
    if (!form.description || form.value === '') return
    try {
      await api.addAsset({ ...form, value: Number(form.value), account_id: form.account_id || null })
      setForm({ description: '', value: '', account_id: '', kind: 'liquid' })
      load()
    } catch (err) { setError(err.message) }
  }

  async function handleEditValue(id, value) {
    try {
      await api.updateAsset(id, { value: Number(value) || 0 })
      load()
    } catch (err) { setError(err.message) }
  }

  async function handleDelete(id) {
    if (!confirm('Delete this asset? This cannot be undone.')) return
    try {
      await api.deleteAsset(id)
      load()
    } catch (err) { setError(err.message) }
  }

  async function handleSaveTarget() {
    if (!targetInput) return
    try {
      await api.setAssetTarget({ target_value: Number(targetInput) })
      setEditingTarget(false)
      setTargetInput('')
      load()
    } catch (e) { setError(e.message) }
  }

  function AssetTable({ items, emptyLabel }) {
    return (
      <div className="table-scroll">
      <table>
        <thead><tr><th>Description</th><th>Account</th><th>Value</th><th></th></tr></thead>
        <tbody>
          {items.map(a => (
            <tr key={a.id}>
              <td>{a.description}</td>
              <td>{a.account_name || '-'}</td>
              <td><MoneyInput value={a.value} onBlur={v => handleEditValue(a.id, v)} /></td>
              <td><button className="icon-btn danger round" onClick={() => handleDelete(a.id)}><IconClose width={14} height={14} /></button></td>
            </tr>
          ))}
          {items.length === 0 && <tr><td colSpan={4} className="muted">{emptyLabel}</td></tr>}
        </tbody>
      </table>
      </div>
    )
  }

  return (
    <div>
      <h1>Assets</h1>
      <p className="muted">Lifetime Assets Tracker</p>
      {error && <div className="error-box">{error}</div>}

      <div className="cards-row">
        <div className="card stat stat-savings">
          <div className="muted">Total Assets Value</div>
          <div className="stat-value">{formatRupiah(total)}</div>
        </div>
        <div className="card stat stat-income">
          <div className="muted">Liquid Assets</div>
          <div className="stat-value">{formatRupiah(liquidTotal)}</div>
        </div>
        <div className="card stat stat-expense">
          <div className="muted">Non-Liquid Assets</div>
          <div className="stat-value">{formatRupiah(nonLiquidTotal)}</div>
        </div>
        <div className="card stat">
          <div className="muted">
            Target {target?.last_update && <span>(update {target.last_update})</span>}
          </div>
          {editingTarget ? (
            <div className="inline-form" style={{ marginTop: '.4rem' }}>
              <MoneyInput placeholder="Target value" value={targetInput} onChange={setTargetInput} />
              <button onClick={handleSaveTarget}>Save</button>
              <button className="secondary" onClick={() => setEditingTarget(false)}>Cancel</button>
            </div>
          ) : (
            <>
              <div className="stat-value">{target ? formatRupiah(target.target_value) : '-'}</div>
              <div className="progress-bar"><div className="progress-fill" style={{ width: `${targetPct}%` }} /></div>
              <span className="progress-label">{targetPct}%</span>
              <div>
                <button className="icon-btn" onClick={() => { setEditingTarget(true); setTargetInput(target?.target_value || '') }}>
                  <IconEdit width={14} height={14} /> edit target
                </button>
              </div>
            </>
          )}
        </div>
      </div>

      <form className="card inline-form" onSubmit={handleAdd}>
        <input placeholder="Description" value={form.description} onChange={e => setForm(f => ({ ...f, description: e.target.value }))} required />
        <MoneyInput placeholder="Value" value={form.value} onChange={v => setForm(f => ({ ...f, value: v }))} required />
        <select value={form.account_id} onChange={e => setForm(f => ({ ...f, account_id: e.target.value }))}>
          <option value="">Account...</option>
          {accounts.map(a => <option key={a.id} value={a.id}>{a.name}</option>)}
        </select>
        <select value={form.kind} onChange={e => setForm(f => ({ ...f, kind: e.target.value }))}>
          <option value="liquid">Liquid</option>
          <option value="non_liquid">Non-liquid</option>
        </select>
        <button type="submit">Add</button>
      </form>

      <div className="grid-2">
        <div className="card">
          <h2>Liquid Assets</h2>
          <AssetTable items={liquid} emptyLabel="No liquid assets yet." />
        </div>
        <div className="card">
          <h2>Non-Liquid Assets</h2>
          <AssetTable items={nonLiquid} emptyLabel="No non-liquid assets yet." />
        </div>
      </div>
    </div>
  )
}
