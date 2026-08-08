package k8sclient

import (
	"context"
	"fmt"
	"strings"
)

// AIContext is the evidence Kubby collects about one resource before asking an
// LLM about it. The counts travel with the text so the UI can tell the user
// exactly what is about to leave their machine — an AI answer is only worth
// trusting if you can see what it was based on.
type AIContext struct {
	Text          string `json:"text"`
	Events        int    `json:"events"`
	LogContainers int    `json:"logContainers"`
	LogLines      int    `json:"logLines"`
	HasYAML       bool   `json:"hasYAML"`
	Chars         int    `json:"chars"`
}

const (
	diagLogTail   = 60
	diagLogChars  = 3000
	diagYAMLChars = 6000
)

// DiagnosticContext assembles a compact text blob (events + logs + YAML) about
// one resource, to feed an LLM for a plain-language explanation of problems.
func DiagnosticContext(ctx context.Context, c *Cluster, kind, namespace, name string) (*AIContext, error) {
	out := &AIContext{}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s: %s", kind, name)
	if namespace != "" {
		fmt.Fprintf(&b, " (namespace: %s)", namespace)
	}
	b.WriteString("\n\n")

	// Events carry the most diagnostic signal (FailedScheduling, BackOff, ...).
	if events, err := ListEvents(ctx, c, kind, namespace, name); err == nil && len(events) > 0 {
		out.Events = len(events)
		b.WriteString("## Events\n")
		for _, e := range events {
			fmt.Fprintf(&b, "- [%s] %s: %s (x%d, %s)\n", e.Type, e.Reason, e.Message, e.Count, e.Age)
		}
		b.WriteString("\n")
	}

	// For pods, include recent logs from each container.
	if kind == "Pod" {
		if containers, err := PodContainers(ctx, c, namespace, name); err == nil {
			for _, ct := range containers {
				logs, err := PodLogs(ctx, c, namespace, name, ct, diagLogTail)
				if err != nil || strings.TrimSpace(logs) == "" {
					continue
				}
				trimmed := tailStr(logs, diagLogChars)
				out.LogContainers++
				out.LogLines += strings.Count(strings.TrimRight(trimmed, "\n"), "\n") + 1
				fmt.Fprintf(&b, "## Logs — container %s (recent)\n```\n%s\n```\n\n", ct, trimmed)
			}
		}
	}

	// The spec (from the top of the YAML) plus whatever fits.
	if y, err := GetYAML(ctx, c, kind, namespace, name); err == nil {
		out.HasYAML = true
		fmt.Fprintf(&b, "## Manifest (YAML)\n```yaml\n%s\n```\n", headStr(y, diagYAMLChars))
	}

	out.Text = b.String()
	out.Chars = len(out.Text)
	return out, nil
}

func headStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n… (truncated)"
}

func tailStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return "… (truncated)\n" + s[len(s)-max:]
}
