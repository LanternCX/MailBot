# 开发与验证

## 结构

`cmd/mailbot` 为交互式或 headless 入口，`cmd/package` 生成跨平台发布包。`internal/mailbot` 包含配置、交互、IMAP / MIME、SQLite 队列、Telegram、AI 协议适配、代理和日志。`deploy/mailbot.service` 用于 Linux 常驻运行。

## 检查与构建

```sh
gofmt -w cmd internal
go vet ./...
go test -race -timeout 60s ./...
go test -cover ./...
go run ./cmd/package --variant linux-x64 --tag dev
```

测试不使用生产配置、邮箱或密钥。本地测试服务覆盖 IMAP、Telegram / AI HTTP 以及 SOCKS4a / SOCKS5 / HTTP CONNECT。DeepSeek 真实账号、模型可用性和出站网络仍需部署验证。

## 支持范围

邮件读取、MIME 解析、自动概括、按需翻译、Telegram 命令和按钮、配置向导、代理、持久化队列、独立重试与恢复由 Go 实现。AI 仅支持 DeepSeek 的 OpenAI 兼容 Chat Completions，默认模型 deepseek-chat；其他服务商、LiteLLM 全路由兼容、云认证和本地推理不属于支持范围。

原版 Python 源码可从 Git 历史读取。当前测试验证邮件和 Telegram 可观察行为、AI JSON 解析、DeepSeek 请求协议、错误回退、代理和队列恢复；本地模拟接口不能证明线上长期稳定性。

交付前运行上述全量检查，并用 cmd/package 构建四个平台。macOS 本机可运行版本输出与配置检查；其他平台交叉构建不等同于运行验证。
