package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// nowPlayingView renders the bottom panel: album cover on the left, track
// info + progress + controls + audio visualizer on the right.
func (m Model) nowPlayingView(w, h int) string {
	inner, innerH := w-2, h-2

	// Cover is square in pixels: a cell is ~1:2, so cols = 2*rows. Shrink it
	// if there isn't room left for the info column.
	coverRows := innerH
	coverCols := coverRows * 2
	if maxCols := inner / 2; coverCols > maxCols {
		coverCols = maxCols &^ 1
		coverRows = coverCols / 2
	}
	cover := m.cover.Render(coverCols, coverRows)
	if cover == nil { // no art yet: dim placeholder block
		ph := styleMuted.Render(strings.Repeat("░", coverCols))
		for i := 0; i < coverRows; i++ {
			cover = append(cover, ph)
		}
	}

	const gap = 2
	rightW := inner - coverCols - gap - 1 // 1 col of right padding

	var right []string
	if s := m.state; s != nil {
		status := "❚❚ Paused"
		if s.Playing {
			status = "▶ Playing"
		}
		shuffle, repeat := "off", s.Repeat
		if s.Shuffle {
			shuffle = "on"
		}
		if repeat == "" {
			repeat = "off"
		}
		right = []string{
			styleBold.Render(s.Track.Title),
			styleText.Render(s.Track.Artist),
			styleMuted.Render(s.Track.Album),
			progressBar(m.elapsed, s.Track.Duration, rightW),
			spread(styleAccent.Render(status),
				styleMuted.Render(fmt.Sprintf("⇄ %s  ↻ %s  vol %d%%", shuffle, repeat, s.Volume)), rightW),
			styleMuted.Render(s.DeviceName),
		}
	} else {
		right = []string{
			styleBold.Render("Nothing playing"),
			styleMuted.Render("Open a playlist, move to a track and press enter."),
			styleMuted.Render("Spotify must be open on one of your devices."),
			"", "", "",
		}
	}
	right = append(right, m.viz.View(rightW, innerH-len(right))...)

	lines := make([]string, innerH)
	for i := range lines {
		left := strings.Repeat(" ", coverCols)
		if i < len(cover) {
			left = cover[i]
		}
		r := ""
		if i < len(right) {
			r = right[i]
		}
		// 1 border + 1 pad + cover + gap puts the text column at a fixed
		// screen column, whatever the lines above it contain.
		textX := m.dims().sideW + 2 + coverCols + gap
		lines[i] = " " + left + strings.Repeat(" ", gap) + at(textX) + ansi.Truncate(r, rightW, "")
	}
	return panel("Now Playing", lines, w, h, false)
}

// progressBar renders "1:23 ━━━━━━────── 3:45" in exactly w cells.
//
// Position is continuous, not per cell: the cell the playhead is in fades from
// the track colour to the fill colour as it is crossed, so the bar advances
// smoothly instead of jumping a whole cell every few seconds.
func progressBar(elapsed, total time.Duration, w int) string {
	elapsed = max(min(elapsed, total), 0)
	l, r := fmtDur(elapsed), fmtDur(total)
	barW := w - len(l) - len(r) - 2
	if barW < 4 || total <= 0 {
		return styleMuted.Render(l + " / " + r)
	}

	x := float64(barW) * float64(elapsed) / float64(total) // 0..barW
	filled := min(int(x), barW)
	frac := x - float64(filled)

	var bar strings.Builder
	if filled > 0 {
		bar.WriteString(styleAccent.Render(strings.Repeat("━", filled)))
	}
	rest := barW - filled
	if rest > 0 {
		edge := lipgloss.NewStyle().Foreground(lipgloss.Color(mix(string(colMuted), string(colGreen), frac)))
		bar.WriteString(edge.Render("━"))
		bar.WriteString(styleMuted.Render(strings.Repeat("─", rest-1)))
	}
	return styleMuted.Render(l) + " " + bar.String() + " " + styleMuted.Render(r)
}
