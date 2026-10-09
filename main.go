package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/zenbiscuitslol/spoTERM/internal/mock"
	"github.com/zenbiscuitslol/spoTERM/internal/player"
	"github.com/zenbiscuitslol/spoTERM/internal/spotify"
	"github.com/zenbiscuitslol/spoTERM/internal/ui"
)

// deviceName is how spoTERM's built-in player shows up in Spotify.
const deviceName = "spoTERM"

// srcDir is the checkout this binary was built from; install.sh sets it via -ldflags.
var srcDir string

const setupHelp = `SPOTIFY_CLIENT_ID is not set.

One-time setup:
  1. Create an app at https://developer.spotify.com/dashboard
  2. Add this Redirect URI to it: ` + spotify.RedirectURL + `
  3. Export its Client ID (no secret needed):
       export SPOTIFY_CLIENT_ID=your_client_id
  4. Run spoTERM again; a browser window opens for the one-time login.

Try the UI without an account: spoTERM --demo
`

func main() {
	demo := flag.Bool("demo", false, "use built-in sample data instead of Spotify")
	logout := flag.Bool("logout", false, "forget the cached Spotify login and exit")
	noPlayer := flag.Bool("no-player", false, "don't start the built-in player; control another Spotify device instead")
	update := flag.Bool("update", false, "pull the latest changes and rebuild spoTERM")
	flag.Parse()

	if *update {
		if err := selfUpdate(); err != nil {
			fatal(err)
		}
		return
	}

	if *logout {
		if err := spotify.Logout(); err != nil {
			fatal(err)
		}
		fmt.Println("Logged out.")
		return
	}

	var (
		be   ui.Backend
		opts []ui.Option
		mgr  *player.Manager
	)
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

		if !*noPlayer {
			var err error
			if mgr, err = player.New(deviceName); err == nil {
				c.Device = deviceName
				opts = append(opts, ui.WithPlayer(func(ctx context.Context) error {
					if err := mgr.Start(); err != nil {
						return err
					}
					return c.WaitForDevice(ctx, deviceName)
				}))
			} else if errors.Is(err, player.ErrNotInstalled) {
				// Still useful against another Spotify device.
				opts = append(opts, ui.WithNotice("go-librespot not installed: using other Spotify devices (see README)"))
			} else {
				fatal(err)
			}
		}
	}

	ui.LoadTheme()
	p := tea.NewProgram(ui.New(be, opts...), tea.WithAltScreen())
	_, err := p.Run()
	if mgr != nil {
		mgr.Stop() // don't leave the player running after the TUI is gone
	}
	ui.Cleanup(os.Stdout, os.Getenv) // drop the cover image from the terminal
	if err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

// selfUpdate pulls srcDir and rebuilds over the running binary.
func selfUpdate() error {
	if srcDir == "" {
		return errors.New("not built with install.sh, so the source directory is unknown; run ./install.sh from the repo")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	head := func() string {
		out, _ := exec.Command("git", "-C", srcDir, "rev-parse", "HEAD").Output()
		return string(out)
	}
	before := head()
	if err := run("git", "-C", srcDir, "pull", "--ff-only"); err != nil {
		return err
	}
	if before != "" && before == head() {
		return nil // git already said "Already up to date."
	}
	tmp := filepath.Join(filepath.Dir(exe), ".spoTERM.new")
	if err := run("go", "build", "-C", srcDir, "-ldflags", "-X main.srcDir="+srcDir, "-o", tmp, "."); err != nil {
		return err
	}
	if err := os.Rename(tmp, exe); err != nil {
		return err
	}
	fmt.Println("Updated.")
	return nil
}
