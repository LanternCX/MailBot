# Go 服务与迁移

## 构建和发布

开发需要 Go 1.26+；运行机器不需要 Python、Go、C 运行时或外部数据库。

```sh
go test -race ./...
go vet ./...
CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o dist/mailbot ./cmd/mailbot
./dist/mailbot                  # 交互式配置
./dist/mailbot --headless -c config.json
```

配置选项和交互说明见 [配置文档](configuration.md)。本地生成发布包：

```sh
go run ./cmd/package --variant linux-x64 --tag dev
```

支持 linux-x64、linux-arm64、macos-arm64、windows-x64。`--name` 可指定程序名，`--entry` 指定 Go 主包，`--clean` 清理该平台的构建目录。输出为 `dist/<平台>/` 和 `dist/mailbot-<版本>-<平台>.zip`。压缩包只包含空的初始配置，不打包你实际使用的账号密钥。

## 投递与恢复

账号、文件夹、UIDVALIDITY 和 UID 共同标识邮件。首次扫描以最高 UID 为基线，不转发已有邮件。抓取进度与每个目标的待投递任务在同一 SQLite 事务中保存，成功后才按配置标记已读。

此后的重启继续使用已有进度，发送失败持续退避重试，Telegram 限流遵循 retry_after；长消息分段保存发送进度。进程用操作系统文件锁独占数据库，异常退出后自动释放。每个账号和投递目标独立运行，网络操作和代理握手均有上限。

Telegram 已收到消息但响应丢失，或本地进度保存失败时仍可能重复发送；不承诺恰好一次投递。按钮所需的已完成邮件保留 30 天；待投递邮件不自动清理，长期断网时应留意磁盘容量。备份 SQLite 应停止服务或使用 SQLite 在线备份工具，不要只复制活动数据库的主文件。

## 从旧服务切换

可以读取已有 JSON 配置；启用 AI 时需改用 DeepSeek，其他平台配置不再支持。先备份配置与规则，在独立目录运行 `--check`，核对账号和 DeepSeek 模型、密钥及地址。Go 版取代 Python 运行与打包入口，旧代码仍可从 Git 历史取得。

切换前停止旧进程，同一 bot 不应由两个实例同时 getUpdates。首次迁移没有旧版持久化进度，**不会自动补发旧版已漏掉的历史邮件**。上线前手动核对邮件，并尽量安排短暂切换窗口。之后保留 SQLite 即可恢复停机期间的进度。

默认只转发新 UID 中仍未读的邮件；如需补抓已在其他客户端读过的新邮件，设置 `unread_only: false`。首次基线和 UIDVALIDITY 重置仍会跳过当前已有邮件，重置后需手动核对。旧消息的 UID 按钮缓存没有持久化，不能从 Python 进程自动迁移；Go 新消息的按钮跨重启有效。

网络封锁、DNS 错误或失效的代理不能通过重写消除。正式切换前在部署机器验证 Telegram 连接、邮箱登录、三种模式、规则修改、重启后补抓和断网后补发。仓库自动测试使用本地 IMAP / HTTP / 代理，不读取生产密钥。

## systemd

以下命令供管理员在确认切换后执行：

```sh
sudo useradd --system --home-dir /var/lib/mailbot --shell /usr/sbin/nologin mailbot
sudo install -m 755 dist/mailbot /usr/local/bin/mailbot
sudo install -d -o mailbot -g mailbot -m 700 /etc/mailbot /var/lib/mailbot
sudo install -o mailbot -g mailbot -m 600 config.json /etc/mailbot/config.json
# 设置 state_path 为 /var/lib/mailbot/mailbot.sqlite3
# 如有规则，将 rules.md 放入 /etc/mailbot/，所有者设为 mailbot
sudo install -m 644 deploy/mailbot.service /etc/systemd/system/mailbot.service
sudo systemctl daemon-reload
sudo systemctl enable --now mailbot
sudo journalctl -u mailbot -f
```

配置目录必须允许运行用户写入，供 Telegram 设置、规则和轮转日志使用。服务单元仅放开配置与状态目录，使用专门用户并在异常退出后重启。SIGINT / SIGTERM 会取消网络请求并等待任务退出。

`/status` 与每分钟健康日志显示最近成功轮询、最近成功投递和积压数量。进程存在不代表网络可用，应监控这些成功时间与持续连接错误。
