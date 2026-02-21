# 配置与使用

MailBot 提供交互式 CLI 菜单引导你完成设置。

## 菜单选项

**主菜单：**
1.  **Start Service (启动服务)**：启动 IMAP 监听。日志将实时显示在控制台中。
2.  **Config Wizard (配置向导)**：添加或删除邮箱账户。
3.  **Bot Settings (机器人设置)**：管理你的 Telegram Bot 配置。
4.  **AI Settings (AI 设置)**：管理 AI 模型、API Key 和行为规则。
5.  **System Settings (系统设置)**：高级配置 (轮询频率、重试次数、日志级别)。
6.  **Test Connection (测试连接)**：验证你的 Telegram 设置是否生效。
7.  **Exit (退出)**：退出程序。

---

## 详细设置

### 1. Config Wizard (配置向导)

在此添加或管理你的 IMAP 账户。你需要：
*   **Provider (服务商)**：如 Gmail (需使用应用专用密码)、Outlook 等。
*   **Email Address (邮箱地址)**：完整的邮箱地址。
*   **Password (密码)**：你的 **App Password (应用专用密码)** (而非登录账号密码)。
*   **IMAP Settings (IMAP 设置)**：通常可以自动检测。

### 2. Bot Settings (机器人设置)

配置你的 Telegram 通知渠道。首先需要启动你的机器人：
1.  在 Telegram 中搜索并打开你的机器人，发送 `/start` 来启动对话。
2.  然后在 MailBot 菜单中配置以下信息：
    *   **Bot Token (机器人令牌)**：来自 BotFather 的 HTTP API Token。
    *   **Chat ID (用户 ID)**：你的 Telegram 用户 ID（可通过 @userinfobot 获取）。

### 3. AI Settings (AI 设置)

通过 LiteLLM 的双级菜单配置 AI 分析。
*   **Enable/Disable (开关)**：开启或关闭 AI 分析。
*   **Provider Group → Platform (分组 → 平台)**：先选分组再选具体平台。预设覆盖 OpenAI/OpenRouter/Together/Fireworks，Anthropic/Gemini/Mistral/Groq，DeepSeek/Qwen/Moonshot/Minimax，Perplexity/Cohere，以及 Ollama 与自定义 OpenAI 兼容端点。
*   **API Key**：仅在所选平台需要时提示输入。
*   **Model (模型)**：可覆盖菜单显示的默认模型名。
*   **Base URL**：Ollama、OpenRouter、Together、Fireworks、自定义 OpenAI 兼容端点均可编辑。
*   **Default Mode & Language (默认模式与语言)**：选择邮件处理模式：
    - **Raw (极速模式)**：简单转发，无 AI 分析（零 Token 成本）
    - **Hybrid (混合模式)**：邮件预览 + 按需 AI（**推荐给想省钱的用户**）
      - 邮件预览 + 三个按钮：摘要、原文、翻译
      - 摘要按钮：按需 AI 分析（仅点击时消耗 Token）
      - 原文按钮：显示完整邮件（无成本，使用缓存）
      - 翻译按钮：仅在邮件语言与目标语言不同时显示（点击时消耗 Token）
      - **节省对比**：相比智能模式，省 80-90% Token（不点击 AI 时）
    - **Agent (智能模式)**：对每封邮件自动进行 AI 摘要（每封邮件消耗 Token）
      - 每封邮件自动生成 AI 摘要
      - 原文按钮：显示完整邮件（无成本）
      - 翻译按钮：手动翻译（点击时消耗 Token）
    
    同时选择输出语言 (自动检测，或指定特定语言)。

### 4. Rules (规则系统)

MailBot 支持通过 Telegram 动态管理 AI 的行为规则，无需手动编辑文件。
*   在 Telegram 中发送 `/rules` 可以查看当前生效的规则。
*   跟随 Bot 的指引，通过自然语言对话来添加或删除规则。
*   这些规则将保存到本地的 `rules.md` 中，并在 AI 处理邮件时生效。

*   **规则执行顺序**：规则按照在 `rules.md` 中出现的顺序依次执行。如果多个规则匹配同一封邮件，所有适用的规则都会按顺序执行。
*   **规则优先级**：更具体的规则 (如同时匹配特定发件人和关键字) 应该放在通用规则之前，以避免冲突。

例如：
> "添加规则：忽略所有来自 no-reply@example.com 的邮件"

> "添加规则：遇到包含 '验证码' 的邮件，请提取验证码并加粗显示"

> "添加规则：标记包含项目截止日期的邮件为 ⚠️"

### 5. System Settings (系统设置)

自定义邮件获取器的行为。
*   **Polling Interval (轮询间隔)**：检查新邮件的频率 (单位：秒)。(默认值：60s，最小值：10s)
    - **建议**：
      - **10-30s**：对于重要账户，需要立即获得通知 (如安全警报、紧急工作邮件)
      - **60s**：平衡型设置，适合大多数用户。良好的收件箱监控。
      - **300s 及以上 (5 分钟+)**：用于邮件量少的账户或希望减少 API 调用频率的场景。
*   **Max Retries (最大重试次数)**：在放弃该抓取周期前，网络失败时的最大重试次数。(默认值：3，建议值：2-5)
    - 更高的值能在不稳定网络上提高可靠性，但可能延迟错误检测。
*   **Log Level (日志级别)**：控制台输出的日志级别 (DEBUG, INFO, WARNING, ERROR)。
    - 设置为 **DEBUG** 以排查连接或 API 问题。
    - 生产环境建议使用 **INFO** (正常操作)。

---

## Headless 模式运行

适用于服务器或 Docker 等无交互环境。

1.  首先使用交互式向导完成配置。
2.  添加 `--headless` 标志运行：
    ```bash
    python main.py --headless
    ```
    或者指定自定义配置文件：
    ```bash
    python main.py --headless -c /path/to/my_config.json
    ```
