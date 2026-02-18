# 安装指南

MailBot 默认在当前工作目录查找 `config.json`；如果希望指定其他位置，请在启动时通过 `-c PATH` / `--config PATH` 参数指定。如果文件丢失，CLI 菜单或 Headless 模式运行会自动创建。

## Google/Gmail

要使用 Gmail 账户，你必须开启 **两步验证** 并生成 **应用专用密码**。  
现已不再支持使用主 Google 账号密码进行 IMAP 访问。

1.  **开启两步验证**：前往你的 Google 账号 > 安全设置。
2.  **创建应用专用密码**：参考官方指南：
    [**使用应用专用密码登录 (Google 支持)**](https://support.google.com/mail/answer/185833?hl=zh-Hans)
3.  **使用应用专用密码**：当 MailBot 询问密码时，输入生成的 16 位密码。

---

## Telegram Bot

你需要一个 Telegram Bot Token 来发送消息，以及你的 User ID (Chat ID) 来接收消息。

### 1. 创建机器人 (获取 Token)
1.  打开 Telegram 并搜索 [`@BotFather`](https://t.me/BotFather)。
2.  发送命令 `/newbot`。
3.  按照提示为你的机器人命名。
4.  复制 BotFather 提供的 **HTTP API Token** (例如：`123456789:ABCdefGHIjklMNOpqrSTUvwxyz`)。

### 2. 获取你的 Chat ID
1.  开始与你的新机器人对话并发送一条消息 (例如：`/start`)。
2.  在 Telegram 上搜索 [`@userinfobot`](https://t.me/userinfobot)。
3.  向它发送任何消息。它会回复你的 `Id` (例如：`12345678`)，或者你也可以使用 API 方法：
    *   访问：`https://api.telegram.org/bot<YOUR_TOKEN>/getUpdates`
    *   查找 `"chat"` 下的 `"id"` 字段。
4.  在 MailBot 提示时输入此 ID。

---

## 代理与网络

如果你的 IMAP 服务商或 Telegram 端点必须通过 SOCKS/HTTP 代理访问，请在配置中添加 `proxy` 块或通过菜单的系统设置启用。它支持：

* `scheme (协议方案)`：`socks5`, `socks4`, 或 `http`
* `host`/`port` (主机/端口)：代理地址/端口
* `username` / `password` (用户名/密码)：可选的认证信息

启用代理后，MailBot 会自动设置 `http(s)_proxy` 环境变量，并通过代理路由 IMAP 套接字，因此所有网络流量 (IMAP 获取 + Telegram API) 都会遵循该配置。如果你更喜欢交互式操作，请使用 CLI 向导的系统设置。

## 其他邮箱服务商

对于其他服务商 (Outlook, Yahoo, iCloud 等)，请确保：
1.  在账号设置中开启了 **IMAP 访问**。
2.  如果开启了两步验证 (强烈建议)，请使用 **应用专用密码**。


---

## 更多设置

有关详细的配置选项，请参阅 [配置文档](configuration.md)。
