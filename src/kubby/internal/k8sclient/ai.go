package k8sclient

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	// PreviousLogContainers counts restarted containers whose pre-restart logs
	// were attached; their lines are included in LogLines.
	PreviousLogContainers int  `json:"previousLogContainers"`
	HasYAML               bool `json:"hasYAML"`
	Chars                 int  `json:"chars"`
}

const (
	diagLogTail   = 60
	diagLogChars  = 3000
	diagYAMLChars = 6000
	// Previous-instance logs get a smaller budget so a restarted multi-container
	// Pod stays inside the 32 KiB evidence limit App.AskAboutResource enforces.
	diagPreviousLogChars = 2000
	// diagLogConcurrency bounds concurrent log reads for one Pod's evidence.
	diagLogConcurrency = 4
)

var (
	bearerSecretPattern   = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]+`)
	keyValueSecretPattern = regexp.MustCompile(`(?i)\b(password|passwd|token|api[_-]?key|client[_-]?secret|authorization)\s*[:=]\s*("[^"\r\n]*"|'[^'\r\n]*'|[^\s,;]+)`)
	uriSecretPattern      = regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.-]*://)[^/\s:@]+:[^@\s/]+@`)
	sensitiveYAMLKey      = regexp.MustCompile(`(?i)(^|[./_-])(([a-z0-9]+[_-])?(password|passwd|token|secret)|api[_-]?key|client[_-]?secret|authorization|auth|credentials?|private[_-]?key|access[_-]?key|secret[_-]?key|database[_-]?url|db[_-]?url|connection[_-]?string)$`)
	camelKeyBoundary      = regexp.MustCompile(`([a-z0-9])([A-Z])`)
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

	// For pods, include recent logs from each container — and, for a container
	// that has restarted, the instance before the restart, which is where a
	// crash was written while the current instance may still be silent.
	if kind == "Pod" {
		if pod, err := c.Clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{}); err == nil {
			requests := diagnosticLogRequests(pod)
			fetchDiagnosticLogs(ctx, c, namespace, name, requests)
			for _, request := range requests {
				if strings.TrimSpace(request.text) == "" {
					continue
				}
				if request.previous {
					trimmed := tailStr(redactSensitiveText(request.text), diagPreviousLogChars)
					out.PreviousLogContainers++
					out.LogLines += strings.Count(strings.TrimRight(trimmed, "\n"), "\n") + 1
					fmt.Fprintf(&b, "## Logs — container %s (previous instance, before restart %d)\n```\n%s\n```\n\n", request.container, request.restarts, trimmed)
					continue
				}
				trimmed := tailStr(redactSensitiveText(request.text), diagLogChars)
				out.LogContainers++
				out.LogLines += strings.Count(strings.TrimRight(trimmed, "\n"), "\n") + 1
				fmt.Fprintf(&b, "## Logs — container %s (recent)\n```\n%s\n```\n\n", request.container, trimmed)
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

// diagnosticLogRequest is one log read for AI evidence; text is filled by
// fetchDiagnosticLogs and stays empty when the read fails.
type diagnosticLogRequest struct {
	container string
	previous  bool
	restarts  int32
	text      string
}

// diagnosticLogRequests lists the current logs of every container and, for a
// container the kubelet reports as having terminated before its last restart,
// that previous instance — the only case in which previous logs exist.
func diagnosticLogRequests(pod *corev1.Pod) []*diagnosticLogRequest {
	statuses := map[string]corev1.ContainerStatus{}
	for _, status := range pod.Status.ContainerStatuses {
		statuses[status.Name] = status
	}
	requests := []*diagnosticLogRequest{}
	for _, container := range pod.Spec.Containers {
		requests = append(requests, &diagnosticLogRequest{container: container.Name})
		status := statuses[container.Name]
		if status.RestartCount > 0 && status.LastTerminationState.Terminated != nil {
			requests = append(requests, &diagnosticLogRequest{container: container.Name, previous: true, restarts: status.RestartCount})
		}
	}
	return requests
}

// fetchDiagnosticLogs reads the requests concurrently under a small bound, so a
// multi-container Pod that has restarted does not pay one round-trip after
// another. Each goroutine writes only its own request.
func fetchDiagnosticLogs(ctx context.Context, c *Cluster, namespace, name string, requests []*diagnosticLogRequest) {
	semaphore := make(chan struct{}, diagLogConcurrency)
	var wg sync.WaitGroup
	for _, request := range requests {
		wg.Add(1)
		go func(request *diagnosticLogRequest) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			if text, err := PodLogs(ctx, c, namespace, name, request.container, diagLogTail, request.previous); err == nil {
				request.text = text
			}
		}(request)
	}
	wg.Wait()
}

func redactSensitiveText(text string) string {
	text = bearerSecretPattern.ReplaceAllString(text, "Bearer [REDACTED]")
	text = uriSecretPattern.ReplaceAllString(text, `${1}[REDACTED]@`)
	return keyValueSecretPattern.ReplaceAllStringFunc(text, func(match string) string {
		separator := strings.IndexAny(match, ":=")
		if separator < 0 {
			return "[REDACTED]"
		}
		return strings.TrimSpace(match[:separator]) + "=" + "[REDACTED]"
	})
}

// RedactSensitiveText removes common credential shapes from free-form text that
// may be copied into diagnostics or sent outside the process.
func RedactSensitiveText(text string) string {
	return redactSensitiveText(text)
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
	redactSensitiveYAMLValues(doc)
	out, err := yaml.Marshal(doc)
	if err != nil {
		return redactSensitiveText(text)
	}
	// Defense in depth for credential-shaped strings embedded in fields whose
	// key is not itself sensitive (for example a database URL in `endpoint`).
	return redactSensitiveText(string(out))
}

// redactSensitiveYAMLValues walks arbitrary manifests, including ConfigMaps and
// custom resources. Kubernetes schemas cannot enumerate every credential field,
// so sensitive-looking keys are default-deny while ordinary string values still
// receive the same text redaction used for logs and events.
func redactSensitiveYAMLValues(value interface{}) {
	switch node := value.(type) {
	case map[string]interface{}:
		for key, child := range node {
			normalizedKey := camelKeyBoundary.ReplaceAllString(key, `${1}_${2}`)
			if sensitiveYAMLKey.MatchString(normalizedKey) {
				node[key] = "[REDACTED]"
				continue
			}
			switch typed := child.(type) {
			case string:
				node[key] = redactSensitiveText(typed)
			default:
				redactSensitiveYAMLValues(child)
			}
		}
	case []interface{}:
		for _, child := range node {
			redactSensitiveYAMLValues(child)
		}
	}
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
