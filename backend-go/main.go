package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB is the shared Postgres connection pool, available to every handler via
// the App struct. All SQL in this project is raw (no ORM) - same philosophy
// as the Python version it replaces.
type App struct {
	DB  *pgxpool.Pool
	Cfg Config
}

type Config struct {
	Port          string
	DatabaseURL   string
	SessionSecret string
	StaticDir     string
	AppBaseURL    string
	ServiceToken  string // shared secret for /internal/* endpoints (bot multiplexers)
}

func loadConfig() Config {
	return Config{
		Port:           getenv("PORT", "5000"),
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		SessionSecret:  getenv("SECRET_KEY", "dev-secret-change-me"),
		StaticDir:      getenv("STATIC_DIR", "/app/react_dist"),
		AppBaseURL:     getenv("APP_BASE_URL", "http://localhost:5173"),
		ServiceToken:   os.Getenv("SERVICE_TOKEN"),
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	cfg := loadConfig()
	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(context.Background()); err != nil {
		log.Fatalf("db ping: %v", err)
	}

	app := &App{DB: pool, Cfg: cfg}
	if err := initSchema(context.Background(), app); err != nil {
		log.Fatalf("schema init: %v", err)
	}
	initSessionStore(cfg.SessionSecret)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Route("/api", func(api chi.Router) {
		api.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		})
		RegisterAuthRoutes(api, app)     // login/signup/me/logout/password-reset/verify-email
		RegisterFinanceRoutes(api, app)  // spending/budgeting/assets/setup/categories/accounts/income-sources/reports/trend/fx-rates
		RegisterAdminRoutes(api, app)    // «redacted-vault-secret» accounts/stats/reset-demo-data + bot (/api/bot/*) + internal (/api/internal/*) + integrations CRUD
		RegisterGamificationRoutes(api, app) // points/streak/badge
	})

	// SPA static files - serve react build, fallback to index.html for client routing
	r.Get("/*", SPAHandler(cfg.StaticDir))

	log.Printf("accountamily-go listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, r); err != nil {
		log.Fatal(err)
	}
}
