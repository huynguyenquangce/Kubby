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
	got := redactSensitiveText(`Authorization: Bearer unique.jwt.token password=unique-password api_key: unique-key client_secret="unique secret with spaces"`)
	for _, secret := range []string{"unique.jwt.token", "unique-password", "unique-key", "unique secret with spaces"} {
		if strings.Contains(got, secret) {
			t.Fatalf("redacted text still contains %q: %s", secret, got)
		}
	}
}
