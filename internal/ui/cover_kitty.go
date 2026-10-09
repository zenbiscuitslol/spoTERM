package ui

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync/atomic"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
)

// coverMode selects how album art is drawn.
type coverMode int

const (
	coverBlocks coverMode = iota // half-block characters, any truecolor terminal
	coverKitty                   // Kitty graphics protocol with Unicode placeholders
)

// Every transmit gets a fresh image ID, which doubles as the placeholder
// foreground colour (24 bits), the way the protocol links cells to an image.
// Reusing one ID would leave the cell text unchanged when the cover changes;
// Bubble Tea only rewrites changed rows, and the rows it skips kept showing
// pieces of the previous cover (a band of the old art across the new one). With
// a new ID every cover row changes, so every row is redrawn.
const kittyIDBase = 0xC0FFEE

var kittyIDSeq, kittyLastID atomic.Uint32

func newKittyID() int {
	id := kittyIDBase + int(kittyIDSeq.Add(1)%0xFFFF)
	kittyLastID.Store(uint32(id))
	return id
}

// detectCoverMode picks a renderer. SPOTERM_COVER=blocks|kitty overrides it.
//
// Placeholders are supported by Kitty and Ghostty. WezTerm speaks the Kitty
// protocol but not placeholders, and multiplexers swallow the escapes, so
// those get half-blocks.
func detectCoverMode(getenv func(string) string) coverMode {
	switch getenv("SPOTERM_COVER") {
	case "blocks":
		return coverBlocks
	case "kitty":
		return coverKitty
	}
	if getenv("TMUX") != "" || getenv("STY") != "" {
		return coverBlocks
	}
	term, prog := getenv("TERM"), getenv("TERM_PROGRAM")
	if getenv("KITTY_WINDOW_ID") != "" || term == "xterm-kitty" ||
		prog == "ghostty" || term == "xterm-ghostty" {
		return coverKitty
	}
	return coverBlocks
}

// renderKitty transmits the image once (as a virtual placement sized
// cols x rows cells) and returns rows of placeholder cells that the terminal
// replaces with the matching slice of the image. The transmit sequence rides
// at the start of the first row: it is zero-width, and Bubble Tea only
// rewrites a line when it changes, so the image is re-sent only when the
// cover or its size changes.
func (c *coverRenderer) renderKitty(cols, rows int) []string {
	id := newKittyID()
	var buf bytes.Buffer
	// Free the previous image (a no-op if it is already gone).
	if c.kittyID != 0 {
		buf.WriteString(ansi.KittyGraphics(nil, "a=d", "d=I", fmt.Sprintf("i=%d", c.kittyID), "q=2"))
	}
	err := ansi.WriteKittyGraphics(&buf, c.img, &kitty.Options{
		Action:           kitty.TransmitAndPut,
		Transmission:     kitty.Direct,
		Format:           kitty.PNG,
		Chunk:            true,
		Quite:            2, // no replies: they would show up as keyboard input
		ID:               id,
		VirtualPlacement: true,
		DoNotMoveCursor:  true, // as kitten icat does for placeholders
		Columns:          cols,
		Rows:             rows,
	})
	if err != nil {
		return nil // caller falls back to half-blocks
	}
	c.kittyID = id

	// Cells are addressed by row/column diacritics; the colour carries the ID.
	fg := fmt.Sprintf("\x1b[38;2;%d;%d;%dm", id>>16&0xff, id>>8&0xff, id&0xff)
	out := make([]string, rows)
	for row := range out {
		var sb strings.Builder
		if row == 0 {
			sb.Write(buf.Bytes())
		}
		sb.WriteString(fg)
		for col := 0; col < cols; col++ {
			sb.WriteRune(kitty.Placeholder)
			sb.WriteRune(kitty.Diacritic(row))
			sb.WriteRune(kitty.Diacritic(col))
		}
		sb.WriteString("\x1b[39m")
		out[row] = sb.String()
	}
	return out
}

// Cleanup removes the cover from the terminal. Call it after the program exits.
func Cleanup(w io.Writer, getenv func(string) string) {
	if id := kittyLastID.Load(); id != 0 && detectCoverMode(getenv) == coverKitty {
		io.WriteString(w, ansi.KittyGraphics(nil, "a=d", "d=I", fmt.Sprintf("i=%d", id), "q=2"))
	}
}
