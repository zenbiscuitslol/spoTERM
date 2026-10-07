// Package ui contains the Bubble Tea model and all rendering for spotiCLI.
package ui

import (
	"context"
	"errors"
	"image"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/zenbiscuitslol/spotiCLI/internal/spotify"
)

const (
	frameRate = 20
	tickEvery = time.Second / frameRate

	pollEvery = 4 * frameRate // ticks between playback polls (~4s)
	statusTTL = 6 * frameRate // how long a footer message stays

	minW, minH = 60, 19
)

// refreshDelay is how long to wait after a command before re-reading state;
// Spotify's playback state lags a beat behind. Tests set it to zero.
var refreshDelay = 400 * time.Millisecond

type pane int

const (
	paneSidebar pane = iota
	paneTracks
)

// sideTab selects what the sidebar lists.
type sideTab int

const (
	tabPlaylists sideTab = iota
	tabAlbums
	numTabs
)

func (t sideTab) String() string {
	if t == tabAlbums {
		return "Albums"
	}
	return "Playlists"
}

// sideList is one sidebar tab's contents and scroll state. Albums are
// playlist-shaped (Album set), so both tabs share the open/play flow.
type sideList struct {
	items          []spotify.Playlist
	loaded         bool
	loading        bool
	err            string
	cursor, offset int
}

// Messages produced by background commands.
type (
	tickMsg time.Time
	listMsg struct {
		tab  sideTab
		list []spotify.Playlist
		err  error
	}
	tracksMsg struct {
		pl     spotify.Playlist
		tracks []spotify.Track
		err    error
	}
	stateMsg struct {
		state *spotify.State
		err   error
	}
	coverMsg struct {
		url string
		img image.Image
		err error
	}
)

type Model struct {
	w, h int
	be   Backend

	focus pane

	// Sidebar: one list per tab.
	tab  sideTab
	side [numTabs]sideList

	// Track table for the opened playlist or album.
	opened    spotify.Playlist
	hasOpen   bool
	tracks    []spotify.Track
	trLoading bool
	trErr     string
	trCursor  int
	trOffset  int

	// Playback, mirrored from the API and interpolated between polls.
	state    *spotify.State
	elapsed  time.Duration
	coverURL string

	// Housekeeping.
	ticks    int
	lastPoll int
	polling  bool
	status   string
	statusAt int

	viz   *visualizer
	cover *coverRenderer
}

func New(be Backend) Model {
	m := Model{
		be:    be,
		viz:   newVisualizer(),
		cover: newCoverRenderer(nil),
	}
	m.side[tabPlaylists].loading = true // Init loads it
	return m
}

func (m Model) Init() tea.Cmd {
	m.polling = true // the first poll is issued below
	return tea.Batch(tick(), m.loadList(tabPlaylists), m.pollCmd())
}

func tick() tea.Cmd {
	return tea.Tick(tickEvery, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// --- commands ---------------------------------------------------------------

func call[T any](timeout time.Duration, f func(ctx context.Context) T) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		return f(ctx)
	}
}

func (m Model) loadList(tab sideTab) tea.Cmd {
	return call(30*time.Second, func(ctx context.Context) tea.Msg {
		var (
			l   []spotify.Playlist
			err error
		)
		if tab == tabAlbums {
			l, err = m.be.Albums(ctx)
		} else {
			l, err = m.be.Playlists(ctx)
		}
		return listMsg{tab, l, err}
	})
}

func (m Model) loadTracks(pl spotify.Playlist) tea.Cmd {
	return call(45*time.Second, func(ctx context.Context) tea.Msg {
		t, err := m.be.Tracks(ctx, pl)
		return tracksMsg{pl, t, err}
	})
}

func (m Model) pollCmd() tea.Cmd {
	return call(15*time.Second, func(ctx context.Context) tea.Msg {
		s, err := m.be.Playback(ctx)
		return stateMsg{s, err}
	})
}

func (m Model) loadCover(url string) tea.Cmd {
	return call(20*time.Second, func(ctx context.Context) tea.Msg {
		img, err := m.be.Image(ctx, url)
		return coverMsg{url, img, err}
	})
}

// act runs a playback command, then re-reads the state shortly after so the
// UI reflects what Spotify actually did.
func (m Model) act(f func(ctx context.Context, be Backend) error) tea.Cmd {
	return call(20*time.Second, func(ctx context.Context) tea.Msg {
		if err := f(ctx, m.be); err != nil {
			return stateMsg{err: err}
		}
		time.Sleep(refreshDelay)
		s, err := m.be.Playback(ctx)
		return stateMsg{s, err}
	})
}

