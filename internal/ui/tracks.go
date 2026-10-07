package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func fmtDur(d time.Duration) string {
	s := int(d.Seconds())
	return fmt.Sprintf("%d:%02d", s/60, s%60)
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

	row := func(num, title, artist, album, dur string, st lipgloss.Style) string {
		cells := strings.Join([]string{
			fit(num, numW), fit(title, titleW), fit(artist, artistW), fit(album, albumW),
		}, " ")
		return st.Render(cells + " " + fmt.Sprintf("%*s", timeW, dur))
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
			st = st.Background(colSelBg)
		}
		lines = append(lines, row(num, t.Title, t.Artist, t.Album, fmtDur(t.Duration), st))
	}
	return panel(title, lines, w, h, focused)
}
