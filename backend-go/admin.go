package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// RegisterAdminRoutes wires: super-admin endpoints (/admin/accounts,
// /admin/stats, /admin/reset-demo-data), per-account integrations CRUD
// (/integrations), bot-token-authenticated endpoints (/bot/config,
// /bot/spending, /bot/spending/recent, /bot/spending/search,
// /bot/spending/<id>/category, /bot/spending/<id>/date), and
// service-token-authenticated internal endpoints for the bot multiplexer
// processes (/internal/integrations, /internal/integrations-version). See
// the Python app.py routes of the same names for the exact contract.
func RegisterAdminRoutes(r chi.Router, app *App) {
	// ---------- Per-account integrations CRUD (login required) ----------
	r.Get("/integrations", requireLogin(app, listIntegrations))
	r.Post("/integrations", requireLogin(app, addIntegration))
	r.Patch("/integrations/{id}", requireLogin(app, updateIntegration))
	r.Delete("/integrations/{id}", requireLogin(app, deleteIntegration))

	// ---------- Super-admin ----------
	r.Get("/admin/stats", requireAdmin(app, adminStats))
	r.Get("/admin/accounts", requireAdmin(app, adminListAccounts))
	r.Patch("/admin/accounts/{id}", requireAdmin(app, adminUpdateAccount))
	r.Delete("/admin/accounts/{id}", requireAdmin(app, adminDeleteAccount))
	r.Post("/admin/reset-demo-data", requireAdmin(app, adminResetDemoData))

	// ---------- Bot-token authenticated (no session cookie) ----------
	r.Get("/bot/config", botAuth(app, botConfig))
	r.Post("/bot/spending", botAuth(app, botAddSpending))
	r.Get("/bot/spending/recent", botAuth(app, botRecentSpending))
	r.Get("/bot/spending/search", botAuth(app, botSearchSpending))
	r.Patch("/bot/spending/{id}/category", botAuth(app, botUpdateCategory))
	r.Patch("/bot/spending/{id}/date", botAuth(app, botUpdateDate))

	// ---------- Service-token authenticated internal (bot multiplexers) ----------
	r.Get("/internal/integrations", serviceAuth(app, internalListIntegrations))
	r.Get("/internal/integrations-version", serviceAuth(app, internalIntegrationsVersion))
}

// ---------- auth helpers ----------

func requireLogin(app *App, h func(http.ResponseWriter, *http.Request, *App, int64)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		aid, ok := currentAccountID(r)
		if !ok {
			jsonError(w, http.StatusUnauthorized, "login required")
			return
		}
		h(w, r, app, aid)
	}
}

func requireAdmin(app *App, h func(http.ResponseWriter, *http.Request, *App, int64)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		aid, ok := currentAccountID(r)
		if !ok {
			jsonError(w, http.StatusUnauthorized, "login required")
			return
		}
		var isAdmin bool
		err := app.DB.QueryRow(context.Background(), "SELECT is_admin FROM users WHERE id=$1", aid).Scan(&isAdmin)
		if err != nil || !isAdmin {
			jsonError(w, http.StatusForbidden, "admin required")
			return
		}
		h(w, r, app, aid)
	}
}

// botAuth resolves the account via X-Bot-Token (matched against
// integrations.bot_token), mirroring Python's _check_bot_token().
func botAuth(app *App, h func(http.ResponseWriter, *http.Request, *App, int64)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("X-Bot-Token")
		if token == "" {
			jsonError(w, http.StatusForbidden, "forbidden")
			return
		}
		var aid int64
		err := app.DB.QueryRow(context.Background(),
			"SELECT account_id FROM integrations WHERE bot_token=$1 AND enabled=TRUE", token).Scan(&aid)
		if err != nil {
			jsonError(w, http.StatusForbidden, "forbidden")
			return
		}
		h(w, r, app, aid)
	}
}

// serviceAuth checks X-Service-Token against app.Cfg.ServiceToken.
func serviceAuth(app *App, h func(http.ResponseWriter, *http.Request, *App)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if app.Cfg.ServiceToken == "" || r.Header.Get("X-Service-Token") != app.Cfg.ServiceToken {
			jsonError(w, http.StatusForbidden, "forbidden")
			return
		}
		h(w, r, app)
	}
}

// bumpIntegrationsVersion signals bot multiplexer processes that
// integrations changed; ported from Python's bump_integrations_version().
func bumpIntegrationsVersion(ctx context.Context, app *App) error {
	_, err := app.DB.Exec(ctx,
		"INSERT INTO meta (key, value) VALUES ('integrations_version', $1) "+
			"ON CONFLICT (key) DO UPDATE SET value=excluded.value",
		strconv.FormatInt(time.Now().Unix(), 10))
	return err
}

