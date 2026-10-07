package spotify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type recorded struct {
	Method, Path, Query, Body string
}

// fake starts a server answering by "METHOD /path"; it records every request.
func fake(t *testing.T, routes map[string]func(w http.ResponseWriter, r *http.Request)) (*Client, *[]recorded) {
	t.Helper()
	var mu sync.Mutex
	var log []recorded
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		log = append(log, recorded{r.Method, r.URL.Path, r.URL.RawQuery, string(b)})
		mu.Unlock()
		h, ok := routes[r.Method+" "+r.URL.Path]
		if !ok {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	return &Client{http: srv.Client(), base: srv.URL}, &log
}

func js(body string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, body)
	}
}

func apiErr(status int, reason string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		fmt.Fprintf(w, `{"error":{"status":%d,"message":"nope","reason":%q}}`, status, reason)
	}
}

func TestPlaylistsAcceptsRenamedItemsField(t *testing.T) {
	c, _ := fake(t, map[string]func(http.ResponseWriter, *http.Request){
		"GET /me/playlists": js(`{"items":[
			{"id":"a","name":"New","uri":"spotify:playlist:a","items":{"total":7}},
			{"id":"b","name":"Old","uri":"spotify:playlist:b","tracks":{"total":3}}],"next":null}`),
		"GET /me/tracks": js(`{"total":42,"items":[]}`),
		"GET /me":        js(`{"id":"me"}`),
	})
	got, err := c.Playlists(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || !got[0].Liked || got[0].Tracks != 42 {
		t.Fatalf("liked songs should come first: %+v", got)
	}
	if got[1].Tracks != 7 || got[2].Tracks != 3 {
		t.Errorf("track counts wrong: %+v", got)
	}
}

func TestPlaylistsFollowsPagination(t *testing.T) {
	var base string
	c, _ := fake(t, map[string]func(http.ResponseWriter, *http.Request){
		"GET /me/playlists": func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("offset") == "50" {
				js(`{"items":[{"id":"2","name":"Two"}],"next":null}`)(w, r)
				return
			}
			js(fmt.Sprintf(`{"items":[{"id":"1","name":"One"}],"next":"%s/me/playlists?offset=50"}`, base))(w, r)
		},
		"GET /me/tracks": js(`{"total":0}`),
		"GET /me":        js(`{"id":"me"}`),
	})
	base = c.base
	got, err := c.Playlists(context.Background())
	if err != nil || len(got) != 3 {
		t.Fatalf("want liked + 2 playlists, got %+v, %v", got, err)
	}
}

func TestPlaylistTracksUsesItemsEndpoint(t *testing.T) {
	c, log := fake(t, map[string]func(http.ResponseWriter, *http.Request){
		"GET /playlists/abc/items": js(`{"items":[
			{"item":{"uri":"spotify:track:1","name":"Song","type":"track","duration_ms":61000,
			  "artists":[{"name":"A"},{"name":"B"}],
			  "album":{"name":"Alb","images":[{"url":"big","width":640},{"url":"mid","width":300},{"url":"small","width":64}]}}},
			{"track":{"uri":"spotify:track:2","name":"OldShape","type":"track","artists":[],"album":{}}},
			{"item":null},
			{"item":{"uri":"spotify:local:x","name":"Local","is_local":true}},
			{"item":{"uri":"spotify:episode:9","name":"Pod","type":"episode"}}
		],"next":null}`),
	})
	got, err := c.Tracks(context.Background(), Playlist{ID: "abc"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 playable tracks, got %+v", got)
	}
	if got[0].Artist != "A, B" || got[0].Duration.Seconds() != 61 || got[0].ImageURL != "mid" {
		t.Errorf("track decoded wrong: %+v", got[0])
	}
	if (*log)[0].Path != "/playlists/abc/items" {
		t.Errorf("hit removed endpoint: %s", (*log)[0].Path)
	}
}

func TestPlaylistTracksForbiddenIsExplained(t *testing.T) {
	c, _ := fake(t, map[string]func(http.ResponseWriter, *http.Request){
		"GET /playlists/x/items": apiErr(403, ""),
	})
	_, err := c.Tracks(context.Background(), Playlist{ID: "x"})
	if err == nil || !strings.Contains(err.Error(), "own or collaborate") {
		t.Fatalf("want friendly 403 message, got %v", err)
	}
}

func TestPlaybackIdleIs204(t *testing.T) {
	c, _ := fake(t, map[string]func(http.ResponseWriter, *http.Request){
		"GET /me/player": func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) },
	})
	st, err := c.Playback(context.Background())
	if st != nil || err != nil {
		t.Fatalf("want nil,nil got %v,%v", st, err)
	}
}

