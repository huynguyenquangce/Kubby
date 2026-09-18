package k8sclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// An expired certificate is the most predictable outage there is. This check
// reads every certificate Kubby can see without an agent — TLS Secrets,
// cert-manager Certificates, the kubeconfig's client certificate, the API
// server's serving certificate and webhook CA bundles — and reports days left.
// Only certificate metadata leaves this file; Secret private keys never do.

const (
	certCriticalDays = 7
	certWarningDays  = 30

	certManagerCertificateKind = "Certificate.cert-manager.io"
	certManagerNameAnnotation  = "cert-manager.io/certificate-name"

	apiServerCertTTL     = 10 * time.Minute
	apiServerDialTimeout = 5 * time.Second
)

// CertificateCheck is one certificate, or a reference to one that is missing.
type CertificateCheck struct {
	Source         string         `json:"source"` // "TLS Secret", "cert-manager Certificate", …
	Kind           string         `json:"kind"`   // drawer kind; "" when not a cluster object
	Namespace      string         `json:"namespace"`
	Name           string         `json:"name"`
	HasCertificate bool           `json:"hasCertificate"`
	Subject        string         `json:"subject"`
	DNSNames       []string       `json:"dnsNames"`
	Issuer         string         `json:"issuer"`
	NotAfter       string         `json:"notAfter"`
	DaysLeft       int            `json:"daysLeft"`
	Expires        string         `json:"expires"` // "in 12d" or "3d ago"
	UsedBy         []string       `json:"usedBy"`
	ManagedBy      string         `json:"managedBy"`
	Severity       string         `json:"severity"`
	Findings       []CheckFinding `json:"findings"`
}

type CertificateReport struct {
	Certificates         []CertificateCheck `json:"certificates"`
	Critical             int                `json:"critical"`
	Warning              int                `json:"warning"`
	CertManagerInstalled bool               `json:"certManagerInstalled"`
	Warnings             []string           `json:"warnings"`
}

type certManagerState struct {
	namespace, name string
	secretName      string
	ready           string // "True", "False" or ""
	reason, message string
	renewalTime     time.Time
}

