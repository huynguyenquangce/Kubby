package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAIProviderConversationContracts(t *testing.T) {
	history := []AIMessage{{Role: "user", Content: "Why Pending?"}, {Role: "assistant", Content: "Check storage."}, {Role: "user", Content: "What next?"}}
	system := aiResourceSystemPrompt("Pod synthetic in namespace test", "reviewed synthetic evidence", "vi")
	for _, tc := range []struct{ provider, path, response string }{
		{"openai", "/v1/chat/completions", `{"choices":[{"message":{"content":"  synthetic answer  "}}]}`},
		{"anthropic", "/v1/messages", `{"content":[{"type":"text","text":"synthetic "},{"type":"text","text":"answer"}]}`},
		{"ollama", "/api/chat", `{"message":{"content":" synthetic answer "}}`},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			requests := make(chan map[string]any, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != tc.path {
					t.Errorf("unexpected provider method/path: %s %s", r.Method, r.URL.Path)
				}
				if r.Header.Get("Content-Type") != "application/json" {
					t.Error("missing JSON content type")
				}
				if tc.provider == "openai" && r.Header.Get("Authorization") == "" {
					t.Error("missing credential header")
				}
				if tc.provider == "anthropic" && (r.Header.Get("x-api-key") == "" || r.Header.Get("anthropic-version") == "") {
					t.Error("missing Anthropic headers")
				}
				if tc.provider == "ollama" && (r.Header.Get("Authorization") != "" || r.Header.Get("x-api-key") != "") {
					t.Error("local provider received credentials")
				}
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
				}
				requests <- payload
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.response))
			}))
			defer server.Close()
			cfg := AIConfig{Provider: tc.provider, Endpoint: server.URL, Model: "synthetic-model"}
			if tc.provider != "ollama" {
				cfg.APIKey = "synthetic-noncredential"
			}
			answer, err := callAI(context.Background(), cfg, system, history)
			if err != nil || answer != "synthetic answer" {
				t.Fatalf("provider answer invalid: %q, %v", answer, err)
			}
			payload := <-requests
			if payload["model"] != "synthetic-model" {
				t.Fatal("configured model was lost")
			}
			messages := payload["messages"].([]any)
			offset := 0
			if tc.provider == "anthropic" {
				if payload["system"] != system {
					t.Fatal("reviewed system evidence changed")
				}
			} else {
				offset = 1
				first := messages[0].(map[string]any)
				if first["role"] != "system" || first["content"] != system {
					t.Fatal("reviewed evidence missing")
				}
			}
			if len(messages) != len(history)+offset {
				t.Fatal("follow-up history truncated")
			}
			for i, msg := range history {
				got := messages[i+offset].(map[string]any)
				if got["role"] != msg.Role || got["content"] != msg.Content {
					t.Fatal("conversation order/content changed")
				}
			}
			if tc.provider == "ollama" && (payload["stream"] != false || payload["options"].(map[string]any)["num_ctx"] != float64(ollamaNumCtx)) {
				t.Fatal("local evidence context/response contract changed")
			}
		})
	}
}

func TestAIProviderErrorsAndCancellation(t *testing.T) {
	for _, tc := range []struct {
		name, provider, body string
		status               int
		want                 string
	}{
		{"HTTP failure", "openai", `{"error":"synthetic rate limit"}`, 429, "HTTP 429"},
		{"invalid JSON", "openai", `not JSON`, 200, "invalid"},
		{"no choices", "openai", `{"choices":[]}`, 200, "no content"},
		{"missing local model", "ollama", `{"error":"synthetic model not found"}`, 200, "model not found"},
		{"empty local answer", "ollama", `{"message":{"content":" "}}`, 200, "empty answer"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			_, err := callAI(context.Background(), AIConfig{Provider: tc.provider, Endpoint: server.URL, APIKey: "synthetic-noncredential"}, "synthetic evidence", []AIMessage{{Role: "user", Content: "synthetic question"}})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("unexpected provider error: %v", err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := callAI(ctx, AIConfig{Provider: "ollama", Endpoint: "http://127.0.0.1:1"}, "synthetic", []AIMessage{{Role: "user", Content: "question"}})
	if err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("cancelled request did not stop: %v", err)
	}
}

func TestAIProviderEchoedCredentialsNeverReachErrors(t *testing.T) {
	synthetic := strings.Repeat("synthetic-noncredential-", 20)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("echo " + synthetic + " password=synthetic-password Authorization: Bearer synthetic-token"))
	}))
	defer server.Close()
	_, err := callAI(context.Background(), AIConfig{Provider: "openai", Endpoint: server.URL, APIKey: synthetic}, "synthetic", []AIMessage{{Role: "user", Content: "question"}})
	if err == nil {
		t.Fatal("expected HTTP error")
	}
	for _, value := range []string{"synthetic-noncredential", "synthetic-password", "synthetic-token"} {
		if strings.Contains(err.Error(), value) {
			t.Fatal("provider error leaked credential text")
		}
	}
}