func pathInt64(r *http.Request, key string) (int64, bool) {
	v, err := strconv.ParseInt(chi.URLParam(r, key), 10, 64)
	return v, err == nil
}

// ---------- Integrations CRUD ----------

type integrationRow struct {
	ID            int64     `json:"id"`
	Platform      string    `json:"platform"`
	ChatID        *string   `json:"chat_id"`
	Enabled       bool      `json:"enabled"`
	BotToken      string    `json:"bot_token"`
	PlatformToken *string   `json:"platform_token"`
	CreatedAt     time.Time `json:"created_at"`
}

func listIntegrations(w http.ResponseWriter, r *http.Request, app *App, aid int64) {
	rows, err := app.DB.Query(context.Background(),
		"SELECT id, platform, chat_id, enabled, bot_token, platform_token, created_at FROM integrations WHERE account_id=$1 ORDER BY id", aid)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []integrationRow{}
	for rows.Next() {
		var ir integrationRow
		if err := rows.Scan(&ir.ID, &ir.Platform, &ir.ChatID, &ir.Enabled, &ir.BotToken, &ir.PlatformToken, &ir.CreatedAt); err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, ir)
	}
	writeJSON(w, http.StatusOK, out)
}

func genBotToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func addIntegration(w http.ResponseWriter, r *http.Request, app *App, aid int64) {
	var d struct {
		Platform      string  `json:"platform"`
		PlatformToken *string `json:"platform_token"`
		ChatID        *string `json:"chat_id"`
	}
	if err := decodeJSON(r, &d); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if d.Platform != "telegram" && d.Platform != "discord" {
		jsonError(w, http.StatusBadRequest, "platform must be telegram or discord")
		return
	}
	botToken, err := genBotToken()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var newID int64
	err = app.DB.QueryRow(context.Background(),
		"INSERT INTO integrations (account_id, platform, bot_token, platform_token, chat_id) VALUES ($1,$2,$3,$4,$5) RETURNING id",
		aid, d.Platform, botToken, d.PlatformToken, d.ChatID).Scan(&newID)
	if err != nil {
		// unique violation (account_id,platform) or bot_token collision
		jsonError(w, http.StatusConflict, "integration for this platform already exists")
		return
	}
	if err := bumpIntegrationsVersion(context.Background(), app); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": newID, "bot_token": botToken})
}

