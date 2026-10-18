# 配置与使用

默认读取当前工作目录的 `config.json`，也可用 `-c PATH` / `--config PATH` 指定。无参数启动打开交互菜单；服务器使用 `--headless`。配置文件不存在时，可直接使用菜单创建配置。`--check` 校验文件而不连接网络、创建数据库或日志。

## 交互菜单

输入菜单编号或完整选项名，Enter 使用提示中的默认值。`:cancel` 放弃当前对话，EOF 退出。真实终端中的密码和令牌输入不回显；已保存的密钥不会打印出来。

| 菜单 | 功能 |
| --- | --- |
| Start Service | 启动，Ctrl+C 停止并返回菜单 |
| Config Wizard | 查看、添加、删除账号；Gmail / Outlook / QQ / 163 预设或自定义 IMAP；可立即验证登录 |
| Bot Settings | 配置或删除 Telegram bot、chat_id 和 API 地址 |
| AI Settings | 开关 DeepSeek AI，设置模型、密钥、地址、模式和语言 |
| System Settings | 轮询间隔、重试次数、日志级别、代理 |
| Test Connection | 验证 Telegram bot，可发送测试预览 |

配置修改在完成对话后保存；取消不保存部分输入。

## 简短通知与完整阅读

在配置中加入：

```json
"reader": {
  "listen": "127.0.0.1:8080",
  "public_url": "https://mail.example.com"
}
```

将自己的 HTTPS 子域名反向代理到监听地址。例如 Caddy：

```caddyfile
mail.example.com {
    reverse_proxy 127.0.0.1:8080
}
```

域名需指向部署机器，反向代理需能申请 HTTPS 证书。公共地址必须为 HTTPS 根地址，不带路径、查询参数或账号密码。阅读服务监听 HTTP，通常仅绑定回环地址，由反向代理提供 HTTPS；容器部署可调整监听地址。两项均留空则关闭阅读服务。

启用后，新生成的 Telegram 通知统一为短卡片，提供「查看正文」按钮，不等待 AI，也不显示摘要、翻译或原文回调按钮。验证码从标题和正文的本地关键词附近识别，支持数字及字母数字混合码，以代码格式突出显示，并提供「复制验证码」按钮；无法识别时保留预览和正文入口。此前已保存的待投递消息仍按原进度发送。

新邮件保留 HTML 与纯文本；网页过滤危险内容，并隔离展示 HTML，保留安全的文字、表格、内联样式、样式表与响应式媒体查询。CSS 中的资源请求和外部样式表不加载。新收到的 CID 内嵌图片支持 PNG、JPEG、GIF、WebP，每张不超过 5 MiB；SVG 与普通附件不提供。外部图片自动经服务器代理加载。浏览器不会直接连接发件方的图片服务器，也不会向其发送用户 IP、Cookie 或页面来源；首次下载仍可能触发邮件中的追踪标记。代理仅接受该邮件中的 HTTP/HTTPS 图片地址，禁止内网、回环、链路本地等地址及非标准端口，每次重定向重新检查目标，并限制下载时间、大小和图片尺寸。图片缓存保存在 SQLite 中，最长 30 天，总量不超过 64 MiB，过期或被淘汰后再次查看会重新下载。固定宽度邮件可能需要在正文区域横向滚动。历史邮件显示已有正文，不重新抓取 HTML 或内嵌图片。超过 `max_message_bytes` 的邮件仍显示大小限制提示，完整内容需前往原邮箱查看。

启用全局 `proxy` 时，图片下载复用该出网代理，域名通过代理连接 Cloudflare DNS（1.1.1.1）的 TCP 53 端口解析；校验通过后仅将目标 IP 交给代理，避免二次解析绕过内网限制。

每封邮件使用随机专属链接，持有链接即可阅读，转发链接会分享正文访问权限。页面无公开邮件列表，不加载外部字体，邮件脚本不能执行；反向代理日志也应避免记录 `/read/` 下的完整路径。链接和正文随 SQLite 保存，重启不失效。已完成邮件按入库时间保留 30 天，待投递邮件保留至完成；定时清理后页面显示“邮件不存在或已过期”。备份数据库也包含这些访问链接，应同邮件正文一样保护。

阅读服务监听失败会记录日志并重试，不停止邮件收取和 Telegram 投递。阅读功能尚未配置时，沿用下面的邮件模式。

## 邮件模式与 Telegram

每个账号的每个文件夹使用独立 IMAP 连接。服务器支持 IDLE 时，收到新邮件通知立即查询新增 UID，并按 `poll_interval` 与一分钟中较短的间隔补查，避免漏通知；不支持时按 `poll_interval` 轮询。断线后从持久化进度补抓。实际延迟还受邮箱服务器、网络与 Telegram 影响，不保证固定秒数。

所有模式下，本地识别为验证码的邮件均跳过 AI，完整转发正文。识别检查标题及正文前 4,000 字中的常见验证码关键词，可能漏判或误判。验证码使用独立发送任务，不受普通邮件 AI 分析阻塞，但仍遵循 Telegram 限流，也可能等待正在进行的发送。邮件大小限制和只抓未读配置仍生效。

