package sync

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestThreadGroupStopChan(t *testing.T) {
	var tg ThreadGroup

	select {
	case <-tg.StopChan():
		t.Fatal("StopChan closed before Stop")
	default:
	}
	if tg.StopCtx().Err() != nil {
		t.Fatal("StopCtx done before Stop")
	}

	if err := tg.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	select {
	case <-tg.StopChan():
	default:
		t.Fatal("StopChan not closed after Stop")
	}
	if tg.StopCtx().Err() == nil {
		t.Fatal("StopCtx not done after Stop")
	}
}

func TestThreadGroupStopWaitsForDone(t *testing.T) {
	var tg ThreadGroup

	var finished atomic.Bool
	if err := tg.Add(); err != nil {
		t.Fatalf("Add: %v", err)
	}
	go func() {
		defer tg.Done()
		<-tg.StopChan()
		time.Sleep(50 * time.Millisecond)
		finished.Store(true)
	}()

	if err := tg.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if !finished.Load() {
		t.Fatal("Stop returned before the running thread called Done")
	}
}

func TestThreadGroupStoppedErrors(t *testing.T) {
	var tg ThreadGroup
	if err := tg.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	if err := tg.Add(); err != ErrStopped {
		t.Errorf("Add after Stop: got %v, want ErrStopped", err)
	}
	if err := tg.Flush(); err != ErrStopped {
		t.Errorf("Flush after Stop: got %v, want ErrStopped", err)
	}
	if err := tg.Stop(); err != ErrStopped {
		t.Errorf("second Stop: got %v, want ErrStopped", err)
	}
}

func TestThreadGroupOnStopAfterStopOrder(t *testing.T) {
	var tg ThreadGroup

	var mu sync.Mutex
	var order []string
	record := func(s string) func() {
		return func() {
			mu.Lock()
			defer mu.Unlock()
			order = append(order, s)
		}
	}

	// A running thread that only exits once OnStop fires lets us verify that
	// OnStop runs before the wait and AfterStop after it.
	release := make(chan struct{})
	tg.OnStop(record("onStop1"))
	tg.OnStop(func() {
		record("onStop2")()
		close(release)
	})
	tg.AfterStop(record("afterStop1"))
	tg.AfterStop(record("afterStop2"))

	if err := tg.Add(); err != nil {
		t.Fatalf("Add: %v", err)
	}
	go func() {
		defer tg.Done()
		<-release
		record("threadDone")()
	}()

	if err := tg.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// OnStop and AfterStop both run in reverse order of registration.
	want := []string{"onStop2", "onStop1", "threadDone", "afterStop2", "afterStop1"}
	if len(order) != len(want) {
		t.Fatalf("got order %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("got order %v, want %v", order, want)
		}
	}
}

func TestThreadGroupOnStopAfterStopWhenStopped(t *testing.T) {
	var tg ThreadGroup
	if err := tg.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// After Stop, registered functions run immediately.
	var ran int
	tg.OnStop(func() { ran++ })
	tg.AfterStop(func() { ran++ })
	if ran != 2 {
		t.Fatalf("got %d immediate calls, want 2", ran)
	}
}

func TestThreadGroupFlush(t *testing.T) {
	var tg ThreadGroup

	var finished atomic.Bool
	if err := tg.Add(); err != nil {
		t.Fatalf("Add: %v", err)
	}
	go func() {
		defer tg.Done()
		time.Sleep(50 * time.Millisecond)
		finished.Store(true)
	}()

	if err := tg.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if !finished.Load() {
		t.Fatal("Flush returned before the running thread called Done")
	}

	if err := tg.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestThreadGroupConcurrentAddStop(t *testing.T) {
	var tg ThreadGroup

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				if tg.Add() != nil {
					return
				}
				go func() {
					defer tg.Done()
					<-tg.StopChan()
				}()
			}
		}()
	}

	time.Sleep(10 * time.Millisecond)
	if err := tg.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	wg.Wait()
}
