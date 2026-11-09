package i18n

// FolderLabel turns a server IMAP folder name into a localized label for the
// well-known folders, and returns the name unchanged for everything else (a
// user's own folders keep their given names). Registered as the "folderLabel"
// template function: {{folderLabel $.Lang .Name}}.
func FolderLabel(lang, name string) string {
	var key string
	switch name {
	case "INBOX", "Inbox":
		key = "folder.inbox"
	case "Sent", "Sent Items", "Sent Mail", "Sent Messages":
		key = "folder.sent"
	case "Drafts", "Draft":
		key = "folder.drafts"
	case "Trash", "Deleted Items", "Deleted Messages", "Bin":
		key = "folder.trash"
	case "Junk", "Junk Mail", "Junk E-mail", "Spam", "Bulk Mail":
		key = "folder.spam"
	case "Archive", "Archives", "All Mail":
		key = "folder.archive"
	default:
		return name
	}
	return T(lang, key)
}