Raw 发送发件人、标题、时间、50 字正文预览和网页链接，不调用 AI。

Hybrid 对少于 100 字或前 300 字包含验证码、登录等模式的邮件直接发送预览；其他邮件提供摘要、原文和翻译按钮。原文不消耗 AI 调用。翻译按钮依据简单语言检测与目标语言比较显示。

Agent 自动生成摘要、类别和提取出的验证码，提供原文和条件翻译按钮。AI 失败时发送预览并保留原文入口，不阻塞收信。每次 AI 请求最多使用前 2,000 个正文字符，超时为 30 秒，全局并发上限为两个。

| 命令 | 用法 |
| --- | --- |
| `/settings`、`/mode` | 打开设置面板，进入语言或模式子菜单，可返回或关闭；也支持 `/mode raw|hybrid|agent` |
| `/rules` | 查看编号规则，使用 Add / Delete 按钮，或 `/rules add`、`/rules delete` 进入下一条消息输入；再次 `/rules` 取消 |
| `/rules add TEXT`、`/rules delete NUMBER`、`/rules clear` | 直接管理规则 |
| `/ai` | 回复一条有文本的消息进行分析，结果回复到原消息 |
| `/help`、`/start` | 帮助 |
| `/status` | 待投递数量、各文件夹最近成功检查、各目标最近成功发送 |
| `/language auto|zh|en|ja` | 设置语言的快捷命令 |

设置按钮原地更新面板，摘要、翻译、原文作为回复发送。语言和模式写回实际使用的 JSON 文件，并同步到数据库；停机后手动编辑 JSON 会在下次启动生效。关闭 AI 后默认使用 Raw。只有配置中允许的 chat_id 可操作；群组中的所有成员都能操作，请使用受信任的群组。

规则文件为配置文件旁的 `rules.md`，按编号顺序放入 AI 提示词。配置中的语言约束优先，其次是用户规则，再次是基础提示词。规则影响 AI 输出，不是确定性的邮件过滤器。

## JSON 字段


完整结构见仓库根目录的 `config.example.json`。密钥以明文保存，程序写入配置使用 0600 权限。

| 字段 | 默认值 | 含义 |
| --- | --- | --- |
| `poll_interval` | `60` | 不支持 IDLE 时的轮询秒数，同时限制 IDLE 补查间隔，至少 10 |
| `max_retries` | `3` | 快速重试次数，失败后仍继续周期性重连 |
| `log_level` | `INFO` | 控制台级别：DEBUG / INFO / WARNING / ERROR / CRITICAL |
| `network_timeout` | `30` | IMAP 网络操作与代理握手超时秒数 |
| `state_path` | `mailbot.sqlite3` | 数据库文件，相对路径以配置文件目录为准 |
| `unread_only` | `true` | 只抓取新 UID 中仍未读的邮件；设 false 也补抓已被其他客户端读过的新邮件 |
| `mark_seen` | `true` | 在待投递任务持久化成功后，将邮件标记为已读；false 保留邮箱未读状态 |
| `max_message_bytes` | `0` | 默认不限制单封邮件大小；设置正整数后，超限邮件发送提示与原文入口 |
| `accounts` | `[]` | name、email、password、imap_host、imap_port（993）、use_ssl（true）、folders（INBOX）、enabled（true）、web_url |
| `notifiers` | `[]` | type=telegram、enabled；telegram 内为 bot_token、chat_id、api_base、timeout（30 秒） |
| `proxy` | `null` | enabled（true）、scheme（socks5）、host、port、可选 username/password；支持 socks5、socks4a（配置值 socks4）、HTTP CONNECT |
| `ai` | 禁用 | enabled、provider、api_key、model、base_url、default_mode（hybrid）、language（auto） |

`use_ssl: false` 保留普通 IMAP 连接行为，只应在你信任的网络中使用。代理配置同时用于 IMAP、Telegram 和 AI，目标域名由代理解析，不修改全局 socket 或代理环境变量。没有显式代理时直接连接。

## DeepSeek AI

只支持 DeepSeek，使用 OpenAI 兼容的 Chat Completions 协议。AI 设置无需选择平台，只需填写密钥、模型、地址、模式和语言。

- `provider`：`deepseek`。旧 `openai` / `custom` 值仅作为同一协议的配置别名，不再选择其他服务商。
- `model`：默认 `deepseek-chat`；可填写 DeepSeek 提供的其他模型名。
- `api_key`：DeepSeek 密钥；为空时读取 `DEEPSEEK_API_KEY`。
- `base_url`：默认 `https://api.deepseek.com`；为空时先读取 `DEEPSEEK_API_BASE`。可填写根地址、`/v1` 地址或完整 `/chat/completions` 接口，支持自有代理。
- 请求使用 Bearer 鉴权、`messages` 和 JSON 输出格式。接口明确拒绝 `response_format` 时重试一次，不带该参数。其他错误使用原有 AI 失败回退。

不支持其他服务商专用协议、云平台认证、设备登录或本地模型加载。

## 日志与部署

日志位于配置目录下的 `logs/mailbot.log`，单文件 5 MiB、三个备份。Headless 使用 `--headless -c PATH`。部署见 [Go 服务](go-service.md)。
