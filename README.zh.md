# mpn-mail

[Tiếng Việt](README.md) · [English](README.en.md) · **简体中文**

[Mini Panel](https://github.com/marixdev/mini-panel)（`mpn`）的轻量网页邮件。一个静态编译的 Go 程序，不需要外部数据库，通过服务器自身的 IMAP/SMTP 收发邮件。

本项目基于 vul-os 的 [lilmail](https://github.com/vul-os/lilmail) v1.14.0 定制，保留原有许可证（见下文）。lilmail 的源代码仍保留在本仓库的 git 历史中。

## 与 lilmail 的不同

- 界面支持越南语、英语和简体中文。默认越南语，用户可通过 `lang` cookie 切换，默认语言由 `[server] language` 设置。文本位于 `i18n/vi.go`、`i18n/en.go` 和 `i18n/zh.go`，模板通过 `{{T .Lang "键"}}` 调用。
- 使用 Mini Panel 的徽标和网站图标，以及与面板相同的绿色配色（色相 165），支持浅色和深色。
- SMTP 登录与 IMAP 一样使用完整邮箱地址作为用户名。原版只发送 `@` 之前的部分，因此 `name@domain` 这类虚拟邮箱会被拒绝。若服务器只接受短用户名，请调用 `SetAuthUsername`。
- IMAP 和 SMTP 连接在 20 秒后超时，服务器无响应时不再卡住。命令读取不设超时，否则会中断 IMAP IDLE。
- 空闲时自动退出，并可接收 systemd 传入的套接字，无人使用时不占用内存。

## 配置

将 `config.toml.example` 复制为工作目录下的 `config.toml`。程序只从当前目录读取该文件，且只接受 `-version` 参数。mpn-mail 新增的键：

| 键 | 默认值 | 含义 |
|---|---|---|
| `[server] language` | `"vi"` | 默认语言：`"vi"`、`"en"` 或 `"zh"`（简体中文）。 |
| `[server] idle_exit_seconds` | `0` | 在这么多秒内没有任何请求或 SSE 连接时进程退出。`0` 表示关闭。 |

配合 Mini Panel 的邮件服务器使用时（Dovecot 使用完整地址认证），请设置 `[auth] allow_full_email_username = true`。

## 用 systemd 节省内存

设置 `idle_exit_seconds`，并让 systemd 持有端口。第一个连接到来时 systemd 启动进程，空闲时进程退出并释放内存。

```ini
# mpn-mail.socket
[Socket]
ListenStream=127.0.0.1:3000

[Install]
WantedBy=sockets.target
```

```ini
# mpn-mail.service
[Service]
ExecStart=/usr/local/bin/mpn-mail
WorkingDirectory=/var/lib/mpn-mail
User=mpn-mail
```

进程通过 `LISTEN_FDS`（fd 3）接收套接字；没有该变量时自行监听 `[server] port`。`/health` 请求不计为活动，因此健康检查不会让进程保持运行。

## 发布

推送 `v*` 标签会运行 [release.yml](.github/workflows/release.yml)，发布 `mpn-mail_linux_amd64`（Mini Panel 安装的二进制，使用 `-trimpath` 构建）、`mpn-mail_<版本>_<系统>_<架构>.zip` 压缩包、源代码压缩包和 `SHA256SUMS`。可用 `scripts/verify.sh --tag vX.Y.Z <文件>` 校验下载的文件。

## 构建

需要 Go 1.25 或更高版本。

```
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-s -w" -o mpn-mail .
go test ./...
```

部分测试检查 Unix 文件权限（`0600`、`0700`），因此在 Windows 上会失败，只在 Linux 上有意义。

## 从 lilmail 更新

远程仓库 `upstream` 指向 `https://github.com/vul-os/lilmail`。lilmail 的每个发布版本都附带源代码压缩包 `lilmail_<版本>_source.zip` 和 `SHA256SUMS` 以供核对。

## 许可证

沿用 lilmail 的许可证：[MIT](LICENSE-MIT) 或 [Apache-2.0](LICENSE-APACHE)，任选其一。第三方软件及其许可证列于 `THIRD-PARTY-NOTICES.txt`，程序运行时可在 `/licenses.txt` 查看。
