// Package mock is an in-memory stand-in for the Spotify client. It powers
// `spotici --demo` and the UI tests, so the app can run without credentials.
package mock

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"math"
	"sync"
	"time"

	"github.com/zenbiscuitslol/spotiCLI/internal/spotify"
)

const coverURL = "mock://cover"

var playlistData = []struct {
	name   string
	tracks int
}{
	{"Liked Songs", 312}, {"Discover Weekly", 30}, {"Release Radar", 24},
	{"Daily Mix 1", 50}, {"Daily Mix 2", 50}, {"Late Night Drive", 87},
	{"Focus Flow", 120}, {"Gym Hype", 64}, {"Chill Lo-fi", 203},
	{"Road Trip", 95}, {"Throwback 2000s", 140}, {"Indie Mix", 58},
	{"Jazz Classics", 76}, {"Study Beats", 160}, {"Sunday Morning", 41},
	{"Acoustic Evenings", 33},
}

// Backend implements ui.Backend with canned data and a simulated player.
type Backend struct {
	mu      sync.Mutex
	queue   []spotify.Track
	idx     int
	active  bool
	playing bool
	base    time.Duration // progress at `since`
	since   time.Time
}

func New() *Backend { return &Backend{} }

func (b *Backend) Playlists(context.Context) ([]spotify.Playlist, error) {
	out := make([]spotify.Playlist, len(playlistData))
	for i, p := range playlistData {
		out[i] = spotify.Playlist{
			ID: fmt.Sprintf("mock:%d", i), URI: fmt.Sprintf("mock:%d", i),
			Name: p.name, Tracks: p.tracks, Liked: i == 0,
			// 14: followed but readable (a wrong guess); 15: really forbidden.
			Locked: i == 14 || i == 15,
		}
	}
	return out, nil
}

// Albums derives saved albums from the sample tracks.
func (b *Backend) Albums(context.Context) ([]spotify.Playlist, error) {
	var out []spotify.Playlist
	seen := map[string]int{}
	for _, t := range Tracks() {
		if i, ok := seen[t.Album]; ok {
			out[i].Tracks++
			continue
		}
		seen[t.Album] = len(out)
		out = append(out, spotify.Playlist{
			ID: "mock:album:" + t.Album, URI: "mock:album:" + t.Album, Name: t.Album,
			Tracks: 1, Album: true, Artist: t.Artist, Year: "2024", ImageURL: coverURL,
		})
	}
	return out, nil
}

func (b *Backend) Tracks(_ context.Context, pl spotify.Playlist) ([]spotify.Track, error) {
	if pl.ID == "mock:15" {
		return nil, spotify.ErrPlaylistForbidden
	}
	if !pl.Album {
		return Tracks(), nil
	}
	var out []spotify.Track
	for _, t := range Tracks() {
		if t.Album == pl.Name {
			out = append(out, t)
		}
	}
	return out, nil
}

func (b *Backend) progress() time.Duration {
	if b.playing {
		return b.base + time.Since(b.since)
	}
	return b.base
}

func (b *Backend) Playback(context.Context) (*spotify.State, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.active {
		return nil, nil
	}
	return &spotify.State{
		Playing: b.playing, Track: b.queue[b.idx], Progress: b.progress(),
		Volume: 70, Repeat: "off", DeviceName: "Demo device",
	}, nil
}

func (b *Backend) Play(_ context.Context, _ spotify.Playlist, tracks []spotify.Track, idx int) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.queue, b.idx, b.active, b.playing = tracks, idx, true, true
	b.base, b.since = 0, time.Now()
	return nil
}

func (b *Backend) Resume(context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.active && !b.playing {
		b.playing, b.since = true, time.Now()
	}
	return nil
}

func (b *Backend) Pause(context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.playing {
		b.base, b.playing = b.progress(), false
	}
	return nil
}

func (b *Backend) skip(delta int) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.active {
		b.idx = (b.idx + delta + len(b.queue)) % len(b.queue)
		b.base, b.since = 0, time.Now()
	}
	return nil
}

func (b *Backend) Next(context.Context) error     { return b.skip(1) }
func (b *Backend) Previous(context.Context) error { return b.skip(-1) }

func (b *Backend) Image(context.Context, string) (image.Image, error) { return Cover(), nil }

