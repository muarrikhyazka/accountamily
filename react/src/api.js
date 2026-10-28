const BASE = '/api'

async function req(path, opts = {}) {
  const res = await fetch(BASE + path, {
    credentials: 'include',
    headers: opts.body ? { 'Content-Type': 'application/json' } : undefined,
    ...opts,
  })
  let data = null
  try { data = await res.json() } catch { /* no body */ }
  if (!res.ok) throw new Error(data?.error || `HTTP ${res.status}`)
  return data
}

export const api = {
  ping: () => req('/ping'),
  login: (username, password) => req('/login', { method: 'POST', body: JSON.stringify({ username, password }) }),
  signup: (email, password, display_name) => req('/signup', { method: 'POST', body: JSON.stringify({ email, password, display_name }) }),
  logout: () => req('/logout', { method: 'POST' }),
  me: () => req('/me'),
  updateSettings: (payload) => req('/me/settings', { method: 'PATCH', body: JSON.stringify(payload) }),
  currencies: () => req('/currencies'),
  fxRates: () => req('/fx-rates'),
  integrations: () => req('/integrations'),
  addIntegration: (payload) => req('/integrations', { method: 'POST', body: JSON.stringify(payload) }),
  updateIntegration: (id, payload) => req(`/integrations/${id}`, { method: 'PATCH', body: JSON.stringify(payload) }),
  deleteIntegration: (id) => req(`/integrations/${id}`, { method: 'DELETE' }),
  adminAccounts: () => req('/admin/accounts'),
  adminUpdateAccount: (id, payload) => req(`/admin/accounts/${id}`, { method: 'PATCH', body: JSON.stringify(payload) }),
  adminDeleteAccount: (id) => req(`/admin/accounts/${id}`, { method: 'DELETE' }),
  adminStats: () => req('/admin/stats'),
  adminResetDemoData: () => req('/admin/reset-demo-data', { method: 'POST' }),
  setup: () => req('/setup'),
  addCategory: (payload) => req('/categories', { method: 'POST', body: JSON.stringify(payload) }),
  updateCategory: (id, payload) => req(`/categories/${id}`, { method: 'PATCH', body: JSON.stringify(payload) }),
  deleteCategory: (id, force) => req(`/categories/${id}${force ? '?force=1' : ''}`, { method: 'DELETE' }),
  addAccount: (payload) => req('/accounts', { method: 'POST', body: JSON.stringify(payload) }),
  updateAccount: (id, payload) => req(`/accounts/${id}`, { method: 'PATCH', body: JSON.stringify(payload) }),
  deleteAccount: (id) => req(`/accounts/${id}`, { method: 'DELETE' }),
  addIncomeSource: (payload) => req('/income-sources', { method: 'POST', body: JSON.stringify(payload) }),
  updateIncomeSource: (id, payload) => req(`/income-sources/${id}`, { method: 'PATCH', body: JSON.stringify(payload) }),
  deleteIncomeSource: (id, force) => req(`/income-sources/${id}${force ? '?force=1' : ''}`, { method: 'DELETE' }),
  trend: (year, month, count = 6, valueMax = 'all') => req(`/trend?year=${year}&month=${month}&count=${count}${valueMax !== 'all' ? `&value_max=${valueMax}` : ''}`),
  budgeting: (year) => req(`/budgeting/${year}`),
  setBudgetIncome: (payload) => req('/budgeting/income', { method: 'POST', body: JSON.stringify(payload) }),
  setBudgetExpense: (payload) => req('/budgeting/expense', { method: 'POST', body: JSON.stringify(payload) }),
  spending: (year, month, page = 1, perPage = 20, opts = {}) => {
    const params = new URLSearchParams({ year, month, page, per_page: perPage })
    if (opts.search) params.set('search', opts.search)
    if (opts.category) params.set('category', opts.category)
    if (opts.sort) params.set('sort', opts.sort)
    if (opts.order) params.set('order', opts.order)
    if (opts.valueMax && opts.valueMax !== 'all') params.set('value_max', opts.valueMax)
    return req(`/spending?${params.toString()}`)
  },
  addSpending: (payload) => req('/spending', { method: 'POST', body: JSON.stringify(payload) }),
  updateSpending: (id, payload) => req(`/spending/${id}`, { method: 'PATCH', body: JSON.stringify(payload) }),
  deleteSpending: (id) => req(`/spending/${id}`, { method: 'DELETE' }),
  assets: () => req('/assets'),
  addAsset: (payload) => req('/assets', { method: 'POST', body: JSON.stringify(payload) }),
  updateAsset: (id, payload) => req(`/assets/${id}`, { method: 'PATCH', body: JSON.stringify(payload) }),
  deleteAsset: (id) => req(`/assets/${id}`, { method: 'DELETE' }),
  setAssetTarget: (payload) => req('/assets/target', { method: 'POST', body: JSON.stringify(payload) }),
  report: (year, month, valueMax = 'all') => req(`/report/${year}/${month}${valueMax !== 'all' ? `?value_max=${valueMax}` : ''}`),
  reportYear: (year, valueMax = 'all') => req(`/report/year/${year}${valueMax !== 'all' ? `?value_max=${valueMax}` : ''}`),
  reportMonthCompare: (year, month, valueMax = 'all') => req(`/report/month-compare/${year}/${month}${valueMax !== 'all' ? `?value_max=${valueMax}` : ''}`),
  patternsYear: (year, valueMax = 'all') => req(`/patterns/year/${year}${valueMax !== 'all' ? `?value_max=${valueMax}` : ''}`),
  weeklyReport: (year, month, valueMax = 'all') => req(`/report/weekly/${year}/${month}${valueMax !== 'all' ? `?value_max=${valueMax}` : ''}`),
  years: (valueMax = 'all') => req(`/years${valueMax !== 'all' ? `?value_max=${valueMax}` : ''}`),
  yearsByCategory: (valueMax = 'all') => req(`/years/by-category${valueMax !== 'all' ? `?value_max=${valueMax}` : ''}`),
  yearsMonthly: (valueMax = 'all') => req(`/years/monthly${valueMax !== 'all' ? `?value_max=${valueMax}` : ''}`),
  yearsDayOfMonthAvg: (valueMax = 'all') => req(`/years/day-of-month-avg${valueMax !== 'all' ? `?value_max=${valueMax}` : ''}`),
  yearsDatemonthAvg: (valueMax = 'all') => req(`/years/datemonth-avg${valueMax !== 'all' ? `?value_max=${valueMax}` : ''}`),
  summary: (year, month) => req(`/summary?year=${year}&month=${month}`),
}

