package ui

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
)

// coverMode selects how album art is drawn.
type coverMode int

const (
	coverBlocks coverMode = iota // half-block characters, any truecolor terminal
	coverKitty                   // Kitty graphics protocol with Unicode placeholders
)

// kittyImageID identifies the cover inside the terminal. One fixed ID means a
// new cover simply replaces the previous one. It doubles as the placeholder
// foreground colour (0xC0FFEE), which is how the protocol links cells to it.
const kittyImageID = 0xC0FFEE

// detectCoverMode picks a renderer. SPOTICLI_COVER=blocks|kitty overrides it.
//
// Placeholders are supported by Kitty and Ghostty. WezTerm speaks the Kitty
// protocol but not placeholders, and multiplexers swallow the escapes, so
// those get half-blocks.
func detectCoverMode(getenv func(string) string) coverMode {
	switch getenv("SPOTICLI_COVER") {
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
	var buf bytes.Buffer
	err := ansi.WriteKittyGraphics(&buf, c.img, &kitty.Options{
		Action:           kitty.TransmitAndPut,
		Transmission:     kitty.Direct,
		Format:           kitty.PNG,
		Chunk:            true,
		Quite:            2, // no replies: they would show up as keyboard input
		ID:               kittyImageID,
		VirtualPlacement: true,
		Columns:          cols,
		Rows:             rows,
	})
	if err != nil {
		return nil // caller falls back to half-blocks
	}

	// Cells are addressed by row/column diacritics; the colour carries the ID.
	fg := fmt.Sprintf("\x1b[38;2;%d;%d;%dm", kittyImageID>>16&0xff, kittyImageID>>8&0xff, kittyImageID&0xff)
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
	if detectCoverMode(getenv) == coverKitty {
		io.WriteString(w, ansi.KittyGraphics(nil, "a=d", "d=I", fmt.Sprintf("i=%d", kittyImageID), "q=2"))
	}
}
