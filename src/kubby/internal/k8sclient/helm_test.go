package k8sclient

import (
	"testing"

	"helm.sh/helm/v3/pkg/action"
)

func TestVerifyChartDigestRequiresThePreviewedArtifact(t *testing.T) {
	const digest = "sha256:0123456789abcdef"
	if err := verifyChartDigest("", digest); err == nil {
		t.Fatal("install without a preview digest should be rejected")
	}
	if err := verifyChartDigest("sha256:different", digest); err == nil {
		t.Fatal("a chart that changed after preview should be rejected")
	}
	if err := verifyChartDigest(digest, digest); err != nil {
		t.Fatalf("the exact previewed chart should be accepted: %v", err)
	}
}

func TestHelmOpenPGPVerificationRemainsDisabled(t *testing.T) {
	// Kubby uses its own preview SHA-256 gate. The Helm v3 OpenPGP implementation
	// is deprecated upstream and must not become reachable through a new option.
	options := action.ChartPathOptions{Verify: true}
	configureChartSource(&options, "https://charts.example.com", "1.2.3")
	if options.Verify {
		t.Fatal("Helm chart provenance verification unexpectedly enabled")
	}
}
