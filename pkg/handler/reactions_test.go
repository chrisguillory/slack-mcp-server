package handler

import (
	"os"
	"testing"
)

// TestUnitIsReactionAllowed validates the SLACK_MCP_ADD_REACTION_TOOL policy parsing.
func TestUnitIsReactionAllowed(t *testing.T) {
	handler := &ReactionsHandler{}

	t.Run("default disabled when unset", func(t *testing.T) {
		t.Setenv("SLACK_MCP_ADD_REACTION_TOOL", "")
		if handler.isReactionAllowed("C123") {
			t.Fatalf("expected disabled by default, got allowed")
		}
	})

	t.Run("enabled for all with true", func(t *testing.T) {
		t.Setenv("SLACK_MCP_ADD_REACTION_TOOL", "true")
		if !handler.isReactionAllowed("C123") {
			t.Fatalf("expected allowed for all when true")
		}
	})

	t.Run("enabled for all with 1", func(t *testing.T) {
		t.Setenv("SLACK_MCP_ADD_REACTION_TOOL", "1")
		if !handler.isReactionAllowed("C123") {
			t.Fatalf("expected allowed for all when 1")
		}
	})

	t.Run("allowlist specific channels", func(t *testing.T) {
		t.Setenv("SLACK_MCP_ADD_REACTION_TOOL", "C111,C222")
		if !handler.isReactionAllowed("C111") {
			t.Fatalf("expected C111 allowed")
		}
		if handler.isReactionAllowed("C999") {
			t.Fatalf("expected C999 not allowed")
		}
	})

	t.Run("denylist specific channels with !", func(t *testing.T) {
		t.Setenv("SLACK_MCP_ADD_REACTION_TOOL", "!C111,!C222")
		if handler.isReactionAllowed("C111") {
			t.Fatalf("expected C111 denied by ! list")
		}
		if !handler.isReactionAllowed("C999") {
			t.Fatalf("expected channels not in ! list to be allowed")
		}
	})

	t.Run("whitespace trimming and robustness", func(t *testing.T) {
		t.Setenv("SLACK_MCP_ADD_REACTION_TOOL", "  C111 ,  C222  ")
		if !handler.isReactionAllowed("C222") {
			t.Fatalf("expected C222 allowed with whitespace")
		}
	})

	// Ensure no leakage between tests
	t.Cleanup(func() { os.Unsetenv("SLACK_MCP_ADD_REACTION_TOOL") })
}
