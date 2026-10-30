package sources

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const defaultToleranceSeconds = 300 // 5 minutes

// VerifyGenericHMAC verifies a generic HMAC-SHA256 webhook signature.
func VerifyGenericHMAC(secret string, headerName string, encoding string, headers http.Header, body []byte) error {
	if secret == "" {
		return fmt.Errorf("webhook secret is required for generic HMAC verification")
	}
	if headerName == "" {
		headerName = "X-Signature-256"
		if headers.Get(headerName) == "" && headers.Get("X-Hub-Signature-256") != "" {
			headerName = "X-Hub-Signature-256"
		}
	}

	sigHeader := headers.Get(headerName)
	if sigHeader == "" {
		return fmt.Errorf("missing signature header %q", headerName)
	}

	rawSig := strings.TrimPrefix(sigHeader, "sha256=")

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	computedHash := mac.Sum(nil)

	var expectedSig string
	if strings.EqualFold(encoding, "base64") {
		expectedSig = base64.StdEncoding.EncodeToString(computedHash)
	} else {
		expectedSig = hex.EncodeToString(computedHash)
	}

	if subtle.ConstantTimeCompare([]byte(rawSig), []byte(expectedSig)) != 1 {
		return fmt.Errorf("invalid HMAC signature")
	}
	return nil
}

// VerifySlackSignature verifies incoming Slack webhook signatures (v0=...).
// Base string is "v0:{timestamp}:{body}".
func VerifySlackSignature(signingSecret string, headers http.Header, body []byte, now time.Time, toleranceSec int) error {
	if signingSecret == "" {
		return fmt.Errorf("slack signing secret is required")
	}
	if toleranceSec <= 0 {
		toleranceSec = defaultToleranceSeconds
	}

	sigHeader := headers.Get("X-Slack-Signature")
	if sigHeader == "" {
		return fmt.Errorf("missing X-Slack-Signature header")
	}

	tsStr := headers.Get("X-Slack-Request-Timestamp")
	if tsStr == "" {
		return fmt.Errorf("missing X-Slack-Request-Timestamp header")
	}

	tsInt, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid X-Slack-Request-Timestamp: %w", err)
	}

	reqTime := time.Unix(tsInt, 0)
	diff := math.Abs(now.Sub(reqTime).Seconds())
	if diff > float64(toleranceSec) {
		return fmt.Errorf("slack request timestamp out of tolerance window (drift: %.0fs, max: %ds)", diff, toleranceSec)
	}

	baseStr := fmt.Sprintf("v0:%s:%s", tsStr, string(body))
	mac := hmac.New(sha256.New, []byte(signingSecret))
	mac.Write([]byte(baseStr))
	expected := "v0=" + hex.EncodeToString(mac.Sum(nil))

	if subtle.ConstantTimeCompare([]byte(sigHeader), []byte(expected)) != 1 {
		return fmt.Errorf("invalid slack signature")
	}
	return nil
}

// VerifyStripeSignature verifies Stripe webhook signatures (t={timestamp},v1={signature}).
// Base string is "{timestamp}.{body}".
func VerifyStripeSignature(endpointSecret string, headers http.Header, body []byte, now time.Time, toleranceSec int) error {
	if endpointSecret == "" {
		return fmt.Errorf("stripe webhook secret is required")
	}
	if toleranceSec <= 0 {
		toleranceSec = defaultToleranceSeconds
	}

	sigHeader := headers.Get("Stripe-Signature")
	if sigHeader == "" {
		return fmt.Errorf("missing Stripe-Signature header")
	}

	var timestampStr string
	var signatures []string

	pairs := strings.Split(sigHeader, ",")
	for _, pair := range pairs {
		parts := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(parts) == 2 {
			switch parts[0] {
			case "t":
				timestampStr = parts[1]
			case "v1":
				signatures = append(signatures, parts[1])
			}
		}
	}

	if timestampStr == "" || len(signatures) == 0 {
		return fmt.Errorf("malformed Stripe-Signature header: missing timestamp or v1 signature")
	}

	tsInt, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid stripe timestamp: %w", err)
	}

	reqTime := time.Unix(tsInt, 0)
	diff := math.Abs(now.Sub(reqTime).Seconds())
	if diff > float64(toleranceSec) {
		return fmt.Errorf("stripe request timestamp out of tolerance window (drift: %.0fs, max: %ds)", diff, toleranceSec)
	}

	payloadToSign := append([]byte(timestampStr+"."), body...)
	mac := hmac.New(sha256.New, []byte(endpointSecret))
	mac.Write(payloadToSign)
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	matched := false
	for _, sig := range signatures {
		if subtle.ConstantTimeCompare([]byte(sig), []byte(expectedSig)) == 1 {
			matched = true
			break
		}
	}

	if !matched {
		return fmt.Errorf("invalid stripe webhook signature")
	}
	return nil
}
