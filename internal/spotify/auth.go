package spotify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

// RedirectURL must be added to the app's "Redirect URIs" in the Spotify
// developer dashboard. Spotify requires the loopback IP, not "localhost".
const (
	redirectHost = "127.0.0.1:8888"
	RedirectURL  = "http://" + redirectHost + "/callback"
)

var scopes = []string{
	"user-read-playback-state",
	"user-modify-playback-state",
	"user-read-currently-playing",
	"playlist-read-private",
	"playlist-read-collaborative",
	"user-library-read",
}

var endpoint = oauth2.Endpoint{
	AuthURL:   "https://accounts.spotify.com/authorize",
	TokenURL:  "https://accounts.spotify.com/api/token",
	AuthStyle: oauth2.AuthStyleInParams, // PKCE: client_id in the body, no secret
}

// Connect returns an authenticated client. It reuses the token cached in the
// user config dir; if there is none, it runs the browser login (PKCE flow,
// so no client secret is needed) and caches the result.
func Connect(ctx context.Context, clientID string) (*Client, error) {
	cfg := &oauth2.Config{
		ClientID:    clientID,
		Endpoint:    endpoint,
		RedirectURL: RedirectURL,
		Scopes:      scopes,
	}

	tok, err := loadToken()
	if err != nil {
		tok, err = login(ctx, cfg)
		if err != nil {
			return nil, err
		}
		if err := saveToken(tok); err != nil {
			return nil, err
		}
	}

	src := &persistSource{src: cfg.TokenSource(context.Background(), tok), last: tok.AccessToken}
	hc := oauth2.NewClient(context.Background(), src)
	hc.Timeout = 20 * time.Second
	return &Client{http: hc, base: apiBase}, nil
}

// Logout deletes the cached token.
func Logout() error {
	p, err := tokenPath()
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func login(ctx context.Context, cfg *oauth2.Config) (*oauth2.Token, error) {
	verifier := oauth2.GenerateVerifier()
	state := oauth2.GenerateVerifier() // any unguessable string works as state
	authURL := cfg.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier))

	ln, err := net.Listen("tcp", redirectHost)
	if err != nil {
		return nil, fmt.Errorf("can't listen on %s for the login callback: %w", redirectHost, err)
	}
	type result struct {
		code string
		err  error
	}
	done := make(chan result, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		var res result
		switch {
		case q.Get("state") != state:
			res.err = errors.New("login failed: state mismatch")
		case q.Get("error") != "":
			res.err = fmt.Errorf("login denied: %s", q.Get("error"))
		default:
			res.code = q.Get("code")
		}
		if res.err != nil {
			http.Error(w, res.err.Error(), http.StatusBadRequest)
		} else {
			fmt.Fprint(w, "<h3>spoTERM is connected. You can close this tab.</h3>")
		}
		select {
		case done <- res:
		default:
		}
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go srv.Serve(ln)
	defer srv.Close()

	fmt.Println("Open this URL to log in to Spotify (trying to open your browser):")
	fmt.Println(authURL)
	openBrowser(authURL)

	select {
	case res := <-done:
		if res.err != nil {
			return nil, res.err
		}
		return cfg.Exchange(ctx, res.code, oauth2.VerifierOption(verifier))
	case <-time.After(3 * time.Minute):
		return nil, errors.New("login timed out")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func openBrowser(u string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", u)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	default:
		cmd = exec.Command("xdg-open", u)
	}
	_ = cmd.Start() // best effort; the URL is printed anyway
}

// --- token cache -----------------------------------------------------------

func tokenPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "spoTERM", "token.json"), nil
}

func loadToken() (*oauth2.Token, error) {
	p, err := tokenPath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var t oauth2.Token
	if err := json.Unmarshal(b, &t); err != nil {
		return nil, err
	}
	if t.RefreshToken == "" {
		return nil, errors.New("cached token has no refresh token")
	}
	return &t, nil
}

func saveToken(t *oauth2.Token) error {
	p, err := tokenPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(t)
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o600)
}

// persistSource writes refreshed tokens back to disk so a refresh survives
// restarts (Spotify may rotate the refresh token).
type persistSource struct {
	mu   sync.Mutex
	src  oauth2.TokenSource
	last string
}

func (p *persistSource) Token() (*oauth2.Token, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	t, err := p.src.Token()
	if err == nil && t.AccessToken != p.last {
		p.last = t.AccessToken
		_ = saveToken(t)
	}
	return t, err
}
