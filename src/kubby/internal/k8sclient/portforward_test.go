package k8sclient

import (
	"sync"
	"testing"
)

func TestPortForwardSessionCloseIsIdempotentAndConcurrentSafe(t *testing.T) {
	stopCh := make(chan struct{})
	session := &PortForwardSession{stopCh: stopCh}
	var wg sync.WaitGroup

	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			session.Close()
		}()
	}
	wg.Wait()

	select {
	case <-stopCh:
	default:
		t.Fatal("Close() did not close the stop channel")
	}
}
