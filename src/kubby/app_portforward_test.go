package main

import (
	"fmt"
	"sync"
	"testing"
)

func TestListPortForwardsPreservesOriginalTarget(t *testing.T) {
	a := NewApp()
	a.pfSessions["forward-b"] = &portForwardEntry{info: PortForwardInfo{
		Key: "forward-b", Kind: "Service", Namespace: "apps", Name: "web",
		PodName: "web-abc", LocalPort: 49153, RemotePort: 80, KeepRunning: true,
	}}
	a.pfSessions["forward-a"] = &portForwardEntry{info: PortForwardInfo{
		Key: "forward-a", Kind: "Pod", Namespace: "apps", Name: "api",
		PodName: "api-abc", LocalPort: 49152, RemotePort: 8080,
	}}

	got := a.ListPortForwards()
	if len(got) != 2 {
		t.Fatalf("ListPortForwards() returned %d entries, want 2", len(got))
	}
	if got[0].Key != "forward-a" || got[1].Key != "forward-b" {
		t.Fatalf("ListPortForwards() keys = %q, %q; want stable key order", got[0].Key, got[1].Key)
	}
	if got[1].Kind != "Service" || got[1].Name != "web" || !got[1].KeepRunning {
		t.Fatalf("ListPortForwards() lost target metadata: %#v", got[1])
	}
}

func TestStopAllPortForwardsInvalidatesPendingStarts(t *testing.T) {
	a := NewApp()
	a.pfSessions["forward-a"] = &portForwardEntry{info: PortForwardInfo{Key: "forward-a"}}
	before := a.portForwardEpoch()

	a.stopAllPortForwards()

	if got := a.ListPortForwards(); len(got) != 0 {
		t.Fatalf("ListPortForwards() after stop all = %#v, want empty", got)
	}
	if after := a.portForwardEpoch(); after <= before {
		t.Fatalf("port-forward epoch did not advance: before=%d after=%d", before, after)
	}
}

func TestPortForwardRegistryAllowsConcurrentListAndStop(t *testing.T) {
	a := NewApp()
	for i := range 64 {
		key := fmt.Sprintf("forward-%02d", i)
		a.pfSessions[key] = &portForwardEntry{info: PortForwardInfo{Key: key}}
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range 64 {
			_ = a.ListPortForwards()
		}
	}()
	go func() {
		defer wg.Done()
		for i := range 64 {
			a.StopPortForward(fmt.Sprintf("forward-%02d", i))
		}
	}()
	wg.Wait()

	if got := a.ListPortForwards(); len(got) != 0 {
		t.Fatalf("ListPortForwards() after concurrent stop = %#v, want empty", got)
	}
}
