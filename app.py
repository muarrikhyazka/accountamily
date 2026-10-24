import os, sqlite3, functools
from datetime import datetime, date
from flask import Flask, request, jsonify, session, send_from_directory
from werkzeug.security import generate_password_hash, check_password_hash

DB_PATH = os.environ.get("DB_PATH", "/app/data/accountamily.sqlite")
STATIC_DIR = os.environ.get("STATIC_DIR", "/app/react_dist")
GOOGLE_CLIENT_ID = os.environ.get("GOOGLE_CLIENT_ID", "")

app = Flask(__name__, static_folder=STATIC_DIR, static_url_path="")
app.secret_key = os.environ.get("SECRET_KEY", "dev-secret-change-me")

CURRENCIES = {
    "IDR": {"symbol": "Rp", "locale": "id-ID"},
    "USD": {"symbol": "$", "locale": "en-US"},
    "EUR": {"symbol": "\u20ac", "locale": "de-DE"},
    "GBP": {"symbol": "\u00a3", "locale": "en-GB"},
    "SGD": {"symbol": "S$", "locale": "en-SG"},
    "MYR": {"symbol": "RM", "locale": "ms-MY"},
    "JPY": {"symbol": "\u00a5", "locale": "ja-JP"},
    "AUD": {"symbol": "A$", "locale": "en-AU"},
    "INR": {"symbol": "\u20b9", "locale": "en-IN"},
}

DEFAULT_CATEGORIES = {
    "Needs": ["Home Rent", "Utilities", "Food", "Supplies", "Transportation", "Healthcare",
              "Energy", "Skincare & Beauty", "Services", "Others"],
    "Wants": ["Clothes & Bags", "Application", "Charity", "Travel", "Gifts", "Learning",
              "Jajan", "Internet & Pulsa", "Electricity", "Entertainment", "Business"],
    "Savings": ["Child", "Emergency Funds", "House", "Car"],
}
DEFAULT_INCOME_SOURCES = ["Salary", "Freelance"]
DEFAULT_BANK_ACCOUNTS = ["Bank 1", "Bank 2"]


def get_db():
    conn = sqlite3.connect(DB_PATH)
    conn.row_factory = sqlite3.Row
    conn.execute("PRAGMA foreign_keys = ON")
    return conn


def init_db():
    os.makedirs(os.path.dirname(DB_PATH), exist_ok=True)
    conn = get_db()
    with open(os.path.join(os.path.dirname(__file__), "schema.sql")) as f:
        conn.executescript(f.read())
    conn.commit()
    conn.close()


def seed_defaults(conn, account_id):
    """Starter categories/accounts/income sources for a brand-new signup so
    the app isn't empty on first login. Fully editable/deletable from Setup."""
    for name in DEFAULT_BANK_ACCOUNTS:
        conn.execute("INSERT OR IGNORE INTO accounts (account_id, name, kind) VALUES (?,?,'bank')", (account_id, name))
    for name in DEFAULT_INCOME_SOURCES:
        conn.execute("INSERT OR IGNORE INTO income_sources (account_id, name) VALUES (?,?)", (account_id, name))
    for main_cat, names in DEFAULT_CATEGORIES.items():
        for name in names:
            conn.execute("INSERT OR IGNORE INTO categories (account_id, name, main_category) VALUES (?,?,?)", (account_id, name, main_cat))


def login_required(f):
    @functools.wraps(f)
    def wrapper(*args, **kwargs):
        if "account_id" not in session:
            return jsonify({"error": "unauthorized"}), 401
        return f(*args, **kwargs)
    return wrapper


def admin_required(f):
    @functools.wraps(f)
    def wrapper(*args, **kwargs):
        if "account_id" not in session:
            return jsonify({"error": "unauthorized"}), 401
        conn = get_db()
        row = conn.execute("SELECT is_admin FROM users WHERE id=?", (session["account_id"],)).fetchone()
        conn.close()
        if not row or not row["is_admin"]:
            return jsonify({"error": "forbidden"}), 403
        return f(*args, **kwargs)
    return wrapper


def current_account_id():
    return session["account_id"]


# ---------- Auth ----------
@app.post("/api/signup")
def signup():
    d = request.get_json(force=True)
    email = (d.get("email") or "").strip().lower()
    password = d.get("password") or ""
    display_name = (d.get("display_name") or "").strip() or email.split("@")[0]
    if not email or "@" not in email or len(password) < 8:
        return jsonify({"error": "valid email and password (8+ chars) required"}), 400
    conn = get_db()
    try:
        cur = conn.execute(
            "INSERT INTO users (email, password_hash, display_name) VALUES (?,?,?)",
            (email, generate_password_hash(password), display_name))
        conn.commit()
        account_id = cur.lastrowid
    except sqlite3.IntegrityError:
        conn.close()
        return jsonify({"error": "email already registered"}), 409
    seed_defaults(conn, account_id)
    conn.commit()
    conn.close()
    session["account_id"] = account_id
    session["display"] = display_name
    return jsonify({"ok": True, "display_name": display_name})


@app.post("/api/login")
def login():
    data = request.get_json(force=True)
    email = (data.get("email") or data.get("username") or "").strip().lower()
    password = data.get("password") or ""
    conn = get_db()
    row = conn.execute("SELECT * FROM users WHERE email=?", (email,)).fetchone()
    conn.close()
    if row and row["password_hash"] and check_password_hash(row["password_hash"], password):
        if row["is_disabled"]:
            return jsonify({"error": "account disabled"}), 403
        session["account_id"] = row["id"]
        session["display"] = row["display_name"] or email
        return jsonify({"ok": True, "display_name": session["display"]})
    return jsonify({"error": "invalid credentials"}), 401


@app.post("/api/login/google")
def login_google():
    """Verify a Google Identity Services ID token (sent by the frontend's
    Google Sign-In button) and log in / auto-create the account. GIS does the
    OAuth dance client-side; we just verify the token's signature+audience."""
    if not GOOGLE_CLIENT_ID:
        return jsonify({"error": "Google login not configured"}), 503
    token = (request.get_json(force=True) or {}).get("credential", "")
    try:
        from google.oauth2 import id_token
        from google.auth.transport import requests as google_requests
        info = id_token.verify_oauth2_token(token, google_requests.Request(), GOOGLE_CLIENT_ID)
    except Exception:
        return jsonify({"error": "invalid Google token"}), 401

    sub = info["sub"]
    email = info.get("email", "").lower()
    name = info.get("name") or email.split("@")[0]
    conn = get_db()
    row = conn.execute("SELECT * FROM users WHERE google_sub=? OR email=?", (sub, email)).fetchone()
    if row:
        account_id = row["id"]
        if not row["google_sub"]:
            conn.execute("UPDATE users SET google_sub=? WHERE id=?", (sub, account_id))
            conn.commit()
        display_name = row["display_name"] or name
    else:
        cur = conn.execute(
            "INSERT INTO users (email, google_sub, display_name) VALUES (?,?,?)",
            (email, sub, name))
        conn.commit()
        account_id = cur.lastrowid
        display_name = name
        seed_defaults(conn, account_id)
        conn.commit()
    conn.close()
    session["account_id"] = account_id
    session["display"] = display_name
    return jsonify({"ok": True, "display_name": display_name})


