package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/zenbiscuitslol/spoTERM/internal/spotify"
)

// searchDelay is how long typing must pause before a search is sent.
// Tests set it to zero.
var searchDelay = 300 * time.Millisecond

const (
	secTracks = iota
	secAlbums
	secArtists
	numSecs
)

var secNames = [numSecs]string{"Songs", "Albums", "Artists"}

type (
	searchDebounceMsg struct{ seq int }
	searchMsg         struct {
		seq   int
		query string
		res   spotify.SearchResults
		err   error
	}
)

// searchState is the full-screen search overlay.
type searchState struct {
	open     bool
	input    string
	searched string // query the shown results belong to
	seq      int    // bumped on every edit; stale replies are dropped
	loading  bool
	err      string
	res      spotify.SearchResults
	sec      int
	cur, off [numSecs]int
}

func (s *searchState) count(sec int) int {
	switch sec {
	case secTracks:
		return len(s.res.Tracks)
	case secAlbums:
		return len(s.res.Albums)
	}
	return len(s.res.Artists)
}

// step moves the cursor one result, flowing into the next or previous
// non-empty section at the edges so down/up reach every result.
func (s *searchState) step(d int) {
	if i := s.cur[s.sec] + d; i >= 0 && i < s.count(s.sec) {
		s.cur[s.sec] = i
		return
	}
	for n := 1; n < numSecs; n++ {
		sec := ((s.sec+d*n)%numSecs + numSecs) % numSecs
		if (d > 0 && sec < s.sec) || (d < 0 && sec > s.sec) {
			break // don't wrap around
		}
		if c := s.count(sec); c > 0 {
			s.sec = sec
			s.cur[sec] = map[bool]int{true: 0, false: c - 1}[d > 0]
			return
		}
	}
}

// rows is how many result rows each section gets for a body of h lines.
func searchRows(h int) int { return max((h-numSecs)/numSecs, 1) }

func (m Model) searchDims() (inputH, resH, resW, detW int) {
	inputH = 3
	resH = m.h - 1 - inputH
	resW = m.w * 6 / 10
	return inputH, resH, resW, m.w - resW
}

func (m Model) searchCmd() tea.Cmd {
	q, seq := strings.TrimSpace(m.srch.input), m.srch.seq
	return call(20*time.Second, func(ctx context.Context) tea.Msg {
		res, err := m.be.Search(ctx, q)
		return searchMsg{seq, q, res, err}
	})
}

// startSearch sends the current input now; it is a no-op for an empty one.
func (m Model) startSearch() (tea.Model, tea.Cmd) {
	if strings.TrimSpace(m.srch.input) == "" {
		return m, nil
	}
	m.srch.loading, m.srch.err = true, ""
	return m, m.searchCmd()
}

func (m Model) onSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := &m.srch
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "ctrl+s":
		s.open = false
	case "tab", "right":
		s.sec = (s.sec + 1) % numSecs
	case "shift+tab", "left":
		s.sec = (s.sec + numSecs - 1) % numSecs
	case "up":
		s.step(-1)
	case "down":
		s.step(1)
	case "enter":
		if strings.TrimSpace(s.input) != s.searched || s.count(s.sec) == 0 {
			s.seq++ // supersede the pending debounce
			return m.startSearch()
		}
		return m.pickResult()
	case "backspace", "ctrl+h":
		if r := []rune(s.input); len(r) > 0 {
			s.input = string(r[:len(r)-1])
		}
		return m.inputChanged()
	case "ctrl+u":
		s.input = ""
		return m.inputChanged()
	default:
		if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
			s.input += string(msg.Runes)
			return m.inputChanged()
		}
	}
	s.off[s.sec] = follow(s.cur[s.sec], s.off[s.sec], searchRows(m.searchBodyH()))
	return m, nil
}

func (m Model) searchBodyH() int {
	_, resH, _, _ := m.searchDims()
	return resH - 2
}

// inputChanged schedules a debounced search for the new input.
func (m Model) inputChanged() (tea.Model, tea.Cmd) {
	m.srch.seq++
	seq := m.srch.seq
	if strings.TrimSpace(m.srch.input) == "" {
		m.srch.res, m.srch.searched, m.srch.loading, m.srch.err = spotify.SearchResults{}, "", false, ""
		return m, nil
	}
	return m, tea.Tick(searchDelay, func(time.Time) tea.Msg { return searchDebounceMsg{seq} })
}

func (m Model) onSearchResult(msg searchMsg) (tea.Model, tea.Cmd) {
	s := &m.srch
	if msg.seq != s.seq {
		return m, nil // typed more since: a newer search is coming
	}
	s.loading = false
	if msg.err != nil {
		s.err = msg.err.Error()
		return m, nil
	}
	s.res, s.searched, s.err = msg.res, msg.query, ""
	s.cur, s.off, s.sec = [numSecs]int{}, [numSecs]int{}, 0
	for s.sec < numSecs-1 && s.count(s.sec) == 0 {
		s.sec++
	}
	return m, nil
}

