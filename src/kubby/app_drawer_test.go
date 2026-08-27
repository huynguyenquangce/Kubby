package main

import (
	"testing"
	"time"

	"kubby/internal/k8sclient"
)

func TestDrawerSnapshotCancellationOwnsExactOperation(t *testing.T) {
	app := NewApp()
	ctxA, doneA, err := app.beginDrawerSnapshot("drawer-a")
	if err != nil {
		t.Fatal(err)
	}
	defer doneA()
	ctxB, doneB, err := app.beginDrawerSnapshot("drawer-b")
	if err != nil {
		t.Fatal(err)
	}
	defer doneB()

	app.CancelDrawerSnapshot("drawer-a")
	select {
	case <-ctxA.Done():
	case <-time.After(time.Second):
		t.Fatal("cancel did not reach the matching drawer operation")
	}
	select {
	case <-ctxB.Done():
		t.Fatal("canceling drawer-a also canceled drawer-b")
	default:
	}
}

func TestDrawerSnapshotOperationIDsAreRequiredAndUnique(t *testing.T) {
	app := NewApp()
	if _, _, err := app.beginDrawerSnapshot(""); err == nil {
		t.Fatal("empty drawer operation ID was accepted")
	}
	_, done, err := app.beginDrawerSnapshot("same")
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	if _, _, err := app.beginDrawerSnapshot("same"); err == nil {
		t.Fatal("duplicate in-flight drawer operation ID was accepted")
	}
}

func TestCompletedCanceledDrawerDoesNotReleaseReusedOperationID(t *testing.T) {
	app := NewApp()
	_, finishOld, err := app.beginDrawerSnapshot("reused")
	if err != nil {
		t.Fatal(err)
	}
	app.CancelDrawerSnapshot("reused")
	ctxNew, finishNew, err := app.beginDrawerSnapshot("reused")
	if err != nil {
		t.Fatal(err)
	}
	defer finishNew()

	finishOld()
	if _, _, err := app.beginDrawerSnapshot("reused"); err == nil {
		t.Fatal("old request cleanup released the replacement operation")
	}
	select {
	case <-ctxNew.Done():
		t.Fatal("old request cleanup canceled the replacement operation")
	default:
	}
}

func TestClusterTransitionCancelsDrawerSnapshots(t *testing.T) {
	app := NewApp()
	ctx, done, err := app.beginDrawerSnapshot("old-cluster")
	if err != nil {
		t.Fatal(err)
	}
	defer done()

	app.storeVerifiedCluster("next", &k8sclient.Cluster{})
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("cluster transition did not cancel the old drawer snapshot")
	}
}
