package ui

import (
	"context"
	"image"

	"github.com/zenbiscuitslol/spotiCLI/internal/mock"
	"github.com/zenbiscuitslol/spotiCLI/internal/spotify"
)

// Backend is everything the UI needs from Spotify. *spotify.Client is the
// real implementation and *mock.Backend a fake for --demo and tests.
type Backend interface {
	Playlists(ctx context.Context) ([]spotify.Playlist, error)
	// Albums returns saved albums as playlist-shaped entries (Album set);
	// Tracks and Play accept them like any playlist.
	Albums(ctx context.Context) ([]spotify.Playlist, error)
	Tracks(ctx context.Context, pl spotify.Playlist) ([]spotify.Track, error)
	// Playback returns nil, nil when nothing is playing on any device.
	Playback(ctx context.Context) (*spotify.State, error)
	Play(ctx context.Context, pl spotify.Playlist, tracks []spotify.Track, idx int) error
	Resume(ctx context.Context) error
	Pause(ctx context.Context) error
	Next(ctx context.Context) error
	Previous(ctx context.Context) error
	Image(ctx context.Context, url string) (image.Image, error)
}

var (
	_ Backend = (*spotify.Client)(nil)
	_ Backend = (*mock.Backend)(nil)
)
