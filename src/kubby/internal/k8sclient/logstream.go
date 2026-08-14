package k8sclient

import (
	"bufio"
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
)

const (
	logBatchInterval = 40 * time.Millisecond
	logBatchSize     = 512
)

// StreamLogs follows a container's logs, emitting small batches until the
// context is cancelled or the stream ends. Batching keeps high-rate streams from
// crossing the Wails bridge and repainting the WebView once per line. It uses the
// no-timeout Stream client so a follow does not get killed by the connect timeout.
func StreamLogs(ctx context.Context, c *Cluster, namespace, name, container string, emit func([]string)) error {
	follow := true
	tail := int64(200)
	opts := &corev1.PodLogOptions{Follow: follow, TailLines: &tail}
	if container != "" {
		opts.Container = container
	}

	stream, err := c.Stream.CoreV1().Pods(namespace).GetLogs(name, opts).Stream(ctx)
	if err != nil {
		return err
	}
	defer stream.Close()

	lines := make(chan string, logBatchSize*2)
	scanDone := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(stream)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			select {
			case lines <- scanner.Text():
			case <-ctx.Done():
				scanDone <- nil
				close(lines)
				return
			}
		}
		scanDone <- scanner.Err()
		close(lines)
	}()

	completed := emitLogBatches(ctx, lines, logBatchInterval, logBatchSize, emit)
	if !completed {
		return nil
	}
	return <-scanDone
}

// emitLogBatches is kept separate from the Kubernetes stream so its timing,
// final flush and bounded batch size can be regression-tested without a cluster.
// It returns false when cancellation made any buffered lines stale.
func emitLogBatches(
	ctx context.Context,
	lines <-chan string,
	interval time.Duration,
	maxBatch int,
	emit func([]string),
) bool {
	if maxBatch < 1 {
		maxBatch = 1
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	batch := make([]string, 0, maxBatch)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		emit(append([]string(nil), batch...))
		batch = batch[:0]
	}

	for {
		// Once a batch is full, stop consuming until the next tick. EventsEmit
		// queues work onto the WebView thread and does not provide backpressure;
		// flushing immediately at every size limit can therefore build an
		// unbounded UI queue for a very noisy Pod.
		input := lines
		if len(batch) >= maxBatch {
			input = nil
		}
		select {
		case <-ctx.Done():
			return false
		case line, ok := <-input:
			if !ok {
				flush()
				return true
			}
			batch = append(batch, line)
		case <-ticker.C:
			flush()
		}
	}
}