@app.post("/api/logout")
def logout():
    session.clear()
    return jsonify({"ok": True})


@app.get("/api/me")
def me():
    if "account_id" not in session:
        return jsonify({"authenticated": False})
    conn = get_db()
    row = conn.execute("SELECT email, currency, locale, is_admin FROM users WHERE id=?", (session["account_id"],)).fetchone()
    conn.close()
    return jsonify({
        "authenticated": True,
        "username": row["email"] if row else None,
        "display_name": session.get("display"),
        "currency": row["currency"] if row else "IDR",
        "locale": row["locale"] if row else "id-ID",
        "is_admin": bool(row["is_admin"]) if row else False,
    })


@app.patch("/api/me/settings")
@login_required
def update_settings():
    d = request.get_json(force=True)
    fields, params = [], []
    if d.get("display_name"):
        fields.append("display_name=?")
        params.append(d["display_name"].strip())
    if d.get("currency") in CURRENCIES:
        fields.append("currency=?")
        params.append(d["currency"])
        fields.append("locale=?")
        params.append(CURRENCIES[d["currency"]]["locale"])
    if not fields:
        return jsonify({"ok": True})
    conn = get_db()
    params.append(current_account_id())
    conn.execute(f"UPDATE users SET {','.join(fields)} WHERE id=?", params)
    conn.commit()
    conn.close()
    if d.get("display_name"):
        session["display"] = d["display_name"].strip()
    return jsonify({"ok": True})


@app.get("/api/currencies")
def list_currencies():
    return jsonify(CURRENCIES)


@app.get("/api/ping")
def ping():
    return jsonify({"ok": True})


# ---------- Setup / master lists ----------
@app.get("/api/setup")
@login_required
def get_setup():
    aid = current_account_id()
    conn = get_db()
    accounts = [dict(r) for r in conn.execute("SELECT * FROM accounts WHERE account_id=? ORDER BY id", (aid,))]
    categories = [dict(r) for r in conn.execute("SELECT * FROM categories WHERE account_id=? ORDER BY id", (aid,))]
    income_sources = [dict(r) for r in conn.execute("SELECT * FROM income_sources WHERE account_id=? ORDER BY id", (aid,))]
    conn.close()
    return jsonify({"accounts": accounts, "categories": categories, "income_sources": income_sources})


# ---------- Categories CRUD ----------
@app.post("/api/categories")
@login_required
def add_category():
    aid = current_account_id()
    d = request.get_json(force=True)
    name = (d.get("name") or "").strip()
    main_category = d.get("main_category")
    if not name or main_category not in ("Needs", "Wants", "Savings"):
        return jsonify({"error": "name and valid main_category required"}), 400
    conn = get_db()
    try:
        cur = conn.execute("INSERT INTO categories (account_id, name, main_category) VALUES (?,?,?)", (aid, name, main_category))
        conn.commit()
        new_id = cur.lastrowid
    except sqlite3.IntegrityError:
        conn.close()
        return jsonify({"error": "category already exists"}), 409
    conn.close()
    return jsonify({"ok": True, "id": new_id})


@app.patch("/api/categories/<int:cid>")
@login_required
def update_category(cid):
    aid = current_account_id()
    d = request.get_json(force=True)
    fields, params = [], []
    if d.get("name"):
        fields.append("name=?")
        params.append(d["name"].strip())
    if d.get("main_category") in ("Needs", "Wants", "Savings"):
        fields.append("main_category=?")
        params.append(d["main_category"])
    if not fields:
        return jsonify({"ok": True})
    conn = get_db()
    params += [cid, aid]
    try:
        conn.execute(f"UPDATE categories SET {','.join(fields)} WHERE id=? AND account_id=?", params)
        conn.commit()
    except sqlite3.IntegrityError:
        conn.close()
        return jsonify({"error": "name already used"}), 409
    conn.close()
    return jsonify({"ok": True})


@app.delete("/api/categories/<int:cid>")
@login_required
def delete_category(cid):
    aid = current_account_id()
    conn = get_db()
    spending_count = conn.execute("SELECT COUNT(*) c FROM spending WHERE category_id=? AND account_id=?", (cid, aid)).fetchone()["c"]
    budget_count = conn.execute("SELECT COUNT(*) c FROM budget_expense WHERE category_id=? AND account_id=?", (cid, aid)).fetchone()["c"]
    force = request.args.get("force") == "1"
    if (spending_count or budget_count) and not force:
        conn.close()
        return jsonify({"error": "category in use", "spending_count": spending_count, "budget_count": budget_count}), 409
    conn.execute("UPDATE spending SET category_id=NULL WHERE category_id=? AND account_id=?", (cid, aid))
    conn.execute("DELETE FROM budget_expense WHERE category_id=? AND account_id=?", (cid, aid))
    conn.execute("DELETE FROM categories WHERE id=? AND account_id=?", (cid, aid))
    conn.commit()
    conn.close()
    return jsonify({"ok": True})


# ---------- Accounts CRUD (bank + asset accounts) ----------
@app.post("/api/accounts")
@login_required
def add_account():
    aid = current_account_id()
    d = request.get_json(force=True)
    name = (d.get("name") or "").strip()
    kind = d.get("kind", "bank")
    if not name or kind not in ("bank", "asset"):
        return jsonify({"error": "name and valid kind required"}), 400
    conn = get_db()
    try:
        cur = conn.execute("INSERT INTO accounts (account_id, name, kind) VALUES (?,?,?)", (aid, name, kind))
        conn.commit()
        new_id = cur.lastrowid
    except sqlite3.IntegrityError:
        conn.close()
        return jsonify({"error": "account already exists"}), 409
    conn.close()
    return jsonify({"ok": True, "id": new_id})


@app.patch("/api/accounts/<int:acc_id>")
@login_required
def update_account(acc_id):
    aid = current_account_id()
    d = request.get_json(force=True)
    if not d.get("name"):
        return jsonify({"ok": True})
    conn = get_db()
    try:
        conn.execute("UPDATE accounts SET name=? WHERE id=? AND account_id=?", (d["name"].strip(), acc_id, aid))
        conn.commit()
    except sqlite3.IntegrityError:
        conn.close()
        return jsonify({"error": "name already used"}), 409
    conn.close()
    return jsonify({"ok": True})


