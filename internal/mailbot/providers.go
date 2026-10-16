package mailbot

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
)

type aiRequest struct {
	Endpoint string
	Headers  http.Header
	Payload  any
	Protocol string
}

func buildAIRequest(c AIConfig, prompt, content string) (aiRequest, error) {
	switch strings.ToLower(c.Provider) {
	case "", "deepseek", "openai", "custom":
	default:
		return aiRequest{}, fmt.Errorf("only DeepSeek with the OpenAI-compatible API is supported")
	}
	base := strings.TrimRight(c.BaseURL, "/")
	if base == "" {
		base = strings.TrimRight(os.Getenv("DEEPSEEK_API_BASE"), "/")
	}
	if base == "" {
		base = "https://api.deepseek.com"
	}
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return aiRequest{}, fmt.Errorf("invalid DeepSeek base_url")
	}
	if !strings.HasSuffix(base, "/chat/completions") {
		base += "/chat/completions"
	}
	model := strings.TrimPrefix(c.Model, "deepseek/")
	if model == "" {
		model = "deepseek-chat"
	}
	key := c.APIKey
	if key == "" {
		key = os.Getenv("DEEPSEEK_API_KEY")
	}
	headers := make(http.Header)
	headers.Set("Content-Type", "application/json")
	if key != "" {
		headers.Set("Authorization", "Bearer "+key)
	}
	return aiRequest{Endpoint: base, Headers: headers, Protocol: "compatible", Payload: map[string]any{"model": model, "messages": []map[string]string{{"role": "system", "content": prompt}, {"role": "user", "content": content}}, "temperature": 0.3, "max_tokens": 800, "response_format": map[string]string{"type": "json_object"}}}, nil
}
