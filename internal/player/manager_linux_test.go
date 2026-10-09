package player

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestMain doubles as the "spoTERM" process in TestDaemonDiesWithParent.
func TestMain(m *testing.M) {
	if bin := os.Getenv("PLAYER_TEST_BIN"); bin != "" {
		mgr := &Manager{DeviceName: "x", Bin: bin,
			ConfigDir: os.Getenv("PLAYER_TEST_DIR") + "/cfg", LogPath: os.Getenv("PLAYER_TEST_DIR") + "/log"}
		if err := mgr.Start(); err != nil {
			os.Exit(2)
		}
		time.Sleep(time.Minute) // wait to be killed
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func alive(pid int) bool {
	if syscall.Kill(pid, 0) != nil {
		return false
	}
	// A reaped-late zombie still answers kill(0); treat it as dead.
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	return err == nil && !strings.Contains(string(b), ") Z ")
}

// If spoTERM is killed without a chance to clean up, the kernel must take the
// player down with it.
func TestDaemonDiesWithParent(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "pid")
	bin := filepath.Join(dir, "fake")
	os.WriteFile(bin, []byte("#!/bin/sh\necho $$ > "+pidFile+"\nexec sleep 60\n"), 0o755)

	parent := exec.Command(os.Args[0])
	parent.Env = append(os.Environ(), "PLAYER_TEST_BIN="+bin, "PLAYER_TEST_DIR="+dir)
	if err := parent.Start(); err != nil {
		t.Fatal(err)
	}
	defer parent.Process.Kill()

	var pid int
	waitFor(t, "daemon to start", func() bool {
		b, err := os.ReadFile(pidFile)
		if err != nil {
			return false
		}
		pid, err = strconv.Atoi(strings.TrimSpace(string(b)))
		return err == nil && alive(pid)
	})

	parent.Process.Signal(syscall.SIGKILL)
	parent.Wait()
	waitFor(t, "daemon to exit after parent was killed", func() bool { return !alive(pid) })
}
