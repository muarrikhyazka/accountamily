"""Migrate data from Google Sheet "Prasidana Tata Dana 2026" into tatadana.sqlite.
Run: DB_PATH=./data/tatadana.sqlite python3 migrate_from_sheet.py
"""
import os, sqlite3, re, json
from datetime import datetime
from google.oauth2 import service_account
from googleapiclient.discovery import build

SHEET_ID = os.environ.get("SHEET_ID", "1IlaCSyVfmz0wiQlazoj5z53nVvC7V4DeGrRbtFqHTmM")
SA_FILE = os.environ.get("SA_FILE", "/home/muarrikh/apps/rumahbot/google_service_account.json")
DB_PATH = os.environ.get("DB_PATH", "/app/data/tatadana.sqlite")
SKIP_ASSETS = os.environ.get("SKIP_ASSETS", "0") == "1"

MONTHS = ["January", "February", "March", "April", "May", "June", "July", "August",
          "September", "October", "November", "December"]

MONTHS_ID = {"Jan": 1, "Feb": 2, "Mar": 3, "Apr": 4, "May": 5, "Jun": 6,
             "Jul": 7, "Aug": 8, "Sep": 9, "Oct": 10, "Nov": 11, "Dec": 12}


def get_sheets():
    creds = service_account.Credentials.from_service_account_file(
        SA_FILE, scopes=["https://www.googleapis.com/auth/spreadsheets.readonly"])
    return build("sheets", "v4", credentials=creds)


def rupiah_to_int(s):
    if not s:
        return 0
    s = s.replace("Rp", "").strip()
    neg = s.startswith("-")
    s = s.lstrip("-")
    s = re.sub(r"[.,]", "", s)  # sheet uses , or . as thousand sep depending on locale
    if not s.isdigit():
        return 0
    val = int(s)
    return -val if neg else val


def parse_sheet_date(s):
    """'28 Aug 2026' -> '2026-08-28'"""
    s = s.strip()
    m = re.match(r"(\d{1,2})\s+(\w{3})\s+(\d{4})", s)
    if not m:
        return None
    day, mon, year = m.groups()
    month = MONTHS_ID.get(mon)
    if not month:
        return None
    return f"{year}-{month:02d}-{int(day):02d}"


def get_year():
    return int(os.environ.get("MIGRATE_YEAR", datetime.now().year))


