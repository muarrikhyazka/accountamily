package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// RegisterFinanceRoutes wires every personal-finance endpoint a logged-in
// tenant uses: /spending (CRUD + search/recent for bots is in bot.go),
// /budgeting/*, /assets/*, /setup, /categories, /accounts, /income-sources,
// /trend, /report/*, /patterns/*, /years*, /summary, /fx-rates. Ported 1:1
// from app.py's routes of the same names - see that file for the reference
// behavior/JSON shape. Every handler (except /fx-rates, public in Python
// too) starts with requireAccount() and every query is scoped by
// account_id=$N - never trust a path/body id alone.
func RegisterFinanceRoutes(r chi.Router, app *App) {
	r.Get("/fx-rates", app.handleFxRates) // no login_required in Python either

	r.Get("/setup", app.handleGetSetup)

	r.Post("/categories", app.handleAddCategory)
	r.Patch("/categories/{cid}", app.handleUpdateCategory)
	r.Delete("/categories/{cid}", app.handleDeleteCategory)

	r.Post("/accounts", app.handleAddAccount)
	r.Patch("/accounts/{aid}", app.handleUpdateAccount)
	r.Delete("/accounts/{aid}", app.handleDeleteAccount)

	r.Post("/income-sources", app.handleAddIncomeSource)
	r.Patch("/income-sources/{sid}", app.handleUpdateIncomeSource)
	r.Delete("/income-sources/{sid}", app.handleDeleteIncomeSource)

	r.Get("/trend", app.handleTrend)
	r.Get("/years", app.handleYearsOverview)
	r.Get("/years/by-category", app.handleYearsByCategory)
	r.Get("/years/monthly", app.handleYearsMonthly)
	r.Get("/years/day-of-month-avg", app.handleYearsDayOfMonthAvg)
	r.Get("/years/datemonth-avg", app.handleYearsDatemonthAvg)

	r.Get("/report/weekly/{year}/{month}", app.handleWeeklyReport)

	r.Get("/budgeting/{year}", app.handleGetBudgeting)
	r.Post("/budgeting/income", app.handleSetBudgetIncome)
	r.Post("/budgeting/expense", app.handleSetBudgetExpense)

	r.Get("/spending", app.handleListSpending)
	r.Post("/spending", app.handleAddSpending)
	r.Delete("/spending/{sid}", app.handleDeleteSpending)
	r.Patch("/spending/{sid}", app.handleUpdateSpending)

	r.Get("/assets", app.handleGetAssets)
	r.Post("/assets", app.handleAddAsset)
	r.Patch("/assets/{aid}", app.handleUpdateAsset)
	r.Delete("/assets/{aid}", app.handleDeleteAsset)
	r.Post("/assets/target", app.handleSetAssetTarget)

	r.Get("/report/year/{year}", app.handleReportYear)
	r.Get("/report/month-compare/{year}/{month}", app.handleReportMonthCompare)
	r.Get("/report/{year}/{month}", app.handleReport)

	r.Get("/patterns/year/{year}", app.handlePatternsYear)

	r.Get("/summary", app.handleSummary)
}

// ---------- shared helpers ----------

func (app *App) requireAccount(w http.ResponseWriter, r *http.Request) (int64, bool) {
	aid, ok := currentAccountID(r)
	if !ok {
		jsonError(w, http.StatusUnauthorized, "unauthorized")
		return 0, false
	}
	return aid, true
}

func dbError(w http.ResponseWriter, err error) {
	jsonError(w, http.StatusInternalServerError, "db error: "+err.Error())
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

func urlInt64(r *http.Request, name string) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, name), 10, 64)
}

func queryIntDefault(r *http.Request, name string, def int) int {
	v := r.URL.Query().Get(name)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func queryIntPtr(r *http.Request, name string) *int {
	v := r.URL.Query().Get(name)
	if v == "" {
		return nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return nil
	}
	return &n
}

// valueMaxCeiling ports get_value_max(): value_max=10jt|100jt caps results.
func valueMaxCeiling(r *http.Request) *int64 {
	switch r.URL.Query().Get("value_max") {
	case "10jt":
		c := int64(10_000_000)
		return &c
	case "100jt":
		c := int64(100_000_000)
		return &c
	}
	return nil
}

// appendCeiling appends " AND <alias> <= $N" to query/args when a value_max
// filter is active - ports value_filter(alias).
func appendCeiling(query string, args []any, r *http.Request, alias string) (string, []any) {
	ceiling := valueMaxCeiling(r)
	if ceiling == nil {
		return query, args
	}
	args = append(args, *ceiling)
	return query + fmt.Sprintf(" AND %s <= $%d", alias, len(args)), args
}

func asInt64(v any) (int64, bool) {
	switch t := v.(type) {
	case float64:
		return int64(t), true
	case int64:
		return t, true
	case int:
		return int64(t), true
	case json.Number:
		n, err := t.Int64()
		return n, err == nil
	}
	return 0, false
}

// ---------- row types (mirror sqlite dict(r) shapes) ----------

type accountRow struct {
	ID        int64  `json:"id"`
	AccountID int64  `json:"account_id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
}

type categoryRow struct {
	ID           int64  `json:"id"`
	AccountID    int64  `json:"account_id"`
	Name         string `json:"name"`
	MainCategory string `json:"main_category"`
}

type incomeSourceRow struct {
	ID        int64  `json:"id"`
	AccountID int64  `json:"account_id"`
	Name      string `json:"name"`
}

// ---------- fx rates (12h file cache, no new deps) ----------

const fxCacheTTL = 12 * time.Hour
const fxAPIURL = "https://open.er-api.com/v6/latest/IDR"

type fxCache struct {
	Base      string             `json:"base"`
	Rates     map[string]float64 `json:"rates"`
	FetchedAt *float64           `json:"fetched_at"`
}

func fxCachePath() string {
	return getenv("FX_CACHE_PATH", "/app/data/fx_rates.json")
}

func loadFxCacheFile(path string) *fxCache {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var c fxCache
	if err := json.Unmarshal(b, &c); err != nil {
		return nil
	}
	return &c
}

func saveFxCacheFile(path string, c *fxCache) {
	_ = os.MkdirAll(dirOf(path), 0o755)
	b, err := json.Marshal(c)
	if err != nil {
		return
	}
	_ = os.WriteFile(path, b, 0o644)
}

func dirOf(path string) string {
	i := strings.LastIndex(path, "/")
	if i < 0 {
		return "."
	}
	return path[:i]
}

// getFxRates ports get_fx_rates(): file-cached rates, refreshed at most once
// per 12h via stdlib net/http, stale cache (or nil) beats a hard failure.
func getFxRates() *fxCache {
	path := fxCachePath()
	cached := loadFxCacheFile(path)
	now := float64(time.Now().Unix())
	if cached != nil && cached.FetchedAt != nil && now-*cached.FetchedAt < fxCacheTTL.Seconds() {
		return cached
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(fxAPIURL)
	if err != nil {
		return cached
	}
	defer resp.Body.Close()
	var data struct {
		Rates map[string]float64 `json:"rates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return cached
	}
	fetchedAt := now
	fresh := &fxCache{Base: "IDR", Rates: data.Rates, FetchedAt: &fetchedAt}
	saveFxCacheFile(path, fresh)
	return fresh
}

func (app *App) handleFxRates(w http.ResponseWriter, r *http.Request) {
	cached := getFxRates()
	if cached == nil {
		writeJSON(w, http.StatusOK, map[string]any{"base": "IDR", "rates": map[string]float64{"IDR": 1}, "fetched_at": nil})
		return
	}
	writeJSON(w, http.StatusOK, cached)
}

// ---------- setup / master lists ----------

