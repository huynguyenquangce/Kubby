package k8sclient

import (
	"strings"
	"testing"
)

func TestRedactDiagnosticYAMLRemovesSecretValues(t *testing.T) {
	input := `apiVersion: v1
kind: Secret
metadata:
  name: credentials
data:
  password: dW5pcXVlLXNlbnRpbmVs
stringData:
  token: unique-sentinel-token
`
	got := redactDiagnosticYAML(input)
	for _, secret := range []string{"dW5pcXVlLXNlbnRpbmVs", "unique-sentinel-token"} {
		if strings.Contains(got, secret) {
			t.Fatalf("redacted Secret still contains %q:\n%s", secret, got)
		}
	}
	if !strings.Contains(got, "password") || !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("redaction should preserve keys and show that values were removed:\n%s", got)
	}
}

func TestRedactDiagnosticYAMLRemovesLiteralEnvValues(t *testing.T) {
	input := `apiVersion: v1
kind: Pod
spec:
  containers:
    - name: app
      env:
        - name: PASSWORD
          value: unique-sentinel-password
        - name: FROM_SECRET
          valueFrom:
            secretKeyRef:
              name: credentials
              key: password
`
	got := redactDiagnosticYAML(input)
	if strings.Contains(got, "unique-sentinel-password") {
		t.Fatalf("redacted Pod still contains the literal env value:\n%s", got)
	}
	if !strings.Contains(got, "secretKeyRef") {
		t.Fatalf("redaction should preserve non-literal secret references:\n%s", got)
	}
}

func TestRedactSensitiveTextRemovesCommonCredentialShapes(t *testing.T) {
	got := redactSensitiveText(`Authorization: Bearer unique.jwt.token password=unique-password api_key: unique-key client_secret="unique secret with spaces" postgres://dbuser:unique-db-password@database/app`)
	for _, secret := range []string{"unique.jwt.token", "unique-password", "unique-key", "unique secret with spaces", "unique-db-password", "dbuser"} {
		if strings.Contains(got, secret) {
			t.Fatalf("redacted text still contains %q: %s", secret, got)
		}
	}
}

func TestRedactDiagnosticYAMLDefaultsSensitiveKeysToRedacted(t *testing.T) {
	input := `apiVersion: example.io/v1
kind: Database
metadata:
  name: app
  annotations:
    example.io/password: annotation-sentinel
spec:
  token: nested-token-sentinel
  connection:
    databaseURL: postgres://app:uri-password-sentinel@db/app
  secretKeyRef:
    name: database-credentials
    key: password
`
	got := redactDiagnosticYAML(input)
	for _, secret := range []string{"annotation-sentinel", "nested-token-sentinel", "uri-password-sentinel"} {
		if strings.Contains(got, secret) {
			t.Fatalf("redacted custom resource still contains %q:\n%s", secret, got)
		}
	}
	if !strings.Contains(got, "secretKeyRef") || !strings.Contains(got, "database-credentials") {
		t.Fatalf("redaction should preserve references rather than erase their structure:\n%s", got)
	}
}

func TestRedactDiagnosticYAMLRedactsConfigMapCredentialKeys(t *testing.T) {
	input := `apiVersion: v1
kind: ConfigMap
metadata:
  name: app
data:
  password: configmap-password-sentinel
  normal-setting: visible
`
	got := redactDiagnosticYAML(input)
	if strings.Contains(got, "configmap-password-sentinel") {
		t.Fatalf("redacted ConfigMap still contains its password:\n%s", got)
	}
	if !strings.Contains(got, "normal-setting") || !strings.Contains(got, "visible") {
		t.Fatalf("ordinary ConfigMap values should remain useful evidence:\n%s", got)
	}
}

func TestRedactDiagnosticYAMLRedactsCamelCaseCredentialKeys(t *testing.T) {
	input := `apiVersion: example.io/v1
kind: IdentityProvider
spec:
  accessToken: access-token-sentinel
  refreshToken: refresh-token-sentinel
  adminPassword: admin-password-sentinel
  displayName: safe-visible-value
`
	got := redactDiagnosticYAML(input)
	for _, secret := range []string{"access-token-sentinel", "refresh-token-sentinel", "admin-password-sentinel"} {
		if strings.Contains(got, secret) {
			t.Fatalf("redacted custom resource still contains camelCase credential %q:\n%s", secret, got)
		}
	}
	if !strings.Contains(got, "safe-visible-value") {
		t.Fatalf("ordinary camelCase values should remain useful evidence:\n%s", got)
	}
}
