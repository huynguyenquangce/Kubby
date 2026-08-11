package k8sclient

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"sigs.k8s.io/yaml"
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

var (
	bearerSecretPattern   = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]+`)
	keyValueSecretPattern = regexp.MustCompile(`(?i)\b(password|passwd|token|api[_-]?key|client[_-]?secret|authorization)\s*[:=]\s*("[^"\r\n]*"|'[^'\r\n]*'|[^\s,;]+)`)
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
			fmt.Fprintf(&b, "- [%s] %s: %s (x%d, %s)\n", e.Type, e.Reason, redactSensitiveText(e.Message), e.Count, e.Age)
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
				trimmed := tailStr(redactSensitiveText(logs), diagLogChars)
				out.LogContainers++
				out.LogLines += strings.Count(strings.TrimRight(trimmed, "\n"), "\n") + 1
				fmt.Fprintf(&b, "## Logs — container %s (recent)\n```\n%s\n```\n\n", ct, trimmed)
			}
		}
	}

	// The spec (from the top of the YAML) plus whatever fits.
	if y, err := GetYAML(ctx, c, kind, namespace, name); err == nil {
		out.HasYAML = true
		fmt.Fprintf(&b, "## Manifest (YAML)\n```yaml\n%s\n```\n", headStr(redactDiagnosticYAML(y), diagYAMLChars))
	}

	out.Text = b.String()
	out.Chars = len(out.Text)
	return out, nil
}

func redactSensitiveText(text string) string {
	text = bearerSecretPattern.ReplaceAllString(text, "Bearer [REDACTED]")
	return keyValueSecretPattern.ReplaceAllStringFunc(text, func(match string) string {
		separator := strings.IndexAny(match, ":=")
		if separator < 0 {
			return "[REDACTED]"
		}
		return strings.TrimSpace(match[:separator]) + "=" + "[REDACTED]"
	})
}

func redactDiagnosticYAML(text string) string {
	var doc map[string]interface{}
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
		return redactSensitiveText(text)
	}
	if kind, _ := doc["kind"].(string); strings.EqualFold(kind, "Secret") {
		for _, field := range []string{"data", "stringData"} {
			if values, ok := doc[field].(map[string]interface{}); ok {
				redacted := make(map[string]interface{}, len(values))
				for key := range values {
					redacted[key] = "[REDACTED]"
				}
				doc[field] = redacted
			}
		}
	}
	redactLiteralEnvValues(doc)
	out, err := yaml.Marshal(doc)
	if err != nil {
		return redactSensitiveText(text)
	}
	return string(out)
}

func redactLiteralEnvValues(value interface{}) {
	switch node := value.(type) {
	case map[string]interface{}:
		for key, child := range node {
			if key == "env" {
				if entries, ok := child.([]interface{}); ok {
					for _, entry := range entries {
						if env, ok := entry.(map[string]interface{}); ok {
							if _, present := env["value"]; present {
								env["value"] = "[REDACTED]"
							}
						}
					}
				}
			}
			redactLiteralEnvValues(child)
		}
	case []interface{}:
		for _, child := range node {
			redactLiteralEnvValues(child)
		}
	}
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
