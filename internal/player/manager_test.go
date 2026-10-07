package player

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeBin writes a stand-in for go-librespot: it prints a login link, records
// its arguments, then sleeps until signalled.
func fakeBin(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "go-librespot")
	script := `#!/bin/sh
echo "args: $*"
echo 'visit "https://accounts.spotify.com/authorize?code=1&state=x", then come back'
exec sleep 60
`
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func newTestManager(t *testing.T, bin string) *Manager {
	t.Helper()
	d := t.TempDir()
	return &Manager{
		DeviceName: "spotiCLI",
		Bin:        bin,
		ConfigDir:  filepath.Join(d, "cfg"),
		LogPath:    filepath.Join(d, "state", "librespot.log"),
	}
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestStartWritesConfigLogsAndStops(t *testing.T) {
	m := newTestManager(t, fakeBin(t))
	urls := make(chan string, 2)
	m.OnLoginURL = func(u string) { urls <- u }

	if err := m.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Stop)
	if !m.Running() {
		t.Fatal("should be running after Start")
	}

	cfg, err := os.ReadFile(filepath.Join(m.ConfigDir, "config.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`device_name: "spotiCLI"`, "audio_backend: pulseaudio", "type: interactive", "enabled: false"} {
		if !strings.Contains(string(cfg), want) {
			t.Errorf("config.yml is missing %q:\n%s", want, cfg)
		}
	}

	select {
	case u := <-urls:
		if u != "https://accounts.spotify.com/authorize?code=1&state=x" {
			t.Errorf("login URL parsed wrong: %q", u)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("login URL was not reported")
	}

	waitFor(t, "output in the log", func() bool {
		b, _ := os.ReadFile(m.LogPath)
		return strings.Contains(string(b), "args: --config_dir "+m.ConfigDir)
	})

	start := time.Now()
	m.Stop()
	if m.Running() {
		t.Error("should not be running after Stop")
	}
	if time.Since(start) > stopGrace {
		t.Error("Stop should end the process with SIGTERM, not wait for the kill timeout")
	}
	m.Stop() // idempotent
}

func TestLoginURLReportedOnlyOnce(t *testing.T) {
	m := newTestManager(t, fakeBin(t))
	n := 0
	m.OnLoginURL = func(string) { n++ }
	tee := &lineTee{w: discard{}, onURL: m.OnLoginURL}
	for i := 0; i < 3; i++ {
		tee.Write([]byte("go to https://accounts.spotify.com/authorize?x=1\n"))
	}
	if n != 1 {
		t.Errorf("callback fired %d times, want 1", n)
	}
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

func TestStartFailsCleanlyForBadBinary(t *testing.T) {
	m := newTestManager(t, filepath.Join(t.TempDir(), "nope"))
	if err := m.Start(); err == nil {
		t.Fatal("expected an error")
	}
	if m.Running() {
		t.Error("nothing should be running")
	}
	m.Stop() // must not panic
}

func TestFind(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := Find(func(string) string { return "" }); err != ErrNotInstalled {
		t.Errorf("got %v, want ErrNotInstalled", err)
	}

	bin := fakeBin(t)
	got, err := Find(func(k string) string {
		if k == "SPOTICLI_LIBRESPOT" {
			return bin
		}
		return ""
	})
	if err != nil || got != bin {
		t.Errorf("env override: got %q, %v", got, err)
	}

	if _, err := Find(func(string) string { return "/no/such/file" }); err == nil || err == ErrNotInstalled {
		t.Errorf("a wrong SPOTICLI_LIBRESPOT should be reported as such, got %v", err)
	}
}
