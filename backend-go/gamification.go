package main

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// RegisterGamificationRoutes wires the one read endpoint the frontend needs;
// points/streak are updated as a side effect of handleAddSpending (see
// awardPointsForLog below), not via their own write endpoint.
func RegisterGamificationRoutes(r chi.Router, app *App) {
	r.Get("/gamification", app.handleGetGamification)
}

const pointsPerLog = 10
const streakBonusPerDay = 5 // extra points per consecutive day, capped at streakBonusCap days
const streakBonusCap = 10

// badge thresholds, ordered low to high - first one not met is "next badge"
var badgeThresholds = []struct {
	Points int64
	Name   string
}{
	{0, "Getting Started"},
	{50, "Tracker"},
	{200, "Budget Buddy"},
	{500, "Money Master"},
	{1500, "Finance Wizard"},
}

type gamificationState struct {
	Points       int64  `json:"points"`
	StreakDays   int    `json:"streak_days"`
	Badge        string `json:"badge"`
	NextBadge    string `json:"next_badge,omitempty"`
	NextBadgeAt  int64  `json:"next_badge_at,omitempty"`
}

func currentBadge(points int64) (badge, next string, nextAt int64) {
	badge = badgeThresholds[0].Name
	for i, b := range badgeThresholds {
		if points >= b.Points {
			badge = b.Name
		}
		if points < b.Points && next == "" {
			next = b.Name
			nextAt = b.Points
		}
		_ = i
	}
	return
}

func (app *App) handleGetGamification(w http.ResponseWriter, r *http.Request) {
	aid, ok := app.requireAccount(w, r)
	if !ok {
		return
	}
	state := app.loadGamificationState(context.Background(), aid)
	writeJSON(w, http.StatusOK, state)
}

func (app *App) loadGamificationState(ctx context.Context, aid int64) gamificationState {
	var points int64
	var streak int
	err := app.DB.QueryRow(ctx, "SELECT points, streak_days FROM user_points WHERE account_id=$1", aid).Scan(&points, &streak)
	if err != nil {
		points, streak = 0, 0
	}
	badge, next, nextAt := currentBadge(points)
	return gamificationState{Points: points, StreakDays: streak, Badge: badge, NextBadge: next, NextBadgeAt: nextAt}
}

// awardPointsForLog is called from handleAddSpending right after a spending
// row is successfully inserted. Streak logic: logging on a new calendar day
// that's exactly 1 day after last_log_date extends the streak; logging again
// same day is a no-op for the streak (but still earns base points); any
// bigger gap resets the streak to 1. ponytail: streak uses the server's
// local date, not the user's timezone - fine for a single-family IDR app,
// revisit if this ever serves users across timezones.
func (app *App) awardPointsForLog(ctx context.Context, aid int64) {
	today := time.Now().Format("2006-01-02")
	var lastLog *string
	var streak int
	err := app.DB.QueryRow(ctx, "SELECT last_log_date::text, streak_days FROM user_points WHERE account_id=$1", aid).Scan(&lastLog, &streak)
	exists := err == nil

	newStreak := 1
	if exists && lastLog != nil {
		switch *lastLog {
		case today:
			newStreak = streak // already logged today, streak unchanged
		default:
			y, _ := time.Parse("2006-01-02", *lastLog)
			if time.Now().Format("2006-01-02") == y.AddDate(0, 0, 1).Format("2006-01-02") {
				newStreak = streak + 1
			} else {
				newStreak = 1
			}
		}
	}

	bonus := streakBonusPerDay * min(newStreak, streakBonusCap)
	earned := int64(0)
	if !exists || lastLog == nil || *lastLog != today {
		earned = int64(pointsPerLog + bonus)
	}

	if !exists {
		_, _ = app.DB.Exec(ctx,
			"INSERT INTO user_points (account_id, points, streak_days, last_log_date) VALUES ($1,$2,$3,$4)",
			aid, earned, newStreak, today)
		return
	}
	_, _ = app.DB.Exec(ctx,
		"UPDATE user_points SET points = points + $1, streak_days = $2, last_log_date = $3 WHERE account_id=$4",
		earned, newStreak, today, aid)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
