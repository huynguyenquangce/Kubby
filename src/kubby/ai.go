package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// AIConfig is the local, per-user AI provider configuration (stored beside the
// recent-connections file). The API key never leaves this machine except in the
// request to the provider the user chose.
type AIConfig struct {
	Provider string `json:"provider"` // "anthropic" | "ollama" | "openai"
	Endpoint string `json:"endpoint"` // base URL (blank = provider default)
	APIKey   string `json:"apiKey"`
	Model    string `json:"model"`
	Language string `json:"language"` // "" / "auto" | "en" | "vi"
}

// AIConfigView is safe to expose to the WebView. API keys are write-only from
// the renderer's perspective; HasAPIKey is enough to support editing settings.
type AIConfigView struct {
	Provider  string `json:"provider"`
	Endpoint  string `json:"endpoint"`
	Model     string `json:"model"`
	Language  string `json:"language"`
	HasAPIKey bool   `json:"hasApiKey"`
}

// AIMessage is one turn of the resource-scoped conversation. The frontend owns
// the thread and replays it on each request; nothing is persisted.
type AIMessage struct {
	Role    string `json:"role"` // "user" | "assistant"
	Content string `json:"content"`
}

// defaultModel is the model used when the user leaves the field blank.
func defaultModel(provider string) string {
	switch provider {
	case "anthropic":
		return "claude-haiku-4-5-20251001"
	case "openai":
		return "gpt-4o-mini"
	case "ollama":
		return "llama3.1"
	}
	return ""
}

// providerLabel is the human name shown in the assistant header.
func providerLabel(provider string) string {
	switch provider {
	case "anthropic":
		return "Anthropic (Claude)"
	case "openai":
		return "OpenAI-compatible"
	case "ollama":
		return "Ollama (local)"
	}
	return ""
}

func aiConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	kubbyDir := filepath.Join(dir, "kubby")
	if err := os.MkdirAll(kubbyDir, 0o700); err != nil {
		return "", err
	}
	_ = os.Chmod(kubbyDir, 0o700)
	return filepath.Join(kubbyDir, "ai.json"), nil
}

func loadAIConfig() AIConfig {
	var cfg AIConfig
	path, err := aiConfigPath()
	if err != nil {
		return cfg
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(data, &cfg)
	return cfg
}

func saveAIConfig(cfg AIConfig) error {
	path, err := aiConfigPath()
	if err != nil {
		return err
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600) // WriteFile preserves an existing file's mode.
}

func mergeAIConfig(current, next AIConfig) (AIConfig, error) {
	next.Provider = strings.TrimSpace(next.Provider)
	next.Endpoint = strings.TrimSpace(next.Endpoint)
	next.Model = strings.TrimSpace(next.Model)
	if next.Provider == "" {
		return AIConfig{}, fmt.Errorf("pick an AI provider first")
	}
	if next.Provider == "ollama" {
		next.APIKey = ""
		return next, nil
	}
	if strings.TrimSpace(next.APIKey) == "" {
		if current.Provider != next.Provider || strings.TrimSpace(current.APIKey) == "" {
			return AIConfig{}, fmt.Errorf("this provider needs an API key")
		}
		next.APIKey = current.APIKey
	}
	return next, nil
}

// callAI dispatches a system prompt plus the full conversation to the configured
// provider. The resource context lives in the system prompt, so every follow-up
// question is answered against the same snapshot the thread started with.
func callAI(ctx context.Context, cfg AIConfig, system string, msgs []AIMessage) (string, error) {
	if len(msgs) == 0 {
		return "", fmt.Errorf("no question to send")
	}
	// A local model on CPU is far slower than a hosted one — prompt processing
	// alone can take a minute on a laptop — so Ollama gets a much longer budget.
	// There is no per-token cost or rate limit to protect against locally.
	timeout := 120 * time.Second
	if cfg.Provider == "ollama" {
		timeout = 10 * time.Minute
	}
	client := &http.Client{Timeout: timeout}
	switch cfg.Provider {
	case "anthropic":
		return callAnthropic(ctx, client, cfg, system, msgs)
	case "ollama":
		return callOllama(ctx, client, cfg, system, msgs)
	case "openai":
		return callOpenAI(ctx, client, cfg, system, msgs)
	default:
		return "", fmt.Errorf("no AI provider configured — open Settings (⚙) to pick one")
	}
}

// chatMessages converts the thread to the {role, content} shape used by both the
// Anthropic and the OpenAI/Ollama chat APIs.
func chatMessages(msgs []AIMessage) []map[string]any {
	out := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		role := m.Role
		if role != "assistant" {
			role = "user"
		}
		out = append(out, map[string]any{"role": role, "content": m.Content})
	}
	return out
}

func postJSON(ctx context.Context, client *http.Client, url string, body any, headers map[string]string) ([]byte, error) {
	buf, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not reach the AI endpoint: %w", err)
	}
	defer resp.Body.Close()
	const maxAIResponse = 2 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxAIResponse+1))
	if err != nil {
		return nil, fmt.Errorf("could not read the AI response: %w", err)
	}
	if len(data) > maxAIResponse {
		return nil, fmt.Errorf("the AI response exceeded the 2 MiB safety limit")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("the AI returned HTTP %d: %s", resp.StatusCode, truncateErr(string(data)))
	}
	return data, nil
}