@app.delete("/api/accounts/<int:acc_id>")
@login_required
def delete_account(acc_id):
    aid = current_account_id()
    conn = get_db()
    used = conn.execute("SELECT COUNT(*) c FROM assets WHERE account_ref=? AND account_id=?", (acc_id, aid)).fetchone()["c"]
    if used:
        conn.close()
        return jsonify({"error": "account in use", "count": used}), 409
    conn.execute("DELETE FROM accounts WHERE id=? AND account_id=?", (acc_id, aid))
    conn.commit()
    conn.close()
    return jsonify({"ok": True})


# ---------- Income sources CRUD ----------
@app.post("/api/income-sources")
@login_required
def add_income_source():
    aid = current_account_id()
    d = request.get_json(force=True)
    name = (d.get("name") or "").strip()
    if not name:
        return jsonify({"error": "name required"}), 400
    conn = get_db()
    try:
        cur = conn.execute("INSERT INTO income_sources (account_id, name) VALUES (?,?)", (aid, name))
        conn.commit()
        new_id = cur.lastrowid
    except sqlite3.IntegrityError:
        conn.close()
        return jsonify({"error": "income source already exists"}), 409
    conn.close()
    return jsonify({"ok": True, "id": new_id})


@app.patch("/api/income-sources/<int:sid>")
@login_required
def update_income_source(sid):
    aid = current_account_id()
    d = request.get_json(force=True)
    if not d.get("name"):
        return jsonify({"ok": True})
    conn = get_db()
    try:
        conn.execute("UPDATE income_sources SET name=? WHERE id=? AND account_id=?", (d["name"].strip(), sid, aid))
        conn.commit()
    except sqlite3.IntegrityError:
        conn.close()
        return jsonify({"error": "name already used"}), 409
    conn.close()
    return jsonify({"ok": True})


@app.delete("/api/income-sources/<int:sid>")
@login_required
def delete_income_source(sid):
    aid = current_account_id()
    conn = get_db()
    used = conn.execute("SELECT COUNT(*) c FROM budget_income WHERE source_id=? AND account_id=?", (sid, aid)).fetchone()["c"]
    force = request.args.get("force") == "1"
    if used and not force:
        conn.close()
        return jsonify({"error": "income source in use", "count": used}), 409
    conn.execute("DELETE FROM budget_income WHERE source_id=? AND account_id=?", (sid, aid))
    conn.execute("DELETE FROM income_sources WHERE id=? AND account_id=?", (sid, aid))
    conn.commit()
    conn.close()
    return jsonify({"ok": True})


def get_value_max():
    v = request.args.get("value_max")
    return {"10jt": 10_000_000, "100jt": 100_000_000}.get(v)


def value_filter(alias="amount"):
    ceiling = get_value_max()
    if ceiling is None:
        return "", []
    return f" AND {alias} <= ?", [ceiling]


# ---------- Trend ----------
@app.get("/api/trend")
@login_required
def trend():
    aid = current_account_id()
    year = request.args.get("year", type=int, default=datetime.now().year)
    month = request.args.get("month", type=int, default=datetime.now().month)
    count = request.args.get("count", type=int, default=6)
    conn = get_db()
    out = []
    y, m = year, month
    vfrag, vparams = value_filter()
    for _ in range(count):
        income = conn.execute(
            "SELECT COALESCE(SUM(amount),0) t FROM budget_income WHERE account_id=? AND year=? AND month=?", (aid, y, m)).fetchone()["t"]
        expense = conn.execute(
            f"SELECT COALESCE(SUM(amount),0) t FROM spending WHERE account_id=? AND strftime('%Y',tx_date)=? AND strftime('%m',tx_date)=?{vfrag}",
            [aid, str(y), f"{m:02d}"] + vparams).fetchone()["t"]
        out.append({"year": y, "month": m, "income": income, "expense": expense})
        m -= 1
        if m == 0:
            m = 12
            y -= 1
    out.reverse()
    conn.close()
    return jsonify(out)


@app.get("/api/years")
@login_required
def years_overview():
    aid = current_account_id()
    conn = get_db()
    vfrag, vparams = value_filter()
    yrs = [r["y"] for r in conn.execute(
        "SELECT DISTINCT CAST(strftime('%Y',tx_date) AS INTEGER) y FROM spending WHERE account_id=? "
        "UNION SELECT DISTINCT year FROM budget_income WHERE account_id=? "
        "ORDER BY y", (aid, aid))]
    out = []
    for y in yrs:
        income = conn.execute(
            "SELECT COALESCE(SUM(amount),0) t FROM budget_income WHERE account_id=? AND year=?", (aid, int(y))).fetchone()["t"]
        expense = conn.execute(
            f"SELECT COALESCE(SUM(amount),0) t FROM spending WHERE account_id=? AND strftime('%Y',tx_date)=?{vfrag}",
            [aid, str(y)] + vparams).fetchone()["t"]
        out.append({"year": int(y), "income": income, "expense": expense})
    conn.close()
    return jsonify(out)


@app.get("/api/years/by-category")
@login_required
def years_by_category():
    aid = current_account_id()
    conn = get_db()
    vfrag, vparams = value_filter("sp.amount")
    rows = conn.execute(
        f"""SELECT CAST(strftime('%Y',tx_date) AS INTEGER) as year,
                  COALESCE(c.name, sp.category_name, 'Other') as category_name,
                  SUM(sp.amount) as total
           FROM spending sp LEFT JOIN categories c ON c.id=sp.category_id
           WHERE sp.account_id=?{vfrag}
           GROUP BY year, category_name""", [aid] + vparams).fetchall()
    conn.close()
    years = sorted({r["year"] for r in rows})
    cats = {}
    for r in rows:
        cats.setdefault(r["category_name"], {}).__setitem__(r["year"], r["total"])
    out = [{"category_name": name, **{str(y): vals.get(y, 0) for y in years}}
           for name, vals in sorted(cats.items(), key=lambda kv: -sum(kv[1].values()))]
    return jsonify({"years": years, "rows": out})


@app.get("/api/years/monthly")
@login_required
def years_monthly():
    aid = current_account_id()
    conn = get_db()
    vfrag, vparams = value_filter()
    rows = conn.execute(
        f"""SELECT CAST(strftime('%Y',tx_date) AS INTEGER) as year,
                  CAST(strftime('%m',tx_date) AS INTEGER) as month,
                  SUM(amount) as total
           FROM spending WHERE account_id=?{vfrag} GROUP BY year, month""", [aid] + vparams).fetchall()
    conn.close()
    years = sorted({r["year"] for r in rows})
    by_year = {y: [0] * 12 for y in years}
    for r in rows:
        by_year[r["year"]][r["month"] - 1] = r["total"]
    return jsonify({"years": years, "by_year": by_year})


