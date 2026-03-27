package tui

import tea "github.com/charmbracelet/bubbletea"

// Key bindings for the TUI.
type keyMap struct {
	Quit       tea.KeyType
	Refresh    rune
	Logs       rune
	Attach     rune
	Pause      rune
	Resume     rune
	Kill       rune
	Back       rune
	Search     rune
	Filter     rune
	TabNext    rune
}

var keys = keyMap{
	Quit:    tea.KeyCtrlC,
	Refresh: 'r',
	Logs:    'l',
	Attach:  'a',
	Pause:   'p',
	Resume:  'R',
	Kill:    'K',
	Back:    'q',
	Search:  '/',
	Filter:  'f',
	TabNext: '\t',
}
