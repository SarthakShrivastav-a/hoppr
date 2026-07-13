package tui

import "github.com/charmbracelet/bubbles/key"

// keyMap holds every binding.
//
// Both vim motions and arrow keys are bound everywhere. This is not a
// preference: an SSH manager's audience is disproportionately people who live
// in vim, and "add j/k" is the single most common issue filed against tools in
// this category — sometimes the only issue filed.
//
// Nothing load-bearing is bound to an exotic Ctrl combination, because Windows
// consoles flatten several of them (Ctrl+Space arrives indistinguishable from
// Space) and a binding that silently does nothing is worse than no binding.
type keyMap struct {
	Up     key.Binding
	Down   key.Binding
	Top    key.Binding
	Bottom key.Binding
	PageUp key.Binding
	PageDn key.Binding

	Connect key.Binding
	Search  key.Binding
	Clear   key.Binding

	Add    key.Binding
	Edit   key.Binding
	Delete key.Binding
	Import key.Binding
	Tag    key.Binding

	Keys   key.Binding
	Doctor key.Binding
	Fix    key.Binding
	Adopt  key.Binding
	Probe  key.Binding

	Detail key.Binding
	Help   key.Binding
	Back   key.Binding
	Quit   key.Binding

	Confirm key.Binding
	Next    key.Binding
	Prev    key.Binding
}

var defaultKeys = keyMap{
	Up:     key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:   key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	Top:    key.NewBinding(key.WithKeys("home", "g"), key.WithHelp("g", "top")),
	Bottom: key.NewBinding(key.WithKeys("end", "G"), key.WithHelp("G", "bottom")),
	PageUp: key.NewBinding(key.WithKeys("pgup", "ctrl+u"), key.WithHelp("pgup", "page up")),
	PageDn: key.NewBinding(key.WithKeys("pgdown", "ctrl+d"), key.WithHelp("pgdn", "page down")),

	Connect: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "connect")),
	Search:  key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
	Clear:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear")),

	Add:    key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add")),
	Edit:   key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit")),
	Delete: key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete")),
	Import: key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "import command")),
	Tag:    key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "filter by tag")),

	Keys:   key.NewBinding(key.WithKeys("K"), key.WithHelp("K", "keys")),
	Doctor: key.NewBinding(key.WithKeys("D"), key.WithHelp("D", "doctor")),
	Fix:    key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "fix")),
	Adopt:  key.NewBinding(key.WithKeys("A"), key.WithHelp("A", "adopt key")),
	Probe:  key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh status")),

	Detail: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "detail")),
	Help:   key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	Back:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	Quit:   key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),

	Confirm: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "confirm")),
	Next:    key.NewBinding(key.WithKeys("tab", "down"), key.WithHelp("tab", "next field")),
	Prev:    key.NewBinding(key.WithKeys("shift+tab", "up"), key.WithHelp("shift+tab", "prev field")),
}
