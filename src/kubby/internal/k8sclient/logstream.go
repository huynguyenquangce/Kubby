package k8sclient

import (
	"bufio"
	"context"

	corev1 "k8s.io/api/core/v1"
)

// StreamLogs follows a container's logs, calling emit for each line until the
// context is cancelled or the stream ends. It uses the no-timeout Stream client
// so a follow does not get killed by the connect timeout.
func StreamLogs(ctx context.Context, c *Cluster, namespace, name, container string, emit func(string)) error {
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

	scanner := bufio.NewScanner(stream)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return nil
		default:
			emit(scanner.Text())
		}
	}
	return scanner.Err()
}
