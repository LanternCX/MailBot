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
