// Deterministic color per category name (not main_category) - consistent across pages.
const PALETTE = [
  '#ef4444', '#f97316', '#f59e0b', '#84cc16', '#22c55e', '#10b981',
  '#14b8a6', '#06b6d4', '#3b82f6', '#6366f1', '#8b5cf6', '#a855f7',
  '#d946ef', '#ec4899', '#f43f5e',
]

export function categoryColor(name) {
  if (!name) return '#94a3b8'
  let hash = 0
  for (let i = 0; i < name.length; i++) hash = (hash * 31 + name.charCodeAt(i)) >>> 0
  return PALETTE[hash % PALETTE.length]
}

// Small, visually-distinct series palette (for comparing few series side by side, e.g. weeks/years)
// unlike categoryColor's hash-based pick, this guarantees high contrast between adjacent picks.
const SERIES_PALETTE = ['#ef4444', '#3b82f6', '#22c55e', '#f59e0b', '#a855f7', '#06b6d4', '#ec4899', '#84cc16']

export function seriesColor(index) {
  return SERIES_PALETTE[index % SERIES_PALETTE.length]
}
