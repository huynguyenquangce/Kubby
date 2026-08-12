package main

import (
	"strings"
	"testing"

	"kubby/internal/buildinfo"
	"kubby/internal/k8sclient"
)

// The diagnostics report exists to be pasted into a bug report, so the two things
// worth pinning down are that it stays readable and that it never carries a
// secret. Both are checked here against the not-connected path, which needs no
// cluster and no Wails runtime.

func TestDiagnosticsReportShape(t *testing.T) {
	a := &App{clusters: nil}
	report := a.Diagnostics("boom: something failed at 10:00")

	for _, want := range []string{
		"review before sharing",
		"## App",
		"version",
		buildinfo.Get().Version,
		"## Clusters",
		"(none — not connected)",
		"## AI assistant",
		"## Last error shown",
		"boom: something failed at 10:00",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("report is missing %q\n--- report ---\n%s", want, report)
		}
	}
}

func TestUpdateYAMLRejectsChangedActiveCluster(t *testing.T) {
	a := NewApp()
	a.clusters["connection-b"] = &clusterEntry{
		id: "connection-b", name: "cluster-b", context: "cluster-b", cluster: &k8sclient.Cluster{},
	}
	a.order = []string{"connection-b"}
	a.activeID = "connection-b"
	err := a.UpdateYAML("cluster-a", "ConfigMap", "default", "settings", "ignored")
	if err == nil || !strings.Contains(err.Error(), "refusing stale YAML update") {
		t.Fatalf("expected stale-cluster rejection, got %v", err)
	}
}

func TestDiagnosticsNeverIncludesTheAPIKey(t *testing.T) {
	// GetAIStatus is the only AI source the report reads, and it is documented as
	// never returning the key. Assert it here too: this report is the one place a
	// leak would be pasted into a public issue.
	cfg := loadAIConfig()
	if strings.TrimSpace(cfg.APIKey) == "" {
		t.Skip("no API key configured on this machine — nothing to leak")
	}
	report := (&App{}).Diagnostics("")
	if strings.Contains(report, cfg.APIKey) {
		t.Fatal("the diagnostics report contains the AI API key")
	}
}

func TestDiagnosticsReportsMissingErrorHonestly(t *testing.T) {
	report := (&App{}).Diagnostics("   ")
	if !strings.Contains(report, "(none this session)") {
		t.Errorf("a blank last-error should say so explicitly\n--- report ---\n%s", report)
	}
}

func TestCountOrRefused(t *testing.T) {
	// -1 means the list was refused, which must not render as a count.
	if got := countOrRefused(-1); !strings.Contains(got, "no permission") {
		t.Errorf("countOrRefused(-1) = %q, want it to explain the refusal", got)
	}
	if got := countOrRefused(0); got != "0" {
		t.Errorf("countOrRefused(0) = %q, want %q — zero nodes is a fact, not an error", got, "0")
	}
}

func TestOrElse(t *testing.T) {
	cases := []struct{ in, fallback, want string }{
		{"v1.36.1", "(unknown)", "v1.36.1"},
		{"", "(unknown)", "(unknown)"},
		{"   ", "(unknown)", "(unknown)"}, // whitespace is not a value
	}
	for _, c := range cases {
		if got := orElse(c.in, c.fallback); got != c.want {
			t.Errorf("orElse(%q, %q) = %q, want %q", c.in, c.fallback, got, c.want)
		}
	}
}