@app.get("/api/years/day-of-month-avg")
@login_required
def years_day_of_month_avg():
    aid = current_account_id()
    conn = get_db()
    ceiling = get_value_max()
    rows = conn.execute("SELECT tx_date, amount FROM spending WHERE account_id=?", (aid,)).fetchall()
    conn.close()

    by_day = {d: 0 for d in range(1, 32)}
    months_with_day = {d: set() for d in range(1, 32)}
    for r in rows:
        if ceiling is not None and r["amount"] > ceiling:
            continue
        d = date.fromisoformat(r["tx_date"])
        by_day[d.day] += r["amount"]
        months_with_day[d.day].add((d.year, d.month))

    out = [{"day": d, "avg": (by_day[d] / len(months_with_day[d])) if months_with_day[d] else 0}
           for d in range(1, 32) if months_with_day[d]]
    return jsonify(out)


@app.get("/api/years/datemonth-avg")
@login_required
def years_datemonth_avg():
    aid = current_account_id()
    conn = get_db()
    ceiling = get_value_max()
    rows = conn.execute("SELECT tx_date, amount FROM spending WHERE account_id=?", (aid,)).fetchall()
    conn.close()

    by_md = {}
    years_with_md = {}
    for r in rows:
        if ceiling is not None and r["amount"] > ceiling:
            continue
        d = date.fromisoformat(r["tx_date"])
        key = (d.month, d.day)
        by_md[key] = by_md.get(key, 0) + r["amount"]
        years_with_md.setdefault(key, set()).add(d.year)

    out = []
    for month in range(1, 13):
        days_in_month = 29 if month == 2 else (30 if month in (4, 6, 9, 11) else 31)
        for day in range(1, days_in_month + 1):
            key = (month, day)
            if key in years_with_md:
                out.append({
                    "month": month, "day": day,
                    "label": f"{day} {MONTH_ABBR[month - 1]}",
                    "avg": by_md[key] / len(years_with_md[key]),
                })
    return jsonify(out)


MONTH_ABBR = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"]


@app.get("/api/report/weekly/<int:year>/<int:month>")
@login_required
def weekly_report(year, month):
    aid = current_account_id()
    conn = get_db()
    vfrag, vparams = value_filter("sp.amount")
    rows = conn.execute(
        f"SELECT tx_date, amount, COALESCE(c.name, sp.category_name, 'Other') as category_name "
        f"FROM spending sp LEFT JOIN categories c ON c.id=sp.category_id "
        f"WHERE sp.account_id=? AND strftime('%Y',tx_date)=? AND strftime('%m',tx_date)=?{vfrag}",
        [aid, str(year), f"{month:02d}"] + vparams).fetchall()
    conn.close()
    weeks = {}
    for r in rows:
        day = int(r["tx_date"][8:10])
        wk = (day - 1) // 7 + 1
        weeks.setdefault(wk, {"week": wk, "total": 0, "by_category": {}})
        weeks[wk]["total"] += r["amount"]
        weeks[wk]["by_category"][r["category_name"]] = weeks[wk]["by_category"].get(r["category_name"], 0) + r["amount"]
    out = []
    for wk in sorted(weeks):
        w = weeks[wk]
        out.append({
            "week": wk, "total": w["total"],
            "by_category": [{"category_name": k, "total": v} for k, v in sorted(w["by_category"].items(), key=lambda x: -x[1])],
        })
    return jsonify(out)


# ---------- Budgeting ----------
@app.get("/api/budgeting/<int:year>")
@login_required
def get_budgeting(year):
    aid = current_account_id()
    conn = get_db()
    income = [dict(r) for r in conn.execute(
        "SELECT bi.*, s.name as source_name FROM budget_income bi "
        "JOIN income_sources s ON s.id = bi.source_id WHERE bi.account_id=? AND bi.year=?", (aid, year))]
    expense = [dict(r) for r in conn.execute(
        "SELECT be.*, c.name as category_name, c.main_category FROM budget_expense be "
        "JOIN categories c ON c.id = be.category_id WHERE be.account_id=? AND be.year=?", (aid, year))]
    savings = [dict(r) for r in conn.execute("SELECT * FROM savings_list WHERE account_id=? AND year=?", (aid, year))]

    income_history_rows = conn.execute(
        "SELECT source_id, year, SUM(amount) as total FROM budget_income WHERE account_id=? AND year<? GROUP BY source_id, year", (aid, year)).fetchall()
    expense_history_rows = conn.execute(
        "SELECT category_id, year, SUM(amount) as total FROM budget_expense WHERE account_id=? AND year<? GROUP BY category_id, year", (aid, year)).fetchall()
    conn.close()

    def build_history(rows, id_field):
        out = {}
        for r in rows:
            out.setdefault(r[id_field], {})[r["year"]] = r["total"]
        return out

    return jsonify({
        "income": income, "expense": expense, "savings": savings,
        "income_history": build_history(income_history_rows, "source_id"),
        "expense_history": build_history(expense_history_rows, "category_id"),
    })


@app.post("/api/budgeting/income")
@login_required
def set_budget_income():
    aid = current_account_id()
    d = request.get_json(force=True)
    conn = get_db()
    conn.execute(
        "INSERT INTO budget_income (account_id, year, month, source_id, amount) VALUES (?,?,?,?,?) "
        "ON CONFLICT(account_id, year, month, source_id) DO UPDATE SET amount=excluded.amount",
        (aid, d["year"], d["month"], d["source_id"], d["amount"]))
    conn.commit()
    conn.close()
    return jsonify({"ok": True})


@app.post("/api/budgeting/expense")
@login_required
def set_budget_expense():
    aid = current_account_id()
    d = request.get_json(force=True)
    conn = get_db()
    conn.execute(
        "INSERT INTO budget_expense (account_id, year, month, category_id, amount) VALUES (?,?,?,?,?) "
        "ON CONFLICT(account_id, year, month, category_id) DO UPDATE SET amount=excluded.amount",
        (aid, d["year"], d["month"], d["category_id"], d["amount"]))
    conn.commit()
    conn.close()
    return jsonify({"ok": True})


# ---------- Spending ----------
@app.get("/api/spending")
@login_required
def list_spending():
    aid = current_account_id()
    year = request.args.get("year", type=int)
    month = request.args.get("month", type=int)
    page = request.args.get("page", type=int, default=1)
    per_page = min(request.args.get("per_page", type=int, default=20), 500)
    search = request.args.get("search", type=str, default="").strip()
    category = request.args.get("category", type=str, default="").strip()
    sort = request.args.get("sort", type=str, default="tx_date")
    order = request.args.get("order", type=str, default="desc")

    conn = get_db()
    base = "FROM spending sp LEFT JOIN categories c ON c.id = sp.category_id WHERE sp.account_id=?"
    params = [aid]
    if year:
        base += " AND strftime('%Y', sp.tx_date) = ?"
        params.append(str(year))
    if month:
        base += " AND strftime('%m', sp.tx_date) = ?"
        params.append(f"{month:02d}")
    if search:
        base += " AND sp.description LIKE ?"
        params.append(f"%{search}%")
    if category:
        base += " AND sp.category_name = ?"
        params.append(category)
    vfrag, vparams = value_filter("sp.amount")
    base += vfrag
    params += vparams

    sort_cols = {"tx_date": "sp.tx_date", "description": "sp.description", "category_name": "sp.category_name", "amount": "sp.amount"}
    sort_col = sort_cols.get(sort, "sp.tx_date")
    order_dir = "ASC" if order.lower() == "asc" else "DESC"

    total = conn.execute(f"SELECT COUNT(*) c {base}", params).fetchone()["c"]
    q = f"SELECT sp.*, c.name as category_master {base} ORDER BY {sort_col} {order_dir}, sp.id DESC LIMIT ? OFFSET ?"
    rows = [dict(r) for r in conn.execute(q, params + [per_page, (page - 1) * per_page])]
    conn.close()
    return jsonify({"rows": rows, "total": total, "page": page, "per_page": per_page})


