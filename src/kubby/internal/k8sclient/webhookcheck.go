package k8sclient

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// A webhook whose backend is gone is one of the few single faults that stop a
// whole cluster: with failurePolicy Fail the API server rejects every request
// the webhook matches, including the Pods that would bring the webhook back.
// The Webhook Doctor checks each webhook's backend and states the blast radius.

// WebhookCheck is one webhook entry of a Validating or Mutating configuration.
type WebhookCheck struct {
	ConfigKind         string         `json:"configKind"`
	Configuration      string         `json:"configuration"`
	Webhook            string         `json:"webhook"`
	FailurePolicy      string         `json:"failurePolicy"`
	TimeoutSeconds     int32          `json:"timeoutSeconds"`
	Target             string         `json:"target"`
	ServiceNamespace   string         `json:"serviceNamespace"`
	ServiceName        string         `json:"serviceName"`
	ReadyEndpoints     int            `json:"readyEndpoints"` // -1 when not applicable or unknown
	Rules              string         `json:"rules"`
	Scope              string         `json:"scope"`
	AffectedNamespaces int            `json:"affectedNamespaces"` // -1 when unknown
	Severity           string         `json:"severity"`
	Findings           []CheckFinding `json:"findings"`
}

type WebhookReport struct {
	Webhooks []WebhookCheck `json:"webhooks"`
	Critical int            `json:"critical"`
	Warning  int            `json:"warning"`
	Warnings []string       `json:"warnings"`
}

const (
	webhookDefaultTimeout = int32(10)
	webhookDefaultPort    = int32(443)
)

type webhookSpec struct {
	configKind        string
	configName        string
	name              string
	client            admissionregistrationv1.WebhookClientConfig
	rules             []admissionregistrationv1.RuleWithOperations
	failurePolicy     admissionregistrationv1.FailurePolicyType
	timeout           int32
	namespaceSelector *metav1.LabelSelector
}

type webhookBackend struct {
	exists         bool
	externalName   bool
	ports          []corev1.ServicePort
	readyEndpoints int // -1 when EndpointSlices could not be read
	err            error
}

type webhookScope struct {
	all   bool
	names map[string]bool
	count int // -1 when unknown
	text  string
}

func analyzeWebhooks(ctx context.Context, c *Cluster, in *checkInputs) WebhookReport {
	report := WebhookReport{Webhooks: []WebhookCheck{}, Warnings: []string{}}
	if in.validatingErr != nil {
		report.Warnings = append(report.Warnings, fmt.Sprintf("ValidatingWebhookConfigurations could not be read: %v", in.validatingErr))
	}
	if in.mutatingErr != nil {
		report.Warnings = append(report.Warnings, fmt.Sprintf("MutatingWebhookConfigurations could not be read: %v", in.mutatingErr))
	}
	backends := map[string]*webhookBackend{}
	for _, spec := range webhookSpecs(in.validating, in.mutating) {
		scope := resolveWebhookScope(spec.namespaceSelector, in.namespaces, in.namespacesErr == nil)
		check := evaluateWebhook(ctx, c, spec, scope, backends)
		switch check.Severity {
		case SeverityCritical:
			report.Critical++
		case SeverityWarning:
			report.Warning++
		}
		report.Webhooks = append(report.Webhooks, check)
	}
	sort.SliceStable(report.Webhooks, func(i, j int) bool {
		a, b := report.Webhooks[i], report.Webhooks[j]
		if checkSeverityRank(a.Severity) != checkSeverityRank(b.Severity) {
			return checkSeverityRank(a.Severity) > checkSeverityRank(b.Severity)
		}
		return a.Configuration+"/"+a.Webhook < b.Configuration+"/"+b.Webhook
	})
	return report
}

// webhookSpecs flattens both configuration kinds, applying the v1 defaults:
// failurePolicy Fail and a 10 second timeout.
func webhookSpecs(validating []admissionregistrationv1.ValidatingWebhookConfiguration, mutating []admissionregistrationv1.MutatingWebhookConfiguration) []webhookSpec {
	specs := []webhookSpec{}
	add := func(kind, config, name string, client admissionregistrationv1.WebhookClientConfig, rules []admissionregistrationv1.RuleWithOperations,
		policy *admissionregistrationv1.FailurePolicyType, timeout *int32, selector *metav1.LabelSelector) {
		spec := webhookSpec{configKind: kind, configName: config, name: name, client: client, rules: rules,
			failurePolicy: admissionregistrationv1.Fail, timeout: webhookDefaultTimeout, namespaceSelector: selector}
		if policy != nil {
			spec.failurePolicy = *policy
		}
		if timeout != nil {
			spec.timeout = *timeout
		}
		specs = append(specs, spec)
	}
	for i := range validating {
		config := &validating[i]
		if config.DeletionTimestamp != nil {
			continue
		}
		for _, hook := range config.Webhooks {
			add("ValidatingWebhookConfiguration", config.Name, hook.Name, hook.ClientConfig, hook.Rules, hook.FailurePolicy, hook.TimeoutSeconds, hook.NamespaceSelector)
		}
	}
	for i := range mutating {
		config := &mutating[i]
		if config.DeletionTimestamp != nil {
			continue
		}
		for _, hook := range config.Webhooks {
			add("MutatingWebhookConfiguration", config.Name, hook.Name, hook.ClientConfig, hook.Rules, hook.FailurePolicy, hook.TimeoutSeconds, hook.NamespaceSelector)
		}
	}
	return specs
}