func TestPlaybackDecodes(t *testing.T) {
	c, _ := fake(t, map[string]func(http.ResponseWriter, *http.Request){
		"GET /me/player": js(`{"is_playing":true,"progress_ms":5000,"shuffle_state":true,"repeat_state":"context",
			"device":{"name":"Laptop","volume_percent":55},
			"item":{"uri":"spotify:track:1","name":"S","type":"track","duration_ms":9000,"artists":[{"name":"A"}],"album":{"name":"Al"}}}`),
	})
	st, err := c.Playback(context.Background())
	if err != nil || st == nil {
		t.Fatal(st, err)
	}
	if !st.Playing || st.Progress.Seconds() != 5 || st.Volume != 55 || !st.Shuffle || st.Repeat != "context" || st.DeviceName != "Laptop" {
		t.Errorf("%+v", st)
	}
}

func TestPlayBodies(t *testing.T) {
	tracks := []Track{{URI: "u0"}, {URI: "u1"}, {URI: "u2"}}

	c, log := fake(t, map[string]func(http.ResponseWriter, *http.Request){
		"PUT /me/player/play": func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) },
	})
	if err := c.Play(context.Background(), Playlist{URI: "spotify:playlist:p"}, tracks, 1); err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	json.Unmarshal([]byte((*log)[0].Body), &body)
	if body["context_uri"] != "spotify:playlist:p" || body["offset"].(map[string]any)["uri"] != "u1" {
		t.Errorf("playlist body: %s", (*log)[0].Body)
	}

	if err := c.Play(context.Background(), Playlist{Liked: true}, tracks, 1); err != nil {
		t.Fatal(err)
	}
	json.Unmarshal([]byte((*log)[1].Body), &body)
	uris := body["uris"].([]any)
	if len(uris) != 2 || uris[0] != "u1" {
		t.Errorf("liked body should queue from the selected track: %s", (*log)[1].Body)
	}
}

func TestCommandRetriesOnFirstDeviceWhenNoneActive(t *testing.T) {
	calls := 0
	c, log := fake(t, map[string]func(http.ResponseWriter, *http.Request){
		"POST /me/player/next": func(w http.ResponseWriter, r *http.Request) {
			calls++
			if r.URL.Query().Get("device_id") == "" {
				apiErr(404, "NO_ACTIVE_DEVICE")(w, r)
				return
			}
			w.WriteHeader(204)
		},
		"GET /me/player/devices": js(`{"devices":[{"id":"locked","is_restricted":true},{"id":"dev1"},{"id":"dev2"}]}`),
	})
	if err := c.Next(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || !strings.Contains((*log)[len(*log)-1].Query, "device_id=dev1") {
		t.Errorf("want retry on dev1, log=%+v", *log)
	}
}

func TestNoDevicesGivesHelpfulError(t *testing.T) {
	c, _ := fake(t, map[string]func(http.ResponseWriter, *http.Request){
		"PUT /me/player/pause":   apiErr(404, "NO_ACTIVE_DEVICE"),
		"GET /me/player/devices": js(`{"devices":[]}`),
	})
	err := c.Pause(context.Background())
	if err == nil || !strings.Contains(err.Error(), "open Spotify") {
		t.Fatalf("got %v", err)
	}
}

func TestRateLimitMessage(t *testing.T) {
	c, _ := fake(t, map[string]func(http.ResponseWriter, *http.Request){
		"GET /me/player": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(429)
		},
	})
	_, err := c.Playback(context.Background())
	if err == nil || !strings.Contains(err.Error(), "retry in 7s") {
		t.Fatalf("got %v", err)
	}
}

func TestAlbumsDecodes(t *testing.T) {
	c, _ := fake(t, map[string]func(http.ResponseWriter, *http.Request){
		"GET /me/albums": js(`{"items":[
			{"album":{"id":"al1","name":"Afterglow","uri":"spotify:album:al1","release_date":"2021-05-14","total_tracks":9,
			  "artists":[{"name":"A"},{"name":"B"}],
			  "images":[{"url":"big","width":640},{"url":"mid","width":300},{"url":"small","width":64}]}},
			{"album":{"id":"al2","name":"Year Only","uri":"spotify:album:al2","release_date":"1999","total_tracks":1,"artists":[],"images":[]}},
			{"album":null}
		],"next":null}`),
	})
	got, err := c.Albums(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 albums (null skipped), got %+v", got)
	}
	a := got[0]
	if !a.Album || a.Name != "Afterglow" || a.Artist != "A, B" || a.Year != "2021" || a.Tracks != 9 || a.ImageURL != "mid" {
		t.Errorf("album decoded wrong: %+v", a)
	}
	if got[1].Year != "1999" {
		t.Errorf("year-only release date: %+v", got[1])
	}
}

