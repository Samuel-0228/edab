package sources

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestGenericHMACVerification(t *testing.T) {
	secret := "topsecret123"
	body := []byte(`{"event":"test","status":"ok"}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sigHex := hex.EncodeToString(mac.Sum(nil))

	headers := http.Header{}
	headers.Set("X-Signature-256", "sha256="+sigHex)

	// Valid hex with sha256= prefix
	err := VerifyGenericHMAC(secret, "", "hex", headers, body)
	if err != nil {
		t.Fatalf("expected valid signature, got: %v", err)
	}

	// Valid base64
	macB64 := hmac.New(sha256.New, []byte(secret))
	macB64.Write(body)
	sigB64 := base64.StdEncoding.EncodeToString(macB64.Sum(nil))
	headersB64 := http.Header{}
	headersB64.Set("X-Custom-Sig", sigB64)
	errB64 := VerifyGenericHMAC(secret, "X-Custom-Sig", "base64", headersB64, body)
	if errB64 != nil {
		t.Fatalf("expected valid base64 signature, got: %v", errB64)
	}

	// Invalid signature
	headersBad := http.Header{}
	headersBad.Set("X-Signature-256", "bad_signature")
	if err := VerifyGenericHMAC(secret, "", "hex", headersBad, body); err == nil {
		t.Fatal("expected error on invalid signature")
	}

	// Missing header
	if err := VerifyGenericHMAC(secret, "X-Missing", "hex", http.Header{}, body); err == nil {
		t.Fatal("expected error on missing header")
	}
}

func TestSlackSignatureVerification(t *testing.T) {
	signingSecret := "slacksecret456"
	body := []byte(`{"text":"hello from slack"}`)
	now := time.Now()
	tsStr := fmt.Sprintf("%d", now.Unix())

	baseStr := fmt.Sprintf("v0:%s:%s", tsStr, string(body))
	mac := hmac.New(sha256.New, []byte(signingSecret))
	mac.Write([]byte(baseStr))
	sigHex := "v0=" + hex.EncodeToString(mac.Sum(nil))

	headers := http.Header{}
	headers.Set("X-Slack-Signature", sigHex)
	headers.Set("X-Slack-Request-Timestamp", tsStr)

	// Valid
	err := VerifySlackSignature(signingSecret, headers, body, now, 300)
	if err != nil {
		t.Fatalf("expected valid slack signature, got: %v", err)
	}

	// Tampered body
	errTampered := VerifySlackSignature(signingSecret, headers, []byte(`{"tampered":true}`), now, 300)
	if errTampered == nil {
		t.Fatal("expected failure on tampered body")
	}

	// Timestamp drift > 5 min
	oldNow := now.Add(10 * time.Minute)
	errDrift := VerifySlackSignature(signingSecret, headers, body, oldNow, 300)
	if errDrift == nil {
		t.Fatal("expected failure on timestamp drift")
	}
}

func TestStripeSignatureVerification(t *testing.T) {
	secret := "whsec_stripe_test"
	body := []byte(`{"id":"evt_123","type":"payment_intent.succeeded"}`)
	now := time.Now()
	tsStr := fmt.Sprintf("%d", now.Unix())

	payloadToSign := append([]byte(tsStr+"."), body...)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payloadToSign)
	sigHex := hex.EncodeToString(mac.Sum(nil))

	headerVal := fmt.Sprintf("t=%s,v1=%s,v0=unused", tsStr, sigHex)
	headers := http.Header{}
	headers.Set("Stripe-Signature", headerVal)

	// Valid
	err := VerifyStripeSignature(secret, headers, body, now, 300)
	if err != nil {
		t.Fatalf("expected valid stripe signature, got: %v", err)
	}

	// Invalid secret
	errBadSecret := VerifyStripeSignature("wrong_secret", headers, body, now, 300)
	if errBadSecret == nil {
		t.Fatal("expected failure on wrong secret")
	}

	// Drift
	oldNow := now.Add(10 * time.Minute)
	errDrift := VerifyStripeSignature(secret, headers, body, oldNow, 300)
	if errDrift == nil {
		t.Fatal("expected failure on timestamp drift")
	}
}

func TestValidateRequestWithVerifiers(t *testing.T) {
	body := []byte(`{"event":"ping"}`)
	secret := "testsec"

	// 1. Generic HMAC config
	cfgHMAC, _ := json.Marshal(WebhookConfig{
		Secret:            secret,
		SignatureVerifier: "generic_hmac",
	})
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	headers := http.Header{}
	headers.Set("X-Signature-256", hex.EncodeToString(mac.Sum(nil)))

	if err := ValidateRequest(cfgHMAC, headers, body); err != nil {
		t.Fatalf("ValidateRequest failed with generic_hmac: %v", err)
	}
}