func evaluateWebhook(ctx context.Context, c *Cluster, spec webhookSpec, scope webhookScope, backends map[string]*webhookBackend) WebhookCheck {
	check := WebhookCheck{
		ConfigKind: spec.configKind, Configuration: spec.configName, Webhook: spec.name,
		FailurePolicy: string(spec.failurePolicy), TimeoutSeconds: spec.timeout,
		ReadyEndpoints: -1, Rules: webhookRulesText(spec.rules),
		Scope: scope.text, AffectedNamespaces: scope.count, Findings: []CheckFinding{},
	}
	failing := spec.failurePolicy != admissionregistrationv1.Ignore
	brokenSeverity := SeverityWarning
	consequence := fmt.Sprintf("Matching requests are admitted without this check, so its policy is silently not enforced, and each can wait up to %ds.", spec.timeout)
	if failing {
		brokenSeverity = SeverityCritical
		consequence = fmt.Sprintf("The API server rejects every matching request (%s) in %s.", check.Rules, scope.text)
	}
	add := func(severity, title, detail string) {
		check.Findings = append(check.Findings, CheckFinding{Severity: severity, Title: title, Detail: detail})
	}

	switch {
	case spec.client.Service != nil:
		ref := spec.client.Service
		port := webhookDefaultPort
		if ref.Port != nil {
			port = *ref.Port
		}
		path := ""
		if ref.Path != nil {
			path = *ref.Path
		}
		check.ServiceNamespace, check.ServiceName = ref.Namespace, ref.Name
		check.Target = fmt.Sprintf("Service %s/%s:%d%s", ref.Namespace, ref.Name, port, path)
		backend := webhookServiceBackend(ctx, c, backends, ref.Namespace, ref.Name)
		switch {
		case backend.err != nil:
			add(SeverityWarning, "Backend Service could not be read", backend.err.Error())
		case !backend.exists:
			add(brokenSeverity, fmt.Sprintf("Service %s/%s does not exist", ref.Namespace, ref.Name), consequence)
		case backend.externalName:
			add(SeverityInfo, "Backend is an ExternalName Service", "Its endpoints live outside the cluster and are not checked.")
		case !servicePortExists(backend.ports, port):
			add(brokenSeverity, fmt.Sprintf("Service %s/%s has no port %d", ref.Namespace, ref.Name, port), consequence)
		case backend.readyEndpoints == 0:
			check.ReadyEndpoints = 0
			add(brokenSeverity, "No ready endpoints behind the Service", consequence)
		default:
			check.ReadyEndpoints = backend.readyEndpoints
		}
		if len(spec.client.CABundle) == 0 {
			add(SeverityWarning, "caBundle is empty",
				"The API server verifies the webhook against its own trusted roots, which usually fails for in-cluster certificates. A CA injector such as cert-manager's cainjector is expected to fill it.")
		}
		if failing && scope.includes(ref.Namespace) && rulesInterceptPodCreation(spec.rules) {
			add(SeverityWarning, "Can block its own recovery",
				fmt.Sprintf("It rejects Pod creation in %s, where its own Service runs: if its Pods are lost while it is unavailable, they cannot be recreated. Exclude that namespace with namespaceSelector.", ref.Namespace))
		}
	case spec.client.URL != nil:
		check.Target = "URL " + webhookURLHost(*spec.client.URL)
		add(SeverityInfo, "External endpoint not checked", "Kubby does not probe webhook URLs outside the cluster from the desktop.")
	}

	if failing && scope.includes(metav1.NamespaceSystem) && rulesInterceptPodCreation(spec.rules) {
		add(SeverityWarning, "Applies to kube-system",
			"An outage blocks Pod creation for control-plane add-ons such as CoreDNS; exclude kube-system unless the webhook must police it.")
	}
	if spec.timeout > webhookDefaultTimeout {
		add(SeverityWarning, fmt.Sprintf("Timeout is %ds", spec.timeout),
			fmt.Sprintf("Every matching API request can wait %ds when the webhook is slow; the default is %ds.", spec.timeout, webhookDefaultTimeout))
	}
	if certs := parsePEMCertificates(spec.client.CABundle); len(certs) > 0 {
		earliest := certs[0]
		for _, cert := range certs[1:] {
			if cert.NotAfter.Before(earliest.NotAfter) {
				earliest = cert
			}
		}
		left := earliest.NotAfter.Sub(checksNow())
		switch {
		case left <= 0:
			add(brokenSeverity, fmt.Sprintf("caBundle certificate expired %s ago", durationText(left)),
				"TLS to the webhook fails. "+consequence)
		case left <= certWarningDays*24*time.Hour:
			add(SeverityWarning, fmt.Sprintf("caBundle certificate expires in %s", durationText(left)),
				"Renew the webhook's CA before it expires, or TLS to the webhook will fail.")
		}
	}
	check.Severity = worstSeverity(check.Findings)
	return check
}

