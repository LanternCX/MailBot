# MailBot

[English](README.md) · [配置文档](docs/zh/configuration.md) · [邮箱与 Telegram 设置](docs/zh/setup_guide.md) · [服务部署](docs/zh/go-service.md)

MailBot 将多个 IMAP 邮箱的新邮件转发到 Telegram，可按需生成摘要、提取验证码和翻译。程序使用 Go，运行时不需要 Python 或外部数据库。

## 功能

- 多账号、多文件夹独立使用 IMAP IDLE 实时监听，断线重连补抓，不支持时回退轮询。
- 本地识别验证码邮件，跳过 AI、完整转发正文，独立发送任务与普通邮件共享 Telegram 限流。
- Raw：50 字邮件预览；Hybrid：短邮件直接预览，长邮件提供按需摘要、原文和翻译；Agent：自动摘要与原文、翻译按钮。
- Telegram 多级设置面板、规则的分步输入和 `/ai` 回复分析。
- 交互式账号、机器人、AI、代理配置，以及 IMAP / Telegram 连接测试。
- 仅支持 DeepSeek AI，使用 OpenAI 兼容的 Chat Completions API。
- SQLite 保存抓取进度和待发送任务，失败重试、重启恢复、分段发送；日志轮转和 systemd 服务管理。

## 快速开始

下载并解压对应平台的发布包，运行其中的程序打开配置菜单。也可从源码构建（Go 1.26+）：

```sh
go build -trimpath -o dist/mailbot ./cmd/mailbot
./dist/mailbot
```

在菜单中添加邮箱和 Telegram bot 后选择 **Start Service**。服务器运行使用：

```sh
./dist/mailbot --config config.json --check
./dist/mailbot --headless --config config.json
```

首次接管邮箱会跳过已有邮件；之后按持久化进度补抓。配置可以沿用原 JSON 文件。详细的迁移边界、投递保证和 systemd 安装方式见[服务文档](docs/zh/go-service.md)。

## 开发

```sh
go test -race ./...
go vet ./...
go run ./cmd/package --variant linux-x64 --tag dev
```

构建支持 Linux x64 / ARM64、macOS ARM64、Windows x64。开发与功能验证说明见[开发指南](docs/dev/development_zh.md)。

MIT License.
