package k8sclient

import (
	"context"
	"io"
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

// StartExec opens an interactive shell into a container and streams its output
// via the emit callback. It tries the given shell (e.g. "/bin/sh"). The session
// runs until the shell exits or Close() is called.
func StartExec(ctx context.Context, c *Cluster, namespace, pod, container, shell string, cols, rows int, emit func(string), onClose func(error)) (*ExecSession, error) {
	if shell == "" {
		shell = "/bin/sh"
	}

	req := c.Clientset.CoreV1().RESTClient().Post().
		Resource("pods").Namespace(namespace).Name(pod).
		SubResource("exec")

	opts := &corev1.PodExecOptions{
		Container: container,
		Command:   []string{shell},
		Stdin:     true,
		Stdout:    true,
		Stderr:    false, // a TTY carries stderr on the stdout stream
		TTY:       true,
	}
	req.VersionedParams(opts, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(c.Rest, "POST", req.URL())
	if err != nil {
		return nil, err
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

	return session, nil
}
