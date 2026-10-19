# Configuration and usage

Run the binary without arguments for the menu, or use `--headless -c PATH` for unattended operation. Configuration defaults to `config.json` in the working directory. `--check` validates without connecting or creating state. `--version` prints the build version.

## Menu

Choose by number or full option name. Enter accepts the displayed default; `:cancel` abandons the current dialog. Secrets are hidden on a real terminal and saved values are not printed.

The menu provides **Start Service** (Ctrl+C returns to the menu), **Config Wizard** (add/remove accounts and verify IMAP login), **Bot Settings** (set/remove bot), **AI Settings** (automatic summaries/translation, main language, and advanced connection settings), **System Settings** (polling, retries, console level and proxy), **Test Connection** (check Telegram and optionally send a test preview), and **Exit**. Cancelled dialogs do not save partial changes.

## Short notifications and full mail reading

Add `"reader": {"listen": "127.0.0.1:8080", "public_url": "https://mail.example.com"}` to the configuration. Point your HTTPS domain at a reverse proxy forwarding to that HTTP listener. For example, Caddy can serve `mail.example.com { reverse_proxy 127.0.0.1:8080 }`. The public URL must be an HTTPS origin without a path, credentials, query or fragment. Leave both fields empty to disable the reader.

New notifications include a concise automatic summary when AI is enabled, otherwise a plain preview. The reader adds a full-body URL button. Numeric and alphanumeric verification codes are detected locally, highlighted and accompanied by a copy button; these messages bypass AI. Previously persisted notifications keep their existing content and delivery progress.

The reading page preserves the original email and offers a translation button in your main language. A POST action generates the translation; viewing or crawling a link never starts an AI request. Successful summaries and translations are cached by email, language and operation, survive restarts, and are removed when the email expires. Original mail remains available on translation failure. Translation is plain text and never replaces or modifies the original HTML.

New mail stores both HTML and plain text. The reader sanitizes HTML in a sandboxed frame, preserving tables, safe inline styles, stylesheets and responsive media queries. CSS resource requests and external stylesheets are blocked. Newly received CID images support PNG, JPEG, GIF and WebP up to 5 MiB each; SVG and ordinary attachments are unavailable. Remote images load automatically through the server proxy. Browsers never connect directly to senders' image servers or disclose viewer IP addresses, cookies or page referrers to them. The first proxy download can still trigger tracking identifiers embedded in an image URL. The proxy only accepts HTTP/HTTPS image sources present in the authorized email. It blocks internal, loopback and link-local destinations and nonstandard ports, validates every redirect destination, and limits download time, bytes and image dimensions. Images are cached in SQLite for up to 30 days with a 64 MiB total payload limit; expired or evicted images are downloaded again when viewed. Scripts and forms remain disabled. Fixed-width messages may require horizontal scrolling. Older mail uses the stored body; HTML and inline images are not fetched again. Messages above `max_message_bytes` retain the existing size-limit notice and must be read in the original mailbox.

When the global `proxy` is enabled, image downloads reuse it. DNS resolution uses Cloudflare DNS (1.1.1.1) over TCP port 53 through that proxy. Only validated destination IPs are passed to the proxy, preventing a second hostname lookup from bypassing internal address restrictions.

Each mail has an unguessable persistent URL: anyone holding it can read the mail. Forwarding it shares access. No public mailbox listing is provided. Avoid logging full `/read/` paths in your reverse proxy. Links survive restarts with the SQLite database; completed mail expires 30 days after ingestion during periodic cleanup, while pending mail is retained. Removed or unknown mail shows an unavailable/expired message. Protect database backups as private mail and access credentials. Reader listener failures are logged and retried without stopping collection or delivery.

## Processing and commands

Each configured folder uses a separate IMAP connection. When the server supports IDLE, new-mail notifications trigger a UID scan immediately; a safety scan at the shorter of `poll_interval` and one minute recovers missed notifications. Otherwise the connection polls at `poll_interval`. Reconnection resumes from durable checkpoints. Delivery latency also depends on the mail provider, network and Telegram; no fixed latency is guaranteed.

Locally recognized verification emails bypass AI. Recognition checks the subject and first 4,000 body characters for common verification-code phrases; it can miss unfamiliar wording or match unrelated mail. A separate worker prevents ordinary AI analysis from blocking these messages. Workers share Telegram cooldowns; an ongoing send or rate limit can still delay delivery. Message-size and unread-only settings still apply.

Enabling AI automatically summarizes ordinary mail in 1–3 sentences in the main language. Summaries use the first 16,000 body characters with a 15-second total budget, including queueing; failures fall back to the original notification. Reader generation is serialized to avoid duplicate charges. Translation processes the full body in chunks of up to 3,000 characters, with a 75-second total budget and a 60,000-character input limit. Failed or partial translations are not cached. Retry explicitly if translation fails. Set the reverse proxy response timeout above 75 seconds for long translations.

`/settings` independently selects the interface language and AI output language (Chinese, English or Japanese). The top-level `ui_language` config field controls Telegram menus, notifications and the web reader; it defaults to Chinese when omitted. `ai.language` controls summaries and translation targets only. Both settings persist across restarts. The terminal menu's Interface Language entry changes Telegram and web language; the administrative terminal itself uses English. `/help`, `/start`, `/status` and `/language zh|en|ja` are available; `/language` changes AI output only. Legacy mode and rule controls only explain that these options are retired; they do not change the automatic workflow. Reply-based `/ai` remains available for compatibility but is not listed in the command menu.

Language changes update the active JSON file and database; JSON edits made while stopped take effect at startup. Old `default_mode` fields are accepted but ignored, and `rules.md` does not affect summaries or translations. Legacy `language: auto` uses Chinese as the main language. Commands are limited to configured chat IDs; all members of a configured group can operate the bot.

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
| ai | disabled | enabled, language (zh); advanced: provider, api_key, model, base_url |

`use_ssl: false` uses ordinary unencrypted IMAP, as in the existing configuration format. All three proxy schemes apply to IMAP, Telegram and AI with proxy-side destination DNS; no global socket or environment mutation is used. Without a proxy, connections are direct.

## DeepSeek AI

DeepSeek is the default, using OpenAI-compatible Chat Completions. Setup asks for your main language and a key if one is not saved. Model and endpoint are optional advanced settings; editing language or disabling AI preserves saved connection details.

- `provider`: `deepseek`. Legacy `openai` / `custom` values are aliases for the same protocol.
- `model`: defaults to `deepseek-chat`; other DeepSeek model names can be configured.
- `api_key`: falls back to `DEEPSEEK_API_KEY`.
- `base_url`: falls back to `DEEPSEEK_API_BASE`, then `https://api.deepseek.com`. Root, `/v1` and full `/chat/completions` endpoints are accepted, including private proxies.
- Requests use Bearer authentication, messages and JSON output. Explicit rejection of `response_format` triggers one retry without it; other failures use the original-mail fallback.

Other provider protocols, cloud authentication, device login and local model loading are outside the supported scope.

Rotating logs are stored in `logs/mailbot.log` beside the configuration: 5 MiB per file, three backups. See [deployment](../zh/go-service.md).
