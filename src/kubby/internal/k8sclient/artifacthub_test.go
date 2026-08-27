package k8sclient

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestArtifactHubResponseIsBoundedAndDecoded(t *testing.T) {
	valid := &http.Response{Body: io.NopCloser(strings.NewReader(`{"packages":[]}`)), ContentLength: -1}
	var payload struct {
		Packages []any `json:"packages"`
	}
	if err := decodeArtifactHubResponse(valid, 64, &payload); err != nil {
		t.Fatalf("valid response: %v", err)
	}

	tooLarge := &http.Response{Body: io.NopCloser(strings.NewReader(`{"packages":[]}`)), ContentLength: -1}
	if err := decodeArtifactHubResponse(tooLarge, 5, &payload); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("oversized response error = %v", err)
	}
}