func analyzeCertificates(ctx context.Context, c *Cluster, namespace string, in *checkInputs) CertificateReport {
	report := CertificateReport{Certificates: []CertificateCheck{}, Warnings: []string{}, CertManagerInstalled: in.certManager}
	if in.tlsSecretsErr != nil {
		report.Warnings = append(report.Warnings, fmt.Sprintf("TLS Secrets could not be read: %v", in.tlsSecretsErr))
	}
	if in.ingressesErr != nil {
		report.Warnings = append(report.Warnings, fmt.Sprintf("Ingresses could not be read: %v", in.ingressesErr))
	}
	if in.certsErr != nil {
		report.Warnings = append(report.Warnings, fmt.Sprintf("cert-manager Certificates could not be read: %v", in.certsErr))
	}

	usedBy := map[string][]string{} // namespace/secret → "Ingress ns/name"
	for _, ingress := range in.ingresses {
		if ingress.DeletionTimestamp != nil {
			continue
		}
		for _, tlsEntry := range ingress.Spec.TLS {
			if tlsEntry.SecretName != "" {
				key := ingress.Namespace + "/" + tlsEntry.SecretName
				usedBy[key] = append(usedBy[key], "Ingress "+ingress.Namespace+"/"+ingress.Name)
			}
		}
	}
	bySecret := map[string]certManagerState{}
	for i := range in.certificates {
		state := readCertManagerCertificate(&in.certificates[i])
		bySecret[state.namespace+"/"+state.secretName] = state
	}

	secrets := map[string]bool{}
	for i := range in.tlsSecrets {
		secret := &in.tlsSecrets[i]
		if secret.DeletionTimestamp != nil {
			continue
		}
		key := secret.Namespace + "/" + secret.Name
		secrets[key] = true
		check := CertificateCheck{Source: "TLS Secret", Kind: "Secret", Namespace: secret.Namespace, Name: secret.Name,
			UsedBy: sortedUnique(usedBy[key]), Findings: []CheckFinding{}}
		certs := parsePEMCertificates(secret.Data[corev1.TLSCertKey])
		if len(certs) == 0 {
			check.Findings = append(check.Findings, CheckFinding{SeverityWarning, "tls.crt holds no parseable certificate",
				"Anything serving this Secret fails its TLS handshake."})
		} else {
			describeCertificateChain(&check, certs)
		}
		if name := secret.Annotations[certManagerNameAnnotation]; name != "" {
			check.ManagedBy = "cert-manager Certificate " + secret.Namespace + "/" + name
		}
		if state, ok := bySecret[key]; ok {
			check.ManagedBy = "cert-manager Certificate " + state.namespace + "/" + state.name
			applyCertManagerState(&check, state)
		}
		report.Certificates = append(report.Certificates, finishCertificate(check))
	}

	// A Certificate whose Secret does not exist yet has never been issued.
	for key, state := range bySecret {
		if secrets[key] || in.tlsSecretsErr != nil {
			continue
		}
		check := CertificateCheck{Source: "cert-manager Certificate", Kind: certManagerCertificateKind,
			Namespace: state.namespace, Name: state.name, UsedBy: sortedUnique(usedBy[key]), Findings: []CheckFinding{}}
		severity := SeverityWarning
		if len(check.UsedBy) > 0 {
			severity = SeverityCritical
		}
		detail := fmt.Sprintf("Its Secret %s has not been issued.", state.secretName)
		if state.ready == "False" {
			detail += " " + strings.TrimSpace(state.reason+": "+state.message)
		}
		check.Findings = append(check.Findings, CheckFinding{severity, "Not issued yet", detail})
		report.Certificates = append(report.Certificates, finishCertificate(check))
	}

	// An Ingress naming a TLS Secret that does not exist makes the controller
	// serve its default certificate. Confirm absence with a Get: the Secret may
	// exist with another type, which the TLS-only List does not return.
	if in.tlsSecretsErr == nil {
		reported := map[string]bool{}
		for _, ingress := range in.ingresses {
			if ingress.DeletionTimestamp != nil {
				continue
			}
			for _, tlsEntry := range ingress.Spec.TLS {
				key := ingress.Namespace + "/" + tlsEntry.SecretName
				if tlsEntry.SecretName == "" || secrets[key] || reported[key] {
					continue
				}
				if _, managed := bySecret[key]; managed {
					continue // reported above as not issued yet
				}
				reported[key] = true
				check := CertificateCheck{Source: "Ingress TLS", Kind: "Ingress", Namespace: ingress.Namespace, Name: ingress.Name,
					UsedBy: sortedUnique(usedBy[key]), Findings: []CheckFinding{}}
				secret, err := c.Clientset.CoreV1().Secrets(ingress.Namespace).Get(ctx, tlsEntry.SecretName, metav1.GetOptions{})
				switch {
				case apierrors.IsNotFound(err):
					check.Findings = append(check.Findings, CheckFinding{SeverityWarning, fmt.Sprintf("TLS Secret %s does not exist", tlsEntry.SecretName),
						fmt.Sprintf("The ingress controller serves its default certificate for %s.", strings.Join(tlsEntry.Hosts, ", "))})
				case err != nil:
					continue
				case secret.Type != corev1.SecretTypeTLS:
					check.Findings = append(check.Findings, CheckFinding{SeverityInfo, fmt.Sprintf("Secret %s has type %s", tlsEntry.SecretName, secret.Type),
						"Most ingress controllers expect kubernetes.io/tls; its certificate was not checked."})
				default:
					continue
				}
				report.Certificates = append(report.Certificates, finishCertificate(check))
			}
		}
	}

	if c.Rest != nil && len(c.Rest.TLSClientConfig.CertData) > 0 {
		check := CertificateCheck{Source: "Kubeconfig client certificate", Findings: []CheckFinding{}}
		if certs := parsePEMCertificates(c.Rest.TLSClientConfig.CertData); len(certs) > 0 {
			describeCertificateChain(&check, certs)
			check.Name = certs[0].Subject.CommonName
			check.Findings = append(check.Findings, CheckFinding{SeverityInfo, "Used by this connection",
				"When it expires, Kubby and kubectl with this kubeconfig can no longer authenticate."})
		}
		report.Certificates = append(report.Certificates, finishCertificate(check))
	}
	if cert, err := apiServerCertificate(ctx, c); err != nil {
		report.Warnings = append(report.Warnings, fmt.Sprintf("API server certificate could not be read: %v", err))
	} else if cert != nil {
		check := CertificateCheck{Source: "API server certificate", Name: c.Rest.Host, Findings: []CheckFinding{}}
		describeCertificateChain(&check, []*x509.Certificate{cert})
		report.Certificates = append(report.Certificates, finishCertificate(check))
	}
	for _, spec := range webhookSpecs(in.validating, in.mutating) {
		certs := parsePEMCertificates(spec.client.CABundle)
		if len(certs) == 0 {
			continue
		}
		check := CertificateCheck{Source: "Webhook CA bundle", Kind: spec.configKind, Name: spec.configName,
			UsedBy: []string{"Webhook " + spec.name}, Findings: []CheckFinding{}}
		describeCertificateChain(&check, certs)
		report.Certificates = append(report.Certificates, finishCertificate(check))
	}

	for _, check := range report.Certificates {
		switch check.Severity {
		case SeverityCritical:
			report.Critical++
		case SeverityWarning:
			report.Warning++
		}
	}
	sort.SliceStable(report.Certificates, func(i, j int) bool {
		a, b := report.Certificates[i], report.Certificates[j]
		if checkSeverityRank(a.Severity) != checkSeverityRank(b.Severity) {
			return checkSeverityRank(a.Severity) > checkSeverityRank(b.Severity)
		}
		if a.HasCertificate != b.HasCertificate {
			return !a.HasCertificate
		}
		if a.DaysLeft != b.DaysLeft {
			return a.DaysLeft < b.DaysLeft
		}
		return a.Source+a.Namespace+a.Name < b.Source+b.Namespace+b.Name
	})
	return report
}

