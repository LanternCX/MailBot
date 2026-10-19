# MailBot

[中文](README_ZH.md) · [Configuration](docs/en/configuration.md) · [Mailbox and Telegram setup](docs/en/setup_guide.md) · [Service deployment (Chinese)](docs/zh/go-service.md)

MailBot forwards new messages from multiple IMAP mailboxes to Telegram, with optional summaries, code extraction and translation. It runs as a standalone Go binary with embedded SQLite; no Python or external database is required.

## Features

- IMAP IDLE watches each account and folder on its own connection, with reconnect catch-up and polling fallback.
- Locally recognized verification emails bypass AI and send their full text through a separate delivery worker, sharing Telegram rate limits.
- Automatic concise summaries in your main language, full HTML mail reading and on-demand translation with persistent caching. AI failures preserve the original notification.
- Telegram settings keep interface language and AI output language separate; model and endpoint settings are advanced options. No mode or custom-rule setup is needed.
- Interactive account, bot, AI and proxy setup, including IMAP and Telegram connection tests.
- DeepSeek AI through the OpenAI-compatible Chat Completions API.
- Durable queue and checkpoints, retry after failures, restart recovery, multipart progress, rotating logs and a systemd unit.

## Run

Download and extract a release archive for your platform. Run the executable to open the configuration menu. To build from source, install Go 1.26+:

```sh
go build -trimpath -o dist/mailbot ./cmd/mailbot
./dist/mailbot
```

Configure accounts and a Telegram bot, then choose **Start Service**. For unattended operation:

```sh
./dist/mailbot --config config.json --check
./dist/mailbot --headless --config config.json
```

Existing JSON configuration is accepted; enabled AI must use DeepSeek. The first scan establishes a baseline and skips historical mail; subsequent runs resume from durable checkpoints. See the [deployment guide](docs/zh/go-service.md) for migration limits and delivery guarantees.

## Develop

```sh
go test -race ./...
go vet ./...
go run ./cmd/package --variant linux-x64 --tag dev
```

Release targets: Linux x64 / ARM64, macOS ARM64 and Windows x64. See the [development guide (Chinese)](docs/dev/development_zh.md).

MIT License.
