#!/usr/bin/env python3
"""Multi-tenant Telegram bot for Accountamily.

One process, many accounts: polls /api/internal/integrations for every
enabled Telegram integration, spins up one python-telegram-bot Application
per platform_token (each tenant brings their own BotFather bot), and routes
each incoming message through that tenant's X-Bot-Token to the backend.

Reload latency: a cheap GET /api/internal/integrations-version check runs
every VERSION_CHECK_SECONDS (default 5s); the expensive full fetch+rebuild
only runs when that version actually changed. ponytail: still polling, not
true push - a webhook/SSE from the backend would remove the ~5s worst-case
lag entirely; add it if that becomes noticeable.
"""
import asyncio, json, logging, os, re
from datetime import datetime, timezone, timedelta
from urllib.request import urlopen, Request
from urllib.error import HTTPError, URLError
from telegram import Update
from telegram.ext import Application, MessageHandler, CommandHandler, ContextTypes, filters

logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(levelname)s] %(message)s")
logger = logging.getLogger("accountamily-telegram")

BACKEND_URL = os.environ["BACKEND_URL"]           # e.g. http://accountamily:5000
SERVICE_TOKEN = os.environ["SERVICE_TOKEN"]
REFRESH_SECONDS = int(os.environ.get("REFRESH_SECONDS", "300"))  # fallback full-rebuild cadence even if version never changes
VERSION_CHECK_SECONDS = int(os.environ.get("VERSION_CHECK_SECONDS", "5"))
TZ = timezone(timedelta(hours=7))

AMOUNT_RE = re.compile(r"(\d[\d.,]*)\s*(k|rb|jt|ribu|juta)?", re.IGNORECASE)
MULTIPLIER = {"k": 1_000, "rb": 1_000, "ribu": 1_000, "jt": 1_000_000, "juta": 1_000_000}


def backend_get(path, params=None):
    url = f"{BACKEND_URL}{path}"
    if params:
        from urllib.parse import urlencode
        url += "?" + urlencode(params)
    req = Request(url, headers={"X-Service-Token": SERVICE_TOKEN})
    with urlopen(req, timeout=10) as resp:
        return json.loads(resp.read().decode())


def bot_api(bot_token, method, path, body=None, params=None):
    url = f"{BACKEND_URL}{path}"
    if params:
        from urllib.parse import urlencode
        url += "?" + urlencode(params)
    data = json.dumps(body).encode() if body is not None else None
    req = Request(url, data=data, method=method, headers={"Content-Type": "application/json", "X-Bot-Token": bot_token})
    with urlopen(req, timeout=10) as resp:
        return json.loads(resp.read().decode())


def parse_message(text, categories, keywords):
    """'50000 makan siang' or '50k makan' -> (amount, description, category)."""
    m = AMOUNT_RE.search(text)
    if not m:
        return None
    raw, unit = m.groups()
    amount = int(float(raw.replace(".", "").replace(",", "."))) if "," in raw else int(raw.replace(".", ""))
    if unit:
        amount *= MULTIPLIER.get(unit.lower(), 1)
    description = (text[:m.start()] + text[m.end():]).strip() or "expense"
    category = None
    desc_lower = description.lower()
    for word in re.findall(r"[a-z]+", desc_lower):
        if word in keywords:
            category = keywords[word]
            break
    return amount, description, category


def make_handlers(bot_token, chat_id):
    async def handle_message(update: Update, context: ContextTypes.DEFAULT_TYPE):
        if chat_id and str(update.effective_chat.id) != str(chat_id):
            return
        text = update.message.text or ""
        try:
            cfg = bot_api(bot_token, "GET", "/api/bot/config")
        except (HTTPError, URLError) as e:
            await update.message.reply_text("Backend unreachable, try again later.")
            return
        parsed = parse_message(text, cfg.get("categories", []), cfg.get("category_keywords", {}))
        if not parsed:
            return
        amount, description, category = parsed
        tx_date = datetime.now(TZ).strftime("%Y-%m-%d")
        try:
            result = bot_api(bot_token, "POST", "/api/bot/spending", {
                "tx_date": tx_date, "description": description, "category": category, "amount": amount, "source": "telegram",
            })
        except (HTTPError, URLError):
            await update.message.reply_text("Failed to save - backend error.")
            return
        cat_label = category or "Other"
        await update.message.reply_text(f"Logged: {description} - Rp{amount:,} ({cat_label})".replace(",", "."))

    async def handle_recent(update: Update, context: ContextTypes.DEFAULT_TYPE):
        if chat_id and str(update.effective_chat.id) != str(chat_id):
            return
        try:
            rows = bot_api(bot_token, "GET", "/api/bot/spending/recent", params={"n": 5})
        except (HTTPError, URLError):
            await update.message.reply_text("Backend unreachable.")
            return
        if not rows:
            await update.message.reply_text("No entries yet.")
            return
        lines = [f"{r['tx_date']} - {r['description']} - Rp{r['amount']:,} ({r.get('category_name') or 'Other'})".replace(",", ".") for r in rows]
        await update.message.reply_text("\n".join(lines))

    return handle_message, handle_recent


async def run():
    apps = {}  # account_id -> Application
    last_version = None
    last_full_rebuild = 0.0
    loop = asyncio.get_event_loop()
    while True:
        try:
            version = backend_get("/api/internal/integrations-version").get("version")
        except Exception as e:
            logger.error(f"Could not fetch integrations version: {e}")
            version = last_version  # assume unchanged, don't force a rebuild on transient errors

        now = loop.time()
        changed = version != last_version
        if not changed and (now - last_full_rebuild) < REFRESH_SECONDS:
            await asyncio.sleep(VERSION_CHECK_SECONDS)
            continue
        last_version = version
        last_full_rebuild = now

        try:
            integrations = backend_get("/api/internal/integrations", params={"platform": "telegram"})
        except Exception as e:
            logger.error(f"Could not fetch integrations: {e}")
            integrations = []

        seen = set()
        for integ in integrations:
            aid = integ["account_id"]
            seen.add(aid)
            if aid in apps:
                continue
            platform_token = integ.get("platform_token")
            if not platform_token:
                continue
            try:
                application = Application.builder().token(platform_token).build()
                handle_message, handle_recent = make_handlers(integ["bot_token"], integ.get("chat_id"))
                application.add_handler(CommandHandler("recent", handle_recent))
                application.add_handler(MessageHandler(filters.TEXT & ~filters.COMMAND, handle_message))
                await application.initialize()
                await application.start()
                await application.updater.start_polling()
                apps[aid] = application
                logger.info(f"Started Telegram bot for account {aid}")
            except Exception as e:
                logger.error(f"Failed to start bot for account {aid}: {e}")

        for aid in list(apps.keys()):
            if aid not in seen:
                logger.info(f"Stopping Telegram bot for account {aid} (integration removed/disabled)")
                app_to_stop = apps.pop(aid)
                await app_to_stop.updater.stop()
                await app_to_stop.stop()
                await app_to_stop.shutdown()

        await asyncio.sleep(VERSION_CHECK_SECONDS)


if __name__ == "__main__":
    asyncio.run(run())
