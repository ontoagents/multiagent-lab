package procgroup

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestStopperRetriesTerminationFailure(t *testing.T) {
	exited := make(chan struct{})
	defer close(exited)
	waitStarted := make(chan struct{})
	denied := errors.New("permission denied")
	var attempts, waits atomic.Int32
	stop := processStopper(42, time.Second, func(_ int, graceful bool) error {
		attempts.Add(1)
		return denied
	}, func() error { waits.Add(1); close(waitStarted); <-exited; return nil })
	for range 2 {
		if err := stop(); !errors.Is(err, denied) {
			t.Fatalf("termination failure must be reported: %v", err)
		}
	}
	if attempts.Load() != 4 {
		t.Fatalf("each retry must try graceful then force: %d", attempts.Load())
	}
	// No additional waiter may be created by retries.
	select {
	case <-waitStarted:
	case <-time.After(time.Second):
		t.Fatal("child waiter did not start")
	}
	if waits.Load() != 1 {
		t.Fatalf("waiters=%d", waits.Load())
	}
}

func TestStopperConfirmsExitAfterForce(t *testing.T) {
	exited := make(chan struct{})
	var forced atomic.Bool
	stop := processStopper(42, time.Millisecond, func(_ int, graceful bool) error {
		if !graceful {
			forced.Store(true)
			close(exited)
		}
		return nil
	}, func() error { <-exited; return nil })
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	if !forced.Load() {
		t.Fatal("unresponsive process was not force stopped")
	}
	if err := stop(); err != nil {
		t.Fatalf("already exited retry: %v", err)
	}
}

func TestStopperDoesNotReportUnconfirmedExit(t *testing.T) {
	exited := make(chan struct{})
	stop := processStopper(42, time.Millisecond, func(int, bool) error { return nil }, func() error { <-exited; return nil })
	if err := stop(); err == nil {
		t.Fatal("successful signals are not proof of process exit")
	}
	close(exited)
}
