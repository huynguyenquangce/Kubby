package k8sclient

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

var checksTestNow = time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)

// pinChecksNow fixes the clock the health checks read, for deterministic ages.
func pinChecksNow(t *testing.T) {
	t.Helper()
	previous := checksNow
	checksNow = func() time.Time { return checksTestNow }
	t.Cleanup(func() { checksNow = previous })
}

func testCertificatePEM(t *testing.T, commonName string, notBefore, notAfter time.Time) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: commonName},
		Issuer:       pkix.Name{CommonName: commonName},
		DNSNames:     []string{commonName},
		NotBefore:    notBefore, NotAfter: notAfter,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func podCreateRules() []admissionregistrationv1.RuleWithOperations {
	return []admissionregistrationv1.RuleWithOperations{{
		Operations: []admissionregistrationv1.OperationType{admissionregistrationv1.Create},
		Rule:       admissionregistrationv1.Rule{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"pods"}},
	}}
}

func webhookService(namespace, name string, ports ...int32) *corev1.Service {
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name}}
	for _, port := range ports {
		svc.Spec.Ports = append(svc.Spec.Ports, corev1.ServicePort{Port: port})
	}
	return svc
}

func readyEndpointSlice(namespace, service string) *discoveryv1.EndpointSlice {
	ready := true
	return &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: service + "-abc", Labels: map[string]string{discoveryv1.LabelServiceName: service}},
		Endpoints:  []discoveryv1.Endpoint{{Addresses: []string{"10.0.0.9"}, Conditions: discoveryv1.EndpointConditions{Ready: &ready}}},
	}
}

func webhookByName(t *testing.T, report *ClusterChecksReport, name string) WebhookCheck {
	t.Helper()
	for _, hook := range report.Webhooks.Webhooks {
		if hook.Webhook == name {
			return hook
		}
	}
	t.Fatalf("webhook %s not reported: %+v", name, report.Webhooks.Webhooks)
	return WebhookCheck{}
}

func findingTitles(findings []CheckFinding) string {
	titles := make([]string, 0, len(findings))
	for _, finding := range findings {
		titles = append(titles, finding.Severity+":"+finding.Title)
	}
	return strings.Join(titles, " | ")
}

