package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestExportedClusterWriteBindingsRequireConnectionOwnership(t *testing.T) {
	typeOfApp := reflect.TypeOf(&App{})
	legacy := []string{
		"DeleteResource",
		"ScaleDeployment",
		"RestartDeployment",
		"RestartStatefulSet",
		"RestartDaemonSet",
		"SetDeploymentPaused",
		"RollbackDeployment",
		"SetNodeSchedulable",
		"DrainNode",
		"RunCronJobNow",
		"HelmRollback",
		"HelmUninstall",
		"HelmUpgradeValues",
		"HelmInstall",
		"HelmTest",
	}
	for _, name := range legacy {
		if _, ok := typeOfApp.MethodByName(name); ok {
			t.Errorf("legacy write binding %s is still exported", name)
		}
		if _, ok := typeOfApp.MethodByName(name + "Owned"); !ok {
			t.Errorf("owned write binding %sOwned is missing", name)
		}
	}
}

func TestOwnedHelmWritesRejectMissingConnectionID(t *testing.T) {
	app := NewApp()
	_, _, _, err := app.beginOwnedHelmOperation("")
	if err == nil || !strings.Contains(err.Error(), "expected connection ID") {
		t.Fatalf("missing Helm ownership error = %v", err)
	}
}