@app.post("/api/spending")
@login_required
def add_spending():
    aid = current_account_id()
    d = request.get_json(force=True)
    conn = get_db()
    cat_id = None
    if d.get("category"):
        row = conn.execute("SELECT id FROM categories WHERE name=? AND account_id=?", (d["category"], aid)).fetchone()
        cat_id = row["id"] if row else None
    client_id = d.get("client_id")
    if client_id:
        existing = conn.execute("SELECT id FROM spending WHERE client_id=? AND account_id=?", (client_id, aid)).fetchone()
        if existing:
            conn.close()
            return jsonify({"ok": True, "id": existing["id"], "duplicate": True})
    cur = conn.execute(
        "INSERT INTO spending (account_id, tx_date, description, category_id, category_name, amount, source, client_id) VALUES (?,?,?,?,?,?,?,?)",
        (aid, d["tx_date"], d["description"], cat_id, d.get("category"), d["amount"], d.get("source", "web"), client_id))
    conn.commit()
    new_id = cur.lastrowid
    conn.close()
    return jsonify({"ok": True, "id": new_id})


@app.delete("/api/spending/<int:sid>")
@login_required
def delete_spending(sid):
    aid = current_account_id()
    conn = get_db()
    conn.execute("DELETE FROM spending WHERE id=? AND account_id=?", (sid, aid))
    conn.commit()
    conn.close()
    return jsonify({"ok": True})


@app.patch("/api/spending/<int:sid>")
@login_required
def update_spending(sid):
    aid = current_account_id()
    d = request.get_json(force=True)
    fields, params = [], []
    for k in ("tx_date", "description", "amount", "checked"):
        if k in d:
            fields.append(f"{k}=?")
            params.append(d[k])
    if d.get("category"):
        conn = get_db()
        row = conn.execute("SELECT id FROM categories WHERE name=? AND account_id=?", (d["category"], aid)).fetchone()
        fields.append("category_id=?")
        params.append(row["id"] if row else None)
        fields.append("category_name=?")
        params.append(d["category"])
        conn.close()
    if not fields:
        return jsonify({"ok": True})
    conn = get_db()
    params += [sid, aid]
    conn.execute(f"UPDATE spending SET {','.join(fields)} WHERE id=? AND account_id=?", params)
    conn.commit()
    conn.close()
    return jsonify({"ok": True})


# ---------- Assets ----------
@app.get("/api/assets")
@login_required
def get_assets():
    aid = current_account_id()
    conn = get_db()
    assets = [dict(r) for r in conn.execute(
        "SELECT a.*, ac.name as account_name FROM assets a LEFT JOIN accounts ac ON ac.id=a.account_ref "
        "WHERE a.account_id=? ORDER BY a.id", (aid,))]
    target = conn.execute("SELECT * FROM asset_target WHERE account_id=? ORDER BY id DESC LIMIT 1", (aid,)).fetchone()
    conn.close()
    return jsonify({"assets": assets, "target": dict(target) if target else None})


@app.post("/api/assets")
@login_required
def add_asset():
    aid = current_account_id()
    d = request.get_json(force=True)
    conn = get_db()
    conn.execute(
        "INSERT INTO assets (account_id, description, value, account_ref, kind) VALUES (?,?,?,?,?)",
        (aid, d["description"], d["value"], d.get("account_id"), d.get("kind", "liquid")))
    conn.commit()
    conn.close()
    return jsonify({"ok": True})


@app.patch("/api/assets/<int:asset_id>")
@login_required
def update_asset(asset_id):
    aid = current_account_id()
    d = request.get_json(force=True)
    conn = get_db()
    conn.execute("UPDATE assets SET value=? WHERE id=? AND account_id=?", (d["value"], asset_id, aid))
    conn.commit()
    conn.close()
    return jsonify({"ok": True})


@app.delete("/api/assets/<int:asset_id>")
@login_required
def delete_asset(asset_id):
    aid = current_account_id()
    conn = get_db()
    conn.execute("DELETE FROM assets WHERE id=? AND account_id=?", (asset_id, aid))
    conn.commit()
    conn.close()
    return jsonify({"ok": True})


@app.post("/api/assets/target")
@login_required
def set_asset_target():
    aid = current_account_id()
    d = request.get_json(force=True)
    conn = get_db()
    conn.execute(
        "INSERT INTO asset_target (account_id, target_value, last_update) VALUES (?,?,?)",
        (aid, d["target_value"], date.today().isoformat()))
    conn.commit()
    conn.close()
    return jsonify({"ok": True})


# ---------- Report / Summary ----------
@app.get("/api/report/<int:year>/<int:month>")
@login_required
def report(year, month):
    aid = current_account_id()
    conn = get_db()
    vfrag, vparams = value_filter("sp.amount")
    income_total = conn.execute(
        "SELECT COALESCE(SUM(amount),0) t FROM budget_income WHERE account_id=? AND year=? AND month=?", (aid, year, month)).fetchone()["t"]
    expense_budget = conn.execute(
        "SELECT COALESCE(SUM(amount),0) t FROM budget_expense WHERE account_id=? AND year=? AND month=?", (aid, year, month)).fetchone()["t"]
    savings_total = conn.execute(
        "SELECT COALESCE(SUM(amount),0) t FROM savings_list WHERE account_id=? AND year=? AND month=?", (aid, year, month)).fetchone()["t"]

    realization = [dict(r) for r in conn.execute(
        f"""SELECT c.name as category_name, c.main_category,
                  COALESCE(be.amount,0) as allocation,
                  COALESCE((SELECT SUM(sp.amount) FROM spending sp
                            WHERE sp.category_id=c.id AND sp.account_id=?
                            AND strftime('%Y',sp.tx_date)=? AND strftime('%m',sp.tx_date)=?{vfrag}),0) as realization
           FROM categories c
           LEFT JOIN budget_expense be ON be.category_id=c.id AND be.account_id=? AND be.year=? AND be.month=?
           WHERE c.account_id=?
           ORDER BY c.id""",
        [aid, str(year), f"{month:02d}"] + vparams + [aid, year, month, aid])]
    conn.close()
    left_for_budgeting = income_total - expense_budget - savings_total
    return jsonify({
        "total_income": income_total,
        "budgeted_expenses": expense_budget,
        "savings": savings_total,
        "left_for_budgeting": left_for_budgeting,
        "expenses_list": realization,
    })


