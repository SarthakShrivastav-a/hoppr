package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/SarthakShrivastav-a/hoppr/internal/sshconfig"
)

// Run starts the TUI against the config at path.
func Run(configPath string, readOnly bool) error {
	// A mangled console codepage makes Bubble Tea's input decoder produce
	// utf8.RuneError and bail out. Putting the console into UTF-8 mode first is
	// the difference between hoppr working and hoppr refusing to start on a
	// machine whose locale is not English.
	restore, err := prepareConsole()
	if err != nil {
		return err
	}
	defer restore()

	tree, err := sshconfig.LoadTree(configPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", configPath, err)
	}

	p := tea.NewProgram(
		New(configPath, tree, readOnly),
		tea.WithAltScreen(),

		// Mouse support is deliberately not enabled. Turning it on takes text
		// selection away from the terminal, so copying a hostname out of the
		// list stops working — and copying a hostname out of the list is one of
		// the things people open this for.
	)

	// Bubble Tea restores the terminal on a clean exit, but a panic bypasses
	// that and leaves the user in a raw-mode shell with no echo, which reads as
	// "this tool broke my terminal". Catch it, restore, then re-panic so the
	// stack trace is still there to report.
	defer func() {
		if r := recover(); r != nil {
			restore()
			panic(r)
		}
	}()

	_, err = p.Run()
	return err
}
