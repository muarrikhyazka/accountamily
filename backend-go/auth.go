package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"
)

// ---------- static data (ported from Python CURRENCIES / DEFAULT_* dicts) ----------

type currencyInfo struct {
	Symbol string `json:"symbol"`
	Locale string `json:"locale"`
}

var currencies = map[string]currencyInfo{
	"IDR": {"Rp", "id-ID"},
	"USD": {"$", "en-US"},
	"EUR": {"€", "de-DE"},
	"GBP": {"£", "en-GB"},
	"SGD": {"S$", "en-SG"},
	"MYR": {"RM", "ms-MY"},
	"JPY": {"¥", "ja-JP"},
	"AUD": {"A$", "en-AU"},
	"INR": {"₹", "en-IN"},
}

var defaultCategories = map[string][]string{
	"Needs": {"Home Rent", "Utilities", "Food", "Supplies", "Transportation", "Healthcare",
		"Energy", "Skincare & Beauty", "Services", "Others"},
	"Wants": {"Clothes & Bags", "Application", "Charity", "Travel", "Gifts", "Learning",
		"Jajan", "Internet & Pulsa", "Electricity", "Entertainment", "Business"},
	"Savings": {"Child", "Emergency Funds", "House", "Car"},
}
var defaultIncomeSources = []string{"Salary", "Freelance"}
var defaultBankAccounts = []string{"Bank 1", "Bank 2"}

// seedDefaults ports Python's seed_defaults(): starter categories/accounts/
// income sources for a brand-new signup. All ON CONFLICT DO NOTHING, same
// as SQLite's INSERT OR IGNORE.
func seedDefaults(ctx context.Context, app *App, accountID int64) error {
	for _, name := range defaultBankAccounts {
		if _, err := app.DB.Exec(ctx,
			"INSERT INTO accounts (account_id, name, kind) VALUES ($1,$2,'bank') ON CONFLICT (account_id, name) DO NOTHING",
			accountID, name); err != nil {
			return err
		}
	}
	for _, name := range defaultIncomeSources {
		if _, err := app.DB.Exec(ctx,
			"INSERT INTO income_sources (account_id, name) VALUES ($1,$2) ON CONFLICT (account_id, name) DO NOTHING",
			accountID, name); err != nil {
			return err
		}
	}
	for mainCat, names := range defaultCategories {
		for _, name := range names {
			if _, err := app.DB.Exec(ctx,
				"INSERT INTO categories (account_id, name, main_category) VALUES ($1,$2,$3) ON CONFLICT (account_id, name) DO NOTHING",
				accountID, name, mainCat); err != nil {
				return err
			}
		}
	}
	return nil
}

// ---------- rate limiting ----------
// ponytail: in-memory sliding window, single-process only (same limitation
// as the Python original) - swap for a shared store (Redis) if we ever run
// >1 instance.
const (
	rateLimitWindow = 5 * time.Minute
	rateLimitMax    = 5
)

var (
	rateLimitMu   sync.Mutex
	rateLimitHits = map[string][]time.Time{}
)

func rateLimited(key string) bool {
	rateLimitMu.Lock()
	defer rateLimitMu.Unlock()
	now := time.Now()
	hits := rateLimitHits[key][:0:0]
	for _, t := range rateLimitHits[key] {
		if now.Sub(t) < rateLimitWindow {
			hits = append(hits, t)
		}
	}
	hits = append(hits, now)
	rateLimitHits[key] = hits
	return len(hits) > rateLimitMax
}

func clientIP(r *http.Request) string {
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		return strings.TrimSpace(strings.Split(ip, ",")[0])
	}
	return r.RemoteAddr
}

