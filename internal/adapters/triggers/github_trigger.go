package triggers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"kerob/internal/core/event"
)

var (
	ErrInvalidGitHubSignature = errors.New("invalid github webhook signature")
)

// ValidateGitHubSignature verifies the X-Hub-Signature-256 header using the webhook secret.
func ValidateGitHubSignature(secret string, body []byte, signatureHeader string) bool {
	if secret == "" {
		return true // No secret verification required
	}
	if signatureHeader == "" {
		return false
	}

	sigHex := strings.TrimPrefix(signatureHeader, "sha256=")
	expectedSig, err := hex.DecodeString(sigHex)
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	actualSig := mac.Sum(nil)

	return hmac.Equal(actualSig, expectedSig)
}

// NormalizeGitHubEvent parses incoming GitHub webhook payload and produces a normalized Event.
func NormalizeGitHubEvent(sourceID string, r *http.Request, body []byte) (*event.Event, error) {
	ghEvent := r.Header.Get("X-GitHub-Event")
	if ghEvent == "" {
		ghEvent = "webhook"
	}

	deliveryID := r.Header.Get("X-GitHub-Delivery")
	if deliveryID == "" {
		deliveryID = fmt.Sprintf("gh-%d", len(body))
	}

	var rawMap map[string]any
	if err := json.Unmarshal(body, &rawMap); err != nil {
		return nil, fmt.Errorf("invalid github json payload: %w", err)
	}

	action, _ := rawMap["action"].(string)

	var eventType string
	payload := make(map[string]any)

	switch ghEvent {
	case "issues":
		if action != "" {
			eventType = fmt.Sprintf("github.issue.%s", action)
		} else {
			eventType = "github.issue"
		}
		if issue, ok := rawMap["issue"].(map[string]any); ok {
			payload["title"] = issue["title"]
			payload["body"] = issue["body"]
			payload["number"] = issue["number"]
			payload["html_url"] = issue["html_url"]
			if user, ok := issue["user"].(map[string]any); ok {
				payload["author"] = user["login"]
			}
		}

	case "push":
		eventType = "github.push"
		payload["ref"] = rawMap["ref"]
		payload["before"] = rawMap["before"]
		payload["after"] = rawMap["after"]
		if pusher, ok := rawMap["pusher"].(map[string]any); ok {
			payload["author"] = pusher["name"]
		}
		if commits, ok := rawMap["commits"].([]any); ok {
			payload["commit_count"] = len(commits)
		}

	case "pull_request":
		if action != "" {
			eventType = fmt.Sprintf("github.pull_request.%s", action)
		} else {
			eventType = "github.pull_request"
		}
		if pr, ok := rawMap["pull_request"].(map[string]any); ok {
			payload["title"] = pr["title"]
			payload["body"] = pr["body"]
			payload["number"] = pr["number"]
			payload["html_url"] = pr["html_url"]
			if user, ok := pr["user"].(map[string]any); ok {
				payload["author"] = user["login"]
			}
		}

	default:
		eventType = fmt.Sprintf("github.%s", ghEvent)
		for k, v := range rawMap {
			payload[k] = v
		}
	}

	if repo, ok := rawMap["repository"].(map[string]any); ok {
		payload["repository"] = repo["full_name"]
	}

	externalID := fmt.Sprintf("github:%s:%s", ghEvent, deliveryID)
	return event.New(sourceID, externalID, eventType, payload, map[string]any{
		"github_event":    ghEvent,
		"github_delivery": deliveryID,
		"action":          action,
	})
}
