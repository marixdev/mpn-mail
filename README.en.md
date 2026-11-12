# mpn-mail

[Tiếng Việt](README.md) · **English** · [简体中文](README.zh.md)

Lightweight web mail for [Mini Panel](https://github.com/marixdev/mini-panel) (`mpn`). A single static Go program with no external database; it reads and sends mail over the server's own IMAP/SMTP.

This is a customised build of [lilmail](https://github.com/vul-os/lilmail) v1.14.0 by vul-os and keeps the original licence (see below). The lilmail source is still in this repository's git history.

## What differs from lilmail

- Vietnamese, English and Simplified Chinese interface. Vietnamese is the default; users switch with the `lang` cookie, or the default is set with `[server] language`. Strings live in `i18n/vi.go`, `i18n/en.go` and `i18n/zh.go`; templates call `{{T .Lang "key"}}`.
- Mini Panel logo and favicon, and the panel's green palette (hue 165), light and dark.
- SMTP login uses the full email address as the user name, as IMAP does. The original sent only the part before `@`, so virtual mailboxes such as `name@domain` were refused. For a server that only takes the short name, call `SetAuthUsername`.
- IMAP and SMTP connections time out after 20 seconds instead of hanging when the server does not answer. There is no read timeout on commands, because it would cut IMAP IDLE.
- Exits when idle and accepts a socket from systemd, so the web mail uses no RAM while nobody is using it.

## Configuration

Copy `config.toml.example` to `config.toml` in the working directory. The file is read from the current directory only, and the program takes only the `-version` flag. Keys mpn-mail adds:

| Key | Default | Meaning |
|---|---|---|
| `[server] language` | `"vi"` | Default language: `"vi"`, `"en"` or `"zh"` (Simplified Chinese). |
| `[server] idle_exit_seconds` | `0` | Seconds without any request or SSE connection after which the process exits. `0` turns it off. |

With the Mini Panel mail server (Dovecot authenticates with the full address), set `[auth] allow_full_email_username = true`.

## Saving RAM with systemd

Set `idle_exit_seconds` and let systemd hold the port. systemd starts the process on the first connection; when idle the process exits and frees its RAM.

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

The process receives the socket through `LISTEN_FDS` (fd 3). Without it, it opens `[server] port` itself. `/health` requests do not count as activity, so health checks do not keep the process awake.

## Releases

Pushing a `v*` tag runs [release.yml](.github/workflows/release.yml). It publishes `mpn-mail_linux_amd64` (the binary Mini Panel installs, built with `-trimpath`), `mpn-mail_<version>_<os>_<arch>.zip` bundles, a source archive and `SHA256SUMS`. Check a download with `scripts/verify.sh --tag vX.Y.Z <file>`.

## Build

Requires Go 1.25 or newer.

```
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-s -w" -o mpn-mail .
go test ./...
```

Some tests check Unix file modes (`0600`, `0700`), so they fail on Windows and are only meaningful on Linux.

## Updating from lilmail

The `upstream` remote points at `https://github.com/vul-os/lilmail`. Every lilmail release ships a source archive `lilmail_<version>_source.zip` and `SHA256SUMS` to compare against.

## Licence

lilmail's licence is kept: [MIT](LICENSE-MIT) or [Apache-2.0](LICENSE-APACHE), at your option. Third-party software and its licences are listed in `THIRD-PARTY-NOTICES.txt` and served at `/licenses.txt` while running.
