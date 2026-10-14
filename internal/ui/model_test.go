package ui

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"github.com/zenbiscuitslol/spoTERM/internal/mock"
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
	case "ctrl+s":
		return tea.KeyMsg{Type: tea.KeyCtrlS}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
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
	os.Setenv("SPOTERM_COVER", "blocks")
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
	withColor(t)
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

func TestVolumeKeys(t *testing.T) {
	tm := boot(110, 32)
	tm = press(tm, "tab", "enter", "+", "+", "-")
	if !strings.Contains(tm.View(), "vol 75%") {
		t.Errorf("expected vol 75%%:\n%s", tm.View())
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

func TestPlayerStartupReportsReady(t *testing.T) {
	started := false
	m := New(mock.New(), WithPlayer(func(context.Context) error { started = true; return nil }))
	var tm tea.Model = m
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 110, Height: 32})
	if !strings.Contains(tm.View(), "Starting player") {
		t.Error("footer should say the player is starting")
	}
	tm = settle(tm, m.Init())
	if !started {
		t.Fatal("Init should start the player")
	}
	if !strings.Contains(tm.View(), "Player ready") {
		t.Errorf("footer should report the player is ready:\n%s", tm.View())
	}
}

func TestPlayerStartupFailureIsShown(t *testing.T) {
	m := New(mock.New(), WithPlayer(func(context.Context) error { return errors.New("player did not appear") }))
	var tm tea.Model = m
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 110, Height: 32})
	tm = settle(tm, m.Init())
	if !strings.Contains(tm.View(), "player: player did not appear") {
		t.Errorf("failure should be visible:\n%s", tm.View())
	}
}

func TestNoPlayerOptionLeavesFooterAlone(t *testing.T) {
	tm := boot(110, 32)
	if strings.Contains(tm.View(), "player") {
		t.Error("no player was configured, so nothing about it should show")
	}
}

func TestProgressBarIsExactWidthAndMovesInsideACell(t *testing.T) {
	// Tests have no terminal, so colour is off; the sub-cell fade is colour.
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(old)

	total := 4 * time.Minute
	prevFilled := -1
	for e := time.Duration(0); e <= total; e += 3 * time.Second {
		bar := progressBar(e, total, 60)
		if w := ansi.StringWidth(bar); w != 60 {
			t.Fatalf("elapsed %v: bar is %d wide, want 60", e, w)
		}
		filled := strings.Count(ansi.Strip(bar), "━")
		if filled < prevFilled {
			t.Fatalf("elapsed %v: bar went backwards (%d -> %d)", e, prevFilled, filled)
		}
		prevFilled = filled
	}

	// Half a second later the cell the playhead is in has a different colour,
	// so the bar visibly advances well before it reaches the next cell.
	a, b := progressBar(100*time.Second, total, 60), progressBar(100500*time.Millisecond, total, 60)
	if a == b {
		t.Error("the bar should move sub-cell, not only when a whole cell fills")
	}
}

func TestPlaybackClockIsSmooth(t *testing.T) {
	tm := boot(110, 32)
	tm = press(tm, "tab", "enter") // start playing
	t0 := time.Unix(1_000_000, 0)
	tick := func(i int) { tm, _ = tm.Update(tickMsg(t0.Add(time.Duration(i) * 50 * time.Millisecond))) }

	for i := 0; i < 20; i++ {
		tick(i)
	}
	m := tm.(Model)
	base := m.elapsed

	// A poll that lands 1s behind (typical request latency) must not snap the clock back.
	behind := *m.state
	behind.Progress = base - time.Second
	tm, _ = tm.Update(stateMsg{state: &behind})
	if got := tm.(Model).elapsed; got != base {
		t.Fatalf("poll moved the clock from %v to %v; it should be absorbed gradually", base, got)
	}

	// Over the next ticks the clock keeps moving forward and converges.
	prev := base
	for i := 20; i < 120; i++ {
		tick(i)
		got := tm.(Model).elapsed
		if got < prev {
			t.Fatalf("tick %d: clock went backwards (%v -> %v)", i, prev, got)
		}
		prev = got
	}
	want := base - time.Second + 100*50*time.Millisecond // Spotify's position plus the time that passed
	if d := prev - want; d < -60*time.Millisecond || d > 60*time.Millisecond {
		t.Errorf("clock ended at %v, want about %v", prev, want)
	}

	// A big disagreement (a seek elsewhere) snaps immediately.
	far := *tm.(Model).state
	far.Progress = prev + 30*time.Second
	tm, _ = tm.Update(stateMsg{state: &far})
	if got := tm.(Model).elapsed; got != far.Progress {
		t.Errorf("a 30s jump should snap, got %v want %v", got, far.Progress)
	}
}

