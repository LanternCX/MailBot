# Setup Guide

MailBot looks for `config.json` in the current working directory by default; pass `-c PATH` / `--config PATH` when launching if you want to point at a different location. The CLI menu or headless run will create the file automatically if it is missing.

## Google/Gmail

To use a Gmail account, you must enable **2-Step Verification** and generate an **App Password**.  
Using your main Google account password is no longer supported for IMAP access.

1.  **Enable 2-Step Verification**: Go to your Google Account > Security settings.
2.  **Create an App Password**: Follow the official guide:
    [**Sign in with App Passwords (Google Support)**](https://support.google.com/mail/answer/185833?hl=en)
3.  **Use the App Password**: When MailBot asks for your password, enter the 16-character generated password.

---

## Telegram Bot

You need a Telegram Bot Token to send messages and your User ID (Chat ID) to receive them.

### 1. Create a Bot (Get the Token)
1.  Open Telegram and search for [`@BotFather`](https://t.me/BotFather).
2.  Send the command `/newbot`.
3.  Follow the instructions to name your bot.
4.  Copy the **HTTP API Token** provided by BotFather (e.g., `123456789:ABCdefGHIjklMNOpqrSTUvwxyz`).

### 2. Get Your Chat ID
1.  Start a chat with your new bot and send a message (e.g., `/start`).
2.  Search for [`@userinfobot`](https://t.me/userinfobot) on Telegram.
3.  Send any message to it. It will reply with your `Id` (e.g., `12345678`), or you can simply use the API method:
    *   Visit: `https://api.telegram.org/bot<YOUR_TOKEN>/getUpdates`
    *   Find the `"id"` field under `"chat"`.
4.  Enter this ID into MailBot when prompted.

---

## Proxy & Network

If your IMAP provider or Telegram endpoint must be reached via a SOCKS/HTTP proxy, add a `proxy` block to your configuration or enable it through the menu’s system settings. It supports:

* `scheme`: `socks5`, `socks4`, or `http`
* `host`/`port`: address of the proxy
* `username` / `password`: optional credentials

With the proxy enabled MailBot automatically sets the `http(s)_proxy` environment variables and routes IMAP sockets through the proxy so all network traffic (IMAP fetching + Telegram API) obeys it. Use the CLI wizard’s system settings if you prefer an interactive form.

## Other Email Providers

For other providers (Outlook, Yahoo, iCloud, etc.), ensure that:
1.  **IMAP Access** is enabled in your account settings.
2.  You use an **App Password** if 2FA is enabled (highly recommended).

---

## Local PyInstaller Build & Testing

You can build a single-file executable locally to verify packaging before pushing to GitHub Actions. This catches issues like missing hidden imports or data files early.

### Prerequisites

```bash
# Make sure you are in the project root with the venv activated
pip install pyinstaller
pip install -r requirements.txt
```

### Build the Full Binary

```bash
.venv/bin/python -m PyInstaller --clean --onefile \
    --additional-hooks-dir hooks \
    --collect-all rich \
    --hidden-import litellm \
    --collect-data litellm \
    --name MailBot main.py
```

The executable will be at `dist/MailBot`. Run it:

```bash
./dist/MailBot --help
./dist/MailBot --headless -c config.json
```

### Quick Smoke Test (litellm only)

A dedicated smoke-test script verifies that litellm imports and loads its cost-map data correctly inside a PyInstaller bundle:

```bash
# Build the smoke-test binary
.venv/bin/python -m PyInstaller --clean --onefile \
    --additional-hooks-dir hooks \
    --hidden-import litellm \
    --collect-data litellm \
    --name smoketest scripts/smoketest_litellm.py

# Run it — should print "OK"
./dist/smoketest
```

Expected output:

```
frozen=True  _MEIPASS=/var/folders/.../...
OK — litellm <version> imported successfully
model_cost type=dict  entries=NNNN
```

If you see `FAIL — FileNotFoundError` or `FAIL — ImportError`, the packaging flags or `hooks/hook-litellm.py` need attention.

### Or Use the Package Script

```bash
.venv/bin/python scripts/package.py --clean --entry main.py --variant macos-arm64
```

The resulting zip will be at `dist/macos-arm64/`.

> **Tip:** Clean up test artifacts with `rm -rf dist/ build/` when done.