// describeCertificateChain fills the leaf's identity and grades the chain by
// whichever certificate expires first — an expired intermediate breaks TLS as
// surely as an expired leaf.
func describeCertificateChain(check *CertificateCheck, certs []*x509.Certificate) {
	leaf := certs[0]
	earliest := leaf
	for _, cert := range certs[1:] {
		if cert.NotAfter.Before(earliest.NotAfter) {
			earliest = cert
		}
	}
	now := checksNow()
	left := earliest.NotAfter.Sub(now)
	check.HasCertificate = true
	check.Subject = leaf.Subject.CommonName
	check.DNSNames = append([]string(nil), leaf.DNSNames...)
	check.Issuer = leaf.Issuer.CommonName
	check.NotAfter = earliest.NotAfter.UTC().Format(time.RFC3339)
	check.DaysLeft = int(left.Hours() / 24)
	if left < 0 {
		check.DaysLeft = -int((-left).Hours()/24) - 1
		check.Expires = durationText(left) + " ago"
	} else {
		check.Expires = "in " + durationText(left)
	}
	chainNote := ""
	if earliest != leaf {
		chainNote = fmt.Sprintf(" The first to expire is %q in the chain, not the leaf.", earliest.Subject.CommonName)
	}
	switch {
	case left <= 0:
		check.Findings = append(check.Findings, CheckFinding{SeverityCritical, "Expired " + check.Expires,
			"Clients that verify it refuse the connection." + chainNote})
	case left <= certCriticalDays*24*time.Hour:
		check.Findings = append(check.Findings, CheckFinding{SeverityCritical, "Expires " + check.Expires,
			"Renew it now." + chainNote})
	case left <= certWarningDays*24*time.Hour:
		check.Findings = append(check.Findings, CheckFinding{SeverityWarning, "Expires " + check.Expires,
			"Plan its renewal." + chainNote})
	}
	if now.Before(leaf.NotBefore) {
		check.Findings = append(check.Findings, CheckFinding{SeverityWarning, "Not valid yet",
			fmt.Sprintf("It becomes valid %s; check the issuer's clock.", leaf.NotBefore.UTC().Format(time.RFC3339))})
	}
}