func (app *App) handleGetSetup(w http.ResponseWriter, r *http.Request) {
	aid, ok := app.requireAccount(w, r)
	if !ok {
		return
	}
	ctx := context.Background()

	accounts := []accountRow{}
	rows, err := app.DB.Query(ctx, "SELECT id, account_id, name, kind FROM accounts WHERE account_id=$1 ORDER BY id", aid)
	if err != nil {
		dbError(w, err)
		return
	}
	for rows.Next() {
		var a accountRow
		if err := rows.Scan(&a.ID, &a.AccountID, &a.Name, &a.Kind); err != nil {
			rows.Close()
			dbError(w, err)
			return
		}
		accounts = append(accounts, a)
	}
	rows.Close()

	categories := []categoryRow{}
	rows, err = app.DB.Query(ctx, "SELECT id, account_id, name, main_category FROM categories WHERE account_id=$1 ORDER BY id", aid)
	if err != nil {
		dbError(w, err)
		return
	}
	for rows.Next() {
		var c categoryRow
		if err := rows.Scan(&c.ID, &c.AccountID, &c.Name, &c.MainCategory); err != nil {
			rows.Close()
			dbError(w, err)
			return
		}
		categories = append(categories, c)
	}
	rows.Close()

	incomeSources := []incomeSourceRow{}
	rows, err = app.DB.Query(ctx, "SELECT id, account_id, name FROM income_sources WHERE account_id=$1 ORDER BY id", aid)
	if err != nil {
		dbError(w, err)
		return
	}
	for rows.Next() {
		var s incomeSourceRow
		if err := rows.Scan(&s.ID, &s.AccountID, &s.Name); err != nil {
			rows.Close()
			dbError(w, err)
			return
		}
		incomeSources = append(incomeSources, s)
	}
	rows.Close()

	writeJSON(w, http.StatusOK, map[string]any{"accounts": accounts, "categories": categories, "income_sources": incomeSources})
}

// ---------- categories CRUD ----------

