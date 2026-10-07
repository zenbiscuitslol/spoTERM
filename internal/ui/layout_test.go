package ui

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"github.com/zenbiscuitslol/spotiCLI/internal/mock"
	"github.com/zenbiscuitslol/spotiCLI/internal/spotify"
)

// screenRow plays one line of output onto a row of cells the way a terminal
// that counts each syllable cluster as one cell (Kitty does this for Tamil and
// other Indic scripts) would: printable text advances by its own width, and
// "CSI n G" jumps to an absolute column. It returns the cell contents by column.
func screenRow(line string) map[int]string {
	cells := map[int]string{}
	x := 0
	rs := []rune(line)
	for i := 0; i < len(rs); {
		r := rs[i]
		switch {
		case r == 0x1b && i+1 < len(rs) && rs[i+1] == '[': // CSI
			j := i + 2
			for j < len(rs) && (rs[j] < 0x40 || rs[j] > 0x7e) {
				j++
			}
			if j < len(rs) && rs[j] == 'G' {
				n, _ := strconv.Atoi(string(rs[i+2 : j]))
				x = n - 1
			}
			i = j + 1
		case r == 0x1b && i+1 < len(rs) && rs[i+1] == '_': // APC (kitty graphics)
			j := i + 2
			for j < len(rs) && !(rs[j] == 0x1b && j+1 < len(rs) && rs[j+1] == '\\') {
				j++
			}
			i = j + 2
		default:
			// A cluster is a base rune plus its combining marks.
			j := i + 1
			for j < len(rs) && unicode.Is(unicode.M, rs[j]) {
				j++
			}
			cells[x] = string(rs[i:j])
			x += ansi.StringWidth(string(rs[i]))
			i = j
		}
	}
	return cells
}

// tamilBackend adds a playlist whose name terminals and ansi.StringWidth
// measure differently.
type tamilBackend struct{ *mock.Backend }

func (b tamilBackend) Playlists(ctx context.Context) ([]spotify.Playlist, error) {
	pl, err := b.Backend.Playlists(ctx)
	pl[3].Name = "★ தமிழ் ★ மி"
	return pl, err
}

// withColor turns colour output on for a test. Without it lipgloss emits no
// styling at all, which hides bugs in how styles and escapes interact.
func withColor(t *testing.T) {
	t.Helper()
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })
}

func TestWideNamesCannotShiftTheRestOfTheRow(t *testing.T) {
	withColor(t)
	// Sanity: the terminal model and ansi.StringWidth really disagree on this
	// name, otherwise the test proves nothing.
	name := "★ தமிழ் ★ மி"
	termW := 0
	for x := range screenRow(name) {
		termW = max(termW, x+1)
	}
	if ansi.StringWidth(name) == termW {
		t.Fatalf("name should be measured differently (both say %d)", termW)
	}

	for _, sz := range [][2]int{{80, 24}, {110, 32}, {159, 45}} {
		m := New(tamilBackend{mock.New()})
		var tm tea.Model = m
		tm, _ = tm.Update(tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
		tm = settle(tm, m.Init())
		tm = press(tm, "tab", "enter") // play, so the cover and visualizer are drawn
		for i := 0; i < 5; i++ {
			tm, _ = tm.Update(tickMsg{})
		}

		d := tm.(Model).dims()
		lines := strings.Split(tm.View(), "\n")
		for i, line := range lines[:d.bodyH] {
			row := screenRow(line)
			// Nothing may be written past the right edge: the terminal would
			// wrap it onto the next row.
			for x, c := range row {
				if x >= sz[0] {
					t.Errorf("%dx%d row %d: %q written at col %d, past the %d-col edge", sz[0], sz[1], i, c, x, sz[0])
					break
				}
			}
			// Sidebar's right border and the main column's left border.
			if got := row[d.sideW-1]; !strings.ContainsAny(got, "│╮╯") {
				t.Errorf("%dx%d row %d: sidebar border at col %d is %q", sz[0], sz[1], i, d.sideW-1, got)
			}
			if got := row[d.sideW]; !strings.ContainsAny(got, "│╭╰") {
				t.Errorf("%dx%d row %d: main panel starts with %q, want a border at col %d", sz[0], sz[1], i, got, d.sideW)
			}
			if got := row[sz[0]-1]; !strings.ContainsAny(got, "│╮╯") {
				t.Errorf("%dx%d row %d: right border at col %d is %q", sz[0], sz[1], i, sz[0]-1, got)
			}
		}
	}
}

func TestAnchoringDoesNotChangeVisibleLayout(t *testing.T) {
	tm := boot(110, 32)
	v := tm.View()
	if strings.Contains(ansi.Strip(v), "\x1b") {
		t.Fatal("anchors should be invisible escapes")
	}
	for i, l := range strings.Split(v, "\n") {
		if w := ansi.StringWidth(l); w > 110 {
			t.Errorf("line %d is %d wide", i, w)
		}
	}
}

func TestBorderMarksBecomeAnchors(t *testing.T) {
	out := anchorBorders("│ab"+borderMark+"│\n│cd"+borderMark+"│", 9)
	if strings.Contains(out, borderMark) || strings.Count(out, at(9)) != 2 {
		t.Errorf("marks not replaced: %q", out)
	}
	if ansi.StringWidth("│ab"+borderMark+"│") != 4 {
		t.Error("the mark must be zero-width")
	}
}

func TestSpreadAtMatchesSpreadVisibly(t *testing.T) {
	for _, w := range []int{6, 12, 30} {
		a, b := spread("hello world", "42 ", w), spreadAt("hello world", "42 ", w, 100)
		if ansi.Strip(a) != ansi.Strip(b) {
			t.Errorf("w=%d: %q vs %q", w, a, b)
		}
	}
}

// Escape sequences must reach the terminal intact. If a style splits one, the
// pieces show up as text ("[41G"), widening the row until it wraps and scrolls
// the screen.
var escFragment = regexp.MustCompile(`\[[0-9;]*[A-Za-z]`)

func TestNoEscapeFragmentsLeakIntoText(t *testing.T) {
	withColor(t)
	for _, sz := range [][2]int{{60, 19}, {80, 24}, {120, 36}, {170, 45}} {
		for _, keys := range [][]string{{}, {"j", "j"}, {"tab", "j"}, {"2"}, {"tab", "enter"}} {
			tm := boot(sz[0], sz[1])
			tm = press(tm, keys...)
			for i := 0; i < 3; i++ {
				tm, _ = tm.Update(tickMsg{})
			}
			for i, line := range strings.Split(tm.View(), "\n") {
				plain := ansi.Strip(line)
				if f := escFragment.FindString(plain); f != "" {
					t.Errorf("%dx%d keys=%v row %d: escape fragment %q leaked into the text: %.60q", sz[0], sz[1], keys, i, f, plain)
				}
				if w := ansi.StringWidth(line); w > sz[0] {
					t.Errorf("%dx%d keys=%v row %d: %d cells wide, would wrap", sz[0], sz[1], keys, i, w)
				}
			}
		}
	}
}
