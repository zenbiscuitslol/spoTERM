package ui

import (
	"fmt"
	"image"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// coverRenderer draws an image. By default it uses half-block characters: each
// terminal cell shows two vertically stacked pixels (foreground = top,
// background = bottom). This works in any truecolor terminal; Lip Gloss
// downgrades the colors automatically on terminals with fewer colors. Kitty
// and Ghostty get the real image instead (see cover_kitty.go).
//
// Swap the source with SetImage once real album art has been downloaded.
type coverRenderer struct {
	img        image.Image
	mode       coverMode
	kittyID    int // image ID currently held by the terminal (0 = none)
	cache      []string
	cols, rows int
}

func newCoverRenderer(img image.Image) *coverRenderer {
	return &coverRenderer{img: img, mode: detectCoverMode(os.Getenv)}
}

func (c *coverRenderer) SetImage(img image.Image) {
	c.img = img
	c.cache = nil
}

// Render returns cols x rows cells of the image, one string per row.
func (c *coverRenderer) Render(cols, rows int) []string {
	if c.img == nil || cols < 1 || rows < 1 {
		return nil
	}
	if c.cache != nil && c.cols == cols && c.rows == rows {
		return c.cache
	}
	if c.mode == coverKitty {
		if out := c.renderKitty(cols, rows); out != nil {
			c.cache, c.cols, c.rows = out, cols, rows
			return out
		}
	}

	b := c.img.Bounds()
	pw, ph := cols, rows*2
	sample := func(px, py int) lipgloss.Color {
		// Box-average the source pixels that fall inside this output pixel.
		x0 := b.Min.X + px*b.Dx()/pw
		x1 := max(b.Min.X+(px+1)*b.Dx()/pw, x0+1)
		y0 := b.Min.Y + py*b.Dy()/ph
		y1 := max(b.Min.Y+(py+1)*b.Dy()/ph, y0+1)
		var r, g, bl, n uint64
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				cr, cg, cb, _ := c.img.At(x, y).RGBA()
				r, g, bl, n = r+uint64(cr>>8), g+uint64(cg>>8), bl+uint64(cb>>8), n+1
			}
		}
		return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", r/n, g/n, bl/n))
	}

	out := make([]string, rows)
	for row := 0; row < rows; row++ {
		var sb strings.Builder
		for col := 0; col < cols; col++ {
			st := lipgloss.NewStyle().
				Foreground(sample(col, row*2)).
				Background(sample(col, row*2+1))
			sb.WriteString(st.Render("▀"))
		}
		out[row] = sb.String()
	}
	c.cache, c.cols, c.rows = out, cols, rows
	return out
}
