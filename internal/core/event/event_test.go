package event

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNewEventValidation(t *testing.T) {
	// Missing sourceID
	_, err := New("", "ext-123", "test.type", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "source_id is required") {
		t.Fatalf("expected source_id required error, got %v", err)
	}

	// Missing externalEventID
	_, err = New("src-123", "", "test.type", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "external_event_id is required") {
		t.Fatalf("expected external_event_id required error, got %v", err)
	}
}

func TestNewEventDefaults(t *testing.T) {
	ev, err := New("src-1", "ext-1", "", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ev.ID == "" {
		t.Error("expected generated UUID for event ID")
	}
	if ev.EventType != "default" {
		t.Errorf("expected default event type 'default', got %s", ev.EventType)
	}
	if string(ev.Payload) != "{}" {
		t.Errorf("expected payload to default to '{}', got %s", string(ev.Payload))
	}
	if string(ev.Metadata) != "{}" {
		t.Errorf("expected metadata to default to '{}', got %s", string(ev.Metadata))
	}
	if ev.CreatedAt.IsZero() {
		t.Error("expected CreatedAt timestamp to be set")
	}
}

func TestNewEventPayloadFormats(t *testing.T) {
	// 1. Struct payload
	type customPayload struct {
		Message string `json:"message"`
		Count   int    `json:"count"`
	}
	p := customPayload{Message: "hello", Count: 42}
	ev1, err := New("src-1", "ext-1", "type.struct", p, nil)
	if err != nil {
		t.Fatalf("failed struct payload: %v", err)
	}
	var out customPayload
	if err := json.Unmarshal(ev1.Payload, &out); err != nil || out.Count != 42 {
		t.Errorf("failed to unmarshal struct payload: %v", err)
	}

	// 2. Raw JSON string
	rawJSON := `{"key":"value","items":[1,2,3]}`
	ev2, err := New("src-1", "ext-2", "type.raw", rawJSON, nil)
	if err != nil {
		t.Fatalf("failed raw json payload: %v", err)
	}
	if string(ev2.Payload) != rawJSON {
		t.Errorf("expected %s, got %s", rawJSON, string(ev2.Payload))
	}

	// 3. Raw bytes
	rawBytes := []byte(`{"status":"ok"}`)
	ev3, err := New("src-1", "ext-3", "type.bytes", rawBytes, nil)
	if err != nil {
		t.Fatalf("failed bytes payload: %v", err)
	}
	if string(ev3.Payload) != string(rawBytes) {
		t.Errorf("expected %s, got %s", string(rawBytes), string(ev3.Payload))
	}

	// 4. Plain text string (should be safely JSON encoded)
	plainText := "plain non-json text"
	ev4, err := New("src-1", "ext-4", "type.text", plainText, nil)
	if err != nil {
		t.Fatalf("failed plain text payload: %v", err)
	}
	if !json.Valid(ev4.Payload) {
		t.Errorf("expected valid json payload for plain text, got %s", string(ev4.Payload))
	}
}