// --- layout -----------------------------------------------------------------

type dims struct{ sideW, mainW, bodyH, tracksH, npH int }

func (m Model) dims() dims {
	bodyH := m.h - 1 // 1 line for the footer
	sideW := clamp(m.w/4, 24, 34)
	npH := clamp(bodyH-10, 8, 12) + 2
	return dims{sideW, m.w - sideW, bodyH, bodyH - npH, npH}
}

func (d dims) sideRows() int  { return d.bodyH - 3 }   // borders + tab strip
func (d dims) trackRows() int { return d.tracksH - 3 } // borders + header row

// --- update -----------------------------------------------------------------

func (m *Model) setStatus(s string) { m.status, m.statusAt = s, m.ticks }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.reclamp()

	case tea.KeyMsg:
		return m.onKey(msg)

	case tickMsg:
		return m.onTick()

	case listMsg:
		sl := &m.side[msg.tab]
		sl.loaded, sl.loading = true, false
		if msg.err != nil {
			sl.err = msg.err.Error()
			return m, nil
		}
		sl.items = msg.list
		// Only the initial playlist load opens something; opening the first
		// album would replace the tracks the user is looking at.
		if msg.tab == tabPlaylists && len(sl.items) > 0 {
			return m.openAt(0, false)
		}

	case tracksMsg:
		if !m.hasOpen || m.opened.ID != msg.pl.ID {
			return m, nil // stale: user opened something else meanwhile
		}
		m.trLoading = false
		// Correct the owner-based guess in the sidebar with what Spotify said.
		switch {
		case errors.Is(msg.err, spotify.ErrPlaylistForbidden):
			m.setLocked(msg.pl.ID, true)
		case msg.err == nil:
			m.setLocked(msg.pl.ID, false)
		}
		if msg.err != nil {
			m.trErr = msg.err.Error()
			return m, nil
		}
		m.tracks = msg.tracks

	case stateMsg:
		m.polling = false
		if msg.err != nil {
			m.setStatus(msg.err.Error())
			return m, nil
		}
		m.state = msg.state
		if msg.state == nil {
			return m, nil
		}
		m.elapsed = msg.state.Progress
		if url := msg.state.Track.ImageURL; url != "" && url != m.coverURL {
			m.coverURL = url
			return m, m.loadCover(url)
		}

	case coverMsg:
		if msg.err != nil {
			m.setStatus("cover: " + msg.err.Error())
		} else if msg.url == m.coverURL {
			m.cover.SetImage(msg.img)
		}
	}
	return m, nil
}

func (m Model) onTick() (tea.Model, tea.Cmd) {
	m.ticks++
	playing := m.state != nil && m.state.Playing
	m.viz.Step(tickEvery.Seconds(), playing)
	if playing {
		m.elapsed += tickEvery
	}
	if m.status != "" && m.ticks-m.statusAt > statusTTL {
		m.status = ""
	}

	cmds := []tea.Cmd{tick()}
	if m.pollDue() {
		m.polling, m.lastPoll = true, m.ticks
		cmds = append(cmds, m.pollCmd())
	}
	return m, tea.Batch(cmds...)
}

// pollDue reports whether to re-read playback now: periodically, and right
// after the current track should have ended. Never more than once a second.
func (m Model) pollDue() bool {
	if m.polling || m.ticks < m.lastPoll+frameRate {
		return false
	}
	ended := m.state != nil && m.state.Playing && m.elapsed >= m.state.Track.Duration
	return ended || m.ticks >= m.lastPoll+pollEvery
}

func (m Model) onKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "tab":
		m.focus = 1 - m.focus
	case "h", "left":
		m.focus = paneSidebar
	case "l", "right":
		m.focus = paneTracks

	case "up", "k":
		m.move(-1)
	case "down", "j":
		m.move(1)

	case "1":
		return m.switchTab(tabPlaylists)
	case "2":
		return m.switchTab(tabAlbums)
	case "[":
		return m.switchTab((m.tab + numTabs - 1) % numTabs)
	case "]":
		return m.switchTab((m.tab + 1) % numTabs)

	case "enter":
		if m.focus == paneSidebar {
			return m.openAt(m.side[m.tab].cursor, true)
		}
		return m.playSelected()

	case " ":
		return m.togglePause()
	case "n":
		return m.skip(func(ctx context.Context, be Backend) error { return be.Next(ctx) })
	case "p":
		return m.skip(func(ctx context.Context, be Backend) error { return be.Previous(ctx) })
	}
	return m, nil
}

