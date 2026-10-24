import { useEffect, useState } from 'react'
import { api } from '../api'
import { categoryColor } from '../categoryColor'
import { IconClose, IconEdit } from '../icons.jsx'
import { useAuth } from '../AuthContext.jsx'

const MAIN_CATS = ['Needs', 'Wants', 'Savings']
const CURRENCY_OPTIONS = ['IDR', 'USD', 'EUR', 'GBP', 'SGD', 'MYR', 'JPY', 'AUD', 'INR']

function AccountSettings() {
  const { user, setUserCurrency } = useAuth()
  const [currency, setCurrencyLocal] = useState(user?.currency || 'IDR')
  const [saved, setSaved] = useState(false)

  async function handleChange(e) {
    const code = e.target.value
    setCurrencyLocal(code)
    await api.updateSettings({ currency: code })
    setUserCurrency(code)
    setSaved(true)
    setTimeout(() => setSaved(false), 1500)
  }

  return (
    <div className="card">
      <h2>Account Settings</h2>
      <p className="muted">Display currency - changes how amounts are formatted everywhere, stored values don't change (no exchange-rate conversion).</p>
      <label style={{ display: 'flex', flexDirection: 'column', gap: '.3rem', maxWidth: '220px' }}>
        <span>Currency</span>
        <select value={currency} onChange={handleChange}>
          {CURRENCY_OPTIONS.map(c => <option key={c} value={c}>{c}</option>)}
        </select>
      </label>
      {saved && <p className="muted" style={{ color: 'var(--primary)' }}>Saved</p>}
    </div>
  )
}

function EditableList({ title, items, onAdd, onUpdate, onDelete, onChanged, extraLabel, dotColor }) {
  const [name, setName] = useState('')
  const [editing, setEditing] = useState(null)
  const [error, setError] = useState('')

  async function handleAdd(e) {
    e.preventDefault()
    if (!name.trim()) return
    setError('')
    try {
      await onAdd(name.trim())
      setName('')
      onChanged && onChanged()
    } catch (e) { setError(e.message) }
  }

  async function handleSave() {
    setError('')
    try {
      await onUpdate(editing.id, editing.name)
      setEditing(null)
      onChanged && onChanged()
    } catch (e) { setError(e.message) }
  }

  async function handleDelete(item) {
    setError('')
    try {
      await onDelete(item.id, false)
      onChanged && onChanged()
    } catch (e) {
      if (e.message.includes('in use')) {
        if (confirm(`"${item.name}" is currently in use. Force delete?`)) {
          try { await onDelete(item.id, true); onChanged && onChanged() } catch (e2) { setError(e2.message) }
        }
      } else setError(e.message)
    }
  }

  return (
    <div className="card">
      <h2>{title}</h2>
      {extraLabel && <p className="muted">{extraLabel}</p>}
      {error && <div className="error-box">{error}</div>}
      <form className="inline-form" onSubmit={handleAdd}>
        <input placeholder="New name..." value={name} onChange={e => setName(e.target.value)} />
        <button type="submit">Add</button>
      </form>
      <div className="category-list" style={{ marginTop: '.9rem' }}>
        {items.map(item => (
          <div className="category-chip" key={item.id}>
            {dotColor && <span className="cat-dot" style={{ background: dotColor(item.name) }} />}
            {editing?.id === item.id ? (
              <>
                <input value={editing.name} onChange={e => setEditing({ ...editing, name: e.target.value })} />
                <button onClick={handleSave}>Save</button>
                <button className="secondary" onClick={() => setEditing(null)}>Cancel</button>
              </>
            ) : (
              <>
                <span>{item.name}</span>
                <button className="icon-btn" onClick={() => setEditing({ id: item.id, name: item.name })}><IconEdit width={13} height={13} /></button>
                <button className="icon-btn danger round" onClick={() => handleDelete(item)}><IconClose width={12} height={12} /></button>
              </>
            )}
          </div>
        ))}
        {items.length === 0 && <div className="muted">No data yet.</div>}
      </div>
    </div>
  )
}

