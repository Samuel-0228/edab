package governance

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode"
)

var (
	emailRegex = regexp.MustCompile(`(?i)\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`)
	ssnRegex   = regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`)
	phoneRegex = regexp.MustCompile(`(?:\+?1[-. ]?)?\(?([0-9]{3})\)?[-. ]?([0-9]{3})[-. ]?([0-9]{4})\b`)

	// Secret Regexes
	awsKeyRegex      = regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)
	githubTokenRegex = regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9_]{36,255}\b`)
	slackTokenRegex  = regexp.MustCompile(`\bxox[baprs]-[0-9A-Za-z-]{20,}\b`)
	jwtRegex         = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]+\.eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b`)
	pemKeyRegex      = regexp.MustCompile(`-----BEGIN (?:[A-Z ]+)?PRIVATE KEY-----[\s\S]*?-----END (?:[A-Z ]+)?PRIVATE KEY-----`)
	authHeaderRegex  = regexp.MustCompile(`(?i)\b(bearer|basic)\s+[A-Za-z0-9._~+/-]+=*`)

	// Potential card sequence (13 to 19 digits with optional spaces or hyphens)
	cardCandidateRegex = regexp.MustCompile(`\b(?:\d[ -]?){13,19}\b`)

	sensitiveKeySubstrings = []string{
		"secret",
		"token",
		"password",
		"passwd",
		"api_key",
		"apikey",
		"access_key",
		"private_key",
		"credential",
		"bearer",
		"auth",
		"webhook_secret",
		"ssh_key",
		"cert",
		"certificate",
		"session",
		"jwt",
	}
)

// isSensitiveKey returns true if the key matches known credential or secret naming patterns.
func isSensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	for _, pattern := range sensitiveKeySubstrings {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

// isValidLuhn validates card numbers using the standard Luhn checksum algorithm.
func isValidLuhn(s string) bool {
	var digits []int
	for _, ch := range s {
		if ch >= '0' && ch <= '9' {
			digits = append(digits, int(ch-'0'))
		}
	}
	if len(digits) < 13 || len(digits) > 19 {
		return false
	}
	sum := 0
	alternate := false
	for i := len(digits) - 1; i >= 0; i-- {
		d := digits[i]
		if alternate {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		alternate = !alternate
	}
	return sum%10 == 0
}

// MaskString applies PII and secret redactions to a string value.
func MaskString(s string, maskPII, maskSecrets bool) string {
	if s == "" {
		return ""
	}

	res := s

	// 1. Secrets masking
	if maskSecrets {
		res = pemKeyRegex.ReplaceAllString(res, "[REDACTED_PRIVATE_KEY]")
		res = awsKeyRegex.ReplaceAllString(res, "[REDACTED_AWS_KEY]")
		res = githubTokenRegex.ReplaceAllString(res, "[REDACTED_GITHUB_TOKEN]")
		res = slackTokenRegex.ReplaceAllString(res, "[REDACTED_SLACK_TOKEN]")
		res = jwtRegex.ReplaceAllString(res, "[REDACTED_JWT]")
		res = authHeaderRegex.ReplaceAllString(res, "[REDACTED_AUTH]")
	}

	// 2. PII masking
	if maskPII {
		res = emailRegex.ReplaceAllString(res, "[REDACTED_EMAIL]")
		res = ssnRegex.ReplaceAllString(res, "[REDACTED_SSN]")
		res = phoneRegex.ReplaceAllString(res, "[REDACTED_PHONE]")

		// Credit cards with Luhn check
		res = cardCandidateRegex.ReplaceAllStringFunc(res, func(candidate string) string {
			clean := strings.Map(func(r rune) rune {
				if unicode.IsDigit(r) {
					return r
				}
				return -1
			}, candidate)
			if isValidLuhn(clean) {
				return "[REDACTED_CARD]"
			}
			return candidate
		})
	}

	return res
}

// MaskValue recursively copies and sanitizes any nested data structure (maps, slices, strings).
func MaskValue(v any, maskPII, maskSecrets bool) any {
	if v == nil {
		return nil
	}

	switch val := v.(type) {
	case string:
		return MaskString(val, maskPII, maskSecrets)

	case *string:
		if val == nil {
			return nil
		}
		masked := MaskString(*val, maskPII, maskSecrets)
		return &masked

	case map[string]any:
		return MaskMap(val, maskPII, maskSecrets)

	case map[string]string:
		res := make(map[string]any, len(val))
		for k, s := range val {
			if maskSecrets && isSensitiveKey(k) {
				res[k] = "[REDACTED]"
			} else {
				res[k] = MaskString(s, maskPII, maskSecrets)
			}
		}
		return res

	case []any:
		res := make([]any, len(val))
		for i, item := range val {
			res[i] = MaskValue(item, maskPII, maskSecrets)
		}
		return res

	case []string:
		res := make([]string, len(val))
		for i, s := range val {
			res[i] = MaskString(s, maskPII, maskSecrets)
		}
		return res

	case json.RawMessage:
		var unmarshaled any
		if err := json.Unmarshal(val, &unmarshaled); err == nil {
			masked := MaskValue(unmarshaled, maskPII, maskSecrets)
			if out, err := json.Marshal(masked); err == nil {
				return json.RawMessage(out)
			}
		}
		return val

	default:
		return v
	}
}

// MaskMap recursively scrubs sensitive keys and masked PII from a map.
func MaskMap(m map[string]any, maskPII, maskSecrets bool) map[string]any {
	if m == nil {
		return nil
	}
	res := make(map[string]any, len(m))
	for k, v := range m {
		if maskSecrets && isSensitiveKey(k) {
			res[k] = "[REDACTED]"
			continue
		}
		res[k] = MaskValue(v, maskPII, maskSecrets)
	}
	return res
}
