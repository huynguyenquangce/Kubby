package main

import (
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

func TestMergeAIConfigPreservesAWriteOnlyKey(t *testing.T) {
	current := AIConfig{Provider: "anthropic", APIKey: "sentinel-secret"}
	next, err := mergeAIConfig(current, AIConfig{Provider: "anthropic", Model: "new-model"})
	if err != nil {
		t.Fatal(err)
	}
	if next.APIKey != current.APIKey {
		t.Fatal("a blank settings field should preserve the stored key")
	}
}

func TestMergeAIConfigRequiresAKeyWhenProviderChanges(t *testing.T) {
	_, err := mergeAIConfig(
		AIConfig{Provider: "anthropic", APIKey: "old-key"},
		AIConfig{Provider: "openai"},
	)
	if err == nil {
		t.Fatal("changing hosted provider without a replacement key should fail")
	}
}

func TestCredentialEndpointRejectsRemotePlainHTTP(t *testing.T) {
	for _, endpoint := range []string{"http://example.com", "http://10.0.0.8:8080"} {
		if err := validateCredentialEndpoint(endpoint); err == nil {
			t.Fatalf("expected %s to be rejected", endpoint)
		}
	}
	for _, endpoint := range []string{"https://example.com", "http://localhost:8080", "http://127.0.0.1:8080"} {
		if err := validateCredentialEndpoint(endpoint); err != nil {
			t.Fatalf("expected %s to be accepted: %v", endpoint, err)
		}
	}
}

func TestOllamaEndpointMustStayOnLoopback(t *testing.T) {
	for _, endpoint := range []string{"", "http://localhost:11434", "http://127.0.0.1:11434", "https://[::1]:11434"} {
		if _, err := localOllamaBaseURL(endpoint); err != nil {
			t.Fatalf("expected %q to be accepted: %v", endpoint, err)
		}
	}
	for _, endpoint := range []string{"http://example.com:11434", "https://10.0.0.8:11434", "ftp://localhost:11434", "localhost:11434"} {
		if _, err := localOllamaBaseURL(endpoint); err == nil {
			t.Fatalf("expected %q to be rejected", endpoint)
		}
	}
}

func TestMergeAIConfigRejectsRemoteOllama(t *testing.T) {
	_, err := mergeAIConfig(AIConfig{}, AIConfig{Provider: "ollama", Endpoint: "http://192.0.2.8:11434"})
	if err == nil {
		t.Fatal("remote Ollama endpoint should not be saved as a local provider")
	}
}

func TestAIRedirectsAreNeverFollowed(t *testing.T) {
	for _, tc := range []struct {
		provider string
		raw      string
		remote   bool
	}{
		{"ollama", "http://127.0.0.1:11434/api/chat", false},
		{"ollama", "http://192.0.2.8:11434/api/chat", true},
		{"anthropic", "https://other.example.test/v1/messages", false},
		{"openai", "http://downgrade.example.test/v1/chat/completions", false},
	} {
		raw := tc.raw
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		req := &http.Request{URL: u}
		gotErr := aiRedirectPolicy(tc.provider)(req, nil)
		if gotErr == nil {
			t.Fatalf("%s redirect to %s should never be followed", tc.provider, raw)
		}
		if !tc.remote && gotErr != http.ErrUseLastResponse {
			t.Fatalf("safe-target redirect should stop without a transport error, got %v", gotErr)
		}
		if tc.remote && gotErr == http.ErrUseLastResponse {
			t.Fatal("remote Ollama redirect should be rejected by the loopback policy")
		}
	}
}

func TestAIResourcePromptUsesTheReviewedEvidenceExactlyOnce(t *testing.T) {
	const evidence = "# Pod: api\nunique-reviewed-evidence"
	got := aiResourceSystemPrompt("Pod api in namespace payments", evidence, "en")
	if strings.Count(got, evidence) != 1 {
		t.Fatalf("reviewed evidence should appear byte-for-byte exactly once:\n%s", got)
	}
	if !strings.Contains(got, "snapshot reviewed in the resource drawer") {
		t.Fatalf("prompt should identify the evidence as the reviewed snapshot:\n%s", got)
	}
}

func TestAIConfigViewCannotCarryAnAPIKey(t *testing.T) {
	typeOfView := reflect.TypeOf(AIConfigView{})
	for i := 0; i < typeOfView.NumField(); i++ {
		field := typeOfView.Field(i)
		if strings.Contains(strings.ToLower(field.Name), "key") && field.Name != "HasAPIKey" {
			t.Fatalf("safe AI config view exposes credential field %q", field.Name)
		}
	}
}
