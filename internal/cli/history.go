package cli

import (
	"os"
	"path/filepath"
)

// historyFilePath returns a writable location for readline's command
// history file. It prefers $HOME, falling back to a temp directory so the
// CLI still works (just without persistent history) in unusual
// environments where HOME isn't set.
func historyFilePath() string {
	dir, err := os.UserHomeDir()
	if err != nil || dir == "" {
		dir = os.TempDir()
	}
	return filepath.Join(dir, ".cli_login_history")
}
