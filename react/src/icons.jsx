// Minimal inline SVG icons (no emoji), stroke-based, 20x20, inherits currentColor.
const base = { width: 20, height: 20, viewBox: '0 0 24 24', fill: 'none', stroke: 'currentColor', strokeWidth: 1.8, strokeLinecap: 'round', strokeLinejoin: 'round' }

export const IconDashboard = (p) => (
  <svg {...base} {...p}><rect x="3" y="3" width="7" height="9" rx="1" /><rect x="14" y="3" width="7" height="5" rx="1" /><rect x="14" y="12" width="7" height="9" rx="1" /><rect x="3" y="16" width="7" height="5" rx="1" /></svg>
)
export const IconBudget = (p) => (
  <svg {...base} {...p}><rect x="3" y="5" width="18" height="14" rx="2" /><path d="M3 10h18" /><circle cx="7" cy="14.5" r="1" /></svg>
)
export const IconSpending = (p) => (
  <svg {...base} {...p}>
    <path d="M6 3h12v18l-2.5-1.5L13 21l-2.5-1.5L8 21l-2-1.5V3z" />
    <path d="M9 8h6M9 12h6M9 16h3" />
  </svg>
)
export const IconSummary = (p) => (
  <svg {...base} {...p}><path d="M4 19V9M10 19V5M16 19v-7M20 19H4" /></svg>
)
export const IconAssets = (p) => (
  <svg {...base} {...p}><path d="M3 7l9-4 9 4-9 4-9-4z" /><path d="M3 12l9 4 9-4M3 17l9 4 9-4" /></svg>
)
export const IconSetup = (p) => (
  <svg {...base} {...p}><circle cx="12" cy="12" r="3" /><path d="M19.4 15a1.7 1.7 0 00.34 1.87l.06.06a2 2 0 11-2.83 2.83l-.06-.06a1.7 1.7 0 00-1.87-.34 1.7 1.7 0 00-1 1.55V21a2 2 0 11-4 0v-.09a1.7 1.7 0 00-1-1.55 1.7 1.7 0 00-1.87.34l-.06.06a2 2 0 11-2.83-2.83l.06-.06a1.7 1.7 0 00.34-1.87 1.7 1.7 0 00-1.55-1H3a2 2 0 110-4h.09a1.7 1.7 0 001.55-1 1.7 1.7 0 00-.34-1.87l-.06-.06a2 2 0 112.83-2.83l.06.06a1.7 1.7 0 001.87.34H9a1.7 1.7 0 001-1.55V3a2 2 0 114 0v.09a1.7 1.7 0 001 1.55 1.7 1.7 0 001.87-.34l.06-.06a2 2 0 112.83 2.83l-.06.06a1.7 1.7 0 00-.34 1.87V9a1.7 1.7 0 001.55 1H21a2 2 0 110 4h-.09a1.7 1.7 0 00-1.55 1z" /></svg>
)
export const IconLogout = (p) => (
  <svg {...base} {...p}><path d="M9 21H5a2 2 0 01-2-2V5a2 2 0 012-2h4" /><path d="M16 17l5-5-5-5" /><path d="M21 12H9" /></svg>
)
export const IconCollapse = (p) => (
  <svg {...base} {...p}><path d="M11 17l-5-5 5-5M18 17l-5-5 5-5" /></svg>
)
export const IconExpand = (p) => (
  <svg {...base} {...p}><path d="M13 17l5-5-5-5M6 17l5-5-5-5" /></svg>
)
export const IconClose = (p) => (
  <svg {...base} strokeWidth={2.4} {...p}><path d="M18 6L6 18M6 6l12 12" /></svg>
)
export const IconEdit = (p) => (
  <svg {...base} {...p}><path d="M12 20h9" /><path d="M16.5 3.5a2.1 2.1 0 013 3L7 19l-4 1 1-4 12.5-12.5z" /></svg>
)
export const IconSun = (p) => (
  <svg {...base} {...p}><circle cx="12" cy="12" r="4" /><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4" /></svg>
)
export const IconMoon = (p) => (
  <svg {...base} {...p}><path d="M21 12.8A9 9 0 1111.2 3a7 7 0 009.8 9.8z" /></svg>
)
export const IconPlus = (p) => (
  <svg {...base} strokeWidth={2.2} {...p}><path d="M12 5v14M5 12h14" /></svg>
)
export const IconGuide = (p) => (
  <svg {...base} {...p}><path d="M4 19.5A2.5 2.5 0 016.5 17H20" /><path d="M6.5 2H20v20H6.5A2.5 2.5 0 014 19.5v-15A2.5 2.5 0 016.5 2z" /><path d="M9 7h8M9 11h8" /></svg>
)
export const IconLink = (p) => (
  <svg {...base} {...p}><path d="M10 13a5 5 0 007.07 0l2.83-2.83a5 5 0 00-7.07-7.07L11.5 4.5" /><path d="M14 11a5 5 0 00-7.07 0L4.1 13.83a5 5 0 007.07 7.07L12.5 19.5" /></svg>
)
export const IconTelegram = (p) => (
  <svg {...base} fill="currentColor" stroke="none" {...p}><path d="M21.5 4.5L2.7 12.1c-1.1.5-1.1 1.2-.2 1.5l4.6 1.4 1.8 5.5c.2.6.4.9.8.9.3 0 .5-.1.7-.4l2.6-2.5 5 3.6c.7.4 1.2.2 1.4-.6l3.1-14.5c.3-1-.3-1.5-1-1z"/></svg>
)
export const IconDiscord = (p) => (
  <svg {...base} fill="currentColor" stroke="none" {...p}><path d="M20 5.5A17 17 0 0015.5 4l-.4.8a13 13 0 017 4-6.5-3-11.2 0-6.5 3a13 13 0 017-4L4.5 4A17 17 0 000 5.5C-1 9 -1.3 12.5 -.9 16a16 16 0 004.9 2.5l1-1.6a9 9 0 01-1.9-.9l.5-.4a13 13 0 0016.8 0l.5.4a9 9 0 01-1.9.9l1 1.6A16 16 0 0024.9 16c.4-3.9-.2-7.4-4.9-10.5zM8.5 13.5c-.9 0-1.6-.8-1.6-1.8s.7-1.8 1.6-1.8 1.6.8 1.6 1.8-.7 1.8-1.6 1.8zm7 0c-.9 0-1.6-.8-1.6-1.8s.7-1.8 1.6-1.8 1.6.8 1.6 1.8-.7 1.8-1.6 1.8z"/></svg>
)
export const IconCopy = (p) => (
  <svg {...base} {...p}><rect x="9" y="9" width="12" height="12" rx="2"/><path d="M5 15H4a2 2 0 01-2-2V4a2 2 0 012-2h9a2 2 0 012 2v1"/></svg>
)
export const IconCheck = (p) => (
  <svg {...base} strokeWidth={2.4} {...p}><path d="M20 6L9 17l-5-5"/></svg>
)
export const IconSortUp = (p) => (
  <svg {...base} strokeWidth={2.2} {...p}><path d="M12 19V5M6 11l6-6 6 6"/></svg>
)
export const IconSortDown = (p) => (
  <svg {...base} strokeWidth={2.2} {...p}><path d="M12 5v14M6 13l6 6 6-6"/></svg>
)
export const IconShield = (p) => (
  <svg {...base} {...p}><path d="M12 3l8 3v6c0 5-3.5 8-8 9-4.5-1-8-4-8-9V6l8-3z" /><path d="M9 12l2 2 4-4" /></svg>
)
