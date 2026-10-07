# spotiCLI

A Spotify TUI written in Go (Bubble Tea). Playlists, Liked Songs and saved albums
on the left (switch with `1`/`2`), tracks in the middle, now-playing with album
art and an audio visualizer at the bottom.

## Setup

1. Create an app at <https://developer.spotify.com/dashboard>.
2. Add `http://127.0.0.1:8888/callback` to its **Redirect URIs**.
3. `export SPOTIFY_CLIENT_ID=<your client id>` (no client secret needed — PKCE).
4. `go run .` — a browser window opens for the one-time login. The token is
   cached in your user config dir (`~/.config/spotiCLI/token.json`).

Playback needs Spotify Premium. Sound comes from the built-in player (below), or
from any other Spotify device you have open if you run with `--no-player`.

`go run . --demo` runs the UI on sample data; `go run . --logout` forgets the login;
`go run . --no-player` skips the built-in player.

## Keys

| key | action |
| --- | --- |
| `↑`/`↓`, `j`/`k` | move |
| `tab`, `h`/`l` | switch pane |
| `1` / `2`, `[` / `]` | sidebar: playlists (with Liked Songs) / saved albums |
| `enter` | open playlist or album / play track |
| `space` | play / pause |
| `n` / `p` | next / previous |
| `t` | cycle color theme |
| `ctrl+s` | search songs, albums and artists |
| `q` | quit |

In search: type to search, `tab` / `shift+tab` switch section (songs, albums, artists), `↑`/`↓` move, `enter` plays a song or opens an album / an artist's top tracks, `esc` closes.

## Album art

Covers are drawn with half-block characters, which works in any truecolor
terminal. In Kitty and Ghostty the real image is shown instead (Kitty graphics
protocol with Unicode placeholders). Inside tmux and in other terminals the
half-block fallback is used. Override the choice with
`SPOTICLI_COVER=blocks` or `SPOTICLI_COVER=kitty`.

## Built-in player

Spotify's Web API can only remote-control a player, so spotiCLI starts
[go-librespot](https://github.com/devgianlu/go-librespot) (a GUI-less Spotify
Connect player) in the background. A device called **spotiCLI** appears in your
Spotify device list, plays through PulseAudio/PipeWire, and is stopped again
when you quit.

1. Download a go-librespot release for your platform from
   <https://github.com/devgianlu/go-librespot/releases/latest> and put the
   binary on your `PATH` (or point `SPOTICLI_LIBRESPOT` at it).
2. Run spotiCLI. On the first run a browser window opens so go-librespot can log
   in to Spotify; the login is cached and not asked for again.

Notes:
- go-librespot is an **unofficial** Spotify client and needs **Premium**. Using
  it is in a grey area of Spotify's terms, so use it at your own discretion.
- Its config and credentials live in `~/.config/spotiCLI/librespot/`, separate
  from any go-librespot setup of your own. Its output goes to
  `~/.local/state/spotiCLI/librespot.log`: check there if no sound plays.
- Without the binary spotiCLI still works, controlling whichever Spotify device
  is open.
