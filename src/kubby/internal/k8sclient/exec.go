package k8sclient

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

// ExecSession is one interactive exec into a container. Write() feeds stdin;
// Close() ends the session.
type ExecSession struct {
	stdinW    io.WriteCloser
	cancel    context.CancelFunc
	sizes     *terminalSizeQueue
	closeOnce sync.Once
}

// Write forwards user keystrokes to the container's stdin.
func (s *ExecSession) Write(data string) error {
	if s.stdinW == nil {
		return nil
	}
	_, err := s.stdinW.Write([]byte(data))
	return err
}

// Resize updates the remote PTY dimensions. Invalid or out-of-range dimensions
// are ignored rather than allowing a WebView measurement glitch to end a shell.
func (s *ExecSession) Resize(cols, rows int) {
	if s.sizes != nil {
		s.sizes.Resize(cols, rows)
	}
}

// Close ends the exec session.
func (s *ExecSession) Close() {
	s.closeOnce.Do(func() {
		if s.sizes != nil {
			s.sizes.Close()
		}
		if s.stdinW != nil {
			_ = s.stdinW.Close()
		}
		if s.cancel != nil {
			s.cancel()
		}
	})
}

// terminalSizeQueue implements remotecommand.TerminalSizeQueue. A one-item
// buffer keeps only the latest resize while the SPDY stream is busy.
type terminalSizeQueue struct {
	sizes     chan remotecommand.TerminalSize
	done      chan struct{}
	closeOnce sync.Once
}

func newTerminalSizeQueue(cols, rows int) *terminalSizeQueue {
	q := &terminalSizeQueue{
		sizes: make(chan remotecommand.TerminalSize, 1),
		done:  make(chan struct{}),
	}
	q.Resize(cols, rows)
	return q
}

func (q *terminalSizeQueue) Next() *remotecommand.TerminalSize {
	select {
	case <-q.done:
		return nil
	default:
	}
	select {
	case <-q.done:
		return nil
	case size := <-q.sizes:
		return &size
	}
}

func (q *terminalSizeQueue) Resize(cols, rows int) {
	if cols < 1 || rows < 1 || cols > 65535 || rows > 65535 {
		return
	}
	size := remotecommand.TerminalSize{Width: uint16(cols), Height: uint16(rows)}
	select {
	case <-q.done:
		return
	default:
	}
	select {
	case q.sizes <- size:
		return
	default:
	}
	select {
	case <-q.sizes:
	default:
	}
	select {
	case q.sizes <- size:
	case <-q.done:
	}
}

func (q *terminalSizeQueue) Close() {
	q.closeOnce.Do(func() { close(q.done) })
}

// emitWriter turns Write calls into a callback (used to stream output to the UI).
type emitWriter struct{ emit func(string) }

func (w emitWriter) Write(p []byte) (int, error) {
	w.emit(string(p))
	return len(p), nil
}

var automaticShellCandidates = []string{
	"/bin/bash",
	"/usr/bin/bash",
	"/bin/ash",
	"/bin/sh",
}

func selectExecShell(requested string, probe func(string) error) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested != "" && !strings.EqualFold(requested, "auto") {
		return requested, nil
	}
	var lastErr error
	for _, candidate := range automaticShellCandidates {
		if err := probe(candidate); err == nil {
			return candidate, nil
		} else {
			lastErr = err
		}
	}
	return "", fmt.Errorf("no supported shell found (tried %s): %w", strings.Join(automaticShellCandidates, ", "), lastErr)
}

func podExecExecutor(c *Cluster, namespace, pod, container string, command []string, tty bool) (remotecommand.Executor, error) {
	req := c.Clientset.CoreV1().RESTClient().Post().
		Resource("pods").Namespace(namespace).Name(pod).
		SubResource("exec")
	opts := &corev1.PodExecOptions{
		Container: container,
		Command:   command,
		Stdin:     tty,
		Stdout:    true,
		Stderr:    !tty,
		TTY:       tty,
	}
	req.VersionedParams(opts, scheme.ParameterCodec)
	return remotecommand.NewSPDYExecutor(c.Rest, "POST", req.URL())
}

func probeExecShell(ctx context.Context, c *Cluster, namespace, pod, container, shell string) error {
	executor, err := podExecExecutor(c, namespace, pod, container, []string{shell, "-c", "exit 0"}, false)
	if err != nil {
		return err
	}
	return executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: io.Discard,
		Stderr: io.Discard,
	})
}

// StartExec opens an interactive shell into a container and streams its output
// via the emit callback. "auto" probes Bash first, then smaller shell fallbacks;
// an explicit shell is used unchanged. The session runs until the shell exits or
// Close() is called. The resolved shell is returned for truthful UI status.
func StartExec(ctx context.Context, c *Cluster, namespace, pod, container, shell string, cols, rows int, emit func(string), onClose func(error)) (*ExecSession, string, error) {
	resolvedShell, err := selectExecShell(shell, func(candidate string) error {
		return probeExecShell(ctx, c, namespace, pod, container, candidate)
	})
	if err != nil {
		return nil, "", err
	}

	exec, err := podExecExecutor(c, namespace, pod, container, []string{resolvedShell}, true)
	if err != nil {
		return nil, "", err
	}

	stdinR, stdinW := io.Pipe()
	execCtx, cancel := context.WithCancel(ctx)
	sizes := newTerminalSizeQueue(cols, rows)
	session := &ExecSession{stdinW: stdinW, cancel: cancel, sizes: sizes}

	go func() {
		err := exec.StreamWithContext(execCtx, remotecommand.StreamOptions{
			Stdin:             stdinR,
			Stdout:            emitWriter{emit: emit},
			Tty:               true,
			TerminalSizeQueue: sizes,
		})
		_ = stdinR.Close()
		sizes.Close()
		if onClose != nil {
			onClose(err)
		}
	}()

	return session, resolvedShell, nil
}