func truncateErr(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 300 {
		return s[:300] + "…"
	}
	return s
}

func callAnthropic(ctx context.Context, client *http.Client, cfg AIConfig, system string, msgs []AIMessage) (string, error) {
	if cfg.APIKey == "" {
		return "", fmt.Errorf("missing API key for Anthropic")
	}
	model := cfg.Model
	if model == "" {
		model = defaultModel("anthropic")
	}
	endpoint := strings.TrimRight(cfg.Endpoint, "/")
	if endpoint == "" {
		endpoint = "https://api.anthropic.com"
	}
	if err := validateCredentialEndpoint(endpoint); err != nil {
		return "", err
	}
	body := map[string]any{
		"model":      model,
		"max_tokens": 1500,
		"system":     system,
		"messages":   chatMessages(msgs),
	}
	data, err := postJSON(ctx, client, endpoint+"/v1/messages", body, map[string]string{
		"x-api-key":         cfg.APIKey,
		"anthropic-version": "2023-06-01",
	})
	if err != nil {
		return "", err
	}
	var out struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, c := range out.Content {
		sb.WriteString(c.Text)
	}
	return strings.TrimSpace(sb.String()), nil
}

// openAIBaseURL resolves the base a `/chat/completions` call hangs off.
//
// Only a bare host gets `/v1` appended — a URL that already carries a path is
// used verbatim. Compatible endpoints do not agree on the version segment:
// OpenAI and Groq end in `/v1`, Gemini's compatibility layer ends in
// `/v1beta/openai`, and internal gateways mount wherever they like. Appending
// `/v1` to those produces a 404.
func openAIBaseURL(endpoint string) (string, error) {
	raw := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if raw == "" {
		return "https://api.openai.com/v1", nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("invalid endpoint %q — expected a URL such as https://api.openai.com", endpoint)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("endpoint %q must start with http:// or https://", endpoint)
	}
	if err := validateCredentialEndpoint(raw); err != nil {
		return "", err
	}
	// Tolerate a full path being pasted in from a provider's docs.
	raw = strings.TrimSuffix(raw, "/chat/completions")
	if u.Path == "" {
		raw += "/v1"
	}
	return raw, nil
}

func validateCredentialEndpoint(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("invalid AI endpoint %q", raw)
	}
	if u.Scheme == "https" {
		return nil
	}
	host := strings.ToLower(u.Hostname())
	ip := net.ParseIP(host)
	if u.Scheme == "http" && (host == "localhost" || (ip != nil && ip.IsLoopback())) {
		return nil
	}
	return fmt.Errorf("AI endpoints that receive an API key must use HTTPS (plain HTTP is allowed only on loopback)")
}

func callOpenAI(ctx context.Context, client *http.Client, cfg AIConfig, system string, msgs []AIMessage) (string, error) {
	if cfg.APIKey == "" {
		return "", fmt.Errorf("missing API key for the OpenAI-compatible endpoint")
	}
	model := cfg.Model
	if model == "" {
		model = defaultModel("openai")
	}
	base, err := openAIBaseURL(cfg.Endpoint)
	if err != nil {
		return "", err
	}
	body := map[string]any{
		"model":    model,
		"messages": append([]map[string]any{{"role": "system", "content": system}}, chatMessages(msgs)...),
	}
	data, err := postJSON(ctx, client, base+"/chat/completions", body, map[string]string{
		"Authorization": "Bearer " + cfg.APIKey,
	})
	if err != nil {
		return "", err
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("the AI returned no content")
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}

// ollamaNumCtx is the context window requested from Ollama, which otherwise
// defaults to 2048 tokens.
const ollamaNumCtx = 8192

func callOllama(ctx context.Context, client *http.Client, cfg AIConfig, system string, msgs []AIMessage) (string, error) {
	model := cfg.Model
	if model == "" {
		model = defaultModel("ollama")
	}
	base := strings.TrimRight(cfg.Endpoint, "/")
	if base == "" {
		base = "http://localhost:11434"
	}
	body := map[string]any{
		"model":    model,
		"stream":   false,
		"messages": append([]map[string]any{{"role": "system", "content": system}}, chatMessages(msgs)...),
		"options": map[string]any{
			// Ollama defaults to a 2048-token context and silently drops
			// whatever overflows — the oldest tokens first, which here is the
			// system prompt carrying all the events/logs/YAML. The model would
			// answer from a truncated snapshot and sound confident about it.
			// One resource's evidence caps out around 10 KB (~3k tokens), and a
			// few follow-up turns sit on top, so 8192 leaves real headroom.
			"num_ctx": ollamaNumCtx,
		},
	}
	data, err := postJSON(ctx, client, base+"/api/chat", body, nil)
	if err != nil {
		return "", err
	}
	var out struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", err
	}
	// Ollama reports a missing model as HTTP 200 with an `error` field, so a
	// bare content read would surface it as an empty answer.
	if out.Error != "" {
		return "", fmt.Errorf("Ollama: %s", out.Error)
	}
	if strings.TrimSpace(out.Message.Content) == "" {
		return "", fmt.Errorf("Ollama returned an empty answer — is the model %q pulled? Run: ollama pull %s", model, model)
	}
	return strings.TrimSpace(out.Message.Content), nil
}
