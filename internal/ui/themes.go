package ui

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// theme is a color scheme. All are designed for dark terminal backgrounds.
type theme struct {
	name                    string
	accent, text, muted     string
	border, selBg           string
	vizLow, vizMid, vizHigh string // visualizer gradient; vizHigh doubles as the error color
	hi                      string // text color of the selected row
}

var themes = []theme{
	{"Spotify", "#1DB954", "#E6E6E6", "#8A8A8A", "#3A3A3A", "#262626", "#1DB954", "#E8C547", "#E8475F", "#1DB954"},
	{"Dracula", "#BD93F9", "#F8F8F2", "#8F9BC4", "#44475A", "#343746", "#50FA7B", "#F1FA8C", "#FF5555", "#50FA7B"},
	{"Nord", "#88C0D0", "#ECEFF4", "#8F9BB3", "#4C566A", "#3B4252", "#A3BE8C", "#EBCB8B", "#BF616A", "#EBCB8B"},
	{"Gruvbox", "#FABD2F", "#EBDBB2", "#A89984", "#504945", "#3C3836", "#B8BB26", "#FABD2F", "#FB4934", "#FE8019"},
	{"Tokyo Night", "#7AA2F7", "#C0CAF5", "#8089B3", "#3B4261", "#292E42", "#9ECE6A", "#E0AF68", "#F7768E", "#BB9AF7"},
	{"Catppuccin", "#CBA6F7", "#CDD6F4", "#9399B2", "#45475A", "#313244", "#A6E3A1", "#F9E2AF", "#F38BA8", "#F5C2E7"},
	{"Solarized", "#2AA198", "#EEE8D5", "#93A1A1", "#586E75", "#073642", "#859900", "#B58900", "#DC322F", "#DCAA1E"},
	{"Rosé Pine", "#EBBCBA", "#E0DEF4", "#908CAA", "#403D52", "#26233A", "#9CCFD8", "#F6C177", "#EB6F92", "#C4A7E7"},
	{"Synthwave", "#FF7EDB", "#F4EEFF", "#A596D6", "#4B3F72", "#2D2550", "#36F9F6", "#FEDE5D", "#FE4450", "#36F9F6"},
	{"Cyberpunk", "#FCEE0A", "#E8F4FF", "#8A9BB8", "#3D4466", "#1F2438", "#00F0FF", "#FCEE0A", "#FF2A6D", "#00F0FF"},
	{"Mono", "#FFFFFF", "#DDDDDD", "#8C8C8C", "#444444", "#2E2E2E", "#8C8C8C", "#C0C0C0", "#FFFFFF", "#FFFFFF"},
}

var themeIdx int

// setTheme makes themes[i] the active scheme (the col*/style* package vars).
func setTheme(i int) {
	themeIdx = (i%len(themes) + len(themes)) % len(themes)
	t := themes[themeIdx]
	colGreen, colText, colMuted = lipgloss.Color(t.accent), lipgloss.Color(t.text), lipgloss.Color(t.muted)
	colHi = lipgloss.Color(t.hi)
	colBorder, colSelBg = lipgloss.Color(t.border), lipgloss.Color(t.selBg)
	colVizLow, colVizMid, colVizHigh = t.vizLow, t.vizMid, t.vizHigh

	styleText = lipgloss.NewStyle().Foreground(colText)
	styleMuted = lipgloss.NewStyle().Foreground(colMuted)
	styleAccent = lipgloss.NewStyle().Foreground(colGreen)
	styleBold = lipgloss.NewStyle().Foreground(colText).Bold(true)
	styleErr = lipgloss.NewStyle().Foreground(lipgloss.Color(colVizHigh))
}

func themeFile() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "spoTERM", "theme")
}

// LoadTheme applies the theme saved by a previous run, if any.
func LoadTheme() {
	b, _ := os.ReadFile(themeFile())
	name := strings.TrimSpace(string(b))
	for i, t := range themes {
		if t.name == name {
			setTheme(i)
		}
	}
}

// nextTheme switches to the next theme and remembers it (best effort).
func nextTheme() string {
	setTheme(themeIdx + 1)
	if f := themeFile(); f != "" {
		if os.MkdirAll(filepath.Dir(f), 0o700) == nil {
			_ = os.WriteFile(f, []byte(themes[themeIdx].name), 0o600)
		}
	}
	return themes[themeIdx].name
}
