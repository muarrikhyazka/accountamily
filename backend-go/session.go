package main

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/sessions"
)

// Sessions mirrors Flask's cookie-session: account_id stored in the
// cookie, HttpOnly, same lifetime semantics. Replace with a shared
// store (Redis) only if this ever runs >1 instance behind a load balancer.
var store *sessions.CookieStore

func initSessionStore(secret string) {
	store = sessions.NewCookieStore([]byte(secret))
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 30,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
}

func getSession(r *http.Request) *sessions.Session {
	s, _ := store.Get(r, "accountamily")
	return s
}

func currentAccountID(r *http.Request) (int64, bool) {
	s := getSession(r)
	v, ok := s.Values["account_id"]
	if !ok {
		return 0, false
	}
	id, ok := v.(int64)
	return id, ok
}

func setSessionAccount(w http.ResponseWriter, r *http.Request, accountID int64, displayName string) error {
	s := getSession(r)
	s.Values["account_id"] = accountID
	s.Values["display"] = displayName
	return s.Save(r, w)
}

func clearSession(w http.ResponseWriter, r *http.Request) error {
	s := getSession(r)
	s.Values = map[interface{}]interface{}{}
	s.Options.MaxAge = -1
	return s.Save(r, w)
}

// ---------- JSON helpers ----------

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decodeJSON(r *http.Request, v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}