// pickResult plays the chosen song, or opens the chosen album or artist in
// the track table, and closes the overlay.
func (m Model) pickResult() (tea.Model, tea.Cmd) {
	s := &m.srch
	i := s.cur[s.sec]
	switch s.sec {
	case secTracks:
		t := s.res.Tracks[i]
		s.open = false
		m.setStatus("Starting " + t.Title + "…")
		return m, m.act(func(ctx context.Context, be Backend) error {
			return be.Play(ctx, spotify.Playlist{}, []spotify.Track{t}, 0)
		})
	case secAlbums:
		s.open = false
		return m.openPlaylist(s.res.Albums[i], true)
	}
	s.open = false
	return m.openPlaylist(s.res.Artists[i], true)
}

// --- view -------------------------------------------------------------------

func (m Model) searchView() string {
	inputH, resH, resW, detW := m.searchDims()
	s := m.srch

	in := []string{" " + styleAccent.Render("› ") + styleText.Render(tail(s.input, m.w-8)) + styleAccent.Render("▌")}
	if s.input == "" {
		in = []string{" " + styleAccent.Render("› ") + styleMuted.Render("type to search songs, albums and artists") + styleAccent.Render("▌")}
	}
	top := anchorBorders(panel("Search", in, m.w, inputH, true), m.w-1)

	innerW, innerH := resW-2, resH-2
	per := searchRows(innerH)
	var lines []string
	for sec := 0; sec < numSecs; sec++ {
		head := fmt.Sprintf(" %s · %d", secNames[sec], s.count(sec))
		if sec == s.sec {
			lines = append(lines, styleAccent.Bold(true).Render(head))
		} else {
			lines = append(lines, styleMuted.Render(head))
		}
		end := min(s.off[sec]+per, s.count(sec))
		for i := s.off[sec]; i < end; i++ {
			st := styleText
			if sec == s.sec && i == s.cur[sec] {
				st = st.Background(colSelBg).Foreground(colHi).Bold(true)
			}
			lines = append(lines, st.Render(fit(resultLine(s.res, sec, i, innerW), innerW)))
		}
		for n := end - s.off[sec]; n < per; n++ { // keep sections a fixed height
			lines = append(lines, "")
		}
	}

	res := panel("Results", lines, resW, resH, false)
	det := panel("Details", m.searchDetails(detW-2), detW, resH, false)
	return top + "\n" + joinBody(res, det, resW, detW) + "\n" + m.searchFooter()
}

func tail(s string, w int) string {
	if r := []rune(s); w > 0 && len(r) > w {
		return string(r[len(r)-w:])
	}
	return s
}

// resultLine is the plain text of one result row.
func resultLine(r spotify.SearchResults, sec, i, w int) string {
	switch sec {
	case secTracks:
		t := r.Tracks[i]
		return " " + spread(t.Title+" — "+t.Artist, fmtDur(t.Duration)+" ", w-1)
	case secAlbums:
		a := r.Albums[i]
		return " " + spread(a.Name+" — "+a.Artist, a.Year+" ", w-1)
	}
	return " " + r.Artists[i].Name
}

func (m Model) searchDetails(w int) []string {
	s := m.srch
	pad := func(l []string) []string {
		for i := range l {
			l[i] = " " + l[i]
		}
		return l
	}
	switch {
	case s.err != "":
		return wrap(s.err, styleErr, w)
	case s.loading:
		return []string{styleMuted.Render(" Searching…")}
	case strings.TrimSpace(s.input) == "":
		return []string{styleMuted.Render(" Type a song, album or artist.")}
	case s.searched == "":
		return nil
	case s.count(secTracks)+s.count(secAlbums)+s.count(secArtists) == 0:
		return []string{styleMuted.Render(" No results for \"" + s.searched + "\"")}
	case s.count(s.sec) == 0:
		return []string{styleMuted.Render(" No " + strings.ToLower(secNames[s.sec]) + " found")}
	}
	i := s.cur[s.sec]
	switch s.sec {
	case secTracks:
		t := s.res.Tracks[i]
		return pad([]string{styleBold.Render(t.Title), styleText.Render(t.Artist), styleMuted.Render(t.Album),
			styleMuted.Render(fmtDur(t.Duration)), "", styleAccent.Render("enter: play")})
	case secAlbums:
		a := s.res.Albums[i]
		return pad([]string{styleBold.Render(a.Name), styleText.Render(a.Artist),
			styleMuted.Render(fmt.Sprintf("%s · %d tracks", a.Year, a.Tracks)), "", styleAccent.Render("enter: open album")})
	}
	a := s.res.Artists[i]
	return pad(append(wrap(a.Name, styleBold, w-1), append(wrap(a.Info, styleMuted, w-1), "", styleAccent.Render("enter: open top tracks"))...))
}

func (m Model) searchFooter() string {
	return styleMuted.Render(fit(" type to search · ↑↓/tab section · enter play/open · esc close", m.w))
}