func (m *Model) move(delta int) {
	d := m.dims()
	if m.focus == paneSidebar {
		sl := &m.side[m.tab]
		sl.cursor = clamp(sl.cursor+delta, 0, max(len(sl.items)-1, 0))
		sl.offset = follow(sl.cursor, sl.offset, d.sideRows())
		return
	}
	m.trCursor = clamp(m.trCursor+delta, 0, max(len(m.tracks)-1, 0))
	m.trOffset = follow(m.trCursor, m.trOffset, d.trackRows())
}

// reclamp keeps scroll offsets valid after a resize.
func (m *Model) reclamp() {
	d := m.dims()
	for i := range m.side {
		sl := &m.side[i]
		sl.offset = follow(sl.cursor, sl.offset, d.sideRows())
	}
	m.trOffset = follow(m.trCursor, m.trOffset, d.trackRows())
}

// setLocked updates the dimmed state of a playlist in whichever tab holds it.
func (m *Model) setLocked(id string, locked bool) {
	for i := range m.side {
		for j := range m.side[i].items {
			if p := &m.side[i].items[j]; p.ID == id && !p.Album && !p.Liked {
				p.Locked = locked
			}
		}
	}
}

// switchTab shows another sidebar list, loading it on first use (or retrying
// after an error).
func (m Model) switchTab(t sideTab) (tea.Model, tea.Cmd) {
	m.tab, m.focus = t, paneSidebar
	sl := &m.side[t]
	if sl.loading || (sl.loaded && sl.err == "") {
		return m, nil
	}
	sl.loading, sl.err = true, ""
	return m, m.loadList(t)
}

// openAt opens entry i of the current tab's list in the track table.
func (m Model) openAt(i int, focusTracks bool) (tea.Model, tea.Cmd) {
	items := m.side[m.tab].items
	if i < 0 || i >= len(items) {
		return m, nil
	}
	m.opened, m.hasOpen = items[i], true
	m.tracks, m.trErr, m.trLoading = nil, "", true
	m.trCursor, m.trOffset = 0, 0
	if focusTracks {
		m.focus = paneTracks
	}
	return m, m.loadTracks(m.opened)
}

func (m Model) playSelected() (tea.Model, tea.Cmd) {
	if !m.hasOpen || m.trCursor >= len(m.tracks) {
		return m, nil
	}
	pl, tracks, idx := m.opened, m.tracks, m.trCursor
	m.setStatus("Starting " + tracks[idx].Title + "…")
	return m, m.act(func(ctx context.Context, be Backend) error {
		return be.Play(ctx, pl, tracks, idx)
	})
}

func (m Model) togglePause() (tea.Model, tea.Cmd) {
	if m.state == nil {
		m.setStatus("Nothing playing — pick a track and press enter")
		return m, nil
	}
	// Optimistic: flip locally, the follow-up poll corrects it if needed.
	s := *m.state
	wasPlaying := s.Playing
	s.Playing = !wasPlaying
	m.state = &s
	return m, m.act(func(ctx context.Context, be Backend) error {
		if wasPlaying {
			return be.Pause(ctx)
		}
		return be.Resume(ctx)
	})
}

func (m Model) skip(f func(context.Context, Backend) error) (tea.Model, tea.Cmd) {
	if m.state == nil {
		m.setStatus("Nothing playing — pick a track and press enter")
		return m, nil
	}
	return m, m.act(f)
}

// --- view -------------------------------------------------------------------

func (m Model) View() string {
	if m.w == 0 {
		return ""
	}
	if m.w < minW || m.h < minH {
		return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center,
			styleMuted.Render("Terminal too small — need at least 60x19"))
	}

	d := m.dims()
	main := lipgloss.JoinVertical(lipgloss.Left,
		m.tracksView(d.mainW, d.tracksH),
		m.nowPlayingView(d.mainW, d.npH),
	)
	body := lipgloss.JoinHorizontal(lipgloss.Top, m.sidebarView(d.sideW, d.bodyH), main)
	return body + "\n" + m.footer()
}

func (m Model) footer() string {
	if m.status != "" {
		return " " + styleErr.Render(fit(m.status, m.w-2))
	}
	return styleMuted.Render(fit(
		" ↑↓ move · tab pane · 1/2 list · enter open/play · space pause · n/p skip · q quit", m.w))
}
