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

## 本地 PyInstaller 打包与测试

在推送到 GitHub Actions 之前，可以在本地构建单文件可执行程序来验证打包是否正确，尽早发现隐藏依赖或数据文件缺失等问题。

### 前置条件

```bash
# 确保在项目根目录，且已激活 venv
pip install pyinstaller
pip install -r requirements.txt
```

### 构建完整二进制

```bash
.venv/bin/python -m PyInstaller --clean --onefile \
    --additional-hooks-dir hooks \
    --collect-all rich \
    --hidden-import litellm \
    --collect-data litellm \
    --name MailBot main.py
```

可执行文件位于 `dist/MailBot`，运行：

```bash
./dist/MailBot --help
./dist/MailBot --headless -c config.json
```

### 快速冒烟测试 (仅 litellm)

项目提供了一个专用冒烟测试脚本，用于验证 litellm 在 PyInstaller bundle 中能否正常 import 并加载 cost-map 数据：

```bash
# 构建冒烟测试二进制
.venv/bin/python -m PyInstaller --clean --onefile \
    --additional-hooks-dir hooks \
    --hidden-import litellm \
    --collect-data litellm \
    --name smoketest scripts/smoketest_litellm.py

# 运行 — 应输出 "OK"
./dist/smoketest
```

期望输出：

```
frozen=True  _MEIPASS=/var/folders/.../...
OK — litellm <version> imported successfully
model_cost type=dict  entries=NNNN
```

如果出现 `FAIL — FileNotFoundError` 或 `FAIL — ImportError`，说明打包参数或 `hooks/hook-litellm.py` 需要修复。

### 或使用打包脚本

```bash
.venv/bin/python scripts/package.py --clean --entry main.py --variant macos-arm64
```

生成的 zip 包位于 `dist/macos-arm64/`。

> **提示：** 测试完成后可用 `rm -rf dist/ build/` 清理构建产物。