// Tracks returns sample tracks.
func Tracks() []spotify.Track {
	m := func(min, sec int) time.Duration {
		return time.Duration(min)*time.Minute + time.Duration(sec)*time.Second
	}
	return []spotify.Track{
		{URI: "mock:track:1", Title: "Neon Horizons", Artist: "The Midnight Cartel", Album: "Afterglow", Duration: m(3, 42), ImageURL: coverURL},
		{URI: "mock:track:2", Title: "Paper Planes in July", Artist: "Wren & the Tides", Album: "Small Hours", Duration: m(4, 8), ImageURL: coverURL},
		{URI: "mock:track:3", Title: "Static Bloom", Artist: "Halcyon Arcade", Album: "Static Bloom", Duration: m(3, 15), ImageURL: coverURL},
		{URI: "mock:track:4", Title: "Run Slow", Artist: "Marlowe Park", Album: "Run Slow", Duration: m(2, 58), ImageURL: coverURL},
		{URI: "mock:track:5", Title: "Glass Cathedral", Artist: "Ember Choir", Album: "Vespers", Duration: m(5, 21), ImageURL: coverURL},
		{URI: "mock:track:6", Title: "Honey Overdrive", Artist: "Velvet Static", Album: "Honey Overdrive", Duration: m(3, 33), ImageURL: coverURL},
		{URI: "mock:track:7", Title: "Low Tide Letters", Artist: "Saoirse Vale", Album: "Low Tide", Duration: m(4, 47), ImageURL: coverURL},
		{URI: "mock:track:8", Title: "Concrete Garden", Artist: "North of Nowhere", Album: "Concrete Garden", Duration: m(3, 5), ImageURL: coverURL},
		{URI: "mock:track:9", Title: "Satellites", Artist: "Juno Lark", Album: "Orbit", Duration: m(3, 51), ImageURL: coverURL},
		{URI: "mock:track:10", Title: "Fever Dream Motel", Artist: "The Paper Kites Club", Album: "Vacancy", Duration: m(4, 12), ImageURL: coverURL},
		{URI: "mock:track:11", Title: "Slow Burn", Artist: "Marlowe Park", Album: "Run Slow", Duration: m(3, 27), ImageURL: coverURL},
		{URI: "mock:track:12", Title: "Moonlit Arcade", Artist: "Halcyon Arcade", Album: "Static Bloom", Duration: m(2, 49), ImageURL: coverURL},
		{URI: "mock:track:13", Title: "Wildflower Radio", Artist: "Wren & the Tides", Album: "Small Hours", Duration: m(3, 38), ImageURL: coverURL},
		{URI: "mock:track:14", Title: "Telescope", Artist: "Juno Lark", Album: "Orbit", Duration: m(4, 2), ImageURL: coverURL},
		{URI: "mock:track:15", Title: "Golden Hour Static", Artist: "Velvet Static", Album: "Honey Overdrive", Duration: m(3, 19), ImageURL: coverURL},
		{URI: "mock:track:16", Title: "Undertow", Artist: "Ember Choir", Album: "Vespers", Duration: m(5, 44), ImageURL: coverURL},
		{URI: "mock:track:17", Title: "City of Lanterns", Artist: "North of Nowhere", Album: "Concrete Garden", Duration: m(3, 56), ImageURL: coverURL},
		{URI: "mock:track:18", Title: "Last Train Home", Artist: "Saoirse Vale", Album: "Low Tide", Duration: m(4, 30), ImageURL: coverURL},
		{URI: "mock:track:19", Title: "Daydream Parade", Artist: "The Midnight Cartel", Album: "Afterglow", Duration: m(3, 11), ImageURL: coverURL},
		{URI: "mock:track:20", Title: "Soft Machines", Artist: "Halcyon Arcade", Album: "Static Bloom", Duration: m(4, 6), ImageURL: coverURL},
	}
}

// Cover returns a procedurally generated square "album cover" so the cover
// renderer has something to draw without a network request.
func Cover() image.Image {
	const size = 256
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	lerp := func(a, b, t float64) float64 { return a + (b-a)*t }
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			fx, fy := float64(x)/size, float64(y)/size
			t := (fx*0.35 + fy*0.65)
			// Dusk gradient: deep indigo -> magenta -> amber.
			var r, g, b float64
			if t < 0.55 {
				u := t / 0.55
				r, g, b = lerp(28, 190, u), lerp(18, 52, u), lerp(84, 130, u)
			} else {
				u := (t - 0.55) / 0.45
				r, g, b = lerp(190, 255, u), lerp(52, 168, u), lerp(130, 70, u)
			}
			// Sun disc.
			d := math.Hypot(fx-0.5, fy-0.42)
			if d < 0.2 {
				r, g, b = 255, lerp(214, 150, d/0.2), lerp(120, 70, d/0.2)
			}
			// Horizon stripes cutting into the lower sun / ground.
			if fy > 0.55 && fy < 0.75 && d < 0.2 {
				if int((fy-0.55)*60)%2 == 0 {
					r, g, b = 28, 18, 84
				}
			}
			if fy >= 0.75 {
				r, g, b = lerp(20, 40, fy), lerp(12, 20, fy), lerp(60, 90, fy)
			}
			img.Set(x, y, color.RGBA{uint8(r), uint8(g), uint8(b), 255})
		}
	}
	return img
}
