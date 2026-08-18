package k8sclient

import (
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"k8s.io/client-go/tools/remotecommand"
)

func TestSelectExecShellPrefersBashThenFallsBack(t *testing.T) {
	var tried []string
	got, err := selectExecShell("auto", func(shell string) error {
		tried = append(tried, shell)
		if shell == "/bin/ash" {
			return nil
		}
		return errors.New("missing")
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "/bin/ash" {
		t.Fatalf("selected shell = %q, want /bin/ash", got)
	}
	want := []string{"/bin/bash", "/usr/bin/bash", "/bin/ash"}
	if !reflect.DeepEqual(tried, want) {
		t.Fatalf("probe order = %#v, want %#v", tried, want)
	}
}

func TestSelectExecShellKeepsExplicitChoiceWithoutProbe(t *testing.T) {
	probes := 0
	got, err := selectExecShell(" /custom/zsh ", func(string) error {
		probes++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "/custom/zsh" || probes != 0 {
		t.Fatalf("explicit selection = %q with %d probes, want /custom/zsh with none", got, probes)
	}
}

func TestSelectExecShellExplainsDistrolessContainer(t *testing.T) {
	_, err := selectExecShell("", func(string) error { return errors.New("not found") })
	if err == nil {
		t.Fatal("missing shells returned nil error")
	}
	for _, shell := range automaticShellCandidates {
		if !strings.Contains(err.Error(), shell) {
			t.Fatalf("error %q does not name attempted shell %q", err, shell)
		}
	}
}

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
