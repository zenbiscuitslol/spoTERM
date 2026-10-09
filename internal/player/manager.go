// Package player runs go-librespot, a GUI-less Spotify Connect player, as a
// background child process. Spotify's Web API can only remote-control a
// player; this is the player, so spoTERM can make sound on its own.
//
// go-librespot is an unofficial client and needs Spotify Premium.
package player

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ErrNotInstalled means no go-librespot binary could be found.
var ErrNotInstalled = errors.New("go-librespot not found: install it, point SPOTERM_LIBRESPOT at it, or run with --no-player")

const stopGrace = 3 * time.Second

// Manager owns one go-librespot process.
type Manager struct {
	DeviceName string // how the player appears in Spotify's device list
	Bin        string // go-librespot executable
	ConfigDir  string // spoTERM's own go-librespot config + credentials
	LogPath    string // where its output goes (the TUI owns the terminal)

	// OnLoginURL is called once if go-librespot prints a Spotify login link
	// (first run only). The default opens it in the browser.
	OnLoginURL func(url string)

	mu   sync.Mutex
	cmd  *exec.Cmd
	done chan struct{} // closed when the process has exited
}

// Find locates the go-librespot binary: $SPOTERM_LIBRESPOT, else PATH.
func Find(getenv func(string) string) (string, error) {
	if p := getenv("SPOTERM_LIBRESPOT"); p != "" {
		if _, err := os.Stat(p); err != nil {
			return "", fmt.Errorf("SPOTERM_LIBRESPOT: %w", err)
		}
		return p, nil
	}
	p, err := exec.LookPath("go-librespot")
	if err != nil {
		return "", ErrNotInstalled
	}
	return p, nil
}

// New returns a Manager using the default binary lookup and spoTERM's
// config and state directories.
func New(deviceName string) (*Manager, error) {
	bin, err := Find(os.Getenv)
	if err != nil {
		return nil, err
	}
	cfg, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	state := os.Getenv("XDG_STATE_HOME")
	if state == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		state = filepath.Join(home, ".local", "state")
	}
	return &Manager{
		DeviceName: deviceName,
		Bin:        bin,
		ConfigDir:  filepath.Join(cfg, "spoTERM", "librespot"),
		LogPath:    filepath.Join(state, "spoTERM", "librespot.log"),
		OnLoginURL: openBrowser,
	}, nil
}

// configYAML is the go-librespot config. Credentials use the interactive
// browser flow and are cached in ConfigDir, so login happens once.
func (m *Manager) configYAML() string {
	return fmt.Sprintf(`device_name: %q
device_type: computer
audio_backend: pulseaudio
zeroconf_enabled: false
credentials:
  type: interactive
server:
  enabled: false
`, m.DeviceName)
}

// Start launches go-librespot. It returns as soon as the process has been
// spawned; the device registers with Spotify a little later.
func (m *Manager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cmd != nil {
		return nil
	}

	if err := os.MkdirAll(m.ConfigDir, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(m.ConfigDir, "config.yml"), []byte(m.configYAML()), 0o600); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.LogPath), 0o700); err != nil {
		return err
	}
	logf, err := os.OpenFile(m.LogPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}

	out := &lineTee{w: logf, onURL: m.OnLoginURL}
	cmd := exec.Command(m.Bin, "--config_dir", m.ConfigDir)
	cmd.Stdout, cmd.Stderr = out, out
	setProcAttr(cmd)

	started, done := make(chan error, 1), make(chan struct{})
	go func() {
		// Parent-death signals are tied to the OS thread that spawned the
		// child. Pin this goroutine so that thread lives as long as the child.
		runtime.LockOSThread()
		defer close(done)
		defer logf.Close()
		if err := cmd.Start(); err != nil {
			started <- err
			return
		}
		started <- nil
		_ = cmd.Wait()
	}()
	if err := <-started; err != nil {
		return fmt.Errorf("start %s: %w", filepath.Base(m.Bin), err)
	}
	m.cmd, m.done = cmd, done
	return nil
}

// Running reports whether the process is alive.
func (m *Manager) Running() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.done == nil {
		return false
	}
	select {
	case <-m.done:
		return false
	default:
		return true
	}
}

// Stop terminates go-librespot: SIGTERM, then SIGKILL after a short grace
// period. Safe to call more than once.
func (m *Manager) Stop() {
	m.mu.Lock()
	cmd, done := m.cmd, m.done
	m.cmd, m.done = nil, nil
	m.mu.Unlock()
	if cmd == nil {
		return
	}
	_ = cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-done:
	case <-time.After(stopGrace):
		_ = cmd.Process.Kill()
		<-done
	}
}

// lineTee copies output to the log file and watches it for the Spotify login
// link, reporting the first one it sees.
type lineTee struct {
	w     io.Writer
	onURL func(string)
	buf   []byte
	fired bool
}

var loginURL = regexp.MustCompile(`https://accounts\.spotify\.com/\S+`)

func (t *lineTee) Write(p []byte) (int, error) {
	n, err := t.w.Write(p)
	if t.fired || t.onURL == nil {
		return n, err
	}
	t.buf = append(t.buf, p...)
	for {
		i := bytes.IndexByte(t.buf, '\n')
		if i < 0 {
			break
		}
		line := string(t.buf[:i])
		t.buf = t.buf[i+1:]
		if u := loginURL.FindString(line); u != "" {
			t.fired = true
			t.buf = nil
			t.onURL(strings.TrimRight(u, `"'.,)`))
			break
		}
	}
	if len(t.buf) > 1<<16 { // a runaway line with no newline
		t.buf = nil
	}
	return n, err
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
	_ = cmd.Start() // best effort; the link is in the log too
}