func TestLateFramesDoNotLoseTime(t *testing.T) {
	tm := boot(110, 32)
	tm = press(tm, "tab", "enter")
	t0 := time.Unix(1_000_000, 0)
	tm, _ = tm.Update(tickMsg(t0))
	start := tm.(Model).elapsed
	// Three frames that each ran 120ms late: a fixed 50ms/tick would lose 210ms.
	for i := 1; i <= 3; i++ {
		tm, _ = tm.Update(tickMsg(t0.Add(time.Duration(i) * 120 * time.Millisecond)))
	}
	if got := tm.(Model).elapsed - start; got != 360*time.Millisecond {
		t.Errorf("advanced %v over 360ms of real time", got)
	}
}

func TestThemeKeyCycles(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Cleanup(func() { setTheme(0) })
	setTheme(0)
	m := New(mock.New())
	m.w, m.h = 100, 30
	for i := 1; i <= len(themes); i++ {
		m2, _ := m.onKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
		m = m2.(Model)
		if themeIdx != i%len(themes) {
			t.Fatalf("press %d: themeIdx=%d", i, themeIdx)
		}
	}
	LoadTheme() // last save was theme 0 again
	if themeIdx != 0 {
		t.Fatalf("LoadTheme: %d", themeIdx)
	}
}

func TestLocalQueueAddRemoveSkipAndShuffle(t *testing.T) {
	tm := boot(110, 32)
	tm = press(tm, "tab", "enter", "down", "down", "a", "down", "a") // play #1, queue #3 and #4
	if got := len(tm.(Model).lq); got != 2 {
		t.Fatalf("local queue has %d tracks, want 2", got)
	}
	tm = press(tm, "Q")
	if v := tm.View(); !strings.Contains(v, "Your queue") || !strings.Contains(v, "Static Bloom") {
		t.Fatalf("queue view missing entries:\n%s", v)
	}
	tm = press(tm, "x") // remove Static Bloom
	if q := tm.(Model).lq; len(q) != 1 || q[0].Title == "Static Bloom" {
		t.Fatalf("x should remove the highlighted entry, got %+v", q)
	}
	tm = press(tm, "esc", "n")
	if m := tm.(Model); len(m.lq) != 0 || m.state.Track.Title != m.tracks[3].Title {
		t.Fatalf("n should hand the head to Spotify and play it, state=%q lq=%d", m.state.Track.Title, len(m.lq))
	}
	tm = press(tm, "s")
	if !tm.(Model).state.Shuffle {
		t.Error("s should turn shuffle on")
	}
}

func TestQueuePushedNearEndOfTrack(t *testing.T) {
	tm := boot(110, 32)
	tm = press(tm, "tab", "enter", "down", "down", "a")
	m := tm.(Model)
	s := *m.state
	s.Progress = s.Track.Duration - 5*time.Second
	tm = settle(tm, func() tea.Msg { return stateMsg{state: &s} })
	if m = tm.(Model); len(m.lq) != 0 {
		t.Fatalf("head should have been pushed, lq=%d", len(m.lq))
	}
	if q, _ := m.be.Queue(context.Background()); len(q) == 0 || q[0].Title != "Static Bloom" {
		t.Errorf("Spotify queue should start with the pushed track, got %+v", q)
	}
}
