package k8sclient

import (
	"context"
	"io"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

// ExecSession is one interactive exec into a container. Write() feeds stdin;
// Close() ends the session.
type ExecSession struct {
	stdinW io.WriteCloser
	cancel context.CancelFunc
}

// Write forwards user keystrokes to the container's stdin.
func (s *ExecSession) Write(data string) error {
	if s.stdinW == nil {
		return nil
	}
	_, err := s.stdinW.Write([]byte(data))
	return err
}

// Close ends the exec session.
func (s *ExecSession) Close() {
	if s.stdinW != nil {
		s.stdinW.Close()
	}
	if s.cancel != nil {
		s.cancel()
	}
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
func StartExec(ctx context.Context, c *Cluster, namespace, pod, container, shell string, emit func(string), onClose func(error)) (*ExecSession, error) {
	if shell == "" {
		shell = "/bin/sh"
	}

	req := c.Clientset.CoreV1().RESTClient().Post().
		Resource("pods").Namespace(namespace).Name(pod).
		SubResource("exec")

	// TTY is off deliberately: output is rendered in a plain <pre>, so a PTY's
	// ANSI/cursor escape codes would render as garbage. Line-mode gives clean,
	// predictable output. Trade-off: full-screen programs (vi, top) won't work.
	opts := &corev1.PodExecOptions{
		Container: container,
		Command:   []string{shell},
		Stdin:     true,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
	}
	req.VersionedParams(opts, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(c.Rest, "POST", req.URL())
	if err != nil {
		return nil, err
	}

	stdinR, stdinW := io.Pipe()
	execCtx, cancel := context.WithCancel(ctx)
	session := &ExecSession{stdinW: stdinW, cancel: cancel}

	go func() {
		err := exec.StreamWithContext(execCtx, remotecommand.StreamOptions{
			Stdin:  stdinR,
			Stdout: emitWriter{emit: emit},
			Stderr: emitWriter{emit: emit},
			Tty:    false,
		})
		stdinR.Close()
		if onClose != nil {
			onClose(err)
		}
	}()

	return session, nil
}
