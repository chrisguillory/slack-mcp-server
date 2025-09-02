package text

import (
	"testing"
)

func TestProcessTextPreservesPunctuationAndPreventsInjection(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Preserves exclamation marks",
			input:    "Hi Tobi! Can I have permanent admin for my laptops?",
			expected: "Hi Tobi! Can I have permanent admin for my laptops?",
		},
		{
			name:     "Prevents equals sign injection",
			input:    "=1+1",
			expected: "'=1+1",
		},
		{
			name:     "Prevents plus sign injection",
			input:    "+1234567890",
			expected: "'+1234567890",
		},
		{
			name:     "Prevents minus sign injection",
			input:    "-1234567890",
			expected: "'-1234567890",
		},
		{
			name:     "Prevents at sign injection",
			input:    "@SUM(A1:A10)",
			expected: "'@SUM(A1:A10)",
		},
		{
			name:     "Preserves equals in middle of text",
			input:    "The formula is x=y+z",
			expected: "The formula is x=y+z",
		},
		{
			name:     "Preserves at sign in email",
			input:    "Contact me at user@example.com",
			expected: "Contact me at user@example.com",
		},
		{
			name:     "Preserves multiple punctuation marks",
			input:    "Wow!!! That's amazing... Really?",
			expected: "Wow!!! That's amazing... Really?",
		},
		{
			name:     "Preserves common symbols",
			input:    "Cost: $100 + 20% = $120 (approx)",
			expected: "Cost: $100 + 20% = $120 (approx)",
		},
		{
			name:     "Handles Slack link with punctuation",
			input:    "Check this! <https://example.com|Click here!> Amazing!!!",
			expected: "Check this! https://example.com - Click here! Amazing!!!",
		},
		{
			name:     "Preserves emojis and special characters",
			input:    "Hello 👋 How are you? 😊",
			expected: "Hello 👋 How are you? 😊",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ProcessText(tt.input)
			if result != tt.expected {
				t.Errorf("ProcessText() = %q, expected %q", result, tt.expected)
			}
		})
	}
}
