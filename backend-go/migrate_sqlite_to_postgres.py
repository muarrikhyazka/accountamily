#!/usr/bin/env python3
"""One-shot (re-runnable) migration: accountamily SQLite -> Postgres.

Reads /home/muarrikh/apps/accountamily/data/accountamily.sqlite (read-only)
and writes into the accountamily Postgres database running in the
accountamily-postgres docker container, via `docker exec -i ... psql`
(stdlib-only, no psycopg2/psql client needed on this host).

Safe to re-run: every INSERT is "ON CONFLICT (id) DO NOTHING" so re-running
before the real cutover just picks up new rows without duplicating existing
ones. Sequences are reset to MAX(id) after load so future app-generated
rows don't collide with migrated ids.

Usage: python3 migrate_sqlite_to_postgres.py [--schema-file PATH]
"""
import sqlite3
import subprocess
import sys
import argparse

SQLITE_PATH = "/home/muarrikh/apps/kinkit/data/accountamily.sqlite"
PG_CONTAINER = "kinkit-postgres"
PG_USER = "kinkit"
PG_DB = "kinkit"
DEFAULT_SCHEMA = "/home/muarrikh/apps/kinkit/backend-go/schema.sql"

# (table, columns in insertion order, {col: type} for cols needing conversion)
# type: 'bool' (0/1 -> TRUE/FALSE), 'ts' (TEXT datetime -> TIMESTAMPTZ literal),
# 'date' (TEXT 'YYYY-MM-DD' -> DATE literal), else plain.
TABLES = [
    ("users", ["id", "email", "password_hash", "google_sub", "display_name",
               "is_admin", "is_disabled", "is_verified", "verification_token",
               "currency", "locale", "created_at"],
     {"is_admin": "bool", "is_disabled": "bool", "is_verified": "bool", "created_at": "ts"}),
    ("password_resets", ["id", "user_id", "token", "expires_at", "used", "created_at"],
     {"expires_at": "ts", "used": "bool", "created_at": "ts"}),
    ("accounts", ["id", "account_id", "name", "kind"], {}),
    ("categories", ["id", "account_id", "name", "main_category"], {}),
    ("income_sources", ["id", "account_id", "name"], {}),
    ("budget_income", ["id", "account_id", "year", "month", "source_id", "amount"], {}),
    ("budget_expense", ["id", "account_id", "year", "month", "category_id", "amount"], {}),
    ("spending", ["id", "account_id", "tx_date", "description", "category_id", "category_name",
                  "amount", "checked", "source", "client_id", "created_at"],
     {"tx_date": "date", "checked": "bool", "created_at": "ts"}),
    ("assets", ["id", "account_id", "description", "value", "account_ref", "kind"], {}),
    ("asset_target", ["id", "account_id", "target_value", "last_update"], {}),
    ("savings_list", ["id", "account_id", "year", "month", "name", "amount"], {}),
    ("integrations", ["id", "account_id", "platform", "bot_token", "platform_token", "chat_id",
                       "enabled", "created_at"],
     {"enabled": "bool", "created_at": "ts"}),
    ("reminder_schedules", ["id", "account_id", "job_name", "days", "hour", "minute", "enabled", "last_run"],
     {"enabled": "bool"}),
    ("meta", ["key", "value"], {}),
    ("mandatory_ingredients", ["id", "account_id", "name"], {}),
    ("category_keywords", ["id", "account_id", "keyword", "category"], {}),
]

# tables with a BIGSERIAL id column (need sequence reset); meta uses a text PK.
SERIAL_TABLES = [t for t, cols, _ in TABLES if cols[0] == "id"]


def sql_literal(val, kind):
    if val is None:
        return "NULL"
    if kind == "bool":
        return "TRUE" if val else "FALSE"
    if kind in ("ts", "date"):
        s = str(val).strip()
        return "'" + s.replace("'", "''") + "'::" + ("timestamptz" if kind == "ts" else "date")
    if isinstance(val, int):
        return str(val)
    if isinstance(val, float):
        return repr(val)
    return "'" + str(val).replace("'", "''") + "'"


def run_psql(sql_text):
    proc = subprocess.run(
        ["docker", "exec", "-i", PG_CONTAINER, "psql", "-v", "ON_ERROR_STOP=1",
         "-U", PG_USER, "-d", PG_DB],
        input=sql_text, text=True, capture_output=True,
    )
    if proc.returncode != 0:
        print(proc.stdout)
        print(proc.stderr, file=sys.stderr)
        raise SystemExit(f"psql failed (exit {proc.returncode})")
    return proc.stdout


def ensure_schema(schema_file):
    out = run_psql("SELECT count(*) FROM information_schema.tables WHERE table_schema='public';")
    count_line = [l.strip() for l in out.splitlines() if l.strip().isdigit()]
    n = int(count_line[0]) if count_line else 0
    if n == 0:
        print("Postgres has no tables yet — loading schema.sql ...")
        with open(schema_file) as f:
            schema_sql = f.read()
        run_psql(schema_sql)
    else:
        print(f"Postgres already has {n} tables — skipping schema load.")


def migrate(schema_file):
    con = sqlite3.connect(f"file:{SQLITE_PATH}?mode=ro", uri=True)
    con.row_factory = sqlite3.Row

    ensure_schema(schema_file)

    sqlite_counts = {}
    statements = []
    for table, cols, conv in TABLES:
        cur = con.execute(f"SELECT {', '.join(cols)} FROM {table}")
        rows = cur.fetchall()
        sqlite_counts[table] = len(rows)
        if not rows:
            continue
        conflict_col = "key" if table == "meta" else "id"
        for row in rows:
            values = [sql_literal(row[c], conv.get(c)) for c in cols]
            statements.append(
                f"INSERT INTO {table} ({', '.join(cols)}) VALUES ({', '.join(values)}) "
                f"ON CONFLICT ({conflict_col}) DO NOTHING;"
            )
    con.close()

    sql_text = "BEGIN;\n" + "\n".join(statements) + "\nCOMMIT;\n"
    # reset sequences to current max id so future inserts don't collide
    for table in SERIAL_TABLES:
        sql_text += (
            f"SELECT setval(pg_get_serial_sequence('{table}', 'id'), "
            f"COALESCE((SELECT MAX(id) FROM {table}), 1), "
            f"(SELECT MAX(id) FROM {table}) IS NOT NULL);\n"
        )
    print(run_psql(sql_text))

    # verification
    print("\n=== Row count verification (sqlite vs postgres) ===")
    mismatches = []
    for table, _, _ in TABLES:
        out = run_psql(f"SELECT count(*) FROM {table};")
        pg_n = int([l.strip() for l in out.splitlines() if l.strip().isdigit()][0])
        sq_n = sqlite_counts[table]
        status = "OK" if pg_n == sq_n else "MISMATCH"
        if pg_n != sq_n:
            mismatches.append(table)
        print(f"{table:24s} sqlite={sq_n:<6d} postgres={pg_n:<6d} {status}")

    if mismatches:
        print(f"\nMISMATCHES in: {mismatches}")
        sys.exit(1)
    else:
        print("\nAll tables match. Migration verified OK.")


if __name__ == "__main__":
    ap = argparse.ArgumentParser()
    ap.add_argument("--schema-file", default=DEFAULT_SCHEMA)
    args = ap.parse_args()
    migrate(args.schema_file)