func webhookServiceBackend(ctx context.Context, c *Cluster, cache map[string]*webhookBackend, namespace, name string) *webhookBackend {
	key := namespace + "/" + name
	if backend, ok := cache[key]; ok {
		return backend
	}
	backend := &webhookBackend{readyEndpoints: -1}
	cache[key] = backend
	svc, err := c.Clientset.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
	switch {
	case apierrors.IsNotFound(err):
		return backend
	case err != nil:
		backend.err = err
		return backend
	}
	backend.exists = true
	backend.ports = svc.Spec.Ports
	backend.externalName = svc.Spec.Type == corev1.ServiceTypeExternalName
	slices, err := c.Clientset.DiscoveryV1().EndpointSlices(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: discoveryv1.LabelServiceName + "=" + name,
	})
	if err != nil {
		return backend
	}
	backend.readyEndpoints = 0
	for _, slice := range slices.Items {
		for _, endpoint := range slice.Endpoints {
			if endpoint.Conditions.Ready == nil || *endpoint.Conditions.Ready {
				backend.readyEndpoints++
			}
		}
	}
	return backend
}

func servicePortExists(ports []corev1.ServicePort, port int32) bool {
	for _, candidate := range ports {
		if candidate.Port == port {
			return true
		}
	}
	return false
}

// resolveWebhookScope evaluates a webhook's namespaceSelector against the
// cluster's namespaces. An empty selector matches every namespace.
func resolveWebhookScope(selector *metav1.LabelSelector, namespaces []corev1.Namespace, known bool) webhookScope {
	empty := selector == nil || (len(selector.MatchLabels) == 0 && len(selector.MatchExpressions) == 0)
	if empty {
		return webhookScope{all: true, count: len(namespaces), text: "all namespaces"}
	}
	if !known {
		return webhookScope{count: -1, text: "namespaces matching " + labelSelectorText(selector)}
	}
	parsed := parseLabelSelector(selector)
	scope := webhookScope{names: map[string]bool{}}
	matched := []string{}
	for _, namespace := range namespaces {
		set := labels.Set{}
		for key, value := range namespace.Labels {
			set[key] = value
		}
		if _, ok := set[corev1.LabelMetadataName]; !ok {
			set[corev1.LabelMetadataName] = namespace.Name
		}
		if parsed.Matches(set) {
			scope.names[namespace.Name] = true
			matched = append(matched, namespace.Name)
		}
	}
	sort.Strings(matched)
	scope.count = len(matched)
	switch {
	case len(matched) == 0:
		scope.text = "no namespace (selector " + labelSelectorText(selector) + " matches none)"
	case len(matched) <= 3:
		scope.text = "namespace" + pluralSuffix(len(matched)) + " " + strings.Join(matched, ", ")
	default:
		scope.text = fmt.Sprintf("%d namespaces matching %s", len(matched), labelSelectorText(selector))
	}
	return scope
}

func (scope webhookScope) includes(namespace string) bool {
	return scope.all || scope.names[namespace]
}

// rulesInterceptPodCreation reports whether any rule matches creating core Pods.
func rulesInterceptPodCreation(rules []admissionregistrationv1.RuleWithOperations) bool {
	for _, rule := range rules {
		if !containsAny(operationStrings(rule.Operations), "CREATE", "*") || !containsAny(rule.APIGroups, "", "*") {
			continue
		}
		if containsAny(rule.Resources, "pods", "*", "*/*") {
			return true
		}
	}
	return false
}

func webhookRulesText(rules []admissionregistrationv1.RuleWithOperations) string {
	parts := []string{}
	for _, rule := range rules {
		resources := make([]string, 0, len(rule.Resources))
		for _, resource := range rule.Resources {
			for _, group := range rule.APIGroups {
				switch group {
				case "":
					resources = append(resources, resource)
				case "*":
					resources = append(resources, resource+".*")
				default:
					resources = append(resources, resource+"."+group)
				}
			}
		}
		parts = append(parts, strings.Join(operationStrings(rule.Operations), "/")+" "+strings.Join(sortedUnique(resources), ", "))
	}
	text := strings.Join(parts, "; ")
	if len(text) > 160 {
		text = text[:157] + "…"
	}
	if text == "" {
		return "no rules"
	}
	return text
}

func operationStrings(operations []admissionregistrationv1.OperationType) []string {
	out := make([]string, 0, len(operations))
	for _, operation := range operations {
		out = append(out, string(operation))
	}
	return out
}

func containsAny(values []string, wanted ...string) bool {
	for _, value := range values {
		for _, candidate := range wanted {
			if value == candidate {
				return true
			}
		}
	}
	return false
}

// webhookURLHost shows only the host of an external webhook URL: the path and
// query may carry tokens.
func webhookURLHost(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "(unparseable URL)"
	}
	return parsed.Scheme + "://" + parsed.Host
}

func pluralSuffix(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}
