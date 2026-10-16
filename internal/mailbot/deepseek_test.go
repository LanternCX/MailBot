package mailbot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestEnabledAIRejectsOtherProvidersDuringConfigCheck(t *testing.T) {
	c := DefaultConfig()
	c.AI.Enabled = true
	c.AI.Provider = "anthropic"
	path := filepath.Join(t.TempDir(), "config.json")
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("unsupported AI accepted by config validation")
	}
}

func TestDeepSeekDefaultsAndConfiguration(t *testing.T) {
	t.Setenv("DEEPSEEK_API_BASE", "")
	t.Setenv("DEEPSEEK_API_KEY", "environment-key")
	c := DefaultConfig().AI
	if c.Provider != "deepseek" || c.Model != "deepseek-chat" {
		t.Fatal("defaults must select DeepSeek")
	}
	r, err := buildAIRequest(c, "system", "email")
	if err != nil || r.Endpoint != "https://api.deepseek.com/chat/completions" || r.Headers.Get("Authorization") != "Bearer environment-key" {
		t.Fatalf("defaults: %+v %v", r, err)
	}
	t.Setenv("DEEPSEEK_API_BASE", "https://environment.example/v1/")
	r, err = buildAIRequest(c, "system", "email")
	if err != nil || r.Endpoint != "https://environment.example/v1/chat/completions" {
		t.Fatalf("environment base: %+v %v", r, err)
	}
	for _, base := range []string{"https://explicit.example/v1", "https://explicit.example/v1/chat/completions/"} {
		c.BaseURL, c.APIKey, c.Model = base, "explicit-key", "deepseek/deepseek-chat"
		r, err = buildAIRequest(c, "system", "email")
		if err != nil || r.Endpoint != "https://explicit.example/v1/chat/completions" || r.Headers.Get("Authorization") != "Bearer explicit-key" || r.Payload.(map[string]any)["model"] != "deepseek-chat" {
			t.Fatalf("explicit configuration: %+v %v", r, err)
		}
	}
	for _, provider := range []string{"anthropic", "gemini", "bedrock", "ollama", "azure", "github_copilot"} {
		if _, err := buildAIRequest(AIConfig{Provider: provider}, "s", "u"); err == nil {
			t.Fatalf("unsupported provider %s accepted", provider)
		}
	}
}
