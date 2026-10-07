package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Palette, loosely based on Spotify's brand colors.
var (
	colGreen  = lipgloss.Color("#1DB954")
	colText   = lipgloss.Color("#E6E6E6")
	colMuted  = lipgloss.Color("#7A7A7A")
	colBorder = lipgloss.Color("#3A3A3A")
	colSelBg  = lipgloss.Color("#262626")

	// Visualizer gradient: low -> mid -> high amplitude.
	colVizLow  = "#1DB954"
	colVizMid  = "#E8C547"
	colVizHigh = "#E8475F"
)

var (
	styleText   = lipgloss.NewStyle().Foreground(colText)
	styleMuted  = lipgloss.NewStyle().Foreground(colMuted)
	styleAccent = lipgloss.NewStyle().Foreground(colGreen)
	styleBold   = lipgloss.NewStyle().Foreground(colText).Bold(true)
	styleErr    = lipgloss.NewStyle().Foreground(lipgloss.Color(colVizHigh))
)

// fit truncates s to w cells and pads it with spaces so it is exactly w wide.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = ansi.Truncate(s, w, "…")
	if pad := w - ansi.StringWidth(s); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}

// block normalizes lines to exactly w x h (clipping or padding as needed).
func block(lines []string, w, h int) []string {
	out := make([]string, h)
	for i := range out {
		if i < len(lines) {
			out[i] = fit(lines[i], w)
		} else {
			out[i] = strings.Repeat(" ", w)
		}
	}
	return out
}

// spread places left and right on one line of width w, right-aligned.
func spread(left, right string, w int) string {
	gap := w - ansi.StringWidth(left) - ansi.StringWidth(right)
	if gap < 1 {
		return ansi.Truncate(left, w-ansi.StringWidth(right)-1, "…") + " " + right
	}
	return left + strings.Repeat(" ", gap) + right
}

// panel draws a rounded box of exactly w x h with a title in the top border.
// The focused panel gets an accent-colored border.
func panel(title string, body []string, w, h int, focused bool) string {
	if w < 4 || h < 2 {
		return ""
	}
	bc := colBorder
	if focused {
		bc = colGreen
	}
	border := lipgloss.NewStyle().Foreground(bc)
	titleSt := lipgloss.NewStyle().Foreground(colGreen).Bold(true)

	title = ansi.Truncate(title, w-6, "…")
	fill := w - 2 - 1 - (ansi.StringWidth(title) + 2)
	top := border.Render("╭─") + titleSt.Render(" "+title+" ") +
		border.Render(strings.Repeat("─", max(fill, 0))+"╮")

	var sb strings.Builder
	sb.WriteString(top + "\n")
	side := border.Render("│")
	for _, l := range block(body, w-2, h-2) {
		sb.WriteString(side + l + borderMark + side + "\n")
	}
	sb.WriteString(border.Render("╰" + strings.Repeat("─", w-2) + "╯"))
	return sb.String()
}

func clamp(v, lo, hi int) int { return max(lo, min(v, hi)) }

// wrap word-wraps a message to w cells in the given style, one string per line.
func wrap(s string, st lipgloss.Style, w int) []string {
	return strings.Split(st.Width(max(w, 1)).Render(s), "\n")
}

// follow returns the scroll offset that keeps cursor inside a window of
// `visible` rows.
func follow(cursor, offset, visible int) int {
	switch {
	case visible <= 0, cursor < offset:
		return max(cursor, 0)
	case cursor >= offset+visible:
		return cursor - visible + 1
	}
	return offset
}

// --- column anchoring -------------------------------------------------------
//
// Terminals and our width tables disagree about some text (Indic scripts with
// spacing vowel signs, some emoji). If we only pad with spaces, one such name
// shifts everything to its right on that row, including the album cover.
// Anchors fix that: each one is a "move cursor to absolute column" escape, so
// a miscounted name can only mis-place its own text. They are zero-width to
// ansi.StringWidth, so layout maths is unaffected.

// at returns the escape that moves the cursor to 0-based screen column x.
func at(x int) string { return fmt.Sprintf("\x1b[%dG", x+1) }

// borderMark is left by panel() just before each row's right border. It is a
// control character (zero width), and View swaps it for the anchor escape once
// the panel's screen column is known.
const borderMark = "\x00"

// anchorBorders replaces the border marks in a rendered panel with anchors to
// column x (the panel's right border).
func anchorBorders(panel string, x int) string {
	return strings.ReplaceAll(panel, borderMark, at(x))
}

// joinBody puts the sidebar and main column side by side, anchoring the main
// column's start and both right borders to fixed screen columns.
func joinBody(side, main string, sideW, mainW int) string {
	side = anchorBorders(side, sideW-1)
	main = anchorBorders(main, sideW+mainW-1)
	sl, ml := strings.Split(side, "\n"), strings.Split(main, "\n")
	if len(sl) != len(ml) {
		return lipgloss.JoinHorizontal(lipgloss.Top, side, main)
	}
	out := make([]string, len(sl))
	for i := range sl {
		out[i] = sl[i] + at(sideW) + ml[i]
	}
	return strings.Join(out, "\n")
}

// spreadAt is spread with the right-hand text anchored to screen column x.
func spreadAt(left, right string, w, x int) string {
	if gap := w - ansi.StringWidth(left) - ansi.StringWidth(right); gap < 1 {
		left = ansi.Truncate(left, w-ansi.StringWidth(right)-1, "…")
	}
	gap := max(w-ansi.StringWidth(left)-ansi.StringWidth(right), 1)
	return left + strings.Repeat(" ", gap) + at(x) + right
}