func TestAlbumTracksUseAlbumEndpointAndFillAlbumInfo(t *testing.T) {
	c, log := fake(t, map[string]func(http.ResponseWriter, *http.Request){
		"GET /albums/al1/tracks": js(`{"items":[
			{"uri":"spotify:track:1","name":"One","type":"track","duration_ms":1000,"artists":[{"name":"A"}]},
			{"uri":"spotify:track:2","name":"Two","type":"track","duration_ms":2000,"artists":[{"name":"A"}]}
		],"next":null}`),
	})
	got, err := c.Tracks(context.Background(), Playlist{ID: "al1", Name: "Afterglow", Album: true, ImageURL: "mid"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Album != "Afterglow" || got[0].ImageURL != "mid" {
		t.Fatalf("album tracks wrong: %+v", got)
	}
	if (*log)[0].Path != "/albums/al1/tracks" {
		t.Errorf("wrong endpoint: %s", (*log)[0].Path)
	}
}

func TestPlayAlbumUsesContextURI(t *testing.T) {
	c, log := fake(t, map[string]func(http.ResponseWriter, *http.Request){
		"PUT /me/player/play": js(``),
	})
	tracks := []Track{{URI: "spotify:track:1"}, {URI: "spotify:track:2"}}
	pl := Playlist{ID: "al1", URI: "spotify:album:al1", Album: true}
	if err := c.Play(context.Background(), pl, tracks, 1); err != nil {
		t.Fatal(err)
	}
	b := (*log)[0].Body
	if !strings.Contains(b, `"context_uri":"spotify:album:al1"`) || !strings.Contains(b, `spotify:track:2`) {
		t.Errorf("body: %s", b)
	}
}

func TestPlaylistsMarkFollowedOnesLocked(t *testing.T) {
	c, _ := fake(t, map[string]func(http.ResponseWriter, *http.Request){
		"GET /me": js(`{"id":"me"}`),
		"GET /me/playlists": js(`{"items":[
			{"id":"own","name":"Mine","owner":{"id":"me"}},
			{"id":"fol","name":"Followed","owner":{"id":"someone"}},
			{"id":"col","name":"Shared","owner":{"id":"someone"},"collaborative":true}],"next":null}`),
		"GET /me/tracks": js(`{"total":0}`),
	})
	got, err := c.Playlists(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	locked := map[string]bool{}
	for _, p := range got {
		locked[p.ID] = p.Locked
	}
	if locked["liked"] || locked["own"] || locked["col"] || !locked["fol"] {
		t.Errorf("only the followed playlist should be locked: %v", locked)
	}
}

func TestPlaylistsNothingLockedWhenUserUnknown(t *testing.T) {
	c, _ := fake(t, map[string]func(http.ResponseWriter, *http.Request){
		"GET /me":           apiErr(500, ""),
		"GET /me/playlists": js(`{"items":[{"id":"x","name":"X","owner":{"id":"someone"}}],"next":null}`),
		"GET /me/tracks":    js(`{"total":0}`),
	})
	got, err := c.Playlists(context.Background())
	if err != nil || got[1].Locked {
		t.Fatalf("should not guess without knowing the user: %+v %v", got, err)
	}
}

func devicesJSON(devs ...string) string { // "id:name:active"
	var items []string
	for _, d := range devs {
		p := strings.SplitN(d, ":", 3)
		items = append(items, fmt.Sprintf(`{"id":%q,"name":%q,"is_active":%s,"is_restricted":false}`, p[0], p[1], p[2]))
	}
	return `{"devices":[` + strings.Join(items, ",") + `]}`
}

func TestPlayPrefersOwnDeviceOverActiveOne(t *testing.T) {
	c, log := fake(t, map[string]func(http.ResponseWriter, *http.Request){
		"GET /me/player/devices": js(devicesJSON("phone:Phone:true", "mine:spotiCLI:false")),
		"PUT /me/player/play":    js(``),
	})
	c.Device = "spotiCLI"
	pl := Playlist{ID: "p", URI: "spotify:playlist:p"}
	if err := c.Play(context.Background(), pl, []Track{{URI: "spotify:track:1"}}, 0); err != nil {
		t.Fatal(err)
	}
	last := (*log)[len(*log)-1]
	if last.Method != "PUT" || last.Query != "device_id=mine" {
		t.Errorf("should play on our device, got %s ?%s", last.Method, last.Query)
	}
}

func TestPlayFallsBackWhenOwnDeviceIsOffline(t *testing.T) {
	c, log := fake(t, map[string]func(http.ResponseWriter, *http.Request){
		"GET /me/player/devices": js(devicesJSON("phone:Phone:true")),
		"PUT /me/player/play":    js(``),
	})
	c.Device = "spotiCLI"
	if err := c.Play(context.Background(), Playlist{URI: "spotify:playlist:p"}, []Track{{URI: "t"}}, 0); err != nil {
		t.Fatal(err)
	}
	if last := (*log)[len(*log)-1]; last.Query != "" {
		t.Errorf("should not target a device when ours is offline, got ?%s", last.Query)
	}
}

func TestPickDevicePrefersNamedThenActiveThenFirst(t *testing.T) {
	c, _ := fake(t, map[string]func(http.ResponseWriter, *http.Request){
		"GET /me/player/devices": js(devicesJSON("a:First:false", "b:Active:true", "c:spotiCLI:false")),
	})
	if id, _ := c.pickDevice(context.Background()); id != "b" {
		t.Errorf("no preference: want active device b, got %q", id)
	}
	c.Device = "spotiCLI"
	if id, _ := c.pickDevice(context.Background()); id != "c" {
		t.Errorf("with preference: want c, got %q", id)
	}
}

func TestWaitForDevice(t *testing.T) {
	old := deviceWaitInterval
	deviceWaitInterval = time.Millisecond
	t.Cleanup(func() { deviceWaitInterval = old })

	var mu sync.Mutex
	calls := 0
	c, _ := fake(t, map[string]func(http.ResponseWriter, *http.Request){
		"GET /me/player/devices": func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			calls++
			n := calls
			mu.Unlock()
			if n < 3 { // registers on the third look
				js(devicesJSON("phone:Phone:true"))(w, r)
				return
			}
			js(devicesJSON("phone:Phone:true", "mine:spotiCLI:false"))(w, r)
		},
	})
	if err := c.WaitForDevice(context.Background(), "spotiCLI"); err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Errorf("polled %d times, want 3", calls)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := c.WaitForDevice(ctx, "nonexistent"); err == nil || !strings.Contains(err.Error(), "did not appear") {
		t.Errorf("want a clear timeout error, got %v", err)
	}
}

const searchJSON = `{
 "tracks":{"items":[{"uri":"spotify:track:1","name":"Song","type":"track","duration_ms":61000,
   "artists":[{"name":"Ann"}],"album":{"name":"LP","images":[]}}, null]},
 "albums":{"items":[{"id":"al1","name":"LP","uri":"spotify:album:al1","release_date":"2020-05-01","total_tracks":9,
   "artists":[{"name":"Ann"}],"images":[]}]},
 "artists":{"items":[{"id":"ar1","name":"Ann","uri":"spotify:artist:ar1","genres":["pop","folk"],"followers":{"total":42}}]}}`

func TestSearchDecodes(t *testing.T) {
	c, log := fake(t, map[string]func(http.ResponseWriter, *http.Request){"GET /search": js(searchJSON)})
	r, err := c.Search(context.Background(), "ann")
	if err != nil {
		t.Fatal(err)
	}
	if q := (*log)[0].Query; !strings.Contains(q, "q=ann") || !strings.Contains(q, "type=track%2Calbum%2Cartist") {
		t.Errorf("query = %s", q)
	}
	if len(r.Tracks) != 1 || r.Tracks[0].Title != "Song" || r.Tracks[0].Duration != 61*time.Second {
		t.Errorf("tracks = %+v", r.Tracks)
	}
	if len(r.Albums) != 1 || !r.Albums[0].Album || r.Albums[0].Year != "2020" || r.Albums[0].Tracks != 9 {
		t.Errorf("albums = %+v", r.Albums)
	}
	if len(r.Artists) != 1 || !r.Artists[0].IsArtist || r.Artists[0].Info != "42 followers · pop, folk" {
		t.Errorf("artists = %+v", r.Artists)
	}
}

func TestArtistTracksUseTopTracksThenFallBackToSearch(t *testing.T) {
	ar := Playlist{ID: "ar1", Name: "Ann", IsArtist: true}
	c, _ := fake(t, map[string]func(http.ResponseWriter, *http.Request){
		"GET /artists/ar1/top-tracks": js(`{"tracks":[{"uri":"spotify:track:9","name":"Hit","type":"track","artists":[{"name":"Ann"}]}]}`),
	})
	if tr, err := c.Tracks(context.Background(), ar); err != nil || len(tr) != 1 || tr[0].Title != "Hit" {
		t.Fatalf("top tracks: %v %v", tr, err)
	}

	c, log := fake(t, map[string]func(http.ResponseWriter, *http.Request){
		"GET /artists/ar1/top-tracks": apiErr(403, ""),
		"GET /search":                 js(searchJSON),
	})
	if tr, err := c.Tracks(context.Background(), ar); err != nil || len(tr) != 1 {
		t.Fatalf("fallback: %v %v", tr, err)
	}
	if q := (*log)[1].Query; !strings.Contains(q, "artist%3A%22Ann%22") {
		t.Errorf("fallback query = %s", q)
	}
}