func runChecks(t *testing.T, cluster *Cluster, namespace string) *ClusterChecksReport {
	t.Helper()
	report, err := ClusterChecks(context.Background(), cluster, namespace)
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func TestWebhookDoctorGradesBackendsByFailurePolicy(t *testing.T) {
	pinChecksNow(t)
	fail, ignore := admissionregistrationv1.Fail, admissionregistrationv1.Ignore
	timeout := int32(30)
	ca := testCertificatePEM(t, "webhook-ca", checksTestNow.Add(-time.Hour), checksTestNow.Add(365*24*time.Hour))
	prod := &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}}
	serviceRef := func(name string) *admissionregistrationv1.ServiceReference {
		return &admissionregistrationv1.ServiceReference{Namespace: "policy-system", Name: name}
	}
	objects := []runtime.Object{
		trafficNamespace("policy-system", nil),
		trafficNamespace("shop", map[string]string{"env": "prod"}),
		trafficNamespace("kube-system", nil),
		webhookService("policy-system", "webhook", 443),
		webhookService("policy-system", "idle", 443),
		webhookService("policy-system", "wrong-port", 8443),
		readyEndpointSlice("policy-system", "webhook"),
		readyEndpointSlice("policy-system", "wrong-port"),
		&admissionregistrationv1.ValidatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{Name: "policy"},
			Webhooks: []admissionregistrationv1.ValidatingWebhook{
				{Name: "missing.policy.dev", ClientConfig: admissionregistrationv1.WebhookClientConfig{Service: serviceRef("gone"), CABundle: ca},
					Rules: podCreateRules(), FailurePolicy: &fail, NamespaceSelector: prod},
				{Name: "healthy.policy.dev", ClientConfig: admissionregistrationv1.WebhookClientConfig{Service: serviceRef("webhook"), CABundle: ca},
					Rules: podCreateRules(), FailurePolicy: &fail, NamespaceSelector: prod},
				{Name: "ignored.policy.dev", ClientConfig: admissionregistrationv1.WebhookClientConfig{Service: serviceRef("idle"), CABundle: ca},
					Rules: podCreateRules(), FailurePolicy: &ignore, NamespaceSelector: prod},
				{Name: "port.policy.dev", ClientConfig: admissionregistrationv1.WebhookClientConfig{Service: serviceRef("wrong-port"), CABundle: ca},
					Rules: podCreateRules(), FailurePolicy: &fail, NamespaceSelector: prod},
			},
		},
		&admissionregistrationv1.MutatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{Name: "injector"},
			Webhooks: []admissionregistrationv1.MutatingWebhook{
				// Defaults to Fail, matches every namespace — including its own.
				{Name: "self.injector.dev", ClientConfig: admissionregistrationv1.WebhookClientConfig{Service: serviceRef("webhook")},
					Rules: podCreateRules(), TimeoutSeconds: &timeout},
			},
		},
	}
	report := runChecks(t, &Cluster{Clientset: kubefake.NewSimpleClientset(objects...)}, "")

	missing := webhookByName(t, report, "missing.policy.dev")
	if missing.Severity != SeverityCritical || missing.AffectedNamespaces != 1 || missing.Scope != "namespace shop" {
		t.Fatalf("missing backend = %+v", missing)
	}
	if !strings.Contains(findingTitles(missing.Findings), "critical:Service policy-system/gone does not exist") ||
		!strings.Contains(missing.Findings[0].Detail, "rejects every matching request (CREATE pods) in namespace shop") {
		t.Fatalf("missing backend findings = %+v", missing.Findings)
	}
	if healthy := webhookByName(t, report, "healthy.policy.dev"); healthy.Severity != SeverityOK || healthy.ReadyEndpoints != 1 {
		t.Fatalf("healthy webhook = %+v", healthy)
	}
	ignored := webhookByName(t, report, "ignored.policy.dev")
	if ignored.Severity != SeverityWarning || !strings.Contains(ignored.Findings[0].Detail, "silently not enforced") {
		t.Fatalf("an Ignore webhook without endpoints is a warning: %+v", ignored)
	}
	if port := webhookByName(t, report, "port.policy.dev"); port.Severity != SeverityCritical ||
		!strings.Contains(findingTitles(port.Findings), "has no port 443") {
		t.Fatalf("a Service without the webhook port = %+v", port)
	}
	self := webhookByName(t, report, "self.injector.dev")
	titles := findingTitles(self.Findings)
	for _, want := range []string{"Can block its own recovery", "Applies to kube-system", "Timeout is 30s", "caBundle is empty"} {
		if !strings.Contains(titles, want) {
			t.Fatalf("self-referential webhook missing %q: %s", want, titles)
		}
	}
	if self.Severity != SeverityWarning || self.Scope != "all namespaces" || self.FailurePolicy != "Fail" {
		t.Fatalf("self-referential webhook = %+v", self)
	}
	if report.Webhooks.Critical != 2 || report.Webhooks.Warning != 2 {
		t.Fatalf("critical=%d warning=%d; want 2 and 2", report.Webhooks.Critical, report.Webhooks.Warning)
	}
	if report.Webhooks.Webhooks[0].Severity != SeverityCritical {
		t.Fatalf("critical webhooks must sort first: %+v", report.Webhooks.Webhooks[0])
	}
}

func TestWebhookDoctorExpiredCABundleBreaksTLS(t *testing.T) {
	pinChecksNow(t)
	expired := testCertificatePEM(t, "old-ca", checksTestNow.Add(-400*24*time.Hour), checksTestNow.Add(-24*time.Hour))
	client := kubefake.NewSimpleClientset(
		trafficNamespace("policy-system", nil),
		webhookService("policy-system", "webhook", 443),
		readyEndpointSlice("policy-system", "webhook"),
		&admissionregistrationv1.ValidatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{Name: "policy"},
			Webhooks: []admissionregistrationv1.ValidatingWebhook{{
				Name:         "expired.policy.dev",
				ClientConfig: admissionregistrationv1.WebhookClientConfig{Service: &admissionregistrationv1.ServiceReference{Namespace: "policy-system", Name: "webhook"}, CABundle: expired},
				Rules:        []admissionregistrationv1.RuleWithOperations{},
			}},
		},
	)
	report := runChecks(t, &Cluster{Clientset: client}, "")
	hook := webhookByName(t, report, "expired.policy.dev")
	if hook.Severity != SeverityCritical || !strings.Contains(findingTitles(hook.Findings), "caBundle certificate expired 24h ago") {
		t.Fatalf("expired CA bundle = %+v", hook)
	}
}
