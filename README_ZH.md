# 🤖 MailBot

![Python](https://img.shields.io/badge/Python-3.10+-3776AB?logo=python&logoColor=white)
![License](https://img.shields.io/badge/License-MIT-green)

[中文](README.md) | [English](README_EN.md)

> 一个简单易用的 IMAP 邮件转发 Telegram 机器人。
>
> An easy-to-use IMAP-to-Telegram mail forwarder with an interactive CLI wizard.

无需手动编辑配置文件——运行 `main` 程序并跟随向导操作即可。

---

## 🎬 功能预览

![预览](./docs/img/preview.png)

## ✨ 功能特性

- **邮件转发**：转发脚本启动后邮箱列表中收到的新邮件到聊天机器人中。
- **交互式配置**：通过命令行向导添加账户、设置 Bot，无需直接修改 JSON。
- **多账户支持**：同时监控多个 IMAP 邮箱和转发到多个 Bot 端（目前仅支持 Telegram）。
- **HTML 解析**：智能提取邮件正文，去除冗余标签。
- **Docker 友好**：支持 Headless 模式运行。

## 🚀 快速开始

### 方式一：直接运行

从 [Releases](../../releases) 下载对应系统的可执行文件，直接运行即可。macOS Gatekeeper 可能会阻止首次启动，执行下面两行命令可移除隔离属性并立即运行：

```bash
xattr -d com.apple.quarantine ./mailbot-macos-arm64
./mailbot-macos-arm64
```

运行可执行文件后会进入命令行交互式菜单，按照向导添加邮箱、Bot Token/Chat ID、系统设置，即可完成配置；详细流程见 [安装指南](docs/setup_guide.md) 和 [配置说明](docs/configuration.md) 中的各节说明。

### 方式二：源码运行

```bash
git clone https://github.com/your-username/MailBot.git
cd MailBot
python -m venv .venv && source .venv/bin/activate  # Windows: .venv\Scripts\activate
pip install -r requirements.txt
python main.py
```

## 📖 文档

- **[安装指南 (Setup Guide)](docs/setup_guide.md)**  
  包含 Gmail/Google App Password 获取教程，以及 Telegram Bot 申请与 User ID 获取方法。

- **[配置说明 (Configuration)](docs/configuration.md)**  
  详细介绍菜单选项、系统设置（轮询间隔、重试次数等）及 Headless 模式。

- **欢迎贡献 (Contribute)**  
  欢迎通过 PR 或 Issue 提交改进建议、配置示例或 Bug 报告，所有反馈都能帮助 MailBot 更好地适配更多场景。

## 🖥 菜单概览

```text
▸ Start Service      — 启动服务（前台运行，实时日志）
▸ Config Wizard      — 添加/删除邮箱账户
▸ Bot Settings       — 设置/修改 Telegram Bot Token & Chat ID
▸ System Settings    — 系统设置（轮询间隔、重试次数、日志级别）
▸ Test Connection    — 发送测试消息到 Telegram
▸ Exit               — 退出
```

## 🏗 项目结构

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