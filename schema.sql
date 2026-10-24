-- Accountamily schema — multi-tenant fork of tatadana.
-- Every data row carries account_id (the owning user's id) so each signup
-- gets fully isolated data. ponytail: "account" == single user for now,
-- no shared/family multi-user accounts yet — add a separate accounts/members
-- table if that's ever needed.
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    email TEXT UNIQUE NOT NULL,
    password_hash TEXT,           -- NULL if signed up via Google only
    google_sub TEXT UNIQUE,       -- Google account id, NULL if password-only
    display_name TEXT,
    is_admin INTEGER NOT NULL DEFAULT 0,    -- super-admin: can view/manage all accounts
    is_disabled INTEGER NOT NULL DEFAULT 0, -- admin-controlled account suspension
    currency TEXT NOT NULL DEFAULT 'IDR',   -- display-only, see CURRENCIES in api.js
    locale TEXT NOT NULL DEFAULT 'id-ID',
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- "bank"/"asset" accounts the user tags transactions/assets with
CREATE TABLE IF NOT EXISTS accounts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id INTEGER NOT NULL REFERENCES users(id),
    name TEXT NOT NULL,
    kind TEXT NOT NULL DEFAULT 'bank',  -- bank | asset
    UNIQUE(account_id, name)
);

CREATE TABLE IF NOT EXISTS categories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id INTEGER NOT NULL REFERENCES users(id),
    name TEXT NOT NULL,
    main_category TEXT NOT NULL CHECK(main_category IN ('Needs','Wants','Savings')),
    UNIQUE(account_id, name)
);

CREATE TABLE IF NOT EXISTS income_sources (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id INTEGER NOT NULL REFERENCES users(id),
    name TEXT NOT NULL,
    UNIQUE(account_id, name)
);

CREATE TABLE IF NOT EXISTS budget_income (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id INTEGER NOT NULL REFERENCES users(id),
    year INTEGER NOT NULL,
    month INTEGER NOT NULL,
    source_id INTEGER NOT NULL REFERENCES income_sources(id),
    amount INTEGER NOT NULL DEFAULT 0,
    UNIQUE(account_id, year, month, source_id)
);

CREATE TABLE IF NOT EXISTS budget_expense (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id INTEGER NOT NULL REFERENCES users(id),
    year INTEGER NOT NULL,
    month INTEGER NOT NULL,
    category_id INTEGER NOT NULL REFERENCES categories(id),
    amount INTEGER NOT NULL DEFAULT 0,
    UNIQUE(account_id, year, month, category_id)
);

CREATE TABLE IF NOT EXISTS spending (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id INTEGER NOT NULL REFERENCES users(id),
    tx_date TEXT NOT NULL,
    description TEXT NOT NULL,
    category_id INTEGER REFERENCES categories(id),
    category_name TEXT,
    amount INTEGER NOT NULL,
    checked INTEGER NOT NULL DEFAULT 0,
    source TEXT NOT NULL DEFAULT 'web',  -- web | telegram | discord | migration
    client_id TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(account_id, client_id)
);

CREATE TABLE IF NOT EXISTS assets (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id INTEGER NOT NULL REFERENCES users(id),
    description TEXT NOT NULL,
    value INTEGER NOT NULL DEFAULT 0,
    account_ref INTEGER REFERENCES accounts(id),  -- the bank/asset account it's tagged to
    kind TEXT NOT NULL DEFAULT 'liquid'  -- liquid | non_liquid
);

CREATE TABLE IF NOT EXISTS asset_target (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id INTEGER NOT NULL REFERENCES users(id),
    target_value INTEGER NOT NULL,
    last_update TEXT
);

CREATE TABLE IF NOT EXISTS savings_list (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id INTEGER NOT NULL REFERENCES users(id),
    year INTEGER NOT NULL,
    month INTEGER NOT NULL,
    name TEXT NOT NULL,
    amount INTEGER NOT NULL DEFAULT 0,
    UNIQUE(account_id, year, month, name)
);

-- Chat-platform integrations (Telegram, Discord, ...), one row per platform
-- per account. bot_token is this account's secret used by the platform
-- adapter process to authenticate its /api/bot/* calls (see app.py).
CREATE TABLE IF NOT EXISTS integrations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id INTEGER NOT NULL REFERENCES users(id),
    platform TEXT NOT NULL CHECK(platform IN ('telegram','discord')),
    bot_token TEXT NOT NULL UNIQUE,      -- accountamily-issued secret, sent as X-Bot-Token
    platform_token TEXT,                 -- the actual Telegram/Discord bot API token
    chat_id TEXT,                        -- allowlisted group/channel id
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(account_id, platform)
);

-- Reminder schedule overrides, replacing rumahbot's in-file /jadwal storage.
-- One row per named job per account; absence of a row = use DEFAULT_REMINDERS.
CREATE TABLE IF NOT EXISTS reminder_schedules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id INTEGER NOT NULL REFERENCES users(id),
    job_name TEXT NOT NULL,
    days TEXT NOT NULL,       -- comma-separated 0-6 (0=Sun)
    hour INTEGER NOT NULL,
    minute INTEGER NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    last_run TEXT,
    UNIQUE(account_id, job_name)
);

-- Mandatory ingredients list (rumahbot's /tambahbahan), per account.
CREATE TABLE IF NOT EXISTS mandatory_ingredients (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id INTEGER NOT NULL REFERENCES users(id),
    name TEXT NOT NULL,
    UNIQUE(account_id, name)
);

-- Learned category keywords (rumahbot's auto-categorization learning), per account.
CREATE TABLE IF NOT EXISTS category_keywords (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id INTEGER NOT NULL REFERENCES users(id),
    keyword TEXT NOT NULL,
    category TEXT NOT NULL,
    UNIQUE(account_id, keyword)
);
