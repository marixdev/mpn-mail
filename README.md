# mpn-mail

Web mail nhẹ cho [Mini Panel](https://github.com/marixdev/mini-panel) (`mpn`). Một tệp Go chạy tĩnh, không cần cơ sở dữ liệu ngoài, đọc và gửi thư qua IMAP/SMTP của chính máy chủ.

Đây là bản tùy biến từ [lilmail](https://github.com/vul-os/lilmail) v1.14.0 của vul-os, giữ nguyên giấy phép gốc (xem bên dưới). Mã nguồn lilmail vẫn nằm trong lịch sử git của repo này.

## Khác gì so với lilmail

- Giao diện tiếng Việt, tiếng Anh và tiếng Trung giản thể. Mặc định là tiếng Việt, người dùng đổi bằng cookie `lang` hoặc chỉnh `[server] language`. Chuỗi chữ nằm ở `i18n/vi.go`, `i18n/en.go` và `i18n/zh.go`, template gọi qua `{{T .Lang "khóa"}}`.
- Logo và favicon của Mini Panel, màu xanh lá (hue 165) cùng bảng màu với panel, sáng và tối.
- Đăng nhập SMTP dùng cả địa chỉ email làm tên đăng nhập, giống IMAP. Bản gốc chỉ gửi phần trước `@`, nên hộp thư ảo kiểu `ten@tenmien` bị từ chối. Máy chủ chỉ nhận tên ngắn thì gọi `SetAuthUsername`.
- Kết nối IMAP và SMTP có thời gian chờ 20 giây, không còn treo khi máy chủ không trả lời. Không đặt thời gian chờ đọc lệnh vì sẽ cắt kết nối IMAP IDLE.
- Tự thoát khi rảnh và nhận socket từ systemd, để web mail không chiếm RAM lúc không ai dùng.

## Cấu hình

Chép `config.toml.example` thành `config.toml` ở thư mục chạy. Tệp chỉ được đọc từ thư mục hiện tại, và chương trình chỉ nhận cờ `-version`. Các khóa mpn-mail thêm vào:

| Khóa | Mặc định | Ý nghĩa |
|---|---|---|
| `[server] language` | `"vi"` | Ngôn ngữ mặc định, `"vi"`, `"en"` hoặc `"zh"` (tiếng Trung giản thể). |
| `[server] idle_exit_seconds` | `0` | Số giây không có yêu cầu hay kết nối SSE nào thì tiến trình tự thoát. `0` là tắt. |

Với máy chủ thư của Mini Panel (Dovecot xác thực bằng địa chỉ đầy đủ), `[auth] allow_full_email_username = true`.

## Chạy tiết kiệm RAM với systemd

Đặt `idle_exit_seconds` rồi cho systemd giữ cổng. Khi có kết nối đầu tiên systemd khởi động tiến trình, khi rảnh tiến trình thoát và nhả RAM.

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

Tiến trình nhận socket qua biến `LISTEN_FDS` (fd 3). Không có biến này nó tự mở cổng `[server] port`. Yêu cầu `/health` không tính là hoạt động, nên kiểm tra sống chết không giữ tiến trình dậy.

## Build

Cần Go 1.25 trở lên.

```
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-s -w" -o mpn-mail .
go test ./...
```

Một số bài kiểm thử kiểm quyền tệp kiểu Unix (`0600`, `0700`), nên chúng báo lỗi khi chạy trên Windows và chỉ có ý nghĩa trên Linux.

## Cập nhật từ lilmail

Remote `upstream` trỏ về `https://github.com/vul-os/lilmail`. Mỗi bản phát hành của lilmail đều kèm tệp mã nguồn `lilmail_<phiên bản>_source.zip` và `SHA256SUMS` để đối chiếu.

## Giấy phép

Giữ nguyên giấy phép của lilmail: [MIT](LICENSE-MIT) hoặc [Apache-2.0](LICENSE-APACHE), tùy bạn chọn. Phần mềm bên thứ ba và giấy phép của chúng nằm trong `THIRD-PARTY-NOTICES.txt` và được phục vụ tại `/licenses.txt` khi chạy.
