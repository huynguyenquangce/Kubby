package main

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"kubby/internal/k8sclient"
)

func TestConnectionsWithSameContextRemainIndependent(t *testing.T) {
	a := NewApp()
	first := &k8sclient.Cluster{}
	second := &k8sclient.Cluster{}
	a.storeVerifiedCluster("default", first)
	a.storeVerifiedCluster("default", second)

	connections := a.ConnectedClusters()
	if len(connections) != 2 {
		t.Fatalf("ConnectedClusters() returned %d entries, want 2", len(connections))
	}
	if connections[0].ID == connections[1].ID {
		t.Fatalf("duplicate contexts share connection ID %q", connections[0].ID)
	}
	if connections[0].Name != "default" || connections[1].Name != "default (2)" {
		t.Fatalf("duplicate context labels = %q, %q", connections[0].Name, connections[1].Name)
	}
	if !connections[1].Active {
		t.Fatal("most recently added connection should be active")
	}

	if err := a.SwitchCluster(connections[0].ID); err != nil {
		t.Fatalf("SwitchCluster(first): %v", err)
	}
	_, _, active, _ := a.activeConnectionSnapshot()
	if active != first {
		t.Fatal("switch by stable ID did not select the first same-named context")
	}

	if got := a.DisconnectCluster(connections[0].ID); got != connections[1].ID {
		t.Fatalf("DisconnectCluster(first) returned %q, want remaining ID %q", got, connections[1].ID)
	}
	_, _, active, _ = a.activeConnectionSnapshot()
	if active != second {
		t.Fatal("disconnecting the first connection removed or replaced the second")
	}
}

func TestConnectionRegistryAllowsConcurrentReadAndSwitch(t *testing.T) {
	a := NewApp()
	a.storeVerifiedCluster("a", &k8sclient.Cluster{})
	a.storeVerifiedCluster("b", &k8sclient.Cluster{})
	connections := a.ConnectedClusters()

	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func(offset int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				_ = a.ConnectedClusters()
				_, _, _, _ = a.activeConnectionSnapshot()
				if err := a.SwitchCluster(connections[(i+offset)%2].ID); err != nil {
					t.Errorf("SwitchCluster: %v", err)
					return
				}
			}
		}(worker)
	}
	wg.Wait()
}

func TestPendingPortForwardCanBeCancelledBeforeReady(t *testing.T) {
	a := NewApp()
	cluster := &k8sclient.Cluster{}
	a.storeVerifiedCluster("a", cluster)
	started := make(chan struct{})
	ready := make(chan struct{})
	errCh := make(chan error, 1)
	a.portForwardStarter = func(context.Context, *k8sclient.Cluster, string, string, string, int, int) (*k8sclient.PortForwardSession, <-chan struct{}, <-chan error, error) {
		close(started)
		return &k8sclient.PortForwardSession{}, ready, errCh, nil
	}

	result := make(chan error, 1)
	go func() {
		_, err := a.StartPortForward("operation-a", "Pod", "default", "api", 0, 8080, false)
		result <- err
	}()
	<-started
	a.CancelPortForwardStart("operation-a")

	if err := <-result; err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("pending StartPortForward returned %v, want cancellation", err)
	}
	if got := a.ListPortForwards(); len(got) != 0 {
		t.Fatalf("cancelled pending forward was registered: %#v", got)
	}
}

func TestClusterSwitchCancelsPendingPortForward(t *testing.T) {
	a := NewApp()
	a.storeVerifiedCluster("a", &k8sclient.Cluster{})
	a.storeVerifiedCluster("b", &k8sclient.Cluster{})
	connections := a.ConnectedClusters()
	if err := a.SwitchCluster(connections[0].ID); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	a.portForwardStarter = func(context.Context, *k8sclient.Cluster, string, string, string, int, int) (*k8sclient.PortForwardSession, <-chan struct{}, <-chan error, error) {
		close(started)
		return &k8sclient.PortForwardSession{}, make(chan struct{}), make(chan error, 1), nil
	}

	result := make(chan error, 1)
	go func() {
		_, err := a.StartPortForward("operation-a", "Pod", "default", "api", 0, 8080, true)
		result <- err
	}()
	<-started
	if err := a.SwitchCluster(connections[1].ID); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("pending StartPortForward returned %v after switch", err)
	}
	if got := a.ListPortForwards(); len(got) != 0 {
		t.Fatalf("old connection forward survived switch: %#v", got)
	}
}

func TestClusterSwitchCancelsHelmOperationsAndOwnershipRejectsStaleWrites(t *testing.T) {
	a := NewApp()
	a.storeVerifiedCluster("a", &k8sclient.Cluster{})
	a.storeVerifiedCluster("b", &k8sclient.Cluster{})
	connections := a.ConnectedClusters()
	if err := a.SwitchCluster(connections[0].ID); err != nil {
		t.Fatal(err)
	}
	ctx, _, done, err := a.beginHelmOperation(connections[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) <= 0 || time.Until(deadline) > helmAppOperationTimeout {
		t.Fatalf("Helm operation deadline = %v, present=%v", deadline, ok)
	}
	if err := a.SwitchCluster(connections[1].ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("cluster switch did not cancel the active Helm operation")
	}
	if _, _, _, err := a.beginHelmOperation(connections[0].ID); err == nil || !strings.Contains(err.Error(), "active cluster changed") {
		t.Fatalf("stale Helm ownership error = %v", err)
	}
}

func TestApplyYAMLOwnedRejectsConnectionSwitch(t *testing.T) {
	a := NewApp()
	a.storeVerifiedCluster("a", &k8sclient.Cluster{})
	a.storeVerifiedCluster("b", &k8sclient.Cluster{})
	connections := a.ConnectedClusters()
	if err := a.SwitchCluster(connections[0].ID); err != nil {
		t.Fatal(err)
	}
	expected := connections[0].ID
	if err := a.SwitchCluster(connections[1].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ApplyYAMLOwned(expected, "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: stale\n"); err == nil || !strings.Contains(err.Error(), "active cluster changed") {
		t.Fatalf("stale apply ownership error = %v", err)
	}
}

func TestLateExecStartCannotSurviveClusterSwitch(t *testing.T) {
	a := NewApp()
	a.storeVerifiedCluster("a", &k8sclient.Cluster{})
	a.storeVerifiedCluster("b", &k8sclient.Cluster{})
	connections := a.ConnectedClusters()
	if err := a.SwitchCluster(connections[0].ID); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	var emit func(string)
	a.execStarter = func(_ context.Context, _ *k8sclient.Cluster, _, _, _, _ string, _, _ int, output func(string), _ func(error)) (*k8sclient.ExecSession, error) {
		emit = output
		close(started)
		<-release
		return &k8sclient.ExecSession{}, nil
	}

	result := make(chan error, 1)
	go func() {
		result <- a.StartExec("exec-a", "default", "api", "", "/bin/sh", 80, 24)
	}()
	<-started
	if err := a.SwitchCluster(connections[1].ID); err != nil {
		t.Fatal(err)
	}
	// A stale callback must be rejected before reaching the Wails runtime. With
	// no runtime context installed, an accidental emit would fail this test.
	emit("stale output")
	close(release)
	if err := <-result; err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("late StartExec returned %v, want cancellation", err)
	}
	a.execMu.Lock()
	defer a.execMu.Unlock()
	if a.execSess != nil {
		t.Fatal("late exec session was installed after cluster switch")
	}
}
