package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/zenbiscuitslol/spoTERM/internal/spotify"
)

func init() { searchDelay = 0 }

func TestSearchTypingDoesNotTriggerGlobals(t *testing.T) {
	tm := press(boot(110, 32), "ctrl+s", "q", "t", " ", "n")
	m := tm.(Model)
	if !m.srch.open || m.srch.input != "qt n" {
		t.Fatalf("open=%v input=%q", m.srch.open, m.srch.input)
	}
	if themeIdx != 0 {
		t.Error("typing 't' changed the theme")
	}
}

func TestSearchShowsResultsAndDetails(t *testing.T) {
	tm := press(boot(110, 32), "ctrl+s", "m", "a", "r", "l", "o", "w", "e")
	v := tm.View()
	for _, want := range []string{"Songs · 2", "Albums · 1", "Artists · 1", "Run Slow", "Marlowe Park", "enter: play"} {
		if !strings.Contains(v, want) {
			t.Errorf("missing %q in\n%s", want, v)
		}
	}
	if strings.Contains(v, "\x00") {
		t.Error("border mark leaked")
	}
}

func TestSearchNoResults(t *testing.T) {
	tm := press(boot(110, 32), "ctrl+s", "z", "z", "z")
	if v := tm.View(); !strings.Contains(v, "No results") {
		t.Errorf("want no-results message:\n%s", v)
	}
}

func TestSearchEnterPlaysSongAndCloses(t *testing.T) {
	tm := press(boot(110, 32), "ctrl+s", "n", "e", "o", "n", "enter") // type, then enter picks first song
	m := tm.(Model)
	if m.srch.open {
		t.Fatal("overlay should close")
	}
	if m.state == nil || m.state.Track.Title != "Neon Horizons" || !m.state.Playing {
		t.Fatalf("state=%+v", m.state)
	}
}

func TestSearchEnterOpensAlbumAndArtist(t *testing.T) {
	tm := press(boot(110, 32), "ctrl+s", "m", "a", "r", "l", "o", "w", "e", "tab", "enter")
	m := tm.(Model)
	if m.srch.open || !m.opened.Album || m.opened.Name != "Run Slow" || len(m.tracks) != 2 {
		t.Fatalf("album: open=%v opened=%+v tracks=%d", m.srch.open, m.opened, len(m.tracks))
	}
	tm = press(tm, "ctrl+s", "tab", "enter") // sections wrap: albums -> artists, then open
	m = tm.(Model)
	if !m.opened.IsArtist || m.opened.Name != "Marlowe Park" || len(m.tracks) != 2 || m.opened.Tracks != 2 {
		t.Fatalf("artist: opened=%+v tracks=%d", m.opened, len(m.tracks))
	}
}

func TestSearchStaleReplyDropped(t *testing.T) {
	m := boot(110, 32).(Model)
	m.srch.open, m.srch.input, m.srch.seq = true, "ab", 5
	tm, _ := m.Update(searchMsg{seq: 4, query: "a", res: spotify.SearchResults{Tracks: []spotify.Track{{Title: "old"}}}})
	if len(tm.(Model).srch.res.Tracks) != 0 {
		t.Error("stale reply was applied")
	}
}

func TestSearchEscClosesAndKeepsQuery(t *testing.T) {
	tm := press(boot(110, 32), "ctrl+s", "a", "esc")
	m := tm.(Model)
	if m.srch.open || m.srch.input != "a" {
		t.Fatalf("open=%v input=%q", m.srch.open, m.srch.input)
	}
	tm = press(tm, "q") // back in the main view, q quits
	if _, cmd := tm.Update(key("q")); cmd == nil {
		t.Error("q should quit outside search")
	}
}

func TestSearchViewFitsTerminal(t *testing.T) {
	withColor(t)
	for i := range themes {
		setTheme(i)
		for _, sz := range [][2]int{{60, 19}, {110, 32}, {200, 50}} {
			tm := press(boot(sz[0], sz[1]), "ctrl+s", "a")
			lines := strings.Split(tm.View(), "\n")
			if len(lines) != sz[1] {
				t.Errorf("%s %dx%d: %d lines", themes[i].name, sz[0], sz[1], len(lines))
			}
			for j, l := range lines {
				if w := ansi.StringWidth(l); w > sz[0] {
					t.Errorf("%s %dx%d: line %d is %d wide", themes[i].name, sz[0], sz[1], j, w)
				}
			}
		}
	}
	setTheme(0)
}

func TestSearchArrowsFlowIntoAlbumsAndArtists(t *testing.T) {
	// "marlowe": 2 songs, 1 album, 1 artist.
	tm := press(boot(110, 32), "ctrl+s", "m", "a", "r", "l", "o", "w", "e")
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyDown})
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m := tm.(Model); m.srch.sec != secAlbums || m.srch.cur[secAlbums] != 0 {
		t.Fatalf("sec=%d cur=%v", m.srch.sec, m.srch.cur)
	}
	tm = press(tm, "enter")
	if m := tm.(Model); m.srch.open || m.opened.Name != "Run Slow" || !m.opened.Album {
		t.Fatalf("album not opened: %+v", m.opened)
	}
	tm = press(tm, "ctrl+s")
	for i := 0; i < 3; i++ {
		tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyUp})
	}
	if m := tm.(Model); m.srch.sec != secTracks || m.srch.cur[secTracks] != 0 {
		t.Fatalf("up: sec=%d cur=%v", m.srch.sec, m.srch.cur)
	}
}
