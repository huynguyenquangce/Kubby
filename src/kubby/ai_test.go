package main

import (
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

func TestAIConfigViewCannotCarryAnAPIKey(t *testing.T) {
	typeOfView := reflect.TypeOf(AIConfigView{})
	for i := 0; i < typeOfView.NumField(); i++ {
		field := typeOfView.Field(i)
		if strings.Contains(strings.ToLower(field.Name), "key") && field.Name != "HasAPIKey" {
			t.Fatalf("safe AI config view exposes credential field %q", field.Name)
		}
	}
}
