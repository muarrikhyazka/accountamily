#!/usr/bin/env python3
"""One-shot seeding: makes an admin account + a few dummy demo accounts with
sample data. Run inside the container: `docker compose exec accountamily python3 seed_demo.py`
ponytail: plain script, not a Flask CLI command - fine at this size."""
import os, sqlite3, random, datetime
from werkzeug.security import generate_password_hash

DB_PATH = os.environ.get("DB_PATH", "/app/data/accountamily.sqlite")

DEFAULT_CATEGORIES = {
    "Needs": ["Home Rent", "Utilities", "Food", "Supplies", "Transportation", "Healthcare"],
    "Wants": ["Clothes & Bags", "Entertainment", "Travel", "Jajan"],
    "Savings": ["Emergency Funds", "House"],
}


def get_db():
    conn = sqlite3.connect(DB_PATH)
    conn.row_factory = sqlite3.Row
    return conn


def create_user(conn, email, password, display_name, is_admin=0):
    existing = conn.execute("SELECT id FROM users WHERE email=?", (email,)).fetchone()
    if existing:
        return existing["id"]
    cur = conn.execute(
        "INSERT INTO users (email, password_hash, display_name, is_admin) VALUES (?,?,?,?)",
        (email, generate_password_hash(password), display_name, is_admin))
    return cur.lastrowid


def seed_account_data(conn, account_id):
    for name in ["Main Bank", "Savings Bank"]:
        conn.execute("INSERT OR IGNORE INTO accounts (account_id, name, kind) VALUES (?,?,'bank')", (account_id, name))
    conn.execute("INSERT OR IGNORE INTO income_sources (account_id, name) VALUES (?,?)", (account_id, "Salary"))

    cat_ids = {}
    for main_cat, names in DEFAULT_CATEGORIES.items():
        for name in names:
            cur = conn.execute(
                "INSERT OR IGNORE INTO categories (account_id, name, main_category) VALUES (?,?,?)",
                (account_id, name, main_cat))
            row = conn.execute("SELECT id FROM categories WHERE account_id=? AND name=?", (account_id, name)).fetchone()
            cat_ids[name] = row["id"]

    today = datetime.date.today()
    random.seed(account_id)  # deterministic per account, reruns don't drift
    for days_ago in range(0, 60):
        if random.random() > 0.6:
            continue
        d = today - datetime.timedelta(days=days_ago)
        name = random.choice(list(cat_ids))
        amount = random.randint(10_000, 500_000)
        conn.execute(
            "INSERT OR IGNORE INTO spending (account_id, tx_date, description, category_id, category_name, amount, source, client_id) "
            "VALUES (?,?,?,?,?,?,?,?)",
            (account_id, d.isoformat(), f"Demo {name} expense", cat_ids[name], name, amount, "web", f"seed-{account_id}-{days_ago}"))

    source_row = conn.execute("SELECT id FROM income_sources WHERE account_id=? AND name='Salary'", (account_id,)).fetchone()
    conn.execute(
        "INSERT OR IGNORE INTO budget_income (account_id, year, month, source_id, amount) VALUES (?,?,?,?,?)",
        (account_id, today.year, today.month, source_row["id"], 8_000_000))


def main():
    conn = get_db()
    admin_id = create_user(conn, "admin@accountamily.app", "AdminDemo123!", "Super Admin", is_admin=1)
    demo1_id = create_user(conn, "demo1@accountamily.app", "DemoUser123!", "Demo Family")
    demo2_id = create_user(conn, "demo2@accountamily.app", "DemoUser123!", "Demo Couple")
    conn.commit()

    for uid in (admin_id, demo1_id, demo2_id):
        seed_account_data(conn, uid)
    conn.commit()
    conn.close()
    print("Seeded accounts: admin@accountamily.app / AdminDemo123! (super admin)")
    print("                  demo1@accountamily.app / DemoUser123!")
    print("                  demo2@accountamily.app / DemoUser123!")


if __name__ == "__main__":
    main()
