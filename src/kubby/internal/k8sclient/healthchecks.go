package k8sclient

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Health checks answer three questions an operator otherwise discovers only
// when something breaks: is an admission webhook about to reject requests
// cluster-wide, is a certificate about to expire, and why is an object stuck
// Terminating. Each check is read-only and derived; nothing here writes.

const (
	SeverityCritical = "critical"
	SeverityWarning  = "warning"
	SeverityInfo     = "info"
	SeverityOK       = "ok"

	// checksCacheTTL bounds how often a live refresh repeats the scan. The
	// stuck-deletion check lists every resource type the cluster serves, which
	// is too much to repeat every five seconds.
	checksCacheTTL = 15 * time.Second
)

// checksNow is replaceable so tests can pin certificate and deletion ages.
var checksNow = time.Now

// CheckFinding is one explained problem or note about a checked object.
type CheckFinding struct {
	Severity string `json:"severity"`
	Title    string `json:"title"`
	Detail   string `json:"detail"`
}

// ClusterChecksReport is the Health checks screen, gathered in one call.
type ClusterChecksReport struct {
	Scope        string            `json:"scope"` // "" = all namespaces
	CheckedAt    string            `json:"checkedAt"`
	Webhooks     WebhookReport     `json:"webhooks"`
	Certificates CertificateReport `json:"certificates"`
	Stuck        StuckReport       `json:"stuck"`
}

type checksCacheEntry struct {
	report  *ClusterChecksReport
	expires time.Time
}

// checkInputs are the Lists the checks share, read concurrently once.
type checkInputs struct {
	namespaces    []corev1.Namespace
	namespacesErr error
	validating    []admissionregistrationv1.ValidatingWebhookConfiguration
	validatingErr error
	mutating      []admissionregistrationv1.MutatingWebhookConfiguration
	mutatingErr   error
	tlsSecrets    []corev1.Secret
	tlsSecretsErr error
	ingresses     []netv1.Ingress
	ingressesErr  error
	certificates  []unstructured.Unstructured
	certManager   bool
	certsErr      error
}

// ClusterChecks runs every health check for a namespace ("" = all). Results
// are cached per connection and scope for a short time, and concurrent callers
// share one scan rather than each starting another.
func ClusterChecks(ctx context.Context, c *Cluster, namespace string) (*ClusterChecksReport, error) {
	c.checksMu.Lock()
	defer c.checksMu.Unlock()
	if entry, ok := c.checksCache[namespace]; ok && checksNow().Before(entry.expires) {
		return entry.report, nil
	}
	report := buildClusterChecks(ctx, c, namespace)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.checksCache == nil {
		c.checksCache = map[string]checksCacheEntry{}
	}
	c.checksCache[namespace] = checksCacheEntry{report: report, expires: checksNow().Add(checksCacheTTL)}
	return report, nil
}

func buildClusterChecks(ctx context.Context, c *Cluster, namespace string) *ClusterChecksReport {
	report := &ClusterChecksReport{Scope: namespace, CheckedAt: checksNow().UTC().Format(time.RFC3339)}
	in := &checkInputs{}
	var wg sync.WaitGroup
	sem := make(chan struct{}, countConcurrency)
	run := func(task func()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			task()
		}()
	}
	run(func() {
		if list, err := c.Clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{}); err == nil {
			in.namespaces = list.Items
		} else {
			in.namespacesErr = err
		}
	})
	run(func() {
		if list, err := c.Clientset.AdmissionregistrationV1().ValidatingWebhookConfigurations().List(ctx, metav1.ListOptions{}); err == nil {
			in.validating = list.Items
		} else {
			in.validatingErr = err
		}
	})
	run(func() {
		if list, err := c.Clientset.AdmissionregistrationV1().MutatingWebhookConfigurations().List(ctx, metav1.ListOptions{}); err == nil {
			in.mutating = list.Items
		} else {
			in.mutatingErr = err
		}
	})
	run(func() {
		// The field selector keeps other Secrets' data off the wire. The type is
		// checked again below because not every client honours field selectors.
		list, err := c.Clientset.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{FieldSelector: "type=" + string(corev1.SecretTypeTLS)})
		if err != nil {
			in.tlsSecretsErr = err
			return
		}
		for i := range list.Items {
			if list.Items[i].Type == corev1.SecretTypeTLS {
				in.tlsSecrets = append(in.tlsSecrets, list.Items[i])
			}
		}
	})
	run(func() {
		if list, err := c.Clientset.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{}); err == nil {
			in.ingresses = list.Items
		} else {
			in.ingressesErr = err
		}
	})
	run(func() {
		if c.Dynamic == nil {
			return
		}
		kind, installed, err := optionalKind(c, certManagerCertificateKind)
		if err != nil || !installed {
			in.certsErr = err
			return
		}
		in.certManager = true
		list, err := c.Dynamic.Resource(kind.GVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			in.certsErr = err
			return
		}
		in.certificates = list.Items
	})
	run(func() { report.Stuck = scanStuckObjects(ctx, c, namespace) })
	wg.Wait()

	report.Webhooks = analyzeWebhooks(ctx, c, in)
	report.Certificates = analyzeCertificates(ctx, c, namespace, in)
	return report
}

func checkSeverityRank(severity string) int {
	switch severity {
	case SeverityCritical:
		return 3
	case SeverityWarning:
		return 2
	case SeverityInfo:
		return 1
	}
	return 0
}

// worstSeverity is the highest severity among findings, ok when there are none.
func worstSeverity(findings []CheckFinding) string {
	worst := SeverityOK
	for _, finding := range findings {
		if checkSeverityRank(finding.Severity) > checkSeverityRank(worst) {
			worst = finding.Severity
		}
	}
	return worst
}

// durationText renders a positive duration the way the rest of Kubby renders
// ages: 45s, 12m, 5h, 3d.
func durationText(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// kubectlResourceArg names a resource the way kubectl accepts it: the lower-case
// kind, qualified by its API group outside the core group.
func kubectlResourceArg(ak APIKind) string {
	if ak.GVR.Group == "" {
		return strings.ToLower(ak.Kind)
	}
	return strings.ToLower(ak.Kind) + "." + ak.GVR.Group
}

func sortedUnique(values []string) []string {
	set := map[string]bool{}
	for _, value := range values {
		if value != "" {
			set[value] = true
		}
	}
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