def migrate():
    svc = get_sheets()
    conn = sqlite3.connect(DB_PATH)
    year = get_year()

    # ---- Budgeting: income sources ----
    r = svc.spreadsheets().values().get(spreadsheetId=SHEET_ID, range="Budgeting!A1:N50").execute()
    rows = r.get("values", [])

    def row_at(idx):
        return rows[idx] if idx < len(rows) else []

    # find section boundaries by label in col B (index 1)
    def find_row(label):
        for i, row in enumerate(rows):
            if len(row) > 1 and row[1].strip() == label:
                return i
        return None

    income_start = find_row("Income Sources")
    total1 = find_row("Total")
    savings_start = find_row("Savings List")
    total2_idx = None
    for i in range(savings_start + 1 if savings_start else 0, len(rows)):
        if len(rows[i]) > 1 and rows[i][1].strip() == "Total":
            total2_idx = i
            break
    budgeting_start = find_row("Budgeting List")

    n_income = n_expense = n_savings = 0

    if income_start is not None and total1 is not None:
        for i in range(income_start + 1, total1):
            row = row_at(i)
            if len(row) < 2 or not row[1].strip():
                continue
            source_name = row[1].strip()
            conn.execute("INSERT OR IGNORE INTO income_sources (name) VALUES (?)", (source_name,))
            src_id = conn.execute("SELECT id FROM income_sources WHERE name=?", (source_name,)).fetchone()[0]
            for m_idx, month_name in enumerate(MONTHS):
                col = m_idx + 2
                amount = rupiah_to_int(row[col]) if col < len(row) else 0
                if amount:
                    conn.execute(
                        "INSERT INTO budget_income (year, month, source_id, amount) VALUES (?,?,?,?) "
                        "ON CONFLICT(year,month,source_id) DO UPDATE SET amount=excluded.amount",
                        (year, m_idx + 1, src_id, amount))
                    n_income += 1

    if savings_start is not None and total2_idx is not None:
        for i in range(savings_start + 1, total2_idx):
            row = row_at(i)
            if len(row) < 2 or not row[1].strip():
                continue
            name = row[1].strip()
            for m_idx, month_name in enumerate(MONTHS):
                col = m_idx + 2
                amount = rupiah_to_int(row[col]) if col < len(row) else 0
                if amount:
                    conn.execute(
                        "INSERT INTO savings_list (year, month, name, amount) VALUES (?,?,?,?) "
                        "ON CONFLICT(year,month,name) DO UPDATE SET amount=excluded.amount",
                        (year, m_idx + 1, name, amount))
                    n_savings += 1

    if budgeting_start is not None:
        total3_idx = None
        for i in range(budgeting_start + 1, len(rows)):
            if len(rows[i]) > 1 and rows[i][1].strip() == "Total":
                total3_idx = i
                break
        end = total3_idx if total3_idx else len(rows)
        for i in range(budgeting_start + 1, end):
            row = row_at(i)
            if len(row) < 2 or not row[1].strip():
                continue
            cat_name = row[1].strip()
            cat_row = conn.execute("SELECT id FROM categories WHERE name=?", (cat_name,)).fetchone()
            if not cat_row:
                continue  # unknown category, skip (shouldn't happen if seed.py ran first)
            cat_id = cat_row[0]
            for m_idx, month_name in enumerate(MONTHS):
                col = m_idx + 2
                amount = rupiah_to_int(row[col]) if col < len(row) else 0
                if amount:
                    conn.execute(
                        "INSERT INTO budget_expense (year, month, category_id, amount) VALUES (?,?,?,?) "
                        "ON CONFLICT(year,month,category_id) DO UPDATE SET amount=excluded.amount",
                        (year, m_idx + 1, cat_id, amount))
                    n_expense += 1

    conn.commit()
    print(f"budgeting: income={n_income} expense={n_expense} savings={n_savings}")

    # ---- Spending ----
    r2 = svc.spreadsheets().values().get(spreadsheetId=SHEET_ID, range="Spending!A1:G2500").execute()
    srows = r2.get("values", [])
    n_spending = n_skipped = 0
    for row in srows:
        if len(row) < 7:
            continue
        checked_raw, date_raw, desc, _, cat_raw, total_raw = row[1], row[2], row[3], row[4] if len(row) > 4 else "", row[5], row[6]
        if not desc or not desc.strip() or desc.strip() == "Description":
            continue
        tx_date = parse_sheet_date(date_raw)
        amount = rupiah_to_int(total_raw)
        if not tx_date or not amount:
            n_skipped += 1
            continue
        cat_name = (cat_raw or "").strip()
        cat_row = conn.execute("SELECT id FROM categories WHERE name=?", (cat_name,)).fetchone()
        cat_id = cat_row[0] if cat_row else None
        checked = 1 if checked_raw.strip().upper() == "TRUE" else 0
        conn.execute(
            "INSERT INTO spending (tx_date, description, category_id, category_name, amount, checked, source) "
            "VALUES (?,?,?,?,?,?,'migration')",
            (tx_date, desc.strip(), cat_id, cat_name, amount, checked))
        n_spending += 1
    conn.commit()
    print(f"spending: migrated={n_spending} skipped={n_skipped}")

    # ---- Assets ----
    n_assets = 0
    if SKIP_ASSETS:
        print("assets: skipped (SKIP_ASSETS=1)")
    else:
        r3 = svc.spreadsheets().values().get(spreadsheetId=SHEET_ID, range="Assets!A1:H40").execute()
        arows = r3.get("values", [])

        def arow(i):
            return arows[i] if i < len(arows) else []

        # target row: label "Last update" at some idx, values on idx+1
        target_idx = None
        for i, row in enumerate(arows):
            if len(row) > 1 and row[1].strip() == "Last update":
                target_idx = i
                break
        if target_idx is not None:
            vrow = arow(target_idx + 1)
            last_update = vrow[1] if len(vrow) > 1 else None
            target_val = rupiah_to_int(vrow[2]) if len(vrow) > 2 else 0
            if target_val:
                conn.execute("INSERT INTO asset_target (target_value, last_update) VALUES (?,?)", (target_val, last_update))

        # Liquid/Non-liquid descriptions table: header row "Descriptions" ... "Value" "Account" ... "Descriptions"
        desc_header_idx = None
        for i, row in enumerate(arows):
            if len(row) > 1 and row[1].strip() == "Descriptions":
                desc_header_idx = i
                break
        if desc_header_idx is not None:
            for i in range(desc_header_idx + 1, len(arows)):
                row = arow(i)
                if len(row) < 2 or not row[1].strip():
                    continue
                desc = row[1].strip()
                if desc == "Add Yours":
                    continue
                value = rupiah_to_int(row[3]) if len(row) > 3 else 0
                account_name = row[4].strip() if len(row) > 4 else None
                acc_id = None
                if account_name:
                    acc_row = conn.execute("SELECT id FROM accounts WHERE name=?", (account_name,)).fetchone()
                    acc_id = acc_row[0] if acc_row else None
                if value:
                    conn.execute(
                        "INSERT INTO assets (description, value, account_id, kind) VALUES (?,?,?,'liquid')",
                        (desc, value, acc_id))
                    n_assets += 1
                # non-liquid column (col 6 = index 6)
                non_liquid_desc = row[6].strip() if len(row) > 6 else None
                if non_liquid_desc and non_liquid_desc != "Add Yours":
                    conn.execute(
                        "INSERT INTO assets (description, value, account_id, kind) VALUES (?,0,NULL,'non_liquid')",
                        (non_liquid_desc,))
                    n_assets += 1

    conn.commit()
    print(f"assets: migrated={n_assets}")
    conn.close()
    print("MIGRATION DONE")


if __name__ == "__main__":
    migrate()
