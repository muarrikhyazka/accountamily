import { IconDashboard, IconSpending, IconBudget, IconAssets, IconSetup, IconPlus } from '../icons.jsx'

function Section({ Icon, title, children }) {
  return (
    <div className="card">
      <h2 style={{ display: 'flex', alignItems: 'center', gap: '.5rem' }}>
        <Icon width={18} height={18} /> {title}
      </h2>
      {children}
    </div>
  )
}

export default function Guide() {
  return (
    <div>
      <h1>Guide</h1>
      <p className="muted">How to use Tatadana, page by page.</p>

      <Section Icon={IconPlus} title="Add Spending">
        <p>Click the green <strong>Add Spending</strong> button in the sidebar from any page. Fill description, amount, category and date, then save. This is the fastest way to log a transaction without leaving your current page.</p>
      </Section>

      <Section Icon={IconDashboard} title="Dashboard">
        <p>Overview of your spending across <strong>Across Years</strong>, <strong>Year</strong>, <strong>Month</strong>, <strong>Week</strong> and <strong>Explore</strong> tabs. Use the sidebar's Year/Month/Value filters to narrow any chart.</p>
      </Section>

      <Section Icon={IconSpending} title="Spending">
        <p>Full transaction list for the selected year/month. Search by description, filter by category, sort any column. Click a row to edit category/date, or delete it. Use the category filter chips to isolate one category at a time.</p>
      </Section>

      <Section Icon={IconBudget} title="Budgeting">
        <p>Set your planned income per source and planned expense per category for a given year. Compare plan vs actual to see where you're over/under budget.</p>
      </Section>

      <Section Icon={IconAssets} title="Assets">
        <p>Track balances across accounts (liquid cash, savings, investments). Set a target net-worth goal and see progress toward it.</p>
      </Section>

      <Section Icon={IconSetup} title="Setup">
        <p>Manage master data: income sources, asset accounts, and spending categories. Categories are grouped under Needs / Wants / Savings — this grouping drives the budgeting breakdown. Deleting an item in use will ask to force-delete (existing transactions keep the old name as text, not removed).</p>
      </Section>

      <div className="card">
        <h2>Tips</h2>
        <ul style={{ margin: 0, paddingLeft: '1.2rem', lineHeight: 1.7 }}>
          <li>Works offline: spending added while offline queues locally and syncs once the server is reachable again (see the connection dot in the sidebar).</li>
          <li>Theme toggle (sun/moon icon) switches light/dark, saved per browser.</li>
          <li>Need to log spending from chat instead of opening the app? See the <strong>Integrations</strong> page to connect Telegram, Discord, or any other chat platform with an API.</li>
        </ul>
      </div>
    </div>
  )
}
