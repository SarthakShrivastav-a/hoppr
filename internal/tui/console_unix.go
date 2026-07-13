//go:build !windows

package tui

// prepareConsole is a no-op outside Windows. A Unix terminal is already UTF-8
// and already speaks ANSI; there is nothing to negotiate.
func prepareConsole() (func(), error) {
	return func() {}, nil
}
