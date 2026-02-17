# 配置与使用

MailBot 提供交互式 CLI 菜单引导你完成设置。

## 菜单选项

**主菜单：**
1.  **Start Service (启动服务)**：启动 IMAP 监听。日志将实时显示在控制台中。
2.  **Config Wizard (配置向导)**：添加或删除邮箱账户。
3.  **Bot Settings (机器人设置)**：管理你的 Telegram Bot 配置。
4.  **System Settings (系统设置)**：高级配置 (轮询频率、重试次数、日志级别)。
5.  **Test Connection (测试连接)**：验证你的 Telegram 设置是否生效。
6.  **Exit (退出)**：退出程序。

---

## 详细设置

### 1. Config Wizard (配置向导)

在此添加或管理你的 IMAP 账户。你需要：
*   **Provider (服务商)**：如 Gmail (需使用应用专用密码)、Outlook 等。
*   **Email Address (邮箱地址)**：完整的邮箱地址。
*   **Password (密码)**：你的 **App Password (应用专用密码)** (而非登录账号密码)。
*   **IMAP Settings (IMAP 设置)**：通常可以自动检测。

### 2. Bot Settings (机器人设置)

配置你的 Telegram 通知渠道。
*   **Chat ID (用户 ID)**：输入你的 Telegram 用户 ID。
*   **Bot Token (机器人令牌)**：来自 BotFather 的 API Token。

### 3. System Settings (系统设置)

自定义邮件获取器的行为。
*   **Polling Interval (轮询间隔)**：检查新邮件的频率 (单位：秒)。(默认值：60s，最小值：10s)
*   **Max Retries (最大重试次数)**：在放弃该抓取周期前，网络失败时的最大重试次数。(默认值：3)
*   **Log Level (日志级别)**：控制台输出的日志级别 (DEBUG, INFO, WARNING, ERROR)。在排查连接问题时请设置为 DEBUG。

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
