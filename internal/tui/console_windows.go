//go:build windows

package tui

import "golang.org/x/sys/windows"

// prepareConsole puts the Windows console into a state Bubble Tea can work in,
// and returns a function that puts it back.
//
// Two things go wrong otherwise, and both of them look like hoppr is broken:
//
//   - A non-UTF-8 codepage (CP850 in much of Europe, CP1251 for Cyrillic, and
//     so on — anything but a US-English default) makes the input decoder read
//     a keystroke as utf8.RuneError and give up. The program exits before it
//     has drawn anything.
//
//   - conhost, unlike Windows Terminal, has virtual-terminal processing off by
//     default, so every ANSI escape sequence is printed as literal text. The
//     UI renders as a wall of "[38;5;111m".
//
// Both are one API call each, and both are restored on the way out so we do not
// leave the user's shell in a codepage it did not ask for.
func prepareConsole() (func(), error) {
	var restores []func()

	restore := func() {
		// Unwind in reverse, so the console ends up exactly as we found it.
		for i := len(restores) - 1; i >= 0; i-- {
			restores[i]()
		}
	}

	// Codepage 65001 is UTF-8.
	const utf8CP = 65001

	if prev, err := windows.GetConsoleOutputCP(); err == nil && prev != utf8CP {
		if err := windows.SetConsoleOutputCP(utf8CP); err == nil {
			restores = append(restores, func() { _ = windows.SetConsoleOutputCP(prev) })
		}
	}
	if prev, err := windows.GetConsoleCP(); err == nil && prev != utf8CP {
		if err := windows.SetConsoleCP(utf8CP); err == nil {
			restores = append(restores, func() { _ = windows.SetConsoleCP(prev) })
		}
	}

	// Enable ANSI escape sequence handling on the output handle.
	if h, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE); err == nil {
		var mode uint32
		if err := windows.GetConsoleMode(h, &mode); err == nil {
			const want = windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING
			if mode&want == 0 {
				if err := windows.SetConsoleMode(h, mode|want); err == nil {
					prev := mode
					restores = append(restores, func() { _ = windows.SetConsoleMode(h, prev) })
				}
			}
		}
	}

	return restore, nil
}