@app.get("/api/report/year/<int:year>")
@login_required
def report_year(year):
    aid = current_account_id()
    conn = get_db()
    vfrag, vparams = value_filter("sp.amount")
    rows = conn.execute(
        f"""SELECT COALESCE(c.name, sp.category_name, 'Other') as category_name,
                  SUM(sp.amount) as total
           FROM spending sp LEFT JOIN categories c ON c.id=sp.category_id
           WHERE sp.account_id=? AND strftime('%Y',sp.tx_date)=?{vfrag}
           GROUP BY category_name ORDER BY total DESC""",
        [aid, str(year)] + vparams).fetchall()
    conn.close()
    return jsonify([dict(r) for r in rows])


@app.get("/api/report/month-compare/<int:year>/<int:month>")
@login_required
def report_month_compare(year, month):
    aid = current_account_id()
    conn = get_db()
    vfrag, vparams = value_filter()

    def expense_for(y, m):
        return conn.execute(
            f"SELECT COALESCE(SUM(amount),0) t FROM spending WHERE account_id=? AND strftime('%Y',tx_date)=? AND strftime('%m',tx_date)=?{vfrag}",
            [aid, str(y), f"{m:02d}"] + vparams).fetchone()["t"]

    current = expense_for(year, month)
    prev_y, prev_m = (year - 1, 12) if month == 1 else (year, month - 1)
    previous = expense_for(prev_y, prev_m)
    last_year = expense_for(year - 1, month)
    conn.close()
    return jsonify({"current": current, "previous_month": previous, "same_month_last_year": last_year})


@app.get("/api/patterns/year/<int:year>")
@login_required
def patterns_year(year):
    aid = current_account_id()
    conn = get_db()
    ceiling = get_value_max()
    rows = conn.execute(
        "SELECT tx_date, amount FROM spending WHERE account_id=? AND strftime('%Y',tx_date)=?", (aid, str(year))).fetchall()
    conn.close()
    if ceiling is not None:
        rows = [r for r in rows if r["amount"] <= ceiling]

    by_date = {}
    by_week = {}
    by_weekday = {}
    by_week_in_month = {}
    dates_seen = set()
    for r in rows:
        d = date.fromisoformat(r["tx_date"])
        by_date.setdefault(d.day, 0)
        by_date[d.day] += r["amount"]
        wk = d.isocalendar()[1]
        by_week.setdefault(wk, 0)
        by_week[wk] += r["amount"]
        by_weekday.setdefault(d.weekday(), 0)
        by_weekday[d.weekday()] += r["amount"]
        wim = (d.day - 1) // 7 + 1
        by_week_in_month.setdefault(wim, 0)
        by_week_in_month[wim] += r["amount"]
        dates_seen.add(d)

    months_present = sorted({(d.year, d.month) for d in dates_seen})
    day_counts = {dom: 0 for dom in range(1, 32)}
    for (yy, mm) in months_present:
        import calendar
        days_in_month = calendar.monthrange(yy, mm)[1]
        for dom in range(1, days_in_month + 1):
            day_counts[dom] += 1
    date_pattern = [{"day": dom, "avg": (by_date.get(dom, 0) / day_counts[dom]) if day_counts[dom] else 0}
                     for dom in range(1, 32) if day_counts[dom] > 0]

    all_weeks = sorted(by_week.keys())
    week_pattern = [{"week": wk, "avg": by_week[wk]} for wk in all_weeks]

    d0 = date(year, 1, 1)
    d1 = date(year, 12, 31)
    occ = {wd: 0 for wd in range(7)}
    cur = d0
    while cur <= d1:
        occ[cur.weekday()] += 1
        cur = date.fromordinal(cur.toordinal() + 1)
    weekday_pattern = [{"weekday": wd, "avg": (by_weekday.get(wd, 0) / occ[wd]) if occ[wd] else 0} for wd in range(7)]

    months_count = len(months_present) or 1
    week_in_month_pattern = [{"week": wim, "avg": by_week_in_month.get(wim, 0) / months_count} for wim in range(1, 6)]

    return jsonify({
        "by_date": date_pattern,
        "by_week": week_pattern,
        "by_weekday": weekday_pattern,
        "by_week_in_month": week_in_month_pattern,
    })


@app.get("/api/summary")
@login_required
def summary():
    aid = current_account_id()
    year = request.args.get("year", type=int, default=datetime.now().year)
    month = request.args.get("month", type=int, default=datetime.now().month)
    conn = get_db()
    by_main = [dict(r) for r in conn.execute(
        """SELECT c.main_category, COALESCE(SUM(sp.amount),0) as total
           FROM spending sp JOIN categories c ON c.id=sp.category_id
           WHERE sp.account_id=? AND strftime('%Y',sp.tx_date)=? AND strftime('%m',sp.tx_date)=?
           GROUP BY c.main_category""",
        (aid, str(year), f"{month:02d}"))]
    by_category = [dict(r) for r in conn.execute(
        """SELECT COALESCE(c.name, sp.category_name, 'Other') as category_name,
                  COALESCE(SUM(sp.amount),0) as total
           FROM spending sp LEFT JOIN categories c ON c.id=sp.category_id
           WHERE sp.account_id=? AND strftime('%Y',sp.tx_date)=? AND strftime('%m',sp.tx_date)=?
           GROUP BY category_name ORDER BY total DESC""",
        (aid, str(year), f"{month:02d}"))]
    accounts = [dict(r) for r in conn.execute("SELECT * FROM accounts WHERE account_id=? AND kind='bank' ORDER BY id", (aid,))]
    conn.close()
    return jsonify({"by_main_category": by_main, "by_category": by_category, "accounts": accounts})


# ---------- Integrations (web-editable bot config) ----------
@app.get("/api/integrations")
@login_required
def list_integrations():
    aid = current_account_id()
    conn = get_db()
    rows = [dict(r) for r in conn.execute(
        "SELECT id, platform, chat_id, enabled, bot_token, platform_token, created_at FROM integrations WHERE account_id=? ORDER BY id", (aid,))]
    conn.close()
    return jsonify(rows)


@app.post("/api/integrations")
@login_required
def add_integration():
    import secrets
    aid = current_account_id()
    d = request.get_json(force=True)
    platform = d.get("platform")
    if platform not in ("telegram", "discord"):
        return jsonify({"error": "platform must be telegram or discord"}), 400
    bot_token = secrets.token_urlsafe(24)
    conn = get_db()
    try:
        cur = conn.execute(
            "INSERT INTO integrations (account_id, platform, bot_token, platform_token, chat_id) VALUES (?,?,?,?,?)",
            (aid, platform, bot_token, d.get("platform_token"), d.get("chat_id")))
        conn.commit()
        new_id = cur.lastrowid
    except sqlite3.IntegrityError:
        conn.close()
        return jsonify({"error": "integration for this platform already exists"}), 409
    conn.close()
    return jsonify({"ok": True, "id": new_id, "bot_token": bot_token})


