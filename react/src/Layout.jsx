import { useState } from 'react'
import { NavLink, Outlet } from 'react-router-dom'
import { useAuth } from './AuthContext.jsx'
import { useYearMonth } from './YearContext.jsx'
import { useOffline } from './OfflineContext.jsx'
import { useTheme, THEMES } from './ThemeContext.jsx'
import { MONTH_NAMES } from './api'
import QuickAddSpending from './QuickAddSpending.jsx'
import { IconDashboard, IconBudget, IconSpending, IconAssets, IconSetup, IconLogout, IconCollapse, IconExpand, IconSun, IconMoon, IconPlus, IconGuide, IconLink, IconShield } from './icons.jsx'

const tenantLinks = [
  { to: '/', label: 'Dashboard', end: true, Icon: IconDashboard },
  { to: '/spending', label: 'Spending', Icon: IconSpending },
  { to: '/budgeting', label: 'Budgeting', Icon: IconBudget },
  { to: '/assets', label: 'Assets', Icon: IconAssets },
  { to: '/setup', label: 'Setup', Icon: IconSetup },
  { to: '/integrations', label: 'Integrations', Icon: IconLink },
  { to: '/guide', label: 'Guide', Icon: IconGuide },
]

// Super admin manages the platform, not personal finances - own tenant
// pages (budgeting/assets/spending/setup/integrations) are irrelevant noise.
const adminLinks = [
  { to: '/admin', label: 'Accounts', end: true, Icon: IconShield },
  { to: '/admin/stats', label: 'Platform Stats', Icon: IconDashboard },
]

const YEAR_RANGE = (() => {
  const y = new Date().getFullYear()
  const arr = []
  for (let i = y - 3; i <= y + 2; i++) arr.push(i)
  return arr
})()

export default function Layout() {
  const { logout, user } = useAuth()
  const { year, setYear, month, setMonth, valueMax, setValueMax } = useYearMonth()
  const { serverOnline, queueSize } = useOffline()
  const { theme, setTheme } = useTheme()
  const [collapsed, setCollapsed] = useState(false)
  const [quickAddOpen, setQuickAddOpen] = useState(false)

  return (
    <div className="app-shell">
      <aside className={`sidebar ${collapsed ? 'collapsed' : ''}`}>
        <div className="sidebar-top">
          {!collapsed && <div className="brand">Kinkit</div>}
          <button className="icon-btn collapse-toggle" onClick={() => setCollapsed(c => !c)} title={collapsed ? 'Expand' : 'Collapse'}>
            {collapsed ? <IconExpand /> : <IconCollapse />}
          </button>
        </div>

        <div className={`conn-status ${serverOnline ? 'online' : 'offline'}`} title={serverOnline ? 'Connected to server' : `Offline${queueSize ? ` - ${queueSize} transaction(s) pending sync` : ''}`}>
          <span className="conn-dot" />
          {!collapsed && <span>{serverOnline ? 'Online' : `Offline${queueSize ? ` (${queueSize})` : ''}`}</span>}
        </div>

        {!collapsed && !user?.is_admin && (
          <div className="sidebar-filters">
            <label>
              <span>Year</span>
              <select value={year} onChange={e => setYear(Number(e.target.value))}>
                {YEAR_RANGE.map(y => <option key={y} value={y}>{y}</option>)}
              </select>
            </label>
            <label>
              <span>Month</span>
              <select value={month} onChange={e => setMonth(Number(e.target.value))}>
                {MONTH_NAMES.map((m, i) => <option key={m} value={i + 1}>{m}</option>)}
              </select>
            </label>
            <label>
              <span>Value</span>
              <select value={valueMax} onChange={e => setValueMax(e.target.value)}>
                <option value="all">All</option>
                <option value="10jt">≤ 10 juta</option>
                <option value="100jt">≤ 100 juta</option>
              </select>
            </label>
          </div>
        )}

        <ul>
          {!user?.is_admin && (
            <li>
              <button className="nav-cta" onClick={() => setQuickAddOpen(true)} title="Add Spending">
                <IconPlus />
                {!collapsed && <span>Add Spending</span>}
              </button>
            </li>
          )}
          {(user?.is_admin ? adminLinks : tenantLinks).map(({ to, label, end, Icon }) => (
            <li key={to}>
              <NavLink to={to} end={end} title={label}>
                <Icon />
                {!collapsed && <span>{label}</span>}
              </NavLink>
            </li>
          ))}
        </ul>
        <div className="sidebar-footer">
          {!collapsed ? (
            <select
              className="theme-select"
              value={theme}
              onChange={e => setTheme(e.target.value)}
              title="Theme"
            >
              {THEMES.map(t => <option key={t.id} value={t.id}>{t.label}</option>)}
            </select>
          ) : (
            <button
              className="icon-btn theme-toggle"
              onClick={() => setTheme(THEMES[(THEMES.findIndex(t => t.id === theme) + 1) % THEMES.length].id)}
              title="Cycle theme"
            >
              {theme === 'dark' ? <IconSun /> : <IconMoon />}
            </button>
          )}
          {!collapsed && <span>{user?.display_name}</span>}
          <button className="icon-btn logout-btn" onClick={logout} title="Logout"><IconLogout /></button>
        </div>
      </aside>
      <main className="content">
        <Outlet />
      </main>
      {quickAddOpen && <QuickAddSpending onClose={() => setQuickAddOpen(false)} onSaved={() => setQuickAddOpen(false)} />}
    </div>
  )
}
