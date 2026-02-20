# 🤖 MailBot

![Python](https://img.shields.io/badge/Python-3.10+-3776AB?logo=python&logoColor=white)
![License](https://img.shields.io/badge/License-MIT-green)

[中文](README_ZH.md) | [English](README.md)

> 你的 AI 邮件管理 Agent

一个能够理解、总结并按照你的规则处理邮件的智能助手。

---

## 🎬 功能预览

![预览](./docs/img/preview.png)

## ✨ 核心特性

- **三种处理模式**：
  - **极速模式 (Raw)**：简单转发，零 Token 成本（无 AI）
  - **混合模式 (Hybrid)**：邮件预览 + 按需 AI（**推荐：省钱 80-90%**）
  - **智能模式 (Agent)**：自动 AI 摘要 + 手动翻译

- **智能代理 (Agent)**：基于 LLM 的邮件内容理解、重要性识别与摘要。
- **极低成本**：使用提示词极少，Token 成本极低。混合模式可节省 80-90% Token。
- **多模型支持**：无缝接入 OpenAI, Claude, Gemini, DeepSeek, Ollama 等主流模型。
- **规则引擎**：在 Telegram 中通过自然语言定义你的邮件处理偏好。
- **智能多语言**：自动语言检测与按需翻译，打破阅读障碍。
- **交互式配置**：通过命令行向导轻松管理账户与 AI 设置。
- **隐私优先**：支持本地模型 (Ollama) 与自建 API，数据自主掌控。
- **动态界面**：根据邮件内容和语言智能显示/隐藏按钮。

## 🧠 支持模型列表

MailBot 基于 LiteLLM 提供了广泛的模型支持：

| 模型分组 | 支持的服务商 |
| :--- | :--- |
| **OpenAI & Compatible** | OpenAI, OpenRouter, Together AI, Fireworks AI |
| **Frontier Models** | Anthropic (Claude), Google Gemini, Mistral, Groq |
| **China-Friendly** | DeepSeek, Qwen (通义千问), Moonshot (Kimi), MiniMax |
| **Research / Web** | Perplexity, Cohere |
| **Local & Custom** | Ollama (本地), 自定义 OpenAI 兼容接口 |

完整的支持列表请参考 [LiteLLM Providers 文档](https://docs.litellm.ai/docs/providers)。

## 🚀 快速开始

### 方式一：直接运行

1. **下载与解压**  
   从 [Releases](../../releases/latest) 下载对应系统的发布包并解压。

   ```bash
   # 示例：解压下载的文件
   unzip mailbot-v1.0.0-macos-arm64.zip -d mailbot
   cd mailbot
   ```

2. **运行程序**  
   直接执行二进制文件，首次运行会自动生成配置文件。

   **Linux / Windows (PowerShell):**
   ```bash
   # Linux
   chmod +x mailbot-linux-x64
   ./mailbot-linux-x64
   
   # Windows
   .\mailbot-windows-x64.exe
   ```

   **macOS (需要清除 Gatekeeper 属性):**
   ```bash
   # 移除隔离属性
   xattr -d com.apple.quarantine mailbot-macos-arm64
   
   # 运行
   ./mailbot-macos-arm64
   ```

3. **配置向导**  
   跟随命令行交互向导，添加邮箱账户并设置 Bot Token。
   ```bash
   # 可选：指定特定配置文件启动
   ./mailbot-macos-arm64 -c ./my_config.json
   ```

### 方式二：源码运行

```bash
git clone https://github.com/your-username/MailBot.git
cd MailBot
python -m venv .venv && source .venv/bin/activate  # Windows: .venv\Scripts\activate
pip install -r requirements.txt
python main.py
```

## 📖 文档

- **[安装指南](docs/zh/setup_guide.md)**  
  包含 Gmail/Google App Password 获取教程，以及 Telegram Bot 申请与 User ID 获取方法。

- **[配置说明](docs/zh/configuration.md)**  
  详细介绍菜单选项、系统设置（轮询间隔、重试次数等）及 Headless 模式。

- **欢迎贡献**  
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

## 🤖 Telegram 指令

| 指令 | 说明 |
| :--- | :--- |
| `/start` | 检查机器人是否在线 |
| `/settings` | 打开交互式设置面板（切换模式/语言） |
| `/ai` | (回复某条消息) 手动触发 AI 分析 |
| `/rules` | 查看或管理 AI 处理规则 (支持自然语言增删) |
| `/help` | 显示帮助信息 |

## 📊 三种运行模式

### 极速模式 (Raw - Simple Forwarding)
- **成本**：零 Token ——不进行 AI 分析
- **使用场景**：快速获取邮件通知，无需内容分析
- **操作**：直接转发邮件到 Telegram

### 混合模式 (Hybrid - Smart Preview) — **推荐（最省钱）**
- **成本**：零 Token 起步；由用户决定是否点击按钮消耗 Token
- **功能**：
  - 邮件预览 + 三个按钮：**摘要**、**原文**、**翻译**
  - 摘要按钮：按需 AI 分析（点击时消耗 Token）
  - 原文按钮：显示完整邮件内容（无成本，使用缓存）
  - 翻译按钮：仅在邮件语言与目标语言不同时显示（点击时消耗 Token）
- **节省**：相比智能模式，省 80-90% Token（不使用 AI 时）
- **适用人群**：既要方便又要省钱的用户

### 智能模式 (Agent - Automatic AI)
- **成本**：每封邮件自动分析，会产生 Token 成本
- **功能**：
  - 自动对每封邮件进行 AI 摘要
  - 原文按钮：显示完整邮件（无成本）
  - 翻译按钮：手动翻译（点击时消耗 Token）
- **适用人群**：追求体验，不在乎 Token 成本的用户

## 🏗 项目架构

![项目架构](./docs/img/framework.jpg)

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
│   ├── ai.py               # AI 分析与规则管理
│   ├── bot.py              # Telegram 机器人处理
│   ├── rules.py            # 规则引擎
│   └── notifiers/          # 通知适配器（Telegram）
├── interface/              # UI 交互层
│   ├── menu.py             # 主菜单（questionary）
│   └── wizard.py           # 向导组件
├── test/                   # 测试文件
├── scripts/                # 实用脚本
├── hooks/                  # Git hooks & 构建脚本
└── logs/                   # 应用日志（自动生成）
```

## 📄 License

MIT