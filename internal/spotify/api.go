package spotify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // register decoders for album art
	_ "image/png"
	"io"
	"maps"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const apiBase = "https://api.spotify.com/v1"

// Client is an authenticated Spotify Web API client. Create one with Connect.
type Client struct {
	http *http.Client // adds and refreshes the OAuth token
	base string       // API root; overridden in tests

	// Device is the name of the preferred playback device (the daemon spoTERM
	// manages). When set, Play starts there instead of on whatever is active.
	Device string
}

// APIError is a non-2xx response from the Web API.
type APIError struct {
	Status     int
	Message    string
	Reason     string // e.g. NO_ACTIVE_DEVICE
	RetryAfter time.Duration
}

func (e *APIError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = http.StatusText(e.Status)
	}
	if e.Status == http.StatusTooManyRequests && e.RetryAfter > 0 {
		return fmt.Sprintf("rate limited by Spotify, retry in %ds", int(e.RetryAfter.Seconds()))
	}
	return fmt.Sprintf("spotify: %s (%d)", msg, e.Status)
}

// do performs a request. target may be a path ("/me/player") or a full URL
// (the "next" link of a paged response). It returns the HTTP status; a 204
// leaves out untouched.
func (c *Client) do(ctx context.Context, method, target string, query url.Values, body, out any) (int, error) {
	if !strings.HasPrefix(target, "http") {
		target = c.base + target
		if len(query) > 0 {
			target += "?" + query.Encode()
		}
	}

	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, rdr)
	if err != nil {
		return 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		var eb struct {
			Error struct {
				Message string `json:"message"`
				Reason  string `json:"reason"`
			} `json:"error"`
		}
		_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&eb)
		ae := &APIError{Status: resp.StatusCode, Message: eb.Error.Message, Reason: eb.Error.Reason}
		if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil {
			ae.RetryAfter = time.Duration(s) * time.Second
		}
		return resp.StatusCode, ae
	}
	if resp.StatusCode == http.StatusNoContent || out == nil {
		return resp.StatusCode, nil
	}
	return resp.StatusCode, json.NewDecoder(resp.Body).Decode(out)
}

type page[T any] struct {
	Items []T    `json:"items"`
	Next  string `json:"next"`
}

// fetchAll follows "next" links until there are none or limit items are held.
func fetchAll[T any](ctx context.Context, c *Client, path string, q url.Values, limit int) ([]T, error) {
	var all []T
	next := path
	for next != "" && len(all) < limit {
		var p page[T]
		if _, err := c.do(ctx, http.MethodGet, next, q, nil, &p); err != nil {
			return all, err
		}
		all = append(all, p.Items...)
		next, q = p.Next, nil
	}
	return all, nil
}

// --- JSON shapes -----------------------------------------------------------