// applyCertManagerState qualifies an expiry with what cert-manager is doing
// about it. A managed certificate inside its renewal window with a scheduled
// renewal is expected, not a warning; a failing renewal is the real problem.
func applyCertManagerState(check *CertificateCheck, state certManagerState) {
	now := checksNow()
	switch {
	case state.ready == "False":
		severity := SeverityWarning
		if check.HasCertificate && check.DaysLeft <= certWarningDays {
			severity = SeverityCritical
		}
		check.Findings = append(check.Findings, CheckFinding{severity, "cert-manager cannot renew it",
			strings.TrimSpace(strings.Trim(state.reason+": "+state.message, ": "))})
	case state.ready == "True" && !state.renewalTime.IsZero() && state.renewalTime.After(now):
		for i := range check.Findings {
			if check.Findings[i].Severity == SeverityWarning && strings.HasPrefix(check.Findings[i].Title, "Expires") {
				check.Findings[i].Severity = SeverityInfo
			}
		}
		check.Findings = append(check.Findings, CheckFinding{SeverityInfo, "Renewal scheduled",
			fmt.Sprintf("cert-manager renews it at %s.", state.renewalTime.UTC().Format(time.RFC3339))})
	case state.ready == "True" && !state.renewalTime.IsZero() && check.HasCertificate && check.DaysLeft <= certCriticalDays:
		check.Findings = append(check.Findings, CheckFinding{SeverityCritical, "Renewal is overdue",
			fmt.Sprintf("cert-manager was due to renew it at %s.", state.renewalTime.UTC().Format(time.RFC3339))})
	}
}

func readCertManagerCertificate(obj *unstructured.Unstructured) certManagerState {
	state := certManagerState{namespace: obj.GetNamespace(), name: obj.GetName()}
	state.secretName, _, _ = unstructured.NestedString(obj.Object, "spec", "secretName")
	if renewal, _, _ := unstructured.NestedString(obj.Object, "status", "renewalTime"); renewal != "" {
		state.renewalTime, _ = time.Parse(time.RFC3339, renewal)
	}
	conditions, _, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	for _, raw := range conditions {
		condition, ok := raw.(map[string]interface{})
		if !ok || condition["type"] != "Ready" {
			continue
		}
		state.ready, _ = condition["status"].(string)
		state.reason, _ = condition["reason"].(string)
		state.message, _ = condition["message"].(string)
	}
	return state
}

func finishCertificate(check CertificateCheck) CertificateCheck {
	if check.UsedBy == nil {
		check.UsedBy = []string{}
	}
	if check.DNSNames == nil {
		check.DNSNames = []string{}
	}
	check.Severity = worstSeverity(check.Findings)
	return check
}

// parsePEMCertificates returns every certificate in a PEM bundle, skipping
// keys and unparseable blocks.
func parsePEMCertificates(data []byte) []*x509.Certificate {
	certs := []*x509.Certificate{}
	for len(data) > 0 {
		var block *pem.Block
		block, data = pem.Decode(data)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		if cert, err := x509.ParseCertificate(block.Bytes); err == nil {
			certs = append(certs, cert)
		}
	}
	return certs
}

// apiServerCertificate reads the certificate the API server presents. The
// handshake verifies nothing on purpose: it only reads the certificate's dates,
// sends no request and no credentials, and closes. The result is cached per
// connection so live refreshes do not repeat the handshake.
func apiServerCertificate(ctx context.Context, c *Cluster) (*x509.Certificate, error) {
	if c.Rest == nil || c.Rest.Host == "" {
		return nil, nil
	}
	c.apiCertMu.Lock()
	defer c.apiCertMu.Unlock()
	if checksNow().Before(c.apiCertExpires) {
		return c.apiCert, c.apiCertErr
	}
	host := c.Rest.Host
	if !strings.Contains(host, "://") {
		host = "https://" + host
	}
	parsed, err := url.Parse(host)
	if err != nil || parsed.Scheme != "https" {
		return nil, nil
	}
	address := parsed.Host
	if parsed.Port() == "" {
		address = net.JoinHostPort(parsed.Hostname(), "443")
	}
	dialCtx, cancel := context.WithTimeout(ctx, apiServerDialTimeout)
	defer cancel()
	dialer := &tls.Dialer{Config: &tls.Config{
		ServerName:         parsed.Hostname(),
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true, // #nosec G402 -- reads the presented certificate's dates only; nothing is sent
	}}
	var cert *x509.Certificate
	conn, err := dialer.DialContext(dialCtx, "tcp", address)
	if err == nil {
		if peers := conn.(*tls.Conn).ConnectionState().PeerCertificates; len(peers) > 0 {
			cert = peers[0]
		}
		_ = conn.Close()
	}
	c.apiCert, c.apiCertErr, c.apiCertExpires = cert, err, checksNow().Add(apiServerCertTTL)
	return cert, err
}