func (app *App) handleAddCategory(w http.ResponseWriter, r *http.Request) {
	aid, ok := app.requireAccount(w, r)
	if !ok {
		return
	}
	var d struct {
		Name         string `json:"name"`
		MainCategory string `json:"main_category"`
	}
	_ = decodeJSON(r, &d)
	name := strings.TrimSpace(d.Name)
	if name == "" || (d.MainCategory != "Needs" && d.MainCategory != "Wants" && d.MainCategory != "Savings") {
		jsonError(w, http.StatusBadRequest, "name and valid main_category required")
		return
	}
	var newID int64
	err := app.DB.QueryRow(context.Background(),
		"INSERT INTO categories (account_id, name, main_category) VALUES ($1,$2,$3) RETURNING id",
		aid, name, d.MainCategory).Scan(&newID)
	if err != nil {
		if isUniqueViolation(err) {
			jsonError(w, http.StatusConflict, "category already exists")
			return
		}
		dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": newID})
}

func (app *App) handleUpdateCategory(w http.ResponseWriter, r *http.Request) {
	aid, ok := app.requireAccount(w, r)
	if !ok {
		return
	}
	cid, err := urlInt64(r, "cid")
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var d map[string]any
	_ = decodeJSON(r, &d)
	fields := []string{}
	args := []any{}
	if v, _ := d["name"].(string); strings.TrimSpace(v) != "" {
		args = append(args, strings.TrimSpace(v))
		fields = append(fields, fmt.Sprintf("name=$%d", len(args)))
	}
	if v, _ := d["main_category"].(string); v == "Needs" || v == "Wants" || v == "Savings" {
		args = append(args, v)
		fields = append(fields, fmt.Sprintf("main_category=$%d", len(args)))
	}
	if len(fields) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	args = append(args, cid, aid)
	query := fmt.Sprintf("UPDATE categories SET %s WHERE id=$%d AND account_id=$%d", strings.Join(fields, ","), len(args)-1, len(args))
	if _, err := app.DB.Exec(context.Background(), query, args...); err != nil {
		if isUniqueViolation(err) {
			jsonError(w, http.StatusConflict, "name already used")
			return
		}
		dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (app *App) handleDeleteCategory(w http.ResponseWriter, r *http.Request) {
	aid, ok := app.requireAccount(w, r)
	if !ok {
		return
	}
	cid, err := urlInt64(r, "cid")
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid id")
		return
	}
	ctx := context.Background()
	var spendingCount, budgetCount int64
	if err := app.DB.QueryRow(ctx, "SELECT COUNT(*) FROM spending WHERE category_id=$1 AND account_id=$2", cid, aid).Scan(&spendingCount); err != nil {
		dbError(w, err)
		return
	}
	if err := app.DB.QueryRow(ctx, "SELECT COUNT(*) FROM budget_expense WHERE category_id=$1 AND account_id=$2", cid, aid).Scan(&budgetCount); err != nil {
		dbError(w, err)
		return
	}
	force := r.URL.Query().Get("force") == "1"
	if (spendingCount > 0 || budgetCount > 0) && !force {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "category in use", "spending_count": spendingCount, "budget_count": budgetCount})
		return
	}
	if _, err := app.DB.Exec(ctx, "UPDATE spending SET category_id=NULL WHERE category_id=$1 AND account_id=$2", cid, aid); err != nil {
		dbError(w, err)
		return
	}
	if _, err := app.DB.Exec(ctx, "DELETE FROM budget_expense WHERE category_id=$1 AND account_id=$2", cid, aid); err != nil {
		dbError(w, err)
		return
	}
	if _, err := app.DB.Exec(ctx, "DELETE FROM categories WHERE id=$1 AND account_id=$2", cid, aid); err != nil {
		dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---------- accounts CRUD (bank + asset accounts) ----------

func (app *App) handleAddAccount(w http.ResponseWriter, r *http.Request) {
	aid, ok := app.requireAccount(w, r)
	if !ok {
		return
	}
	var d struct {
		Name string `json:"name"`
		Kind string `json:"kind"`
	}
	_ = decodeJSON(r, &d)
	if d.Kind == "" {
		d.Kind = "bank"
	}
	name := strings.TrimSpace(d.Name)
	if name == "" || (d.Kind != "bank" && d.Kind != "asset") {
		jsonError(w, http.StatusBadRequest, "name and valid kind required")
		return
	}
	var newID int64
	err := app.DB.QueryRow(context.Background(),
		"INSERT INTO accounts (account_id, name, kind) VALUES ($1,$2,$3) RETURNING id", aid, name, d.Kind).Scan(&newID)
	if err != nil {
		if isUniqueViolation(err) {
			jsonError(w, http.StatusConflict, "account already exists")
			return
		}
		dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": newID})
}

func (app *App) handleUpdateAccount(w http.ResponseWriter, r *http.Request) {
	aid, ok := app.requireAccount(w, r)
	if !ok {
		return
	}
	accID, err := urlInt64(r, "aid")
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var d struct {
		Name string `json:"name"`
	}
	_ = decodeJSON(r, &d)
	if strings.TrimSpace(d.Name) == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	_, err = app.DB.Exec(context.Background(), "UPDATE accounts SET name=$1 WHERE id=$2 AND account_id=$3", strings.TrimSpace(d.Name), accID, aid)
	if err != nil {
		if isUniqueViolation(err) {
			jsonError(w, http.StatusConflict, "name already used")
			return
		}
		dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (app *App) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	aid, ok := app.requireAccount(w, r)
	if !ok {
		return
	}
	accID, err := urlInt64(r, "aid")
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid id")
		return
	}
	ctx := context.Background()
	var used int64
	if err := app.DB.QueryRow(ctx, "SELECT COUNT(*) FROM assets WHERE account_ref=$1 AND account_id=$2", accID, aid).Scan(&used); err != nil {
		dbError(w, err)
		return
	}
	if used > 0 {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "account in use", "count": used})
		return
	}
	if _, err := app.DB.Exec(ctx, "DELETE FROM accounts WHERE id=$1 AND account_id=$2", accID, aid); err != nil {
		dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---------- income sources CRUD ----------

func (app *App) handleAddIncomeSource(w http.ResponseWriter, r *http.Request) {
	aid, ok := app.requireAccount(w, r)
	if !ok {
		return
	}
	var d struct {
		Name string `json:"name"`
	}
	_ = decodeJSON(r, &d)
	name := strings.TrimSpace(d.Name)
	if name == "" {
		jsonError(w, http.StatusBadRequest, "name required")
		return
	}
	var newID int64
	err := app.DB.QueryRow(context.Background(),
		"INSERT INTO income_sources (account_id, name) VALUES ($1,$2) RETURNING id", aid, name).Scan(&newID)
	if err != nil {
		if isUniqueViolation(err) {
			jsonError(w, http.StatusConflict, "income source already exists")
			return
		}
		dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": newID})
}

func (app *App) handleUpdateIncomeSource(w http.ResponseWriter, r *http.Request) {
	aid, ok := app.requireAccount(w, r)
	if !ok {
		return
	}
	sid, err := urlInt64(r, "sid")
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var d struct {
		Name string `json:"name"`
	}
	_ = decodeJSON(r, &d)
	if strings.TrimSpace(d.Name) == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	_, err = app.DB.Exec(context.Background(), "UPDATE income_sources SET name=$1 WHERE id=$2 AND account_id=$3", strings.TrimSpace(d.Name), sid, aid)
	if err != nil {
		if isUniqueViolation(err) {
			jsonError(w, http.StatusConflict, "name already used")
			return
		}
		dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (app *App) handleDeleteIncomeSource(w http.ResponseWriter, r *http.Request) {
	aid, ok := app.requireAccount(w, r)
	if !ok {
		return
	}
	sid, err := urlInt64(r, "sid")
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid id")
		return
	}
	ctx := context.Background()
	var used int64
	if err := app.DB.QueryRow(ctx, "SELECT COUNT(*) FROM budget_income WHERE source_id=$1 AND account_id=$2", sid, aid).Scan(&used); err != nil {
		dbError(w, err)
		return
	}
	force := r.URL.Query().Get("force") == "1"
	if used > 0 && !force {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "income source in use", "count": used})
		return
	}
	if _, err := app.DB.Exec(ctx, "DELETE FROM budget_income WHERE source_id=$1 AND account_id=$2", sid, aid); err != nil {
		dbError(w, err)
		return
	}
	if _, err := app.DB.Exec(ctx, "DELETE FROM income_sources WHERE id=$1 AND account_id=$2", sid, aid); err != nil {
		dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---------- trend ----------

func (app *App) handleTrend(w http.ResponseWriter, r *http.Request) {
	aid, ok := app.requireAccount(w, r)
	if !ok {
		return
	}
	now := time.Now()
	year := queryIntDefault(r, "year", now.Year())
	month := queryIntDefault(r, "month", int(now.Month()))
	count := queryIntDefault(r, "count", 6)
	ctx := context.Background()

	type trendRow struct {
		Year    int   `json:"year"`
		Month   int   `json:"month"`
		Income  int64 `json:"income"`
		Expense int64 `json:"expense"`
	}
	out := []trendRow{}
	y, m := year, month
	for i := 0; i < count; i++ {
		var income int64
		if err := app.DB.QueryRow(ctx, "SELECT COALESCE(SUM(amount),0) FROM budget_income WHERE account_id=$1 AND year=$2 AND month=$3", aid, y, m).Scan(&income); err != nil {
			dbError(w, err)
			return
		}
		q := "SELECT COALESCE(SUM(amount),0) FROM spending WHERE account_id=$1 AND EXTRACT(YEAR FROM tx_date)::int=$2 AND EXTRACT(MONTH FROM tx_date)::int=$3"
		args := []any{aid, y, m}
		q, args = appendCeiling(q, args, r, "amount")
		var expense int64
		if err := app.DB.QueryRow(ctx, q, args...).Scan(&expense); err != nil {
			dbError(w, err)
			return
		}
		out = append(out, trendRow{Year: y, Month: m, Income: income, Expense: expense})
		m--
		if m == 0 {
			m = 12
			y--
		}
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	writeJSON(w, http.StatusOK, out)
}

func (app *App) handleYearsOverview(w http.ResponseWriter, r *http.Request) {
	aid, ok := app.requireAccount(w, r)
	if !ok {
		return
	}
	ctx := context.Background()
	rows, err := app.DB.Query(ctx,
		"SELECT DISTINCT EXTRACT(YEAR FROM tx_date)::int AS y FROM spending WHERE account_id=$1 "+
			"UNION SELECT DISTINCT year FROM budget_income WHERE account_id=$1 ORDER BY y", aid)
	if err != nil {
		dbError(w, err)
		return
	}
	years := []int{}
	for rows.Next() {
		var y int
		if err := rows.Scan(&y); err != nil {
			rows.Close()
			dbError(w, err)
			return
		}
		years = append(years, y)
	}
	rows.Close()

	type yearRow struct {
		Year    int   `json:"year"`
		Income  int64 `json:"income"`
		Expense int64 `json:"expense"`
	}
	out := []yearRow{}
	for _, y := range years {
		var income int64
		if err := app.DB.QueryRow(ctx, "SELECT COALESCE(SUM(amount),0) FROM budget_income WHERE account_id=$1 AND year=$2", aid, y).Scan(&income); err != nil {
			dbError(w, err)
			return
		}
		q := "SELECT COALESCE(SUM(amount),0) FROM spending WHERE account_id=$1 AND EXTRACT(YEAR FROM tx_date)::int=$2"
		args := []any{aid, y}
		q, args = appendCeiling(q, args, r, "amount")
		var expense int64
		if err := app.DB.QueryRow(ctx, q, args...).Scan(&expense); err != nil {
			dbError(w, err)
			return
		}
		out = append(out, yearRow{Year: y, Income: income, Expense: expense})
	}
	writeJSON(w, http.StatusOK, out)
}

func (app *App) handleYearsByCategory(w http.ResponseWriter, r *http.Request) {
	aid, ok := app.requireAccount(w, r)
	if !ok {
		return
	}
	ctx := context.Background()
	q := `SELECT EXTRACT(YEAR FROM sp.tx_date)::int as year,
	             COALESCE(c.name, sp.category_name, 'Other') as category_name,
	             SUM(sp.amount) as total
	      FROM spending sp LEFT JOIN categories c ON c.id=sp.category_id
	      WHERE sp.account_id=$1`
	args := []any{aid}
	q, args = appendCeiling(q, args, r, "sp.amount")
	q += " GROUP BY year, category_name"
	rows, err := app.DB.Query(ctx, q, args...)
	if err != nil {
		dbError(w, err)
		return
	}
	type agg struct {
		year  int
		cat   string
		total int64
	}
	all := []agg{}
	yearSet := map[int]bool{}
	for rows.Next() {
		var a agg
		if err := rows.Scan(&a.year, &a.cat, &a.total); err != nil {
			rows.Close()
			dbError(w, err)
			return
		}
		all = append(all, a)
		yearSet[a.year] = true
	}
	rows.Close()

	years := []int{}
	for y := range yearSet {
		years = append(years, y)
	}
	sort.Ints(years)

	cats := map[string]map[int]int64{}
	sums := map[string]int64{}
	for _, a := range all {
		if cats[a.cat] == nil {
			cats[a.cat] = map[int]int64{}
		}
		cats[a.cat][a.year] = a.total
		sums[a.cat] += a.total
	}
	names := []string{}
	for name := range cats {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool { return sums[names[i]] > sums[names[j]] })

	outRows := []map[string]any{}
	for _, name := range names {
		row := map[string]any{"category_name": name}
		for _, y := range years {
			row[strconv.Itoa(y)] = cats[name][y]
		}
		outRows = append(outRows, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{"years": years, "rows": outRows})
}

func (app *App) handleYearsMonthly(w http.ResponseWriter, r *http.Request) {
	aid, ok := app.requireAccount(w, r)
	if !ok {
		return
	}
	ctx := context.Background()
	q := `SELECT EXTRACT(YEAR FROM tx_date)::int as year, EXTRACT(MONTH FROM tx_date)::int as month, SUM(amount) as total
	      FROM spending WHERE account_id=$1`
	args := []any{aid}
	q, args = appendCeiling(q, args, r, "amount")
	q += " GROUP BY year, month"
	rows, err := app.DB.Query(ctx, q, args...)
	if err != nil {
		dbError(w, err)
		return
	}
	byYear := map[int][12]int64{}
	years := []int{}
	yearSeen := map[int]bool{}
	for rows.Next() {
		var y, m int
		var total int64
		if err := rows.Scan(&y, &m, &total); err != nil {
			rows.Close()
			dbError(w, err)
			return
		}
		if !yearSeen[y] {
			yearSeen[y] = true
			years = append(years, y)
		}
		arr := byYear[y]
		arr[m-1] = total
		byYear[y] = arr
	}
	rows.Close()
	sort.Ints(years)
	outByYear := map[string][]int64{}
	for _, y := range years {
		arr := byYear[y]
		outByYear[strconv.Itoa(y)] = arr[:]
	}
	writeJSON(w, http.StatusOK, map[string]any{"years": years, "by_year": outByYear})
}

type txDateAmount struct {
	Date time.Time
	Amt  int64
}

func (app *App) fetchSpendingDates(ctx context.Context, aid int64, extraWhere string, extraArgs []any) ([]txDateAmount, error) {
	q := "SELECT tx_date, amount FROM spending WHERE account_id=$1" + extraWhere
	args := append([]any{aid}, extraArgs...)
	rows, err := app.DB.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []txDateAmount{}
	for rows.Next() {
		var d time.Time
		var amt int64
		if err := rows.Scan(&d, &amt); err != nil {
			return nil, err
		}
		out = append(out, txDateAmount{Date: d, Amt: amt})
	}
	return out, rows.Err()
}

func (app *App) handleYearsDayOfMonthAvg(w http.ResponseWriter, r *http.Request) {
	aid, ok := app.requireAccount(w, r)
	if !ok {
		return
	}
	rows, err := app.fetchSpendingDates(context.Background(), aid, "", nil)
	if err != nil {
		dbError(w, err)
		return
	}
	ceiling := valueMaxCeiling(r)
	byDay := map[int]int64{}
	monthsWithDay := map[int]map[[2]int]bool{}
	for d := 1; d <= 31; d++ {
		monthsWithDay[d] = map[[2]int]bool{}
	}
	for _, row := range rows {
		if ceiling != nil && row.Amt > *ceiling {
			continue
		}
		day := row.Date.Day()
		byDay[day] += row.Amt
		monthsWithDay[day][[2]int{row.Date.Year(), int(row.Date.Month())}] = true
	}
	type outRow struct {
		Day int     `json:"day"`
		Avg float64 `json:"avg"`
	}
	out := []outRow{}
	for d := 1; d <= 31; d++ {
		if len(monthsWithDay[d]) == 0 {
			continue
		}
		out = append(out, outRow{Day: d, Avg: float64(byDay[d]) / float64(len(monthsWithDay[d]))})
	}
	writeJSON(w, http.StatusOK, out)
}

var monthAbbr = []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}

func (app *App) handleYearsDatemonthAvg(w http.ResponseWriter, r *http.Request) {
	aid, ok := app.requireAccount(w, r)
	if !ok {
		return
	}
	rows, err := app.fetchSpendingDates(context.Background(), aid, "", nil)
	if err != nil {
		dbError(w, err)
		return
	}
	ceiling := valueMaxCeiling(r)
	type md struct{ month, day int }
	byMD := map[md]int64{}
	yearsWithMD := map[md]map[int]bool{}
	for _, row := range rows {
		if ceiling != nil && row.Amt > *ceiling {
			continue
		}
		key := md{int(row.Date.Month()), row.Date.Day()}
		byMD[key] += row.Amt
		if yearsWithMD[key] == nil {
			yearsWithMD[key] = map[int]bool{}
		}
		yearsWithMD[key][row.Date.Year()] = true
	}
	type outRow struct {
		Month int     `json:"month"`
		Day   int     `json:"day"`
		Label string  `json:"label"`
		Avg   float64 `json:"avg"`
	}
	out := []outRow{}
	for month := 1; month <= 12; month++ {
		daysInMonth := 31
		if month == 2 {
			daysInMonth = 29
		} else if month == 4 || month == 6 || month == 9 || month == 11 {
			daysInMonth = 30
		}
		for day := 1; day <= daysInMonth; day++ {
			key := md{month, day}
			if years, ok := yearsWithMD[key]; ok {
				out = append(out, outRow{
					Month: month, Day: day,
					Label: fmt.Sprintf("%d %s", day, monthAbbr[month-1]),
					Avg:   float64(byMD[key]) / float64(len(years)),
				})
			}
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (app *App) handleWeeklyReport(w http.ResponseWriter, r *http.Request) {
	aid, ok := app.requireAccount(w, r)
	if !ok {
		return
	}
	year, err1 := urlInt64(r, "year")
	month, err2 := urlInt64(r, "month")
	if err1 != nil || err2 != nil {
		jsonError(w, http.StatusBadRequest, "invalid year/month")
		return
	}
	q := `SELECT tx_date, amount, COALESCE(c.name, sp.category_name, 'Other') as category_name
	      FROM spending sp LEFT JOIN categories c ON c.id=sp.category_id
	      WHERE sp.account_id=$1 AND EXTRACT(YEAR FROM tx_date)::int=$2 AND EXTRACT(MONTH FROM tx_date)::int=$3`
	args := []any{aid, year, month}
	q, args = appendCeiling(q, args, r, "sp.amount")
	rows, err := app.DB.Query(context.Background(), q, args...)
	if err != nil {
		dbError(w, err)
		return
	}
	type week struct {
		Week       int             `json:"week"`
		Total      int64           `json:"total"`
		ByCategory map[string]int64
	}
	weeks := map[int]*week{}
	order := []int{}
	for rows.Next() {
		var d time.Time
		var amt int64
		var catName string
		if err := rows.Scan(&d, &amt, &catName); err != nil {
			rows.Close()
			dbError(w, err)
			return
		}
		wk := (d.Day()-1)/7 + 1
		wrow, ok := weeks[wk]
		if !ok {
			wrow = &week{Week: wk, ByCategory: map[string]int64{}}
			weeks[wk] = wrow
			order = append(order, wk)
		}
		wrow.Total += amt
		wrow.ByCategory[catName] += amt
	}
	rows.Close()
	sort.Ints(order)
	type catTotal struct {
		CategoryName string `json:"category_name"`
		Total        int64  `json:"total"`
	}
	type outWeek struct {
		Week       int        `json:"week"`
		Total      int64      `json:"total"`
		ByCategory []catTotal `json:"by_category"`
	}
	out := []outWeek{}
	for _, wk := range order {
		wrow := weeks[wk]
		cats := []catTotal{}
		for k, v := range wrow.ByCategory {
			cats = append(cats, catTotal{k, v})
		}
		sort.Slice(cats, func(i, j int) bool { return cats[i].Total > cats[j].Total })
		out = append(out, outWeek{Week: wrow.Week, Total: wrow.Total, ByCategory: cats})
	}
	writeJSON(w, http.StatusOK, out)
}

// ---------- budgeting ----------

func (app *App) handleGetBudgeting(w http.ResponseWriter, r *http.Request) {
	aid, ok := app.requireAccount(w, r)
	if !ok {
		return
	}
	year, err := urlInt64(r, "year")
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid year")
		return
	}
	ctx := context.Background()

	type incomeRow struct {
		ID         int64  `json:"id"`
		AccountID  int64  `json:"account_id"`
		Year       int64  `json:"year"`
		Month      int64  `json:"month"`
		SourceID   int64  `json:"source_id"`
		Amount     int64  `json:"amount"`
		SourceName string `json:"source_name"`
	}
	income := []incomeRow{}
	rows, err := app.DB.Query(ctx,
		"SELECT bi.id, bi.account_id, bi.year, bi.month, bi.source_id, bi.amount, s.name FROM budget_income bi "+
			"JOIN income_sources s ON s.id = bi.source_id WHERE bi.account_id=$1 AND bi.year=$2", aid, year)
	if err != nil {
		dbError(w, err)
		return
	}
	for rows.Next() {
		var ir incomeRow
		if err := rows.Scan(&ir.ID, &ir.AccountID, &ir.Year, &ir.Month, &ir.SourceID, &ir.Amount, &ir.SourceName); err != nil {
			rows.Close()
			dbError(w, err)
			return
		}
		income = append(income, ir)
	}
	rows.Close()

	type expenseRow struct {
		ID           int64  `json:"id"`
		AccountID    int64  `json:"account_id"`
		Year         int64  `json:"year"`
		Month        int64  `json:"month"`
		CategoryID   int64  `json:"category_id"`
		Amount       int64  `json:"amount"`
		CategoryName string `json:"category_name"`
		MainCategory string `json:"main_category"`
	}
	expense := []expenseRow{}
	rows, err = app.DB.Query(ctx,
		"SELECT be.id, be.account_id, be.year, be.month, be.category_id, be.amount, c.name, c.main_category FROM budget_expense be "+
			"JOIN categories c ON c.id = be.category_id WHERE be.account_id=$1 AND be.year=$2", aid, year)
	if err != nil {
		dbError(w, err)
		return
	}
	for rows.Next() {
		var er expenseRow
		if err := rows.Scan(&er.ID, &er.AccountID, &er.Year, &er.Month, &er.CategoryID, &er.Amount, &er.CategoryName, &er.MainCategory); err != nil {
			rows.Close()
			dbError(w, err)
			return
		}
		expense = append(expense, er)
	}
	rows.Close()

	type savingsRow struct {
		ID        int64  `json:"id"`
		AccountID int64  `json:"account_id"`
		Year      int64  `json:"year"`
		Month     int64  `json:"month"`
		Name      string `json:"name"`
		Amount    int64  `json:"amount"`
	}
	savings := []savingsRow{}
	rows, err = app.DB.Query(ctx, "SELECT id, account_id, year, month, name, amount FROM savings_list WHERE account_id=$1 AND year=$2", aid, year)
	if err != nil {
		dbError(w, err)
		return
	}
	for rows.Next() {
		var sr savingsRow
		if err := rows.Scan(&sr.ID, &sr.AccountID, &sr.Year, &sr.Month, &sr.Name, &sr.Amount); err != nil {
			rows.Close()
			dbError(w, err)
			return
		}
		savings = append(savings, sr)
	}
	rows.Close()

	incomeHistory := map[int64]map[int64]int64{}
	rows, err = app.DB.Query(ctx, "SELECT source_id, year, SUM(amount) FROM budget_income WHERE account_id=$1 AND year<$2 GROUP BY source_id, year", aid, year)
	if err != nil {
		dbError(w, err)
		return
	}
	for rows.Next() {
		var sourceID, y, total int64
		if err := rows.Scan(&sourceID, &y, &total); err != nil {
			rows.Close()
			dbError(w, err)
			return
		}
		if incomeHistory[sourceID] == nil {
			incomeHistory[sourceID] = map[int64]int64{}
		}
		incomeHistory[sourceID][y] = total
	}
	rows.Close()

	expenseHistory := map[int64]map[int64]int64{}
	rows, err = app.DB.Query(ctx, "SELECT category_id, year, SUM(amount) FROM budget_expense WHERE account_id=$1 AND year<$2 GROUP BY category_id, year", aid, year)
	if err != nil {
		dbError(w, err)
		return
	}
	for rows.Next() {
		var catID, y, total int64
		if err := rows.Scan(&catID, &y, &total); err != nil {
			rows.Close()
			dbError(w, err)
			return
		}
		if expenseHistory[catID] == nil {
			expenseHistory[catID] = map[int64]int64{}
		}
		expenseHistory[catID][y] = total
	}
	rows.Close()

	writeJSON(w, http.StatusOK, map[string]any{
		"income": income, "expense": expense, "savings": savings,
		"income_history": incomeHistory, "expense_history": expenseHistory,
	})
}

func (app *App) handleSetBudgetIncome(w http.ResponseWriter, r *http.Request) {
	aid, ok := app.requireAccount(w, r)
	if !ok {
		return
	}
	var d struct {
		Year     int64 `json:"year"`
		Month    int64 `json:"month"`
		SourceID int64 `json:"source_id"`
		Amount   int64 `json:"amount"`
	}
	_ = decodeJSON(r, &d)
	_, err := app.DB.Exec(context.Background(),
		"INSERT INTO budget_income (account_id, year, month, source_id, amount) VALUES ($1,$2,$3,$4,$5) "+
			"ON CONFLICT (account_id, year, month, source_id) DO UPDATE SET amount=excluded.amount",
		aid, d.Year, d.Month, d.SourceID, d.Amount)
	if err != nil {
		dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (app *App) handleSetBudgetExpense(w http.ResponseWriter, r *http.Request) {
	aid, ok := app.requireAccount(w, r)
	if !ok {
		return
	}
	var d struct {
		Year       int64 `json:"year"`
		Month      int64 `json:"month"`
		CategoryID int64 `json:"category_id"`
		Amount     int64 `json:"amount"`
	}
	_ = decodeJSON(r, &d)
	_, err := app.DB.Exec(context.Background(),
		"INSERT INTO budget_expense (account_id, year, month, category_id, amount) VALUES ($1,$2,$3,$4,$5) "+
			"ON CONFLICT (account_id, year, month, category_id) DO UPDATE SET amount=excluded.amount",
		aid, d.Year, d.Month, d.CategoryID, d.Amount)
	if err != nil {
		dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---------- spending ----------

type spendingRow struct {
	ID             int64   `json:"id"`
	AccountID      int64   `json:"account_id"`
	TxDate         string  `json:"tx_date"`
	Description    string  `json:"description"`
	CategoryID     *int64  `json:"category_id"`
	CategoryName   *string `json:"category_name"`
	Amount         int64   `json:"amount"`
	Checked        bool    `json:"checked"`
	Source         string  `json:"source"`
	ClientID       *string `json:"client_id"`
	CreatedAt      string  `json:"created_at"`
	CategoryMaster *string `json:"category_master"`
}

func (app *App) handleListSpending(w http.ResponseWriter, r *http.Request) {
	aid, ok := app.requireAccount(w, r)
	if !ok {
		return
	}
	year := queryIntPtr(r, "year")
	month := queryIntPtr(r, "month")
	page := queryIntDefault(r, "page", 1)
	if page < 1 {
		page = 1
	}
	perPage := queryIntDefault(r, "per_page", 20)
	if perPage > 500 {
		perPage = 500
	}
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	category := strings.TrimSpace(r.URL.Query().Get("category"))
	sortParam := r.URL.Query().Get("sort")
	if sortParam == "" {
		sortParam = "tx_date"
	}
	order := strings.ToLower(r.URL.Query().Get("order"))

	base := "FROM spending sp LEFT JOIN categories c ON c.id = sp.category_id WHERE sp.account_id=$1"
	args := []any{aid}
	if year != nil {
		args = append(args, *year)
		base += fmt.Sprintf(" AND EXTRACT(YEAR FROM sp.tx_date)::int = $%d", len(args))
	}
	if month != nil {
		args = append(args, *month)
		base += fmt.Sprintf(" AND EXTRACT(MONTH FROM sp.tx_date)::int = $%d", len(args))
	}
	if search != "" {
		args = append(args, "%"+search+"%")
		base += fmt.Sprintf(" AND sp.description ILIKE $%d", len(args))
	}
	if category != "" {
		args = append(args, category)
		base += fmt.Sprintf(" AND sp.category_name = $%d", len(args))
	}
	base, args = appendCeiling(base, args, r, "sp.amount")

	sortCols := map[string]string{"tx_date": "sp.tx_date", "description": "sp.description", "category_name": "sp.category_name", "amount": "sp.amount"}
	sortCol, ok2 := sortCols[sortParam]
	if !ok2 {
		sortCol = "sp.tx_date"
	}
	orderDir := "DESC"
	if order == "asc" {
		orderDir = "ASC"
	}

	ctx := context.Background()
	var total int64
	if err := app.DB.QueryRow(ctx, "SELECT COUNT(*) "+base, args...).Scan(&total); err != nil {
		dbError(w, err)
		return
	}

	q := "SELECT sp.id, sp.account_id, sp.tx_date::text, sp.description, sp.category_id, sp.category_name, " +
		"sp.amount, sp.checked, sp.source, sp.client_id, sp.created_at::text, c.name " + base +
		fmt.Sprintf(" ORDER BY %s %s, sp.id DESC LIMIT $%d OFFSET $%d", sortCol, orderDir, len(args)+1, len(args)+2)
	listArgs := append(append([]any{}, args...), perPage, (page-1)*perPage)
	rows, err := app.DB.Query(ctx, q, listArgs...)
	if err != nil {
		dbError(w, err)
		return
	}
	out := []spendingRow{}
	for rows.Next() {
		var s spendingRow
		if err := rows.Scan(&s.ID, &s.AccountID, &s.TxDate, &s.Description, &s.CategoryID, &s.CategoryName,
			&s.Amount, &s.Checked, &s.Source, &s.ClientID, &s.CreatedAt, &s.CategoryMaster); err != nil {
			rows.Close()
			dbError(w, err)
			return
		}
		out = append(out, s)
	}
	rows.Close()
	writeJSON(w, http.StatusOK, map[string]any{"rows": out, "total": total, "page": page, "per_page": perPage})
}

func (app *App) handleAddSpending(w http.ResponseWriter, r *http.Request) {
	aid, ok := app.requireAccount(w, r)
	if !ok {
		return
	}
	var d struct {
		TxDate      string  `json:"tx_date"`
		Description string  `json:"description"`
		Category    string  `json:"category"`
		Amount      int64   `json:"amount"`
		Source      string  `json:"source"`
		ClientID    *string `json:"client_id"`
	}
	_ = decodeJSON(r, &d)
	if d.Source == "" {
		d.Source = "web"
	}
	ctx := context.Background()

	var catID *int64
	if d.Category != "" {
		var id int64
		if err := app.DB.QueryRow(ctx, "SELECT id FROM categories WHERE name=$1 AND account_id=$2", d.Category, aid).Scan(&id); err == nil {
			catID = &id
		}
	}

	if d.ClientID != nil && *d.ClientID != "" {
		var existing int64
		if err := app.DB.QueryRow(ctx, "SELECT id FROM spending WHERE client_id=$1 AND account_id=$2", *d.ClientID, aid).Scan(&existing); err == nil {
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": existing, "duplicate": true})
			return
		}
	}

	var categoryName *string
	if d.Category != "" {
		categoryName = &d.Category
	}
	var newID int64
	err := app.DB.QueryRow(ctx,
		"INSERT INTO spending (account_id, tx_date, description, category_id, category_name, amount, source, client_id) "+
			"VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id",
		aid, d.TxDate, d.Description, catID, categoryName, d.Amount, d.Source, d.ClientID).Scan(&newID)
	if err != nil {
		dbError(w, err)
		return
	}
	app.awardPointsForLog(ctx, aid)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": newID})
}

func (app *App) handleDeleteSpending(w http.ResponseWriter, r *http.Request) {
	aid, ok := app.requireAccount(w, r)
	if !ok {
		return
	}
	sid, err := urlInt64(r, "sid")
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if _, err := app.DB.Exec(context.Background(), "DELETE FROM spending WHERE id=$1 AND account_id=$2", sid, aid); err != nil {
		dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (app *App) handleUpdateSpending(w http.ResponseWriter, r *http.Request) {
	aid, ok := app.requireAccount(w, r)
	if !ok {
		return
	}
	sid, err := urlInt64(r, "sid")
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var d map[string]any
	_ = decodeJSON(r, &d)

	fields := []string{}
	args := []any{}
	for _, k := range []string{"tx_date", "description", "amount", "checked"} {
		v, present := d[k]
		if !present {
			continue
		}
		if k == "amount" {
			if n, ok := asInt64(v); ok {
				v = n
			}
		}
		args = append(args, v)
		fields = append(fields, fmt.Sprintf("%s=$%d", k, len(args)))
	}
	if catName, ok := d["category"].(string); ok && catName != "" {
		ctx := context.Background()
		var catID *int64
		var id int64
		if err := app.DB.QueryRow(ctx, "SELECT id FROM categories WHERE name=$1 AND account_id=$2", catName, aid).Scan(&id); err == nil {
			catID = &id
		}
		args = append(args, catID)
		fields = append(fields, fmt.Sprintf("category_id=$%d", len(args)))
		args = append(args, catName)
		fields = append(fields, fmt.Sprintf("category_name=$%d", len(args)))
	}
	if len(fields) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	args = append(args, sid, aid)
	query := fmt.Sprintf("UPDATE spending SET %s WHERE id=$%d AND account_id=$%d", strings.Join(fields, ","), len(args)-1, len(args))
	if _, err := app.DB.Exec(context.Background(), query, args...); err != nil {
		dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---------- assets ----------

type assetRow struct {
	ID          int64   `json:"id"`
	AccountID   int64   `json:"account_id"`
	Description string  `json:"description"`
	Value       int64   `json:"value"`
	AccountRef  *int64  `json:"account_ref"`
	Kind        string  `json:"kind"`
	AccountName *string `json:"account_name"`
}

type assetTargetRow struct {
	ID          int64  `json:"id"`
	AccountID   int64  `json:"account_id"`
	TargetValue int64  `json:"target_value"`
	LastUpdate  string `json:"last_update"`
}

func (app *App) handleGetAssets(w http.ResponseWriter, r *http.Request) {
	aid, ok := app.requireAccount(w, r)
	if !ok {
		return
	}
	ctx := context.Background()
	rows, err := app.DB.Query(ctx,
		"SELECT a.id, a.account_id, a.description, a.value, a.account_ref, a.kind, ac.name FROM assets a "+
			"LEFT JOIN accounts ac ON ac.id=a.account_ref WHERE a.account_id=$1 ORDER BY a.id", aid)
	if err != nil {
		dbError(w, err)
		return
	}
	assets := []assetRow{}
	for rows.Next() {
		var a assetRow
		if err := rows.Scan(&a.ID, &a.AccountID, &a.Description, &a.Value, &a.AccountRef, &a.Kind, &a.AccountName); err != nil {
			rows.Close()
			dbError(w, err)
			return
		}
		assets = append(assets, a)
	}
	rows.Close()

	var target *assetTargetRow
	var t assetTargetRow
	var lastUpdate *string
	err = app.DB.QueryRow(ctx, "SELECT id, account_id, target_value, last_update FROM asset_target WHERE account_id=$1 ORDER BY id DESC LIMIT 1", aid).
		Scan(&t.ID, &t.AccountID, &t.TargetValue, &lastUpdate)
	if err == nil {
		if lastUpdate != nil {
			t.LastUpdate = *lastUpdate
		}
		target = &t
	}
	writeJSON(w, http.StatusOK, map[string]any{"assets": assets, "target": target})
}

func (app *App) handleAddAsset(w http.ResponseWriter, r *http.Request) {
	aid, ok := app.requireAccount(w, r)
	if !ok {
		return
	}
	var d struct {
		Description string `json:"description"`
		Value       int64  `json:"value"`
		AccountID   *int64 `json:"account_id"`
		Kind        string `json:"kind"`
	}
	_ = decodeJSON(r, &d)
	if d.Kind == "" {
		d.Kind = "liquid"
	}
	_, err := app.DB.Exec(context.Background(),
		"INSERT INTO assets (account_id, description, value, account_ref, kind) VALUES ($1,$2,$3,$4,$5)",
		aid, d.Description, d.Value, d.AccountID, d.Kind)
	if err != nil {
		dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (app *App) handleUpdateAsset(w http.ResponseWriter, r *http.Request) {
	aid, ok := app.requireAccount(w, r)
	if !ok {
		return
	}
	assetID, err := urlInt64(r, "aid")
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var d struct {
		Value int64 `json:"value"`
	}
	_ = decodeJSON(r, &d)
	if _, err := app.DB.Exec(context.Background(), "UPDATE assets SET value=$1 WHERE id=$2 AND account_id=$3", d.Value, assetID, aid); err != nil {
		dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (app *App) handleDeleteAsset(w http.ResponseWriter, r *http.Request) {
	aid, ok := app.requireAccount(w, r)
	if !ok {
		return
	}
	assetID, err := urlInt64(r, "aid")
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if _, err := app.DB.Exec(context.Background(), "DELETE FROM assets WHERE id=$1 AND account_id=$2", assetID, aid); err != nil {
		dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (app *App) handleSetAssetTarget(w http.ResponseWriter, r *http.Request) {
	aid, ok := app.requireAccount(w, r)
	if !ok {
		return
	}
	var d struct {
		TargetValue int64 `json:"target_value"`
	}
	_ = decodeJSON(r, &d)
	today := time.Now().Format("2006-01-02")
	_, err := app.DB.Exec(context.Background(),
		"INSERT INTO asset_target (account_id, target_value, last_update) VALUES ($1,$2,$3)", aid, d.TargetValue, today)
	if err != nil {
		dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---------- report / summary ----------

func (app *App) handleReport(w http.ResponseWriter, r *http.Request) {
	aid, ok := app.requireAccount(w, r)
	if !ok {
		return
	}
	year, err1 := urlInt64(r, "year")
	month, err2 := urlInt64(r, "month")
	if err1 != nil || err2 != nil {
		jsonError(w, http.StatusBadRequest, "invalid year/month")
		return
	}
	ctx := context.Background()

	var incomeTotal, expenseBudget, savingsTotal int64
	if err := app.DB.QueryRow(ctx, "SELECT COALESCE(SUM(amount),0) FROM budget_income WHERE account_id=$1 AND year=$2 AND month=$3", aid, year, month).Scan(&incomeTotal); err != nil {
		dbError(w, err)
		return
	}
	if err := app.DB.QueryRow(ctx, "SELECT COALESCE(SUM(amount),0) FROM budget_expense WHERE account_id=$1 AND year=$2 AND month=$3", aid, year, month).Scan(&expenseBudget); err != nil {
		dbError(w, err)
		return
	}
	if err := app.DB.QueryRow(ctx, "SELECT COALESCE(SUM(amount),0) FROM savings_list WHERE account_id=$1 AND year=$2 AND month=$3", aid, year, month).Scan(&savingsTotal); err != nil {
		dbError(w, err)
		return
	}

	ceilingClause := ""
	ceilingArgs := []any{}
	if ceiling := valueMaxCeiling(r); ceiling != nil {
		ceilingArgs = append(ceilingArgs, *ceiling)
		ceilingClause = fmt.Sprintf(" AND sp.amount <= $%d", 4)
	}
	q := fmt.Sprintf(`SELECT c.name, c.main_category, COALESCE(be.amount,0) as allocation,
	            COALESCE((SELECT SUM(sp.amount) FROM spending sp
	                      WHERE sp.category_id=c.id AND sp.account_id=$1
	                      AND EXTRACT(YEAR FROM sp.tx_date)::int=$2 AND EXTRACT(MONTH FROM sp.tx_date)::int=$3%s),0) as realization
	       FROM categories c
	       LEFT JOIN budget_expense be ON be.category_id=c.id AND be.account_id=$1 AND be.year=$2 AND be.month=$3
	       WHERE c.account_id=$1
	       ORDER BY c.id`, ceilingClause)
	args := append([]any{aid, year, month}, ceilingArgs...)
	rows, err := app.DB.Query(ctx, q, args...)
	if err != nil {
		dbError(w, err)
		return
	}
	type expItem struct {
		CategoryName string `json:"category_name"`
		MainCategory string `json:"main_category"`
		Allocation   int64  `json:"allocation"`
		Realization  int64  `json:"realization"`
	}
	expensesList := []expItem{}
	for rows.Next() {
		var e expItem
		if err := rows.Scan(&e.CategoryName, &e.MainCategory, &e.Allocation, &e.Realization); err != nil {
			rows.Close()
			dbError(w, err)
			return
		}
		expensesList = append(expensesList, e)
	}
	rows.Close()

	writeJSON(w, http.StatusOK, map[string]any{
		"total_income":       incomeTotal,
		"budgeted_expenses":  expenseBudget,
		"savings":            savingsTotal,
		"left_for_budgeting": incomeTotal - expenseBudget - savingsTotal,
		"expenses_list":      expensesList,
	})
}

func (app *App) handleReportYear(w http.ResponseWriter, r *http.Request) {
	aid, ok := app.requireAccount(w, r)
	if !ok {
		return
	}
	year, err := urlInt64(r, "year")
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid year")
		return
	}
	q := `SELECT COALESCE(c.name, sp.category_name, 'Other') as category_name, SUM(sp.amount) as total
	      FROM spending sp LEFT JOIN categories c ON c.id=sp.category_id
	      WHERE sp.account_id=$1 AND EXTRACT(YEAR FROM sp.tx_date)::int=$2`
	args := []any{aid, year}
	q, args = appendCeiling(q, args, r, "sp.amount")
	q += " GROUP BY category_name ORDER BY total DESC"
	rows, err := app.DB.Query(context.Background(), q, args...)
	if err != nil {
		dbError(w, err)
		return
	}
	type outRow struct {
		CategoryName string `json:"category_name"`
		Total        int64  `json:"total"`
	}
	out := []outRow{}
	for rows.Next() {
		var o outRow
		if err := rows.Scan(&o.CategoryName, &o.Total); err != nil {
			rows.Close()
			dbError(w, err)
			return
		}
		out = append(out, o)
	}
	rows.Close()
	writeJSON(w, http.StatusOK, out)
}

func (app *App) handleReportMonthCompare(w http.ResponseWriter, r *http.Request) {
	aid, ok := app.requireAccount(w, r)
	if !ok {
		return
	}
	year, err1 := urlInt64(r, "year")
	month, err2 := urlInt64(r, "month")
	if err1 != nil || err2 != nil {
		jsonError(w, http.StatusBadRequest, "invalid year/month")
		return
	}
	ctx := context.Background()
	expenseFor := func(y, m int64) (int64, error) {
		q := "SELECT COALESCE(SUM(amount),0) FROM spending WHERE account_id=$1 AND EXTRACT(YEAR FROM tx_date)::int=$2 AND EXTRACT(MONTH FROM tx_date)::int=$3"
		args := []any{aid, y, m}
		q, args = appendCeiling(q, args, r, "amount")
		var total int64
		err := app.DB.QueryRow(ctx, q, args...).Scan(&total)
		return total, err
	}
	current, err := expenseFor(year, month)
	if err != nil {
		dbError(w, err)
		return
	}
	prevY, prevM := year, month-1
	if month == 1 {
		prevY, prevM = year-1, 12
	}
	previous, err := expenseFor(prevY, prevM)
	if err != nil {
		dbError(w, err)
		return
	}
	lastYear, err := expenseFor(year-1, month)
	if err != nil {
		dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"current": current, "previous_month": previous, "same_month_last_year": lastYear})
}

func (app *App) handlePatternsYear(w http.ResponseWriter, r *http.Request) {
	aid, ok := app.requireAccount(w, r)
	if !ok {
		return
	}
	year, err := urlInt64(r, "year")
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid year")
		return
	}
	rows, err := app.fetchSpendingDates(context.Background(), aid, " AND EXTRACT(YEAR FROM tx_date)::int=$2", []any{year})
	if err != nil {
		dbError(w, err)
		return
	}
	ceiling := valueMaxCeiling(r)

	byDate := map[int]int64{}
	byWeek := map[int]int64{}
	byWeekday := map[int]int64{}
	byWeekInMonth := map[int]int64{}
	monthsPresentSet := map[[2]int]bool{}
	for _, row := range rows {
		if ceiling != nil && row.Amt > *ceiling {
			continue
		}
		d := row.Date
		byDate[d.Day()] += row.Amt
		_, wk := d.ISOWeek()
		byWeek[wk] += row.Amt
		pyWeekday := (int(d.Weekday()) + 6) % 7
		byWeekday[pyWeekday] += row.Amt
		wim := (d.Day()-1)/7 + 1
		byWeekInMonth[wim] += row.Amt
		monthsPresentSet[[2]int{d.Year(), int(d.Month())}] = true
	}

	dayCounts := map[int]int{}
	for ym := range monthsPresentSet {
		daysInMonth := time.Date(ym[0], time.Month(ym[1]+1), 0, 0, 0, 0, 0, time.UTC).Day()
		for dom := 1; dom <= daysInMonth; dom++ {
			dayCounts[dom]++
		}
	}
	type dayPattern struct {
		Day int     `json:"day"`
		Avg float64 `json:"avg"`
	}
	datePattern := []dayPattern{}
	for dom := 1; dom <= 31; dom++ {
		if dayCounts[dom] > 0 {
			datePattern = append(datePattern, dayPattern{Day: dom, Avg: float64(byDate[dom]) / float64(dayCounts[dom])})
		}
	}

	weekKeys := []int{}
	for wk := range byWeek {
		weekKeys = append(weekKeys, wk)
	}
	sort.Ints(weekKeys)
	type weekPattern struct {
		Week int     `json:"week"`
		Avg  float64 `json:"avg"`
	}
	weekPatternOut := []weekPattern{}
	for _, wk := range weekKeys {
		weekPatternOut = append(weekPatternOut, weekPattern{Week: wk, Avg: float64(byWeek[wk])})
	}

	occ := map[int]int{}
	d0 := time.Date(int(year), 1, 1, 0, 0, 0, 0, time.UTC)
	d1 := time.Date(int(year), 12, 31, 0, 0, 0, 0, time.UTC)
	for cur := d0; !cur.After(d1); cur = cur.AddDate(0, 0, 1) {
		pyWeekday := (int(cur.Weekday()) + 6) % 7
		occ[pyWeekday]++
	}
	type weekdayPattern struct {
		Weekday int     `json:"weekday"`
		Avg     float64 `json:"avg"`
	}
	weekdayPatternOut := []weekdayPattern{}
	for wd := 0; wd < 7; wd++ {
		avg := 0.0
		if occ[wd] > 0 {
			avg = float64(byWeekday[wd]) / float64(occ[wd])
		}
		weekdayPatternOut = append(weekdayPatternOut, weekdayPattern{Weekday: wd, Avg: avg})
	}

	monthsCount := len(monthsPresentSet)
	if monthsCount == 0 {
		monthsCount = 1
	}
	type weekInMonthPattern struct {
		Week int     `json:"week"`
		Avg  float64 `json:"avg"`
	}
	weekInMonthOut := []weekInMonthPattern{}
	for wim := 1; wim <= 5; wim++ {
		weekInMonthOut = append(weekInMonthOut, weekInMonthPattern{Week: wim, Avg: float64(byWeekInMonth[wim]) / float64(monthsCount)})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"by_date":          datePattern,
		"by_week":          weekPatternOut,
		"by_weekday":       weekdayPatternOut,
		"by_week_in_month": weekInMonthOut,
	})
}

func (app *App) handleSummary(w http.ResponseWriter, r *http.Request) {
	aid, ok := app.requireAccount(w, r)
	if !ok {
		return
	}
	now := time.Now()
	year := queryIntDefault(r, "year", now.Year())
	month := queryIntDefault(r, "month", int(now.Month()))
	ctx := context.Background()

	type mainCatTotal struct {
		MainCategory string `json:"main_category"`
		Total        int64  `json:"total"`
	}
	byMain := []mainCatTotal{}
	rows, err := app.DB.Query(ctx,
		`SELECT c.main_category, COALESCE(SUM(sp.amount),0) as total
		 FROM spending sp JOIN categories c ON c.id=sp.category_id
		 WHERE sp.account_id=$1 AND EXTRACT(YEAR FROM sp.tx_date)::int=$2 AND EXTRACT(MONTH FROM sp.tx_date)::int=$3
		 GROUP BY c.main_category`, aid, year, month)
	if err != nil {
		dbError(w, err)
		return
	}
	for rows.Next() {
		var m mainCatTotal
		if err := rows.Scan(&m.MainCategory, &m.Total); err != nil {
			rows.Close()
			dbError(w, err)
			return
		}
		byMain = append(byMain, m)
	}
	rows.Close()

	type catTotal struct {
		CategoryName string `json:"category_name"`
		Total        int64  `json:"total"`
	}
	byCategory := []catTotal{}
	rows, err = app.DB.Query(ctx,
		`SELECT COALESCE(c.name, sp.category_name, 'Other') as category_name, COALESCE(SUM(sp.amount),0) as total
		 FROM spending sp LEFT JOIN categories c ON c.id=sp.category_id
		 WHERE sp.account_id=$1 AND EXTRACT(YEAR FROM sp.tx_date)::int=$2 AND EXTRACT(MONTH FROM sp.tx_date)::int=$3
		 GROUP BY category_name ORDER BY total DESC`, aid, year, month)
	if err != nil {
		dbError(w, err)
		return
	}
	for rows.Next() {
		var c catTotal
		if err := rows.Scan(&c.CategoryName, &c.Total); err != nil {
			rows.Close()
			dbError(w, err)
			return
		}
		byCategory = append(byCategory, c)
	}
	rows.Close()

	accounts := []accountRow{}
	rows, err = app.DB.Query(ctx, "SELECT id, account_id, name, kind FROM accounts WHERE account_id=$1 AND kind='bank' ORDER BY id", aid)
	if err != nil {
		dbError(w, err)
		return
	}
	for rows.Next() {
		var a accountRow
		if err := rows.Scan(&a.ID, &a.AccountID, &a.Name, &a.Kind); err != nil {
			rows.Close()
			dbError(w, err)
			return
		}
		accounts = append(accounts, a)
	}
	rows.Close()

	writeJSON(w, http.StatusOK, map[string]any{"by_main_category": byMain, "by_category": byCategory, "accounts": accounts})
}
