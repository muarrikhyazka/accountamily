import { useEffect, useState } from 'react'
import { api } from './api'

/**
 * Compact points/streak/badge strip for the sidebar - the "gamification to
 * encourage kids" feature. refreshKey bump (e.g. after saving a spending
 * entry) re-fetches so the points update immediately.
 */
export default function GamificationBadge({ collapsed, refreshKey }) {
  const [state, setState] = useState(null)

  useEffect(() => {
    api.gamification().then(setState).catch(() => {})
  }, [refreshKey])

  if (!state) return null

  if (collapsed) {
    return (
      <div className="gami-badge-collapsed" title={`${state.points} pts · ${state.streak_days} day streak · ${state.badge}`}>
        🔥{state.streak_days}
      </div>
    )
  }

  return (
    <div className="gami-badge">
      <div className="gami-row">
        <span className="gami-points">⭐ {state.points} pts</span>
        <span className="gami-streak">🔥 {state.streak_days}d</span>
      </div>
      <div className="gami-badge-name">{state.badge}</div>
      {state.next_badge && (
        <div className="progress-bar" title={`${state.next_badge_at - state.points} pts to ${state.next_badge}`}>
          <div className="progress-fill" style={{ width: `${Math.min(100, (state.points / state.next_badge_at) * 100)}%` }} />
        </div>
      )}
    </div>
  )
}
