//go:build darwin || linux

package answerbench

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestManagedProcessKillsDescendants(t *testing.T) {
	pidFile := t.TempDir() + "/descendant.pid"
	ctx, cancel := context.WithCancel(t.Context())
	cmd := child(ctx, os.Args[0], "-test.run=TestManagedProcessTreeHelper")
	cmd.Env = append(os.Environ(), "ANSWERBENCH_TREE_HELPER=parent", "ANSWERBENCH_TREE_PID="+pidFile)
	process, err := startManaged(cmd)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	var pid int
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		raw, readErr := os.ReadFile(pidFile)
		if readErr == nil {
			pid, err = strconv.Atoi(strings.TrimSpace(string(raw)))
			if err != nil {
				t.Fatal(err)
			}
			break
		}
		if !errors.Is(readErr, os.ErrNotExist) {
			t.Fatal(readErr)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pid == 0 {
		t.Fatal("descendant did not start")
	}
	if err := process.stop(cancel); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		err = syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("descendant process %d survived owned cleanup", pid)
}

func TestManagedProcessTreeHelper(t *testing.T) {
	mode := os.Getenv("ANSWERBENCH_TREE_HELPER")
	if mode == "" {
		return
	}
	if mode == "parent" {
		cmd := exec.Command(os.Args[0], "-test.run=TestManagedProcessTreeHelper")
		cmd.Env = append(os.Environ(), "ANSWERBENCH_TREE_HELPER=descendant")
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(os.Getenv("ANSWERBENCH_TREE_PID"), []byte(strconv.Itoa(cmd.Process.Pid)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for {
		time.Sleep(time.Second)
	}
}
