-- Accountamily Postgres schema — ported from the SQLite original.
-- Every data row carries account_id (the owning user's id) so each signup
-- gets fully isolated data. ponytail: "account" == single user for now,
-- no shared/family multi-user accounts yet — add a separate accounts/members
-- table if that's ever needed.
CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    email TEXT UNIQUE NOT NULL,
    password_hash TEXT,            -- NULL if signed up via Google only
    google_sub TEXT UNIQUE,        -- Google account id, NULL if password-only
    display_name TEXT,
    is_admin BOOLEAN NOT NULL DEFAULT FALSE,     -- super-admin: can view/manage all accounts
    is_disabled BOOLEAN NOT NULL DEFAULT FALSE,  -- admin-controlled account suspension
    is_verified BOOLEAN NOT NULL DEFAULT FALSE,  -- set once /api/verify-email consumes the token
    verification_token TEXT,                     -- NULL once verified
    currency TEXT NOT NULL DEFAULT 'IDR',
    locale TEXT NOT NULL DEFAULT 'id-ID',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Password-reset tokens, one-time use, 1hr expiry.
CREATE TABLE IF NOT EXISTS password_resets (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id),
    token TEXT UNIQUE NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- "bank"/"asset" accounts the user tags transactions/assets with
CREATE TABLE IF NOT EXISTS accounts (
    id BIGSERIAL PRIMARY KEY,
    account_id BIGINT NOT NULL REFERENCES users(id),
    name TEXT NOT NULL,
    kind TEXT NOT NULL DEFAULT 'bank',  -- bank | asset
    UNIQUE(account_id, name)
);

CREATE TABLE IF NOT EXISTS categories (
    id BIGSERIAL PRIMARY KEY,
    account_id BIGINT NOT NULL REFERENCES users(id),
    name TEXT NOT NULL,
    main_category TEXT NOT NULL CHECK(main_category IN ('Needs','Wants','Savings')),
    UNIQUE(account_id, name)
);

CREATE TABLE IF NOT EXISTS income_sources (
    id BIGSERIAL PRIMARY KEY,
    account_id BIGINT NOT NULL REFERENCES users(id),
    name TEXT NOT NULL,
    UNIQUE(account_id, name)
);

CREATE TABLE IF NOT EXISTS budget_income (
    id BIGSERIAL PRIMARY KEY,
    account_id BIGINT NOT NULL REFERENCES users(id),
    year INTEGER NOT NULL,
    month INTEGER NOT NULL,
    source_id BIGINT NOT NULL REFERENCES income_sources(id),
    amount BIGINT NOT NULL DEFAULT 0,
    UNIQUE(account_id, year, month, source_id)
);

CREATE TABLE IF NOT EXISTS budget_expense (
    id BIGSERIAL PRIMARY KEY,
    account_id BIGINT NOT NULL REFERENCES users(id),
    year INTEGER NOT NULL,
    month INTEGER NOT NULL,
    category_id BIGINT NOT NULL REFERENCES categories(id),
    amount BIGINT NOT NULL DEFAULT 0,
    UNIQUE(account_id, year, month, category_id)
);

CREATE TABLE IF NOT EXISTS spending (
    id BIGSERIAL PRIMARY KEY,
    account_id BIGINT NOT NULL REFERENCES users(id),
    tx_date DATE NOT NULL,
    description TEXT NOT NULL,
    category_id BIGINT REFERENCES categories(id),
    category_name TEXT,
    amount BIGINT NOT NULL,
    checked BOOLEAN NOT NULL DEFAULT FALSE,
    source TEXT NOT NULL DEFAULT 'web',  -- web | telegram | discord | migration
    client_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(account_id, client_id)
);

CREATE TABLE IF NOT EXISTS assets (
    id BIGSERIAL PRIMARY KEY,
    account_id BIGINT NOT NULL REFERENCES users(id),
    description TEXT NOT NULL,
    value BIGINT NOT NULL DEFAULT 0,
    account_ref BIGINT REFERENCES accounts(id),
    kind TEXT NOT NULL DEFAULT 'liquid'  -- liquid | non_liquid
);

CREATE TABLE IF NOT EXISTS asset_target (
    id BIGSERIAL PRIMARY KEY,
    account_id BIGINT NOT NULL REFERENCES users(id),
    target_value BIGINT NOT NULL,
    last_update TEXT
);

CREATE TABLE IF NOT EXISTS savings_list (
    id BIGSERIAL PRIMARY KEY,
    account_id BIGINT NOT NULL REFERENCES users(id),
    year INTEGER NOT NULL,
    month INTEGER NOT NULL,
    name TEXT NOT NULL,
    amount BIGINT NOT NULL DEFAULT 0,
    UNIQUE(account_id, year, month, name)
);

-- Chat-platform integrations (Telegram, Discord, ...), one row per platform
-- per account. bot_token is this account's secret used by the platform
-- adapter process to authenticate its /api/bot/* calls.
CREATE TABLE IF NOT EXISTS integrations (
    id BIGSERIAL PRIMARY KEY,
    account_id BIGINT NOT NULL REFERENCES users(id),
    platform TEXT NOT NULL CHECK(platform IN ('telegram','discord')),
    bot_token TEXT NOT NULL UNIQUE,
    platform_token TEXT,
    chat_id TEXT,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(account_id, platform)
);

CREATE TABLE IF NOT EXISTS reminder_schedules (
    id BIGSERIAL PRIMARY KEY,
    account_id BIGINT NOT NULL REFERENCES users(id),
    job_name TEXT NOT NULL,
    days TEXT NOT NULL,
    hour INTEGER NOT NULL,
    minute INTEGER NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    last_run TEXT,
    UNIQUE(account_id, job_name)
);

-- Single-row key/value store for cross-process signals (e.g. bumping
-- integrations_version on every integrations write lets the bot
-- multiplexer processes detect a change without re-fetching everything).
CREATE TABLE IF NOT EXISTS meta (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS mandatory_ingredients (
    id BIGSERIAL PRIMARY KEY,
    account_id BIGINT NOT NULL REFERENCES users(id),
    name TEXT NOT NULL,
    UNIQUE(account_id, name)
);

CREATE TABLE IF NOT EXISTS category_keywords (
    id BIGSERIAL PRIMARY KEY,
    account_id BIGINT NOT NULL REFERENCES users(id),
    keyword TEXT NOT NULL,
    category TEXT NOT NULL,
    UNIQUE(account_id, keyword)
);

-- Gamification: one row per account. Points earned by logging spending
-- (kept simple - no separate ledger table since we only need running
-- totals + a daily streak, not a full history of point events).
CREATE TABLE IF NOT EXISTS user_points (
    account_id BIGINT PRIMARY KEY REFERENCES users(id),
    points BIGINT NOT NULL DEFAULT 0,
    streak_days INTEGER NOT NULL DEFAULT 0,
    last_log_date DATE
);