@app.patch("/api/integrations/<int:iid>")
@login_required
def update_integration(iid):
    aid = current_account_id()
    d = request.get_json(force=True)
    fields, params = [], []
    for k in ("platform_token", "chat_id"):
        if k in d:
            fields.append(f"{k}=?")
            params.append(d[k])
    if "enabled" in d:
        fields.append("enabled=?")
        params.append(1 if d["enabled"] else 0)
    if not fields:
        return jsonify({"ok": True})
    conn = get_db()
    params += [iid, aid]
    conn.execute(f"UPDATE integrations SET {','.join(fields)} WHERE id=? AND account_id=?", params)
    conn.commit()
    conn.close()
    return jsonify({"ok": True})


@app.delete("/api/integrations/<int:iid>")
@login_required
def delete_integration(iid):
    aid = current_account_id()
    conn = get_db()
    conn.execute("DELETE FROM integrations WHERE id=? AND account_id=?", (iid, aid))
    conn.commit()
    conn.close()
    return jsonify({"ok": True})


# ---------- Reminder schedules (web-editable, replaces rumahbot's /jadwal) ----------
@app.get("/api/reminders")
@login_required
def list_reminders():
    aid = current_account_id()
    conn = get_db()
    rows = [dict(r) for r in conn.execute("SELECT * FROM reminder_schedules WHERE account_id=? ORDER BY job_name", (aid,))]
    conn.close()
    return jsonify(rows)


@app.post("/api/reminders")
@login_required
def set_reminder():
    aid = current_account_id()
    d = request.get_json(force=True)
    job_name = (d.get("job_name") or "").strip()
    if not job_name:
        return jsonify({"error": "job_name required"}), 400
    conn = get_db()
    conn.execute(
        "INSERT INTO reminder_schedules (account_id, job_name, days, hour, minute, enabled) VALUES (?,?,?,?,?,?) "
        "ON CONFLICT(account_id, job_name) DO UPDATE SET days=excluded.days, hour=excluded.hour, minute=excluded.minute, enabled=excluded.enabled",
        (aid, job_name, d.get("days", "0,1,2,3,4,5,6"), d.get("hour", 8), d.get("minute", 0), 1 if d.get("enabled", True) else 0))
    conn.commit()
    conn.close()
    return jsonify({"ok": True})


@app.delete("/api/reminders/<int:rid>")
@login_required
def delete_reminder(rid):
    aid = current_account_id()
    conn = get_db()
    conn.execute("DELETE FROM reminder_schedules WHERE id=? AND account_id=?", (rid, aid))
    conn.commit()
    conn.close()
    return jsonify({"ok": True})


# ---------- Mandatory ingredients (web-editable, replaces /tambahbahan) ----------
@app.get("/api/ingredients")
@login_required
def list_ingredients():
    aid = current_account_id()
    conn = get_db()
    rows = [dict(r) for r in conn.execute("SELECT * FROM mandatory_ingredients WHERE account_id=? ORDER BY name", (aid,))]
    conn.close()
    return jsonify(rows)


@app.post("/api/ingredients")
@login_required
def add_ingredient():
    aid = current_account_id()
    name = (request.get_json(force=True).get("name") or "").strip()
    if not name:
        return jsonify({"error": "name required"}), 400
    conn = get_db()
    try:
        conn.execute("INSERT INTO mandatory_ingredients (account_id, name) VALUES (?,?)", (aid, name))
        conn.commit()
    except sqlite3.IntegrityError:
        pass
    conn.close()
    return jsonify({"ok": True})


@app.delete("/api/ingredients/<int:iid>")
@login_required
def delete_ingredient(iid):
    aid = current_account_id()
    conn = get_db()
    conn.execute("DELETE FROM mandatory_ingredients WHERE id=? AND account_id=?", (iid, aid))
    conn.commit()
    conn.close()
    return jsonify({"ok": True})


# ---------- Category keywords (web-editable, replaces /koreksi learning) ----------
@app.get("/api/keywords")
@login_required
def list_keywords():
    aid = current_account_id()
    conn = get_db()
    rows = [dict(r) for r in conn.execute("SELECT * FROM category_keywords WHERE account_id=? ORDER BY keyword", (aid,))]
    conn.close()
    return jsonify(rows)


@app.post("/api/keywords")
@login_required
def set_keyword():
    aid = current_account_id()
    d = request.get_json(force=True)
    keyword = (d.get("keyword") or "").strip().lower()
    category = d.get("category")
    if not keyword or not category:
        return jsonify({"error": "keyword and category required"}), 400
    conn = get_db()
    conn.execute(
        "INSERT INTO category_keywords (account_id, keyword, category) VALUES (?,?,?) "
        "ON CONFLICT(account_id, keyword) DO UPDATE SET category=excluded.category",
        (aid, keyword, category))
    conn.commit()
    conn.close()
    return jsonify({"ok": True})


@app.delete("/api/keywords/<int:kid>")
@login_required
def delete_keyword(kid):
    aid = current_account_id()
    conn = get_db()
    conn.execute("DELETE FROM category_keywords WHERE id=? AND account_id=?", (kid, aid))
    conn.commit()
    conn.close()
    return jsonify({"ok": True})


# ---------- Super admin ----------
@app.get("/api/admin/accounts")
@admin_required
def admin_list_accounts():
    conn = get_db()
    rows = [dict(r) for r in conn.execute(
        """SELECT u.id, u.email, u.display_name, u.currency, u.is_admin, u.is_disabled, u.created_at,
                  (SELECT COUNT(*) FROM spending WHERE account_id=u.id) as spending_count,
                  (SELECT COUNT(*) FROM integrations WHERE account_id=u.id) as integration_count
           FROM users u ORDER BY u.id""")]
    conn.close()
    return jsonify(rows)


@app.patch("/api/admin/accounts/<int:uid>")
@admin_required
def admin_update_account(uid):
    d = request.get_json(force=True)
    fields, params = [], []
    if "is_disabled" in d:
        fields.append("is_disabled=?")
        params.append(1 if d["is_disabled"] else 0)
    if "is_admin" in d:
        fields.append("is_admin=?")
        params.append(1 if d["is_admin"] else 0)
    if not fields:
        return jsonify({"ok": True})
    conn = get_db()
    params.append(uid)
    conn.execute(f"UPDATE users SET {','.join(fields)} WHERE id=?", params)
    conn.commit()
    conn.close()
    return jsonify({"ok": True})


