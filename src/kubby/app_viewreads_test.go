package main

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"kubby/internal/k8sclient"
)

// View reads are abandoned when the screen that asked for them is gone. The
// rules that make that safe: only reads register, a write never does, and a
// cancel reaches the reads in flight without touching later ones.

func TestCancelViewReadsCancelsTheReadsInFlight(t *testing.T) {
	app := NewApp()
	ctxA, doneA := app.beginViewRead()
	defer doneA()
	ctxB, doneB := app.beginViewRead()
	defer doneB()

	if cancelled := app.CancelViewReads(); cancelled != 2 {
		t.Fatalf("CancelViewReads cancelled %d reads, want 2", cancelled)
	}
	for name, ctx := range map[string]context.Context{"first": ctxA, "second": ctxB} {
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
			t.Fatalf("the %s view read was not cancelled", name)
		}
	}
	// A read that started afterwards belongs to the new screen and must survive.
	ctxNext, doneNext := app.beginViewRead()
	defer doneNext()
	if cancelled := app.CancelViewReads(); cancelled != 1 {
		t.Fatalf("a finished read is not there to cancel: got %d", cancelled)
	}
	select {
	case <-ctxNext.Done():
	case <-time.After(time.Second):
		t.Fatal("the following read was not cancelled by its own call")
	}
}

func TestFinishedViewReadIsForgotten(t *testing.T) {
	app := NewApp()
	_, done := app.beginViewRead()
	done()
	if cancelled := app.CancelViewReads(); cancelled != 0 {
		t.Fatalf("a completed read was still registered: %d", cancelled)
	}
	app.viewReadMu.Lock()
	remaining := len(app.viewReads)
	app.viewReadMu.Unlock()
	if remaining != 0 {
		t.Fatalf("the registry leaks finished reads: %d left", remaining)
	}
}

func TestViewReadCarriesACancellableContextIntoTheCall(t *testing.T) {
	app := NewApp()
	app.ctx = context.Background()
	cluster := &k8sclient.Cluster{}
	app.stateMu.Lock()
	app.clusters["c1"] = &clusterEntry{id: "c1", name: "test", cluster: cluster}
	app.activeID = "c1"
	app.stateMu.Unlock()

	started := make(chan context.Context, 1)
	release := make(chan struct{})
	var result error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, result = withViewCluster(app, func(ctx context.Context, _ *k8sclient.Cluster) (int, error) {
			started <- ctx
			<-release
			return 0, ctx.Err()
		})
	}()

	ctx := <-started
	app.CancelViewReads()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("the context handed to the call was not cancelled")
	}
	close(release)
	wg.Wait()
	if !errors.Is(result, context.Canceled) {
		t.Fatalf("the call should report the cancellation, got %v", result)
	}
}

// A screen may be abandoned; a write the user confirmed may not. Writes take
// the owned helpers, which hand the call a.ctx and register nothing.
func TestWritesAreNotCancellableViewReads(t *testing.T) {
	app := NewApp()
	app.ctx = context.Background()
	cluster := &k8sclient.Cluster{}
	app.stateMu.Lock()
	app.clusters["c1"] = &clusterEntry{id: "c1", name: "test", cluster: cluster}
	app.activeID = "c1"
	app.stateMu.Unlock()

	inside := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- withOwnedClusterErr(app, "c1", func(ctx context.Context, _ *k8sclient.Cluster) error {
			close(inside)
			time.Sleep(50 * time.Millisecond)
			return ctx.Err()
		})
	}()
	<-inside
	if cancelled := app.CancelViewReads(); cancelled != 0 {
		t.Fatalf("a write registered itself as a cancellable view read: %d", cancelled)
	}
	if err := <-done; err != nil {
		t.Fatalf("the write was disturbed: %v", err)
	}
}

// Every list/snapshot binding a screen calls must be cancellable; if a new one
// is added with the plain helper it silently keeps the old behaviour, so the
// source is checked rather than trusted.
func TestScreenReadsUseTheCancellableHelper(t *testing.T) {
	source := readAppSource(t)
	viewBindings := []string{
		"ListNodes", "ListPods", "PodsSnapshot", "PodsPage",
		"ListDeployments", "ListServices", "ListConfigMaps", "ListSecrets",
		"ListStatefulSets", "ListDaemonSets", "ListJobs", "ListCronJobs", "ListIngresses",
		"ListPVCs", "ListServiceAccounts", "ListPersistentVolumes", "ListStorageClasses",
		"ListRoles", "ListRoleBindings", "ListClusterRoles", "ListClusterRoleBindings",
		"ListCRDs", "ListResourceQuotas", "ListLimitRanges",
		"ListHorizontalPodAutoscalers", "ListPodDisruptionBudgets", "ListNetworkPolicies",
		"ClusterChecks", "ClusterHygiene", "OverviewSnapshot", "ListCustom", "ListCustomPage",
		"NetworkFlows", "ClusterStructure", "Sizing", "ListHelmReleases",
	}
	for _, name := range viewBindings {
		body := methodBody(t, source, name)
		if !strings.Contains(body, "withViewCluster(a,") {
			t.Errorf("%s is a screen read and should use withViewCluster", name)
		}
	}
	// The counterexamples: a write must never be abandoned, and neither may a
	// read that fills shared chrome — the namespace picker belongs to no screen,
	// and cancelling its list left namespaces created after connect invisible.
	for _, name := range []string{"ScaleDeploymentOwned", "DeleteResourceOwned", "DrainNodeOwned", "ListNamespaces", "SidebarCounts"} {
		if strings.Contains(methodBody(t, source, name), "withViewCluster(a,") {
			t.Errorf("%s must not be a cancellable view read", name)
		}
	}
}

func readAppSource(t *testing.T) string {
	t.Helper()
	source, err := os.ReadFile("app.go")
	if err != nil {
		t.Fatalf("read app.go: %v", err)
	}
	return string(source)
}

// methodBody returns one App method's source, from its signature to the next
// top-level declaration.
func methodBody(t *testing.T, source, name string) string {
	t.Helper()
	start := strings.Index(source, "func (a *App) "+name+"(")
	if start < 0 {
		t.Fatalf("App method %s not found in app.go", name)
	}
	rest := source[start:]
	if end := strings.Index(rest[1:], "\nfunc "); end >= 0 {
		return rest[:end+1]
	}
	return rest
}

func TestCancelViewReadsIsBound(t *testing.T) {
	if _, ok := reflect.TypeOf(&App{}).MethodByName("CancelViewReads"); !ok {
		t.Fatal("CancelViewReads must stay exported: the frontend calls it on every view change")
	}
}
