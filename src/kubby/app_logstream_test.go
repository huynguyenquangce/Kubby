package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"kubby/internal/k8sclient"
)

type capturedLogEvent struct {
	name    string
	payload any
}

func logTestApp() (*App, chan capturedLogEvent) {
	a := NewApp()
	a.storeVerifiedCluster("test", &k8sclient.Cluster{})
	events := make(chan capturedLogEvent, 8)
	a.logEmitter = func(name string, payload any) { events <- capturedLogEvent{name, payload} }
	return a, events
}

func waitLogEvent(t *testing.T, events <-chan capturedLogEvent) capturedLogEvent {
	t.Helper()
	select {
	case event := <-events:
		return event
	case <-time.After(2 * time.Second):
		t.Fatal("log event did not arrive")
		return capturedLogEvent{}
	}
}

func waitLogSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatal("log test synchronization timed out")
	}
}

func TestLogStreamNaturalCompletionAndFailureReleaseOwnership(t *testing.T) {
	for _, tc := range []struct {
		name  string
		err   error
		event string
	}{
		{"EOF", nil, "logclosed"}, {"error", errors.New("synthetic stream failure"), "logerror"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, events := logTestApp()
			a.logStreamer = func(_ context.Context, _ *k8sclient.Cluster, _, _, _ string, emit func([]string)) error {
				emit([]string{"final synthetic line"})
				return tc.err
			}
			if err := a.StartLogStream("test", "pod", "container", "stream-1"); err != nil {
				t.Fatal(err)
			}
			if event := waitLogEvent(t, events); event.name != "loglines" {
				t.Fatalf("first event = %s", event.name)
			}
			event := waitLogEvent(t, events)
			if event.name != tc.event || event.payload.(LogStreamError).StreamID != "stream-1" {
				t.Fatalf("completion = %+v", event)
			}
			a.logMu.Lock()
			active := a.logCancel != nil
			a.logMu.Unlock()
			if active {
				t.Fatal("completed stream still owns cancellation")
			}
		})
	}
}

func TestOldLogStreamCannotCompleteReplacement(t *testing.T) {
	a, events := logTestApp()
	oldStarted, oldRelease, oldDone := make(chan struct{}), make(chan struct{}), make(chan struct{})
	newStarted, newRelease := make(chan struct{}), make(chan struct{})
	var releaseOld, releaseNew sync.Once
	t.Cleanup(func() {
		releaseOld.Do(func() { close(oldRelease) })
		releaseNew.Do(func() { close(newRelease) })
		a.stopAllLogStreams()
	})
	a.logStreamer = func(ctx context.Context, _ *k8sclient.Cluster, _, name, _ string, emit func([]string)) error {
		if name == "old" {
			close(oldStarted)
			<-oldRelease
			emit([]string{"stale"})
			close(oldDone)
			return nil
		}
		close(newStarted)
		select {
		case <-newRelease:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	defer a.stopAllLogStreams()
	if err := a.StartLogStream("test", "old", "", "old-id"); err != nil {
		t.Fatal(err)
	}
	waitLogSignal(t, oldStarted)
	if err := a.StartLogStream("test", "new", "", "new-id"); err != nil {
		t.Fatal(err)
	}
	waitLogSignal(t, newStarted)
	releaseOld.Do(func() { close(oldRelease) })
	waitLogSignal(t, oldDone)
	// The new completion must be the first event. An old late batch/EOF is invalid.
	releaseNew.Do(func() { close(newRelease) })
	event := waitLogEvent(t, events)
	if event.name != "logclosed" || event.payload.(LogStreamError).StreamID != "new-id" {
		t.Fatalf("stale event leaked: %+v", event)
	}
}

func TestStoppedLogStreamDoesNotAnnounceEOF(t *testing.T) {
	a, events := logTestApp()
	started, done := make(chan struct{}), make(chan struct{})
	a.logStreamer = func(ctx context.Context, _ *k8sclient.Cluster, _, _, _ string, _ func([]string)) error {
		close(started)
		<-ctx.Done()
		close(done)
		return nil
	}
	if err := a.StartLogStream("test", "pod", "", "stopped"); err != nil {
		t.Fatal(err)
	}
	waitLogSignal(t, started)
	a.stopAllLogStreams()
	waitLogSignal(t, done)
	// A stopped epoch cannot acquire completion ownership, even if transport returns nil.
	if a.finishLogStream(a.logEpoch - 1) {
		t.Fatal("stopped stream accepted completion")
	}
	select {
	case event := <-events:
		t.Fatalf("stop emitted %+v", event)
	default:
	}
}

func TestStaleLogStopCannotCancelReplacement(t *testing.T) {
	a := NewApp()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a.logCancel = cancel
	a.logStreamID = "replacement"
	a.logEpoch = 2
	a.StopLogStream("old")
	a.StopLogStream("")
	if ctx.Err() != nil || !a.isCurrentLogStream(2) {
		t.Fatal("stale stop cancelled replacement")
	}
	a.StopLogStream("replacement")
	if ctx.Err() == nil || a.isCurrentLogStream(2) {
		t.Fatal("owned stop did not stop stream")
	}
}