@app.delete("/api/admin/accounts/<int:uid>")
@admin_required
def admin_delete_account(uid):
    if uid == current_account_id():
        return jsonify({"error": "cannot delete your own admin account"}), 400
    conn = get_db()
    for table in ("spending", "budget_income", "budget_expense", "savings_list", "assets", "asset_target",
                  "accounts", "categories", "income_sources", "integrations", "reminder_schedules",
                  "mandatory_ingredients", "category_keywords"):
        conn.execute(f"DELETE FROM {table} WHERE account_id=?", (uid,))
    conn.execute("DELETE FROM users WHERE id=?", (uid,))
    conn.commit()
    conn.close()
    return jsonify({"ok": True})


# ---------- Bot ingestion (internal; per-account token-protected) ----------
# Each account's chat-platform integrations carry a per-account bot_token
# (issued via /api/integrations), so one Telegram or Discord bot process
# serving many accounts always resolves to exactly the right account's data.
def _check_bot_token():
    token = request.headers.get("X-Bot-Token", "")
    if not token:
        return None
    conn = get_db()
    row = conn.execute("SELECT account_id FROM integrations WHERE bot_token=? AND enabled=1", (token,)).fetchone()
    conn.close()
    return row["account_id"] if row else None


# Internal only: lets the standalone bot-multiplexer process (bots/telegram_bot.py,
# bots/discord_bot.py) discover every tenant's connected platform token without a
# user session - protected by a separate service secret, never exposed to the web UI.
SERVICE_TOKEN = os.environ.get("SERVICE_TOKEN", "")


@app.get("/api/internal/integrations")
def internal_list_integrations():
    if not SERVICE_TOKEN or request.headers.get("X-Service-Token") != SERVICE_TOKEN:
        return jsonify({"error": "forbidden"}), 403
    platform = request.args.get("platform")
    conn = get_db()
    q = "SELECT account_id, platform, bot_token, platform_token, chat_id FROM integrations WHERE enabled=1 AND platform_token IS NOT NULL AND platform_token != ''"
    params = []
    if platform:
        q += " AND platform=?"
        params.append(platform)
    rows = [dict(r) for r in conn.execute(q, params)]
    conn.close()
    return jsonify(rows)


@app.get("/api/bot/config")
def bot_config():
    """Bot adapter calls this on startup/poll to get this account's
    reminders/ingredients/keywords - lets web UI edits take effect without
    restarting the bot process."""
    aid = _check_bot_token()
    if aid is None:
        return jsonify({"error": "forbidden"}), 403
    conn = get_db()
    reminders = [dict(r) for r in conn.execute("SELECT * FROM reminder_schedules WHERE account_id=? AND enabled=1", (aid,))]
    ingredients = [r["name"] for r in conn.execute("SELECT name FROM mandatory_ingredients WHERE account_id=?", (aid,))]
    keywords = {r["keyword"]: r["category"] for r in conn.execute("SELECT keyword, category FROM category_keywords WHERE account_id=?", (aid,))}
    categories = [r["name"] for r in conn.execute("SELECT name FROM categories WHERE account_id=?", (aid,))]
    conn.close()
    return jsonify({"reminders": reminders, "mandatory_ingredients": ingredients, "category_keywords": keywords, "categories": categories})


@app.post("/api/bot/spending")
def bot_add_spending():
    aid = _check_bot_token()
    if aid is None:
        return jsonify({"error": "forbidden"}), 403
    d = request.get_json(force=True)
    conn = get_db()
    cat_id = None
    if d.get("category"):
        row = conn.execute("SELECT id FROM categories WHERE name=? AND account_id=?", (d["category"], aid)).fetchone()
        cat_id = row["id"] if row else None
    source = d.get("source", "telegram")
    cur = conn.execute(
        "INSERT INTO spending (account_id, tx_date, description, category_id, category_name, amount, source) VALUES (?,?,?,?,?,?,?)",
        (aid, d["tx_date"], d["description"], cat_id, d.get("category"), d["amount"], source))
    conn.commit()
    new_id = cur.lastrowid
    conn.close()
    return jsonify({"ok": True, "id": new_id})


@app.get("/api/bot/spending/recent")
def bot_recent_spending():
    aid = _check_bot_token()
    if aid is None:
        return jsonify({"error": "forbidden"}), 403
    n = request.args.get("n", default=5, type=int)
    conn = get_db()
    rows = conn.execute(
        "SELECT id, tx_date, description, category_name, amount FROM spending WHERE account_id=? ORDER BY id DESC LIMIT ?", (aid, n)
    ).fetchall()
    conn.close()
    return jsonify([dict(r) for r in rows])


@app.get("/api/bot/spending/search")
def bot_search_spending():
    aid = _check_bot_token()
    if aid is None:
        return jsonify({"error": "forbidden"}), 403
    q = request.args.get("q", "")
    conn = get_db()
    rows = conn.execute(
        "SELECT id, tx_date, description, category_name, amount FROM spending "
        "WHERE account_id=? AND description LIKE ? ORDER BY id DESC LIMIT 50",
        (aid, f"%{q}%")
    ).fetchall()
    conn.close()
    return jsonify([dict(r) for r in rows])


@app.patch("/api/bot/spending/<int:sid>/category")
def bot_update_category(sid):
    aid = _check_bot_token()
    if aid is None:
        return jsonify({"error": "forbidden"}), 403
    d = request.get_json(force=True)
    category = d.get("category")
    conn = get_db()
    row = conn.execute("SELECT id FROM categories WHERE name=? AND account_id=?", (category, aid)).fetchone()
    cat_id = row["id"] if row else None
    conn.execute("UPDATE spending SET category_id=?, category_name=? WHERE id=? AND account_id=?", (cat_id, category, sid, aid))
    conn.commit()
    conn.close()
    return jsonify({"ok": True})


@app.patch("/api/bot/spending/<int:sid>/date")
def bot_update_date(sid):
    aid = _check_bot_token()
    if aid is None:
        return jsonify({"error": "forbidden"}), 403
    d = request.get_json(force=True)
    tx_date = d.get("tx_date", "")
    try:
        datetime.strptime(tx_date, "%Y-%m-%d")
    except ValueError:
        return jsonify({"error": "tx_date must be YYYY-MM-DD"}), 400
    conn = get_db()
    row = conn.execute("SELECT id FROM spending WHERE id=? AND account_id=?", (sid, aid)).fetchone()
    if not row:
        conn.close()
        return jsonify({"error": "not found"}), 404
    conn.execute("UPDATE spending SET tx_date=? WHERE id=?", (tx_date, sid))
    conn.commit()
    conn.close()
    return jsonify({"ok": True})


# ---------- Static SPA ----------
@app.route("/")
@app.route("/<path:path>")
def index(path=""):
    full = os.path.join(STATIC_DIR, path)
    if path and os.path.isfile(full):
        return send_from_directory(STATIC_DIR, path)
    resp = send_from_directory(STATIC_DIR, "index.html")
    resp.headers["Cache-Control"] = "no-cache, no-store, must-revalidate"
    return resp


if __name__ == "__main__":
    init_db()
    app.run(host="0.0.0.0", port=5000, debug=True)
else:
    init_db()
