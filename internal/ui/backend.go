package ui

import (
	"context"
	"image"

	"github.com/zenbiscuitslol/spoTERM/internal/mock"
	"github.com/zenbiscuitslol/spoTERM/internal/spotify"
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
	SetVolume(ctx context.Context, pct int) error
	SetShuffle(ctx context.Context, on bool) error
	AddToQueue(ctx context.Context, uri string) error
	// Queue returns the upcoming tracks in play order.
	Queue(ctx context.Context) ([]spotify.Track, error)
	// Search finds tracks, albums and artists matching query.
	Search(ctx context.Context, query string) (spotify.SearchResults, error)
	Image(ctx context.Context, url string) (image.Image, error)
}

var (
	_ Backend = (*spotify.Client)(nil)
	_ Backend = (*mock.Backend)(nil)
)
