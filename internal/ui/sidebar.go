package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/zenbiscuitslol/spotiCLI/internal/spotify"
)

// sidebarView renders the tab strip and the scrolling list on the left.
func (m Model) sidebarView(w, h int) string {
	inner, rows := w-2, h-3 // borders + tab strip
	focused := m.focus == paneSidebar
	sl := m.side[m.tab]

	lines := []string{m.tabStrip()}
	title := m.tab.String()
	switch {
	case sl.err != "":
		lines = append(lines, wrap(sl.err, styleErr, inner)...)
	case !sl.loaded:
		lines = append(lines, styleMuted.Render(" Loading "+strings.ToLower(title)+"…"))
	case len(sl.items) == 0:
		lines = append(lines, styleMuted.Render(" Nothing here yet"))
	default:
		title = fmt.Sprintf("%s · %d", title, len(sl.items))
		end := min(sl.offset+rows, len(sl.items))
		for i := sl.offset; i < end; i++ {
			lines = append(lines, playlistRow(sl.items[i], i == sl.cursor, focused, inner))
		}
	}
	return panel(title, lines, w, h, focused)
}

// tabStrip shows the sidebar tabs with the active one highlighted.
func (m Model) tabStrip() string {
	parts := make([]string, numTabs)
	for t := sideTab(0); t < numTabs; t++ {
		label := fmt.Sprintf("%d %s", t+1, t)
		if t == m.tab {
			parts[t] = styleAccent.Bold(true).Render(label)
		} else {
			parts[t] = styleMuted.Render(label)
		}
	}
	return " " + strings.Join(parts, styleMuted.Render("  "))
}

func playlistRow(p spotify.Playlist, cursor, focused bool, w int) string {
	count := fmt.Sprintf("%d ", p.Tracks)
	if p.Album {
		count = p.Year + " "
	}
	// Locked playlists (tracks not readable) are dimmed and marked.
	label, nameStyle := "  "+p.Name, styleText
	if p.Locked {
		label, nameStyle = "⊘ "+p.Name, styleMuted
	}
	// The count is anchored to its column so a name the terminal measures
	// differently than we do can't push it (or the border) out of line.
	countAt := at(1 + w - ansi.StringWidth(count))
	if !cursor {
		return spreadAt(nameStyle.Render(label), styleMuted.Render(count), w, 1+w-ansi.StringWidth(count))
	}

	// Cursor row: highlight bar spanning the full width, including the gap.
	// Dimmer when the sidebar doesn't have focus.
	fg := colHi
	if !focused {
		fg = colText
	}
	if p.Locked {
		fg = colMuted
	}
	bg := lipgloss.NewStyle().Background(colSelBg)
	cnt := bg.Foreground(colMuted).Render(count)
	prefix := "▌ "
	if p.Locked {
		prefix = "▌⊘ "
	}
	name := ansi.Truncate(prefix+p.Name, w-ansi.StringWidth(count)-1, "…")
	name = bg.Foreground(fg).Bold(true).Render(name)
	gap := max(w-ansi.StringWidth(name)-ansi.StringWidth(cnt), 0)
	return name + bg.Render(strings.Repeat(" ", gap)) + countAt + cnt
}