// rateLimitMiddleware mirrors Python's rate_limit(prefix) decorator: 5
// attempts/5min per IP+email (email/username read from the JSON body,
// body is restored for the real handler afterwards).
func rateLimitMiddleware(prefix string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var d map[string]any
			_ = decodeJSON(r, &d)
			ident := ""
			if d != nil {
				if v, ok := d["email"].(string); ok && v != "" {
					ident = v
				} else if v, ok := d["username"].(string); ok {
					ident = v
				}
			}
			ident = strings.ToLower(strings.TrimSpace(ident))
			key := prefix + ":" + clientIP(r) + ":" + ident
			if rateLimited(key) {
				jsonError(w, http.StatusTooManyRequests, "too many attempts, try again in a few minutes")
				return
			}
			// handler needs to read the JSON body again - re-marshal what we decoded.
			if d != nil {
				b, _ := json.Marshal(d)
				r.Body = io.NopCloser(bytes.NewReader(b))
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ---------- auth routes ----------

func RegisterAuthRoutes(r chi.Router, app *App) {
	r.With(rateLimitMiddleware("signup")).Post("/signup", app.handleSignup)
	r.With(rateLimitMiddleware("login")).Post("/login", app.handleLogin)
	r.Post("/logout", app.handleLogout)
	r.Get("/me", app.handleMe)
	r.Patch("/me/settings", app.handleUpdateSettings)
	r.Get("/currencies", app.handleCurrencies)
	r.Get("/verify-email", app.handleVerifyEmail)
	r.With(rateLimitMiddleware("pwreset")).Post("/password-reset/request", app.handlePasswordResetRequest)
	r.Post("/password-reset/confirm", app.handlePasswordResetConfirm)
}

func (app *App) handleSignup(w http.ResponseWriter, r *http.Request) {
	var d struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
	}
	_ = decodeJSON(r, &d)
	email := strings.ToLower(strings.TrimSpace(d.Email))
	password := d.Password
	displayName := strings.TrimSpace(d.DisplayName)
	if displayName == "" {
		displayName = strings.SplitN(email, "@", 2)[0]
	}
	if email == "" || !strings.Contains(email, "@") || len(password) < 8 {
		jsonError(w, http.StatusBadRequest, "valid email and password (8+ chars) required")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "could not hash password")
		return
	}
	verificationToken := randomToken()
	ctx := context.Background()
	var accountID int64
	err = app.DB.QueryRow(ctx,
		"INSERT INTO users (email, password_hash, display_name, is_verified, verification_token) VALUES ($1,$2,$3,FALSE,$4) RETURNING id",
		email, string(hash), displayName, verificationToken).Scan(&accountID)
	if err != nil {
		jsonError(w, http.StatusConflict, "email already registered")
		return
	}
	if err := seedDefaults(ctx, app, accountID); err != nil {
		jsonError(w, http.StatusInternalServerError, "seed failed")
		return
	}
	// ponytail: no SMTP wired yet, add real email send when SMTP creds exist
	// in env - for now the verify link just goes to the log, same as Python.
	log.Printf("verify-email link for %s: %s/verify-email?token=%s", email, app.Cfg.AppBaseURL, verificationToken)
	if err := setSessionAccount(w, r, accountID, displayName); err != nil {
		jsonError(w, http.StatusInternalServerError, "session error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "display_name": displayName})
}

func (app *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	var d struct {
		Email    string `json:"email"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	_ = decodeJSON(r, &d)
	email := d.Email
	if email == "" {
		email = d.Username
	}
	email = strings.ToLower(strings.TrimSpace(email))

	var id int64
	var passwordHash *string
	var displayName *string
	var isDisabled bool
	ctx := context.Background()
	err := app.DB.QueryRow(ctx,
		"SELECT id, password_hash, display_name, is_disabled FROM users WHERE email=$1", email).
		Scan(&id, &passwordHash, &displayName, &isDisabled)
	if err != nil || passwordHash == nil {
		jsonError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	ok, isLegacy := verifyPasswordHash(*passwordHash, d.Password)
	if !ok {
		jsonError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	// Migrated accounts carry werkzeug (scrypt/pbkdf2) hashes, not bcrypt.
	// Transparently upgrade to bcrypt on first successful login so the
	// legacy verify path is only ever needed once per user.
	if isLegacy {
		if newHash, herr := bcrypt.GenerateFromPassword([]byte(d.Password), bcrypt.DefaultCost); herr == nil {
			_, _ = app.DB.Exec(ctx, "UPDATE users SET password_hash=$1 WHERE id=$2", string(newHash), id)
		}
	}
	if isDisabled {
		jsonError(w, http.StatusForbidden, "account disabled")
		return
	}
	name := email
	if displayName != nil && *displayName != "" {
		name = *displayName
	}
	// ponytail: not blocking unverified accounts yet, matches Python.
	if err := setSessionAccount(w, r, id, name); err != nil {
		jsonError(w, http.StatusInternalServerError, "session error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "display_name": name})
}

func (app *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	if err := clearSession(w, r); err != nil {
		jsonError(w, http.StatusInternalServerError, "session error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (app *App) handleMe(w http.ResponseWriter, r *http.Request) {
	accountID, ok := currentAccountID(r)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"authenticated": false})
		return
	}
	var email, currency, locale string
	var isAdmin bool
	err := app.DB.QueryRow(context.Background(),
		"SELECT email, currency, locale, is_admin FROM users WHERE id=$1", accountID).
		Scan(&email, &currency, &locale, &isAdmin)
	resp := map[string]any{"authenticated": true}
	s := getSession(r)
	resp["display_name"] = s.Values["display"]
	if err != nil {
		resp["username"] = nil
		resp["currency"] = "IDR"
		resp["locale"] = "id-ID"
		resp["is_admin"] = false
	} else {
		resp["username"] = email
		resp["currency"] = currency
		resp["locale"] = locale
		resp["is_admin"] = isAdmin
	}
	writeJSON(w, http.StatusOK, resp)
}

func (app *App) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	accountID, ok := currentAccountID(r)
	if !ok {
		jsonError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var d struct {
		DisplayName string `json:"display_name"`
		Currency    string `json:"currency"`
	}
	_ = decodeJSON(r, &d)

	sets := []string{}
	args := []any{}
	n := 1
	name := strings.TrimSpace(d.DisplayName)
	if name != "" {
		sets = append(sets, "display_name=$"+strconv.Itoa(n))
		args = append(args, name)
		n++
	}
	if info, ok := currencies[d.Currency]; ok {
		sets = append(sets, "currency=$"+strconv.Itoa(n))
		args = append(args, d.Currency)
		n++
		sets = append(sets, "locale=$"+strconv.Itoa(n))
		args = append(args, info.Locale)
		n++
	}
	if len(sets) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	args = append(args, accountID)
	query := "UPDATE users SET " + strings.Join(sets, ",") + " WHERE id=$" + strconv.Itoa(n)
	if _, err := app.DB.Exec(context.Background(), query, args...); err != nil {
		jsonError(w, http.StatusInternalServerError, "db error")
		return
	}
	if name != "" {
		s := getSession(r)
		s.Values["display"] = name
		_ = s.Save(r, w)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (app *App) handleCurrencies(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, currencies)
}

func (app *App) handleVerifyEmail(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		jsonError(w, http.StatusBadRequest, "token required")
		return
	}
	ctx := context.Background()
	var id int64
	err := app.DB.QueryRow(ctx, "SELECT id FROM users WHERE verification_token=$1", token).Scan(&id)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid or already-used token")
		return
	}
	if _, err := app.DB.Exec(ctx, "UPDATE users SET is_verified=TRUE, verification_token=NULL WHERE id=$1", id); err != nil {
		jsonError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (app *App) handlePasswordResetRequest(w http.ResponseWriter, r *http.Request) {
	var d struct {
		Email string `json:"email"`
	}
	_ = decodeJSON(r, &d)
	email := strings.ToLower(strings.TrimSpace(d.Email))

	ctx := context.Background()
	var userID int64
	err := app.DB.QueryRow(ctx, "SELECT id FROM users WHERE email=$1", email).Scan(&userID)
	if err == nil {
		token := randomToken()
		expiresAt := time.Now().Add(time.Hour)
		if _, err := app.DB.Exec(ctx,
			"INSERT INTO password_resets (user_id, token, expires_at) VALUES ($1,$2,$3)",
			userID, token, expiresAt); err == nil {
			// ponytail: no SMTP wired yet - reset link just goes to the log.
			log.Printf("password-reset link for %s: %s/reset-password?token=%s", email, app.Cfg.AppBaseURL, token)
		}
	}
	// Always 200 regardless of whether the email exists, to avoid leaking
	// which emails are registered.
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (app *App) handlePasswordResetConfirm(w http.ResponseWriter, r *http.Request) {
	var d struct {
		Token       string `json:"token"`
		NewPassword string `json:"new_password"`
	}
	_ = decodeJSON(r, &d)
	if d.Token == "" || len(d.NewPassword) < 8 {
		jsonError(w, http.StatusBadRequest, "token and password (8+ chars) required")
		return
	}
	ctx := context.Background()
	var id, userID int64
	var expiresAt time.Time
	err := app.DB.QueryRow(ctx,
		"SELECT id, user_id, expires_at FROM password_resets WHERE token=$1 AND used=FALSE", d.Token).
		Scan(&id, &userID, &expiresAt)
	if err != nil || time.Now().After(expiresAt) {
		jsonError(w, http.StatusBadRequest, "invalid or expired token")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(d.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "could not hash password")
		return
	}
	if _, err := app.DB.Exec(ctx, "UPDATE users SET password_hash=$1 WHERE id=$2", string(hash), userID); err != nil {
		jsonError(w, http.StatusInternalServerError, "db error")
		return
	}
	if _, err := app.DB.Exec(ctx, "UPDATE password_resets SET used=TRUE WHERE id=$1", id); err != nil {
		jsonError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---------- small helpers ----------

var errInvalidToken = &tokenError{"invalid token"}

type tokenError struct{ msg string }

func (e *tokenError) Error() string { return e.msg }

func randomToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
