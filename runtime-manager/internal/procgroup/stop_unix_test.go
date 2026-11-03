//go:build !windows

package procgroup

import (
	"os/exec"
	"testing"
	"time"
)

func TestStopperTerminatesRealProcess(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	SetPgid(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	stop := Stopper(cmd.Process, time.Second)
	t.Cleanup(func() { _ = stop() })
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	if err := stop(); err != nil {
		t.Fatalf("already stopped process must be safe to retry: %v", err)
	}
}
