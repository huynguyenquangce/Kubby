package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"kubby/internal/buildinfo"
)

func TestWailsProductVersionMatchesSourceReleaseVersion(t *testing.T) {
	payload, err := os.ReadFile("wails.json")
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Info struct {
			ProductVersion string `json:"productVersion"`
		} `json:"info"`
	}
	if err := json.Unmarshal(payload, &config); err != nil {
		t.Fatalf("parse wails.json: %v", err)
	}
	if !strings.HasSuffix(buildinfo.Version, "-dev") {
		t.Fatalf("checked-in buildinfo.Version %q must retain the -dev suffix", buildinfo.Version)
	}
	want := strings.TrimSuffix(buildinfo.Version, "-dev")
	if config.Info.ProductVersion != want {
		t.Fatalf("wails productVersion = %q, want %q from buildinfo.Version", config.Info.ProductVersion, want)
	}
}
