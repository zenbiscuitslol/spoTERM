package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/zenbiscuitslol/spotiCLI/internal/mock"
	"github.com/zenbiscuitslol/spotiCLI/internal/spotify"
	"github.com/zenbiscuitslol/spotiCLI/internal/ui"
)

const setupHelp = `SPOTIFY_CLIENT_ID is not set.

One-time setup:
  1. Create an app at https://developer.spotify.com/dashboard
  2. Add this Redirect URI to it: ` + spotify.RedirectURL + `
  3. Export its Client ID (no secret needed):
       export SPOTIFY_CLIENT_ID=your_client_id
  4. Run spotici again; a browser window opens for the one-time login.

Try the UI without an account: spotici --demo
`

func main() {
	demo := flag.Bool("demo", false, "use built-in sample data instead of Spotify")
	logout := flag.Bool("logout", false, "forget the cached Spotify login and exit")
	flag.Parse()

	if *logout {
		if err := spotify.Logout(); err != nil {
			fatal(err)
		}
		fmt.Println("Logged out.")
		return
	}

	var be ui.Backend
	if *demo {
		be = mock.New()
	} else {
		id := os.Getenv("SPOTIFY_CLIENT_ID")
		if id == "" {
			fmt.Fprint(os.Stderr, setupHelp)
			os.Exit(1)
		}
		c, err := spotify.Connect(context.Background(), id)
		if err != nil {
			fatal(err)
		}
		be = c
	}

	p := tea.NewProgram(ui.New(be), tea.WithAltScreen())
	_, err := p.Run()
	ui.Cleanup(os.Stdout, os.Getenv) // drop the cover image from the terminal
	if err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
