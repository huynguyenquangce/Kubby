package main

import (
	"fmt"
	"strings"

	"kubby/internal/buildinfo"
	"kubby/internal/k8sclient"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// AppVersion is the one-line identity shown in Settings → About and returned to
// the frontend on demand.
func (a *App) AppVersion() string {
	return buildinfo.Get().String()
}

// Diagnostics renders a copy-pasteable report answering the questions that
// otherwise take three messages to establish: which build, which Kubernetes,
// does metrics-server exist, what actually failed.
//
// lastError is whatever error the frontend last showed the user — the app has no
// central error sink, and the error the user *saw* is the one worth reporting.
//
// What is deliberately NOT in here: the AI API key, any kubeconfig content, any
// resource data. The API server endpoint and context name *are* included because
// they are usually essential to the diagnosis; the report is presented with a
// "review before sharing" line so that stays the user's call.
func (a *App) Diagnostics(lastError string) string {
	var b strings.Builder
	info := buildinfo.Get()

	b.WriteString("Kubby diagnostics — review before sharing.\n")
	b.WriteString("Contains no API key, no kubeconfig content and no resource data.\n\n")

	b.WriteString("## App\n")
	row(&b, "version", info.Version)
	row(&b, "commit", orElse(info.Commit, "(not built from a git checkout)"))
	row(&b, "built", orElse(info.Date, "(unknown)"))
	row(&b, "go", info.Go)
	row(&b, "platform", info.Platform)

	b.WriteString("\n## Clusters\n")
	a.stateMu.RLock()
	connected := len(a.clusters)
	a.stateMu.RUnlock()
	_, activeName, cluster, _ := a.activeConnectionSnapshot()
	row(&b, "connected", fmt.Sprintf("%d", connected))
	if cluster == nil {
		row(&b, "active", "(none — not connected)")
	} else {
		row(&b, "active", activeName)
		d := k8sclient.Diagnose(a.ctx, cluster, activeName)
		row(&b, "reachable", yesNo(d.Reachable))
		row(&b, "endpoint", d.Endpoint)
		row(&b, "server", orElse(d.Version, "(unknown)"))
		row(&b, "server platform", orElse(d.Platform, "(unknown)"))
		row(&b, "nodes", countOrRefused(d.Nodes))
		row(&b, "api groups", fmt.Sprintf("%d", d.APIGroups))
		row(&b, "CRD kinds", fmt.Sprintf("%d", d.CRDKinds))
		row(&b, "metrics", d.Metrics)
		if d.FirstError != "" {
			row(&b, "first probe error", d.FirstError)
		}
	}

	// Provider and model only — never the key. See GetAIStatus.
	st := a.GetAIStatus()
	b.WriteString("\n## AI assistant\n")
	if !st.Configured {
		row(&b, "provider", "(not configured)")
	} else {
		row(&b, "provider", st.Label)
		row(&b, "model", st.Model)
		row(&b, "runs locally", yesNo(st.Local))
	}

	b.WriteString("\n## Last error shown\n")
	if strings.TrimSpace(lastError) == "" {
		b.WriteString("  (none this session)\n")
	} else {
		for _, line := range strings.Split(strings.TrimRight(lastError, "\n"), "\n") {
			fmt.Fprintf(&b, "  %s\n", line)
		}
	}
	return b.String()
}

// CopyToClipboard puts text on the system clipboard via the Wails runtime, which
// works regardless of whether the WebView grants the page clipboard permission.
func (a *App) CopyToClipboard(text string) error {
	return wailsruntime.ClipboardSetText(a.ctx, text)
}

func row(b *strings.Builder, label, value string) {
	fmt.Fprintf(b, "  %-16s %s\n", label, value)
}

func orElse(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func countOrRefused(n int) string {
	if n < 0 {
		return "(could not list — no permission?)"
	}
	return fmt.Sprintf("%d", n)
}
