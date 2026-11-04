package procgroup

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

// Stopper keeps one child waiter across failed attempts. Success means the
// process exit was observed, including after forced termination.
func Stopper(process *os.Process, grace time.Duration) func() error {
	return processStopper(process.Pid, grace, Kill, func() error {
		_, err := process.Wait()
		if errors.Is(err, os.ErrProcessDone) {
			return nil
		}
		return err
	})
}

func processStopper(pid int, grace time.Duration, kill func(int, bool) error, wait func() error) func() error {
	var mu sync.Mutex
	var once sync.Once
	done := make(chan struct{})
	var waitErr error
	result := func() error {
		if waitErr != nil {
			return fmt.Errorf("确认引擎退出失败: %w", waitErr)
		}
		return nil
	}
	return func() error {
		mu.Lock()
		defer mu.Unlock()
		once.Do(func() { go func() { waitErr = wait(); close(done) }() })
		select {
		case <-done:
			return result()
		default:
		}
		softErr := kill(pid, true)
		if softErr == nil {
			timer := time.NewTimer(grace)
			defer timer.Stop()
			select {
			case <-done:
				return result()
			case <-timer.C:
			}
		}
		if err := kill(pid, false); err != nil {
			select {
			case <-done:
				return result()
			default:
				return fmt.Errorf("终止引擎失败: %w", errors.Join(softErr, err))
			}
		}
		timer := time.NewTimer(grace)
		defer timer.Stop()
		select {
		case <-done:
			return result()
		case <-timer.C:
			return fmt.Errorf("终止引擎后未确认退出，请重试停止")
		}
	}
}
