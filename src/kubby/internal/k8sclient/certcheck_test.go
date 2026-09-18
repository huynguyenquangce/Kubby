package k8sclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

var certManagerGVR = schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"}

func tlsSecret(t *testing.T, namespace, name string, notAfter time.Time, annotations map[string]string) *corev1.Secret {
	t.Helper()
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name, Annotations: annotations},
		Type:       corev1.SecretTypeTLS,
		Data: map[string][]byte{
			corev1.TLSCertKey:       testCertificatePEM(t, name+".shop.test", checksTestNow.Add(-60*24*time.Hour), notAfter),
			corev1.TLSPrivateKeyKey: []byte("unique-private-key-sentinel"),
		},
	}
}

func certManagerCertificate(namespace, name, secretName, ready, reason string, renewal time.Time) *unstructured.Unstructured {
	status := map[string]interface{}{
		"conditions": []interface{}{map[string]interface{}{"type": "Ready", "status": ready, "reason": reason, "message": reason + " detail"}},
	}
	if !renewal.IsZero() {
		status["renewalTime"] = renewal.Format(time.RFC3339)
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "cert-manager.io/v1", "kind": "Certificate",
		"metadata": map[string]interface{}{"namespace": namespace, "name": name},
		"spec":     map[string]interface{}{"secretName": secretName},
		"status":   status,
	}}
}

func certificateByName(t *testing.T, report *ClusterChecksReport, source, name string) CertificateCheck {
	t.Helper()
	for _, cert := range report.Certificates.Certificates {
		if cert.Source == source && cert.Name == name {
			return cert
		}
	}
	t.Fatalf("%s %s not reported: %+v", source, name, report.Certificates.Certificates)
	return CertificateCheck{}
}

func TestCertificateChecksGradeExpiryAndCertManagerState(t *testing.T) {
	pinChecksNow(t)
	day := 24 * time.Hour
	opaque := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "opaque"},
		Type:       corev1.SecretTypeOpaque,
		Data:       map[string][]byte{"password": []byte("not-a-certificate")},
	}
	client := kubefake.NewSimpleClientset(
		tlsSecret(t, "shop", "expired", checksTestNow.Add(-2*day-time.Hour), nil),
		tlsSecret(t, "shop", "soon", checksTestNow.Add(3*day), nil),
		tlsSecret(t, "shop", "later", checksTestNow.Add(20*day), nil),
		tlsSecret(t, "shop", "fine", checksTestNow.Add(200*day), nil),
		tlsSecret(t, "shop", "managed", checksTestNow.Add(20*day), map[string]string{certManagerNameAnnotation: "managed"}),
		tlsSecret(t, "shop", "failing", checksTestNow.Add(20*day), nil),
		opaque,
		&netv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "web"},
			Spec:       netv1.IngressSpec{TLS: []netv1.IngressTLS{{Hosts: []string{"shop.test"}, SecretName: "fine"}}},
		},
		&netv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "admin"},
			Spec:       netv1.IngressSpec{TLS: []netv1.IngressTLS{{Hosts: []string{"admin.shop.test"}, SecretName: "nope"}}},
		},
	)
	dynamic := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{certManagerGVR: "CertificateList"},
		certManagerCertificate("shop", "managed", "managed", "True", "Ready", checksTestNow.Add(5*day)),
		certManagerCertificate("shop", "failing", "failing", "False", "IssuerNotReady", time.Time{}),
		certManagerCertificate("shop", "pending", "pending-tls", "False", "DoesNotExist", time.Time{}),
	)
	clientCert := testCertificatePEM(t, "kubernetes-admin", checksTestNow.Add(-day), checksTestNow.Add(10*day))
	cluster := &Cluster{
		Clientset: client, Dynamic: dynamic,
		Rest: &rest.Config{TLSClientConfig: rest.TLSClientConfig{CertData: clientCert}},
		idx: &apiIndex{byKind: map[string][]APIKind{
			"Certificate": {{GVR: certManagerGVR, Kind: "Certificate", Namespaced: true}},
		}},
	}
	report := runChecks(t, cluster, "")

	expired := certificateByName(t, report, "TLS Secret", "expired")
	if expired.Severity != SeverityCritical || expired.Expires != "2d ago" || expired.DaysLeft >= 0 {
		t.Fatalf("expired = %+v", expired)
	}
	if soon := certificateByName(t, report, "TLS Secret", "soon"); soon.Severity != SeverityCritical || soon.Expires != "in 3d" {
		t.Fatalf("soon = %+v", soon)
	}
	if later := certificateByName(t, report, "TLS Secret", "later"); later.Severity != SeverityWarning {
		t.Fatalf("later = %+v", later)
	}
	fine := certificateByName(t, report, "TLS Secret", "fine")
	if fine.Severity != SeverityOK || len(fine.UsedBy) != 1 || fine.UsedBy[0] != "Ingress shop/web" || fine.Subject != "fine.shop.test" {
		t.Fatalf("fine = %+v", fine)
	}
	managed := certificateByName(t, report, "TLS Secret", "managed")
	if managed.Severity != SeverityInfo || managed.ManagedBy != "cert-manager Certificate shop/managed" ||
		!strings.Contains(findingTitles(managed.Findings), "Renewal scheduled") {
		t.Fatalf("a managed certificate with a scheduled renewal is not a warning: %+v", managed)
	}
	failing := certificateByName(t, report, "TLS Secret", "failing")
	if failing.Severity != SeverityCritical || !strings.Contains(findingTitles(failing.Findings), "cert-manager cannot renew it") {
		t.Fatalf("failing renewal = %+v", failing)
	}
	if pending := certificateByName(t, report, "cert-manager Certificate", "pending"); pending.Severity != SeverityWarning || pending.HasCertificate {
		t.Fatalf("an unissued Certificate = %+v", pending)
	}
	admin := certificateByName(t, report, "Ingress TLS", "admin")
	if admin.Severity != SeverityWarning || !strings.Contains(findingTitles(admin.Findings), "TLS Secret nope does not exist") {
		t.Fatalf("missing ingress secret = %+v", admin)
	}
	if kubeconfig := certificateByName(t, report, "Kubeconfig client certificate", "kubernetes-admin"); kubeconfig.Severity != SeverityWarning {
		t.Fatalf("kubeconfig client certificate = %+v", kubeconfig)
	}
	if !report.Certificates.CertManagerInstalled {
		t.Fatal("cert-manager should be detected")
	}
	for _, cert := range report.Certificates.Certificates {
		if cert.Name == "opaque" {
			t.Fatalf("an Opaque Secret is not a TLS certificate: %+v", cert)
		}
		for _, finding := range cert.Findings {
			if strings.Contains(finding.Detail, "unique-private-key-sentinel") {
				t.Fatal("private key material must never reach the report")
			}
		}
	}
	if report.Certificates.Certificates[0].Severity != SeverityCritical {
		t.Fatalf("critical certificates sort first: %+v", report.Certificates.Certificates[0])
	}
}

func TestAPIServerCertificateIsReadOnceAndCached(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	cluster := &Cluster{Rest: &rest.Config{Host: server.URL}}
	cert, err := apiServerCertificate(context.Background(), cluster)
	if err != nil || cert == nil {
		t.Fatalf("certificate=%v err=%v", cert, err)
	}
	server.Close()
	cached, err := apiServerCertificate(context.Background(), cluster)
	if err != nil || cached != cert {
		t.Fatalf("a closed server must be answered from the cache: cert=%v err=%v", cached, err)
	}
}
