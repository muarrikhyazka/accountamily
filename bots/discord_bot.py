#!/usr/bin/env python3
"""Multi-tenant Discord bot for Accountamily.

Same multiplexing pattern as bots/telegram_bot.py: one process, many
accounts, one discord.py Client per connected integration's bot token.
"""
import asyncio, json, logging, os, re
from datetime import datetime, timezone, timedelta
from urllib.request import urlopen, Request
from urllib.error import HTTPError, URLError
import discord

logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(levelname)s] %(message)s")
logger = logging.getLogger("accountamily-discord")

BACKEND_URL = os.environ["BACKEND_URL"]
SERVICE_TOKEN = os.environ["SERVICE_TOKEN"]
REFRESH_SECONDS = int(os.environ.get("REFRESH_SECONDS", "300"))
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


def parse_message(text, keywords):
    m = AMOUNT_RE.search(text)
    if not m:
        return None
    raw, unit = m.groups()
    amount = int(float(raw.replace(".", "").replace(",", "."))) if "," in raw else int(raw.replace(".", ""))
    if unit:
        amount *= MULTIPLIER.get(unit.lower(), 1)
    description = (text[:m.start()] + text[m.end():]).strip() or "expense"
    category = None
    for word in re.findall(r"[a-z]+", description.lower()):
        if word in keywords:
            category = keywords[word]
            break
    return amount, description, category


class TenantClient(discord.Client):
    def __init__(self, bot_token, chat_id, **kwargs):
        super().__init__(**kwargs)
        self.bot_token = bot_token
        self.chat_id = chat_id

    async def on_ready(self):
        logger.info(f"Discord client ready: {self.user}")

    async def on_message(self, message):
        if message.author.bot:
            return
        if self.chat_id and str(message.channel.id) != str(self.chat_id):
            return
        content = message.content.strip()
        if content == "!recent":
            try:
                rows = bot_api(self.bot_token, "GET", "/api/bot/spending/recent", params={"n": 5})
            except (HTTPError, URLError):
                await message.channel.send("Backend unreachable.")
                return
            if not rows:
                await message.channel.send("No entries yet.")
                return
            lines = [f"{r['tx_date']} - {r['description']} - Rp{r['amount']:,} ({r.get('category_name') or 'Other'})".replace(",", ".") for r in rows]
            await message.channel.send("\n".join(lines))
            return

        try:
            cfg = bot_api(self.bot_token, "GET", "/api/bot/config")
        except (HTTPError, URLError):
            return
        parsed = parse_message(content, cfg.get("category_keywords", {}))
        if not parsed:
            return
        amount, description, category = parsed
        tx_date = datetime.now(TZ).strftime("%Y-%m-%d")
        try:
            bot_api(self.bot_token, "POST", "/api/bot/spending", {
                "tx_date": tx_date, "description": description, "category": category, "amount": amount, "source": "discord",
            })
        except (HTTPError, URLError):
            await message.channel.send("Failed to save - backend error.")
            return
        cat_label = category or "Other"
        await message.channel.send(f"Logged: {description} - Rp{amount:,} ({cat_label})".replace(",", "."))


async def run():
    clients = {}  # account_id -> (client, task)
    intents = discord.Intents.default()
    intents.message_content = True

    while True:
        try:
            integrations = backend_get("/api/internal/integrations", params={"platform": "discord"})
        except Exception as e:
            logger.error(f"Could not fetch integrations: {e}")
            integrations = []

        seen = set()
        for integ in integrations:
            aid = integ["account_id"]
            seen.add(aid)
            if aid in clients:
                continue
            platform_token = integ.get("platform_token")
            if not platform_token:
                continue
            client = TenantClient(integ["bot_token"], integ.get("chat_id"), intents=intents)
            task = asyncio.create_task(client.start(platform_token))
            clients[aid] = (client, task)
            logger.info(f"Started Discord client for account {aid}")

        for aid in list(clients.keys()):
            if aid not in seen:
                logger.info(f"Stopping Discord client for account {aid} (integration removed/disabled)")
                client, task = clients.pop(aid)
                await client.close()
                task.cancel()

        await asyncio.sleep(REFRESH_SECONDS)


if __name__ == "__main__":
    asyncio.run(run())
