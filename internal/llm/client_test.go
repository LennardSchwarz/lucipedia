package llm

import (
	"testing"
)

func TestNewClientRequiresAPIKey(t *testing.T) {
	t.Parallel()

	if _, err := NewClient(ClientOptions{}); err == nil {
		t.Fatalf("expected error when API key is missing")
	}
}

func TestNewClientUsesDefaultBaseURL(t *testing.T) {
	t.Parallel()

	client, err := NewClient(ClientOptions{APIKey: "test-key"})
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	if client.BaseURL() != openRouterBaseURL {
		t.Fatalf("expected base url %s, got %s", openRouterBaseURL, client.BaseURL())
	}
}
