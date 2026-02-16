# 🤖 MailBot

![Python](https://img.shields.io/badge/Python-3.10+-3776AB?logo=python&logoColor=white)
![License](https://img.shields.io/badge/License-MIT-green)

[中文](README.md) | [English](README_EN.md)

> 一个简单易用的 IMAP 邮件转发 Telegram 机器人。
>
> An easy-to-use IMAP-to-Telegram mail forwarder with an interactive CLI wizard.

无需手动编辑配置文件——运行 `main` 程序并跟随向导操作即可。

---

## ✨ 功能特性 (Features)

- **交互式配置**：通过命令行向导添加账户、设置 Bot，无需直接修改 JSON。
- **多账户支持**：同时监控多个 IMAP 邮箱。
- **HTML 解析**：智能提取邮件正文，去除冗余标签。
- **安全存储**：密码/Token 本地加密存储（可选）。
- **Docker 友好**：支持 Headless 模式运行。
- **按需转发**：仅转发脚本启动后的新邮件，避免历史邮件轰炸。

## 🚀 快速开始 (Quick Start)

### 方式一：直接运行 (Executable)

从 [Releases](../../releases) 下载对应系统的可执行文件，直接运行即可。

### 方式二：源码运行 (Source)

```bash
git clone https://github.com/your-username/MailBot.git
cd MailBot
python -m venv .venv && source .venv/bin/activate  # Windows: .venv\Scripts\activate
pip install -r requirements.txt
python main.py
```

## 📖 文档 (Documentation)

- **[安装指南 (Setup Guide)](docs/setup_guide.md)**  
  包含 Gmail/Google App Password 获取教程，以及 Telegram Bot 申请与 User ID 获取方法。

- **[配置说明 (Configuration)](docs/configuration.md)**  
  详细介绍菜单选项、系统设置（轮询间隔、重试次数等）及 Headless 模式。

---

## 🖥 菜单概览 (Menu)

```text
▸ Start Service      — 启动服务（前台运行，实时日志）
▸ Config Wizard      — 添加/删除邮箱账户
▸ Bot Settings       — 设置/修改 Telegram Bot Token & Chat ID
▸ System Settings    — 系统设置（轮询间隔、重试次数、日志级别）
▸ Test Connection    — 发送测试消息到 Telegram
▸ Exit               — 退出
```

## 🏗 项目结构 (Project Structure)

```text
MailBot/
├── main.py                 # 程序入口
├── config.json             # 自动生成的配置文件（含账户与密钥）
├── requirements.txt
├── docs/                   # 项目文档（setup_guide, configuration）
├── utils/
│   ├── logger.py           # Rich 日志配置
│   └── helpers.py          # 实用工具（UI组件等）
├── core/                   # 核心逻辑
│   ├── models.py           # 数据模型
│   ├── manager.py          # 服务管理
│   ├── fetcher.py          # IMAP 抓取与重试机制
│   ├── parser.py           # 邮件体 HTML 解析
│   └── notifiers/          # 通知适配器（Telegram）
└── interface/              # UI 交互层
    ├── menu.py             # 主菜单（questionary）
    └── wizard.py           # 向导组件
```

## 📄 License

MIT

