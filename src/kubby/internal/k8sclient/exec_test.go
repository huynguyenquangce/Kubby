package k8sclient

import (
	"sync"
	"testing"
	"time"

	"k8s.io/client-go/tools/remotecommand"
)

func TestTerminalSizeQueueStartsWithInitialSizeAndCoalescesResize(t *testing.T) {
	queue := newTerminalSizeQueue(120, 36)
	defer queue.Close()

	initial := queue.Next()
	if initial == nil || initial.Width != 120 || initial.Height != 36 {
		t.Fatalf("initial size = %#v, want 120x36", initial)
	}

	queue.Resize(80, 24)
	queue.Resize(100, 30)
	latest := queue.Next()
	if latest == nil || latest.Width != 100 || latest.Height != 30 {
		t.Fatalf("coalesced size = %#v, want 100x30", latest)
	}
}

func TestTerminalSizeQueueRejectsInvalidDimensions(t *testing.T) {
	queue := newTerminalSizeQueue(80, 24)
	defer queue.Close()
	_ = queue.Next()

	queue.Resize(0, 24)
	queue.Resize(80, 0)

	select {
	case size := <-queue.sizes:
		t.Fatalf("invalid resize was queued: %#v", size)
	default:
	}
}

func TestTerminalSizeQueueCloseIsConcurrentSafeAndUnblocksNext(t *testing.T) {
	queue := newTerminalSizeQueue(80, 24)
	_ = queue.Next()

	done := make(chan *remotecommand.TerminalSize, 1)
	go func() { done <- queue.Next() }()

	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			queue.Close()
		}()
	}
	wg.Wait()

	select {
	case size := <-done:
		if size != nil {
			t.Fatalf("Next after close = %#v, want nil", size)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not unblock Next")
	}
}

func TestTerminalSizeQueueCloseDiscardsPendingResize(t *testing.T) {
	queue := newTerminalSizeQueue(80, 24)
	queue.Close()
	if size := queue.Next(); size != nil {
		t.Fatalf("Next after close = %#v, want nil", size)
	}
}
