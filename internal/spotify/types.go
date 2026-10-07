// Package spotify is a small client for the parts of the Spotify Web API that
// spotiCLI needs: authentication, playlists, tracks and playback control.
//
// It talks to the API directly instead of using a wrapper library because
// Spotify's February 2026 changes removed endpoints such as
// GET /playlists/{id}/tracks (now /items) that wrappers still call.
package spotify

import "time"

type Playlist struct {
	ID     string
	Name   string
	URI    string
	Tracks int
	Liked  bool // the synthetic "Liked Songs" entry (/me/tracks)

	// Locked marks a playlist whose tracks Spotify probably won't give us:
	// followed, not owned or collaborative. It starts as a guess from the
	// owner and is corrected by what loading the tracks really does.
	Locked bool

	// Set for saved albums, which share the playlist flow (open, list tracks,
	// play via context URI).
	Album    bool
	Artist   string
	Year     string
	ImageURL string // album art, applied to tracks because album listings omit it
}

type Track struct {
	URI      string
	Title    string
	Artist   string
	Album    string
	Duration time.Duration
	ImageURL string
}

// State is a snapshot of the user's current playback.
type State struct {
	Playing    bool
	Track      Track
	Progress   time.Duration
	Volume     int
	Shuffle    bool
	Repeat     string // "off", "track" or "context"
	DeviceName string
}
