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
