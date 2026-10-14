package ui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/zenbiscuitslol/spoTERM/internal/spotify"
)

func fmtDur(d time.Duration) string {
	s := int(d.Seconds())
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// mainView is the queue when it's open, otherwise the track table.
func (m Model) mainView(w, h int) string {
	if !m.queueOpen {
		return m.tracksView(w, h)
	}
	inner := w - 2
	var lines []string
	row := func(i int, t spotify.Track, st lipgloss.Style) {
		lines = append(lines, st.Render(fit(fmt.Sprintf(" %2d  %s — %s", i+1, t.Title, t.Artist), inner)))
	}
	lines = append(lines, styleMuted.Render(fit(" Your queue (a adds, x removes)", inner)))
	if len(m.lq) == 0 {
		lines = append(lines, styleMuted.Render(" empty — press a on a track"))
	}
	for i, t := range m.lq {
		st := styleText
		if i == m.lqCursor {
			st = st.Background(colSelBg).Foreground(colHi).Bold(true)
		}
		row(i, t, st)
	}
	lines = append(lines, "", styleMuted.Render(fit(" Next in Spotify", inner)))
	if m.queueErr != "" {
		lines = append(lines, wrap(m.queueErr, styleErr, inner)...)
	}
	for i, t := range m.queue {
		row(i, t, styleMuted)
	}
	return panel(fmt.Sprintf("Queue · %d yours (esc closes)", len(m.lq)), block(lines, inner, h-2), w, h, true)
}

// tracksView renders the track table for the opened playlist.
func (m Model) tracksView(w, h int) string {
	inner := w - 2
	focused := m.focus == paneTracks

	if !m.hasOpen {
		return panel("Tracks", []string{styleMuted.Render(" Select a playlist or album and press enter")}, w, h, focused)
	}
	pl := m.opened
	name := pl.Name
	if pl.Album && pl.Artist != "" {
		name += " — " + pl.Artist
	}
	title := fmt.Sprintf("%s · %d tracks", ansi.Truncate(name, max(inner-20, 8), "…"), pl.Tracks)

	switch {
	case m.trErr != "":
		return panel(title, wrap(m.trErr, styleErr, inner), w, h, focused)
	case m.trLoading:
		return panel(title, []string{styleMuted.Render(" Loading tracks…")}, w, h, focused)
	case len(m.tracks) == 0:
		return panel(title, []string{styleMuted.Render(" No playable tracks")}, w, h, focused)
	}

	// Columns: fixed #, time; the rest split between title/artist/album.
	const numW, timeW = 4, 6
	rest := max(inner-numW-timeW-4, 6) // 4 single-space gutters
	titleW := rest * 40 / 100
	artistW := rest * 30 / 100
	albumW := rest - titleW - artistW

	// Screen columns of each table column (the panel starts at the sidebar's
	// right edge, plus 1 for its border). Anchored so a title the terminal
	// measures differently than we do only mis-sizes its own cell.
	x := m.dims().sideW + 1
	titleX := x + numW + 1
	artistX := titleX + titleW + 1
	albumX := artistX + artistW + 1
	timeX := albumX + albumW + 1

	// Each cell is styled on its own and the anchors are added afterwards:
	// lipgloss styles some attributes (underline) character by character, which
	// would cut an escape sequence inside the styled text into visible junk.
	// The gutter spaces stay so the row's visible width is the full width; the
	// anchors only correct where each cell starts.
	row := func(num, title, artist, album, dur string, st lipgloss.Style) string {
		cell := func(s string, w int) string { return st.Render(fit(s, w)) }
		gap := st.Render(" ")
		return cell(num, numW) + gap + at(titleX) + cell(title, titleW) + gap + at(artistX) +
			cell(artist, artistW) + gap + at(albumX) + cell(album, albumW) + gap + at(timeX) +
			st.Render(fmt.Sprintf("%*s", timeW, dur))
	}

	lines := []string{row("#", "Title", "Artist", "Album", "Time", styleMuted.Underline(true))}
	end := min(m.trOffset+h-3, len(m.tracks))
	for i := m.trOffset; i < end; i++ {
		t := m.tracks[i]
		st, num := styleText, fmt.Sprintf("%d", i+1)
		if m.state != nil && m.state.Track.URI == t.URI {
			st, num = styleAccent.Bold(true), "▶"
			if !m.state.Playing {
				num = "❚❚"
			}
		}
		if focused && i == m.trCursor {
			st = st.Background(colSelBg).Foreground(colHi).Bold(true)
		}
		lines = append(lines, row(num, t.Title, t.Artist, t.Album, fmtDur(t.Duration), st))
	}
	return panel(title, lines, w, h, focused)
}
