package ui

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
	"github.com/zenbiscuitslol/spoTERM/internal/mock"
)

func env(kv map[string]string) func(string) string {
	return func(k string) string { return kv[k] }
}

func TestDetectCoverMode(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  map[string]string
		want coverMode
	}{
		{"plain terminal", map[string]string{"TERM": "xterm-256color"}, coverBlocks},
		{"kitty window", map[string]string{"KITTY_WINDOW_ID": "3"}, coverKitty},
		{"kitty term", map[string]string{"TERM": "xterm-kitty"}, coverKitty},
		{"ghostty", map[string]string{"TERM_PROGRAM": "ghostty"}, coverKitty},
		{"wezterm lacks placeholders", map[string]string{"TERM_PROGRAM": "WezTerm"}, coverBlocks},
		{"kitty inside tmux", map[string]string{"KITTY_WINDOW_ID": "3", "TMUX": "/tmp/tmux"}, coverBlocks},
		{"force blocks", map[string]string{"KITTY_WINDOW_ID": "3", "SPOTERM_COVER": "blocks"}, coverBlocks},
		{"force kitty", map[string]string{"SPOTERM_COVER": "kitty"}, coverKitty},
	} {
		if got := detectCoverMode(env(tc.env)); got != tc.want {
			t.Errorf("%s: got %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestKittyCoverShape(t *testing.T) {
	c := newCoverRenderer(mock.Cover())
	c.mode = coverKitty
	const cols, rows = 16, 8
	out := c.Render(cols, rows)
	if len(out) != rows {
		t.Fatalf("got %d rows, want %d", len(out), rows)
	}

	// The transmit sequence is in the first row only, and addresses a virtual
	// placement of exactly the cell grid we draw.
	first := out[0]
	id := c.kittyID
	for _, want := range []string{"\x1b_G", "a=T", "U=1", "C=1", "i=" + strconv.Itoa(id), "c=16", "r=8", "q=2", "f=100"} {
		if !strings.Contains(first, want) {
			t.Errorf("transmit sequence is missing %q", want)
		}
	}
	for i, row := range out[1:] {
		if strings.Contains(row, "\x1b_G") {
			t.Errorf("row %d should not re-transmit the image", i+1)
		}
	}

	// Every row occupies exactly cols cells and carries one placeholder per cell.
	for i, row := range out {
		if w := ansi.StringWidth(row); w != cols {
			t.Errorf("row %d is %d cells wide, want %d", i, w, cols)
		}
		if n := strings.Count(row, string(kitty.Placeholder)); n != cols {
			t.Errorf("row %d has %d placeholders, want %d", i, n, cols)
		}
		if !strings.Contains(row, fmt.Sprintf("\x1b[38;2;%d;%d;%dm", id>>16&0xff, id>>8&0xff, id&0xff)) {
			t.Errorf("row %d: placeholder colour does not encode the image ID", i)
		}
	}
	if !strings.Contains(out[3], string(kitty.Placeholder)+string(kitty.Diacritic(3))+string(kitty.Diacritic(0))) {
		t.Error("cells should carry row and column diacritics")
	}

	// Same size again must not re-encode (the first row would be re-sent).
	if again := c.Render(cols, rows); &again[0] != &out[0] {
		t.Error("render should be cached")
	}
}

func TestKittyViewFitsTerminal(t *testing.T) {
	withColor(t)
	m := New(mock.New())
	m.cover.mode = coverKitty
	var tm = tmModel(m)
	tm = settle(tm, m.Init())
	tm = press(tm, "tab", "enter") // play, which loads the cover
	for _, sz := range [][2]int{{60, 19}, {110, 32}, {200, 50}} {
		tm, _ = tm.Update(winSize(sz[0], sz[1]))
		lines := strings.Split(tm.View(), "\n")
		if len(lines) != sz[1] {
			t.Errorf("%dx%d: got %d lines", sz[0], sz[1], len(lines))
		}
		for i, l := range lines {
			if w := ansi.StringWidth(l); w > sz[0] {
				t.Errorf("%dx%d: line %d is %d wide", sz[0], sz[1], i, w)
			}
		}
		if !strings.Contains(tm.View(), string(kitty.Placeholder)) {
			t.Errorf("%dx%d: no placeholders rendered", sz[0], sz[1])
		}
	}
}

// A new cover must change the text of every cover row, not just the first:
// Bubble Tea skips rows whose text is unchanged, and those rows would keep
// showing the previous cover.
func TestNewCoverChangesEveryRow(t *testing.T) {
	c := newCoverRenderer(mock.Cover())
	c.mode = coverKitty
	const cols, rows = 16, 8
	before := append([]string(nil), c.Render(cols, rows)...)
	oldID := c.kittyID

	c.SetImage(mock.Cover()) // next track
	after := c.Render(cols, rows)
	if c.kittyID == oldID {
		t.Fatal("a new cover must get a new image ID")
	}
	for i := range after {
		if after[i] == before[i] {
			t.Errorf("row %d is byte-identical to the previous cover's, so it would not be redrawn", i)
		}
	}
	if !strings.Contains(after[0], "d=I") || !strings.Contains(after[0], fmt.Sprintf("i=%d", oldID)) {
		t.Error("the previous image should be deleted")
	}
}

func TestCleanupDeletesTheLastImage(t *testing.T) {
	c := newCoverRenderer(mock.Cover())
	c.mode = coverKitty
	c.Render(8, 4)
	var sb strings.Builder
	Cleanup(&sb, env(map[string]string{"SPOTERM_COVER": "kitty"}))
	if want := fmt.Sprintf("i=%d", c.kittyID); !strings.Contains(sb.String(), want) || !strings.Contains(sb.String(), "d=I") {
		t.Errorf("cleanup %q should delete %s", sb.String(), want)
	}
	sb.Reset()
	Cleanup(&sb, env(map[string]string{"SPOTERM_COVER": "blocks"}))
	if sb.Len() != 0 {
		t.Errorf("nothing to clean up in blocks mode, got %q", sb.String())
	}
}
