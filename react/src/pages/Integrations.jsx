import { useEffect, useState } from 'react'
import { api } from '../api'
import { IconTelegram, IconDiscord, IconLink, IconCopy, IconCheck, IconClose } from '../icons.jsx'

function CopyBlock({ text }) {
  const [copied, setCopied] = useState(false)
  function copy() {
    navigator.clipboard?.writeText(text)
    setCopied(true)
    setTimeout(() => setCopied(false), 1500)
  }
  return (
    <div className="copy-block">
      <code>{text}</code>
      <button className="icon-btn" onClick={copy} title="Copy">
        {copied ? <IconCheck width={13} height={13} /> : <IconCopy width={13} height={13} />}
      </button>
    </div>
  )
}

const PLATFORM_META = {
  telegram: { label: 'Telegram', Icon: IconTelegram, tokenHint: 'Bot token from @BotFather' },
  discord: { label: 'Discord', Icon: IconDiscord, tokenHint: 'Bot token from the Discord Developer Portal' },
}

function IntegrationCard({ integration, onUpdate, onDelete }) {
  const meta = PLATFORM_META[integration.platform]
  const [platformToken, setPlatformToken] = useState(integration.platform_token || '')
  const [chatId, setChatId] = useState(integration.chat_id || '')
  const [saved, setSaved] = useState(false)

  async function save() {
    await onUpdate(integration.id, { platform_token: platformToken, chat_id: chatId })
    setSaved(true)
    setTimeout(() => setSaved(false), 1500)
  }

  return (
    <div className="card">
      <h2 style={{ display: 'flex', alignItems: 'center', gap: '.5rem' }}>
        <meta.Icon width={18} height={18} /> {meta.label}
        <span className={`badge ${integration.enabled ? 'badge-savings' : ''}`} style={{ marginLeft: 'auto' }}>
          {integration.enabled ? 'Enabled' : 'Disabled'}
        </span>
        <button className="icon-btn danger round" onClick={() => onDelete(integration.id)} title="Remove"><IconClose width={12} height={12} /></button>
      </h2>

      <label style={{ display: 'flex', flexDirection: 'column', gap: '.3rem', marginBottom: '.6rem' }}>
        <span>{meta.tokenHint}</span>
        <input value={platformToken} onChange={e => setPlatformToken(e.target.value)} placeholder="paste bot token" type="password" />
      </label>
      <label style={{ display: 'flex', flexDirection: 'column', gap: '.3rem', marginBottom: '.6rem' }}>
        <span>Allowed chat/channel ID (optional - leave blank to allow any)</span>
        <input value={chatId} onChange={e => setChatId(e.target.value)} placeholder="e.g. -1001234567890" />
      </label>
      <div style={{ display: 'flex', gap: '.6rem', alignItems: 'center' }}>
        <button onClick={save}>Save</button>
        <label style={{ display: 'flex', alignItems: 'center', gap: '.4rem' }}>
          <input type="checkbox" checked={!!integration.enabled} onChange={e => onUpdate(integration.id, { enabled: e.target.checked })} />
          Enabled
        </label>
        {saved && <span className="muted" style={{ color: 'var(--primary)' }}>Saved</span>}
      </div>

      <h3 style={{ marginTop: '1rem' }}>Kinkit-issued secret</h3>
      <p className="muted">Give this to your bot process as <code>ACCOUNTAMILY_BOT_TOKEN</code> - it authenticates the bot's calls to your account, no separate login needed.</p>
      <CopyBlock text={integration.bot_token} />
    </div>
  )
}

export default function Integrations() {
  const [items, setItems] = useState(null)
  const [error, setError] = useState('')

  function load() { api.integrations().then(setItems).catch(e => setError(e.message)) }
  useEffect(load, [])

  async function connect(platform) {
    setError('')
    try {
      await api.addIntegration({ platform })
      load()
    } catch (e) { setError(e.message) }
  }

  async function handleUpdate(id, payload) {
    await api.updateIntegration(id, payload)
    load()
  }

  async function handleDelete(id) {
    if (!confirm('Remove this integration? The bot will stop working until reconnected.')) return
    await api.deleteIntegration(id)
    load()
  }

  if (!items) return <div className="loading">Loading...</div>

  const connected = new Set(items.map(i => i.platform))

  return (
    <div>
      <h1>Integrations</h1>
      <p className="muted">Connect a chat platform's bot to log spending, get reminders, and manage your household - no server access needed, all configured here.</p>
      {error && <div className="error-box">{error}</div>}

      {items.map(i => (
        <IntegrationCard key={i.id} integration={i} onUpdate={handleUpdate} onDelete={handleDelete} />
      ))}

      {!connected.has('telegram') && (
        <div className="card">
          <h2 style={{ display: 'flex', alignItems: 'center', gap: '.5rem' }}><IconTelegram width={18} height={18} /> Telegram</h2>
          <p className="muted">Not connected yet.</p>
          <button onClick={() => connect('telegram')}>Connect Telegram</button>
        </div>
      )}
      {!connected.has('discord') && (
        <div className="card">
          <h2 style={{ display: 'flex', alignItems: 'center', gap: '.5rem' }}><IconDiscord width={18} height={18} /> Discord</h2>
          <p className="muted">Not connected yet.</p>
          <button onClick={() => connect('discord')}>Connect Discord</button>
        </div>
      )}

      <div className="card">
        <h2 style={{ display: 'flex', alignItems: 'center', gap: '.5rem' }}>
          <IconLink width={18} height={18} /> How it works
        </h2>
        <p>Each platform you connect above gets its own secret token (<code>X-Bot-Token</code> header) tied only to your account. One shared bot process can serve every Kinkit user at once - it just looks up the right account from the token each message carries.</p>
        <table className="simple-table" style={{ marginTop: '.8rem' }}>
          <thead><tr><th>Action</th><th>Method & path</th><th>Body / params</th></tr></thead>
          <tbody>
            <tr><td>Bot config (reminders/keywords/categories)</td><td><code>GET /api/bot/config</code></td><td>-</td></tr>
            <tr><td>Add spending</td><td><code>POST /api/bot/spending</code></td><td><code>{'{ "tx_date", "description", "category", "amount" }'}</code></td></tr>
            <tr><td>Recent entries</td><td><code>GET /api/bot/spending/recent?n=5</code></td><td>-</td></tr>
            <tr><td>Search entries</td><td><code>GET /api/bot/spending/search?q=makan</code></td><td>-</td></tr>
            <tr><td>Update category</td><td><code>PATCH /api/bot/spending/&lt;id&gt;/category</code></td><td><code>{'{ "category" }'}</code></td></tr>
            <tr><td>Update date</td><td><code>PATCH /api/bot/spending/&lt;id&gt;/date</code></td><td><code>{'{ "tx_date" }'}</code></td></tr>
          </tbody>
        </table>
      </div>
    </div>
  )
}
