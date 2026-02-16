# 🤖 MailBot

![Python](https://img.shields.io/badge/Python-3.10+-3776AB?logo=python&logoColor=white)
![License](https://img.shields.io/badge/License-MIT-green)

[中文](README_ZH.md) | [English](README.md)

> An easy-to-use IMAP-to-Telegram mail forwarder with an interactive CLI wizard.

No YAML editing required — just run the executable or `main.py` and follow the menu.

---

## ✨ Features

- **Interactive Setup**: Add accounts and configure your bot via a clear CLI wizard.
- **Multi-Account**: Monitor multiple IMAP accounts simultaneously.
- **Smart Parsing**: Extracts clean text from HTML emails.
- **Secure Storage**: Credentials encrypted locally (optional).
- **Docker Friendly**: Supports headless mode for server deployments.
- **Forward New Only**: Forwards only new emails arriving after the script starts, avoiding spam from history.

## 🚀 Quick Start

### Method 1: Executable (Recommended)

Download the latest release for your OS from [Releases](../../releases) and run it directly.

### Method 2: From Source

```bash
git clone https://github.com/your-username/MailBot.git
cd MailBot
python -m venv .venv && source .venv/bin/activate  # Windows: .venv\Scripts\activate
pip install -r requirements.txt
python main.py
```

## 📖 Documentation

- **[Setup Guide](docs/setup_guide.md)**  
  Get your Google App Password, create a Telegram Bot, and find your Chat ID.

- **[Configuration](docs/configuration.md)**  
  Detailed explanation of menu options, system settings (polling, retries), and headless mode.

---

## 🖥 Menu Overview

```text
▸ Start Service      — Run in foreground with live logs
▸ Config Wizard      — Add / remove IMAP accounts step-by-step
▸ Bot Settings       — Set / update Telegram Bot Token & Chat ID
▸ System Settings    — Configure polling interval, retries, log level
▸ Test Connection    — Send a test message to Telegram to verify setup
▸ Exit               — Quit application
```

## 🏗 Project Structure

```text
MailBot/
├── main.py                 # Entry point
├── config.json             # Auto-generated config (credentials)
├── requirements.txt
├── docs/                   # Documentation (setup, config guides)
├── utils/
│   ├── logger.py           # Rich logging setup
│   └── helpers.py          # UI components
├── core/                   # Core logic
│   ├── models.py           # Data models
│   ├── manager.py          # Service orchestration
│   ├── fetcher.py          # IMAP fetching & retry logic
│   ├── parser.py           # HTML body parsing
│   └── notifiers/          # Notification adapters (Telegram)
└── interface/              # Interactive CLI layer
    ├── menu.py             # Main menu
    └── wizard.py           # Configuration wizards
```

## 📄 License

MIT
