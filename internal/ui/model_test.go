package ui

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/zenbiscuitslol/spotiCLI/internal/mock"
)

// settle runs a command and feeds its resulting messages back into the model
// until nothing is left. Animation ticks are dropped so it terminates.
func settle(tm tea.Model, cmd tea.Cmd) tea.Model {
	if cmd == nil {
		return tm
	}
	switch msg := cmd().(type) {
	case tickMsg:
		return tm
	case tea.BatchMsg:
		for _, c := range msg {
			tm = settle(tm, c)
		}
		return tm
	default:
		tm, next := tm.Update(msg)
		return settle(tm, next)
	}
}

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case " ":
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func press(tm tea.Model, keys ...string) tea.Model {
	for _, k := range keys {
		var cmd tea.Cmd
		tm, cmd = tm.Update(key(k))
		tm = settle(tm, cmd)
	}
	return tm
}

func init() {
	refreshDelay = 0
	// Keep tests independent of the terminal they happen to run in.
	os.Setenv("SPOTICLI_COVER", "blocks")
}

func tmModel(m Model) tea.Model { return m }

func winSize(w, h int) tea.Msg { return tea.WindowSizeMsg{Width: w, Height: h} }

// boot returns a model of the given size that has finished its initial load.
func boot(w, h int) tea.Model {
	m := New(mock.New())
	var tm tea.Model = m
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return settle(tm, m.Init())
}

func TestInitialLoadShowsPlaylistsAndTracks(t *testing.T) {
	v := boot(110, 32).View()
	for _, want := range []string{"Playlists · 16", "Liked Songs", "Discover Weekly", "Neon Horizons", "Nothing playing"} {
		if !strings.Contains(v, want) {
			t.Errorf("view is missing %q", want)
		}
	}
}

// Every rendered frame must fit the terminal, otherwise it scrolls or tears.
func TestViewFitsTerminal(t *testing.T) {
	for _, sz := range [][2]int{{60, 19}, {80, 24}, {120, 40}, {200, 50}} {
		tm := boot(sz[0], sz[1])
		tm = press(tm, "tab", "enter", "down", "down") // start playing
		for i := 0; i < 30; i++ {
			tm, _ = tm.Update(tickMsg{})
		}
		lines := strings.Split(tm.View(), "\n")
		if len(lines) != sz[1] {
			t.Errorf("%dx%d: got %d lines", sz[0], sz[1], len(lines))
		}
		for i, l := range lines {
			if w := ansi.StringWidth(l); w > sz[0] {
				t.Errorf("%dx%d: line %d is %d wide", sz[0], sz[1], i, w)
			}
		}
	}
}

func TestTooSmall(t *testing.T) {
	var tm tea.Model = New(mock.New())
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 30, Height: 10})
	if !strings.Contains(tm.View(), "too small") {
		t.Error("expected too-small message")
	}
}

func TestPlayPauseNextPrevious(t *testing.T) {
	tm := boot(110, 32)
	tm = press(tm, "tab", "down", "down", "enter") // third track
	v := tm.View()
	if !strings.Contains(v, "▶ Playing") || !strings.Contains(v, "Static Bloom") {
		t.Fatalf("expected Static Bloom playing:\n%s", v)
	}

	tm = press(tm, " ")
	if !strings.Contains(tm.View(), "Paused") {
		t.Error("space should pause")
	}
	tm = press(tm, " ")
	if !strings.Contains(tm.View(), "▶ Playing") {
		t.Error("space should resume")
	}

	tm = press(tm, "n")
	if !strings.Contains(tm.View(), "Run Slow") {
		t.Error("n should skip to the next track")
	}
	tm = press(tm, "p", "p")
	if !strings.Contains(tm.View(), "Paper Planes in July") {
		t.Error("p should go back")
	}
}

func TestOpenOtherPlaylistReloadsTracks(t *testing.T) {
	tm := boot(110, 32)
	tm = press(tm, "j", "enter")
	v := tm.View()
	if !strings.Contains(v, "Discover Weekly · 30 tracks") {
		t.Errorf("opened playlist title missing:\n%s", v)
	}
}

func TestSpaceWithNothingPlayingExplains(t *testing.T) {
	tm := press(boot(110, 32), " ")
	if !strings.Contains(tm.View(), "Nothing playing — pick a track") {
		t.Error("expected hint in footer")
	}
}

func TestSidebarScrollsToKeepCursorVisible(t *testing.T) {
	tm := boot(80, 20) // too short to show all 16 playlists
	for i := 0; i < 15; i++ {
		tm = press(tm, "j")
	}
	if v := tm.View(); !strings.Contains(v, "Acoustic Even") {
		t.Errorf("last playlist should have scrolled into view:\n%s", v)
	}
}

func TestAlbumsTabLoadsListsAndPlays(t *testing.T) {
	tm := boot(110, 32)
	if strings.Contains(tm.View(), "Albums · ") {
		t.Fatal("albums should not load before the tab is opened")
	}

	tm = press(tm, "2")
	v := tm.View()
	for _, want := range []string{"Albums · 10", "Afterglow", "Static Bloom"} {
		if !strings.Contains(v, want) {
			t.Errorf("albums view is missing %q:\n%s", want, v)
		}
	}
	// Opening the tab must not replace the tracks the user was looking at.
	if !strings.Contains(v, "Neon Horizons") {
		t.Error("switching tabs should keep the open track table")
	}

	tm = press(tm, "j", "enter") // second album: Small Hours
	v = tm.View()
	if !strings.Contains(v, "Small Hours — Wren & the Tides · 2 tracks") || strings.Contains(v, "Neon Horizons") {
		t.Errorf("album tracks not shown:\n%s", v)
	}

	tm = press(tm, "enter")
	if v = tm.View(); !strings.Contains(v, "▶ Playing") || !strings.Contains(v, "Paper Planes in July") {
		t.Errorf("album track should play:\n%s", v)
	}

	// Back on playlists, the cursor is where it was left.
	if v = press(tm, "1").View(); !strings.Contains(v, "Playlists · 16") {
		t.Errorf("tab 1 should return to playlists:\n%s", v)
	}
}

func TestTabBracketsCycle(t *testing.T) {
	tm := press(boot(110, 32), "]")
	if !strings.Contains(tm.View(), "Albums · ") {
		t.Error("] should go to albums")
	}
	if tm = press(tm, "["); !strings.Contains(tm.View(), "Playlists · 16") {
		t.Error("[ should go back to playlists")
	}
}

func TestLockedPlaylistsAreMarkedAndCorrectedByLoading(t *testing.T) {
	tm := boot(110, 40)
	if v := tm.View(); !strings.Contains(v, "⊘ Sunday Morning") || !strings.Contains(v, "⊘ Acoustic Evenings") {
		t.Fatalf("followed playlists should be marked:\n%s", v)
	}
	locked := func(name string) bool {
		for _, p := range tm.(Model).side[tabPlaylists].items {
			if p.Name == name {
				return p.Locked
			}
		}
		t.Fatalf("no playlist %q", name)
		return false
	}

	for i := 0; i < 14; i++ {
		tm = press(tm, "j")
	}
	tm = press(tm, "enter") // Sunday Morning: guessed locked, but its tracks load
	if locked("Sunday Morning") {
		t.Error("a playlist whose tracks load should be unlocked")
	}

	tm = press(tm, "h", "j", "enter") // Acoustic Evenings: really forbidden
	v := tm.View()
	if !locked("Acoustic Evenings") || !strings.Contains(v, "only exposes tracks of playlists you own") {
		t.Errorf("forbidden playlist should stay locked and explain why:\n%s", v)
	}
}
