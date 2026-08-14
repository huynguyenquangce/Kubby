package k8sclient

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestEmitLogBatchesFlushesAtLimitAndClose(t *testing.T) {
	lines := make(chan string, 5)
	for _, line := range []string{"a", "b", "c", "d", "e"} {
		lines <- line
	}
	close(lines)

	var got [][]string
	completed := emitLogBatches(context.Background(), lines, 5*time.Millisecond, 3, func(batch []string) {
		got = append(got, batch)
	})
	if !completed {
		t.Fatal("closed input should complete normally")
	}
	want := [][]string{{"a", "b", "c"}, {"d", "e"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("batches = %#v, want %#v", got, want)
	}
}

func TestEmitLogBatchesRateLimitsFullBatches(t *testing.T) {
	lines := make(chan string, 6)
	for _, line := range []string{"a", "b", "c", "d", "e", "f"} {
		lines <- line
	}
	emitted := make(chan []string, 2)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go emitLogBatches(ctx, lines, 30*time.Millisecond, 3, func(batch []string) {
		emitted <- batch
	})

	select {
	case batch := <-emitted:
		t.Fatalf("full batch emitted before interval: %#v", batch)
	case <-time.After(10 * time.Millisecond):
	}

	select {
	case batch := <-emitted:
		if !reflect.DeepEqual(batch, []string{"a", "b", "c"}) {
			t.Fatalf("first batch = %#v", batch)
		}
	case <-time.After(time.Second):
		t.Fatal("full batch was not emitted on interval")
	}
}

func TestEmitLogBatchesFlushesQuietPartialBatch(t *testing.T) {
	lines := make(chan string, 1)
	lines <- "quiet"
	emitted := make(chan []string, 1)
	done := make(chan bool, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		done <- emitLogBatches(ctx, lines, 5*time.Millisecond, 64, func(batch []string) {
			emitted <- batch
		})
	}()

	select {
	case got := <-emitted:
		if !reflect.DeepEqual(got, []string{"quiet"}) {
			t.Fatalf("batch = %#v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("partial batch was not flushed by the interval")
	}
	cancel()
	if completed := <-done; completed {
		t.Fatal("cancelled batcher reported normal completion")
	}
}

func TestEmitLogBatchesDropsBufferedLinesOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	lines := make(chan string, 1)
	lines <- "stale"
	cancel()

	var got [][]string
	completed := emitLogBatches(ctx, lines, time.Hour, 64, func(batch []string) {
		got = append(got, batch)
	})
	if completed {
		t.Fatal("cancelled batcher reported normal completion")
	}
	if len(got) != 0 {
		t.Fatalf("cancelled batcher emitted stale data: %#v", got)
	}
}
