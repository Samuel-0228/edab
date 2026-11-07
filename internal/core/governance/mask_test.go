package governance

import (
	"strings"
	"testing"
)

func TestMasking(t *testing.T) {
	t.Run("PII masking", func(t *testing.T) {
		input := "User alice@example.com with phone 555-123-4567 and SSN 123-45-6789 registered."
		masked := MaskString(input, true, false)

		if strings.Contains(masked, "alice@example.com") {
			t.Errorf("expected email to be masked, got: %s", masked)
		}
		if !strings.Contains(masked, "[REDACTED_EMAIL]") {
			t.Errorf("expected [REDACTED_EMAIL], got: %s", masked)
		}
		if strings.Contains(masked, "123-45-6789") {
			t.Errorf("expected SSN to be masked, got: %s", masked)
		}
		if !strings.Contains(masked, "[REDACTED_SSN]") {
			t.Errorf("expected [REDACTED_SSN], got: %s", masked)
		}
		if strings.Contains(masked, "555-123-4567") {
			t.Errorf("expected phone to be masked, got: %s", masked)
		}
		if !strings.Contains(masked, "[REDACTED_PHONE]") {
			t.Errorf("expected [REDACTED_PHONE], got: %s", masked)
		}
	})

	t.Run("Credit card with Luhn verification", func(t *testing.T) {
		// Valid test Visa card
		validVisa := "4111 1111 1111 1111"
		maskedValid := MaskString("Paid with "+validVisa, true, false)
		if strings.Contains(maskedValid, "4111") {
			t.Errorf("expected valid card to be masked, got: %s", maskedValid)
		}
		if !strings.Contains(maskedValid, "[REDACTED_CARD]") {
			t.Errorf("expected [REDACTED_CARD], got: %s", maskedValid)
		}

		// Invalid card number (fails Luhn)
		invalidCard := "4111 1111 1111 1112"
		maskedInvalid := MaskString("Order ref "+invalidCard, true, false)
		if strings.Contains(maskedInvalid, "[REDACTED_CARD]") {
			t.Errorf("expected invalid card not to be masked as card, got: %s", maskedInvalid)
		}
	})

	t.Run("Secret tokens masking", func(t *testing.T) {
		input := "AWS key AKIAIOSFODNN7EXAMPLE, GitHub token ghp_1234567890abcdefghijklmnopqrstuvwxyz, and Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozS6-mG_XvT"
		masked := MaskString(input, false, true)

		if strings.Contains(masked, "AKIAIOSFODNN7EXAMPLE") {
			t.Errorf("expected AWS key to be masked")
		}
		if !strings.Contains(masked, "[REDACTED_AWS_KEY]") {
			t.Errorf("expected [REDACTED_AWS_KEY], got: %s", masked)
		}
		if strings.Contains(masked, "ghp_1234567890") {
			t.Errorf("expected GitHub token to be masked")
		}
		if !strings.Contains(masked, "[REDACTED_GITHUB_TOKEN]") {
			t.Errorf("expected [REDACTED_GITHUB_TOKEN], got: %s", masked)
		}
		if strings.Contains(masked, "eyJhbGciOi") {
			t.Errorf("expected JWT to be masked")
		}
	})

	t.Run("Deep map and slice masking", func(t *testing.T) {
		data := map[string]any{
			"api_key": "supersecretkey123",
			"user": map[string]any{
				"email": "bob@acme.corp",
				"notes": "Secret token is ghp_abcdefghijklmnopqrstuvwxyz1234567890",
			},
			"tags": []any{
				"normal",
				"Call 800-555-0199 for help",
			},
		}

		masked, ok := MaskValue(data, true, true).(map[string]any)
		if !ok {
			t.Fatalf("expected map[string]any from MaskValue")
		}

		if masked["api_key"] != "[REDACTED]" {
			t.Errorf("expected api_key to be [REDACTED], got: %v", masked["api_key"])
		}

		userMap, ok := masked["user"].(map[string]any)
		if !ok {
			t.Fatalf("expected user to be map[string]any")
		}
		if userMap["email"] != "[REDACTED_EMAIL]" {
			t.Errorf("expected email to be [REDACTED_EMAIL], got: %v", userMap["email"])
		}
		notesStr, _ := userMap["notes"].(string)
		if strings.Contains(notesStr, "ghp_") {
			t.Errorf("expected github token in notes to be masked")
		}

		tags, ok := masked["tags"].([]any)
		if !ok || len(tags) < 2 {
			t.Fatalf("expected tags to be []any with at least 2 elements")
		}
		tagStr, _ := tags[1].(string)
		if strings.Contains(tagStr, "800-555-0199") {
			t.Errorf("expected phone in tags to be masked")
		}
	})
}
