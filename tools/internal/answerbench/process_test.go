package answerbench

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestManagedProcessCleanupDoesNotStopSibling(t *testing.T) {
	firstCancel, first := startHelper(t)
	secondCancel, second := startHelper(t)
	if err := first.stop(firstCancel); err != nil {
		t.Fatal(err)
	}
	select {
	case <-second.Done():
		t.Fatal("stopping owned child stopped unrelated child")
	default:
	}
	if err := second.stop(secondCancel); err != nil {
		t.Fatal(err)
	}
}

func TestManagedProcessReportsStartFailure(t *testing.T) {
	if _, err := startManaged(exec.Command(filepath.Join(t.TempDir(), "missing"))); err == nil {
		t.Fatal("missing child executable started")
	}
}

func startHelper(t *testing.T) (context.CancelFunc, *managedProcess) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	cmd := child(ctx, os.Args[0], "-test.run=TestManagedProcessHelper")
	cmd.Env = append(os.Environ(), "ANSWERBENCH_PROCESS_HELPER=1")
	process, err := startManaged(cmd)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	return cancel, process
}

func TestManagedProcessHelper(_ *testing.T) {
	if os.Getenv("ANSWERBENCH_PROCESS_HELPER") != "1" {
		return
	}
	for {
		time.Sleep(time.Second)
	}
}
