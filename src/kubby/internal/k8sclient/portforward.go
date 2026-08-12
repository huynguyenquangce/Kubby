package k8sclient

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sync"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

// PortForwardSession is one active local→pod forward. Close() stops it.
type PortForwardSession struct {
	LocalPort  int
	RemotePort int
	PodName    string
	Namespace  string
	stopCh     chan struct{}
	closeOnce  sync.Once
}

// Close tears down the forward.
func (s *PortForwardSession) Close() {
	s.closeOnce.Do(func() {
		if s.stopCh != nil {
			close(s.stopCh)
		}
	})
}

// resolvePodForForward returns a pod name that can back a forward for the given
// target. For a Pod the name is returned as-is; for a Service the first ready
// pod matching its selector is used.
func resolvePodForForward(ctx context.Context, c *Cluster, kind, namespace, name string) (string, error) {
	switch kind {
	case "Pod":
		return name, nil
	case "Service":
		svc, err := c.Clientset.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return "", err
		}
		if len(svc.Spec.Selector) == 0 {
			return "", fmt.Errorf("service %q has no selector — cannot port-forward", name)
		}
		sel := labels.SelectorFromSet(svc.Spec.Selector).String()
		pods, err := c.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: sel})
		if err != nil {
			return "", err
		}
		for _, p := range pods.Items {
			if p.Status.Phase == corev1.PodRunning {
				return p.Name, nil
			}
		}
		return "", fmt.Errorf("no running pod found behind service %q", name)
	default:
		return "", fmt.Errorf("port-forward not supported for kind %q", kind)
	}
}

// StartPortForward forwards localPort → remotePort of a pod backing the target.
// localPort 0 lets the OS pick a free port. The returned session's LocalPort is
// filled in once the forward is ready; ready is signalled on the returned channel.
func StartPortForward(ctx context.Context, c *Cluster, kind, namespace, name string, localPort, remotePort int) (*PortForwardSession, <-chan struct{}, <-chan error, error) {
	podName, err := resolvePodForForward(ctx, c, kind, namespace, name)
	if err != nil {
		return nil, nil, nil, err
	}

	reqURL := c.Clientset.CoreV1().RESTClient().Post().
		Resource("pods").Namespace(namespace).Name(podName).
		SubResource("portforward").URL()

	transport, upgrader, err := spdy.RoundTripperFor(c.Rest)
	if err != nil {
		return nil, nil, nil, err
	}
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, &url.URL{
		Scheme: reqURL.Scheme, Host: reqURL.Host, Path: reqURL.Path,
	})

	stopCh := make(chan struct{})
	readyCh := make(chan struct{})
	errCh := make(chan error, 1)

	ports := []string{fmt.Sprintf("%d:%d", localPort, remotePort)}
	fw, err := portforward.New(dialer, ports, stopCh, readyCh, nil, nil)
	if err != nil {
		return nil, nil, nil, err
	}

	session := &PortForwardSession{
		RemotePort: remotePort, PodName: podName, Namespace: namespace, stopCh: stopCh,
	}

	go func() {
		// Always send the result (nil on clean Close) so readers unblock.
		errCh <- fw.ForwardPorts()
	}()

	// Fill in the actually-bound local port once ready, then re-signal readiness.
	proxyReady := make(chan struct{})
	go func() {
		select {
		case <-readyCh:
			if p, err := fw.GetPorts(); err == nil && len(p) > 0 {
				session.LocalPort = int(p[0].Local)
			}
			close(proxyReady)
		case <-stopCh:
		case <-ctx.Done():
		}
	}()

	return session, proxyReady, errCh, nil
}
