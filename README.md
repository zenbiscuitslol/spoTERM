# spoTERM

A Spotify client for your terminal, written in Go with [Bubble Tea](https://github.com/charmbracelet/bubbletea).
Playlists, Liked Songs and saved albums on the left, tracks in the middle, and a
now-playing bar with album art and an audio visualizer at the bottom.

## Requirements

- Go 1.26+
- Spotify **Premium** (needed for any playback control)
- A truecolor terminal, at least 60×19
- Optional: [go-librespot](https://github.com/devgianlu/go-librespot) for the built-in player (see below)

## Install

1. Create an app at <https://developer.spotify.com/dashboard> and add
   `http://127.0.0.1:8888/callback` to its **Redirect URIs**.
2. Export its Client ID (no client secret needed, spoTERM uses PKCE):
   ```sh
   export SPOTIFY_CLIENT_ID=<your client id>
   ```
3. Build and install:
   ```sh
   git clone https://github.com/zenbiscuitslol/spoTERM && cd spoTERM
   ./install.sh          # builds to ~/.local/bin/spoTERM (keep it on your PATH)
   ```
4. Run `spoTERM`. A browser opens for a one-time login; the token is cached in
   `~/.config/spoTERM/token.json`.

Just want to look around? `spoTERM --demo` runs on sample data, no account needed.

### Flags

| flag | effect |
| --- | --- |
| `--demo` | use built-in sample data instead of Spotify |
| `--no-player` | don't start the built-in player; control another Spotify device |
| `--logout` | forget the cached login and exit |
| `--update` | `git pull` the source checkout and rebuild (needs the `install.sh` build) |

## Controls

| key | action |
| --- | --- |
| `↑`/`↓`, `j`/`k` | move |
| `tab`, `h`/`l` | switch pane |
| `1` / `2`, `[` / `]` | sidebar: playlists (with Liked Songs) / saved albums |
| `enter` | open playlist or album / play track |
| `space` | play / pause |
| `n` / `p` | next / previous |
| `+` / `-` | volume up / down (5%) |
| `s` | toggle shuffle |
| `a` | add the highlighted track to your queue |
| `Q` | show / edit your queue: `↑↓` move, `x` remove, `esc` close |
| `t` | cycle color theme (saved to `~/.config/spoTERM/theme`) |
| `ctrl+s` | search songs, albums and artists |
| `q` | quit |

### Search (`ctrl+s`)

| key | action |
| --- | --- |
| *type* | search (results appear once you pause typing) |
| `tab` / `shift+tab`, `←` / `→` | switch between Songs, Albums and Artists |
| `↑` / `↓` | move; carries on into the next section at the end of a list |
| `enter` | play a song, open an album, or open an artist's top tracks |
| `ctrl+u` | clear the search box |
| `esc` or `ctrl+s` | close search (your query is kept) |

### Queue

Spotify's API can't remove tracks from its own queue, so **your** queue lives in
spoTERM: each track is handed to Spotify about 10 seconds before the current one
ends (or when you press `n`). Handed-over tracks can't be taken back, and your
queue is forgotten on quit. Tracks queued in the Spotify app show as "Next in Spotify".

### Themes

Spotify (default), Dracula, Nord, Gruvbox, Tokyo Night, Catppuccin, Solarized,
Rosé Pine, Synthwave, Cyberpunk, Mono. All are designed for dark terminals.

## Built-in player

Spotify's Web API can only *remote-control* a player. To make sound itself,
spoTERM starts [go-librespot](https://github.com/devgianlu/go-librespot) in the
background; a device called **spoTERM** appears in your Spotify devices, plays
through PulseAudio/PipeWire, and stops when you quit.

1. Download a [go-librespot release](https://github.com/devgianlu/go-librespot/releases/latest)
   and put the binary on your `PATH` (or point `SPOTERM_LIBRESPOT` at it).
2. Run spoTERM. The first run opens a browser so go-librespot can log in; it's cached.

Notes:
- go-librespot is an **unofficial** client and needs Premium. Using it is a grey
  area of Spotify's terms; use your own discretion.
- Its config lives in `~/.config/spoTERM/librespot/` and its log in
  `~/.local/state/spoTERM/librespot.log` (check there if no sound plays).
- Without the binary, spoTERM still works and controls whichever Spotify device is open.

## Album art

Half-block characters in any truecolor terminal; the real image via the Kitty
graphics protocol in Kitty and Ghostty. Inside tmux the half-block fallback is
used. Force one with `SPOTERM_COVER=blocks` or `SPOTERM_COVER=kitty`.

## How it works

```
main.go                 flags, wiring, --update
internal/spotify/       Web API client (direct HTTP, PKCE login, token cache)
internal/player/        runs go-librespot as a child process
internal/ui/            Bubble Tea model + rendering
internal/mock/          fake backend for --demo and tests
```

- **Backend interface** (`internal/ui/backend.go`): the UI only talks to `ui.Backend`.
  `spotify.Client` is the real one, `mock.Backend` the fake, so the whole UI is testable offline.
- **Bubble Tea loop** (`internal/ui/model.go`): a 20 fps tick drives the visualizer
  and progress bar; playback state is polled every ~4s and re-read shortly after
  each command, since Spotify's state lags a beat. All API calls run as `tea.Cmd`s,
  never blocking the UI. Rendering is split by view: `sidebar.go`, `tracks.go`,
  `nowplaying.go`, `search.go`, `cover*.go`, `visualizer.go`, `themes.go`.
- **Why no wrapper library**: Spotify's Feb 2026 API changes removed endpoints that
  wrappers still call, so `internal/spotify/api.go` talks to the API directly.
- **Playback**: Spotify's own queue can't be edited, so spoTERM keeps a local queue
  and pushes the next track to Spotify just before the current one ends.

## Development

```sh
go test ./...
go vet ./...
go run . --demo
```