// Currency formatting WITH real FX conversion. Amounts are always stored in
// IDR; _fxRates (fetched once on app load, see fetchFxRates/AuthContext)
// holds { IDR: 1, USD: 0.000055, ... } from the backend's cached FX rates.
// formatRupiah converts IDR -> selected currency before formatting.
let _currency = { code: 'IDR', symbol: 'Rp', locale: 'id-ID' }
let _fxRates = { IDR: 1 }
const CURRENCY_META = {
  IDR: { symbol: 'Rp', locale: 'id-ID' }, USD: { symbol: '$', locale: 'en-US' },
  EUR: { symbol: '\u20ac', locale: 'de-DE' }, GBP: { symbol: '\u00a3', locale: 'en-GB' },
  SGD: { symbol: 'S$', locale: 'en-SG' }, MYR: { symbol: 'RM', locale: 'ms-MY' },
  JPY: { symbol: '\u00a5', locale: 'ja-JP' }, AUD: { symbol: 'A$', locale: 'en-AU' },
  INR: { symbol: '\u20b9', locale: 'en-IN' },
}
export function setCurrency(code) {
  const meta = CURRENCY_META[code] || CURRENCY_META.IDR
  _currency = { code: code || 'IDR', ...meta }
}
// Call once on app load (AuthContext) to populate the FX table used below.
export async function fetchFxRates() {
  try {
    const d = await api.fxRates()
    if (d?.rates) _fxRates = d.rates
  } catch { /* keep previous/default rates (IDR-only) on failure */ }
}
export function formatRupiah(n) {
  const idr = Number(n) || 0
  const rate = _fxRates[_currency.code] ?? (_currency.code === 'IDR' ? 1 : null)
  // ponytail: no rate cached/fetched yet (or code unknown) -> show as IDR rather than a wrong number
  const v = Math.round(rate != null ? idr * rate : idr)
  const symbol = rate != null ? _currency.symbol : CURRENCY_META.IDR.symbol
  const locale = rate != null ? _currency.locale : CURRENCY_META.IDR.locale
  return symbol + v.toLocaleString(locale)
}

export const MONTHS = ['Jan','Feb','Mar','Apr','May','Jun','Jul','Aug','Sep','Oct','Nov','Dec']
export const MONTH_NAMES = ['January','February','March','April','May','June','July','August','September','October','November','December']