export default function Setup() {
  const [setup, setSetup] = useState({ accounts: [], categories: [], income_sources: [] })
  const [catName, setCatName] = useState('')
  const [catMain, setCatMain] = useState('Needs')
  const [catError, setCatError] = useState('')
  const [catEditing, setCatEditing] = useState(null)

  function load() {
    api.setup().then(setSetup)
  }
  useEffect(load, [])

  const assetAccounts = setup.accounts.filter(a => a.kind === 'asset')

  async function handleAddCategory(e) {
    e.preventDefault()
    setCatError('')
    if (!catName.trim()) return
    try {
      await api.addCategory({ name: catName.trim(), main_category: catMain })
      setCatName('')
      load()
    } catch (e) { setCatError(e.message) }
  }

  async function handleSaveCategory() {
    setCatError('')
    try {
      await api.updateCategory(catEditing.id, { name: catEditing.name, main_category: catEditing.main_category })
      setCatEditing(null)
      load()
    } catch (e) { setCatError(e.message) }
  }

  async function handleDeleteCategory(cat) {
    setCatError('')
    try {
      await api.deleteCategory(cat.id)
      load()
    } catch (e) {
      if (e.message.includes('in use')) {
        if (confirm(`Category "${cat.name}" is already in use. Force delete? (existing transactions remain)`)) {
          try { await api.deleteCategory(cat.id, true); load() } catch (e2) { setCatError(e2.message) }
        }
      } else setCatError(e.message)
    }
  }

  const groupedCats = MAIN_CATS.map(mc => ({ mc, items: setup.categories.filter(c => c.main_category === mc) }))

  return (
    <div>
      <h1>Setup</h1>
      <p className="muted">Manage master data: income sources, asset accounts, and spending categories - same as the ⚙️Setup sheet.</p>

      <AccountSettings />

      <EditableList
        title="Income Sources (Budgeting)"
        extraLabel="Salary/income sources for the Budgeting page"
        items={setup.income_sources}
        onAdd={(name) => api.addIncomeSource({ name })}
        onUpdate={(id, name) => api.updateIncomeSource(id, { name })}
        onDelete={(id, force) => api.deleteIncomeSource(id, force)}
        onChanged={load}
      />

      <EditableList
        title="Asset Accounts (Assets)"
        extraLabel="Used to tag sources on the Assets page"
        items={assetAccounts}
        onAdd={(name) => api.addAccount({ name, kind: 'asset' })}
        onUpdate={(id, name) => api.updateAccount(id, { name })}
        onDelete={(id, force) => api.deleteAccount(id, force)}
        onChanged={load}
      />

      <div className="card">
        <h2>Spending Categories</h2>
        <p className="muted">Add/remove your own categories - colors stay consistent across all pages.</p>
        {catError && <div className="error-box">{catError}</div>}
        <form className="inline-form" onSubmit={handleAddCategory}>
          <input placeholder="Category name" value={catName} onChange={e => setCatName(e.target.value)} />
          <select value={catMain} onChange={e => setCatMain(e.target.value)}>
            {MAIN_CATS.map(m => <option key={m} value={m}>{m}</option>)}
          </select>
          <button type="submit">Add</button>
        </form>

        {groupedCats.map(({ mc, items }) => (
          <div key={mc} style={{ marginTop: '1rem' }}>
            <span className={`badge badge-${mc.toLowerCase()}`}>{mc}</span>
            <div className="category-list" style={{ marginTop: '.6rem' }}>
              {items.map(cat => (
                <div className="category-chip" key={cat.id}>
                  <span className="cat-dot" style={{ background: categoryColor(cat.name) }} />
                  {catEditing?.id === cat.id ? (
                    <>
                      <input value={catEditing.name} onChange={e => setCatEditing({ ...catEditing, name: e.target.value })} />
                      <select value={catEditing.main_category} onChange={e => setCatEditing({ ...catEditing, main_category: e.target.value })}>
                        {MAIN_CATS.map(m => <option key={m} value={m}>{m}</option>)}
                      </select>
                      <button onClick={handleSaveCategory}>Save</button>
                      <button className="secondary" onClick={() => setCatEditing(null)}>Cancel</button>
                    </>
                  ) : (
                    <>
                      <span>{cat.name}</span>
                      <button className="icon-btn" onClick={() => setCatEditing({ id: cat.id, name: cat.name, main_category: cat.main_category })}><IconEdit width={13} height={13} /></button>
                      <button className="icon-btn danger round" onClick={() => handleDeleteCategory(cat)}><IconClose width={12} height={12} /></button>
                    </>
                  )}
                </div>
              ))}
              {items.length === 0 && <div className="muted">No {mc} categories yet.</div>}
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}