func updateIntegration(w http.ResponseWriter, r *http.Request, app *App, aid int64) {
	iid, ok := pathInt64(r, "id")
	if !ok {
		jsonError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var d map[string]any
	if err := decodeJSON(r, &d); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid json")
		return
	}
	var sets []string
	var params []any
	n := 1
	for _, k := range []string{"platform_token", "chat_id"} {
		if v, present := d[k]; present {
			sets = append(sets, k+"=$"+strconv.Itoa(n))
			params = append(params, v)
			n++
		}
	}
	if v, present := d["enabled"]; present {
		sets = append(sets, "enabled=$"+strconv.Itoa(n))
		params = append(params, v)
		n++
	}
	if len(sets) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	params = append(params, iid, aid)
	q := "UPDATE integrations SET " + strings.Join(sets, ",") +
		" WHERE id=$" + strconv.Itoa(n) + " AND account_id=$" + strconv.Itoa(n+1)
	if _, err := app.DB.Exec(context.Background(), q, params...); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := bumpIntegrationsVersion(context.Background(), app); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func deleteIntegration(w http.ResponseWriter, r *http.Request, app *App, aid int64) {
	iid, ok := pathInt64(r, "id")
	if !ok {
		jsonError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if _, err := app.DB.Exec(context.Background(), "DELETE FROM integrations WHERE id=$1 AND account_id=$2", iid, aid); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := bumpIntegrationsVersion(context.Background(), app); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---------- Super-admin ----------

func adminStats(w http.ResponseWriter, r *http.Request, app *App, _ int64) {
	ctx := context.Background()
	var total, active, disabled int64
	if err := app.DB.QueryRow(ctx,
		"SELECT COUNT(*), COALESCE(SUM(CASE WHEN is_disabled THEN 0 ELSE 1 END),0), COALESCE(SUM(CASE WHEN is_disabled THEN 1 ELSE 0 END),0) FROM users").
		Scan(&total, &active, &disabled); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var spendCount, spendTotal int64
	if err := app.DB.QueryRow(ctx, "SELECT COUNT(*), COALESCE(SUM(amount),0) FROM spending").
		Scan(&spendCount, &spendTotal); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	rows, err := app.DB.Query(ctx, "SELECT platform, COUNT(*) FROM integrations WHERE enabled=TRUE GROUP BY platform")
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	byPlatform := []map[string]any{}
	for rows.Next() {
		var platform string
		var n int64
		if err := rows.Scan(&platform, &n); err != nil {
			rows.Close()
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		byPlatform = append(byPlatform, map[string]any{"platform": platform, "n": n})
	}
	rows.Close()

	rows2, err := app.DB.Query(ctx,
		"SELECT to_char(created_at,'YYYY-MM-DD') AS day, COUNT(*) FROM users GROUP BY day ORDER BY day DESC LIMIT 30")
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	signups := []map[string]any{}
	for rows2.Next() {
		var day string
		var n int64
		if err := rows2.Scan(&day, &n); err != nil {
			rows2.Close()
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		signups = append(signups, map[string]any{"day": day, "n": n})
	}
	rows2.Close()

	writeJSON(w, http.StatusOK, map[string]any{
		"accounts":                 map[string]any{"total": total, "active": active, "disabled": disabled},
		"spending":                 map[string]any{"count": spendCount, "total": spendTotal},
		"integrations_by_platform": byPlatform,
		"signups_by_day":           signups,
	})
}

type adminAccountRow struct {
	ID               int64     `json:"id"`
	Email            string    `json:"email"`
	DisplayName      *string   `json:"display_name"`
	Currency         string    `json:"currency"`
	IsAdmin          bool      `json:"is_admin"`
	IsDisabled       bool      `json:"is_disabled"`
	CreatedAt        time.Time `json:"created_at"`
	SpendingCount    int64     `json:"spending_count"`
	IntegrationCount int64     `json:"integration_count"`
}

func adminListAccounts(w http.ResponseWriter, r *http.Request, app *App, _ int64) {
	rows, err := app.DB.Query(context.Background(), `
		SELECT u.id, u.email, u.display_name, u.currency, u.is_admin, u.is_disabled, u.created_at,
		       (SELECT COUNT(*) FROM spending WHERE account_id=u.id) AS spending_count,
		       (SELECT COUNT(*) FROM integrations WHERE account_id=u.id) AS integration_count
		FROM users u ORDER BY u.id`)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []adminAccountRow{}
	for rows.Next() {
		var a adminAccountRow
		if err := rows.Scan(&a.ID, &a.Email, &a.DisplayName, &a.Currency, &a.IsAdmin, &a.IsDisabled, &a.CreatedAt,
			&a.SpendingCount, &a.IntegrationCount); err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, a)
	}
	writeJSON(w, http.StatusOK, out)
}

func adminUpdateAccount(w http.ResponseWriter, r *http.Request, app *App, _ int64) {
	uid, ok := pathInt64(r, "id")
	if !ok {
		jsonError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var d map[string]any
	if err := decodeJSON(r, &d); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid json")
		return
	}
	var sets []string
	var params []any
	n := 1
	if v, present := d["is_disabled"]; present {
		sets = append(sets, "is_disabled=$"+strconv.Itoa(n))
		params = append(params, v)
		n++
	}
	if v, present := d["is_admin"]; present {
		sets = append(sets, "is_admin=$"+strconv.Itoa(n))
		params = append(params, v)
		n++
	}
	if len(sets) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	params = append(params, uid)
	q := "UPDATE users SET " + strings.Join(sets, ",") + " WHERE id=$" + strconv.Itoa(n)
	if _, err := app.DB.Exec(context.Background(), q, params...); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func adminDeleteAccount(w http.ResponseWriter, r *http.Request, app *App, selfID int64) {
	uid, ok := pathInt64(r, "id")
	if !ok {
		jsonError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if uid == selfID {
		jsonError(w, http.StatusBadRequest, "cannot delete your own admin account")
		return
	}
	ctx := context.Background()
	tables := []string{"spending", "budget_income", "budget_expense", "savings_list", "assets", "asset_target",
		"accounts", "categories", "income_sources", "integrations", "reminder_schedules",
		"mandatory_ingredients", "category_keywords"}
	tx, err := app.DB.Begin(ctx)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback(ctx)
	for _, t := range tables {
		if _, err := tx.Exec(ctx, "DELETE FROM "+t+" WHERE account_id=$1", uid); err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if _, err := tx.Exec(ctx, "DELETE FROM users WHERE id=$1", uid); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := tx.Commit(ctx); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// adminResetDemoData: Python's version calls into seed_demo.py (a
// Python-only random-data generator) to wipe + reseed the hardcoded
// demo1/demo2 accounts. That generator isn't ported here - out of scope
// for this Go rewrite - so this stubs the endpoint as 501 rather than
// silently no-op or reimplementing a random demo-data generator.
func adminResetDemoData(w http.ResponseWriter, r *http.Request, app *App, _ int64) {
	jsonError(w, http.StatusNotImplemented, "reset-demo-data not implemented in Go rewrite: depends on Python seed_demo.py, out of scope")
}

// ---------- Bot-token authenticated ----------

func botConfig(w http.ResponseWriter, r *http.Request, app *App, aid int64) {
	ctx := context.Background()
	reminders := []map[string]any{}
	rows, err := app.DB.Query(ctx, "SELECT id, account_id, job_name, days, hour, minute, enabled, last_run FROM reminder_schedules WHERE account_id=$1 AND enabled=TRUE", aid)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for rows.Next() {
		var id, accountID, hour, minute int64
		var jobName, days string
		var enabled bool
		var lastRun *string
		if err := rows.Scan(&id, &accountID, &jobName, &days, &hour, &minute, &enabled, &lastRun); err != nil {
			rows.Close()
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		reminders = append(reminders, map[string]any{
			"id": id, "account_id": accountID, "job_name": jobName, "days": days,
			"hour": hour, "minute": minute, "enabled": enabled, "last_run": lastRun,
		})
	}
	rows.Close()

	ingredients := []string{}
	rows2, err := app.DB.Query(ctx, "SELECT name FROM mandatory_ingredients WHERE account_id=$1", aid)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for rows2.Next() {
		var name string
		if err := rows2.Scan(&name); err != nil {
			rows2.Close()
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		ingredients = append(ingredients, name)
	}
	rows2.Close()

	keywords := map[string]string{}
	rows3, err := app.DB.Query(ctx, "SELECT keyword, category FROM category_keywords WHERE account_id=$1", aid)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for rows3.Next() {
		var k, c string
		if err := rows3.Scan(&k, &c); err != nil {
			rows3.Close()
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		keywords[k] = c
	}
	rows3.Close()

	categories := []string{}
	rows4, err := app.DB.Query(ctx, "SELECT name FROM categories WHERE account_id=$1", aid)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for rows4.Next() {
		var name string
		if err := rows4.Scan(&name); err != nil {
			rows4.Close()
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		categories = append(categories, name)
	}
	rows4.Close()

	writeJSON(w, http.StatusOK, map[string]any{
		"reminders":             reminders,
		"mandatory_ingredients": ingredients,
		"category_keywords":     keywords,
		"categories":            categories,
	})
}

func botAddSpending(w http.ResponseWriter, r *http.Request, app *App, aid int64) {
	var d struct {
		TxDate      string  `json:"tx_date"`
		Description string  `json:"description"`
		Category    *string `json:"category"`
		Amount      int64   `json:"amount"`
		Source      *string `json:"source"`
	}
	if err := decodeJSON(r, &d); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid json")
		return
	}
	ctx := context.Background()
	var catID *int64
	if d.Category != nil && *d.Category != "" {
		var id int64
		err := app.DB.QueryRow(ctx, "SELECT id FROM categories WHERE name=$1 AND account_id=$2", *d.Category, aid).Scan(&id)
		if err == nil {
			catID = &id
		}
	}
	source := "telegram"
	if d.Source != nil && *d.Source != "" {
		source = *d.Source
	}
	var newID int64
	err := app.DB.QueryRow(ctx,
		"INSERT INTO spending (account_id, tx_date, description, category_id, category_name, amount, source) VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id",
		aid, d.TxDate, d.Description, catID, d.Category, d.Amount, source).Scan(&newID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": newID})
}

func botRecentSpending(w http.ResponseWriter, r *http.Request, app *App, aid int64) {
	n := 5
	if v := r.URL.Query().Get("n"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			n = parsed
		}
	}
	rows, err := app.DB.Query(context.Background(),
		"SELECT id, tx_date, description, category_name, amount FROM spending WHERE account_id=$1 ORDER BY id DESC LIMIT $2", aid, n)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	res, ok := scanSpendingRows(rows)
	if !ok {
		jsonError(w, http.StatusInternalServerError, "scan failed")
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func botSearchSpending(w http.ResponseWriter, r *http.Request, app *App, aid int64) {
	q := r.URL.Query().Get("q")
	rows, err := app.DB.Query(context.Background(),
		"SELECT id, tx_date, description, category_name, amount FROM spending WHERE account_id=$1 AND description ILIKE $2 ORDER BY id DESC LIMIT 50",
		aid, "%"+q+"%")
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	res, ok := scanSpendingRows(rows)
	if !ok {
		jsonError(w, http.StatusInternalServerError, "scan failed")
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// scanSpendingRows scans (id, tx_date, description, category_name, amount)
// rows into the [{...}] shape bot_recent_spending/bot_search_spending return.
func scanSpendingRows(rows pgx.Rows) ([]map[string]any, bool) {
	out := []map[string]any{}
	for rows.Next() {
		var id, amount int64
		var txDate time.Time
		var description string
		var categoryName *string
		if err := rows.Scan(&id, &txDate, &description, &categoryName, &amount); err != nil {
			return nil, false
		}
		out = append(out, map[string]any{
			"id": id, "tx_date": txDate.Format("2006-01-02"), "description": description,
			"category_name": categoryName, "amount": amount,
		})
	}
	return out, true
}

func botUpdateCategory(w http.ResponseWriter, r *http.Request, app *App, aid int64) {
	sid, ok := pathInt64(r, "id")
	if !ok {
		jsonError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var d struct {
		Category *string `json:"category"`
	}
	if err := decodeJSON(r, &d); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid json")
		return
	}
	ctx := context.Background()
	var catID *int64
	if d.Category != nil {
		var id int64
		if err := app.DB.QueryRow(ctx, "SELECT id FROM categories WHERE name=$1 AND account_id=$2", *d.Category, aid).Scan(&id); err == nil {
			catID = &id
		}
	}
	if _, err := app.DB.Exec(ctx, "UPDATE spending SET category_id=$1, category_name=$2 WHERE id=$3 AND account_id=$4",
		catID, d.Category, sid, aid); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func botUpdateDate(w http.ResponseWriter, r *http.Request, app *App, aid int64) {
	sid, ok := pathInt64(r, "id")
	if !ok {
		jsonError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var d struct {
		TxDate string `json:"tx_date"`
	}
	if err := decodeJSON(r, &d); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if _, err := time.Parse("2006-01-02", d.TxDate); err != nil {
		jsonError(w, http.StatusBadRequest, "tx_date must be YYYY-MM-DD")
		return
	}
	ctx := context.Background()
	var exists int64
	if err := app.DB.QueryRow(ctx, "SELECT id FROM spending WHERE id=$1 AND account_id=$2", sid, aid).Scan(&exists); err != nil {
		jsonError(w, http.StatusNotFound, "not found")
		return
	}
	if _, err := app.DB.Exec(ctx, "UPDATE spending SET tx_date=$1 WHERE id=$2", d.TxDate, sid); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---------- Service-token authenticated internal (bot multiplexers) ----------

// internalListIntegrations mirrors Python's /api/internal/integrations
// response shape exactly: a bare JSON array of
// {account_id, platform, bot_token, platform_token, chat_id} - this is the
// contract bots/telegram_bot.py and bots/discord_bot.py parse directly.
func internalListIntegrations(w http.ResponseWriter, r *http.Request, app *App) {
	q := "SELECT account_id, platform, bot_token, platform_token, chat_id FROM integrations " +
		"WHERE enabled=TRUE AND platform_token IS NOT NULL AND platform_token != ''"
	var args []any
	if platform := r.URL.Query().Get("platform"); platform != "" {
		q += " AND platform=$1"
		args = append(args, platform)
	}
	rows, err := app.DB.Query(context.Background(), q, args...)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var accountID int64
		var platform, botToken string
		var platformToken, chatID *string
		if err := rows.Scan(&accountID, &platform, &botToken, &platformToken, &chatID); err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, map[string]any{
			"account_id": accountID, "platform": platform, "bot_token": botToken,
			"platform_token": platformToken, "chat_id": chatID,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// internalIntegrationsVersion mirrors Python's /api/internal/integrations-version
// response shape exactly: {"version": "<unix ts string or empty>"}.
func internalIntegrationsVersion(w http.ResponseWriter, r *http.Request, app *App) {
	var version string
	err := app.DB.QueryRow(context.Background(), "SELECT value FROM meta WHERE key='integrations_version'").Scan(&version)
	if err != nil {
		version = ""
	}
	writeJSON(w, http.StatusOK, map[string]any{"version": version})
}