type apiTrack struct {
	URI        string `json:"uri"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	IsLocal    bool   `json:"is_local"`
	DurationMs int    `json:"duration_ms"`
	Artists    []struct {
		Name string `json:"name"`
	} `json:"artists"`
	Album struct {
		Name   string     `json:"name"`
		Images []apiImage `json:"images"`
	} `json:"album"`
}

type apiImage struct {
	URL   string `json:"url"`
	Width int    `json:"width"`
}

// pickImage takes the smallest image that's still >= 300px. Images come
// widest-first.
func pickImage(images []apiImage) string {
	img := ""
	for _, i := range images {
		if img == "" || i.Width >= 300 {
			img = i.URL
		}
	}
	return img
}

// toTrack converts to a Track; ok is false for items we can't play (nulls,
// local files, podcast episodes).
func (t *apiTrack) toTrack() (Track, bool) {
	if t == nil || t.URI == "" || t.IsLocal || (t.Type != "" && t.Type != "track") {
		return Track{}, false
	}
	names := make([]string, len(t.Artists))
	for i, a := range t.Artists {
		names[i] = a.Name
	}
	return Track{
		URI:      t.URI,
		Title:    t.Name,
		Artist:   strings.Join(names, ", "),
		Album:    t.Album.Name,
		Duration: time.Duration(t.DurationMs) * time.Millisecond,
		ImageURL: pickImage(t.Album.Images),
	}, true
}

// --- Library ---------------------------------------------------------------

const (
	maxPlaylists = 500
	maxTracks    = 500
)

// ErrPlaylistForbidden is returned by Tracks for playlists Spotify won't
// expose: it only serves the tracks of playlists you own or collaborate on.
var ErrPlaylistForbidden = errors.New("Spotify only exposes tracks of playlists you own or collaborate on")

// userID returns the current user's ID, or "" if it can't be fetched.
func (c *Client) userID(ctx context.Context) string {
	var me struct {
		ID string `json:"id"`
	}
	if _, err := c.do(ctx, http.MethodGet, "/me", nil, nil, &me); err != nil {
		return ""
	}
	return me.ID
}

// Playlists returns "Liked Songs" followed by the user's playlists.
func (c *Client) Playlists(ctx context.Context) ([]Playlist, error) {
	type apiPlaylist struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		URI  string `json:"uri"`
		// Feb 2026: "tracks" was renamed "items". Accept both.
		Items         *struct{ Total int } `json:"items"`
		Tracks        *struct{ Total int } `json:"tracks"`
		Collaborative bool                 `json:"collaborative"`
		Owner         struct {
			ID string `json:"id"`
		} `json:"owner"`
	}
	raw, err := fetchAll[apiPlaylist](ctx, c, "/me/playlists", url.Values{"limit": {"50"}}, maxPlaylists)
	if err != nil {
		return nil, err
	}

	var liked struct {
		Total int `json:"total"`
	}
	if _, err := c.do(ctx, http.MethodGet, "/me/tracks", url.Values{"limit": {"1"}}, nil, &liked); err != nil {
		return nil, err
	}

	me := c.userID(ctx)
	out := []Playlist{{ID: "liked", Name: "Liked Songs", Tracks: liked.Total, Liked: true}}
	for _, p := range raw {
		n := 0
		switch {
		case p.Items != nil:
			n = p.Items.Total
		case p.Tracks != nil:
			n = p.Tracks.Total
		}
		out = append(out, Playlist{
			ID: p.ID, Name: p.Name, URI: p.URI, Tracks: n,
			Locked: me != "" && p.Owner.ID != me && !p.Collaborative,
		})
	}
	return out, nil
}

// Albums returns the user's saved albums as playlist-shaped entries.
type apiAlbum struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	URI         string `json:"uri"`
	ReleaseDate string `json:"release_date"`
	TotalTracks int    `json:"total_tracks"`
	Artists     []struct {
		Name string `json:"name"`
	} `json:"artists"`
	Images []apiImage `json:"images"`
}

func (a apiAlbum) toPlaylist() Playlist {
	names := make([]string, len(a.Artists))
	for i, ar := range a.Artists {
		names[i] = ar.Name
	}
	return Playlist{
		ID: a.ID, Name: a.Name, URI: a.URI, Tracks: a.TotalTracks,
		Album: true, Artist: strings.Join(names, ", "),
		Year: strings.SplitN(a.ReleaseDate, "-", 2)[0], ImageURL: pickImage(a.Images),
	}
}

func (c *Client) Albums(ctx context.Context) ([]Playlist, error) {
	raw, err := fetchAll[struct {
		Album apiAlbum `json:"album"`
	}](ctx, c, "/me/albums", url.Values{"limit": {"50"}}, maxPlaylists)
	if err != nil {
		return nil, err
	}

	out := make([]Playlist, 0, len(raw))
	for _, it := range raw {
		if it.Album.ID != "" {
			out = append(out, it.Album.toPlaylist())
		}
	}
	return out, nil
}

// albumTracks returns the playable tracks of an album. Album listings omit the
// album itself, so name and art come from the album entry.
func (c *Client) albumTracks(ctx context.Context, pl Playlist) ([]Track, error) {
	raw, err := fetchAll[apiTrack](ctx, c, "/albums/"+url.PathEscape(pl.ID)+"/tracks",
		url.Values{"limit": {"50"}}, maxTracks)
	if err != nil {
		return nil, err
	}
	var tracks []Track
	for i := range raw {
		if t, ok := raw[i].toTrack(); ok {
			t.Album, t.ImageURL = pl.Name, pl.ImageURL
			tracks = append(tracks, t)
		}
	}
	return tracks, nil
}

// Tracks returns the playable tracks of a playlist or album (first 500).
func (c *Client) Tracks(ctx context.Context, pl Playlist) ([]Track, error) {
	var tracks []Track
	if pl.Album {
		return c.albumTracks(ctx, pl)
	}
	if pl.IsArtist {
		return c.artistTracks(ctx, pl)
	}
	if pl.Liked {
		raw, err := fetchAll[struct {
			Track *apiTrack `json:"track"`
		}](ctx, c, "/me/tracks", url.Values{"limit": {"50"}}, maxTracks)
		if err != nil {
			return nil, err
		}
		for _, it := range raw {
			if t, ok := it.Track.toTrack(); ok {
				tracks = append(tracks, t)
			}
		}
		return tracks, nil
	}

	raw, err := fetchAll[struct {
		Item  *apiTrack `json:"item"`  // current field name
		Track *apiTrack `json:"track"` // pre-2026 name
	}](ctx, c, "/playlists/"+url.PathEscape(pl.ID)+"/items", url.Values{"limit": {"50"}}, maxTracks)
	var ae *APIError
	if errors.As(err, &ae) && ae.Status == http.StatusForbidden {
		return nil, ErrPlaylistForbidden
	}
	if err != nil {
		return nil, err
	}
	for _, it := range raw {
		t := it.Item
		if t == nil {
			t = it.Track
		}
		if tr, ok := t.toTrack(); ok {
			tracks = append(tracks, tr)
		}
	}
	return tracks, nil
}

// --- Search ----------------------------------------------------------------

// searchLimit is per type; Spotify lowered the maximum in 2026, 10 is safe.
const searchLimit = 10

// Search finds tracks, albums and artists matching query.
func (c *Client) Search(ctx context.Context, query string) (SearchResults, error) {
	var r struct {
		Tracks struct {
			Items []*apiTrack `json:"items"`
		} `json:"tracks"`
		Albums struct {
			Items []*apiAlbum `json:"items"`
		} `json:"albums"`
		Artists struct {
			Items []*struct {
				ID        string     `json:"id"`
				Name      string     `json:"name"`
				URI       string     `json:"uri"`
				Genres    []string   `json:"genres"`
				Images    []apiImage `json:"images"`
				Followers struct {
					Total int `json:"total"`
				} `json:"followers"`
			} `json:"items"`
		} `json:"artists"`
	}
	q := url.Values{"q": {query}, "type": {"track,album,artist"}, "limit": {strconv.Itoa(searchLimit)}}
	if _, err := c.do(ctx, http.MethodGet, "/search", q, nil, &r); err != nil {
		return SearchResults{}, err
	}

	var out SearchResults
	for _, t := range r.Tracks.Items {
		if tr, ok := t.toTrack(); ok {
			out.Tracks = append(out.Tracks, tr)
		}
	}
	for _, a := range r.Albums.Items {
		if a != nil && a.ID != "" {
			out.Albums = append(out.Albums, a.toPlaylist())
		}
	}
	for _, a := range r.Artists.Items {
		if a == nil || a.ID == "" {
			continue
		}
		info := fmt.Sprintf("%d followers", a.Followers.Total)
		if len(a.Genres) > 0 {
			info += " · " + strings.Join(a.Genres[:min(len(a.Genres), 3)], ", ")
		}
		out.Artists = append(out.Artists, Playlist{
			ID: a.ID, Name: a.Name, URI: a.URI, IsArtist: true, Info: info, ImageURL: pickImage(a.Images),
		})
	}
	return out, nil
}

// artistTracks returns an artist's top tracks. If that endpoint is gone or
// refuses (Spotify has been trimming the API), it falls back to a track
// search restricted to the artist.
func (c *Client) artistTracks(ctx context.Context, pl Playlist) ([]Track, error) {
	var r struct {
		Tracks []*apiTrack `json:"tracks"`
	}
	_, err := c.do(ctx, http.MethodGet, "/artists/"+url.PathEscape(pl.ID)+"/top-tracks",
		url.Values{"market": {"from_token"}}, nil, &r)
	var ae *APIError
	if errors.As(err, &ae) && ae.Status != http.StatusTooManyRequests {
		return c.artistSearchTracks(ctx, pl.Name)
	}
	if err != nil {
		return nil, err
	}
	var tracks []Track
	for _, t := range r.Tracks {
		if tr, ok := t.toTrack(); ok {
			tracks = append(tracks, tr)
		}
	}
	return tracks, nil
}

// artistSearchTracks pages through a track search for the artist. Spotify caps
// how many results one search page returns (as few as 5), so keep asking for
// more until a page comes back empty or we have enough.
func (c *Client) artistSearchTracks(ctx context.Context, name string) ([]Track, error) {
	const maxPages, want = 10, 50
	var out []Track
	seen := map[string]bool{}
	offset := 0
	for range maxPages {
		var r struct {
			Tracks struct {
				Items []*apiTrack `json:"items"`
			} `json:"tracks"`
		}
		q := url.Values{"q": {`artist:"` + name + `"`}, "type": {"track"},
			"limit": {strconv.Itoa(searchLimit)}, "offset": {strconv.Itoa(offset)}}
		if _, err := c.do(ctx, http.MethodGet, "/search", q, nil, &r); err != nil {
			if len(out) > 0 {
				break // keep what we have; later pages may be refused
			}
			return nil, err
		}
		if len(r.Tracks.Items) == 0 {
			break
		}
		offset += len(r.Tracks.Items)
		for _, t := range r.Tracks.Items {
			if tr, ok := t.toTrack(); ok && !seen[tr.URI] {
				seen[tr.URI] = true
				out = append(out, tr)
			}
		}
		if len(out) >= want {
			break
		}
	}
	return out, nil
}

// --- Playback --------------------------------------------------------------

// Playback returns the current playback, or nil if nothing is active.
func (c *Client) Playback(ctx context.Context) (*State, error) {
	var raw struct {
		IsPlaying  bool      `json:"is_playing"`
		ProgressMs int       `json:"progress_ms"`
		Shuffle    bool      `json:"shuffle_state"`
		Repeat     string    `json:"repeat_state"`
		Item       *apiTrack `json:"item"`
		Device     struct {
			Name          string `json:"name"`
			VolumePercent int    `json:"volume_percent"`
		} `json:"device"`
	}
	status, err := c.do(ctx, http.MethodGet, "/me/player", nil, nil, &raw)
	if err != nil || status == http.StatusNoContent {
		return nil, err
	}
	t, ok := raw.Item.toTrack()
	if !ok {
		return nil, nil
	}
	return &State{
		Playing:    raw.IsPlaying,
		Track:      t,
		Progress:   time.Duration(raw.ProgressMs) * time.Millisecond,
		Volume:     raw.Device.VolumePercent,
		Shuffle:    raw.Shuffle,
		Repeat:     raw.Repeat,
		DeviceName: raw.Device.Name,
	}, nil
}

// Play starts tracks[idx] in the context of pl. Playlists play via their
// context URI so next/previous follow the playlist; Liked Songs has no
// context, so up to 100 tracks starting at idx are queued instead.
func (c *Client) Play(ctx context.Context, pl Playlist, tracks []Track, idx int) error {
	if idx < 0 || idx >= len(tracks) {
		return errors.New("no track selected")
	}
	body := map[string]any{}
	if pl.Liked || pl.IsArtist || pl.URI == "" {
		var uris []string
		for _, t := range tracks[idx:min(idx+100, len(tracks))] {
			uris = append(uris, t.URI)
		}
		body["uris"] = uris
	} else {
		body["context_uri"] = pl.URI
		body["offset"] = map[string]string{"uri": tracks[idx].URI}
	}
	if id := c.preferredDevice(ctx); id != "" {
		_, err := c.do(ctx, http.MethodPut, "/me/player/play", url.Values{"device_id": {id}}, body, nil)
		return err
	}
	return c.player(ctx, http.MethodPut, "/me/player/play", body)
}

func (c *Client) Resume(ctx context.Context) error {
	return c.player(ctx, http.MethodPut, "/me/player/play", nil)
}
func (c *Client) Pause(ctx context.Context) error {
	return c.player(ctx, http.MethodPut, "/me/player/pause", nil)
}
func (c *Client) Next(ctx context.Context) error {
	return c.player(ctx, http.MethodPost, "/me/player/next", nil)
}
func (c *Client) SetVolume(ctx context.Context, pct int) error {
	return c.playerQ(ctx, http.MethodPut, "/me/player/volume", url.Values{"volume_percent": {strconv.Itoa(pct)}}, nil)
}
func (c *Client) SetShuffle(ctx context.Context, on bool) error {
	return c.playerQ(ctx, http.MethodPut, "/me/player/shuffle", url.Values{"state": {strconv.FormatBool(on)}}, nil)
}

// AddToQueue appends a track to the end of the playback queue. (Spotify's Web
// API has no way to remove from the queue, so spoTERM can't either.)
func (c *Client) AddToQueue(ctx context.Context, uri string) error {
	return c.playerQ(ctx, http.MethodPost, "/me/player/queue", url.Values{"uri": {uri}}, nil)
}

// Queue returns the upcoming tracks, in play order.
func (c *Client) Queue(ctx context.Context) ([]Track, error) {
	var raw struct {
		Queue []*apiTrack `json:"queue"`
	}
	if status, err := c.do(ctx, http.MethodGet, "/me/player/queue", nil, nil, &raw); err != nil || status == http.StatusNoContent {
		return nil, err
	}
	var out []Track
	for _, t := range raw.Queue {
		if tr, ok := t.toTrack(); ok {
			out = append(out, tr)
		}
	}
	return out, nil
}

func (c *Client) Previous(ctx context.Context) error {
	return c.player(ctx, http.MethodPost, "/me/player/previous", nil)
}

// player sends a playback command. If Spotify reports no active device, it
// retries once on the first available device (which wakes that device up).
func (c *Client) player(ctx context.Context, method, path string, body any) error {
	return c.playerQ(ctx, method, path, nil, body)
}

func (c *Client) playerQ(ctx context.Context, method, path string, q url.Values, body any) error {
	_, err := c.do(ctx, method, path, q, body, nil)
	var ae *APIError
	if errors.As(err, &ae) && ae.Status == http.StatusNotFound && ae.Reason == "NO_ACTIVE_DEVICE" {
		id, derr := c.pickDevice(ctx)
		if derr != nil || id == "" {
			return errors.New("no Spotify device found — open Spotify on a device first")
		}
		q = maps.Clone(q)
		if q == nil {
			q = url.Values{}
		}
		q.Set("device_id", id)
		_, err = c.do(ctx, method, path, q, body, nil)
	}
	return err
}

type device struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	IsActive     bool   `json:"is_active"`
	IsRestricted bool   `json:"is_restricted"`
}

func (c *Client) devices(ctx context.Context) ([]device, error) {
	var res struct {
		Devices []device `json:"devices"`
	}
	if _, err := c.do(ctx, http.MethodGet, "/me/player/devices", nil, nil, &res); err != nil {
		return nil, err
	}
	return res.Devices, nil
}

// preferredDevice returns the ID of the device named c.Device, or "" if none
// is set or it isn't online.
func (c *Client) preferredDevice(ctx context.Context) string {
	if c.Device == "" {
		return ""
	}
	devs, err := c.devices(ctx)
	if err != nil {
		return ""
	}
	for _, d := range devs {
		if d.Name == c.Device && !d.IsRestricted && d.ID != "" {
			return d.ID
		}
	}
	return ""
}

// deviceWaitInterval is how often WaitForDevice polls; tests shorten it.
var deviceWaitInterval = 500 * time.Millisecond

// WaitForDevice blocks until a device called name shows up in the user's
// device list (a freshly started player takes a moment to register), or ctx
// ends.
func (c *Client) WaitForDevice(ctx context.Context, name string) error {
	for {
		devs, err := c.devices(ctx)
		if err == nil {
			for _, d := range devs {
				if d.Name == name {
					return nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("player %q did not appear in Spotify's device list", name)
		case <-time.After(deviceWaitInterval):
		}
	}
}

func (c *Client) pickDevice(ctx context.Context) (string, error) {
	devs, err := c.devices(ctx)
	if err != nil {
		return "", err
	}
	first, active := "", ""
	for _, d := range devs {
		if d.IsRestricted || d.ID == "" {
			continue
		}
		if c.Device != "" && d.Name == c.Device {
			return d.ID, nil // our own player beats anything else
		}
		if d.IsActive && active == "" {
			active = d.ID
		}
		if first == "" {
			first = d.ID
		}
	}
	if active != "" {
		return active, nil
	}
	return first, nil
}

// --- Album art -------------------------------------------------------------

var cdn = &http.Client{Timeout: 15 * time.Second}

// Image downloads and decodes an image (album art lives on a public CDN, so
// no auth header is sent).
func (c *Client) Image(ctx context.Context, u string) (image.Image, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := cdn.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cover download: %s", resp.Status)
	}
	img, _, err := image.Decode(io.LimitReader(resp.Body, 8<<20))
	return img, err
}
