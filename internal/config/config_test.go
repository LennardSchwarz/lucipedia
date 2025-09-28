package config

import (
	"strings"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("DB_PATH", "")
	t.Setenv("SERVER_PORT", "")
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("LLM_ENDPOINT", "")
	t.Setenv("LLM_API_KEY", "")
	t.Setenv("LLM_MODELS", "")
	t.Setenv("SENTRY_DSN", "")
	t.Setenv("ENV", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.DBPath != defaultDBPath {
		t.Errorf("expected default DB path %q, got %q", defaultDBPath, cfg.DBPath)
	}

	if cfg.ServerPort != defaultServerPort {
		t.Errorf("expected default server port %d, got %d", defaultServerPort, cfg.ServerPort)
	}

	if cfg.LogLevel != defaultLogLevel {
		t.Errorf("expected default log level %q, got %q", defaultLogLevel, cfg.LogLevel)
	}

	if cfg.Environment != defaultEnvironment {
		t.Errorf("expected default environment %q, got %q", defaultEnvironment, cfg.Environment)
	}

	if cfg.ShutdownGrace != defaultShutdownGrace {
		t.Errorf("expected shutdown grace %s, got %s", defaultShutdownGrace, cfg.ShutdownGrace)
	}

	if cfg.LLMModels != nil {
		t.Errorf("expected nil LLMModels, got %v", cfg.LLMModels)
	}

	if cfg.LLMEndpoint != "" {
		t.Errorf("expected empty LLM endpoint, got %q", cfg.LLMEndpoint)
	}

	if cfg.LLMAPIKey != "" {
		t.Errorf("expected empty LLM API key, got %q", cfg.LLMAPIKey)
	}

	if cfg.SentryDSN != "" {
		t.Errorf("expected empty Sentry DSN, got %q", cfg.SentryDSN)
	}
}

func TestLoadWithExplicitValues(t *testing.T) {
	t.Setenv("DB_PATH", "/tmp/lucipedia.db")
	t.Setenv("SERVER_PORT", "9090")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("LLM_ENDPOINT", "https://example.com/llm")
	t.Setenv("LLM_API_KEY", "secret")
	t.Setenv("LLM_MODELS", `["alpha","beta"]`)
	t.Setenv("SENTRY_DSN", "dsn")
	t.Setenv("ENV", "production")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.DBPath != "/tmp/lucipedia.db" {
		t.Errorf("expected DB path %q, got %q", "/tmp/lucipedia.db", cfg.DBPath)
	}

	if cfg.ServerPort != 9090 {
		t.Errorf("expected server port 9090, got %d", cfg.ServerPort)
	}

	if cfg.LogLevel != "debug" {
		t.Errorf("expected log level debug, got %q", cfg.LogLevel)
	}

	if cfg.LLMEndpoint != "https://example.com/llm" {
		t.Errorf("expected LLM endpoint https://example.com/llm, got %q", cfg.LLMEndpoint)
	}

	if cfg.LLMAPIKey != "secret" {
		t.Errorf("expected LLM API key secret, got %q", cfg.LLMAPIKey)
	}

	expectedModels := []string{"alpha", "beta"}
	if len(cfg.LLMModels) != len(expectedModels) {
		t.Fatalf("expected %d models, got %d", len(expectedModels), len(cfg.LLMModels))
	}

	for i, model := range cfg.LLMModels {
		if model != expectedModels[i] {
			t.Errorf("expected model %q at index %d, got %q", expectedModels[i], i, model)
		}
	}

	if cfg.SentryDSN != "dsn" {
		t.Errorf("expected Sentry DSN dsn, got %q", cfg.SentryDSN)
	}

	if cfg.Environment != "production" {
		t.Errorf("expected environment production, got %q", cfg.Environment)
	}
}

func TestLoadWithModelObject(t *testing.T) {
	t.Setenv("LLM_MODELS", `{"models":["gamma","delta"]}`)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	expected := []string{"gamma", "delta"}
	if len(cfg.LLMModels) != len(expected) {
		t.Fatalf("expected %d models, got %d", len(expected), len(cfg.LLMModels))
	}

	for i, model := range cfg.LLMModels {
		if model != expected[i] {
			t.Errorf("expected model %q at index %d, got %q", expected[i], i, model)
		}
	}
}

func TestLoadInvalidPort(t *testing.T) {
	t.Setenv("SERVER_PORT", "invalid")

	_, err := Load()
	if err == nil {
		t.Fatalf("expected error for invalid port, got nil")
	}

	if !strings.Contains(err.Error(), "invalid SERVER_PORT value") {
		t.Fatalf("expected error to mention invalid SERVER_PORT value, got %v", err)
	}
}

func TestLoadInvalidModels(t *testing.T) {
	t.Setenv("LLM_MODELS", `{"models":null}`)

	_, err := Load()
	if err == nil {
		t.Fatalf("expected error when models JSON is invalid, got nil")
	}

	if !strings.Contains(err.Error(), "parsing LLM_MODELS") {
		t.Fatalf("expected error to mention parsing LLM_MODELS, got %v", err)
	}
}
