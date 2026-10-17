# Configuration and usage

Run the binary without arguments for the menu, or use `--headless -c PATH` for unattended operation. Configuration defaults to `config.json` in the working directory. `--check` validates without connecting or creating state. `--version` prints the build version.

## Menu

Choose by number or full option name. Enter accepts the displayed default; `:cancel` abandons the current dialog. Secrets are hidden on a real terminal and saved values are not printed.

The menu provides **Start Service** (Ctrl+C returns to the menu), **Config Wizard** (add/remove accounts and verify IMAP login), **Bot Settings** (set/remove bot), **AI Settings** (DeepSeek key, model, endpoint, mode and language), **System Settings** (polling, retries, console level and proxy), **Test Connection** (check Telegram and optionally send a test preview), and **Exit**. Cancelled dialogs do not save partial changes.

## Short notifications and full mail reading

Add `"reader": {"listen": "127.0.0.1:8080", "public_url": "https://mail.example.com"}` to the configuration. Point your HTTPS domain at a reverse proxy forwarding to that HTTP listener. For example, Caddy can serve `mail.example.com { reverse_proxy 127.0.0.1:8080 }`. The public URL must be an HTTPS origin without a path, credentials, query or fragment. Leave both fields empty to disable the reader.

With the reader configured, newly prepared notifications use a short card and a full-body URL button, bypassing AI and its callback buttons. Locally detected numeric verification codes are highlighted for copying. Previously persisted delivery content keeps its existing progress and format. Without a reader, the modes below continue to apply.

New mail stores both HTML and plain text. The reader sanitizes HTML and displays it in a sandboxed frame, preserving tables and supported inline styles. Remote images, CID images, attachments, scripts and forms are unavailable. Fixed-width messages may require horizontal scrolling within the body. Older mail uses stored plain text; historical HTML is not fetched again. Messages above `max_message_bytes` retain the existing size-limit notice and must be read in the original mailbox.

Each mail has an unguessable persistent URL: anyone holding it can read the mail. Forwarding it shares access. No public mailbox listing is provided. Avoid logging full `/read/` paths in your reverse proxy. Links survive restarts with the SQLite database; completed mail expires 30 days after ingestion during periodic cleanup, while pending mail is retained. Removed or unknown mail shows an unavailable/expired message. Protect database backups as private mail and access credentials. Reader listener failures are logged and retried without stopping collection or delivery.

## Processing and commands

Each configured folder uses a separate IMAP connection. When the server supports IDLE, new-mail notifications trigger a UID scan immediately; a safety scan at the shorter of `poll_interval` and one minute recovers missed notifications. Otherwise the connection polls at `poll_interval`. Reconnection resumes from durable checkpoints. Delivery latency also depends on the mail provider, network and Telegram; no fixed latency is guaranteed.

In every mode, locally recognized verification emails send their full text without AI. Recognition checks the subject and first 4,000 body characters for common verification-code phrases; it can miss unfamiliar wording or match unrelated mail. A separate worker prevents ordinary AI analysis from blocking these messages. Workers share Telegram cooldowns; an ongoing send or rate limit can still delay delivery. Message-size and unread-only settings still apply.

Raw sends metadata and a 50-character preview without AI. Hybrid skips AI controls for short/code/login messages; other previews have Summary, Original and conditional Translate controls. Agent generates summaries automatically. AI failures fall back to a preview with an Original control. Analysis uses up to 2,000 body characters with a 30-second timeout and at most two simultaneous calls.

`/settings` and `/mode` open the settings dashboard with language/mode submenus, Back and Close. `/rules` lists preferences with Add/Delete/Close buttons. `/rules add` and `/rules delete` prompt for the next message; `/rules` cancels. Direct `/rules add TEXT`, `/rules delete NUMBER` and `/rules clear` also work. Reply to text with `/ai`; the result replies to the referenced message. `/help`, `/start`, `/status`, `/mode raw|hybrid|agent` and `/language auto|zh|en|ja` are available.

Settings update the active JSON file and database; JSON edits made while stopped take effect at startup. Rules live in `rules.md` beside the configuration. Prompt priority is configuration language constraints, ordered user preferences, then base instructions. Rules guide AI output; they are not deterministic mail filters. Commands are limited to configured chat IDs; all members of a configured group can operate the bot.

## JSON


See `config.example.json` at the repository root. Config and SQLite contain secrets/mail content and should remain private.

| Field | Default | Purpose |
| --- | --- | --- |
| poll_interval | 60 | Poll seconds without IDLE; also bounds the IDLE safety-scan interval, minimum 10 |
| max_retries | 3 | Fast retries before continued periodic reconnection |
| log_level | INFO | DEBUG, INFO, WARNING, ERROR, CRITICAL |
| network_timeout | 30 | IMAP operation and proxy handshake seconds |
| state_path | mailbot.sqlite3 | SQLite path relative to the configuration directory |
| unread_only | true | Fetch only unread messages among new UIDs; false also catches mail read elsewhere |
| mark_seen | true | Mark read only after durable enqueue; false preserves unread state |
| max_message_bytes | 0 | No default message-size limit; positive values replace oversized bodies with a notice |
| accounts | [] | name, email, password, imap_host, imap_port (993), use_ssl (true), folders (INBOX), enabled (true), web_url |
| notifiers | [] | type=telegram, enabled, telegram: bot_token, chat_id, api_base, timeout (30) |
| proxy | null | enabled, scheme (socks5/socks4/http), host, port, username/password |
| ai | disabled | enabled, provider, api_key, model, base_url, default_mode (hybrid), language (auto) |

`use_ssl: false` uses ordinary unencrypted IMAP, as in the existing configuration format. All three proxy schemes apply to IMAP, Telegram and AI with proxy-side destination DNS; no global socket or environment mutation is used. Without a proxy, connections are direct.

## DeepSeek AI

Only DeepSeek is supported, using OpenAI-compatible Chat Completions. AI setup asks for a key, model, base URL, mode and language.

- `provider`: `deepseek`. Legacy `openai` / `custom` values are aliases for the same protocol.
- `model`: defaults to `deepseek-chat`; other DeepSeek model names can be configured.
- `api_key`: falls back to `DEEPSEEK_API_KEY`.
- `base_url`: falls back to `DEEPSEEK_API_BASE`, then `https://api.deepseek.com`. Root, `/v1` and full `/chat/completions` endpoints are accepted, including private proxies.
- Requests use Bearer authentication, messages and JSON output. Explicit rejection of `response_format` triggers one retry without it; other failures use the original-mail fallback.

Other provider protocols, cloud authentication, device login and local model loading are outside the supported scope.

Rotating logs are stored in `logs/mailbot.log` beside the configuration: 5 MiB per file, three backups. See [deployment](../zh/go-service.md).
